package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var r178Roles = []struct{ position, role, want string }{
	{"BOTTOM", "BOTTOM.PRIMARY.BOTTOM.TOP", "bottom"},
	{"JUNGLE", "JUNGLE.PRIMARY.JUNGLE.TOP.FILL", "jungle"},
	{"MIDDLE", "MIDDLE.PRIMARY.MIDDLE.JUNGLE.FILL", "middle"},
	{"MIDDLE", "MIDDLE.PRIMARY.MIDDLE.TOP.FILL", "middle"},
	{"TOP", "TOP.PRIMARY.TOP.BOTTOM", "top"},
	{"TOP", "TOP.PRIMARY.TOP.JUNGLE.FILL", "top"},
	{"UTILITY", "UTILITY.PRIMARY.UTILITY.BOTTOM", "utility"},
	{"", "UTILITY.FILL_PRIMARY.FILL.UNSELECTED.FILL", "utility"},
	{"NONE", "NONE.NONE.NONE.UNSELECTED", ""},
	{" unselected ", "FILL.PRIMARY.JUNGLE.TOP", ""},
	{" bottom ", "TOP.PRIMARY.TOP.JUNGLE.FILL", "bottom"},
	{"unknown", " middle .PRIMARY.JUNGLE.FILL", "middle"},
}

