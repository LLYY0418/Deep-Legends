package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
)

type r183Fixture struct {
	app        *app
	client     *LCUClient
	current    Summoner
	raw        []liveRosterEntry
	snapshot   liveClientSnapshot
	mu         sync.Mutex
	requests   map[string]int
	failedName string
}

func newR183Fixture(t *testing.T) *r183Fixture {
	t.Helper()
	f := &r183Fixture{app: r175App(t), current: Summoner{PUUID: r161Ref(6), GameName: "Player6", TagLine: "CN1"}, requests: make(map[string]int)}
	for i := 0; i < 10; i++ {
		team, liveTeam := int64(100), "ORDER"
		if i >= 5 {
			team, liveTeam = 200, "CHAOS"
		}
		position := []string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}[i%5]
		if i != 2 {
			f.raw = append(f.raw, liveRosterEntry{lcuLivePlayer{PUUID: r161Ref(i), GameName: fmt.Sprintf("Player%d", i), TagLine: "CN1", SelectedPosition: position, ChampionID: 1}, team})
		}
		p := liveClientRosterPlayer{GameName: fmt.Sprintf("Player%d", i), TagLine: "CN1", Team: liveTeam, Position: position, ChampionName: "安妮"}
		if i == 2 {
			p.GameName, p.TagLine = "", ""
		}
		f.snapshot.RosterPlayers = append(f.snapshot.RosterPlayers, p)
	}
	f.client = &LCUClient{baseURL: "http://lcu.local", token: "test", region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	f.client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		f.mu.Lock()
		f.requests[r.URL.Path]++
		failedName := f.failedName
		f.mu.Unlock()
		var value any = map[string]any{}
		status := 200
		switch {
		case r.URL.Path == "/lol-summoner/v1/alias/lookup":
			var i int
			fmt.Sscanf(r.URL.Query().Get("gameName"), "Player%d", &i)
			if r.URL.Query().Get("gameName") == failedName {
				status = 404
			} else {
				value = []any{map[string]any{"puuid": r161Ref(i)}}
			}
		case strings.HasPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
			ref := strings.TrimPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/")
			value = map[string]any{"puuid": ref, "gameName": fmt.Sprintf("Player%d", int(ref[0]-'a')), "tagLine": "CN1"}
		case strings.Contains(r.URL.Path, "/lol-match-history/"):
			value = map[string]any{"games": map[string]any{"games": []any{}}}
		case strings.HasPrefix(r.URL.Path, "/lol-ranked/"):
			value = map[string]any{"queues": []any{}}
		default:
			status = 404
		}
		data, _ := json.Marshal(value)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}
	return f
}
func (f *r183Fixture) recover(queue int64, gameID int64) liveRosterRecoveryResult {
	return f.app.cachedClassicLiveRoster(context.Background(), f.client, f.current, f.raw, f.snapshot, map[int64]string{1: "安妮"}, liveAnonymousRosterQueue(queue), liveRosterRecoveryScope{gameID, queue, "InProgress", len(f.raw)})
}
func (f *r183Fixture) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for path, count := range f.requests {
		if strings.HasPrefix(path, prefix) {
			n += count
		}
	}
	return n
}
func TestR183RankedAnonymousEnemy(t *testing.T) {
	for _, queue := range []int64{440, 420} {
		t.Run(fmt.Sprint(queue), func(t *testing.T) {
			f := newR183Fixture(t)
			got := f.recover(queue, 183)
			if got.Reason != "anonymous-placeholder" || got.Appended != 1 || len(got.Players) != 10 || teamCountsInRaw(got.Players, 100) != 5 || teamCountsInRaw(got.Players, 200) != 5 {
				t.Fatal(got)
			}
			p := got.Players[9]
			if p.team != 100 || p.player.SelectedPosition != "MIDDLE" || p.player.ChampionID != 1 || p.player.NameVisibilityType != "HIDDEN" || p.player.PUUID != "" || p.player.GameName != "" {
				t.Fatal(p)
			}
			if got.Stats != (liveRosterRecoveryStats{AnonymousCount: 1, NamedPresent: 4}) || got.Attempts != 4 {
				t.Fatal(got.Stats, got.Attempts)
			}
			events := r175Events(t, f.app, "live_roster_recovery")
			if len(events) != 1 || events[0]["anonymous_count"] != float64(1) || events[0]["named_present"] != float64(4) || events[0]["named_appended"] != float64(0) || events[0]["unresolved_named"] != float64(0) || events[0]["cached"] != false {
				t.Fatal(events)
			}
			encoded, _ := json.Marshal(events)
			for _, secret := range []string{"Player6", "CN1", r161Ref(6), "安妮", "MIDDLE"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatal("diagnostic leaked", secret)
				}
			}
		})
	}
}
func TestR183AnonymousSafetyGates(t *testing.T) {
	cases := []struct {
		name, reason string
		change       func(*r183Fixture)
	}{
		{"too-many-anonymous", "partial", func(f *r183Fixture) {
			f.snapshot.RosterPlayers[3].GameName = ""
			f.snapshot.RosterPlayers[3].TagLine = ""
		}},
		{"named-unresolved", "partial", func(f *r183Fixture) { f.failedName = "Player4" }},
		{"teams-unverified", "teams-unverified", func(f *r183Fixture) { f.snapshot.RosterPlayers[0].Team = "CHAOS" }},
		{"self-unverified", "self-unverified", func(f *r183Fixture) { f.current.PUUID = r161Ref(15) }},
		{"self-live-unverified", "teams-unverified", func(f *r183Fixture) { f.snapshot.RosterPlayers[6].GameName = "Other" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newR183Fixture(t)
			tc.change(f)
			got := f.recover(440, 183)
			if got.Reason != tc.reason || got.Appended != 0 || len(got.Players) != 9 {
				t.Fatal(got)
			}
			if tc.name == "named-unresolved" && got.Stats.UnresolvedNamed != 1 {
				t.Fatal(got.Stats)
			}
		})
	}
}
func TestR183RecoveryCacheLifecycle(t *testing.T) {
	f := newR183Fixture(t)
	for i := 0; i < 10; i++ {
		got := f.recover(440, 183)
		if got.Stats.Cached != (i > 0) || len(got.Players) != 10 || got.Appended != 1 {
			t.Fatal(i, got)
		}
		got.Players[9].player.ChampionID = 999 // Callers cannot mutate the cache.
	}
	if f.count("/lol-summoner/v1/alias/lookup") != 4 || len(r175Events(t, f.app, "live_roster_recovery")) != 1 {
		t.Fatal(f.requests, r175Events(t, f.app, "live_roster_recovery"))
	}
	// Fingerprints are sets: harmless reordering must not repeat queries.
	f.raw[0], f.raw[1] = f.raw[1], f.raw[0]
	f.snapshot.RosterPlayers[0], f.snapshot.RosterPlayers[1] = f.snapshot.RosterPlayers[1], f.snapshot.RosterPlayers[0]
	if got := f.recover(440, 183); !got.Stats.Cached || got.Players[9].player.ChampionID != 1 {
		t.Fatal(got)
	}
	// A changed gameflow player set recomputes, even if its count stays at nine.
	f.raw[0].player.PUUID = r161Ref(14)
	if got := f.recover(440, 183); got.Stats.Cached || f.count("/lol-summoner/v1/alias/lookup") != 8 {
		t.Fatal(got, f.requests)
	}
	f.recover(440, 184)
	if f.count("/lol-summoner/v1/alias/lookup") != 12 || f.app.liveRosterRecovery.GameID != 184 || len(f.app.liveRosterRecovery.Entries) != 1 {
		t.Fatal(f.requests)
	}
	// An old game cannot hit after a new game has replaced its cache.
	f.recover(440, 183)
	if f.count("/lol-summoner/v1/alias/lookup") != 16 {
		t.Fatal(f.requests)
	}
	// A Live Client identity change also invalidates this input.
	f.raw[0].player.PUUID = r161Ref(1)
	f.snapshot.RosterPlayers[2].GameName = "Player2"
	f.snapshot.RosterPlayers[2].TagLine = "CN1"
	if got := f.recover(440, 183); got.Stats.Cached || got.Stats.NamedAppended != 1 || got.Stats.AnonymousCount != 0 || f.count("/lol-summoner/v1/alias/lookup") != 21 {
		t.Fatal(got, f.requests)
	}
	f.app.clearLiveRosterRecovery()
	if len(f.app.liveRosterRecovery.Entries) != 0 || f.app.liveRosterRecovery.GameID != 0 {
		t.Fatal("cache not cleared")
	}
}
func TestR183PartialResultCached(t *testing.T) {
	f := newR183Fixture(t)
	f.failedName = "Player4"
	for i := 0; i < 10; i++ {
		got := f.recover(440, 183)
		if got.Reason != "partial" || got.Stats.Cached != (i > 0) || got.Stats.UnresolvedNamed != 1 {
			t.Fatal(got)
		}
	}
	if f.count("/lol-summoner/v1/alias/lookup") != 4 || len(r175Events(t, f.app, "live_roster_recovery")) != 1 {
		t.Fatal(f.requests)
	}
}

