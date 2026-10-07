package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func r238SGPFixture(t *testing.T, handler http.HandlerFunc) (*app, *LCUClient, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	a := r175App(t)
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	a.sgp = newSGPProvider()
	a.sgp.http = server.Client()
	a.sgp.serverBases["HN1"] = server.URL
	a.sgp.token, a.sgp.tokenAt, a.sgp.tokenClient = "fixture", time.Now(), client
	return a, client, strings.Repeat("r", 48)
}

func r238Games(ref string) []any {
	return []any{
		map[string]any{"json": map[string]any{"gameId": 23801, "queueId": 420, "gameCreation": time.Now().UnixMilli(), "gameDuration": 1800, "participants": []any{map[string]any{"puuid": ref, "championId": 13, "win": true}}}},
		// The outer page is valid, but this entry has no JSON payload. RawMessage
		// cannot contain illegal JSON inside a successfully decoded outer page.
		map[string]any{"unrelated": "private-payload-must-not-be-logged"},
		map[string]any{"json": map[string]any{"gameId": 23802, "queueId": 420, "gameCreation": time.Now().UnixMilli(), "gameDuration": 1800, "participants": []any{map[string]any{"puuid": ref, "championId": 13, "win": false}}}},
	}
}

func TestR238BadSGPEntryKeepsOtherGamesAndShapeOnly(t *testing.T) {
	var requests atomic.Int32
	ref := strings.Repeat("r", 48)
	a, client, _ := r238SGPFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"games": r238Games(ref)})
	})
	observed := captureSGPObservations(a.sgp)
	cost := &overviewLoadCost{}
	ctx := context.WithValue(t.Context(), overviewLoadCostContextKey{}, cost)
	games, consumed, _, err := a.sgp.matchHistoryFilteredOn(ctx, client, "HN1", ref, 0, 3, []string{"q_420"}, true)
	if err != nil || len(games) != 2 || consumed != 3 || games[0].GameID != 23801 || games[1].GameID != 23802 || cost.decodeFailedCount() != 1 || requests.Load() != 1 {
		t.Fatal("bad entry must not discard good games or refill page", len(games), consumed, err, cost.decodeFailedCount(), requests.Load())
	}
	failures := 0
	for _, event := range *observed {
		if event["event"] != "sgp_game_decode_failed" {
			continue
		}
		failures++
		if event["value_type"] != "invalid-json" || event["payload_bytes"] != 0 || event["first_byte_kind"] != "empty" || event["last_byte_kind"] != "empty" {
			t.Fatal(event)
		}
		encoded, _ := json.Marshal(event)
		if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), ref) {
			t.Fatal("diagnostic contains payload or identity")
		}
	}
	if failures != 1 {
		t.Fatal(failures)
	}
	for _, payload := range [][]byte{[]byte(`{"private":"hidden"`), []byte(`{"participants":[]}`)} {
		_, decodeErr := decodeSGPHistoryGame(payload)
		if decodeErr == nil {
			decodeErr = fmt.Errorf("synthetic decode failure")
		}
		event := sgpGameDecodeDiagnostic(decodeErr, payload)
		if event["payload_bytes"] != len(payload) || event["first_byte_kind"] != "object_open" {
			t.Fatal(event)
		}
		event["content"], event["puuid"] = "private", ref
		encoded, _ := json.Marshal(allowSGPGameDecodeDiagnostic(event))
		if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), ref) {
			t.Fatal("shape allowlist leaked payload")
		}
	}
}

func TestR238TruncatedSGPPageRetriesOnce(t *testing.T) {
	for _, recover := range []bool{true, false} {
		var requests atomic.Int32
		a, client, ref := r238SGPFixture(t, func(w http.ResponseWriter, r *http.Request) {
			n := requests.Add(1)
			if n == 1 || !recover {
				w.Write([]byte(`{"games":[`))
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"games": []any{r238Games(strings.Repeat("r", 48))[0]}})
		})
		games, _, _, err := a.sgp.matchHistoryFilteredOn(t.Context(), client, "HN1", ref, 0, 1, nil, false)
		if requests.Load() != 2 || recover && (err != nil || len(games) != 1) || !recover && err == nil {
			t.Fatal("truncated JSON gets exactly one retry", recover, requests.Load(), len(games), err)
		}
	}
}

