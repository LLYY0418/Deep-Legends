package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func r175App(t *testing.T) *app {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	return &app{storage: trackTestStore(t, &localStore{root: root})}
}
func r175Events(t *testing.T, a *app, event string) []map[string]any {
	t.Helper()
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		if row["event"] == event {
			rows = append(rows, row)
		}
	}
	return rows
}
func r175WaitEvent(t *testing.T, a *app, event string) []map[string]any {
	t.Helper()
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		if rows := r175Events(t, a, event); len(rows) > 0 {
			return rows
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("missing %s", event)
	return nil
}

// Exercise the real cache/LCU/playerlist/enrichment pipeline, not the recovery
// helper in isolation. Every fake endpoint uses synthetic identities only.
func TestR175RecoveredRosterEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name               string
		queue              int64
		anonymous, partial bool
		want               int
		reason             string
	}{
		{"anonymous-ally", 2400, true, false, 10, "anonymous-placeholder"},
		{"ranked-anonymous-ally", 440, true, false, 10, "anonymous-placeholder"},
		{"named-ally", 420, false, false, 10, "resolved"},
		{"partial-ally", 420, false, true, 9, "partial"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := r175App(t)
			var mu sync.Mutex
			history := map[string]int{}
			summonerCalls := 0
			teamOne, teamTwo := []any{}, []any{}
			live := []any{}
			for i := 0; i < 10; i++ {
				team := "ORDER"
				if i >= 5 {
					team = "CHAOS"
				}
				raw := map[string]any{"puuid": r161Ref(i), "gameName": fmt.Sprintf("Player%d", i), "tagLine": "CN1"}
				if i != 4 && !(tc.partial && i == 3) {
					if i < 5 {
						teamOne = append(teamOne, raw)
					} else {
						teamTwo = append(teamTwo, raw)
					}
				}
				riotID := fmt.Sprintf("Player%d#CN1", i)
				if tc.anonymous && i == 4 {
					riotID = ""
				}
				position := []string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}[i%5]
				live = append(live, map[string]any{"riotId": riotID, "team": team, "position": position})
			}
			mode := "CLASSIC"
			mapID := 11
			if tc.queue == 2400 {
				mode = "KIWI"
				mapID = 12
			}
			game := map[string]any{"gameData": map[string]any{"gameId": 175, "queue": map[string]any{"id": tc.queue, "mapId": mapID, "gameMode": mode}, "teamOne": teamOne, "teamTwo": teamTwo}}
			client := &LCUClient{baseURL: "http://lcu.local", token: "test", region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
			client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				status := 200
				var value any
				switch {
				case r.URL.Path == "/lol-gameflow/v1/session":
					value = game
				case r.URL.Path == "/lol-summoner/v1/alias/lookup":
					var i int
					fmt.Sscanf(r.URL.Query().Get("gameName"), "Player%d", &i)
					if tc.partial && i == 3 {
						status = 404
						value = map[string]any{}
					} else {
						value = []any{map[string]any{"puuid": r161Ref(i)}}
					}
				case strings.HasPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
					mu.Lock()
					summonerCalls++
					mu.Unlock()
					ref := strings.TrimPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/")
					value = map[string]any{"puuid": ref, "gameName": "Player" + fmt.Sprint(int(ref[0]-'a')), "tagLine": "CN1"}
				case strings.Contains(r.URL.Path, "/lol-match-history/"):
					mu.Lock()
					history[r.URL.Path]++
					mu.Unlock()
					value = map[string]any{"games": map[string]any{"games": []any{}}}
				default:
					status = 404
					value = map[string]any{}
				}
				data, _ := json.Marshal(value)
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
			})}
			data, _ := json.Marshal(live)
			a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return data, 200, nil }
			t.Cleanup(func() { a.stopMayhemSampler("test-end") })
			response := a.cachedGameplayLive(context.Background(), client, Summoner{PUUID: r161Ref(0), GameName: "Player0", TagLine: "CN1"}, "InProgress")
			if !response.Available || len(response.Players) != tc.want {
				t.Fatalf("roster count=%d available=%v, want %d", len(response.Players), response.Available, tc.want)
			}
			selfTeam := int64(0)
			allies := 0
			var recovered gameplayLivePlayer
			for _, p := range response.Players {
				if p.IsCurrent {
					selfTeam = p.TeamID
				}
				if p.TeamID == 100 {
					allies++
				}
				if p.Hidden || p.GameName == "Player4" {
					recovered = p
				}
			}
			wantAllies := 5
			if tc.partial {
				wantAllies = 4
			}
			if selfTeam != 100 || allies != wantAllies || recovered.TeamID != selfTeam {
				t.Fatalf("recovered player on wrong team: self=%d allies=%d recovered=%#v", selfTeam, allies, recovered)
			}
			mu.Lock()
			historyCount := 0
			for _, n := range history {
				historyCount += n
			}
			namedRequests := history["/lol-match-history/v1/products/lol/"+r161Ref(4)+"/matches"]
			mu.Unlock()
			if historyCount != tc.want-boolInt(tc.anonymous) {
				t.Fatalf("history requests=%d, want %d (anonymous must not load history)", historyCount, tc.want-boolInt(tc.anonymous))
			}
			if tc.anonymous {
				if !recovered.Hidden || recovered.PlayerRef != "" || recovered.HistoryState != "unavailable" || summonerCalls != 9 {
					t.Fatalf("anonymous identity/history escaped guards: %#v calls=%d", recovered, summonerCalls)
				}
				encoded, _ := json.MarshalIndent(response, "", "  ")
				if os.Getenv("R175_WRITE_FIXTURE") == "1" && tc.queue == 2400 {
					if err := os.WriteFile("testdata/r175-anonymous-live.json", append(encoded, '\n'), 0600); err != nil {
						t.Fatal(err)
					}
				}
				fixture, err := os.ReadFile("testdata/r175-anonymous-live.json")
				if err != nil {
					t.Fatal(err)
				}
				var saved gameplayLiveResponse
				if err = json.Unmarshal(fixture, &saved); err != nil {
					t.Fatal(err)
				}
				if len(saved.Players) != 10 || !saved.Players[9].Hidden || saved.Players[9].TeamID != recovered.TeamID || saved.Players[9].PlayerRef != "" {
					t.Fatal("frontend fixture diverged from real anonymous response")
				}
			} else if namedRequests != 1 || recovered.HistoryState != "empty" || recovered.reference.PlayerRef != r161Ref(4) {
				t.Fatalf("named recovery did not load history: requests=%d state=%s", namedRequests, recovered.HistoryState)
			}
			shape := r175Events(t, a, "live_roster_shape")
			if len(shape) == 0 || shape[len(shape)-1]["players"] != float64(tc.want) {
				t.Fatalf("missing complete roster diagnostic: %#v", shape)
			}
			teams, ok := shape[len(shape)-1]["team_counts"].(map[string]any)
			if !ok || teams["100"] != float64(wantAllies) || teams["200"] != float64(5) {
				t.Fatalf("roster diagnostic teams=%#v", teams)
			}
			if tc.partial {
				for _, player := range response.Players {
					if player.GameName == "Player3" {
						t.Fatal("unresolved named player was fabricated")
					}
				}
			}
			recovery := r175Events(t, a, "live_roster_recovery")
			if len(recovery) == 0 || recovery[0]["reason"] != tc.reason {
				t.Fatalf("recovery reason=%#v", recovery)
			}
			if events := r175Events(t, a, "backend_panic"); len(events) != 0 {
				t.Fatalf("real loading panicked: %#v", events)
			}
		})
	}
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestR175CachePanicReleasesAllWaiters(t *testing.T) {
	a := r175App(t)
	client := &LCUClient{}
	entered, release := make(chan struct{}), make(chan struct{})
	a.gameplayLiveLoader = func(context.Context, *LCUClient, Summoner, string) gameplayLiveResponse {
		close(entered)
		<-release
		panic("fake-puuid-secret")
	}
	result := make(chan gameplayLiveResponse, 3)
	request := func() {
		var response gameplayLiveResponse
		defer func() {
			if recover() != nil {
				response = gameplayLiveResponse{Phase: "InProgress"}
			}
			result <- response
		}()
		response = a.cachedGameplayLive(context.Background(), client, Summoner{}, "InProgress")
	}
	go request()
	<-entered
	go request()
	go request()
	// The two followers must have reached the flight before releasing the loader.
	time.Sleep(30 * time.Millisecond)
	close(release)
	for i := 0; i < 3; i++ {
		select {
		case got := <-result:
			if got.Phase != "InProgress" || got.Available || len(got.Players) != 0 {
				t.Fatalf("panic response=%#v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("panic stranded a flight waiter")
		}
	}
	rows := r175WaitEvent(t, a, "backend_panic")
	if len(rows) != 1 || rows[0]["site"] != "cachedGameplayLive" || rows[0]["panic_kind"] != "other" {
		t.Fatalf("panic evidence=%#v", rows)
	}
	a.gameplayLiveLoader = func(context.Context, *LCUClient, Summoner, string) gameplayLiveResponse {
		return gameplayLiveResponse{Phase: "InProgress", Available: true}
	}
	if got := a.cachedGameplayLive(context.Background(), client, Summoner{}, "InProgress"); !got.Available {
		t.Fatal("flight not cleared for retry")
	}
}
func TestR175PrewarmPanicIsContained(t *testing.T) {
	a := r175App(t)
	client := &LCUClient{}
	a.lcu = client
	a.gameplayPrewarmHook = func() { var values []int; index := 1; _ = values[index] }
	a.observeGameplayPhase(context.Background(), client, "InProgress")
	rows := r175WaitEvent(t, a, "backend_panic")
	if rows[0]["site"] != "live-prewarm" || rows[0]["panic_kind"] != "index-out-of-range" {
		t.Fatalf("prewarm evidence=%#v", rows)
	}
}
func TestR175AuthorizedHTTPPanic(t *testing.T) {
	a := r175App(t)
	mux := http.NewServeMux()
	calls := 0
	mux.HandleFunc("GET /api/gameplay/live", a.authorized(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			panic("secret-puuid-query")
		}
		respondJSON(w, map[string]bool{"ok": true})
	}))
	first := httptest.NewRecorder()
	mux.ServeHTTP(first, httptest.NewRequest("GET", "/api/gameplay/live?puuid=secret-puuid-query", nil))
	if first.Code != 500 || !strings.Contains(first.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("HTTP panic result=%d %s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	mux.ServeHTTP(second, httptest.NewRequest("GET", "/api/gameplay/live", nil))
	if second.Code != 200 {
		t.Fatal("next HTTP request failed")
	}
	rows := r175Events(t, a, "backend_panic")
	if len(rows) != 1 || rows[0]["site"] != "/api/gameplay/live" {
		t.Fatalf("unsafe/missing route template: %#v", rows)
	}
}
func TestR175PanicDiagnosticPrivacyAndKinds(t *testing.T) {
	event := backendPanicEvent("privacy-test", "fake-puuid-secret")
	keys := map[string]bool{}
	for key := range event {
		keys[key] = true
	}
	if !reflect.DeepEqual(keys, map[string]bool{"event": true, "site": true, "panic_kind": true, "frames": true}) {
		t.Fatalf("unexpected keys: %#v", keys)
	}
	encoded, _ := json.Marshal(event)
	if strings.Contains(string(encoded), "fake-puuid-secret") {
		t.Fatal("panic value leaked")
	}
	frames := event["frames"].([]string)
	if len(frames) == 0 || len(frames) > 8 {
		t.Fatalf("frames=%#v", frames)
	}
	for _, f := range frames {
		if !panicFrameName.MatchString(f) {
			t.Fatalf("unsafe frame %q", f)
		}
	}
	for name, fn := range map[string]func(){"index-out-of-range": func() { var v []int; i := 1; _ = v[i] }, "nil-pointer": func() { var p *int; _ = *p }, "slice-bounds": func() { v := []int{1}; i := 2; _ = v[:i] }, "type-assertion": func() { var v any = 1; _ = v.(string) }, "closed-channel": func() { c := make(chan int); close(c); c <- 1 }, "other": func() { panic("fake-puuid-secret") }} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if got := backendPanicKind(recover()); got != name {
					t.Fatalf("kind=%s want %s", got, name)
				}
			}()
			fn()
			t.Fatal("fixture did not panic")
		})
	}
}
func TestR175EveryBackendGoroutineHasItsOwnRecovery(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			g, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			fn, ok := g.Call.Fun.(*ast.FuncLit)
			if !ok {
				t.Errorf("unprotected named goroutine %s", fs.Position(g.Pos()))
				return true
			}
			guarded := false
			for _, stmt := range fn.Body.List {
				d, ok := stmt.(*ast.DeferStmt)
				if !ok {
					continue
				}
				switch callee := d.Call.Fun.(type) {
				case *ast.Ident:
					guarded = callee.Name == "recoverPanic"
				case *ast.SelectorExpr:
					guarded = callee.Sel.Name == "recoverPanic"
				}
				if guarded {
					break
				}
			}
			if !guarded {
				t.Errorf("unprotected goroutine %s", fs.Position(g.Pos()))
			}
			return true
		})
	}
}

func TestR175PanicInPoolJobsDoesNotStrandProducer(t *testing.T) {
	a := r175App(t)
	previous := panicDiagnosticStore.Load()
	panicDiagnosticStore.Store(a.storage)
	defer panicDiagnosticStore.Store(previous)
	values := make([]int, 100)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		proParallelLimit(context.Background(), values, 4, func(int) { panic("fake-puuid-pool-secret") })
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("panicking pool jobs stranded producer")
	}
	rows := r175Events(t, a, "backend_panic")
	if len(rows) != len(values) {
		t.Fatalf("jobs swallowed panic evidence: %d", len(rows))
	}
	for _, row := range rows {
		if row["site"] != "proParallelLimit.job" || row["panic_kind"] != "other" {
			t.Fatalf("unexpected pool event=%#v", row)
		}
	}
	encoded, _ := json.Marshal(rows)
	if strings.Contains(string(encoded), "fake-puuid-pool-secret") {
		t.Fatal("pool panic value leaked")
	}
}
