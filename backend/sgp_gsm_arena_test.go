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

func arenaGSMFixture(queue int64) sgpGSMResponse {
	var r sgpGSMResponse
	r.Game.ID, r.Game.QueueID, r.Game.GameMode, r.Game.MapID = 9034935539, queue, "CHERRY", 30
	for i := 0; i < liveGSMRosterSize(queue); i++ {
		// Duplicate champions are legal; only identities distinguish these players.
		p := lcuLivePlayer{PUUID: r161Ref(i), ChampionID: 1, SelectedPosition: "NONE", Spell1ID: 4, Spell2ID: 14}
		if i < 14 {
			r.Game.TeamOne = append(r.Game.TeamOne, p)
		} else {
			r.Game.TeamTwo = append(r.Game.TeamTwo, p)
		}
	}
	return r
}

func TestGSMArenaValidationAndIdentityMerge(t *testing.T) {
	for _, queue := range []int64{1700, 1710, 1750} {
		r := arenaGSMFixture(queue)
		verified := validateGSMRoster(r, r161Ref(0), r.Game.ID, queue)
		if verified.State != capabilityAvailable || len(verified.Players) != liveGSMRosterSize(queue) {
			t.Fatal(queue, verified)
		}
		raw := append([]liveRosterEntry(nil), verified.Players[:14]...)
		got, appended, restored, reason := mergeGSMArenaRoster(raw, verified.Players, liveGSMRosterSize(queue))
		if len(got) != liveGSMRosterSize(queue) || appended != len(got)-14 || restored != appended || reason != "merged" {
			t.Fatal(queue, len(got), appended, restored, reason)
		}
		for _, p := range got {
			if p.team != 100 {
				t.Fatal("GSM sides became Arena squads")
			}
		}
		overfilled := arenaGSMFixture(queue)
		overfilled.Game.TeamTwo = append(overfilled.Game.TeamTwo, lcuLivePlayer{PUUID: r161Ref(25)})
		if result := validateGSMRoster(overfilled, r161Ref(0), overfilled.Game.ID, queue); result.Reason != "roster-invalid" {
			t.Fatal("Arena queue capacity not enforced", queue, result)
		}
		// Full GSM roster replaces anonymous slots without guessing among duplicates.
		raw = append(raw, liveRosterEntry{player: lcuLivePlayer{ChampionID: 1, NameVisibilityType: "HIDDEN", Spell1ID: 99}, team: 100})
		got, _, _, reason = mergeGSMArenaRoster(raw, verified.Players, liveGSMRosterSize(queue))
		if len(got) != liveGSMRosterSize(queue) || reason != "merged" {
			t.Fatal(reason, len(got))
		}
		for _, p := range got[14:] {
			if p.player.Spell1ID != 4 {
				t.Fatal("anonymous metadata assigned by champion")
			}
		}
		_, _, _, reason = mergeGSMArenaRoster(raw, verified.Players[:15], liveGSMRosterSize(queue))
		if reason != "anonymous-roster-incomplete" {
			t.Fatal(reason)
		}
		r.Game.TeamOne[0].ChampionID = 2
		conflict := validateGSMRoster(r, r161Ref(0), r.Game.ID, queue)
		_, _, _, reason = mergeGSMArenaRoster(raw, conflict.Players, liveGSMRosterSize(queue))
		if reason != "roster-conflict" {
			t.Fatal(reason)
		}
	}
	for _, tc := range []struct {
		name, reason string
		change       func(*sgpGSMResponse)
	}{
		{"wrong-game", "game-mismatch", func(r *sgpGSMResponse) { r.Game.ID++ }},
		{"wrong-mode", "mode-mismatch", func(r *sgpGSMResponse) { r.Game.GameMode = "CLASSIC" }},
		{"overfilled", "roster-invalid", func(r *sgpGSMResponse) { r.Game.TeamTwo = append(r.Game.TeamTwo, lcuLivePlayer{PUUID: r161Ref(20)}) }},
		{"duplicate", "identity-invalid", func(r *sgpGSMResponse) { r.Game.TeamTwo[0].PUUID = r.Game.TeamOne[0].PUUID }},
		{"missing-self", "current-player-missing", func(r *sgpGSMResponse) { r.Game.TeamOne[0].PUUID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := arenaGSMFixture(1750)
			tc.change(&r)
			if got := validateGSMRoster(r, r161Ref(0), 9034935539, 1750); got.Reason != tc.reason {
				t.Fatal(got)
			}
		})
	}
}

