package main

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func r309RecordSnapshot(player string) seasonRankRecordSnapshot {
	s := seasonRankRecordSnapshot{player: player, progress: seasonStatsProgress{Season: "S26", RankedComplete: true, Scanned: 363}, byQueue: map[int64]gameplayAggregate{420: {Games: 330, Wins: 162, Losses: 168, WinRate: 49}, 440: {Games: 33, Wins: 12, Losses: 21, WinRate: 36}}}
	for _, q := range []int64{420, 440} {
		v := s.byQueue[q]
		for i := 0; i < v.Games; i++ {
			s.ranked = append(s.ranked, seasonRankedMatch{GameID: int64(len(s.ranked) + 1), CreatedAt: rankSplitStart, QueueID: q, Win: i < v.Wins})
		}
	}
	return s
}
func r309UnknownRanks() []gameplayRank {
	return []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Tier: "GOLD", Wins: 162, WinRate: -1}, {QueueType: "RANKED_FLEX_SR", Tier: "GOLD", Wins: 12, WinRate: -1}}
}
func TestR309RankRecordCacheAndPending(t *testing.T) {
	a := &app{}
	s := r309RecordSnapshot("HN1|player")
	out, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, s)
	if r := out[0]; r.Wins != 162 || r.Losses != 168 || r.RecordSource != "season" || r.RecordPending || !r.seasonFallback {
		t.Fatal(out)
	}
	for _, paused := range []bool{false, true} {
		s.progress.Paused = paused
		s.progress.RankedComplete = false
		out, _ = a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, s)
		if !out[0].RecordPending || out[0].Wins != 0 || out[0].WinRate != -1 {
			t.Fatal(out)
		}
	}
	s.progress.RankedComplete = true
	s.progress.Paused = false
	s.ranked = nil
	out, _ = a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, s)
	if out[0].RecordPending || out[0].Wins != 0 || out[0].Losses != 0 || out[0].WinRate != -1 || out[0].RecordSource != "season" {
		t.Fatal(out)
	}
}
func TestR309RankRecordCacheLRU64Players(t *testing.T) {
	a := &app{}
	for i := 0; i < 65; i++ {
		a.rememberRankSplits(fmt.Sprint(i), map[string]sgpRankedSeason{"RANKED_SOLO_5x5": {CurrentSeasonStart: rankSplitStart}})
	}
	if len(a.rankSplitWindows) != 64 || a.rankSplitWindows["64"] == nil {
		t.Fatal(a.rankSplitWindows)
	}
}
func TestR309CachedSeasonRecordNeverWritesLPBaseline(t *testing.T) {
	tracker, ref, queue, baseline, _ := r182Start(t)
	hash := tracker.accountHash(ref)
	a := &app{}
	out, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, r309RecordSnapshot(ref))
	tracker.observe(ref, out)
	if !out[0].seasonFallback || tracker.history.Baselines[hash][queue] != baseline {
		t.Fatal(out, tracker.history)
	}
	a.rememberOfficialRankWins(nil, ref, "HN1", out)
	if len(a.milestoneCache.wins) != 0 {
		t.Fatal("derived wins entered rank window baseline")
	}
}
func TestR309RankRecordDiagnosticsRejectUnknownValues(t *testing.T) {
	valid := map[string]string{
		"rank_record_state":             `"reason":"known","queue":"solo","state":"known","source":"season","cause":"refresh","ranked_complete":true,"render_seq":1,"foreign":true`,
		"rank_record_final_state":       `"reason":"known","queue":"flex","state":"known","source":"season-cache","foreign":true`,
		"rank_record_downgrade_blocked": `"reason":"retained","queue":"solo","source":"season"`,
		"rank_record_fill_retry":        `"reason":"error","result":"error","attempt":3,"foreign":true`,
		"ranked_record_resolved":        `"reason":"resolved","branch":"budget","queue":"solo","filled_by":"season-cache","upstream_complete":false,"progress_complete":true,"pending":false`,
	}
	for event, fields := range valid {
		t.Run(event, func(t *testing.T) {
			call := func(body string) int {
				w := httptest.NewRecorder()
				(&app{}).handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
				return w.Code
			}
			body := `{"event":"` + event + `",` + fields + `}`
			if call(body) != 204 {
				t.Fatal(body)
			}
			mutations := []string{strings.Replace(body, `"reason":"`, `"reason":"unknown-`, 1)}
			for _, field := range []string{"source", "cause", "state", "queue", "branch", "filled_by", "result"} {
				needle := `"` + field + `":"`
				if strings.Contains(body, needle) {
					mutations = append(mutations, strings.Replace(body, needle, needle+"unknown-", 1))
				}
			}
			if event == "rank_record_fill_retry" {
				mutations = append(mutations, strings.Replace(body, `"attempt":3`, `"attempt":4`, 1))
			}
			for _, mutated := range mutations {
				if call(mutated) != 400 {
					t.Fatalf("accepted unknown value: %s", mutated)
				}
			}
		})
	}
}
func TestR309RankRecordResolvedAggregatesByMinuteAndDimensions(t *testing.T) {
	a := r175App(t)
	now := time.Unix(1800000000, 0)
	event := map[string]any{"event": "ranked_record_resolved", "branch": "budget", "queue": "solo", "upstream_complete": false, "filled_by": "season", "progress_complete": true, "pending": false}
	a.aggregateRankedRecordDiagnostic(event, now)
	a.aggregateRankedRecordDiagnostic(event, now.Add(time.Second))
	a.aggregateRankedRecordDiagnostic(event, now.Add(time.Minute))
	a.flushRankedRecordDiagnostics()
	events := r175Events(t, a, "ranked_record_resolved")
	if len(events) != 2 || events[0]["count"] != float64(2) || events[1]["count"] != float64(1) {
		t.Fatal(events)
	}
	for _, row := range events {
		if row["branch"] != "budget" || row["queue"] != "solo" || row["filled_by"] != "season" {
			t.Fatal(row)
		}
	}
}
func TestR309ChangedWinsCannotReviveTheOldCompletedAggregate(t *testing.T) {
	a := &app{}
	s := r309RecordSnapshot("HN1|player")
	changed := r309UnknownRanks()
	changed[0].Wins++
	for range 6 {
		out, _ := a.applySeasonRankWinRateFallback(changed, EndpointCapability{}, s)
		if out[0].Wins != 162 || out[0].Losses != 168 || out[0].RecordPending || out[1].WinRate != 36 {
			t.Fatal(out)
		}
	}
	s.ranked = append(s.ranked, seasonRankedMatch{GameID: 999, CreatedAt: rankSplitStart, QueueID: 420, Win: true})
	out, _ := a.applySeasonRankWinRateFallback(changed, EndpointCapability{}, s)
	if out[0].Wins != 163 || out[0].Losses != 168 || out[0].RecordPending {
		t.Fatal(out)
	}
}
