package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR316ExpiredSplitNeverReturnsNaturalYear(t *testing.T) {
	now := time.UnixMilli(rankSplitEnd + 3*3600000)
	for _, next := range []int64{0, rankSplitEnd + 2*3600000} {
		start, source := resolveRankSplit(sgpRankedSeason{CurrentSeasonID: 23, CurrentSeasonStart: rankSplitStart, CurrentSeasonEnd: rankSplitEnd, NextSeasonStart: next}, now)
		want, kind := rankSplitEnd, "sgp-end"
		if next > 0 {
			want, kind = next, "sgp-next"
		}
		if start != want || source != kind {
			t.Fatalf("expired next=%d => %d/%s, want %d/%s", next, start, source, want, kind)
		}
	}
}

func TestR316SplitPersistenceChangeAndConservativeLabels(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store}
	queue := "RANKED_SOLO_5x5"
	old := sgpRankedSeason{CurrentSeasonID: 23, CurrentSeasonStart: rankSplitStart, CurrentSeasonEnd: rankSplitEnd, NextSeasonStart: 1799262000000}
	a.rememberRankSplits("HN1|test", map[string]sgpRankedSeason{queue: old})
	label, _, _ := rankSplitLabel(23, rankSplitStart)
	if label != "2026 赛季第三赛段" {
		t.Fatal(label)
	}
	restarted := &app{storage: store}
	windows, persisted := restarted.rankSplitSnapshot("HN1|test")
	if !persisted || windows[queue] != old {
		t.Fatalf("restart %v/%v", windows, persisted)
	}
	now := time.UnixMilli(1799262000001)
	before := a.rankSplitScope("HN1|test", time.UnixMilli(rankSplitStart))
	cachedKey := rankScoreCacheKeyScoped(dataSourceSGP, "HN1", "test", "|"+before)
	cache := a.ensureRankScores()
	cache.mu.Lock()
	cache.putLocked(cachedKey, rankScoreEntry{at: time.Now()})
	cache.mu.Unlock()
	next := sgpRankedSeason{CurrentSeasonID: 24, CurrentSeasonStart: 1799262000000}
	for i := 0; i < 3; i++ {
		a.rememberRankSplits("HN1|test", map[string]sgpRankedSeason{queue: next})
	}
	if a.rankSplitScope("HN1|test", now) == before {
		t.Fatal("window missing from rank cache scope")
	}
	if _, ok := cache.get(cachedKey); ok {
		t.Fatal("old rank aggregate retained")
	}
	// There is no proof for SGP ids21/22 in available history. Even a matching
	// guessed rule must not turn id24 into an asserted label.
	if label, _, _ := rankSplitLabel(24, next.CurrentSeasonStart); label != "" {
		t.Fatal("unproven mapping", label)
	}
	if label, id, date := rankSplitLabel(23, next.CurrentSeasonStart); label != "" || id == date {
		t.Fatal("mismatch was accepted")
	}
	snap := seasonRankRecordSnapshot{player: "HN1|test", progress: seasonStatsProgress{RankedComplete: true}, ranked: []seasonRankedMatch{{GameID: 1, QueueID: 420, CreatedAt: next.CurrentSeasonStart - 1, Win: true}, {GameID: 2, QueueID: 420, CreatedAt: next.CurrentSeasonStart, Win: false}}}
	out, _ := a.applySeasonRankWinRateFallback([]gameplayRank{{QueueType: queue, WinRate: -1}}, EndpointCapability{}, snap)
	if out[0].Wins != 0 || out[0].Losses != 1 || out[0].RecordSplitFrom != "1月7日" || out[0].RecordSplitLabel != "" {
		t.Fatal(out)
	}
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	a.recordDiagnostic(map[string]any{"event": "r316_log_probe"})
	store.diagnosticMu.Lock()
	err := store.flushDiagnosticLocked()
	store.diagnosticMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(store.root, "logs", "diagnostics.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), `"event":"rank_split_changed"`) != 1 {
		t.Fatal("change diagnostic must occur once", string(raw))
	}
	if !strings.Contains(string(raw), `"event":"rank_split_label_uncertain"`) {
		t.Fatal("missing uncertain diagnostic")
	}
}
func TestR316CorruptOrMissingSplitStateUsesBoundedConstant(t *testing.T) {
	for _, raw := range []string{"", "not json", `{"RANKED_SOLO_5x5":{"currentSeasonId":-1,"currentSeasonStart":10}}`} {
		store := trackTestStore(t, &localStore{root: t.TempDir()})
		if raw != "" {
			if err := writeLocalStoreFile(store, rankSplitStateFile, []byte(raw)); err != nil {
				t.Fatal(err)
			}
		}
		windows, _ := (&app{storage: store}).rankSplitSnapshot("HN1|test")
		if len(windows) != 0 {
			t.Fatal(windows)
		}
		start, source := resolveRankSplit(windows["RANKED_SOLO_5x5"], time.UnixMilli(rankSplitStart))
		if start != rankSplitStart || source != "constant" {
			t.Fatal(start, source)
		}
	}
}

