package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR211SGPForceSummaryFreshAndAllExpectedReusesFirstPage(t *testing.T) {
	a, ref, _ := newGameplayOverviewSGPFixture(t, true)
	a.storage = r175App(t).storage
	// Foreground overview requests carry the fresh-history marker. The season
	// head scanner has its own background context and must not affect this count.
	upstream := a.sgp.http.Transport
	var foreground atomic.Int64
	background := make(chan context.Context, 16)
	a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/match-history-query/") && strings.HasSuffix(r.URL.Path, "/SUMMARY") {
			if r.Context().Value(overviewFreshHistoryKey{}) == true {
				foreground.Add(1)
			} else {
				background <- r.Context()
			}
		}
		return upstream.RoundTrip(r)
	})
	t.Cleanup(func() {
		deadline := time.Now().Add(3 * time.Second)
		for {
			a.seasonBackfillMu.Lock()
			running := len(a.seasonBackfills)
			a.seasonBackfillMu.Unlock()
			if running == 0 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("season scanner did not finish")
			}
			select {
			case ctx := <-background:
				<-ctx.Done()
			default:
				runtime.Gosched()
			}
		}
	})
	request := func(filter, expected string) gameplayOverview {
		t.Helper()
		w := httptest.NewRecorder()
		body := `{"playerRef":"` + ref + `","count":20,"force":true,"matchFilter":"` + filter + `"`
		if expected != "" {
			body += `,"expectGameId":"` + expected + `"`
		}
		body += `}`
		a.handleGameplayOverview(w, httptest.NewRequest("POST", "/api/gameplay/overview", strings.NewReader(body)))
		var result gameplayOverview
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		return result
	}
	request("all", "")
	// Cancellation is the scanner's existing completion signal: its cache write,
	// diagnostic and season-progress broadcast occur before the deferred cancel.
	select {
	case ctx := <-background:
		select {
		case <-ctx.Done():
		case <-time.After(3 * time.Second):
			t.Fatal("season scan completion timeout")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("force did not trigger a season scan")
	}
	head := []map[string]any{}
	for _, event := range r175Events(t, a, "season_stats_head_refresh") {
		if event["skipped"] != true {
			head = append(head, event)
		}
	}
	if len(head) != 1 || head[0]["fresh"] != true || head[0]["use_history_cache"] != false || head[0]["sgp_history_calls"] != float64(3) || head[0]["sgp_history_cache_hits"] != float64(0) {
		t.Fatal("fresh background season scan missing", head)
	}
	warm := foreground.Load()
	request("all", "")
	baseline := foreground.Load() - warm
	if baseline == 0 {
		t.Fatal("foreground SUMMARY requests were not counted")
	}
	before := foreground.Load()
	response := request("all", "420")
	if foreground.Load()-before != baseline || response.ExpectedGamePresent == nil || !*response.ExpectedGamePresent {
		t.Fatalf("all verification fetched an extra page: initial=%d new=%d present=%v", baseline, foreground.Load()-before, response.ExpectedGamePresent)
	}
	response = request("flex", "420")
	if response.ExpectedGamePresent == nil || !*response.ExpectedGamePresent || response.Matches[0].QueueID != 440 {
		t.Fatal(response)
	}
	costs := r175Events(t, a, "overview_load_cost")
	if len(costs) != 4 {
		t.Fatalf("missing overview costs: %d", len(costs))
	}
	for _, row := range costs {
		if row["sgp_history_cache_hits"] != float64(0) {
			t.Fatalf("force reused history cache: %+v", row)
		}
	}
}

