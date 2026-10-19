package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func proOPGGFixture(t *testing.T) []proOPGGBuild {
	t.Helper()
	b, err := os.ReadFile("testdata/opgg-esports-builds-worlds-2026-10-19.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Data struct{ Builds []proOPGGBuild }
	}
	if err = json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	for i := range data.Data.Builds {
		data.Data.Builds[i].League = "Worlds"
	}
	return data.Data.Builds
}
func proOPGGNow() time.Time { return time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC) }
func proOPGGReply(rows []proOPGGBuild) *http.Response {
	b, _ := json.Marshal(map[string]any{"data": map[string]any{"builds": rows}})
	return proHTTPBody(b)
}

func TestProOPGGRealRecordsExactChampionAndWindow(t *testing.T) {
	rows := proOPGGFixture(t)
	wrong := rows[0]
	wrong.ID = "other-hero"
	wrong.ChampionID = "59"
	rows = append(rows, wrong)
	configs, games, failed := proOPGGRecommendations(rows, 103, "mid", "阿狸", proOPGGNow(), proOPGGNow(), false)
	if len(configs) != 3 || games != 3 || failed != 0 {
		t.Fatal("wrong champion or old game admitted", len(configs), games, failed)
	}
	first := configs[0]
	encoded, _ := json.Marshal(first)
	var fields map[string]any
	_ = json.Unmarshal(encoded, &fields)
	if wins, exists := fields["recordWins"]; !exists || wins != float64(0) {
		t.Fatal("confirmed zero wins must survive JSON encoding")
	}
	if first.Title != "FUR Tutsz" || first.PlayerName != "Tutsz" || first.ChampionID != 103 || len(first.SelectedPerkIDs) != 9 || len(first.StatModIDs) != 3 || first.SelectedComplete == nil || !*first.SelectedComplete || len(first.ItemIDs) == 0 || first.Spell1ID == 0 || first.Spell2ID == 0 || first.Result != "loss" || !strings.Contains(first.EventLabel, "Worlds") {
		t.Fatalf("incomplete public record: %+v", first)
	}
	if first.Build == nil || len(first.Build.SkillOrder) != 18 || configs[1].Build == nil || len(configs[1].Build.SkillOrder) != 17 || configs[2].Build != nil {
		t.Fatal("skill levels must use only the game's supplied order")
	}
	if configs[1].RecordGames != 2 || configs[1].RecordWins != 2 {
		t.Fatal("per-player record lost", configs[1])
	}
	rows[0].Perk = nil
	configs, _, failed = proOPGGRecommendations(rows[:4], 103, "mid", "阿狸", proOPGGNow(), proOPGGNow(), false)
	if len(configs) != 2 || failed != 1 {
		t.Fatal("incomplete rune page must not be invented", len(configs), failed)
	}
	rows = proOPGGFixture(t)
	rows[0].Player.ID = ""
	_, _, failed = proOPGGRecommendations(rows, 103, "mid", "阿狸", proOPGGNow(), proOPGGNow(), false)
	if failed != 1 {
		t.Fatal("missing identity ID must not merge unrelated player records")
	}
}

func TestProOPGGAnonymousTransportAndBoundaries(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), nil, nil)
	jar, _ := cookiejar.New(nil)
	origin, _ := url.Parse("https://" + esportsOPGGHost)
	jar.SetCookies(origin, []*http.Cookie{{Name: "private", Value: "forbidden"}})
	mode := "ok"
	calls := 0
	p.provider.client = &http.Client{Jar: jar, Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://esports.op.gg/matches/graphql/__query__ListBuilds" || r.Method != "POST" {
			t.Fatal("wrong route", r.URL, r.Method)
		}
		for _, h := range []string{"Cookie", "Authorization", "X-Riot-Token", "X-Api-Key"} {
			if r.Header.Get(h) != "" {
				t.Fatal("credential forwarded", h)
			}
		}
		var body struct{ Variables map[string]any }
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Variables) != 4 || body.Variables["championId"] != "103" || body.Variables["leagueId"] != "96" {
			t.Fatal("unsafe query", body)
		}
		if mode == "redirect" {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://example.com/"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		if mode == "oversized" {
			return proHTTPBody([]byte(strings.Repeat("x", (2<<20)+1))), nil
		}
		if mode == "graphql" {
			return proHTTPBody([]byte(`{"errors":[{"message":"failure"}],"data":null}`)), nil
		}
		return proOPGGReply([]proOPGGBuild{}), nil
	})}
	if _, err := p.fetchOPGGBuilds(t.Context(), 103, "96", 0); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"redirect", "oversized", "graphql"} {
		mode = m
		before := calls
		if _, err := p.fetchOPGGBuilds(t.Context(), 103, "96", 0); err == nil {
			t.Fatal("unsafe response accepted", m)
		}
		if calls != before+1 {
			t.Fatal("redirect followed")
		}
	}
	p.provider.featureGates.apply(map[string]bool{featureGateOPGG: false})
	before := calls
	if _, err := p.fetchOPGGBuilds(t.Context(), 103, "96", 0); err == nil || calls != before {
		t.Fatal("disabled source sent request")
	}
	if featureGateForChampionHost(esportsOPGGHost) != featureGateOPGG || !allowedChampionHost(esportsOPGGHost) || systemProxyTargets()[esportsOPGGHost] != "https://esports.op.gg/" {
		t.Fatal("OP.GG route/gate registration missing")
	}
}