func TestR178GameflowExactPosition(t *testing.T) {
	for _, tc := range r178Roles {
		if got := normalizeGameflowPosition(tc.position, tc.role); got != tc.want {
			t.Errorf("%q + %q = %q want %q", tc.position, tc.role, got, tc.want)
		}
	}
	if got := normalizePosition("BOTTOM", "DUO_SUPPORT"); got != "utility" {
		t.Fatal("timeline support regressed", got)
	}
}
func r178JSON(value any, status int) *http.Response {
	b, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b)))}
}
func TestR178RealRosterPositionsAndPendingCache(t *testing.T) {
	a := r175App(t)
	roles := []string{r178Roles[4].role, r178Roles[1].role, r178Roles[2].role, r178Roles[0].role, r178Roles[6].role}
	positions := []string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}
	teamOne, teamTwo, live := []any{}, []any{}, []any{}
	snapshot := []gameplayLivePlayer{}
	for i := 0; i < 10; i++ {
		raw := map[string]any{"puuid": r161Ref(i), "gameName": fmt.Sprintf("Player%d", i), "tagLine": "CN1", "selectedPosition": positions[i%5], "selectedRole": roles[i%5]}
		team := "ORDER"
		if i < 5 {
			teamOne = append(teamOne, raw)
		} else {
			teamTwo = append(teamTwo, raw)
			team = "CHAOS"
		}
		live = append(live, map[string]any{"riotId": fmt.Sprintf("Player%d#CN1", i), "team": team, "position": positions[i%5]})
		if i < 5 {
			ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: r161Ref(i), ServerID: "HN1", GameName: fmt.Sprintf("Player%d", i), TagLine: "CN1"})
			snapshot = append(snapshot, gameplayLivePlayer{gameplayPlayer: gameplayPlayer{PlayerRef: ref}, Position: strings.ToLower(positions[i%5])})
		}
	}
	a.rememberLivePositionSnapshot(9009400838, snapshot)
	client := &LCUClient{baseURL: "http://lcu.invalid", token: "test", region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/lol-gameflow/v1/session":
			return r178JSON(map[string]any{"gameData": map[string]any{"gameId": 9009400838, "queue": map[string]any{"id": 440, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": teamOne, "teamTwo": teamTwo}}, 200), nil
		case strings.Contains(r.URL.Path, "/lol-match-history/"):
			return r178JSON(map[string]any{"games": map[string]any{"games": []any{}}}, 200), nil
		case strings.HasPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
			ref := strings.TrimPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/")
			return r178JSON(map[string]any{"puuid": ref, "gameName": fmt.Sprintf("Player%d", int(ref[0]-'a')), "tagLine": "CN1"}, 200), nil
		default:
			return r178JSON(map[string]any{}, 404), nil
		}
	})}
	now := time.Now()
	a.liveClientPlayerListNow = func() time.Time { return now }
	var probes int
	ready := false
	positionsReady := false
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) {
		probes++
		if !ready {
			return nil, 503, fmt.Errorf("not started")
		}
		rows := live
		if !positionsReady {
			rows = []any{}
			for i := 0; i < 10; i++ {
				team := "ORDER"
				if i >= 5 {
					team = "CHAOS"
				}
				rows = append(rows, map[string]any{"riotId": fmt.Sprintf("Player%d#CN1", i), "team": team})
			}
		}
		b, _ := json.Marshal(rows)
		return b, 200, nil
	}
	current := Summoner{PUUID: r161Ref(0), GameName: "Player0", TagLine: "CN1"}
	first := a.cachedGameplayLive(context.Background(), client, current, "InProgress")
	if len(first.Players) != 10 || !gameplayLivePositionsPending(first) || gameplayLiveSnapshotComplete(first) {
		t.Fatalf("pending roster=%#v", first)
	}
	for _, team := range []int64{100, 200} {
		seen := map[string]bool{}
		for _, p := range first.Players {
			if p.TeamID == team {
				seen[p.Position] = true
			}
		}
		if len(seen) != 5 {
			t.Fatalf("team %d positions=%v", team, seen)
		}
	}
	sources, dup := liveFinalPositionShape(first.Players)
	if sources["snapshot"] != 5 || sources["gameflow"] != 5 || dup["100"] != 0 || dup["200"] != 0 {
		t.Fatal(sources, dup)
	}
	events := r175Events(t, a, "live_position_shape")
	if len(events) != 1 || events[0]["position_source_counts"] == nil || events[0]["team_duplicate_positions"] == nil {
		t.Fatal(events)
	}
	// Expire short pending cache; a formerly immutable complete roster must load again.
	a.liveSnapshots.mu.Lock()
	a.liveSnapshots.at = time.Now().Add(-4 * time.Second)
	a.liveSnapshots.mu.Unlock()
	now = now.Add(10 * time.Second)
	ready = true
	partial := a.cachedGameplayLive(context.Background(), client, current, "InProgress")
	if !gameplayLivePositionsPending(partial) {
		t.Fatal("10 player list without positions falsely ready")
	}
	a.liveSnapshots.mu.Lock()
	a.liveSnapshots.at = time.Now().Add(-4 * time.Second)
	a.liveSnapshots.mu.Unlock()
	now = now.Add(10 * time.Second)
	positionsReady = true
	second := a.cachedGameplayLive(context.Background(), client, current, "InProgress")
	sources, dup = liveFinalPositionShape(second.Players)
	if gameplayLivePositionsPending(second) || !gameplayLiveSnapshotComplete(second) || sources["liveclient"] != 10 || probes != 3 {
		t.Fatalf("ready pending=%v complete=%v sources=%v probes=%d", gameplayLivePositionsPending(second), gameplayLiveSnapshotComplete(second), sources, probes)
	}
	a.liveSnapshots.mu.Lock()
	a.liveSnapshots.at = time.Now().Add(-time.Minute)
	a.liveSnapshots.mu.Unlock()
	_ = a.cachedGameplayLive(context.Background(), client, current, "InProgress")
	if probes != 3 {
		t.Fatal("ready must stay silent", probes)
	}
}
func TestR178NonLaneOtherPreserved(t *testing.T) {
	a := r175App(t)
	client := &LCUClient{baseURL: "http://lcu.invalid", token: "test"}
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/lol-gameflow/v1/session" {
			return r178JSON(map[string]any{"gameData": map[string]any{"gameId": 178, "queue": map[string]any{"id": 2400, "mapId": 12, "gameMode": "KIWI"}, "teamOne": []any{map[string]any{"selectedPosition": "NONE", "selectedRole": "NONE.NONE.NONE.UNSELECTED"}}}}, 200), nil
		}
		return r178JSON(map[string]any{}, 404), nil
	})}
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return nil, 503, fmt.Errorf("offline") }
	t.Cleanup(func() { a.stopMayhemSampler("test-end") })
	result := a.loadGameplayLive(context.Background(), client, Summoner{}, "InProgress")
	if len(result.Players) != 1 || result.Players[0].Position != "other" || gameplayLivePositionsPending(result) {
		t.Fatalf("nonlane changed: %#v", result)
	}
}