func TestR211MasteryRealRiotFieldsAndFullCache(t *testing.T) {
	raw, err := os.ReadFile("testdata/r211/masteries-topking-five.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []ChampionMastery
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 || rows[0].ChampionLevel != 72 || *rows[0].TokensEarned != 65 || *rows[0].MarkRequiredForNextLevel != 2 || *rows[0].ChampionSeasonMilestone != 17 || rows[0].MilestoneGrades[0] != "A+" || rows[0].NextSeasonMilestone.RequireGradeCounts["S-"] != 2 || rows[0].LastPlayTime <= 0 || rows[0].ChampionPointsSinceLastLevel == nil || rows[0].ChampionPointsUntilNextLevel == nil {
		t.Fatal(rows[0])
	}
	// The same decoder is used by LCU; this is a Riot fixture, not a claimed live LCU capture.
	decoded, err := decodeChampionMasteries(raw)
	if err != nil || !reflect.DeepEqual(decoded, rows) {
		t.Fatal(decoded, err)
	}
	names := map[int64]string{126: "杰斯", 897: "奎桑提", 39: "艾瑞莉娅", 799: "安蓓萨", 68: "兰博", 1: "安妮"}
	result := masteryDetails(rows, names)
	if len(result.Items) != 5 || result.TotalChampions != 6 || result.TotalScore != 264 || result.TotalPoints <= 0 {
		t.Fatal(result)
	}
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/champion-masteries/by-puuid/fixture") {
			t.Fatal(r.URL.Path)
		}
		return r99Response(string(raw)), nil
	})
	if calls != 0 {
		t.Fatal("eager full mastery read")
	}
	for i := 0; i < 2; i++ {
		got, err := p.fullMasteries(context.Background(), "fixture")
		if err != nil || len(got) != 5 {
			t.Fatal(got, err)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	a, ref, _ := newGameplayOverviewSGPFixture(t, false)
	reference, _ := a.resolveGameplayReferenceDetails(ref)
	reference.ServerID = "HN10"
	remote := a.registerGameplayReferenceDetails(reference)
	w := httptest.NewRecorder()
	a.handleGameplayMasteries(w, httptest.NewRequest("GET", "/api/gameplay/masteries?playerRef="+remote, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"available":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestR211SeasonFinalizeNeverMutatesAccumulator(t *testing.T) {
	stats := map[int64]*gameplaySeasonChampionStat{13: {ChampionID: 13, Games: 33, Wins: 20, TotalKills: 330, TotalDeaths: 99, TotalAssists: 198}}
	before := *stats[13]
	first := seasonStatsFinalize(stats, nil)
	for i := 0; i < 3; i++ {
		if got := seasonStatsFinalize(stats, nil); !reflect.DeepEqual(first, got) || *stats[13] != before {
			t.Fatalf("finalize changed raw cache: rows=%+v raw=%+v", got, stats[13])
		}
	}
	if first[0].Kills != 10 || first[0].Deaths != 3 || first[0].Assists != 6 || first[0].KDA != 5.33 {
		t.Fatal(first)
	}
}

func TestR211SeasonCacheRoundTripAndIncrement(t *testing.T) {
	cache := seasonStatsCache{SchemaVersion: 10, Source: seasonStatsSource, Stats: []gameplaySeasonChampionStat{{ChampionID: 13, Games: 2, TotalKills: 10, TotalDeaths: 4, TotalAssists: 6}}}
	raw, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"kills"`) || strings.Contains(string(raw), `"kda"`) {
		t.Fatalf("response averages persisted: %s", raw)
	}
	var loaded seasonStatsCache
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	stats := map[int64]*gameplaySeasonChampionStat{13: &loaded.Stats[0]}
	seasonStatsAccumulate(stats, nil, &riotMatchInfo{QueueID: 420, GameCreation: seasonStartS26.Add(1).UnixMilli(), GameDuration: 1800, Participants: []riotParticipant{{PUUID: "subject", ChampionID: 13, Kills: 8, Deaths: 2, Assists: 9}}}, "subject", seasonStartS26.UnixMilli())
	rows := seasonStatsFinalize(stats, nil)
	if rows[0].Games != 3 || rows[0].Kills != 6 || rows[0].Deaths != 2 || rows[0].Assists != 5 {
		t.Fatal(rows)
	}
	if seasonStatsCount(rows) != seasonStatsOverall(rows).Games {
		t.Fatal("count differs from overall")
	}
}

func TestR211ExpectedGameUsesAllIDsAndRiotFreshBypassesIdentityCache(t *testing.T) {
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("queue") != "" {
			t.Fatal("expected check used queue filter")
		}
		if calls == 1 {
			return r99Response(`["KR_100"]`), nil
		}
		return r99Response(`["KR_9014822625","KR_100"]`), nil
	})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := p.matchIDs(ctx, "fixture", 0, 20); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	ids, err := p.matchIDs(context.WithValue(ctx, overviewFreshHistoryKey{}, true), "fixture", 0, 20)
	if err != nil || calls != 2 {
		t.Fatal(calls, err)
	}
	response := gameplayOverview{Matches: []gameplayMatch{{GameID: 100}}, Pagination: gameplayPagination{Filter: "flex"}}
	applyExpectedOverviewGame(&response, "9014822625", ids)
	if response.ExpectedGamePresent == nil || !*response.ExpectedGamePresent || response.LatestAllGameID != "9014822625" {
		t.Fatal(response)
	}
	applyExpectedOverviewGame(&response, "0", ids)
	if *response.ExpectedGamePresent || response.LatestAllGameID != "9014822625" {
		t.Fatal(response)
	}
}

func TestR211DirtyDiagnosticKeepsAttempts(t *testing.T) {
	a := r175App(t)
	for _, attempt := range []string{"1", "2"} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(`{"event":"overview_dirty_rescan","reason":"retry","attempt":`+attempt+`,"filter":"flex","finished_game_id":"9014822625","finished_queue_id":2400,"expected_present":false,"outcome":"retry","since_end_ms":7000}`)))
		if w.Code != http.StatusNoContent {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	rows := r175Events(t, a, "overview_dirty_rescan")
	if len(rows) != 2 {
		t.Fatal(rows)
	}
}

func TestR211TimelineWholeGameSingleFetchAndLegacyExtraction(t *testing.T) {
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		calls++
		return r99Response(`{"info":{"frames":[{"timestamp":60000,"participantFrames":{"1":{},"2":{},"3":{}},"events":[{"type":"ITEM_PURCHASED","participantId":1,"itemId":1055,"timestamp":30000},{"type":"SKILL_LEVEL_UP","participantId":2,"skillSlot":2,"timestamp":40000},{"type":"ITEM_PURCHASED","participantId":3,"itemId":1001,"timestamp":45000}]}]}}`), nil
	})
	a := &app{riot: p, matchTimelines: newMatchTimelineCache()}
	var whole matchTimelineResponse
	w := httptest.NewRecorder()
	a.handleGameplayMatchTimeline(w, httptest.NewRequest("POST", "/api/gameplay/match-timeline", strings.NewReader(`{"gameId":100,"region":"kr"}`)))
	if err := json.Unmarshal(w.Body.Bytes(), &whole); err != nil || len(whole.Participants) != 3 {
		t.Fatal(w.Body.String(), err)
	}
	for _, id := range []int64{1, 2, 3} {
		w = httptest.NewRecorder()
		body := `{"gameId":100,"region":"kr","participantId":` + strconv.FormatInt(id, 10) + `}`
		a.handleGameplayMatchTimeline(w, httptest.NewRequest("POST", "/api/gameplay/match-timeline", strings.NewReader(body)))
		var got matchTimelineResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		encodedGot, _ := json.Marshal(got)
		encodedWant, _ := json.Marshal(whole.forParticipant(id))
		if string(encodedGot) != string(encodedWant) {
			t.Fatal(got, whole.forParticipant(id))
		}
	}
	if calls != 1 {
		t.Fatalf("fetched whole timeline %d times", calls)
	}
}
