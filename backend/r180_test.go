package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type r180HistoryFixture struct {
	a                    *app
	c                    *LCUClient
	now                  int64
	lcuCalls, sgpCalls   atomic.Int64
	generation           atomic.Int32
	sgpFailed, lcuFailed atomic.Bool
}

func r180Fixture(t *testing.T) *r180HistoryFixture {
	t.Helper()
	a, _, _ := newGameplayOverviewSGPFixture(t, false)
	a.storage = r175App(t).storage
	f := &r180HistoryFixture{a: a, c: a.lcu, now: time.Now().UnixMilli()}
	f.generation.Store(1)
	a.summoner.PUUID = r161Ref(0)
	f.c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		f.lcuCalls.Add(1)
		if !strings.Contains(r.URL.Path, "/lol-match-history/") {
			return r178JSON(map[string]any{}, 404), nil
		}
		if r.URL.Query().Get("begIndex") != "0" || r.URL.Query().Get("endIndex") != "29" {
			t.Errorf("LCU window: %s", r.URL)
		}
		if f.lcuFailed.Load() {
			return r178JSON(map[string]any{}, 503), nil
		}
		ref := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/lol-match-history/v1/products/lol/"), "/matches")
		if ref == "current-summoner" {
			ref = a.summoner.PUUID
		}
		games := []any{}
		for i := int64(1); i <= 11; i++ {
			games = append(games, f.lcuGame(i, 440, ref))
		}
		games = append(games, f.lcuGame(999, 420, ref))
		return r178JSON(map[string]any{"games": map[string]any{"games": games}}, 200), nil
	})}
	a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/leagues-ledge/") {
			return r178JSON(map[string]any{"queues": []any{}}, 200), nil
		}
		f.sgpCalls.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/SUMMARY") || (r.URL.Query().Get("count") != "30" && r.URL.Query().Get("count") != "10") {
			t.Errorf("unexpected SGP %s", r.URL)
		}
		if f.sgpFailed.Load() {
			return r178JSON(map[string]any{}, 500), nil
		}
		parts := strings.Split(r.URL.Path, "/")
		ref := parts[len(parts)-2]
		games := []any{}
		for i := int64(1); i <= 11+int64(f.generation.Load()); i++ {
			games = append(games, map[string]any{"json": f.sgpGame(i, 440, ref)})
		}
		games = append(games, map[string]any{"json": f.sgpGame(999, 420, ref)})
		if tag := r.URL.Query().Get("tag"); tag != "" {
			filtered := []any{}
			if tag == "q_440" {
				for i := 11 + int64(f.generation.Load()); i >= 1 && len(filtered) < 10; i-- {
					filtered = append(filtered, map[string]any{"json": f.sgpGame(i, 440, ref)})
				}
			} else if tag == "q_420" {
				filtered = append(filtered, map[string]any{"json": f.sgpGame(999, 420, ref)})
			}
			games = filtered
		}
		return r178JSON(map[string]any{"games": games}, 200), nil
	})}
	return f
}
func (f *r180HistoryFixture) created(i int64) int64 {
	if i == 999 {
		return f.now - 60000
	}
	return f.now - (20-i)*60000
}
func (f *r180HistoryFixture) lcuGame(i, queue int64, ref string) map[string]any {
	return map[string]any{"gameId": 100 + i, "queueId": queue, "gameCreation": f.created(i), "gameDuration": 1800,
		"participants":          []any{map[string]any{"participantId": 1, "teamId": 100, "championId": 61, "stats": map[string]any{"win": true, "kills": 2, "deaths": 1, "assists": 3}}},
		"participantIdentities": []any{map[string]any{"participantId": 1, "player": map[string]any{"puuid": ref, "gameName": "Fixture"}}}}
}
func (f *r180HistoryFixture) sgpGame(i, queue int64, ref string) map[string]any {
	return map[string]any{"gameId": 100 + i, "queueId": queue, "gameCreation": f.created(i), "gameDuration": 1800,
		"metadata":     map[string]any{"matchId": fmt.Sprintf("HN1_%d", 100+i)},
		"participants": []any{map[string]any{"puuid": ref, "participantId": 1, "teamId": 100, "championId": 61, "win": true, "kills": 2, "deaths": 1, "assists": 3}}}
}
func (f *r180HistoryFixture) load(ref string, self bool, scope *liveHistoryFreshnessScope, team int64, slot int) livePlayerMatchesResult {
	return f.a.livePlayerMatchesForGame(context.Background(), f.c, gameplayReference{PlayerRef: ref, ServerID: "HN1"}, ref, self, nil, scope, team, slot)
}
func (f *r180HistoryFixture) expire(scope *liveHistoryFreshnessScope, ref string, self bool) {
	f.a.livePlayerMatchesMu.Lock()
	defer f.a.livePlayerMatchesMu.Unlock()
	key := liveHistoryCacheKey(scope, ref, self)
	cached := f.a.livePlayerMatchCache[key]
	cached.FetchedAt = time.Now().Add(-46 * time.Second)
	f.a.livePlayerMatchCache[key] = cached
}

