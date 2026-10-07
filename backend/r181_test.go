package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func r181SelfScope(f *r180HistoryFixture, previousQueue int64) *liveHistoryFreshnessScope {
	f.a.observeLiveHistoryGame(f.c, "InProgress", 112, previousQueue)
	f.a.clearLiveHistoryFreshness() // End/Lobby must retain the observed game.
	f.a.observeLiveHistoryGame(f.c, "ChampSelect", 181, 440)
	return f.a.liveHistoryFreshnessForGame(f.c, 181, 440)
}
func TestR181SelfLatestMergeAndPreviousGame(t *testing.T) {
	f := r180Fixture(t)
	ref := f.a.summoner.PUUID
	scope := r181SelfScope(f, 440)
	result := f.load(ref, true, scope, 100, 0)
	shown := recentMatchesForPlayer(result.Matches, ref, 10, 440)
	if len(shown) != 10 || shown[0].GameID != 112 || result.Source != "sgp" {
		t.Fatalf("self missing latest: %#v source=%s", shown, result.Source)
	}
	stats, games := liveRecentPlayerStats(result.Matches, ref, 440)
	if stats.Games != 10 || stats.Wins != 10 || stats.Losses != 0 || stats.KDA != 5 || recentRankedRecord(games).Games != 10 {
		t.Fatal("self duplicate statistics", stats, games)
	}
	seen := map[int64]bool{}
	for _, m := range result.Matches {
		if seen[m.GameID] {
			t.Fatal("duplicate", m.GameID)
		}
		seen[m.GameID] = true
	}
	events := r175Events(t, f.a, "live_history_freshness")
	if len(events) != 1 || events[0]["source"] != "sgp" || events[0]["missing_newer_in_queue"] != nil || events[0]["prev_game_in_lcu"] != nil || events[0]["prev_game_in_sgp"] != true || events[0]["prev_game_shown"] != true || events[0]["load_index"] != float64(1) {
		t.Fatal(events)
	}
	encoded, _ := json.Marshal(events)
	for _, s := range []string{ref, "puuid", "gameId", "game_id"} {
		if strings.Contains(string(encoded), s) {
			t.Fatal("private identifier", string(encoded))
		}
	}
}
func TestR181SelfSGPFailureKeepsLCU(t *testing.T) {
	f := r180Fixture(t)
	ref := f.a.summoner.PUUID
	f.sgpFailed.Store(true)
	want := loadLiveLCUMatches(context.Background(), f.c, gameplayReference{PlayerRef: ref}, ref, true, nil)
	got := f.load(ref, true, r181SelfScope(f, 440), 100, 0)
	if !reflect.DeepEqual(want.Matches, got.Matches) || got.Source != "lcu" {
		t.Fatal(got)
	}
	events := r175Events(t, f.a, "live_history_freshness")
	if len(events) != 1 || events[0]["sample_failed"] != true || events[0]["prev_game_shown"] != false {
		t.Fatal(events)
	}
}
func TestR181SelfRefreshRecordsOnlyChangedLatest(t *testing.T) {
	f := r180Fixture(t)
	ref := f.a.summoner.PUUID
	f.generation.Store(0)
	scope := r181SelfScope(f, 440)
	first := f.load(ref, true, scope, 100, 0)
	if recentMatchesForPlayer(first.Matches, ref, 10, 440)[0].GameID != 111 {
		t.Fatal(first)
	}
	f.generation.Store(1)
	f.expire(scope, ref, true)
	f.load(ref, true, scope, 100, 0)
	f.expire(scope, ref, true)
	f.load(ref, true, scope, 100, 0) // A genuine third fetch, unchanged display.
	events := r175Events(t, f.a, "live_history_freshness")
	if len(events) != 2 || events[0]["load_index"] != float64(1) || events[1]["load_index"] != float64(2) || events[0]["prev_game_shown"] != false || events[1]["prev_game_shown"] != true || f.sgpCalls.Load() != 3 {
		t.Fatal(events, f.sgpCalls.Load())
	}
}
func TestR181FreshnessSixRecordCapAndQueueBoundary(t *testing.T) {
	t.Run("cap", func(t *testing.T) {
		f := r180Fixture(t)
		ref := f.a.summoner.PUUID
		scope := r181SelfScope(f, 440)
		for i := int32(0); i < 7; i++ {
			f.generation.Store(i)
			if i > 0 {
				f.expire(scope, ref, true)
			}
			f.load(ref, true, scope, 100, 0)
		}
		events := r175Events(t, f.a, "live_history_freshness")
		if len(events) != 6 || events[5]["load_index"] != float64(6) {
			t.Fatal(events)
		}
	})
	for _, queue := range []int64{0, 420} {
		t.Run(fmt.Sprint(queue), func(t *testing.T) {
			f := r180Fixture(t)
			ref := f.a.summoner.PUUID
			f.load(ref, true, r181SelfScope(f, queue), 100, 0)
			events := r175Events(t, f.a, "live_history_freshness")
			for _, key := range []string{"prev_game_in_lcu", "prev_game_in_sgp", "prev_game_shown"} {
				if _, ok := events[0][key]; ok {
					t.Fatal("unknown/different queue previous game", events)
				}
			}
		})
	}
	t.Run("next-inprogress", func(t *testing.T) {
		f := r180Fixture(t)
		r181SelfScope(f, 440)
		f.a.observeLiveHistoryGame(f.c, "InProgress", 181, 440)
		if s := f.a.liveHistoryFreshnessForGame(f.c, 181, 440); s.previousGameID != 112 {
			t.Fatal(s)
		}
		f.a.observeLiveHistoryGame(&LCUClient{}, "InProgress", 182, 440)
		if s := f.a.liveHistoryFreshnessForGame(f.c, 182, 440); s.previousGameID != 0 {
			t.Fatal("client crossed", s)
		}
	})
}

