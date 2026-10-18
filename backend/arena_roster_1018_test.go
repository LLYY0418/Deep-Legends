package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Replay the 1018 log's timing/shape with synthetic identities: gameflow 17,
// LiveClient unavailable three times, then 18 with Karma at missing index 4.
// Also cover a successful but incomplete first playerlist so it cannot freeze.
func TestArenaKarmaRosterRetriesToEighteen(t *testing.T) {
	for _, firstSource := range []string{"unavailable", "seventeen", "alias-delayed"} {
		t.Run(firstSource, func(t *testing.T) {
			f := newR183Fixture(t)
			f.current = Summoner{PUUID: r161Ref(0), GameName: "Player0", TagLine: "CN1"}
			const gameID int64 = 9036435746
			var flow lcuGameflowSession
			flow.GameData.GameID, flow.GameData.Queue.ID, flow.GameData.Queue.GameMode = gameID, 1750, "CHERRY"
			var live []map[string]any
			for i := 0; i < 18; i++ {
				champion, title := int64(1), "黑暗之女"
				if i == 4 {
					champion, title = 43, "天启者"
				} else {
					flow.GameData.TeamOne = append(flow.GameData.TeamOne, lcuLivePlayer{PUUID: r161Ref(i), ChampionID: champion})
				}
				live = append(live, map[string]any{"riotId": fmt.Sprintf("Player%d#CN1", i), "championName": title, "team": "ORDER", "position": "OTHER"})
			}
			var aliasAttempts atomic.Int64
			old := f.client.http.Transport
			f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if firstSource == "alias-delayed" && r.URL.Path == "/lol-summoner/v1/alias/lookup" && aliasAttempts.Add(1) <= 3 {
					return r178JSON(map[string]any{}, 404), nil
				}
				if r.URL.Path == "/lol-gameflow/v1/session" {
					return r178JSON(flow, 200), nil
				}
				return old.RoundTrip(r)
			})
			now := time.Now()
			f.app.liveClientPlayerListNow = func() time.Time { return now }
			var probes atomic.Int64
			f.app.liveClientPlayerList = func(context.Context) ([]byte, int, error) {
				n := probes.Add(1)
				if n <= 3 && firstSource != "alias-delayed" {
					if firstSource == "unavailable" {
						return nil, 503, fmt.Errorf("not started")
					}
					partial := append([]map[string]any(nil), live[:4]...)
					partial = append(partial, live[5:]...)
					data, _ := json.Marshal(partial)
					return data, 200, nil
				}
				data, _ := json.Marshal(live)
				return data, 200, nil
			}
			for attempt := 0; attempt < 4; attempt++ {
				f.app.liveSnapshots.mu.Lock()
				f.app.liveSnapshots.at = time.Now().Add(-4 * time.Second)
				f.app.liveSnapshots.mu.Unlock()
				now = now.Add(10 * time.Second)
				result := f.app.cachedGameplayLive(context.Background(), f.client, f.current, "InProgress")
				want := 17
				if attempt == 3 || firstSource == "alias-delayed" {
					want = 18
				}
				if len(result.Players) != want {
					t.Fatalf("attempt=%d players=%d want=%d", attempt, len(result.Players), want)
				}
				if attempt < 3 && gameplayLiveSnapshotComplete(result) {
					t.Fatal("17-player snapshot froze retries")
				}
				if attempt == 3 {
					count := 0
					for _, p := range result.Players {
						if p.reference.PlayerRef == r161Ref(4) {
							count++
							if p.ChampionID != 43 || p.GameName != "Player4" || p.Hidden || p.PlayerRef == "" || p.HistoryState == "unavailable" {
								t.Fatalf("Karma not enriched: %+v", p)
							}
						}
					}
					if count != 1 {
						t.Fatal("Karma count", count)
					}
					if f.count("/lol-match-history/v1/products/lol/"+r161Ref(4)) == 0 {
						t.Fatal("recovered Karma skipped history")
					}
					encoded, _ := json.Marshal(result)
					if strings.Contains(string(encoded), r161Ref(4)) {
						t.Fatal("raw reference leaked")
					}
				}
			}
			expectedProbes := 4
			if firstSource == "alias-delayed" {
				expectedProbes = 1
			}
			if probes.Load() != int64(expectedProbes) {
				t.Fatal("incomplete playerlist cached permanently", probes.Load())
			}
			events := r175Events(t, f.app, "arena_identity_recovery")
			found := false
			for _, event := range events {
				if event["players_before"] == float64(17) && event["players_after"] == float64(18) {
					found = true
				}
			}
			if !found {
				t.Fatal("recovery evidence missing", events)
			}
		})
	}
}
