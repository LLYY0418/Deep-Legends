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

// 1018: nine gameflow identities plus one anonymous LiveClient entry. Filling
// the tenth slot must not freeze identity recovery after GSM's first 404.
func TestClassicHiddenPlaceholderRetriesGSM(t *testing.T) {
	const classicHiddenRef = "11111111-2222-3333-4444-555555555555"
	for _, placeholder := range []bool{false, true} {
		t.Run(map[bool]string{false: "nine-identities", true: "ten-with-placeholder"}[placeholder], func(t *testing.T) {
			f := newR183Fixture(t)
			gsm := gsmFixtureResponse()
			gsm.Game.TeamOne[2].ChampionID = 4
			gsm.Game.Selections[0].ChampionID = 4
			f.app.allSkins = []Skin{{ChampionID: 4, ChampionName: "卡牌大师"}, {ChampionID: 1, ChampionName: "安妮"}}
			f.snapshot.RosterPlayers[2].ChampionName = "卡牌大师"
			if placeholder {
				f.raw = append(f.raw, liveRosterEntry{player: lcuLivePlayer{NameVisibilityType: "HIDDEN", ChampionID: 4, SelectedPosition: "MIDDLE"}, team: 100})
			}
			old := f.client.http.Transport
			f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/lol-gameflow/v1/session" {
					var flow lcuGameflowSession
					flow.GameData.GameID, flow.GameData.Queue.ID, flow.GameData.Queue.GameMode = gsm.Game.ID, 420, "CLASSIC"
					for _, row := range f.raw {
						if row.team == 100 {
							flow.GameData.TeamOne = append(flow.GameData.TeamOne, row.player)
						} else {
							flow.GameData.TeamTwo = append(flow.GameData.TeamTwo, row.player)
						}
					}
					return r178JSON(flow, 200), nil
				}
				if r.URL.Path == "/lol-summoner/v2/summoners/puuid/"+classicHiddenRef {
					return r178JSON(Summoner{PUUID: classicHiddenRef, GameName: "CardFixture", TagLine: "CN1"}, 200), nil
				}
				return old.RoundTrip(r)
			})
			var list []map[string]any
			for _, p := range f.snapshot.RosterPlayers {
				list = append(list, map[string]any{"riotIdGameName": p.GameName, "riotIdTagLine": p.TagLine, "team": p.Team, "position": p.Position, "championName": p.ChampionName})
			}
			data, _ := json.Marshal(list)
			f.app.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return data, 200, nil }
			p := newSGPProvider()
			f.app.sgp = p
			p.sessionOwner, p.sessionToken, p.sessionAt = f.client, "fixture", time.Now()
			var calls atomic.Int64
			p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasPrefix(r.URL.Path, "/gsm/") && calls.Add(1) > 1 {
					return r178JSON(gsm, 200), nil
				}
				return r178JSON(map[string]any{}, 404), nil
			})}
			first := f.app.cachedGameplayLive(context.Background(), f.client, f.current, "InProgress")
			hidden := 0
			for _, player := range first.Players {
				if player.Hidden {
					hidden++
				}
			}
			if len(first.Players) != 10 || hidden != 1 || gameplayLiveSnapshotComplete(first) {
				t.Fatalf("placeholder froze the snapshot: count=%d hidden=%d complete=%v", len(first.Players), hidden, gameplayLiveSnapshotComplete(first))
			}
			f.app.liveSnapshots.mu.Lock()
			f.app.liveSnapshots.at = time.Now().Add(-4 * time.Second)
			f.app.liveSnapshots.mu.Unlock()
			p.gsm.mu.Lock()
			for k, e := range p.gsm.entries {
				e.At = time.Now().Add(-11 * time.Second)
				p.gsm.entries[k] = e
			}
			p.gsm.mu.Unlock()
			second := f.app.cachedGameplayLive(context.Background(), f.client, f.current, "InProgress")
			found := 0
			for _, player := range second.Players {
				if player.reference.PlayerRef == classicHiddenRef {
					found++
					if player.Hidden || player.IdentityUnresolved || player.GameName != "CardFixture" || player.ChampionID != 4 || player.PlayerRef == "" || player.HistoryState == "unavailable" {
						t.Fatalf("recovered card not enriched: %+v", player)
					}
				}
			}
			if len(second.Players) != 10 || found != 1 || calls.Load() != 2 {
				t.Fatalf("retry lost: players=%d found=%d calls=%d", len(second.Players), found, calls.Load())
			}
			if f.count("/lol-match-history/v1/products/lol/"+classicHiddenRef) == 0 {
				t.Fatal("recovered identity skipped history")
			}
		})
	}
}

func TestClassicHiddenProfileDoesNotFreezeCompleteRoster(t *testing.T) {
	response := gameplayLiveResponse{Available: true, GameID: 1454, QueueID: 420, Phase: "InProgress"}
	for i := 0; i < 10; i++ {
		team := int64(100)
		if i >= 5 {
			team = 200
		}
		response.Players = append(response.Players, gameplayLivePlayer{gameplayPlayer: gameplayPlayer{PlayerRef: "player_fixture", PrivateHistory: true}, TeamID: team, HistoryState: "unavailable"})
	}
	if !gameplayLiveSnapshotComplete(response) {
		t.Fatal("private history is not hidden identity")
	}
	response.Players[7].Hidden = true
	if gameplayLiveSnapshotComplete(response) {
		t.Fatal("queryable hidden profile froze retries")
	}
	response.Players[7].Hidden = false
	response.Players[7].IdentityUnresolved = true
	if gameplayLiveSnapshotComplete(response) {
		t.Fatal("unresolved profile froze retries")
	}
}
