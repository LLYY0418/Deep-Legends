package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func summaryFixtureHTML(puuid string, season int) []byte {
	row, _ := json.Marshal([]any{"$", "$Lfixture", nil, map[string]any{"puuid": puuid, "queueType": "TOTAL", "season": map[string]any{"display_value": "S2026", "season_id": season}}})
	profile, _ := json.Marshal(map[string]any{"region": "kr", "data": map[string]any{"gameName": "Fixture", "tagline": "KR1", "puuid": puuid}})
	push, _ := json.Marshal([]any{1, "52:" + string(profile) + "\n6f:" + string(row) + "\n"})
	return []byte("<script>self.__next_f.push(" + string(push) + ")</script>")
}
func summaryFixtureResponse() []byte {
	return []byte(`0:{"a":"$@17"}
1:[{"champion_stat":{"play":20}}]
17:[{"is_all_champions":true,"play":573,"win":323,"lose":250,"win_rate":56,"kda":{"kda":2.93,"kill":3520,"death":3010,"assist":5310,"avg_kill":6.1,"avg_death":5.3,"avg_assist":9.3},"cs":147,"cs_per_min":5.7},{"is_all_champions":false,"name":"未来守护者","key":"jayce","play":81,"win":42,"lose":39,"win_rate":52,"kda":{"kda":3.29,"kill":716,"death":405,"assist":616,"avg_kill":8.8,"avg_death":5,"avg_assist":7.6},"cs":198,"cs_per_min":7.5}]
`)
}
func TestOPGGSeasonSummaryUsesExplicitAllRowAndPerGameAverages(t *testing.T) {
	meta, err := parseOPGGSummaryMetadata(summaryFixtureHTML("subject-puuid-0000001", 33), "subject-puuid-0000001")
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseOPGGSeasonSummary(summaryFixtureResponse(), meta, map[string]championMetadata{"jayce": {ID: 126, NameZH: "杰斯"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Overall.Games != 573 || result.Overall.Wins != 323 || result.Overall.Losses != 250 || result.Overall.Kills != 6.1 {
		t.Fatalf("wrong season aggregate: %+v", result)
	}
	if result.Champions[0].Kills != 8.8 || result.Champions[0].Games != 81 || result.Champions[0].ChampionID != 126 {
		t.Fatal("used totals as averages, or wrong hero")
	}
	if result.SeasonID != 33 || result.Queue != "RANKED" {
		t.Fatal("lost season/queue scope")
	}
	if _, err := parseOPGGSummaryMetadata(summaryFixtureHTML("other", 33), "subject-puuid-0000001"); err == nil {
		t.Fatal("accepted another account")
	}
	for _, bad := range [][]byte{[]byte(`0:{"a":"$@7"}`), []byte(strings.ReplaceAll(string(summaryFixtureResponse()), `"lose":250`, `"lose":251`)), []byte(strings.ReplaceAll(string(summaryFixtureResponse()), `"is_all_champions":true`, `"is_all_champions":false`))} {
		if _, err := parseOPGGSeasonSummary(bad, meta, map[string]championMetadata{"jayce": {ID: 126}}); err == nil {
			t.Fatal("accepted malformed or inconsistent aggregate")
		}
	}
}
func newSummaryFixtureApp(t *testing.T, requests *atomic.Int32) *app {
	t.Helper()
	c := newChampionProvider()
	c.championMeta = map[int]championMetadata{126: {ID: 126, Key: "Jayce", NameZH: "杰斯"}}
	c.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.Host != "op.gg" || r.URL.Path != "/zh-cn/lol/summoners/kr/Fixture-KR1" {
			t.Fatalf("unexpected upstream: %s", r.URL)
		}
		data := summaryFixtureHTML("subject-puuid-0000001", 33)
		if r.Method == "POST" {
			if r.Header.Get("Next-Action") != opggSeasonSummaryAction {
				t.Fatal("wrong action")
			}
			var body []map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body[0]["season_id"] != float64(33) || body[0]["game_type"] != "RANKED" {
				t.Fatal("wrong season query")
			}
			data = summaryFixtureResponse()
		}
		time.Sleep(10 * time.Millisecond)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
	})}
	return &app{champions: c, opgg: newOPGGInsights()}
}
func TestOPGGSeasonSummarySingleFlightCacheAndPrivacy(t *testing.T) {
	var requests atomic.Int32
	a := newSummaryFixtureApp(t, &requests)
	ref := gameplayReference{Region: "kr", PlayerRef: "subject-puuid-0000001", GameName: "Fixture", TagLine: "KR1"}
	var wait sync.WaitGroup
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := a.opggSeasonSummary(context.Background(), ref, false)
			if err != nil || result.Overall.Games != 573 {
				t.Errorf("summary=%+v error=%v", result, err)
			}
		}()
	}
	wait.Wait()
	if requests.Load() != 2 {
		t.Fatalf("expected one page + one aggregated query, got %d", requests.Load())
	}
	_, _ = a.opggSeasonSummary(context.Background(), ref, true)
	if requests.Load() != 2 {
		t.Fatal("rapid force refresh bypassed cooldown")
	}
	ref.Privacy = "PRIVATE"
	if _, err := a.opggSeasonSummary(context.Background(), ref, false); err == nil {
		t.Fatal("private account used public cached stats")
	}
	if requests.Load() != 2 {
		t.Fatal("private account reached OP.GG")
	}
}
func TestOPGGSeasonSummaryHandlerRequiresOpaqueKRReference(t *testing.T) {
	var calls atomic.Int32
	a := newSummaryFixtureApp(t, &calls)
	invoke := func(ref string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/gameplay/season-summary", strings.NewReader(`{"playerRef":"`+ref+`"}`))
		w := httptest.NewRecorder()
		a.handleOPGGSeasonSummary(w, r)
		return w
	}
	if w := invoke("subject-puuid-0000001"); w.Code != 404 {
		t.Fatal("accepted stable PUUID directly")
	}
	ref := a.registerGameplayReferenceDetails(gameplayReference{Region: "kr", PlayerRef: "subject-puuid-0000001", GameName: "Fixture", TagLine: "KR1"})
	if w := invoke(ref); w.Code != 200 || !strings.Contains(w.Body.String(), `"games":573`) {
		t.Fatalf("handler: %d %s", w.Code, w.Body.String())
	}
	private := a.registerGameplayReferenceDetails(gameplayReference{Region: "kr", PlayerRef: "private-puuid-0000001", Privacy: "PRIVATE", GameName: "Fixture", TagLine: "KR1"})
	if invoke(private).Code != 403 {
		t.Fatal("private reference not blocked")
	}
	if calls.Load() != 2 {
		t.Fatal("invalid reference caused upstream fetch")
	}
}

