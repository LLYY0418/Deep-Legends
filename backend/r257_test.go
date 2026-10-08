package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestR257PersistedBoundariesAcrossThreeEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		capped, ranked, mayhem bool
	}{
		{"both", true, true, true}, {"uncapped", false, false, false}, {"ranked-only", true, true, false}, {"mayhem-only", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, public, _ := newGameplayOverviewSGPFixture(t, false)
			a.storage = r175App(t).storage
			ref, _ := a.resolveGameplayReferenceDetails(public)
			season, _ := currentRankedSeason(time.Now())
			cache := seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: season, AccountHash: a.storage.accountHash(Summoner{PUUID: ref.PlayerRef}), Complete: true, HeadCheckedAt: time.Now(), CappedByUpstream: tc.capped,
				Stats:   []gameplaySeasonChampionStat{{ChampionID: 103, Games: 208}},
				Streams: map[string]seasonStatsStream{"ranked": {Complete: true, CappedByUpstream: tc.ranked, OldestCreatedAt: 1775889886204}, "mayhem": {Complete: true, CappedByUpstream: tc.mayhem, OldestCreatedAt: 1778580000000}}}
			if _, err := a.storage.saveSeasonStatsReported(cache); err != nil {
				t.Fatal(err)
			}
			for _, endpoint := range []string{"overview", "season-summary", "champion-table"} {
				w := httptest.NewRecorder()
				switch endpoint {
				case "overview":
					a.handleGameplayOverview(w, httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"playerRef":"`+public+`","count":20}`)))
				case "season-summary":
					a.handleGameplaySeasonSummary(w, httptest.NewRequest(http.MethodGet, "/api/gameplay/season-summary?playerRef="+public, nil))
				case "champion-table":
					a.handleGameplayChampionTable(w, httptest.NewRequest(http.MethodGet, "/api/gameplay/champion-table?queue=440&playerRef="+public, nil))
				}
				var response struct {
					Progress map[string]any `json:"seasonStatsProgress"`
				}
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil {
					t.Fatal(endpoint, w.Code, w.Body.String())
				}
				p := response.Progress
				if p == nil {
					t.Fatal(endpoint, "missing progress")
				}
				if got, ok := p["upstreamCapped"]; tc.capped {
					if !ok || got != true {
						t.Fatal(endpoint, "missing cap", p)
					}
				} else if ok {
					t.Fatal(endpoint, "uncapped flag", p)
				}
				for key, want := range map[string]int64{"rankedOldestAt": 1775889886204, "mayhemOldestAt": 1778580000000} {
					valid := tc.capped && ((key == "rankedOldestAt" && tc.ranked) || (key == "mayhemOldestAt" && tc.mayhem))
					got, ok := p[key]
					if valid {
						if !ok || got != float64(want) {
							t.Fatal(endpoint, key, "boundary mismatch", p)
						}
					} else if ok {
						t.Fatal(endpoint, key, "uncapped date", p)
					}
				}
			}
			// These read-only endpoints must not start any season history request.
			a.seasonBackfillMu.Lock()
			running := len(a.seasonBackfills)
			a.seasonBackfillMu.Unlock()
			if running != 0 {
				t.Fatal("boundary reads started a scan")
			}
		})
	}
}

func TestR257RefreshAndSSEPreserveBoundaries(t *testing.T) {
	for _, failed := range []bool{false, true} {
		a, public, _ := newGameplayOverviewSGPFixture(t, false)
		a.storage = r175App(t).storage
		ref, _ := a.resolveGameplayReferenceDetails(public)
		season, _ := currentRankedSeason(time.Now())
		cache := seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: season, AccountHash: a.storage.accountHash(Summoner{PUUID: ref.PlayerRef}), Complete: true, HeadCheckedAt: time.Now(), CappedByUpstream: true,
			Streams: map[string]seasonStatsStream{"ranked": {Complete: true, CappedByUpstream: true, OldestCreatedAt: 1775889886204}, "mayhem": {Complete: true, CappedByUpstream: true, OldestCreatedAt: 1778580000000}}}
		if _, err := a.storage.saveSeasonStatsReported(cache); err != nil {
			t.Fatal(err)
		}
		if failed {
			a.sgp.http.Transport = sgpRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("synthetic failure") })
		}
		p, skipped, _, _ := a.refreshSeasonHead(t.Context(), a.lcu, ref, Summoner{PUUID: ref.PlayerRef}, ref.PlayerRef, nil)
		if !p.UpstreamCapped || p.RankedOldestAt != 1775889886204 || p.MayhemOldestAt != 1778580000000 || skipped == failed {
			t.Fatal("refresh lost boundary", failed, p, skipped)
		}
		events := make(chan string, 1)
		a.eventSubscribers = map[chan string]struct{}{events: {}}
		a.publishSeasonSnapshot(ref, ref.PlayerRef, cache)
		var payload struct {
			Snapshot struct {
				Progress seasonStatsProgress `json:"seasonStatsProgress"`
			} `json:"snapshot"`
		}
		if err := json.Unmarshal([]byte(<-events), &payload); err != nil {
			t.Fatal(err)
		}
		got := payload.Snapshot.Progress
		if !got.UpstreamCapped || got.RankedOldestAt != 1775889886204 || got.MayhemOldestAt != 1778580000000 {
			t.Fatal("SSE lost boundary", got)
		}
	}
}
