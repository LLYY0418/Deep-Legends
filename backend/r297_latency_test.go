package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Real loader and LCU HTTP fixture. Every history response is deliberately six
// seconds late, while the session endpoint remains local and immediate.
func TestR297ChampSelectFastChannelDuringSixSecondHistories(t *testing.T) {
	baseline := os.Getenv("R297_MEASURE_BASELINE") == "1"
	var selection atomic.Int64
	selection.Store(13)
	var histories atomic.Int32
	var hover atomic.Bool
	session := func() lcuChampSelectSession {
		local := int64(0)
		s := lcuChampSelectSession{GameID: 297, QueueID: 420, LocalPlayerCellID: &local}
		for i := range 10 {
			cell := int64(i)
			p := lcuChampSelectPlayer{CellID: &cell, PUUID: fmt.Sprintf("r297-player-%02d-reference", i), GameName: fmt.Sprintf("Fixture%d", i), TagLine: "TEST", ChampionID: selection.Load() + int64(i), AssignedPosition: "middle", Spell1ID: 4, Spell2ID: 14}
			if i == 0 && hover.Load() {
				p.ChampionPickIntent = selection.Load()
				p.ChampionID = 0
			}
			if i < 5 {
				s.MyTeam = append(s.MyTeam, p)
			} else {
				s.TheirTeam = append(s.TheirTeam, p)
			}
		}
		return s
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/lol-champ-select/v1/session":
			json.NewEncoder(w).Encode(session())
		case r.URL.Path == "/lol-gameflow/v1/gameflow-phase":
			io.WriteString(w, `"ChampSelect"`)
		case strings.HasPrefix(r.URL.Path, "/lol-perks/v1/recommended-pages/"):
			io.WriteString(w, `[{"recommendationId":"fixture","primaryPerkStyleId":8000,"secondaryPerkStyleId":8100,"keystone":{"id":8005},"perks":[{"id":9111},{"id":9104},{"id":8014},{"id":8139},{"id":8135}]}]`)
		case strings.HasPrefix(r.URL.Path, "/lol-game-data/assets/v1/champions/"):
			io.WriteString(w, `{"spells":[{"name":"Q","description":"Fixture"},{"name":"W"},{"name":"E"},{"name":"R"}]}`)
		case r.URL.Path == "/lol-gameflow/v1/session":
			one, two := []map[string]any{}, []map[string]any{}
			for i := range 10 {
				p := map[string]any{"puuid": fmt.Sprintf("r297-player-%02d-reference", i), "championId": selection.Load() + int64(i), "selectedPosition": "middle", "gameName": fmt.Sprintf("Fixture%d", i)}
				if i < 5 {
					one = append(one, p)
				} else {
					two = append(two, p)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{"gameId": 297, "queue": map[string]any{"id": 420, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": one, "teamTwo": two}})
		case r.URL.Path == "/lol-lobby/v2/lobby":
			io.WriteString(w, `{"gameConfig":{"queueId":420,"mapId":11,"gameMode":"CLASSIC"}}`)
		case r.URL.Path == "/lol-champ-select/v1/current-champion":
			fmt.Fprint(w, selection.Load())
		case strings.HasPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
			json.NewEncoder(w).Encode(Summoner{PUUID: strings.TrimPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/"), GameName: "Fixture", TagLine: "TEST"})
		case strings.HasPrefix(r.URL.Path, "/lol-ranked/"):
			io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"I"}]}`)
		case strings.HasPrefix(r.URL.Path, "/lol-match-history/"):
			histories.Add(1)
			timer := time.NewTimer(6 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				return
			}
			io.WriteString(w, `{"games":{"gameCount":0,"games":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
	a := &app{liveRosterConcurrencyOverride: 10}
	if os.Getenv("R297_BROWSER_PROOF") == "1" {
		a.lcu = client
		a.connected = true
		a.summoner = Summoner{PUUID: "r297-player-00-reference", GameName: "Fixture", TagLine: "TEST"}
		a.eventSubscribers = map[chan string]struct{}{}
		// Drive the same bounded event pump as real LCU events; bypassing it
		// would omit the throttle window from the browser acceptance numbers.
		pickEvents := make(chan time.Time, 128)
		pumpContext, stopPump := context.WithCancel(context.Background())
		pumpDone := make(chan struct{})
		go func() { defer close(pumpDone); a.runChampSelectLitePump(pumpContext, client, pickEvents) }()
		defer func() { stopPump(); <-pumpDone }()
		mux := http.NewServeMux()
		startLive := make(chan struct{})
		var startOnce sync.Once
		mux.HandleFunc("/fixture/start", func(w http.ResponseWriter, r *http.Request) {
			startOnce.Do(func() { close(startLive) })
			w.WriteHeader(204)
		})
		mux.HandleFunc("/api/gameplay/live", func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-startLive:
				a.handleGameplayLive(w, r)
			case <-r.Context().Done():
			}
		})
		mux.HandleFunc("/api/gameplay/client-recommendation", a.handleClientRecommendation)
		mux.HandleFunc("/api/gameplay/champselect-latency", a.handleChampSelectLatency)
		mux.HandleFunc("/api/events", a.handleEvents)
		mux.HandleFunc("/fixture/ready", func(w http.ResponseWriter, r *http.Request) {
			a.eventMu.Lock()
			ready := len(a.eventSubscribers) > 0
			a.eventMu.Unlock()
			respondJSON(w, map[string]any{"ready": ready})
		})
		mux.HandleFunc("/fixture/select", func(w http.ResponseWriter, r *http.Request) {
			id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
			selection.Store(id)
			hover.Store(r.URL.Query().Get("hover") == "1")
			select {
			case pickEvents <- time.Now():
				w.WriteHeader(204)
			case <-r.Context().Done():
			}
		})
		service := httptest.NewServer(mux)
		defer service.Close()
		cmd := exec.Command("node", "../scripts/r297-latency-browser.cjs", service.URL)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		return
	}
	subscriber := make(chan string, 128)
	a.eventSubscribers = map[chan string]struct{}{subscriber: {}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	progress := make(chan gameplayLiveResponse, 64)
	ctx = context.WithValue(ctx, liveProgressListenerKey{}, func(v gameplayLiveResponse) {
		select {
		case progress <- v:
		default:
		}
	})
	done := make(chan gameplayLiveResponse, 1)
	start := time.Now()
	go func() {
		done <- a.cachedGameplayLive(ctx, client, Summoner{PUUID: "r297-player-00-reference"}, "ChampSelect")
	}()
	select {
	case p := <-progress:
		if len(p.Players) != 10 || p.Players[0].CellID == nil {
			t.Fatal("initial cells missing")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for _, id := range []int64{1, 2, 3} {
		selection.Store(id)
		at := time.Now()
		a.publishChampSelectLite(ctx, client, at)
		raw := <-subscriber
		var lite champSelectLite
		if json.Unmarshal([]byte(raw), &lite) != nil || lite.Type != "champselect-lite" {
			t.Fatalf("bad SSE %s", raw)
		}
		if strings.Contains(raw, "Fixture") || strings.Contains(raw, "reference") || strings.Contains(raw, "puuid") {
			t.Fatal("identity in fast SSE")
		}
		if len(lite.Cells) != 10 || lite.Cells[0].ChampionID != id {
			t.Fatal("wrong selection")
		}
		ms := time.Since(at).Milliseconds()
		t.Logf("R297 event_to_sse champion=%d duration_ms=%d histories_inflight=%d", id, ms, histories.Load())
		if ms > 300 {
			t.Fatal("fast channel exceeded 300ms")
		}
		select {
		case <-done:
			t.Fatal("histories finished before fast channel")
		default:
		}
	}
	select {
	case got := <-done:
		elapsed := time.Since(start).Milliseconds()
		t.Logf("R297 full_roster_ms=%d history_requests=%d", elapsed, histories.Load())
		if elapsed < 6000 || !baseline && got.Players[0].ChampionID != 3 {
			t.Fatalf("old full response overwrote latest champion: %d", got.Players[0].ChampionID)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	before := histories.Load()
	selection.Store(22)
	cached := a.cachedGameplayLive(ctx, client, Summoner{PUUID: "r297-player-00-reference"}, "ChampSelect")
	if !baseline && (cached.Players[0].ChampionID != 22 || histories.Load() != before) {
		t.Fatalf("same-game signature failed or reloaded history: champ=%d reads=%d/%d", cached.Players[0].ChampionID, before, histories.Load())
	}
	t.Logf("R297 changed_signature_cached_history_reads=%d", histories.Load()-before)
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return []byte(`[]`), 200, nil }
	for _, phase := range []string{"GameStart", "InProgress", "EndOfGame"} {
		start := time.Now()
		got := a.cachedGameplayLive(ctx, client, Summoner{PUUID: "r297-player-00-reference"}, phase)
		ms := time.Since(start).Milliseconds()
		t.Logf("R297 phase=%s duration_ms=%d players=%d", phase, ms, len(got.Players))
		if ms > 1000 {
			t.Fatal("non-upstream phase overhead exceeded 1s")
		}
	}
	start = time.Now()
	a.liveSnapshots.invalidate()
	a.cachedGameplayLive(ctx, client, Summoner{PUUID: "r297-player-00-reference"}, "InProgress")
	t.Logf("R297 in_game_refresh_ms=%d", time.Since(start).Milliseconds())
}

func TestR297LiteChampionNameUsesLockedPickBeforeOldIntent(t *testing.T) {
	cell := int64(1)
	for _, tc := range []struct{ selected, intent, want int64 }{{1, 222, 1}, {0, 222, 222}, {-3, -3, 0}} {
		v := champSelectLiteFromSession(lcuChampSelectSession{MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: tc.selected, ChampionPickIntent: tc.intent}}})
		if len(v.Cells) != 1 || v.Cells[0].ChampionName != bundledChampionNames()[tc.want] {
			t.Fatalf("selected=%d intent=%d: %+v", tc.selected, tc.intent, v.Cells)
		}
	}
}

func TestR297ChampSelectLeadingThrottleAndTrailing(t *testing.T) {
	var mu sync.Mutex
	times := []time.Time{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		io.WriteString(w, `{"gameId":297,"myTeam":[]}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan time.Time, 100)
	finished := make(chan struct{})
	a := &app{}
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
	go func() { defer close(finished); a.runChampSelectLitePump(ctx, client, events) }()
	start := time.Now()
	events <- start
	// An event storm longer than the former tail debounce must keep delivering.
	for range 12 {
		time.Sleep(25 * time.Millisecond)
		events <- time.Now()
	}
	time.Sleep(350 * time.Millisecond)
	cancel()
	<-finished
	mu.Lock()
	defer mu.Unlock()
	if len(times) < 3 || len(times) > 5 || times[0].Sub(start) > 100*time.Millisecond {
		t.Fatalf("not leading/bounded/trailing: %v", times)
	}
	for i := 1; i < len(times); i++ {
		if times[i].Sub(times[i-1]) < 145*time.Millisecond {
			t.Fatal("unbounded SSE rate")
		}
	}
}

func TestR297DeadProxyFallbackLatency(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer target.Close()
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	address := dead.URL
	dead.Close()
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", address)
	p := newChampionProvider()
	defer p.client.CloseIdleConnections()
	rt := p.client.Transport.(*automaticNetworkTransport)
	rt.probes = map[string]string{}
	for i := range 3 {
		start := time.Now()
		res, err := p.client.Get(target.URL)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		ms := time.Since(start).Milliseconds()
		t.Logf("R297 proxy_unreachable_request=%d duration_ms=%d fallback_count=%d", i+1, ms, rt.count)
		if ms > 1000 {
			t.Fatal("local dead proxy overhead exceeded 1s")
		}
	}
}