func TestGSMArenaFourMissingPlayersEndToEnd(t *testing.T) {
	for _, status := range []int{200, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := newR183Fixture(t)
			f.current = Summoner{PUUID: r161Ref(0), GameName: "Player0", TagLine: "CN1"}
			gsm := arenaGSMFixture(1750)
			old := f.client.http.Transport
			f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/lol-gameflow/v1/session" {
					var flow lcuGameflowSession
					flow.GameData.GameID, flow.GameData.Queue.ID, flow.GameData.Queue.GameMode = gsm.Game.ID, 1750, "CHERRY"
					flow.GameData.TeamOne = gsm.Game.TeamOne
					// Stale five-player TeamTwo must still be discarded, independently of GSM.
					for i := 20; i < 25; i++ {
						flow.GameData.TeamTwo = append(flow.GameData.TeamTwo, lcuLivePlayer{PUUID: r161Ref(i), ChampionID: 99})
					}
					return r178JSON(flow, 200), nil
				}
				if strings.Contains(r.URL.Path, "/lol-match-history/") {
					ref := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/lol-match-history/v1/products/lol/"), "/matches")
					if ref == "current-summoner" {
						ref = f.current.PUUID
					}
					var h lcuMatchHistory
					g := lcuGame{GameID: 99001, QueueID: 1750, GameMode: "CHERRY", GameDuration: 1200, GameCreation: 1786300000000}
					id := lcuParticipantIdentity{ParticipantID: 1}
					id.Player.PUUID = ref
					g.ParticipantIdentities = []lcuParticipantIdentity{id}
					g.Participants = []lcuParticipant{{ParticipantID: 1, TeamID: 100, ChampionID: 1}}
					g.Participants[0].Stats.SubteamPlacement = 1
					g.Participants[0].Stats.Kills = 3
					h.Games.Games = []lcuGame{g}
					h.Games.GameCount = 1
					return r178JSON(h, 200), nil
				}
				return old.RoundTrip(r)
			})
			var live []map[string]any
			for i := 0; i < 18; i++ {
				p := map[string]any{"championName": "安妮", "team": "ORDER", "position": "OTHER"}
				if i < 14 {
					p["riotId"] = fmt.Sprintf("Player%d#CN1", i)
				}
				live = append(live, p)
			}
			data, _ := json.Marshal(live)
			f.app.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return data, 200, nil }
			p := newSGPProvider()
			f.app.sgp = p
			p.sessionOwner, p.sessionToken, p.sessionAt = f.client, "test", time.Now()
			var calls atomic.Int64
			p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasPrefix(r.URL.Path, "/gsm/") {
					calls.Add(1)
					return r178JSON(gsm, status), nil
				}
				return r178JSON(map[string]any{}, 404), nil
			})}
			for attempt := 0; attempt < 2; attempt++ {
				response := f.app.loadGameplayLive(context.Background(), f.client, f.current, "InProgress")
				want := 18
				if status != 200 {
					want = 14
				}
				if len(response.Players) != want || response.RawCount != 14 {
					t.Fatalf("players=%d raw=%d want=%d", len(response.Players), response.RawCount, want)
				}
				if status == 200 {
					for _, player := range response.Players {
						if player.Hidden || player.IdentityUnresolved || player.GameName == "" || player.TeamID != 100 || len(player.RecentGames) != 1 || player.HistoryState != "ok" {
							t.Fatalf("missing recovered profile/history: %+v", player)
						}
					}
				} else if gameplayLiveSnapshotComplete(response) {
					t.Fatal("14/18 players stopped identity retries")
				}
				encoded, _ := json.Marshal(response)
				for i := 0; i < 18; i++ {
					if strings.Contains(string(encoded), r161Ref(i)) {
						t.Fatal("stable identity exposed")
					}
				}
			}
			if calls.Load() != 1 {
				t.Fatal("GSM request cache not reused", calls.Load())
			}
		})
	}
}

func TestGSMArenaPartialCacheAndSnapshotCompleteness(t *testing.T) {
	p := newSGPProvider()
	client := &LCUClient{}
	p.sessionOwner, p.sessionToken, p.sessionAt = client, "test", time.Now()
	r := arenaGSMFixture(1750)
	r.Game.TeamTwo[0].PUUID = ""
	r.Game.Selections = gsmFixtureResponse().Game.Selections
	r.Game.Selections[0].PUUID = ""
	var calls atomic.Int64
	p.http = &http.Client{Transport: sgpRoundTripFunc(func(*http.Request) (*http.Response, error) { calls.Add(1); return r178JSON(r, 200), nil })}
	got := p.liveGameRoster(context.Background(), client, "HN1", r161Ref(0), r.Game.ID, 1750)
	if got.State != capabilityAvailable || len(got.Players) != 17 {
		t.Fatal(got)
	}
	p.liveGameRoster(context.Background(), client, "HN1", r161Ref(0), r.Game.ID, 1750)
	if calls.Load() != 1 {
		t.Fatal("partial roster was not cached")
	}
	for key, entry := range p.gsm.entries {
		if entry.TTL != 3*time.Second {
			t.Fatal("partial Arena roster cached as complete", entry.TTL)
		}
		entry.At = time.Now().Add(-4 * time.Second)
		p.gsm.entries[key] = entry
	}
	p.liveGameRoster(context.Background(), client, "HN1", r161Ref(0), r.Game.ID, 1750)
	if calls.Load() != 2 {
		t.Fatal("partial roster was not refreshed after expiry")
	}
	response := gameplayLiveResponse{Available: true, GameID: r.Game.ID, QueueID: 1750, GameMode: "CHERRY", Phase: "InProgress", ArenaGroupingUnavailable: true}
	response.Players = make([]gameplayLivePlayer, 14)
	if gameplayLiveSnapshotComplete(response) {
		t.Fatal("partial roster is complete")
	}
	response.Players = make([]gameplayLivePlayer, 18)
	if !gameplayLiveSnapshotComplete(response) {
		t.Fatal("full roster without inferred squads should settle")
	}
	response.ArenaGroupingRetryable = true
	if gameplayLiveSnapshotComplete(response) {
		t.Fatal("pending explicit grouping must retry")
	}
	response.ArenaGroupingRetryable = false
	response.Players[17].IdentityUnresolved = true
	if gameplayLiveSnapshotComplete(response) {
		t.Fatal("anonymous roster is complete")
	}
}
