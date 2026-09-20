package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR88ArenaBanWildcardActuallyPatches(t *testing.T) {
	for _, queue := range []int64{1750, 3110, 3100} {
		t.Run(fmt.Sprint(queue), func(t *testing.T) {
			f := newExecutionFixture(t)
			f.configure(true, false, true, "lock-now")
			f.session.QueueID = queue
			f.session.Timer.Phase = "BAN_PICK"
			s := f.r.currentWatch()
			s.ChampSelect.Groups["arena"] = s.ChampSelect.Groups["practice"]
			f.r.apply(s)
			f.tick(t)
			if f.count() != 1 || f.last().Body["championId"] != float64(141) || f.last().Body["completed"] != false {
				t.Fatalf("expected verified hover PATCH: %+v", f.patches)
			}
			f.tick(t)
			if f.count() != 2 || f.last().Body["completed"] != true {
				t.Fatal("confirmed hover must lock", f.patches)
			}
			var probe map[string]any
			for _, e := range f.events {
				if e["event"] == "champselect_ban_probe" {
					probe = e
				}
			}
			if fmt.Sprint(probe["raw_head"]) != "[-1]" {
				t.Fatal(probe)
			}
		})
	}
}

func TestR88OrdinaryLiveLoadsCacheAndReuseWithoutExtendingTTL(t *testing.T) {
	var identities atomic.Int32
	ref := strings.Repeat("r", 48)
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/lol-gameflow/v1/session" {
			return response2351(map[string]any{"gameData": map[string]any{"gameId": 88, "queue": map[string]any{"id": 450, "gameMode": "ARAM"}, "teamOne": []map[string]any{{"puuid": ref, "summonerId": 88}}}}), nil
		}
		if strings.Contains(r.URL.Path, "/summoners/puuid/") {
			identities.Add(1)
			return response2351(Summoner{PUUID: ref, GameName: "test", SummonerID: 88}), nil
		}
		if strings.Contains(r.URL.Path, "/lol-match-history/") {
			return response2351(map[string]any{"games": map[string]any{"gameCount": 0, "games": []any{}}}), nil
		}
		return response2351([]any{}), nil
	})}}
	a := &app{liveClientPlayerList: func(context.Context) ([]byte, int, error) { return nil, 0, errors.New("fixture") }}
	first := a.cachedGameplayLive(context.Background(), c, Summoner{PUUID: "self"}, "InProgress")
	if !first.Available || len(first.Players) != 1 || a.liveSnapshots.at.IsZero() {
		t.Fatalf("ordinary load not cached: %+v", first)
	}
	at := a.liveSnapshots.at
	for range 3 {
		got := a.cachedGameplayLive(context.Background(), c, Summoner{PUUID: "self"}, "InProgress")
		if !got.Available || a.liveSnapshots.at != at {
			t.Fatal("hit consumed or extended cache")
		}
	}
	if identities.Load() != 1 {
		t.Fatal("roster reloaded", identities.Load())
	}
	a.liveSnapshots.at = time.Now().Add(-time.Minute)
	a.cachedGameplayLive(context.Background(), c, Summoner{PUUID: "self"}, "InProgress")
	if time.Since(a.liveSnapshots.at) < 50*time.Second {
		t.Fatal("complete in-game snapshot was rebuilt")
	}
}

func TestR88SummonerCacheCoalescesExpiresAndIsolatesClients(t *testing.T) {
	var calls atomic.Int32
	makeClient := func() *LCUClient {
		return &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			time.Sleep(5 * time.Millisecond)
			return response2351(Summoner{PUUID: strings.Repeat("p", 48), GameName: "test"}), nil
		})}}
	}
	c := makeClient()
	ref := gameplayReference{PlayerRef: strings.Repeat("p", 48)}
	var wg sync.WaitGroup
	for range 18 {
		wg.Add(1)
		go func() { defer wg.Done(); loadGameplaySummoner(c, ref) }()
	}
	wg.Wait()
	for range 3 {
		loadGameplaySummoner(c, ref)
	}
	if calls.Load() != 1 {
		t.Fatal("same identity duplicated", calls.Load())
	}
	loadGameplaySummoner(makeClient(), ref)
	if calls.Load() != 2 {
		t.Fatal("cross-client reuse")
	}
	c.gameplaySummoners.mu.Lock()
	for k, v := range c.gameplaySummoners.entries {
		v.at = time.Now().Add(-gameplaySummonerTTL)
		c.gameplaySummoners.entries[k] = v
	}
	c.gameplaySummoners.mu.Unlock()
	loadGameplaySummoner(c, ref)
	if calls.Load() != 3 {
		t.Fatal("expired identity reused")
	}
}