func TestR183RankedAnonymousEnemyEndToEnd(t *testing.T) {
	f := newR183Fixture(t)
	f.app.allSkins = []Skin{{ChampionID: 1, ChampionName: "安妮"}}
	baseTransport := f.client.http.Transport
	gameID := int64(183)
	full := false
	f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/lol-gameflow/v1/session" {
			return baseTransport.RoundTrip(r)
		}
		teamOne, teamTwo := []lcuLivePlayer{}, []lcuLivePlayer{}
		for _, entry := range f.raw {
			if entry.team == 100 {
				teamOne = append(teamOne, entry.player)
			} else {
				teamTwo = append(teamTwo, entry.player)
			}
		}
		if full {
			teamOne = append(teamOne, lcuLivePlayer{NameVisibilityType: "HIDDEN", SelectedPosition: "MIDDLE", ChampionID: 1})
		}
		value := map[string]any{"gameData": map[string]any{"gameId": gameID, "queue": map[string]any{"id": 440, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": teamOne, "teamTwo": teamTwo}}
		data, _ := json.Marshal(value)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})
	var live []any
	for _, p := range f.snapshot.RosterPlayers {
		riotID := ""
		if p.GameName != "" {
			riotID = p.GameName + "#" + p.TagLine
		}
		live = append(live, map[string]any{"riotId": riotID, "team": p.Team, "position": p.Position, "championName": p.ChampionName})
	}
	data, _ := json.Marshal(live)
	f.app.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return data, 200, nil }
	response := f.app.loadGameplayLive(context.Background(), f.client, f.current, "InProgress")
	if len(response.Players) != 10 || !response.Available {
		t.Fatal(response)
	}
	var hidden gameplayLivePlayer
	for _, p := range response.Players {
		if p.Hidden {
			hidden = p
		}
	}
	if hidden.Position != "middle" || hidden.ChampionID != 1 || hidden.PlayerRef != "" || hidden.reference.PlayerRef != "" || hidden.Rank != nil || hidden.HistoryState != "unavailable" || hidden.PremadeGroup != "" || hidden.PremadeSize != 0 || hidden.MySquad || hidden.IsCurrent || hidden.TeamID != 100 {
		t.Fatal(hidden)
	}
	if f.count("/lol-summoner/v2/summoners/puuid/") != 9 || f.count("/lol-match-history/") != 9 || f.count("/lol-ranked/") != 9 || f.count("/lol-summoner/v1/alias/lookup") != 4 {
		t.Fatal(f.requests)
	}
	for i := 1; i < 10; i++ {
		if got := f.app.loadGameplayLive(context.Background(), f.client, f.current, "InProgress"); len(got.Players) != 10 {
			t.Fatal(got)
		}
	}
	if f.count("/lol-summoner/v1/alias/lookup") != 4 || len(r175Events(t, f.app, "live_roster_recovery")) != 1 {
		t.Fatal(f.requests)
	}
	if len(r175Events(t, f.app, "backend_panic")) != 0 {
		t.Fatal("pipeline panicked")
	}
	encoded, _ := json.MarshalIndent(response, "", "  ")
	if os.Getenv("R183_WRITE_FIXTURE") == "1" {
		if err := os.WriteFile("testdata/r183-ranked-anonymous-live.json", append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := os.ReadFile("testdata/r183-ranked-anonymous-live.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture gameplayLiveResponse
	if err = json.Unmarshal(saved, &fixture); err != nil || len(fixture.Players) != 10 || fixture.QueueID != 440 || fixture.Players[9].Position != hidden.Position || !fixture.Players[9].Hidden || fixture.Players[9].ChampionID != hidden.ChampionID || fixture.Players[9].PlayerRef != "" {
		t.Fatal("fixture diverged", err)
	}
	// Gameflow itself may recover the hidden row; it must not append a second one.
	full = true
	if got := f.app.loadGameplayLive(context.Background(), f.client, f.current, "InProgress"); len(got.Players) != 10 || f.count("/lol-summoner/v1/alias/lookup") != 4 {
		t.Fatal(got)
	}
	// A new complete game still clears the old recovery cache, despite skipping recovery.
	gameID++
	f.app.loadGameplayLive(context.Background(), f.client, f.current, "InProgress")
	if f.app.liveRosterRecovery.GameID != 184 || len(f.app.liveRosterRecovery.Entries) != 0 {
		t.Fatal("full new game retained recovery entries")
	}
}

func TestR183RecoveryConcurrentAndStaleFlight(t *testing.T) {
	for _, newGame := range []bool{false, true} {
		t.Run(fmt.Sprint(newGame), func(t *testing.T) {
			f := newR183Fixture(t)
			base := f.client.http.Transport
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/lol-summoner/v1/alias/lookup" {
					block := false
					once.Do(func() { block = true; close(entered) })
					if block {
						<-release
					}
				}
				return base.RoundTrip(r)
			})
			results := make(chan liveRosterRecoveryResult, 2)
			go func() { results <- f.recover(440, 183) }()
			<-entered
			if newGame {
				latest := f.recover(440, 184)
				if latest.Stats.Cached {
					t.Fatal("new game reused old flight")
				}
				close(release)
				<-results
				if !f.recover(440, 184).Stats.Cached || f.count("/lol-summoner/v1/alias/lookup") != 8 || f.app.liveRosterRecovery.GameID != 184 {
					t.Fatal(f.requests)
				}
				rows := r175Events(t, f.app, "live_roster_recovery")
				if len(rows) != 1 || rows[0]["game_id"] != float64(184) {
					t.Fatal(rows)
				}
			} else {
				go func() { results <- f.recover(440, 183) }()
				close(release)
				first, second := <-results, <-results
				if first.Stats.Cached == second.Stats.Cached || f.count("/lol-summoner/v1/alias/lookup") != 4 || len(first.Players) != 10 || len(second.Players) != 10 {
					t.Fatal(first, second, f.requests)
				}
			}
		})
	}
}