func r178FreshnessFixture(t *testing.T, failed bool) (*app, *LCUClient, *atomic.Int64, *atomic.Int64) {
	a, _, _ := newGameplayOverviewSGPFixture(t, false)
	a.storage = r175App(t).storage
	lcuCalls, sgpCalls := &atomic.Int64{}, &atomic.Int64{}
	now := time.Now().UnixMilli()
	a.lcu.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		lcuCalls.Add(1)
		if !strings.Contains(r.URL.Path, "/lol-match-history/") {
			return r178JSON(map[string]any{}, 404), nil
		}
		if r.URL.Query().Get("begIndex") != "0" || r.URL.Query().Get("endIndex") != "29" {
			t.Errorf("LCU window changed %s", r.URL.RawQuery)
		}
		return r178JSON(map[string]any{"games": map[string]any{"games": []any{map[string]any{"gameId": 1, "queueId": 440, "gameCreation": now - 120*60000, "gameDuration": 1800}}}}, 200), nil
	})}
	a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		sgpCalls.Add(1)
		if !strings.Contains(r.URL.Path, "/SUMMARY") || r.URL.Query().Get("count") != "30" {
			t.Errorf("not SUMMARY30 %s", r.URL)
		}
		if failed {
			return r178JSON(map[string]any{}, 500), nil
		}
		games := []any{}
		for i := int64(1); i <= 3; i++ {
			games = append(games, map[string]any{"json": map[string]any{"gameId": i, "queueId": 440, "gameCreation": now - (150-i*30)*60000, "gameDuration": 1800}})
		}
		return r178JSON(map[string]any{"games": games}, 200), nil
	})}
	return a, a.lcu, lcuCalls, sgpCalls
}

