package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Feed protocol JSON through the real field detector, enrichment and live
// grouping path. No hand-built liveClientArenaGrouping or guard-only calls.
func TestR96ExplicitSubteamGuard(t *testing.T) {
	for _, field := range []string{"subteamId", "playerSubteamId"} {
		for _, conflict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/conflict=%t", field, conflict), func(t *testing.T) {
				players, raw := r62ArenaFixturePlayers(t, 18)
				var entries []map[string]any
				if err := json.Unmarshal(raw, &entries); err != nil {
					t.Fatal(err)
				}
				for i := range entries {
					entries[i][field] = (i/3+3)%6 + 1
				}
				// Swap two actual subteam values, not list positions. Each group is still
				// structurally valid (six groups of three), but the known squad is split.
				if conflict {
					entries[2][field], entries[3][field] = entries[3][field], entries[2][field]
				}
				raw, err := json.Marshal(entries)
				if err != nil {
					t.Fatal(err)
				}
				snapshot, shape, err := parseLiveClientPlayerList(raw, 3)
				if err != nil || !shape.Grouped || shape.GroupField != field || snapshot.Grouping.IdentifiedPlayers != 18 {
					t.Fatalf("field detector fixture failed: %+v %v", shape, err)
				}
				a := r90Fixture(t, players, raw)
				r90RememberAllies(a, 0, 3)
				a.liveClientAllGameData = func(context.Context) ([]byte, int, error) { return []byte(`{}`), 200, nil }
				got := a.loadGameplayLive(context.Background(), a.lcu, a.summoner, "InProgress")
				marked := 0
				for _, p := range got.Players {
					if p.MySquad {
						marked++
					}
					if conflict && p.ArenaGroup != "" {
						t.Fatalf("conflicting candidate accepted: %q", p.ArenaGroup)
					}
				}
				if marked != 3 {
					t.Fatal("guard erased known squad", marked)
				}
				events := r90Events(t, a, "arena_group_order_rejected")
				if conflict {
					if got.ArenaGrouped || got.ArenaMascotMapping || !got.ArenaGroupingUnavailable || got.ArenaGroupingRetryable {
						t.Fatalf("conflict not hard-rejected: %+v", got)
					}
					if len(events) != 1 || events[0]["reason"] != "allies-cross-blocks" || events[0]["source"] != "live-client" {
						t.Fatal("wrong rejection path", events)
					}
				} else {
					if !got.ArenaGrouped || got.ArenaGroupingUnavailable || got.ArenaGroupSource != "live-client" {
						t.Fatal("consistent explicit groups were rejected", events)
					}
					for i, p := range got.Players {
						if p.ArenaGroup != fmt.Sprint((i/3+3)%6+1) {
							t.Fatal("explicit group changed", i, p.ArenaGroup)
						}
					}
					if len(events) != 0 {
						t.Fatal("unexpected rejection", events)
					}
				}
			})
		}
	}
}

func TestR96PartialTruthDoesNotEndAsNone(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	a := r90Fixture(t, players, raw)
	a.lcu.platformProbe = true
	a.lcu.region = "TENCENT"
	a.lcu.rsoPlatform = "HN1"
	r90RememberAllies(a, 0, 3)
	a.loadGameplayLive(context.Background(), a.lcu, a.summoner, "InProgress")
	info := r90TruthInfo()
	info.Participants[17].PlayerSubteamID = 0 // Unrelated malformed participant.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"games": []map[string]any{{"json": info}}})
	}))
	defer server.Close()
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token = "fixture"
	provider.tokenAt = time.Now()
	provider.tokenClient = a.lcu
	a.sgp = provider
	// Exhaust the real retry schedule, rather than directly testing the emitter.
	a.finishArenaGroupTruth(context.Background(), a.lcu)
	if calls.Load() != 4 {
		t.Fatal("did not exhaust bounded retries", calls.Load())
	}
	events := r90Events(t, a, "arena_group_truth_check")
	if len(events) != 4 {
		t.Fatal("partial truth must not be followed by none", events)
	}
	for _, e := range events {
		if e["truth_source"] != "sgp" || e["conclusive"] != false || e["my_squad_correct"] != true {
			t.Fatal("partial truth changed production verdict", e)
		}
	}
	a.arenaTruth.mu.Lock()
	checked := a.arenaTruth.records[0].checked
	a.arenaTruth.mu.Unlock()
	if checked {
		t.Fatal("incomplete truth marked complete")
	}
	// A later live observation cannot erase the evidence. No transport still
	// must not replace a real partial truth with a misleading none.
	a.sgp = nil
	a.loadGameplayLive(context.Background(), a.lcu, a.summoner, "Reconnect")
	a.finishArenaGroupTruth(context.Background(), a.lcu)
	if len(r90Events(t, a, "arena_group_truth_check")) != 4 {
		t.Fatal("reconnect lost observed-truth state", r90Events(t, a, "arena_group_truth_check"))
	}
	info.Participants[17].PlayerSubteamID = 6
	a.checkArenaGroupTruth(a.lcu, "HN1", info)
	events = r90Events(t, a, "arena_group_truth_check")
	if len(events) != 5 || events[4]["conclusive"] != true {
		t.Fatal("later complete truth no longer accepted", events)
	}
}
