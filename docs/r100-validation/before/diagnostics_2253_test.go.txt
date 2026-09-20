package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func Test2253SameServerAliasObjectResponse(t *testing.T) {
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", region: "TENCENT", rsoPlatform: "HN10", platformProbe: true}
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != "GET" || req.URL.Path != "/lol-summoner/v1/alias/lookup" || req.URL.Query().Get("tagLine") != "11401" {
			t.Fatalf("unexpected lookup %s %s", req.Method, req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"puuid":"fixture-same-server-player"}`))}, nil
	})}
	ref, err := (&app{}).resolveTencentRiotID(context.Background(), client, "同区测试", "11401", "HN10")
	if err != nil || ref.PlayerRef != "fixture-same-server-player" || ref.ServerID != "HN10" {
		t.Fatalf("ref=%+v err=%v", ref, err)
	}
}

func Test2253ActionZeroHoverThenLock(t *testing.T) {
	f := newR78ChampSelectFixture(t, "normal", "ban", 0, []int64{141}, map[int64]champSelectGridSelectionStatus{141: {}})
	f.session.Actions[0][0].ID = 0
	f.enable("normal", "ban", []int64{141})
	settings := f.runner.currentWatch()
	group := settings.ChampSelect.Groups["normal"]
	group.Ban.DelayMS = 30
	settings.ChampSelect.Groups["normal"] = group
	f.runner.apply(settings)
	first := f.waitAfterEvaluate(t)
	if first.Completed {
		t.Fatal("first request must only hover")
	}
	wait2253Submitted(t, f.runner, 0, false)
	f.session.Actions[0][0].ChampionID = 141
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	// Repeated hover events during the lock delay must not imply manual takeover.
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	second := f.waitPatch(t)
	if !second.Completed || second.ChampionID != 141 {
		t.Fatalf("lock=%+v", second)
	}
}

func Test2253AliasResponseSupportsOnlyObjectAndArray(t *testing.T) {
	var aliases lcuSummonerAliasResponse
	for _, body := range []string{`{"puuid":"fixture-player"}`, `[{"puuid":"fixture-player"}]`} {
		if err := json.Unmarshal([]byte(body), &aliases); err != nil || len(aliases) != 1 {
			t.Fatalf("shape %s: %v", body, err)
		}
	}
	if err := json.Unmarshal([]byte(`"not-an-alias"`), &aliases); err == nil {
		t.Fatal("unexpected alias shape accepted")
	}
}

func Test2253SameServerAliasRejectsMultiplePlayers(t *testing.T) {
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", region: "TENCENT", rsoPlatform: "HN10", platformProbe: true}
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"puuid":"fixture-player-one"},{"puuid":"fixture-player-two"}]`))}, nil
	})}
	ref, err := (&app{}).resolveTencentRiotID(context.Background(), client, "同区测试", "11401", "HN10")
	if err == nil || ref.PlayerRef != "" {
		t.Fatalf("ambiguous identity selected: %+v, %v", ref, err)
	}
}

func Test2253PendingPickRechecksTurnBeforeWriting(t *testing.T) {
	f := newR78ChampSelectFixture(t, "normal", "pick", 0, []int64{61}, map[int64]champSelectGridSelectionStatus{61: {}})
	f.enable("normal", "pick", []int64{61})
	var reads atomic.Int32
	transport := f.client.http.Transport
	f.client.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == champSelectAPI+"/session" && reads.Add(1) > 1 {
			copy := *f.session
			copy.Actions = [][]lcuChampSelectAction{{{ID: 42, ActorCellID: 8, Type: "pick", IsInProgress: true}}}
			data, _ := json.Marshal(copy)
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
		}
		return transport.RoundTrip(req)
	})
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f.runner.mu.Lock()
		pending := len(f.runner.pending)
		f.runner.mu.Unlock()
		if pending == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if reads.Load() < 2 || f.patches.Load() != 0 {
		t.Fatalf("reads=%d writes=%d", reads.Load(), f.patches.Load())
	}
}