func TestProOPGGCacheSingleflightRestartAndPartialFailure(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	p := newProRuneProvider(proTestProvider(), store, nil)
	now := proOPGGNow()
	p.now = func() time.Time { return now }
	fixture := proOPGGFixture(t)
	var calls atomic.Int32
	mode := "ok"
	transport := championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		var body struct{ Variables struct{ LeagueID string } }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if mode == "all-fail" || mode == "partial" && body.Variables.LeagueID == "96" {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		if body.Variables.LeagueID == "96" {
			return proOPGGReply(fixture), nil
		}
		return proOPGGReply([]proOPGGBuild{}), nil
	})
	p.provider.client = &http.Client{Transport: transport}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := p.opggRecommend(t.Context(), 103, "mid", false)
			if err != nil || len(response.Pros) != 3 {
				t.Error(err, len(response.Pros))
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 5 {
		t.Fatal("duplicate requests", calls.Load())
	}
	restarted := newProRuneProvider(proTestProvider(), store, nil)
	restarted.now = p.now
	restarted.provider.client = p.provider.client
	if r, err := restarted.opggRecommend(t.Context(), 103, "mid", false); err != nil || len(r.Pros) != 3 || calls.Load() != 5 {
		t.Fatal("restart ignored cache", err, calls.Load())
	}
	now = now.Add(time.Minute)
	mode = "partial"
	r, err := p.opggRecommend(t.Context(), 103, "mid", true)
	if err != nil || len(r.Pros) != 3 || !r.Stale || r.Reason == "" {
		t.Fatal("partial failure must keep OP.GG rows without fallback", r, err)
	}
	if !r.ReadAt.Equal(proOPGGNow()) {
		t.Fatal("partial refresh changed the last complete read time", r.ReadAt)
	}
	mode = "all-fail"
	now = now.Add(time.Minute)
	r, err = p.opggRecommend(t.Context(), 103, "mid", true)
	if err == nil || len(r.Pros) != 3 || !r.Stale {
		t.Fatal("overall failure must expose fallback eligibility and retain stale rows", r, err)
	}
}

func TestProOPGGEmptySuccessDoesNotStartOfficialFallback(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), nil, nil)
	p.now = proOPGGNow
	var official atomic.Int32
	p.provider.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != esportsOPGGHost {
			official.Add(1)
		}
		return proOPGGReply([]proOPGGBuild{}), nil
	})}
	a := &app{proRunes: p}
	w := httptest.NewRecorder()
	a.handleGameplayProRunes(w, httptest.NewRequest("GET", "/api/gameplay/pro-runes?championId=103&position=mid", nil))
	var result proRuneResponse
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Reason != "no-sample" || result.Preparing || official.Load() != 0 {
		t.Fatal("empty success incorrectly fell back", result, official.Load())
	}
	// An overall failure reaches the existing official provider (already preparing).
	p.preparing = true
	p.provider.client = &http.Client{Transport: championRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	w = httptest.NewRecorder()
	a.handleGameplayProRunes(w, httptest.NewRequest("GET", "/api/gameplay/pro-runes?championId=103&position=mid&refresh=1", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if !result.Preparing || result.Reason != "preparing" {
		t.Fatal("official fallback missing", result)
	}
}

func TestProOPGGPaginationStopsAtThirtyDaysAndRejectsDisorder(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), nil, nil)
	row := proOPGGFixture(t)[0]
	calls := 0
	disorder := false
	p.provider.client = &http.Client{Transport: championRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if disorder {
			a, b := row, row
			a.Game.BeginAt = proOPGGNow().Add(-time.Hour)
			b.Game.BeginAt = proOPGGNow()
			return proOPGGReply([]proOPGGBuild{a, b}), nil
		}
		if calls == 1 {
			rows := make([]proOPGGBuild, proOPGGPageSize)
			for i := range rows {
				rows[i] = row
				rows[i].ID = strconv.Itoa(i)
			}
			return proOPGGReply(rows), nil
		}
		old := row
		old.Game.BeginAt = proOPGGNow().Add(-31 * 24 * time.Hour)
		return proOPGGReply([]proOPGGBuild{old}), nil
	})}
	rows, err := p.opggLeagueBuilds(t.Context(), 103, "96", "Worlds", proOPGGNow())
	if err != nil || calls != 2 || len(rows) != 100 {
		t.Fatal("pagination/window", len(rows), calls, err)
	}
	disorder = true
	if _, err = p.opggLeagueBuilds(t.Context(), 103, "96", "Worlds", proOPGGNow()); err == nil {
		t.Fatal("unordered feed could silently truncate recent matches")
	}
}

func TestProRuneOutsideRosterUsesOfficialEventMetadata(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), nil, nil)
	event := proTestEvent()
	game := proTestGame()
	event.Match.Teams[0].ID = "123456789"
	event.Match.Teams[0].Code = "OUT"
	game.Metadata.Blue.TeamID = "123456789"
	game.Metadata.Blue.Participants[0].SummonerName = "OUT OutsidePlayer"
	rows := p.rows(proRuneSnapshot{Events: map[string]proRuneEvent{event.ID: event}, Games: map[string]proRuneIndexGame{game.ID: game}}, time.Now())
	if len(rows) != 2 || rows[0].Player != "OutsidePlayer" {
		t.Fatal("official fallback dropped a non-roster player", rows)
	}
}