// R180 replaces evidence-driven later switching with immediate source merging.
func TestR178FreshnessEvidenceSwitchAndSelf(t *testing.T) {
	a, c, lcu, sgp := r178FreshnessFixture(t, false)
	scope := a.liveHistoryFreshnessForGame(c, 178, 440)
	ref := r161Ref(1)
	load := func(self bool) livePlayerMatchesResult {
		return a.livePlayerMatchesForGame(context.Background(), c, gameplayReference{PlayerRef: ref, ServerID: "HN1"}, ref, self, nil, scope, 100, 0)
	}
	first, second := load(false), load(false)
	if first.Source != "lcu+sgp" || len(first.Matches) != 3 || !reflect.DeepEqual(first, second) || lcu.Load() != 1 || sgp.Load() != 1 {
		t.Fatal(first, second, lcu.Load(), sgp.Load())
	}
	events := r175Events(t, a, "live_history_freshness")
	if len(events) != 1 || events[0]["missing_newer_any"] != float64(2) {
		t.Fatal(events)
	}
	oldTransport := c.http.Transport
	c.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "/current-summoner/matches") {
			t.Errorf("self endpoint changed: %s", r.URL.Path)
		}
		return oldTransport.RoundTrip(r)
	})
	self := load(true)
	if self.Source != "lcu+sgp" || lcu.Load() != 2 || sgp.Load() != 2 {
		t.Fatal("self changed", self, lcu.Load(), sgp.Load())
	}
	c.http.Transport = oldTransport
	encoded, _ := json.Marshal(r175Events(t, a, "live_history_freshness"))
	for _, forbidden := range []string{ref, "game_id", "gameId", "playerRef", "puuid"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("diagnostic identity/match id", string(encoded))
		}
	}
	key := liveHistoryCacheKey(scope, ref, false)
	expire := func() {
		a.livePlayerMatchesMu.Lock()
		cached := a.livePlayerMatchCache[key]
		cached.FetchedAt = time.Now().Add(-46 * time.Second)
		a.livePlayerMatchCache[key] = cached
		a.livePlayerMatchesMu.Unlock()
	}
	expire()
	a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("offline") })}
	if fallback := load(false); fallback.Source != "lcu" || len(fallback.Matches) != 1 {
		t.Fatal(fallback)
	}
	expire()
	a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return r178JSON(map[string]any{"games": []any{}}, 200), nil
	})}
	if emptyFallback := load(false); emptyFallback.Source != "lcu" || len(emptyFallback.Matches) != 1 {
		t.Fatal(emptyFallback)
	}
	next := a.liveHistoryFreshnessForGame(c, 179, 440)
	if next == scope || len(next.records) != 0 {
		t.Fatal("game scope not reset")
	}
}
func TestR178FreshnessSampleFailure(t *testing.T) {
	a, c, _, _ := r178FreshnessFixture(t, true)
	scope := a.liveHistoryFreshnessForGame(c, 178, 440)
	ref := r161Ref(1)
	first := a.livePlayerMatchesForGame(context.Background(), c, gameplayReference{}, ref, false, nil, scope, 200, 0)
	second := a.livePlayerMatchesForGame(context.Background(), c, gameplayReference{}, ref, false, nil, scope, 200, 0)
	if !reflect.DeepEqual(first, second) || first.Source != "lcu" {
		t.Fatal(first, second)
	}
	events := r175Events(t, a, "live_history_freshness")
	if len(events) != 1 || events[0]["sample_failed"] != true || events[0]["missing_newer_in_queue"] != nil || events[0]["sgp_newest_any_age_min"] != nil {
		t.Fatal(events)
	}
}
func TestR178FreshnessConcurrentAllPlayers(t *testing.T) {
	a, c, _, sgp := r178FreshnessFixture(t, false)
	scope := a.liveHistoryFreshnessForGame(c, 178, 440)
	var wg sync.WaitGroup
	for round := 0; round < 2; round++ {
		for i := 1; i < 9; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ref := r161Ref(i)
				a.livePlayerMatchesForGame(context.Background(), c, gameplayReference{ServerID: "HN1"}, ref, false, nil, scope, 100, i-1)
			}(i)
		}
		wg.Wait()
	}
	if sgp.Load() != 8 || len(r175Events(t, a, "live_history_freshness")) != 8 {
		t.Fatalf("samples requests=%d events=%d", sgp.Load(), len(r175Events(t, a, "live_history_freshness")))
	}
}
func TestR178FreshnessMixedQueues(t *testing.T) {
	now := time.UnixMilli(1800000000000)
	matches := []gameplayMatch{{GameID: 1, QueueID: 440, CreatedAt: now.Add(-90 * time.Minute).UnixMilli()}, {GameID: 2, QueueID: 420, CreatedAt: now.Add(-5 * time.Minute).UnixMilli()}, {GameID: 3, QueueID: 440, CreatedAt: now.Add(-120 * time.Minute).UnixMilli()}}
	d := liveHistoryFreshnessDiagnostic(livePlayerMatchesResult{Matches: matches, Source: "lcu"}, "", 440, 200, 0, false, now)
	if *d["newest_any_age_min"].(*float64) != 5 || *d["newest_queue_age_min"].(*float64) != 90 || d["queue_games"] != 2 || d["window_games"] != 3 {
		t.Fatal(d)
	}
	if liveNewestAgeMinutes(nil, 440, now) != nil || liveMissingNewerGames(nil, matches, 440) != nil {
		t.Fatal("missing timestamp invented")
	}
	sgp := append(append([]gameplayMatch{}, matches...), gameplayMatch{GameID: 4, CreatedAt: now.UnixMilli()}, gameplayMatch{GameID: 4, CreatedAt: now.UnixMilli()})
	if liveMissingNewerGames(matches, sgp, 0) != 1 {
		t.Fatal("distinct missing count")
	}
}