func TestR88ArenaRankingsOPAndPartialSchema(t *testing.T) {
	data, err := os.ReadFile("testdata/r88/yourgg-arena-rankings.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	observe := func(e map[string]any) { events = append(events, e) }
	got, err := parseYourGGArenaRankings(data, time.Now(), observe)
	if err != nil || len(got.Rows) != 173 || got.Rows[0].Grade != "OP" || len(events) != 0 {
		t.Fatalf("OP fixture failed: rows=%d err=%v events=%v", len(got.Rows), err, events)
	}
	bad := bytes.Replace(data, []byte(`"tier":"OP"`), []byte(`"tier":"XYZ"`), 1)
	got, err = parseYourGGArenaRankings(bad, time.Now(), observe)
	if err != nil || len(got.Rows) != 172 || len(events) != 1 || events[0]["field"] != "tier" || events[0]["value"] != "XYZ" {
		t.Fatalf("partial failed: %d %v %v", len(got.Rows), err, events)
	}
	var payload map[string]any
	json.Unmarshal(data, &payload)
	rows := payload["response"].(map[string]any)["champions"].([]any)
	rows[1].(map[string]any)["championId"] = rows[0].(map[string]any)["championId"]
	rows[2].(map[string]any)["averagePlacement"] = 9
	rows[3].(map[string]any)["matches"] = "changed"
	bad, _ = json.Marshal(payload)
	events = nil
	got, err = parseYourGGArenaRankings(bad, time.Now(), observe)
	if err != nil || len(got.Rows) != 170 || len(events) != 1 || events[0]["skipped_rows"] != 3 {
		t.Fatalf("row degradation failed: %d %v %v", len(got.Rows), err, events)
	}
}

func TestR88DiagnosticContractsAndBuildMarker(t *testing.T) {
	for _, body := range []string{
		`{"event":"live_refresh_client","reason":"phase","source":"sse","phaseChanged":true}`,
		`{"event":"claim_progress_client","reason":"end","claiming":false,"done":3,"total":3,"durationMs":1500}`,
		`{"event":"local_request_client","reason":"complete","endpoint":"status","startedAt":1700000000000,"completedAt":1700000000050,"durationMs":50,"httpStatus":200,"errorKind":"none"}`,
	} {
		store := newFlowDiagnosticStore(t)
		a := &app{storage: store}
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		data, _ := store.readDiagnosticLog()
		var e map[string]any
		if w.Code != 204 || json.Unmarshal(data, &e) != nil || e["build_fingerprint"] != buildFingerprint {
			t.Fatalf("contract failed: %s", data)
		}
		switch e["event"] {
		case "live_refresh_client":
			for _, key := range []string{"claiming", "done", "total", "duration_ms"} {
				if _, ok := e[key]; ok {
					t.Fatal("fabricated live field", key)
				}
			}
		case "claim_progress_client":
			if e["done"] != float64(3) || e["duration_ms"] != float64(1500) {
				t.Fatal(e)
			}
			if _, ok := e["phase_changed"]; ok {
				t.Fatal(e)
			}
		case "local_request_client":
			if e["started_at"] != float64(1700000000000) || e["duration_ms"] != float64(50) {
				t.Fatal(e)
			}
		}
	}
	store := newFlowDiagnosticStore(t)
	a := &app{storage: store}
	a.enableDiagnosticRotationSnapshot()
	a.recordAppStartDiagnostic()
	store.onDiagnosticRotation()
	data, _ := store.readDiagnosticLog()
	var markers []bool
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var e map[string]any
		json.Unmarshal(line, &e)
		if e["event"] == "app_start" {
			markers = append(markers, e["log_rotation"].(bool))
		}
	}
	if fmt.Sprint(markers) != "[false true]" {
		t.Fatal(string(data))
	}
}

func TestR88ConcurrentCachedProbeRejectsSupersededSnapshot(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var probes atomic.Int32
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/lol-gameflow/v1/session" {
			if probes.Add(1) == 1 {
				close(started)
				<-release
				return response2351(map[string]any{"gameData": map[string]any{"gameId": 1}}), nil
			}
			return response2351(map[string]any{"gameData": map[string]any{"gameId": 2, "queue": map[string]any{"id": 450, "gameMode": "ARAM"}}}), nil
		}
		return response2351([]any{}), nil
	})}}
	a := &app{liveClientPlayerList: func(context.Context) ([]byte, int, error) { return nil, 0, errors.New("fixture") }}
	a.liveSnapshots = liveSnapshotCache{client: c, identity: "self", phase: "InProgress", at: time.Now(), response: gameplayLiveResponse{GameID: 1, Available: true}}
	done := make(chan gameplayLiveResponse, 1)
	go func() { done <- a.cachedGameplayLive(context.Background(), c, Summoner{PUUID: "self"}, "InProgress") }()
	<-started
	newer := a.cachedGameplayLive(context.Background(), c, Summoner{PUUID: "self"}, "InProgress")
	close(release)
	older := <-done
	if newer.GameID != 2 || older.GameID != 2 {
		t.Fatalf("superseded snapshot escaped: first=%d late=%d", newer.GameID, older.GameID)
	}
}
