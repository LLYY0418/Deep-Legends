package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestR286RankedCompletesWithoutMayhem(t *testing.T) {
	var p seasonStatsProgress
	if err := json.Unmarshal([]byte(`{"complete":false,"rankedComplete":true,"paused":true}`), &p); err != nil {
		t.Fatal(err)
	}
	a := &app{}
	ranks, _ := a.applySeasonRankWinRateFallback([]gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: 7, WinRate: -1}}, EndpointCapability{}, p, map[int64]gameplayAggregate{420: {Games: 10, Wins: 7, Losses: 3, WinRate: 70}})
	if ranks[0].Losses != 3 || ranks[0].WinRate != 70 {
		t.Fatalf("ranked complete must ignore stuck mayhem: %+v", ranks)
	}
}
func r286SeasonFixture(t *testing.T, empty, boundary, down bool) (*app, *LCUClient, string) {
	t.Helper()
	ref := strings.Repeat("z", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down {
			http.Error(w, "unavailable", 503)
			return
		}
		games := []any{}
		if strings.Contains(r.URL.RawQuery, "q_420") {
			games = append(games, map[string]any{"json": map[string]any{"gameId": 28601, "queueId": 420, "gameCreation": time.Now().UnixMilli(), "gameDuration": 1800, "participants": []any{map[string]any{"puuid": ref, "championId": 103, "win": true}}}})
			if empty {
				games = append(games, map[string]any{})
			} else {
				games = append(games, map[string]any{"json": "broken-nonempty"})
			}
			if boundary {
				games = append(games, map[string]any{"json": map[string]any{"gameId": 28600, "queueId": 420, "gameCreation": seasonStartS26.Add(-time.Second).UnixMilli()}})
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"games": games})
	}))
	t.Cleanup(server.Close)
	a := r175App(t)
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	a.sgp = newSGPProvider()
	a.sgp.http = server.Client()
	a.sgp.serverBases["HN1"] = server.URL
	a.sgp.token = "test"
	a.sgp.tokenAt = time.Now()
	a.sgp.tokenClient = client
	return a, client, ref
}
func TestR286EmptyNeedsTwoDistinctScans(t *testing.T) {
	for _, head := range []bool{false, true} {
		t.Run(map[bool]string{false: "backfill", true: "head"}[head], func(t *testing.T) {
			a, c, ref := r286SeasonFixture(t, true, false, false)
			scan := r231Scan(ref)
			scan.headOnly = head
			a.seasonScanPagesWithHistoryCache(context.Background(), c, "HN1", ref, scan, 2, false)
			if scan.cache.Streams["ranked"].Complete || scan.cache.Streams["ranked"].StopReason != "decode_failed" {
				t.Fatal("first empty must stop", scan.cache)
			}
			// Persist and recreate state: a second scan must survive app/cache reload.
			hash := a.storage.accountHash(Summoner{PUUID: ref})
			scan.cache.AccountHash = hash
			a.finishSeasonScan(scan, nil, hash)
			cached, err := a.storage.loadSeasonStats(seasonStatsSource, hash, "S26")
			if err != nil {
				t.Fatal(err)
			}
			next := r231Scan(ref)
			next.cache = cached
			next.headOnly = head
			next.seen = map[int64]bool{28601: true}
			a.seasonScanPagesWithHistoryCache(context.Background(), c, "HN1", ref, next, 2, false)
			b, _ := json.Marshal(next.cache)
			var saved map[string]any
			json.Unmarshal(b, &saved)
			if !next.cache.Streams["ranked"].Complete || saved["skippedUndecodable"] != float64(1) {
				t.Fatalf("second empty must skip exactly once: %s", b)
			}
		})
	}
}
func TestR286NonemptyNeverSkippedAndBoundaryWins(t *testing.T) {
	for _, boundary := range []bool{false, true} {
		for _, head := range []bool{false, true} {
			a, c, ref := r286SeasonFixture(t, false, boundary, false)
			s := r231Scan(ref)
			s.headOnly = head
			for i := 0; i < 2; i++ {
				a.seasonScanPagesWithHistoryCache(t.Context(), c, "HN1", ref, s, 2, false)
			}
			if s.cache.Streams["ranked"].Complete != boundary {
				t.Fatalf("boundary=%v head=%v: %+v", boundary, head, s.cache)
			}
			if boundary && (s.cache.Streams["ranked"].StopReason != "season_start" || s.queueStats[420].Games != 1) {
				t.Fatal(s.cache, s.queueStats)
			}
		}
	}
}
func TestR286FailedHeadPausesAndPersists(t *testing.T) {
	a, c, ref := r286SeasonFixture(t, true, false, true)
	p, _, _, _ := a.refreshSeasonHead(t.Context(), c, gameplayReference{ServerID: "HN1"}, Summoner{PUUID: ref}, ref, nil)
	b, _ := json.Marshal(p)
	var state map[string]any
	json.Unmarshal(b, &state)
	if state["paused"] != true || p.Collecting {
		t.Fatalf("outage must pause: %s", b)
	}
	_, snapshot, _, _ := a.loadSeasonChampionStatsSnapshot(gameplayReference{ServerID: "HN1"}, Summoner{PUUID: ref}, ref)
	b, _ = json.Marshal(snapshot)
	json.Unmarshal(b, &state)
	if state["paused"] != true || snapshot.Collecting {
		t.Fatalf("persisted pause: %s", b)
	}
}

func TestR286RankRecordDiagnosticHTTP(t *testing.T) {
	a := r175App(t)
	for _, queue := range []string{"solo", "flex"} {
		for _, state := range []string{"known", "collecting", "paused", "unavailable"} {
			r := httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(`{"event":"rank_record_state","reason":"`+state+`","queue":"`+queue+`","state":"`+state+`"}`))
			w := httptest.NewRecorder()
			a.handleClientDiagnostic(w, r)
			if w.Code != 204 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
	}
	events := r175Events(t, a, "rank_record_state")
	if len(events) != 8 {
		t.Fatal(events)
	}
	for _, e := range events {
		if e["queue"] != "solo" && e["queue"] != "flex" {
			t.Fatal(e)
		}
		if e["state"] == nil {
			t.Fatal(e)
		}
	}
}
func TestR286SameScanCannotAgeEmpty(t *testing.T) {
	s := r231Scan("test")
	s.scanID = 1
	for i := 0; i < 3; i++ {
		if !s.seasonDecodeIncomplete("ranked", 0, 1, []sgpEmptyHistoryEntry{{Offset: 150}}) {
			t.Fatal("same scan skipped empty")
		}
	}
	if s.cache.SkippedUndecodable != 0 {
		t.Fatal(s.cache)
	}
	s.scanID = 2
	if s.seasonDecodeIncomplete("ranked", 0, 1, []sgpEmptyHistoryEntry{{Offset: 150}}) || s.cache.SkippedUndecodable != 1 {
		t.Fatal(s.cache)
	}
}