func TestR316ExpiredSplitRefreshBypassesCacheAndBacksOff(t *testing.T) {
	for _, mode := range []string{"new", "old", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int64
			client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
			provider := newSGPProvider()
			provider.serverBases["HN1"] = "https://sgp.test"
			provider.sessionToken, provider.sessionAt, provider.sessionOwner = "test", time.Now(), client
			provider.http = &http.Client{Transport: r278Transport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				body := `{"seasons":{"RANKED_SOLO_5x5":{"currentSeasonId":23,"currentSeasonStart":1785351600000,"currentSeasonEnd":1799247600000,"nextSeasonStart":1799262000000}}}`
				if mode == "new" {
					body = `{"seasons":{"RANKED_SOLO_5x5":{"currentSeasonId":24,"currentSeasonStart":1799262000000}}}`
				}
				if mode == "invalid" {
					body = `not JSON`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			a := &app{sgp: provider}
			a.rememberRankSplits("HN1|player", map[string]sgpRankedSeason{"RANKED_SOLO_5x5": {CurrentSeasonID: 23, CurrentSeasonStart: rankSplitStart, CurrentSeasonEnd: rankSplitEnd, NextSeasonStart: 1799262000000}})
			now := time.UnixMilli(1799262000001)
			for i := 0; i < 2; i++ {
				a.refreshExpiredRankSplit(context.Background(), client, "HN1", "player", false, "PUBLIC", now)
			}
			if calls.Load() != 1 {
				t.Fatalf("did not refresh once with backoff: %d", calls.Load())
			}
			windows, _ := a.rankSplitSnapshot("HN1|player")
			start, source := resolveRankSplit(windows["RANKED_SOLO_5x5"], now)
			if mode == "new" {
				if source != "sgp" || start != 1799262000000 {
					t.Fatal(start, source)
				}
			} else if source != "sgp-next" || start != 1799262000000 {
				t.Fatal(start, source)
			}
			a.refreshExpiredRankSplit(context.Background(), client, "HN1", "player", false, "PUBLIC", now.Add(6*time.Second))
			if mode != "new" && calls.Load() != 2 {
				t.Fatal("failed or old response was not retried")
			}
		})
	}
}
func TestR316PartialQueueResponseAndServerPersistenceIsolation(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	a := &app{storage: store}
	old := sgpRankedSeason{CurrentSeasonID: 23, CurrentSeasonStart: rankSplitStart, CurrentSeasonEnd: rankSplitEnd}
	a.rememberRankSplits("HN1|one", map[string]sgpRankedSeason{"RANKED_SOLO_5x5": old, "RANKED_FLEX_SR": old})
	a.rememberRankSplits("HN1|one", map[string]sgpRankedSeason{"RANKED_SOLO_5x5": old})
	if windows, _ := a.rankSplitSnapshot("HN1|one"); windows["RANKED_FLEX_SR"] != old {
		t.Fatal("partial response erased flex")
	}
	restarted := &app{storage: store}
	if windows, _ := restarted.rankSplitSnapshot("HN2|two"); len(windows) != 0 {
		t.Fatal("cross-server persisted schedule used")
	}
}

func TestR316NewServerSplitSupersedesOtherPlayersStaleMemoryAndCache(t *testing.T) {
	a := &app{}
	queue := "RANKED_SOLO_5x5"
	old := sgpRankedSeason{CurrentSeasonID: 23, CurrentSeasonStart: rankSplitStart, CurrentSeasonEnd: rankSplitEnd}
	next := sgpRankedSeason{CurrentSeasonID: 24, CurrentSeasonStart: 1799262000000}
	now := time.UnixMilli(next.CurrentSeasonStart + 1)
	a.rememberRankSplits("HN1|B", map[string]sgpRankedSeason{queue: old})
	cache := a.ensureRankScores()
	oldScope := a.rankSplitScope("HN1|B", now)
	oldKey := rankScoreCacheKeyScoped(dataSourceSGP, "HN1", "B", "|"+oldScope)
	tierKey := rankScoreCacheKeyScoped(dataSourceSGP, "HN1", "B", rankScoreTierOnlyScope)
	otherKey := rankScoreCacheKeyScoped(dataSourceSGP, "HN2", "C", "|split=old")
	cache.mu.Lock()
	for _, key := range []string{oldKey, tierKey, otherKey} {
		cache.putLocked(key, rankScoreEntry{at: time.Now()})
	}
	cache.mu.Unlock()
	a.rememberRankSplits("HN1|A", map[string]sgpRankedSeason{queue: next})
	// No further SGP response for B: the already known server schedule is enough.
	windows, persisted := a.rankSplitSnapshot("HN1|B")
	if windows[queue] != next || !persisted {
		t.Fatalf("stale player replaced newer shared split: %v persisted=%v", windows, persisted)
	}
	if a.rankSplitScope("HN1|B", now) == oldScope {
		t.Fatal("old cache scope reused")
	}
	if _, ok := cache.get(oldKey); ok {
		t.Fatal("other player's old split aggregate retained")
	}
	for _, key := range []string{tierKey, otherKey} {
		if _, ok := cache.get(key); !ok {
			t.Fatal("unrelated tier/server cache evicted", key)
		}
	}
	snapshot := seasonRankRecordSnapshot{player: "HN1|B", progress: seasonStatsProgress{RankedComplete: true}, ranked: []seasonRankedMatch{{GameID: 1, QueueID: 420, CreatedAt: next.CurrentSeasonStart - 1, Win: true}, {GameID: 2, QueueID: 420, CreatedAt: next.CurrentSeasonStart, Win: false}}}
	ranks, _ := a.applySeasonRankWinRateFallback([]gameplayRank{{QueueType: queue, WinRate: -1}}, EndpointCapability{}, snapshot)
	if ranks[0].Wins != 0 || ranks[0].Losses != 1 || ranks[0].RecordSplitStart != next.CurrentSeasonStart || ranks[0].RecordSplitSource != "persisted" {
		t.Fatalf("old split game leaked into B's card: %+v", ranks[0])
	}
}