func TestR180MergedLatestSameQueueAndDedup(t *testing.T) {
	f := r180Fixture(t)
	ref := r161Ref(1)
	f.a.setQueueFilterCapability("HN1", "flex:q_440", queueFilterCapabilityUnsupported)
	scope := f.a.liveHistoryFreshnessForGame(f.c, 180, 440)
	result := f.load(ref, false, scope, 100, 2)
	selected := recentMatchesForPlayer(result.Matches, ref, 10, 440)
	if len(selected) != 10 || selected[0].GameID != 112 || selected[9].GameID != 103 {
		t.Fatalf("latest ten: %#v", selected)
	}
	if len(result.Matches) != 13 {
		t.Fatalf("duplicates survived merge: %d", len(result.Matches))
	}
	seen := map[int64]bool{}
	for _, m := range result.Matches {
		if seen[m.GameID] {
			t.Fatal("duplicate", m.GameID)
		}
		seen[m.GameID] = true
	}
	stats, games := liveRecentPlayerStats(result.Matches, ref, 440)
	record := recentRankedRecord(games)
	if stats.Games != 10 || stats.Wins != 10 || stats.Losses != 0 || stats.KDA != 5 || record.Games != 10 || record.Wins != 10 || len(games) != 10 || games[0].CreatedAt != f.created(12) {
		t.Fatal(stats, record, games)
	}
	// Both real adapters use the same numeric ID; metadata region prefixes are irrelevant.
	if result.Evidence.LCU[0].GameID != 101 || !liveHistoryContainsGame(result.Evidence.SGP, 112) {
		t.Fatal("different id schemes")
	}
	events := r175Events(t, f.a, "live_history_freshness")
	if len(events) != 1 || events[0]["missing_newer_in_queue"] != float64(1) || events[0]["missing_newer_any"] != float64(0) || events[0]["slot"] != float64(2) || events[0]["team"] != float64(100) || events[0]["shown_newest_age_min"] != float64(8) || events[0]["lcu_newest_queue_age_min"] != float64(9) || events[0]["sgp_newest_queue_age_min"] != float64(8) {
		t.Fatal(events)
	}
	encoded, _ := json.Marshal(events)
	for _, s := range []string{ref, "playerRef", "puuid", "gameId", "game_id", "matchId"} {
		if strings.Contains(string(encoded), s) {
			t.Fatal("private diagnostic", string(encoded))
		}
	}
}
func TestR180BypassStaleSGPCacheWithoutWriting(t *testing.T) {
	f := r180Fixture(t)
	ref := r161Ref(1)
	ctx := context.Background()
	f.generation.Store(0)
	if _, _, _, err := f.a.sgp.matchHistory(ctx, f.c, ref, 0, 30, true); err != nil {
		t.Fatal(err)
	}
	f.a.sgp.mu.Lock()
	before := make(map[string]sgpHistoryCacheEntry)
	for k, v := range f.a.sgp.historyCache {
		before[k] = v
	}
	f.a.sgp.mu.Unlock()
	if len(before) == 0 {
		t.Fatal("stale fixture not cached")
	}
	f.generation.Store(1)
	scope := f.a.liveHistoryFreshnessForGame(f.c, 180, 440)
	result := f.load(ref, false, scope, 100, 0)
	selected := recentMatchesForPlayer(result.Matches, ref, 10, 440)
	if len(selected) != 10 || selected[0].GameID != 112 || f.sgpCalls.Load() != 2 {
		t.Fatal("stale SGP page used", selected, f.sgpCalls.Load())
	}
	f.a.sgp.mu.Lock()
	unchanged := reflect.DeepEqual(before, f.a.sgp.historyCache)
	f.a.sgp.mu.Unlock()
	if !unchanged {
		t.Fatal("live request wrote five-minute page cache")
	}
	old, _, _, err := f.a.sgp.matchHistory(ctx, f.c, ref, 0, 30, true)
	if err != nil || f.sgpCalls.Load() != 2 || len(old) != 12 {
		t.Fatal("shared stale cache was replaced", len(old), err, f.sgpCalls.Load())
	}
}
func TestR180SourceFailuresAndSelf(t *testing.T) {
	t.Run("sgp-failed", func(t *testing.T) {
		f := r180Fixture(t)
		ref := r161Ref(1)
		f.sgpFailed.Store(true)
		scope := f.a.liveHistoryFreshnessForGame(f.c, 180, 440)
		want := loadLiveLCUMatches(context.Background(), f.c, gameplayReference{PlayerRef: ref}, ref, false, nil)
		got := f.load(ref, false, scope, 200, 3)
		if !reflect.DeepEqual(want.Matches, got.Matches) || got.Source != "lcu" || got.State != want.State {
			t.Fatal(want, got)
		}
		events := r175Events(t, f.a, "live_history_freshness")
		if len(events) != 1 || events[0]["sample_failed"] != true || events[0]["missing_newer_in_queue"] != nil || events[0]["sgp_newest_queue_age_min"] != nil {
			t.Fatal(events)
		}
	})
	t.Run("lcu-failed", func(t *testing.T) {
		f := r180Fixture(t)
		f.lcuFailed.Store(true)
		ref := r161Ref(1)
		scope := f.a.liveHistoryFreshnessForGame(f.c, 180, 440)
		got := f.load(ref, false, scope, 200, 0)
		if got.Source != "sgp" || got.State != "ok" || recentMatchesForPlayer(got.Matches, ref, 10, 440)[0].GameID != 112 {
			t.Fatal(got)
		}
	})
	t.Run("sgp-unavailable", func(t *testing.T) {
		f := r180Fixture(t)
		f.a.sgp = nil
		got := f.load(r161Ref(1), false, f.a.liveHistoryFreshnessForGame(f.c, 180, 440), 100, 1)
		if got.Source != "lcu" || got.State != "ok" || f.sgpCalls.Load() != 0 {
			t.Fatal(got)
		}
	})
	t.Run("self", func(t *testing.T) {
		f := r180Fixture(t)
		ref := f.a.summoner.PUUID
		scope := f.a.liveHistoryFreshnessForGame(f.c, 180, 440)
		transport := f.c.http.Transport
		f.c.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if !strings.Contains(r.URL.Path, "/current-summoner/matches") {
				t.Errorf("self endpoint %s", r.URL)
			}
			return transport.RoundTrip(r)
		})
		got := f.load(ref, true, scope, 100, 0)
		again := f.load(ref, true, scope, 100, 0)
		if f.sgpCalls.Load() != 1 || f.lcuCalls.Load() != 1 || !reflect.DeepEqual(got, again) || got.Source != "lcu+sgp" || recentMatchesForPlayer(got.Matches, ref, 10, 440)[0].GameID != 112 {
			t.Fatal(got, f.sgpCalls.Load())
		}
		events := r175Events(t, f.a, "live_history_freshness")
		if len(events) != 1 || events[0]["is_current"] != true || events[0]["shown_newest_age_min"] != float64(8) || events[0]["sgp_newest_queue_age_min"] != float64(8) {
			t.Fatal(events)
		}
	})
}
func TestR180PollingFlightsAndExpiry(t *testing.T) {
	f := r180Fixture(t)
	scope := f.a.liveHistoryFreshnessForGame(f.c, 180, 440)
	var wg sync.WaitGroup
	for tick := 0; tick < 16; tick++ {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(i int) { defer wg.Done(); f.load(r161Ref(i), i == 0, scope, 100+int64(i/5)*100, i%5) }(i)
		}
		wg.Wait()
	}
	if f.sgpCalls.Load() != 10 || f.lcuCalls.Load() != 1 || len(r175Events(t, f.a, "live_history_freshness")) != 10 {
		t.Fatal("polling amplified requests", f.sgpCalls.Load(), f.lcuCalls.Load(), r175Events(t, f.a, "live_history_freshness"))
	}
	ref := r161Ref(1)
	f.expire(scope, ref, false)
	f.generation.Store(2)
	got := f.load(ref, false, scope, 100, 1)
	if f.sgpCalls.Load() != 11 || recentMatchesForPlayer(got.Matches, ref, 10, 440)[0].GameID != 113 || len(r175Events(t, f.a, "live_history_freshness")) != 11 {
		t.Fatal("expiry or once/game diagnostics", got)
	}
	next := f.a.liveHistoryFreshnessForGame(f.c, 181, 440)
	f.load(ref, false, next, 100, 1)
	if f.sgpCalls.Load() != 12 || len(r175Events(t, f.a, "live_history_freshness")) != 12 {
		t.Fatal("new game did not refresh")
	}
}
func TestR180ConcurrentSources(t *testing.T) {
	f := r180Fixture(t)
	lcuStarted, sgpStarted, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	oldLCU, oldSGP := f.c.http.Transport, f.a.sgp.http.Transport
	f.c.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(lcuStarted)
		<-release
		return oldLCU.RoundTrip(r)
	})
	f.a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(sgpStarted)
		<-release
		return oldSGP.RoundTrip(r)
	})
	done := make(chan struct{})
	go func() {
		f.load(f.a.summoner.PUUID, true, f.a.liveHistoryFreshnessForGame(f.c, 180, 440), 100, 0)
		close(done)
	}()
	for _, started := range []chan struct{}{lcuStarted, sgpStarted} {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("sources serialized")
		}
	}
	unblock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loader stuck")
	}
}
func TestR180QualitySkippedCountsAndSlots(t *testing.T) {
	ref := r161Ref(1)
	now := time.UnixMilli(1800000000000)
	good := gameplayMatch{GameID: 9009800745, CreatedAt: now.Add(-time.Minute).UnixMilli(), QueueID: 440, Result: "win", SubjectParticipantID: 1, Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: ref, Kills: 2, Deaths: 1, Assists: 3}}}
	poor := good
	poor.Result = "unknown"
	poor.SubjectParticipantID = 0
	poor.Participants = []gameplayParticipant{{ParticipantID: 2, PlayerRef: "wrong"}}
	merged := mergeLivePlayerMatches([]gameplayMatch{poor}, []gameplayMatch{good}, ref)
	if len(merged) != 1 || !reflect.DeepEqual(merged[0], good) {
		t.Fatal("lost usable subject/result", merged)
	}
	// Identity, result and queue each participate in the preference, in both directions.
	for _, field := range []string{"subject", "result", "queue"} {
		bad := good
		switch field {
		case "subject":
			bad.SubjectParticipantID = 0
			bad.Participants = poor.Participants
		case "result":
			bad.Result = "unknown"
		case "queue":
			bad.QueueID = 0
		}
		for _, reverse := range []bool{false, true} {
			a, b := bad, good
			if reverse {
				a, b = b, a
			}
			got := mergeLivePlayerMatches([]gameplayMatch{a}, []gameplayMatch{b}, ref)
			if len(got) != 1 || !reflect.DeepEqual(got[0], good) {
				t.Fatal(field, reverse, got)
			}
		}
	}
	remake := good
	remake.GameID++
	remake.Result = "remake"
	unmatched := good
	unmatched.GameID += 2
	unmatched.SubjectParticipantID = 0
	unmatched.Participants = poor.Participants
	other := unmatched
	other.GameID++
	other.QueueID = 420
	lcu := []gameplayMatch{good, remake, unmatched, other}
	d := liveHistoryFreshnessDiagnostic(livePlayerMatchesResult{Matches: lcu, Evidence: &liveHistoryEvidence{LCU: lcu}}, ref, 440, 200, 3, false, now)
	if d["subject_unmatched"] != 1 || d["remake_skipped"] != 1 || d["queue_games"] != 3 || *d["shown_newest_age_min"].(*float64) != 1 {
		t.Fatal(d)
	}
	if liveMissingNewerGames(nil, []gameplayMatch{good}, 440) != nil || liveNewestAgeMinutes(nil, 440, now) != nil {
		t.Fatal("unknown baseline invented")
	}
	players := []gameplayLivePlayer{{TeamID: 100, Position: "bottom"}, {TeamID: 200, Position: "utility"}, {TeamID: 100, Position: "top"}, {TeamID: 100, Position: "middle"}, {TeamID: 200, Position: "top"}}
	if got := liveHistoryRosterSlots(players, true); !reflect.DeepEqual(got, []int{2, 1, 0, 1, 0}) {
		t.Fatal(got)
	}

}

