package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// Reproduce the two 2322 log shapes: 17 gameflow / 18 playerlist, and a
// remembered hidden squad member omitted from gameflow. Identities are synthetic.
func TestArenaIdentityRecoveryEndToEnd(t *testing.T) {
	for _, scenario := range []string{"unicode-name", "decoded-squad", "anonymous-squad", "full-anonymous-roster", "alias-unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			f := newR183Fixture(t)
			f.current = Summoner{PUUID: r161Ref(0), GameName: "Player0", TagLine: "CN1"}
			const gameID int64 = 9035680301
			const missingRef = "00112233-4455-6677-8899-aabbccddeeff"
			missingName, missingTag := "T1ˇFixture", "18602"
			var flow lcuGameflowSession
			flow.GameData.GameID, flow.GameData.Queue.ID, flow.GameData.Queue.GameMode = gameID, 1750, "CHERRY"
			var live []map[string]any
			for i := 0; i < 18; i++ {
				champion, name, tag, title := int64(1), fmt.Sprintf("Player%d", i), "CN1", "黑暗之女"
				if i == 2 {
					champion, name, tag, title = 777, missingName, missingTag, "封魔剑魂"
				}
				if i != 2 {
					flow.GameData.TeamOne = append(flow.GameData.TeamOne, lcuLivePlayer{PUUID: r161Ref(i), ChampionID: champion})
				}
				live = append(live, map[string]any{"riotIdGameName": name, "riotIdTagLine": tag, "riotId": name + "#" + tag, "championName": title, "team": "ORDER", "position": "OTHER"})
			}
			if scenario == "decoded-squad" || scenario == "anonymous-squad" {
				ally := lcuLivePlayer{ChampionID: 777, NameVisibilityType: "HIDDEN"}
				if scenario == "decoded-squad" {
					ally.ObfuscatedPUUID = deobfuscateHiddenPlayerReference(missingRef)
					live[2]["riotIdGameName"], live[2]["riotIdTagLine"], live[2]["riotId"] = "", "", ""
				}
				f.app.arenaAllyPlayers = []lcuLivePlayer{{PUUID: r161Ref(0), ChampionID: 1}, ally}
				f.app.arenaAllyGameID = gameID
			}
			if scenario == "full-anonymous-roster" {
				flow.GameData.TeamOne = append(flow.GameData.TeamOne, lcuLivePlayer{ChampionID: 777, NameVisibilityType: "HIDDEN"})
			}
			old := f.client.http.Transport
			var history, aliasCalls atomic.Int64
			f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.URL.Path == "/lol-gameflow/v1/session":
					return r178JSON(flow, 200), nil
				case r.URL.Path == "/lol-summoner/v1/alias/lookup":
					aliasCalls.Add(1)
					if r.URL.Query().Get("gameName") != missingName || r.URL.Query().Get("tagLine") != missingTag {
						t.Errorf("Riot ID altered or known player looked up: %s", r.URL.RawQuery)
					}
					if scenario == "alias-unavailable" {
						return r178JSON(map[string]any{}, 404), nil
					}
					return r178JSON([]any{map[string]any{"puuid": missingRef}}, 200), nil
				case r.URL.Path == "/lol-summoner/v2/summoners/puuid/"+missingRef:
					return r178JSON(Summoner{PUUID: missingRef, GameName: missingName, TagLine: missingTag}, 200), nil
				case strings.Contains(r.URL.Path, "/lol-match-history/") && strings.Contains(r.URL.Path, missingRef):
					history.Add(1)
				}
				return old.RoundTrip(r)
			})
			data, _ := json.Marshal(live)
			f.app.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return data, 200, nil }
			response := f.app.loadGameplayLive(context.Background(), f.client, f.current, "InProgress")
			if len(response.Players) != 18 {
				t.Fatalf("players=%d, want 18", len(response.Players))
			}
			matches := 0
			for _, p := range response.Players {
				if p.GameName != missingName {
					continue
				}
				matches++
				if p.TagLine != missingTag || p.ChampionID != 777 || p.Hidden || p.IdentityUnresolved {
					t.Fatalf("lost name or champion: %+v", p)
				}
				if scenario == "alias-unavailable" {
					if p.PlayerRef != "" || gameplayLiveSnapshotComplete(response) {
						t.Fatal("name-only recovery stopped identity retries")
					}
				} else if p.reference.PlayerRef != missingRef || p.HistoryState == "unavailable" || history.Load() == 0 {
					t.Fatalf("restored identity skipped enrichment: %+v, history calls=%d", p, history.Load())
				}
			}
			if matches != 1 {
				t.Fatalf("recovered name count=%d", matches)
			}
			if scenario == "decoded-squad" && aliasCalls.Load() != 0 {
				t.Fatal("decoded identity unnecessarily looked up by alias")
			}
			encoded, _ := json.Marshal(response)
			if strings.Contains(string(encoded), missingRef) {
				t.Fatal("stable identity exposed")
			}
		})
	}
}