func TestR181LaneMatchupFullRowsAndRecommendationCache(t *testing.T) {
	p := newChampionProvider()
	p.cache = newChampionDataCache(nil)
	p.patch = "16.16.1"
	p.championMeta[799] = championMetadata{ID: 799, Key: "Fixture", Slug: "fixture"}
	p.championIDs["fixture"] = 799
	p.championKeys["fixture"] = "Fixture"
	p.static["item/3153.png"] = championAssetDescription{Name: "夹具物品"}
	counters := []any{}
	for i := 0; i < 61; i++ {
		id := 1000 + i
		if i == 33 {
			id = 266
		}
		p.championMeta[id] = championMetadata{ID: id, Key: fmt.Sprint(id), Slug: fmt.Sprint(id)}
		counters = append(counters, map[string]any{"champion_id": id, "play": 1000, "win": 200 + i*10})
	}
	payload, _ := json.Marshal(map[string]any{"data": map[string]any{"summary": map[string]any{"id": 799, "average_stats": map[string]any{"play": 1000, "win_rate": 0.52}, "positions": []any{map[string]any{"name": "TOP", "stats": map[string]any{"play": 1000, "role_rate": 1, "win_rate": 0.52}}}}, "core_items": []any{map[string]any{"ids": []int{3153}, "play": 1000, "win": 520}}, "counters": counters}, "meta": map[string]any{"version": "16.16"}})
	var upstream atomic.Int64
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == opggChampionHost {
			if strings.HasSuffix(r.URL.Path, "/versions") {
				return r178JSON(map[string]any{"data": []string{"16.16"}}, 200), nil
			}
			upstream.Add(1)
			if r.URL.Path != "/api/KR/champions/ranked/799/TOP" || r.URL.Query().Get("tier") != "emerald_plus" {
				t.Errorf("mismatched upstream %s", r.URL)
			}
			var body any
			json.Unmarshal(payload, &body)
			return r178JSON(body, 200), nil
		}
		return r178JSON([]any{}, 200), nil
	})}
	a := &app{champions: p}
	w := httptest.NewRecorder()
	a.handleGameplayRecommendations(w, httptest.NewRequest("GET", "/api/gameplay/recommendations?championId=799&queueId=440&gameMode=CLASSIC&mapId=11&position=top&tier=emerald_plus", nil))
	if w.Code != 200 {
		_, detailErr := p.loadDetail(context.Background(), "ranked", "fixture", "top", "emerald_plus")
		t.Fatalf("recommendation %d %s underlying=%v", w.Code, w.Body.String(), detailErr)
	}
	var rec gameplayRecommendationsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.Recommendations.Hero.WeakAgainst) != 5 || len(rec.Recommendations.Hero.StrongAgainst) != 5 {
		t.Fatal(rec.Recommendations.Hero)
	}
	for _, rows := range [][]gameplayRecommendationMatchup{rec.Recommendations.Hero.WeakAgainst, rec.Recommendations.Hero.StrongAgainst} {
		for _, row := range rows {
			if row.ChampionID == 266 {
				t.Fatal("fixture enemy is in top five")
			}
		}
	}
	before := upstream.Load()
	if before != 1 {
		t.Fatal("unexpected recommendation count", before)
	}
	w = httptest.NewRecorder()
	a.handleGameplayLaneMatchup(w, httptest.NewRequest("GET", "/api/gameplay/lane-matchup?champion=799&enemy=266&position=top&tier=emerald_plus", nil))
	var pair gameplayLaneMatchup
	json.Unmarshal(w.Body.Bytes(), &pair)
	if w.Code != 200 || pair.ChampionID != 799 || pair.EnemyChampionID != 266 || pair.WinRate != 53 || pair.Games != 1000 {
		t.Fatal(w.Code, w.Body.String())
	}
	if upstream.Load() != before {
		t.Fatalf("matchup bypassed recommendation cache: %d -> %d", before, upstream.Load())
	}
	w = httptest.NewRecorder()
	a.handleGameplayLaneMatchup(w, httptest.NewRequest("GET", "/api/gameplay/lane-matchup?champion=799&enemy=99999&position=top&tier=emerald_plus", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "{}" || upstream.Load() != before {
		t.Fatal(w.Code, w.Body.String(), upstream.Load())
	}
}

func TestR181DiagnosticsFromActualSenders(t *testing.T) {
	data, err := os.ReadFile("testdata/r181-runtime-events.json")
	if err != nil {
		t.Fatal(err)
	}
	var bodies []json.RawMessage
	if err = json.Unmarshal(data, &bodies); err != nil {
		t.Fatal(err)
	}
	a := r175App(t)
	for _, body := range bodies {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(string(body))))
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	cards := r175Events(t, a, "lane_matchup_card")
	if len(cards) != 2 {
		t.Fatal(cards)
	}
	for _, e := range cards {
		if e["mode"] != "a+b" || e["shown"] != false || e["own_locked"] != false || e["hidden_reason"] != "pair-no-data" || e["enemy_champion_id"] != float64(266) || e["tier"] != "emerald_plus" {
			t.Fatal(e)
		}
	}
	lanes := r175Events(t, a, "lane_matchup_candidate_fetch")
	if len(lanes) != 1 || lanes[0]["reason"] != "lanes-pending" || lanes[0]["tier"] != "emerald_plus" || lanes[0]["enemy_locked_count"] != float64(4) {
		t.Fatal(lanes)
	}
	// Hostile extra properties cannot enter the log even through the HTTP boundary.
	w := httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(`{"event":"lane_matchup_card","reason":"render","mode":"a","shown":true,"ownLocked":true,"hiddenReason":"pair-no-data","enemyChampionId":266,"tier":"emerald_plus","gameId":9010029782,"puuid":"PRIVATE","playerRef":"PRIVATE"}`)))
	if w.Code != 400 {
		t.Fatal("unknown identity fields must be rejected", w.Code)
	}
	w = httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(`{"event":"lane_matchup_card","reason":"render","mode":"a","shown":true,"ownLocked":true,"hiddenReason":"pair-no-data","enemyChampionId":266,"tier":"emerald_plus","gameId":9010029782}`)))
	cards = r175Events(t, a, "lane_matchup_card")
	if w.Code != 204 || len(cards) != 3 {
		t.Fatal(w.Code, cards)
	}
	if _, ok := cards[2]["hidden_reason"]; ok {
		t.Fatal("visible card has hidden reason", cards[2])
	}
	for _, e := range cards {
		for _, key := range []string{"game_id", "gameId", "puuid", "playerRef"} {
			if _, ok := e[key]; ok {
				t.Fatal("identity leak", e)
			}
		}
	}
}

