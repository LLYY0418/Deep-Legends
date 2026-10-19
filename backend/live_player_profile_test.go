package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLivePlayerProfileResolvesWithoutPreviousEncounter(t *testing.T) {
	for _, scenario := range []string{"lcu-failed-sgp-name", "lcu-empty-sgp-name", "lcu-display-only", "both-unavailable", "wrong-sgp-identity"} {
		t.Run(scenario, func(t *testing.T) {
			f := newR183Fixture(t)
			// Overview starts season cache work; drain it before the fixture store/TempDir close.
			waitGameplaySeasonJobsBeforeCleanup(t, f.app)
			const realName = "首次遇见的玩家"
			var flow lcuGameflowSession
			flow.GameData.GameID, flow.GameData.Queue.ID, flow.GameData.Queue.GameMode = 100018, 1750, "CHERRY"
			for i := 0; i < 18; i++ {
				player := lcuLivePlayer{PUUID: r161Ref(i), ChampionID: 1}
				if i == 2 {
					player.ChampionID = 79
					player.NameVisibilityType = "HIDDEN"
					player.GameName = "酒桶"
					player.SummonerName = "酒桶"
					player.TagLine = "ALIAS"
				}
				flow.GameData.TeamOne = append(flow.GameData.TeamOne, player)
			}
			old := f.client.http.Transport
			f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/lol-gameflow/v1/session" {
					return r178JSON(flow, 200), nil
				}
				if r.URL.Path == "/lol-summoner/v2/summoners/puuid/"+r161Ref(2) {
					if scenario == "lcu-display-only" {
						return r178JSON(Summoner{PUUID: r161Ref(2), DisplayName: realName, Privacy: "PRIVATE"}, 200), nil
					}
					if scenario == "lcu-empty-sgp-name" {
						return r178JSON(Summoner{PUUID: r161Ref(2)}, 200), nil
					}
					return r178JSON(map[string]any{}, 404), nil
				}
				return old.RoundTrip(r)
			})
			f.app.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return []byte(`[]`), 200, nil }
			p := newSGPProvider()
			f.app.sgp = p
			p.sessionOwner, p.sessionToken, p.sessionAt = f.client, "fixture", time.Now()
			var calls atomic.Int64
			p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasPrefix(r.URL.Path, "/summoner-ledge/") {
					calls.Add(1)
					var refs []string
					if err := json.NewDecoder(r.Body).Decode(&refs); err != nil || len(refs) != 1 || refs[0] != r161Ref(2) {
						t.Errorf("wrong scoped identity: %v", refs)
					}
					if !strings.Contains(r.URL.Path, "/regions/hn1/") {
						t.Errorf("wrong region: %s", r.URL.Path)
					}
					if scenario == "both-unavailable" {
						return r178JSON(map[string]any{}, 404), nil
					}
					ref := r161Ref(2)
					if scenario == "wrong-sgp-identity" {
						ref = r161Ref(3)
					}
					return r178JSON([]sgpSummoner{{PUUID: ref, Name: realName, Level: 321, Privacy: "PRIVATE"}}, 200), nil
				}
				return r178JSON(map[string]any{}, 404), nil
			})}
			// Neither opening an overview nor a previous game/ChampSelect supplies identity.
			if len(f.app.arenaAllyPlayers) != 0 {
				t.Fatal("fixture has remembered identities")
			}
			response := f.app.loadGameplayLive(context.Background(), f.client, f.current, "InProgress")
			if len(response.Players) != 18 {
				t.Fatal(len(response.Players))
			}
			var found *gameplayLivePlayer
			for i := range response.Players {
				if response.Players[i].reference.PlayerRef == r161Ref(2) {
					found = &response.Players[i]
				}
			}
			if found == nil {
				t.Fatal("clickable reference lost")
			}
			if scenario == "both-unavailable" || scenario == "wrong-sgp-identity" {
				if !found.Hidden {
					t.Fatal("unverified profile exposed")
				}
				return
			}
			if found.Hidden || found.IdentityUnresolved || found.DisplayName != realName || found.GameName == "酒桶" || found.TagLine != "" || !found.PrivateHistory {
				t.Fatalf("live card still masks a resolved profile: %+v", found)
			}
			ref, ok := f.app.resolveGameplayReferenceDetails(found.PlayerRef)
			if !ok || ref.PlayerRef != r161Ref(2) || ref.GameName == "酒桶" || ref.TagLine != "" {
				t.Fatalf("click retained source alias: %+v", ref)
			}
			if scenario == "lcu-display-only" {
				if calls.Load() != 0 {
					t.Fatal("unnecessary SGP lookup")
				}
				return
			}
			// The exact public reference emitted by the card opens the same profile in overview.
			overview := f.app.loadGameplayOverview(context.Background(), f.client, f.current, ref, 0, 1, "all", false)
			if overview.Player.Hidden || overview.Player.DisplayName != realName || overview.Player.DisplayName != found.DisplayName {
				t.Fatalf("overview/live disagree: %+v / %+v", overview.Player, found)
			}
			if calls.Load() != 1 {
				t.Fatalf("overview did not reuse scoped SGP profile: calls=%d", calls.Load())
			}
		})
	}
}

func TestLivePlayerProfileCanceledDoesNotQueryFallback(t *testing.T) {
	f := newR183Fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	loaded, cap := f.app.loadLiveGameplaySummoner(ctx, f.client, gameplayReference{PlayerRef: r161Ref(2), ServerID: "HN1"})
	if gameplaySummonerHasName(loaded) || cap.State == capabilityAvailable || f.count("/lol-summoner/") != 0 {
		t.Fatal("cancelled request queried profile")
	}
}