// Opt-in probe against the public responses observed during this investigation.
// Not required in CI, never reads API keys or contacts Riot.
func TestOPGGSeasonSummaryObservedResponse(t *testing.T) {
	if os.Getenv("OPGG_SUMMARY_OBSERVED_PROBE") != "1" {
		t.Skip("explicit local fixture probe")
	}
	raw, err := os.ReadFile("/private/tmp/opgg-season-response.txt")
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile("/private/tmp/opgg-jugking-season.html")
	if err != nil {
		t.Fatal(err)
	}
	req, err := os.ReadFile("/private/tmp/opgg-season-request.json")
	if err != nil {
		t.Fatal(err)
	}
	var request []struct {
		PUUID string `json:"puuid"`
	}
	if json.Unmarshal(req, &request) != nil || len(request) != 1 {
		t.Fatal("invalid observed request")
	}
	meta, err := parseOPGGSummaryMetadata(page, request[0].PUUID)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]championMetadata{}
	for k, id := range map[string]int{"jayce": 126, "ambessa": 799, "leesin": 64, "qiyana": 246, "camille": 164, "jarvaniv": 59, "chogath": 31} {
		byKey[k] = championMetadata{ID: id}
	}
	result, err := parseOPGGSeasonSummary(raw, meta, byKey)
	if err != nil {
		t.Fatal(err)
	}
	if result.Overall.Games != 573 || len(result.Champions) != 7 {
		t.Fatalf("unexpected observed summary: %+v", result)
	}
	t.Logf("verified %s: games=%d wins=%d losses=%d, top heroes=%d", result.Season, result.Overall.Games, result.Overall.Wins, result.Overall.Losses, len(result.Champions))
	if os.Getenv("OPGG_SUMMARY_LIVE_FETCH") == "1" {
		c := newChampionProvider()
		c.championMeta = map[int]championMetadata{}
		for key, item := range byKey {
			item.Key = key
			c.championMeta[item.ID] = item
		}
		a := &app{champions: c, opgg: newOPGGInsights()}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		live, err := a.opggSeasonSummary(ctx, gameplayReference{Region: "kr", GameName: "JUGKlNG", TagLine: "kr", PlayerRef: request[0].PUUID}, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("production fetch: %s games=%d wins=%d losses=%d top=%d", live.Season, live.Overall.Games, live.Overall.Wins, live.Overall.Losses, len(live.Champions))
	}
}