func TestR181RealSelfCanonicalReferenceAndObservedPrevious(t *testing.T) {
	f := r180Fixture(t)
	canonical := f.a.summoner.PUUID
	alias := r161Ref(8)
	var selection atomic.Bool
	oldLCU := f.c.http.Transport
	oldSGP := f.a.sgp.http.Transport
	f.a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/SUMMARY") && !strings.Contains(r.URL.Path, "/"+canonical+"/SUMMARY") {
			t.Errorf("self SGP used roster alias: %s", r.URL.Path)
		}
		return oldSGP.RoundTrip(r)
	})
	f.c.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/lol-match-history/") {
			if !strings.Contains(r.URL.Path, "/current-summoner/matches") {
				t.Errorf("self LCU used alias: %s", r.URL.Path)
			}
			return oldLCU.RoundTrip(r)
		}
		switch r.URL.Path {
		case "/lol-gameflow/v1/session":
			return r178JSON(map[string]any{"gameData": map[string]any{"gameId": 112, "queue": map[string]any{"id": 440, "mapId": 11, "gameMode": "CLASSIC"}}}, 200), nil
		case "/lol-lobby/v2/lobby":
			return r178JSON(map[string]any{"gameConfig": map[string]any{"queueId": 440, "mapId": 11, "gameMode": "CLASSIC"}}, 200), nil
		case "/lol-champ-select/v1/session":
			if selection.Load() {
				return r178JSON(map[string]any{"gameId": 181, "localPlayerCellId": 0, "myTeam": []any{map[string]any{"cellId": 0, "puuid": alias, "assignedPosition": "TOP"}}}, 200), nil
			}
		}
		return r178JSON(map[string]any{}, 404), nil
	})
	f.a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return []byte(`[]`), 200, nil }
	current := Summoner{PUUID: canonical}
	f.a.loadGameplayLive(context.Background(), f.c, current, "InProgress")
	f.a.loadGameplayLive(context.Background(), f.c, current, "Lobby")
	selection.Store(true)
	response := f.a.loadGameplayLive(context.Background(), f.c, current, "ChampSelect")
	if len(response.Players) != 1 || !response.Players[0].IsCurrent || response.Players[0].reference.PlayerRef != canonical || len(response.Players[0].RecentGames) != 10 || response.Players[0].RecentGames[0].CreatedAt != f.created(12) {
		t.Fatalf("self canonicalization/display: %#v", response)
	}
	events := r175Events(t, f.a, "live_history_freshness")
	if len(events) != 1 || events[0]["prev_game_shown"] != true || events[0]["prev_game_in_sgp"] != true {
		t.Fatal(events)
	}
}
