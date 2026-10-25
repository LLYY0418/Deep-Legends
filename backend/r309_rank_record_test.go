package main

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func r309RecordSnapshot(player string) seasonRankRecordSnapshot {
	return seasonRankRecordSnapshot{player: player, progress: seasonStatsProgress{Season: "S26", RankedComplete: true, Scanned: 363}, byQueue: map[int64]gameplayAggregate{420: {Games: 330, Wins: 162, Losses: 168, WinRate: 49}, 440: {Games: 33, Wins: 12, Losses: 21, WinRate: 36}}}
}
func r309UnknownRanks() []gameplayRank {
	return []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Tier: "GOLD", Wins: 162, WinRate: -1}, {QueueType: "RANKED_FLEX_SR", Tier: "GOLD", Wins: 12, WinRate: -1}}
}
func TestR309RankRecordCacheAndPending(t *testing.T) {
	a := &app{}
	snapshot := r309RecordSnapshot("HN1|player")
	filled, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	if filled[0].RecordSource != "season" || filled[0].RecordPending || !filled[0].seasonFallback {
		t.Fatal(filled)
	}
	snapshot.progress.Paused = true
	paused, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	if paused[0].RecordSource != "season-cache" || paused[0].RecordPending {
		t.Fatal(paused)
	}
	snapshot.progress.RankedComplete = false
	snapshot.progress.Paused = true
	snapshot.byQueue = nil
	cached, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	if cached[0].Wins != 162 || cached[0].Losses != 168 || cached[0].RecordSource != "season-cache" || cached[0].RecordPending || !cached[0].seasonFallback {
		t.Fatal(cached)
	}
	changed := r309UnknownRanks()
	changed[0].Wins++
	pending, _ := a.applySeasonRankWinRateFallback(changed, EndpointCapability{}, snapshot)
	if pending[0].WinRate != -1 || !pending[0].RecordPending || pending[0].RecordSource != "none" || pending[1].RecordSource != "season-cache" {
		t.Fatal(pending)
	}
	snapshot.progress.Season = "S27"
	switched, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	if switched[1].WinRate != -1 || !switched[1].RecordPending {
		t.Fatal(switched)
	}
	snapshot = r309RecordSnapshot("HN1|player")
	a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	verified := r309UnknownRanks()
	for i := range verified {
		verified[i].Losses = 20
		verified[i].WinRate = 70
	}
	upstream, _ := a.applySeasonRankWinRateFallback(verified, EndpointCapability{}, snapshot)
	if upstream[0].Losses != 20 || upstream[0].RecordSource != "upstream" || upstream[0].RecordPending {
		t.Fatal(upstream)
	}
	snapshot.progress.RankedComplete = false
	snapshot.byQueue = nil
	invalidated, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	if invalidated[0].WinRate != -1 {
		t.Fatal(invalidated)
	}
	snapshot = r309RecordSnapshot("HN1|empty")
	snapshot.byQueue = map[int64]gameplayAggregate{}
	empty, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	if empty[0].RecordPending || empty[0].WinRate != -1 {
		t.Fatal(empty)
	}
	snapshot.byQueue = nil
	missing, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	if !missing[0].RecordPending {
		t.Fatal(missing)
	}
}
func TestR309RankRecordCacheLRU64Players(t *testing.T) {
	a := &app{}
	for i := 0; i < 64; i++ {
		a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, r309RecordSnapshot(fmt.Sprint(i)))
	}
	snapshot := r309RecordSnapshot("0")
	snapshot.progress.RankedComplete = false
	snapshot.byQueue = nil
	a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, r309RecordSnapshot("64"))
	if len(a.rankRecordCache) != 64 || a.rankRecordCache["0"] == nil || a.rankRecordCache["1"] != nil || a.rankRecordCache["64"] == nil {
		t.Fatal(a.rankRecordCache)
	}
}
func TestR309CachedSeasonRecordNeverWritesLPBaseline(t *testing.T) {
	tracker, ref, queue, baseline, _ := r182Start(t)
	hash := tracker.accountHash(ref)
	a := &app{}
	snapshot := r309RecordSnapshot(ref)
	a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	snapshot.progress.RankedComplete = false
	snapshot.byQueue = nil
	cached, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	tracker.observe(ref, cached)
	if cached[0].RecordSource != "season-cache" || !cached[0].seasonFallback || tracker.history.Baselines[hash][queue] != baseline {
		t.Fatal(cached, tracker.history)
	}
	a.rememberOfficialRankWins(nil, ref, "HN1", cached)
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
	snapshot := r309RecordSnapshot("player")
	a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
	changed := r309UnknownRanks()
	changed[0].Wins++
	for range 6 {
		ranks, _ := a.applySeasonRankWinRateFallback(changed, EndpointCapability{}, snapshot)
		if ranks[0].WinRate != -1 || !ranks[0].RecordPending || ranks[1].WinRate != 36 {
			t.Fatal(ranks)
		}
	}
	aggregate := snapshot.byQueue[420]
	aggregate.Games++
	aggregate.Wins++
	snapshot.byQueue[420] = aggregate
	snapshot.progress.Scanned++
	ranks, _ := a.applySeasonRankWinRateFallback(changed, EndpointCapability{}, snapshot)
	if ranks[0].Wins != 163 || ranks[0].Losses != 168 || ranks[0].RecordSource != "season" || ranks[0].RecordPending {
		t.Fatal(ranks)
	}
}