func Test2253CustomLegacyBanAndPickUseSameVerifiedModule(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{}, map[int64]champSelectGridSelectionStatus{141: {}, 61: {}})
	f.session.GameID, f.session.QueueID = 12345, 3110
	f.session.Actions[0][0].ID = 0
	f.enable("practice", "ban", []int64{141})
	f.enable("practice", "pick", []int64{61})
	f.runner.setCustomSession(true)
	settings := f.runner.currentWatch()
	settings.MasterEnabled = false
	f.runner.apply(settings)
	banJSON, _ := json.Marshal(f.session)
	pick := *f.session
	pick.Actions = [][]lcuChampSelectAction{{{ID: 5, Type: "pick", ActorCellID: 7, IsInProgress: true}}}
	pickJSON, _ := json.Marshal(pick)
	var picking atomic.Bool
	var hover atomic.Int64
	writes := make(chan string, 8)
	transport := f.client.http.Transport
	f.client.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := ""
		switch req.URL.Path {
		case champSelectLegacyAPI + "/implementation-active":
			body = `true`
		case champSelectLegacyAPI + "/session":
			body = string(banJSON)
			if picking.Load() {
				body = string(pickJSON)
			}
			var session lcuChampSelectSession
			_ = json.Unmarshal([]byte(body), &session)
			session.Actions[0][0].ChampionID = hover.Load()
			encoded, _ := json.Marshal(session)
			body = string(encoded)
		case champSelectLegacyAPI + "/bannable-champion-ids":
			body = `[141]`
		case champSelectLegacyAPI + "/pickable-champion-ids":
			body = `[61]`
		default:
			if req.Method == "PATCH" {
				writes <- req.URL.Path
				if !strings.HasPrefix(req.URL.Path, champSelectLegacyAPI+"/session/actions/") {
					t.Error("custom action sent to the wrong module")
				}
				clone := req.Clone(req.Context())
				urlCopy := *req.URL
				urlCopy.Path = strings.Replace(urlCopy.Path, champSelectLegacyAPI, champSelectAPI, 1)
				clone.URL = &urlCopy
				return transport.RoundTrip(clone)
			}
			return transport.RoundTrip(req)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	for _, step := range []struct {
		id        int64
		side      string
		completed bool
	}{{141, "ban", false}, {141, "ban", true}, {61, "pick", false}, {61, "pick", true}} {
		if step.side == "pick" && !picking.Load() {
			picking.Store(true)
			hover.Store(0)
		}
		request := f.waitAfterEvaluate(t)
		if request.Type != step.side || request.ChampionID != step.id || request.Completed != step.completed {
			t.Fatalf("step=%+v request=%+v", step, request)
		}
		<-writes
		wait2253Submitted(t, f.runner, map[string]int64{"ban": 0, "pick": 5}[step.side], step.completed)
		hover.Store(step.id)
	}
	if f.runner.champSelectSnapshot().ActionType != "pick" || !f.runner.customPaused() {
		t.Fatal("wrong current action or custom watch automation resumed")
	}
}

func wait2253Submitted(t *testing.T, r *watchRunner, id int64, completed bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		record, exists := r.champSelect.submitted[id]
		busy := r.champSelect.inFlight[id]
		r.mu.Unlock()
		if exists && record.Completed == completed && !busy {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("submission not recorded")
}

func Test2253LegacySessionRequiresExactGameAndPlayer(t *testing.T) {
	cell := int64(0)
	session := lcuChampSelectSession{GameID: 55, QueueID: 3110, LocalPlayerCellID: &cell}
	for _, tc := range []struct {
		name       string
		active     bool
		game, cell int64
	}{
		{"inactive", false, 55, 0}, {"other-game", true, 56, 0}, {"other-player", true, 55, 1}, {"same", true, 55, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture"}
			client.http = &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				var body []byte
				if strings.HasSuffix(req.URL.Path, "/implementation-active") {
					body, _ = json.Marshal(tc.active)
				} else {
					body, _ = json.Marshal(lcuChampSelectSession{GameID: tc.game, LocalPlayerCellID: &tc.cell})
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}
			_, source, _ := champSelectSessionSource(context.Background(), client, session)
			if (source == champSelectLegacyAPI) != (tc.name == "same") {
				t.Fatalf("source=%s", source)
			}
		})
	}
}

func Test2253SubmitRechecksOriginalCellAndQueue(t *testing.T) {
	for _, kind := range []string{"cell", "queue"} {
		t.Run(kind, func(t *testing.T) {
			f := newR78ChampSelectFixture(t, "normal", "pick", 0, []int64{61}, map[int64]champSelectGridSelectionStatus{61: {}})
			f.enable("normal", "pick", []int64{61})
			decision := champSelectDecision{Action: champSelectActionPick, ActionID: 42, ChampionID: 61, SessionAPI: champSelectAPI, QueueID: 400, LocalCellID: 7}
			f.runner.mu.Lock()
			f.runner.champSelect.groupID = "normal"
			f.runner.mu.Unlock()
			if kind == "cell" {
				other := int64(8)
				f.session.LocalPlayerCellID = &other
				f.session.Actions[0][0].ActorCellID = other
			} else {
				f.session.QueueID = 420
			}
			if f.runner.champSelectRequestStillCurrent(context.Background(), f.client, decision) {
				t.Fatal("changed scope accepted")
			}
		})
	}
}