func TestR238BadHeadPageRetainsValidStatsWithoutFalseCompletion(t *testing.T) {
	a, client, ref := r238SGPFixture(t, func(w http.ResponseWriter, r *http.Request) {
		games := []any{}
		if r.URL.Query().Get("tag") == "q_420" {
			games = r238Games(strings.Repeat("r", 48))
		}
		json.NewEncoder(w).Encode(map[string]any{"games": games})
	})
	for _, head := range []bool{true, false} {
		cost := &overviewLoadCost{}
		ctx := context.WithValue(t.Context(), overviewLoadCostContextKey{}, cost)
		scan := r231Scan(ref)
		scan.headOnly = head
		a.seasonScanPagesWithHistoryCache(ctx, client, "HN1", ref, scan, 2, false)
		if !scan.interrupted || scan.cache.Complete || scan.scanned != 2 || scan.stats[13] == nil || scan.stats[13].Games != 2 || cost.decodeFailedCount() != 1 {
			t.Fatal("valid games must be retained with incomplete season", head, scan.interrupted, scan.scanned, scan.stats, cost.decodeFailedCount())
		}
		cursor := scan.cache.Streams["ranked"]
		if cursor.Complete || cursor.ResumeIndex != 0 || cursor.StopReason != "decode_failed" {
			t.Fatal("bad page must remain retryable", head, cursor)
		}
	}
}

func TestR238HeadRefreshReportsDecodeFailures(t *testing.T) {
	a, client, ref := r238SGPFixture(t, func(w http.ResponseWriter, r *http.Request) {
		games := []any{}
		if len(r.URL.Query()["tag"]) == 0 {
			games = []any{map[string]any{}}
		}
		json.NewEncoder(w).Encode(map[string]any{"games": games})
	})
	waitGameplaySeasonJobsBeforeCleanup(t, a)
	a.startSeasonStatsRefresh(client, gameplayReference{ServerID: "HN1"}, Summoner{PUUID: ref}, ref, nil, false)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		events := r175Events(t, a, "season_stats_head_refresh")
		if len(events) > 0 {
			if len(events) != 1 || events[0]["decode_failed"] != float64(1) || events[0]["complete"] != false {
				t.Fatal(events)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("head refresh diagnostic not committed")
}

func TestR238ProIdentityTenPlayersTenOverviewOpensAreBounded(t *testing.T) {
	a := r175App(t)
	badge := &proIdentityBadge{PlayerName: "fixture", TeamCode: "fixture"}
	index := proIdentityIndex{byKey: map[string]*proIdentityBadge{}, candidates: 10}
	playerRef := func(player int) string { return strings.Repeat("p", 40) + fmt.Sprintf("%08d", player) }
	for player := 0; player < 10; player++ {
		index.byKey["puuid:"+playerRef(player)] = badge
	}
	for open := 0; open < 10; open++ {
		match := gameplayMatch{GameID: 23801}
		for player := 0; player < 10; player++ {
			ref := gameplayReference{Region: "kr", PlayerRef: playerRef(player)}
			match.Participants = append(match.Participants, gameplayParticipant{PlayerRef: ref.PlayerRef, reference: ref})
		}
		// This is the actual decoration path run for every match on every open.
		a.publicizeMatchReferencesWithProIndex(&match, index)
		for _, participant := range match.Participants {
			if got := participant.ProPlayer; got == nil || *got != *badge {
				t.Fatal("logging guard altered identity result")
			}
		}
	}
	if len(a.proIdentityDiagnosticKeys) != 10 || len(r175Events(t, a, "pro_identity_match")) > 10 || len(r175Events(t, a, "diagnostic_dedup")) != 0 {
		t.Fatal("repeated players flooded diagnostics")
	}
	a.resetDiagnosticDeduplication()
	if a.claimProIdentityDiagnostic(gameplayReference{Region: "kr", PlayerRef: playerRef(0)}, 23801) {
		t.Fatal("log rotation reset the player/game guard")
	}
	if !a.claimProIdentityDiagnostic(gameplayReference{Region: "kr", PlayerRef: playerRef(0)}, 23802) {
		t.Fatal("new game must have a diagnostic slot")
	}
}