func TestR180RealRosterStageDiagnostics(t *testing.T) {
	f := r180Fixture(t)
	var ingame atomic.Bool
	old := f.c.http.Transport
	positions := []string{"BOTTOM", "UTILITY", "TOP", "JUNGLE", "MIDDLE"}
	f.c.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/lol-match-history/") {
			return old.RoundTrip(r)
		}
		teams := [2][]any{}
		for i := 0; i < 10; i++ {
			ref, name := r161Ref(i), fmt.Sprintf("Player%d", i)
			if i >= 5 && !ingame.Load() {
				ref, name = "", ""
			}
			position := positions[i%5]
			if ingame.Load() {
				position = "TOP"
			}
			teams[i/5] = append(teams[i/5], map[string]any{"cellId": i, "puuid": ref, "gameName": name, "tagLine": "CN1", "selectedPosition": position, "assignedPosition": positions[i%5]})
		}
		switch {
		case r.URL.Path == "/lol-gameflow/v1/session":
			return r178JSON(map[string]any{"gameData": map[string]any{"gameId": 180, "queue": map[string]any{"id": 440, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": teams[0], "teamTwo": teams[1]}}, 200), nil
		case r.URL.Path == "/lol-lobby/v2/lobby":
			return r178JSON(map[string]any{"gameConfig": map[string]any{"queueId": 440, "mapId": 11, "gameMode": "CLASSIC"}}, 200), nil
		case r.URL.Path == "/lol-champ-select/v1/session":
			return r178JSON(map[string]any{"gameId": 180, "localPlayerCellId": 0, "myTeam": teams[0], "theirTeam": teams[1]}, 200), nil
		case strings.HasPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
			ref := strings.TrimPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/")
			return r178JSON(map[string]any{"puuid": ref, "gameName": fmt.Sprintf("Player%d", int(ref[0]-'a')), "tagLine": "CN1"}, 200), nil
		default:
			return r178JSON(map[string]any{}, 404), nil
		}
	})
	f.a.liveClientPlayerList = func(context.Context) ([]byte, int, error) {
		live := []any{}
		for i := 0; i < 10; i++ {
			team := "ORDER"
			if i >= 5 {
				team = "CHAOS"
			}
			live = append(live, map[string]any{"riotId": fmt.Sprintf("Player%d#CN1", i), "team": team, "position": positions[i%5]})
		}
		b, _ := json.Marshal(live)
		return b, 200, nil
	}
	current := Summoner{PUUID: f.a.summoner.PUUID, GameName: "Player0", TagLine: "CN1"}
	for tick := 0; tick < 3; tick++ {
		progressCount, earlyHistory := 0, false
		ctx := context.WithValue(context.Background(), liveProgressPublisherKey{}, func(value gameplayLiveResponse) {
			progressCount++
			pending, ready := 0, 0
			for _, player := range value.Players {
				if player.HistoryState == "pending" {
					pending++
				}
				if player.HistoryState == "ok" {
					ready++
				}
			}
			if pending > 0 && ready > 0 {
				earlyHistory = true
			}
			encoded, _ := json.Marshal(value)
			for i := 0; i < 10; i++ {
				if strings.Contains(string(encoded), r161Ref(i)) {
					t.Error("raw player identity in incremental payload")
				}
			}
		})
		response := f.a.loadGameplayLive(ctx, f.c, current, "ChampSelect")
		if progressCount != 11 || !earlyHistory {
			t.Fatal("real roster did not progressively publish", progressCount, earlyHistory)
		}
		costs := r175Events(t, f.a, "live_load_cost")
		if len(costs) == 0 {
			t.Fatal("missing live costs")
		}
		for _, key := range []string{"sgp_ms", "lcu_ms", "slowest_player_ms"} {
			if _, ok := costs[len(costs)-1][key]; !ok {
				t.Fatal("missing timing", key)
			}
		}
		if len(response.Players) != 10 {
			t.Fatal(response)
		}
		for _, p := range response.Players[:5] {
			if len(p.RecentGames) != 10 || p.RecentRankedRecord == nil || p.RecentRankedRecord.Games != 10 {
				t.Fatal("wrong displayed count", p)
			}
		}
	}
	if f.sgpCalls.Load() != 5 || len(r175Events(t, f.a, "live_history_freshness")) != 5 {
		t.Fatal("selection budget", f.sgpCalls.Load())
	}
	ingame.Store(true)
	response := f.a.loadGameplayLive(context.Background(), f.c, current, "InProgress")
	if len(response.Players) != 10 || f.sgpCalls.Load() != 10 {
		t.Fatal("game budget", f.sgpCalls.Load(), response)
	}
	events := r175Events(t, f.a, "live_history_freshness")
	if len(events) != 10 {
		t.Fatal("missing player events", events)
	}
	unique := map[string]bool{}
	for _, e := range events {
		age := float64(8)

		if e["shown_newest_age_min"] != age || e["subject_unmatched"] != float64(0) || e["remake_skipped"] != float64(0) {
			t.Fatal("wrong final diagnostic window", e)
		}
		key := fmt.Sprintf("%v:%v", e["team"], e["slot"])
		if unique[key] {
			t.Fatal("duplicate team+slot", events)
		}
		unique[key] = true
	}
	for _, team := range []int{100, 200} {
		for slot := 0; slot < 5; slot++ {
			if !unique[fmt.Sprintf("%d:%d", team, slot)] {
				t.Fatal("wrong slot", events)
			}
		}
	}
	// Both final position corrections and the logged slot mapping match lane order.
	want := []int{3, 4, 0, 1, 2, 3, 4, 0, 1, 2}
	if got := liveHistoryRosterSlots(response.Players, true); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	for _, p := range response.Players {
		if p.positionSource != "liveclient" || len(p.RecentGames) != 10 || p.RecentRankedRecord == nil || p.RecentRankedRecord.Games != 10 {
			t.Fatal("position/history not finalized", p)
		}
	}
	// Retain one event per player on an additional in-game poll.
	f.a.loadGameplayLive(context.Background(), f.c, current, "InProgress")
	if f.sgpCalls.Load() != 10 || len(r175Events(t, f.a, "live_history_freshness")) != 10 {
		t.Fatal("ingame poll duplicated request/diagnostic")
	}
}