func TestArenaIdentityRecoveryScopeAndConflicts(t *testing.T) {
	f := newR183Fixture(t)
	response := gameplayLiveResponse{GameID: 10, QueueID: 1750, GameMode: "CHERRY", Phase: "InProgress"}
	f.app.arenaAllyGameID = 9
	f.app.arenaAllyPlayers = []lcuLivePlayer{{PUUID: r161Ref(2), ChampionID: 777}}
	if got := f.app.restoreArenaIdentityRoster(&response, nil); len(got) != 0 {
		t.Fatal("cross-game squad restored")
	}
	f.app.arenaAllyGameID = 10
	raw := []liveRosterEntry{{player: lcuLivePlayer{PUUID: r161Ref(0), GameName: "Player0", TagLine: "CN1", ChampionID: 777}, team: 100}}
	got := f.app.restoreArenaIdentityRoster(&response, raw)
	if len(got) != 2 || got[1].player.PUUID != r161Ref(2) {
		t.Fatal("duplicate champion swallowed a distinct identity")
	}
	snapshot := liveClientSnapshot{RosterPlayers: []liveClientRosterPlayer{{GameName: "Player0", TagLine: "CN1"}, {GameName: "Player0", TagLine: "CN1"}}}
	if got := f.app.recoverArenaPlayerList(context.Background(), f.client, &response, raw, snapshot, nil); len(got) != 1 {
		t.Fatal("duplicate Riot ID appended")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if got := f.app.recoverArenaPlayerList(cancelled, f.client, &response, raw, snapshot, nil); len(got) != 1 {
		t.Fatal("cancelled recovery changed roster")
	}
}

func TestArenaIdentityRecoveryRejectsDuplicateAliasAndKeepsPartialProgress(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		f := newR183Fixture(t)
		response := gameplayLiveResponse{GameID: 10, QueueID: 1750, GameMode: "CHERRY", Phase: "InProgress"}
		raw := []liveRosterEntry{{player: lcuLivePlayer{GameName: "Player1", TagLine: "CN1"}, team: 100}, {player: lcuLivePlayer{GameName: "Player2", TagLine: "CN1"}, team: 100}}
		snapshot := liveClientSnapshot{RosterPlayers: []liveClientRosterPlayer{{GameName: "Player1", TagLine: "CN1"}, {GameName: "Player2", TagLine: "CN1"}}}
		old := f.client.http.Transport
		f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/lol-summoner/v1/alias/lookup" && r.URL.Query().Get("gameName") == "Player2" {
				if duplicate {
					return r178JSON([]any{map[string]any{"puuid": r161Ref(1)}}, 200), nil
				}
				return r178JSON(map[string]any{}, 404), nil
			}
			return old.RoundTrip(r)
		})
		got := f.app.recoverArenaPlayerList(context.Background(), f.client, &response, raw, snapshot, nil)
		if len(got) != 2 {
			t.Fatal("changed count", len(got))
		}
		if duplicate {
			if got[0].player.PUUID != "" || got[1].player.PUUID != "" {
				t.Fatal("conflicting aliases accepted")
			}
		} else if got[0].player.PUUID != r161Ref(1) || got[1].player.PUUID != "" {
			t.Fatal("one failed lookup discarded another player's resolved identity")
		}
	}
}
