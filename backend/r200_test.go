package main

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type r200Reply struct {
	status int
	body   string
}
type r200Fixture struct {
	*executionFixture
	replies map[string]r200Reply
	reads   []string
}

func newR200Fixture(t *testing.T, cards string) *r200Fixture {
	f := newExecutionFixture(t)
	f.session.QueueID = 2400
	f.session.Timer.Phase = "BAN_PICK"
	f.session.AllowSubsetChampionPicks = true
	f.session.BenchEnabled = true
	f.session.Actions = [][]lcuChampSelectAction{{{ID: 5, Type: "pick", ActorCellID: 7, IsInProgress: true}}}
	f.picks = []int64{22, 136, 48, 57, 13, 75, 99, 101}
	f.grid = nil
	for _, id := range f.picks {
		f.grid = append(f.grid, champSelectGridChampion{ID: id, Owned: true})
	}
	s := f.r.currentWatch()
	g := s.ChampSelect.Groups["aram"]
	g.Pick.Enabled = true
	g.Pick.Strategy = "lock-now"
	g.Pick.Champions["default"] = []int64{22, 136, 48, 57, 13}
	g.Bench.Enabled = true
	g.Bench.HoldMS = 0
	s.ChampSelect.Groups["aram"] = g
	f.r.apply(s)
	v := &r200Fixture{executionFixture: f, replies: map[string]r200Reply{"/help": {200, `{"functions":[]}`}, champSelectSubsetPaths[0]: {200, cards}, champSelectSubsetPaths[1]: {404, `{}`}}}
	original := f.c.http.Transport
	f.c.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		f.mu.Lock()
		v.reads = append(v.reads, req.URL.Path)
		reply, ok := v.replies[req.URL.Path]
		f.mu.Unlock()
		if ok {
			return &http.Response{StatusCode: reply.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(reply.body))}, nil
		}
		return original.RoundTrip(req)
	})
	return v
}
func (v *r200Fixture) setReply(path string, status int, body string) {
	v.mu.Lock()
	v.replies[path] = r200Reply{status, body}
	v.mu.Unlock()
}
func (v *r200Fixture) readCount(path string) int {
	v.mu.Lock()
	defer v.mu.Unlock()
	n := 0
	for _, p := range v.reads {
		if p == path {
			n++
		}
	}
	return n
}
func (v *r200Fixture) hasTrace(stage, reason string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, e := range v.events {
		if e["event"] == "champselect_trace" && e["stage"] == stage && e["reason"] == reason {
			return true
		}
	}
	return false
}
func (v *r200Fixture) requireTrace(t *testing.T, stage, reason string) {
	t.Helper()
	if !v.hasTrace(stage, reason) {
		t.Fatalf("missing %s/%s trace: %v", stage, reason, v.events)
	}
}
func TestR200OfferedSubsetOnly(t *testing.T) {
	v := newR200Fixture(t, `[136,75,99]`)
	v.tick(t)
	if v.count() != 1 || v.last().Body["championId"] != float64(136) {
		t.Fatalf("picked outside cards: %v", v.patches)
	}
	if v.readCount(champSelectAPI+"/pickable-champion-ids") != 0 {
		t.Fatal("card mode used full pickable list")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	source, candidates := false, false
	for _, e := range v.events {
		if e["stage"] == "source" && reflect.DeepEqual(e["subset_ids"], []int64{136, 75, 99}) && e["subset_mode"] == true && e["subset_source"] == champSelectSubsetPaths[0] {
			source = true
		}
		if e["event"] == "champselect_candidates" && e["available_count"] == 3 && e["availability_source"] == "subset" {
			candidates = true
		}
	}
	if !source || !candidates {
		t.Fatalf("missing subset diagnostics: source=%t candidates=%t", source, candidates)
	}
}
func TestR200NoPoolChampion(t *testing.T) {
	v := newR200Fixture(t, `[75,99,101]`)
	v.tick(t)
	v.tick(t)
	if v.count() != 0 {
		t.Fatal("picked outside cards")
	}
	v.requireTrace(t, "subset", "no-pool-champion")
	n := 0
	for _, row := range v.r.champSelectSnapshot().Records {
		if row.Message == "开局卡片中没有选用序列里的英雄" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("duplicate/missing card record: %d", n)
	}
}
func TestR200SubsetUnavailableNeverFallsBack(t *testing.T) {
	v := newR200Fixture(t, `{}`)
	v.setReply(champSelectSubsetPaths[0], 500, `{}`)
	v.tick(t)
	v.tick(t)
	if v.count() != 0 || v.readCount(champSelectAPI+"/pickable-champion-ids") != 0 {
		t.Fatalf("fallback wrote/read full heroes: %v", v.reads)
	}
	v.requireTrace(t, "subset", "subset-unavailable")
	if v.readCount("/help") != 1 {
		t.Fatal("help read more than once per session")
	}
}
func TestR200SubsetDiscoveryAndParsing(t *testing.T) {
	for _, tc := range []struct {
		name, help, first, second string
		status                    int
		source                    string
	}{
		{"help-object", `{"functions":[{"name":"GET /lol-champ-select/v1/my-subset"}]}`, `[136]`, `[]`, 200, "/lol-champ-select/v1/my-subset"},
		{"first-integers", `{}`, `[136,75]`, `[]`, 200, champSelectSubsetPaths[0]},
		{"second-objects", `{}`, `{}`, `[{"championId":136},{"championId":75}]`, 404, champSelectSubsetPaths[1]},
		{"malformed-first", `{}`, `[1.5]`, `[136]`, 200, champSelectSubsetPaths[1]},
		{"non-200-first", `{}`, `[22]`, `[136]`, 201, champSelectSubsetPaths[1]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newR200Fixture(t, tc.first)
			v.setReply("/help", 200, tc.help)
			v.setReply(champSelectSubsetPaths[0], tc.status, tc.first)
			v.setReply(champSelectSubsetPaths[1], 200, tc.second)
			if tc.source == "/lol-champ-select/v1/my-subset" {
				v.setReply(tc.source, 200, `[{"championId":136}]`)
			}
			v.tick(t)
			if v.count() != 1 || v.last().Body["championId"] != float64(136) {
				t.Fatalf("discovery failed: %v", v.patches)
			}
			v.r.mu.Lock()
			source := v.r.champSelect.subsetEndpoint
			v.r.mu.Unlock()
			if source != tc.source {
				t.Fatal(source)
			}
			if v.readCount("/help") != 1 || v.readCount("/swagger/v3/openapi.json") != 0 {
				t.Fatal("incorrect discovery")
			}
			probe := false
			matches := false
			for _, e := range v.events {
				if e["event"] == "subset_probe" && e["path"] == source && e["status_code"] == int64(200) && e["valid"] == true {
					probe = true
				}
				if e["event"] == "subset_help_matches" {
					matches = true
				}
			}
			if !probe || !matches {
				t.Fatalf("missing discovery diagnostics: %v", v.events)
			}
		})
	}
	for _, raw := range []string{`null`, `{}`, `[null]`, `["136"]`, `[1.5]`, `[{"championId":null}]`, `[{"id":136}]`, `[136,{"championId":75}]`} {
		if ids, _ := champSelectSubsetIDs([]byte(raw)); ids != nil {
			t.Fatalf("accepted invalid subset %s: %v", raw, ids)
		}
	}
	ids, _ := champSelectSubsetIDs([]byte(`[]`))
	if ids == nil || len(ids) != 0 {
		t.Fatal("valid empty cards unavailable")
	}
	paths := champSelectSubsetHelpPaths([]byte(`{"functions":[{"method":"POST","path":"/lol-champ-select/v1/subset"},{"method":"GET","path":"/lol-other/v1/subset"},{"method":"GET","path":"/lol-champ-select/v1/subset/{id}"},{"httpMethod":"GET","name":"mySubset","path":"/lol-lobby-team-builder/v1/cards"}]}`))
	if !reflect.DeepEqual(paths, []string{"/lol-lobby-team-builder/v1/cards"}) {
		t.Fatal(paths)
	}
}
func TestR200ChangedCardsPreferEarlierPoolHero(t *testing.T) {
	v := newR200Fixture(t, `[57,75]`)
	s := v.r.currentWatch()
	g := s.ChampSelect.Groups["aram"]
	g.Pick.Strategy = "show-only"
	s.ChampSelect.Groups["aram"] = g
	v.r.apply(s)
	v.tick(t)
	if v.last().Body["championId"] != float64(57) {
		t.Fatal(v.last())
	}
	v.setReply(champSelectSubsetPaths[0], 200, `[136,57]`)
	v.tick(t)
	if v.count() != 2 || v.last().Body["championId"] != float64(136) {
		t.Fatal(v.patches)
	}
	if v.readCount("/help") != 1 {
		t.Fatal("rediscovered same session")
	}
	v.mu.Lock()
	v.session.GameID++
	v.session.Actions[0][0].ChampionID = 0
	v.session.MyTeam[0].ChampionPickIntent = 0
	v.mu.Unlock()
	v.tick(t)
	if v.readCount("/help") != 2 {
		t.Fatal("new session retained old discovery")
	}
}
func TestR200PickFailureAdvancesAndStopsAtSix(t *testing.T) {
	v := newR200Fixture(t, `[]`)
	v.session.AllowSubsetChampionPicks = false
	v.applyWrites = false
	for i := 0; i < 9; i++ {
		v.age()
		v.tick(t)
	}
	if v.count() != 6 {
		t.Fatalf("requests=%d: %v", v.count(), v.patches)
	}
	for i, w := range v.patches {
		if w.Body["championId"] != float64([]int64{22, 22, 136, 136, 48, 48}[i]) {
			t.Fatal(v.patches)
		}
	}
	v.requireTrace(t, "confirmation", "attempt-limit")
	stopped := false
	for _, r := range v.r.champSelectSnapshot().Records {
		if strings.Contains(r.Message, "已停止尝试，请手动操作") {
			stopped = true
		}
	}
	if !stopped {
		t.Fatal("missing existing exhausted prompt")
	}
}
func (v *r200Fixture) benchSetup() {
	v.session.AllowSubsetChampionPicks = false
	v.session.Actions = nil
	v.session.Timer.Phase = "FINALIZATION"
	v.session.MyTeam[0].ChampionID = 104
	v.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 13}}
	v.r.mu.Lock()
	v.r.champSelect.benchFirstSeen[13] = time.Now().Add(-3 * time.Second)
	v.r.champSelect.benchFirstSeen[57] = time.Now().Add(-3 * time.Second)
	v.r.mu.Unlock()
}
func (v *r200Fixture) ageBench() {
	v.r.mu.Lock()
	defer v.r.mu.Unlock()
	if v.r.champSelect.benchSwap != nil {
		v.r.champSelect.benchSwap.SettledAt = time.Now().Add(-3 * time.Second)
	}
}
func TestR200BenchPostflight(t *testing.T) {
	for _, mode := range []string{"applied", "not-applied", "target-gone"} {
		t.Run(mode, func(t *testing.T) {
			v := newR200Fixture(t, `[]`)
			v.benchSetup()
			v.applyWrites = mode == "applied"
			v.tick(t)
			if v.count() != 1 {
				t.Fatal(v.patches)
			}
			if mode != "applied" {
				v.ageBench()
			}
			if mode == "target-gone" {
				v.mu.Lock()
				v.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 57}}
				v.mu.Unlock()
				v.r.mu.Lock()
				v.r.champSelect.benchFirstSeen[57] = time.Now().Add(-3 * time.Second)
				v.r.mu.Unlock()
			}
			v.tick(t)
			v.requireTrace(t, "bench-postflight", mode)
			if mode == "not-applied" {
				if v.count() != 2 {
					t.Fatal("missing bench retry", v.patches)
				}
				v.ageBench()
				v.tick(t)
				v.tick(t)
				if v.count() != 2 {
					t.Fatal("unbounded bench retry")
				}
			}
			if mode == "target-gone" && (v.count() != 2 || !strings.HasSuffix(v.last().Path, "/57")) {
				t.Fatal("gone target not advanced", v.patches)
			}
			confirmed := false
			for _, e := range v.events {
				if e["event"] == "watch_action" && e["action"] == champSelectActionBench && e["result"] == "fired" {
					confirmed = true
				}
			}
			if confirmed != (mode == "applied") {
				t.Fatalf("HTTP success incorrectly treated as confirmed: %v", v.events)
			}
		})
	}
}
func TestR200UnfinishedPickBlocksBench(t *testing.T) {
	v := newR200Fixture(t, `[]`)
	v.benchSetup()
	v.session.Actions = [][]lcuChampSelectAction{{{ID: 5, Type: "pick", ActorCellID: 7, ChampionID: 104, Completed: false}}}
	v.tick(t)
	if v.count() != 0 {
		t.Fatal("swapped before pick completed")
	}
	v.requireTrace(t, "bench-gate", "pick-unfinished")
}
func TestR200CameraSaveHelpThenProbe(t *testing.T) {
	for _, tc := range []struct {
		help   string
		status int
		want   string
	}{
		{`{"functions":[{"name":"POST /lol-game-settings/v1/save"}]}`, 204, "ok-help"},
		{`{}`, 204, "ok-probe"}, {`{}`, 404, "unsupported"}, {`{}`, 405, "unsupported"}, {`{}`, 500, "failed"},
	} {
		t.Run(tc.want+tc.help, func(t *testing.T) {
			calls := []string{}
			c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls = append(calls, req.Method+" "+req.URL.Path)
				if req.URL.Path == "/help" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.help))}, nil
				}
				if req.Method != http.MethodPost || req.URL.Path != "/lol-game-settings/v1/save" {
					t.Fatal("unexpected", req.URL)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})}}
			called, result, err := saveLCUGameSettings(context.Background(), c, func() bool { return true })
			if !called || result != tc.want || (err != nil) != (tc.status == 500) || !reflect.DeepEqual(calls, []string{"GET /help", "POST /lol-game-settings/v1/save"}) {
				t.Fatal(called, result, err, calls)
			}
		})
	}
}
