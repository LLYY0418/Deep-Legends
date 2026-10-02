package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Real loadGameplayLive pipeline, synthetic accounts; our TOP is missing from
// gameflow and has no Riot ID in Live Client. Requests retain R183 counters.
type r194Fixture struct {
	*r183Fixture
	queue  int64
	gameID int64
}

func newR194Fixture(t *testing.T, queue int64, bot bool) *r194Fixture {
	t.Helper()
	f := &r194Fixture{r183Fixture: newR183Fixture(t), queue: queue, gameID: 194}
	f.current = Summoner{PUUID: r161Ref(1), GameName: "Player1", TagLine: "CN1"}
	f.raw = nil
	for i := 0; i < 10; i++ {
		p := &f.snapshot.RosterPlayers[i]
		p.GameName, p.TagLine = fmt.Sprintf("Player%d", i), "CN1"
		if i == 0 {
			p.GameName, p.TagLine, p.IsBot = "", "", bot
			continue
		}
		team := int64(100)
		if i >= 5 {
			team = 200
		}
		f.raw = append(f.raw, liveRosterEntry{lcuLivePlayer{PUUID: r161Ref(i), GameName: p.GameName, TagLine: p.TagLine, SelectedPosition: p.Position, ChampionID: 1}, team})
	}
	f.app.allSkins = []Skin{{ChampionID: 1, ChampionName: "安妮"}}
	base := f.client.http.Transport
	f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/lol-gameflow/v1/session" {
			return base.RoundTrip(r)
		}
		one, two := []lcuLivePlayer{}, []lcuLivePlayer{}
		for _, entry := range f.raw {
			if entry.team == 100 {
				one = append(one, entry.player)
			} else {
				two = append(two, entry.player)
			}
		}
		value := map[string]any{"gameData": map[string]any{"gameId": f.gameID, "queue": map[string]any{"id": f.queue, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": one, "teamTwo": two}}
		data, _ := json.Marshal(value)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})
	f.app.liveClientPlayerList = func(context.Context) ([]byte, int, error) {
		live := []any{}
		for _, p := range f.snapshot.RosterPlayers {
			riotID := ""
			if p.GameName != "" {
				riotID = p.GameName + "#" + p.TagLine
			}
			live = append(live, map[string]any{"riotId": riotID, "team": p.Team, "position": p.Position, "championName": p.ChampionName, "isBot": p.IsBot})
		}
		data, err := json.Marshal(live)
		return data, 200, err
	}
	return f
}

func (f *r194Fixture) load(phase string) gameplayLiveResponse {
	return f.app.loadGameplayLive(context.Background(), f.client, f.current, phase)
}

