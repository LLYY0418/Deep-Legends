package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The snapshot observer only orders I/O. No rank/progress result is replaced.
func TestR309RankRecordOverviewPaths(t *testing.T) {
	for _, branch := range []string{"normal", "budget-newer-snapshot", "manual-head-skipped", "stream-final"} {
		t.Run(branch, func(t *testing.T) {
			a, public, _ := newGameplayOverviewSGPFixture(t, false)
			a.storage = trackTestStore(t, &localStore{root: t.TempDir()})
			waitGameplaySeasonJobsBeforeCleanup(t, a)
			if err := os.MkdirAll(filepath.Join(a.storage.root, "logs"), 0700); err != nil {
				t.Fatal(err)
			}
			subject := strings.Repeat("o", 48)
			season, _ := currentRankedSeason(time.Now())
			cache := seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: season, AccountHash: a.storage.accountHash(Summoner{PUUID: subject}), Complete: branch != "budget-newer-snapshot", HeadCheckedAt: time.Now(), HeadGameID: 420, UpdatedAt: time.Now(), Stats: []gameplaySeasonChampionStat{{ChampionID: 103, Games: 363, Wins: 174}}, QueueStats: map[int64]gameplayAggregate{420: {QueueID: 420, Games: 330, Wins: 162, Losses: 168, WinRate: 49}, 440: {QueueID: 440, Games: 33, Wins: 12, Losses: 21, WinRate: 36}}}
			if branch == "budget-newer-snapshot" {
				cache.Stats = nil
				cache.QueueStats = nil
			}
			if branch == "manual-head-skipped" {
				cache.HeadGameID = 0
			}
			if _, err := a.storage.saveSeasonStatsReported(cache); err != nil {
				t.Fatal(err)
			}
			// A live scan is already in flight; manual refresh shares it instead of
			// manufacturing a completion event. The persisted recent head also skips
			// automatic refreshes in the other three paths.
			if branch == "budget-newer-snapshot" {
				a.seasonBackfills = map[string]struct{}{sourceScopedKey(seasonStatsSource, cache.AccountHash+"|"+season): {}}
			}
			firstSnapshot, rankReady := make(chan struct{}), make(chan struct{})
			var snapshotOnce, rankOnce, updateOnce sync.Once
			a.overviewSnapshotLoaded = func() { snapshotOnce.Do(func() { close(firstSnapshot) }) }
			transport := a.sgp.http.Transport
			a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "/leagues-ledge/") {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","wins":162},{"queueType":"RANKED_FLEX_SR","tier":"GOLD","wins":12}]}`)), Request: r}, nil
				}
				if branch == "budget-newer-snapshot" && strings.Contains(r.URL.Path, "/match-history-query/") {
					<-firstSnapshot
					<-rankReady
					updateOnce.Do(func() {
						latest := cache
						latest.Complete = true
						latest.Stats = []gameplaySeasonChampionStat{{ChampionID: 103, Games: 363, Wins: 174}}
						latest.QueueStats = map[int64]gameplayAggregate{420: {Games: 330, Wins: 162, Losses: 168, WinRate: 49}, 440: {Games: 33, Wins: 12, Losses: 21, WinRate: 36}}
						if _, err := a.storage.saveSeasonStatsReported(latest); err != nil {
							t.Error(err)
						}
					})
					return nil, context.DeadlineExceeded
				}
				return transport.RoundTrip(r)
			})
			lcu := a.lcu.http.Transport
			a.lcu.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "/lol-ranked/") {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","wins":162},{"queueType":"RANKED_FLEX_SR","tier":"GOLD","wins":12}]}`)), Request: r}, nil
				}
				return lcu.RoundTrip(r)
			})
			// Cards callback runs after upstream rank resolution, so the budget branch
			// can expire only after ranks and the old snapshot are both actually ready.
			ctx := context.WithValue(context.Background(), localOverviewCardsProgressKey{}, func(p gameplayOverview) {
				if len(p.Ranks) > 0 {
					rankOnce.Do(func() { close(rankReady) })
				}
			})
			var expire atomic.Pointer[context.CancelFunc]
			if branch == "budget-newer-snapshot" {
				base := a.sgp.http.Transport
				a.overviewTimeout = func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
					c, cancel := context.WithCancel(parent)
					expire.Store(&cancel)
					return r86DeadlineContext{c}, cancel
				}
				a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					res, err := base.RoundTrip(r)
					if err == context.DeadlineExceeded {
						(*expire.Load())()
					}
					return res, err
				})
			}
			body := `{"playerRef":"` + public + `","count":20,"force":false}`
			if branch == "budget-newer-snapshot" {
				body = `{"playerRef":"` + public + `","count":20,"force":true}`
			}
			if branch == "manual-head-skipped" {
				body = `{"playerRef":"` + public + `","count":20,"force":true,"freshHistory":true}`
			}
			req := httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(body)).WithContext(ctx)
			if branch == "stream-final" {
				req.Header.Set("Accept", "application/x-ndjson")
			}
			recorder := httptest.NewRecorder()
			a.handleGameplayOverview(recorder, req)
			if recorder.Code != 200 {
				t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
			}
			var got gameplayOverview
			if branch == "stream-final" {
				decoder := json.NewDecoder(recorder.Body)
				progress, complete := false, false
				for decoder.More() {
					var frame struct {
						Type     string           `json:"type"`
						Overview gameplayOverview `json:"overview"`
					}
					if err := decoder.Decode(&frame); err != nil {
						t.Fatal(err)
					}
					if frame.Type == "progress" {
						progress = true
					}
					if frame.Type == "complete" {
						complete = true
						got = frame.Overview
					}
				}
				if !progress || !complete {
					t.Fatalf("missing stream frame: progress=%v complete=%v", progress, complete)
				}
			} else if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if branch == "budget-newer-snapshot" && !got.Pagination.BudgetExceeded {
				t.Fatal("budget branch did not run")
			}
			if !got.SeasonStatsProgress.RankedComplete {
				t.Fatalf("final progress is incomplete: %+v", got.SeasonStatsProgress)
			}
			expected := map[string][3]int{"RANKED_SOLO_5x5": {162, 168, 49}, "RANKED_FLEX_SR": {12, 21, 36}}
			if len(got.Ranks) != 2 {
				t.Fatalf("ranks=%+v", got.Ranks)
			}
			for _, rank := range got.Ranks {
				if [3]int{rank.Wins, rank.Losses, rank.WinRate} != expected[rank.QueueType] {
					t.Errorf("%s lost season fill: got %d胜 %d负 rate=%d, want %v", branch, rank.Wins, rank.Losses, rank.WinRate, expected[rank.QueueType])
				}
			}
			if branch == "manual-head-skipped" {
				if err := r281AwaitSeasonJobs(a, "manual no-new-game head must finish", 3); err != nil {
					t.Fatal(err)
				}
				events := r175Events(t, a, "season_stats_head_refresh")
				found := false
				for _, event := range events {
					if event["skipped"] == true && event["skip_reason"] == "no_new_game" {
						found = true
					}
				}
				if !found {
					t.Fatalf("manual refresh did not skip its unchanged head: %v", events)
				}
			}
			// The in-flight marker was fixture-owned, not an actual background job.
			if branch == "budget-newer-snapshot" {
				a.seasonBackfillMu.Lock()
				clear(a.seasonBackfills)
				a.seasonBackfillMu.Unlock()
			}
		})
	}
}