func TestR194QuickplayAnonymousTopEndToEnd(t *testing.T) {
	f := newR194Fixture(t, 480, false)
	response := f.load("InProgress")
	counts := map[int64]int{}
	var hidden gameplayLivePlayer
	for _, p := range response.Players {
		counts[p.TeamID]++
		if p.Hidden {
			hidden = p
		}
	}
	if !response.Available || len(response.Players) != 10 || counts[100] != 5 || counts[200] != 5 || response.MergeAppended != 1 {
		t.Fatalf("quickplay recovery: players=%d teams=%v appended=%d", len(response.Players), counts, response.MergeAppended)
	}
	if !hidden.Hidden || hidden.Position != "top" || hidden.ChampionID != 1 || hidden.TeamID != 100 || hidden.PlayerRef != "" || hidden.reference.PlayerRef != "" || hidden.Rank != nil || hidden.HistoryState != "unavailable" || hidden.PremadeGroup != "" || hidden.PremadeSize != 0 || hidden.MySquad || hidden.IsCurrent {
		t.Fatalf("anonymous TOP safety: %+v", hidden)
	}
	if f.count("/lol-summoner/v2/summoners/puuid/") != 9 || f.count("/lol-match-history/") != 9 || f.count("/lol-ranked/") != 9 || f.count("/lol-summoner/v1/alias/lookup") != 4 {
		t.Fatal(f.requests)
	}
	rows := r175Events(t, f.app, "live_roster_recovery")
	if len(rows) != 1 || rows[0]["reason"] != "anonymous-placeholder" || rows[0]["queue_id"] != float64(480) || rows[0]["appended"] != float64(1) || rows[0]["anonymous_count"] != float64(1) || rows[0]["named_present"] != float64(4) {
		t.Fatal(rows)
	}
	if len(r175Events(t, f.app, "backend_panic")) != 0 {
		t.Fatal("load pipeline panicked")
	}
	if got := f.load("Reconnect"); len(got.Players) != 10 || f.count("/lol-summoner/v1/alias/lookup") != 4 || len(r175Events(t, f.app, "live_roster_recovery")) != 1 {
		t.Fatal("Reconnect did not retain recovered roster/cache", got)
	}
	encoded, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("R194_WRITE_FIXTURE") == "1" {
		if err := os.WriteFile("testdata/r194-quickplay-anonymous-live.json", append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := os.ReadFile("testdata/r194-quickplay-anonymous-live.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture gameplayLiveResponse
	if err := json.Unmarshal(saved, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Players) != 10 || fixture.QueueID != 480 || fixture.Players[9].Position != hidden.Position || !fixture.Players[9].Hidden || fixture.Players[9].PlayerRef != "" || fixture.Players[9].ChampionID != hidden.ChampionID || fixture.Players[9].TeamID != 100 {
		t.Fatal("saved backend fixture diverged")
	}
}

func TestR194TenPlayerQueues(t *testing.T) {
	for _, queue := range []int64{400, 420, 430, 440, 450, 480, 490, 700, 720, 900, 930, 1900, 2300, 2400, 3220, 3270} {
		if !liveTenPlayerRosterQueue(queue) || !liveAnonymousRosterQueue(queue) {
			t.Errorf("queue %d must allow verified 5v5 recovery", queue)
		}
	}
	for _, queue := range []int64{820, 830, 840, 850, 860, 870, 880, 890, 950, 960, 1300, 1700, 1710, 1750, 3100, 4210, 999999} {
		if liveTenPlayerRosterQueue(queue) || liveAnonymousRosterQueue(queue) {
			t.Errorf("queue %d must not allow anonymous recovery", queue)
		}
	}
}

func TestR194BotsAndUnsupportedDiagnostic(t *testing.T) {
	t.Run("880-once-per-game", func(t *testing.T) {
		f := newR194Fixture(t, 880, true)
		for _, phase := range []string{"InProgress", "InProgress", "Reconnect"} {
			response := f.load(phase)
			if len(response.Players) != 9 || response.MergeAppended != 0 {
				t.Fatalf("bots must remain unsupported: %+v", response)
			}
		}
		rows := r175Events(t, f.app, "live_roster_recovery")
		if len(rows) != 1 || rows[0]["reason"] != "queue-unsupported" || rows[0]["queue_id"] != float64(880) || rows[0]["appended"] != float64(0) || f.count("/lol-summoner/v1/alias/lookup") != 0 {
			t.Fatal(rows, f.requests)
		}
		fields := " event game_id queue_id phase raw_count playerlist_count appended alias_attempts reason anonymous_count named_present named_appended unresolved_named cached time build_fingerprint run_id log_seq "
		for key := range rows[0] {
			if !strings.Contains(fields, " "+key+" ") {
				t.Fatalf("unexpected recovery diagnostic field: %s", key)
			}
		}
		f.gameID++
		f.load("InProgress")
		if rows := r175Events(t, f.app, "live_roster_recovery"); len(rows) != 2 || rows[1]["game_id"] != float64(195) {
			t.Fatal(rows)
		}
		f.app.clearLiveRosterRecovery()
		f.load("InProgress")
		if len(r175Events(t, f.app, "live_roster_recovery")) != 3 {
			t.Fatal("clear did not reset unsupported diagnostic")
		}
	})
	// With the queue gate deliberately bypassed by using an eligible queue,
	// isBot must independently prevent an anonymous placeholder. Without this
	// case the isBot mutant would be masked by the unsupported-880 gate.
	t.Run("isBot-independent-gate-and-cache", func(t *testing.T) {
		f := newR194Fixture(t, 480, false)
		if got := f.load("InProgress"); len(got.Players) != 10 {
			t.Fatal("initial human anonymous slot was not recovered")
		}
		f.snapshot.RosterPlayers[0].IsBot = true
		f.app.clearLiveClientProbe()
		got := f.load("InProgress")
		if len(got.Players) != 9 || got.MergeAppended != 0 {
			t.Fatalf("bot incorrectly recovered or old anonymous cache reused: %+v", got)
		}
		rows := r175Events(t, f.app, "live_roster_recovery")
		if len(rows) != 2 || rows[1]["reason"] != "partial" || rows[1]["anonymous_count"] != float64(0) || rows[1]["cached"] != false {
			t.Fatal(rows)
		}
		if !f.app.liveClientProbe.Snapshot.RosterPlayers[0].IsBot {
			t.Fatal("raw isBot was dropped while parsing Live Client")
		}
	})
}

func TestR194QuickplaySnapshotComplete(t *testing.T) {
	f := newR194Fixture(t, 480, false)
	response := gameplayLiveResponse{Available: true, QueueID: 480}
	for _, p := range f.raw {
		response.Players = append(response.Players, gameplayLivePlayer{TeamID: p.team, HistoryState: "unavailable"})
	}
	if gameplayLiveSnapshotComplete(response) {
		t.Fatal("9-player quickplay snapshot must remain refreshable")
	}
	if !gameplayLiveSnapshotComplete(f.load("InProgress")) {
		t.Fatal("recovered 5+5 quickplay snapshot must be complete")
	}
}
