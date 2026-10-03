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
	"time"
)

type r197Fixture struct {
	a           *app
	client      *LCUClient
	mu          sync.Mutex
	game        lcuGame
	snapshot    gameplayLiveResponse
	calls       []string
	detailCalls int
	failDetails int
	sessionID   int64
	waitCalls   int
	now         time.Time
	stopAt      int
	cancel      context.CancelFunc
}

func newR197Fixture(t *testing.T, queue int64) *r197Fixture {
	f := &r197Fixture{a: r175App(t), sessionID: 9012766973, now: time.Unix(0, 0)}
	f.game = lcuGame{GameID: f.sessionID, QueueID: queue, GameMode: "CLASSIC", GameDuration: 1800}
	if queue == 2400 {
		f.game.GameMode = "ARAM_MAYHEM"
	}
	f.game.Teams = []lcuTeam{{TeamID: 100}, {TeamID: 200}}
	f.snapshot = gameplayLiveResponse{GameID: f.sessionID, QueueID: queue, GameMode: f.game.GameMode, Available: true, Phase: "InProgress"}
	for i := 0; i < 10; i++ {
		team := int64(100)
		if i >= 5 {
			team = 200
		}
		champion := int64(i + 1)
		if i == 9 {
			champion = 235
		}
		participant := lcuParticipant{ParticipantID: int64(i + 1), TeamID: team, ChampionID: champion}
		participant.Stats.Kills = 4
		participant.Stats.Deaths = 2
		participant.Stats.Assists = 8
		participant.Stats.Win = true
		f.game.Participants = append(f.game.Participants, participant)
		identity := lcuParticipantIdentity{ParticipantID: int64(i + 1)}
		identity.Player.PUUID = r161Ref(i)
		identity.Player.GameName = fmt.Sprintf("Fixture%d", i)
		identity.Player.TagLine = "TEST"
		f.game.ParticipantIdentities = append(f.game.ParticipantIdentities, identity)
		player := gameplayLivePlayer{gameplayPlayer: gameplayPlayer{PlayerRef: r161Ref(i), GameName: identity.Player.GameName, DisplayName: identity.Player.GameName, reference: gameplayReference{PlayerRef: r161Ref(i), GameName: identity.Player.GameName}}, TeamID: team, ChampionID: champion, ChampionName: fmt.Sprintf("英雄%d", champion), Position: "support", HistoryState: "loaded"}
		if i == 9 {
			player.Hidden = true
			player.PlayerRef = ""
			player.GameName = ""
			player.DisplayName = "隐藏玩家"
			player.reference = gameplayReference{}
			player.HistoryState = "unavailable"
			player.ChampionName = "赛娜"
		}
		f.snapshot.Players = append(f.snapshot.Players, player)
	}
	f.client = &LCUClient{baseURL: "http://lcu.local", token: "test", platformProbe: true}
	f.client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		var payload any
		status := 200
		switch {
		case r.URL.Path == "/lol-gameflow/v1/gameflow-phase":
			payload = "EndOfGame"
		case r.URL.Path == "/lol-gameflow/v1/session":
			var session lcuGameflowSession
			session.GameData.GameID = f.sessionID
			payload = session
		case r.URL.Path == fmt.Sprintf("/lol-match-history/v1/games/%d", f.game.GameID):
			f.detailCalls++
			payload = f.game
			if f.detailCalls <= f.failDetails {
				status = 404
				payload = map[string]string{"error": "not-ready"}
			}
		case strings.Contains(r.URL.Path, "current-summoner/matches"):
			var history lcuMatchHistory
			summary := f.game
			summary.Participants = summary.Participants[:1]
			summary.ParticipantIdentities = summary.ParticipantIdentities[:1]
			history.Games.Games = []lcuGame{summary}
			payload = history
		case strings.Contains(r.URL.Path, "/matches"):
			var history lcuMatchHistory
			history.Games.Games = []lcuGame{f.game}
			payload = history
		case strings.HasPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
			ref := strings.TrimPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/")
			name := ""
			for _, identity := range f.game.ParticipantIdentities {
				if identity.Player.PUUID == ref {
					name = identity.Player.GameName
					break
				}
			}
			payload = map[string]any{"puuid": ref, "gameName": name, "tagLine": "TEST", "displayName": name}
		case strings.Contains(r.URL.Path, "/lol-ranked/v1/ranked-stats/"):
			payload = map[string]any{"queues": []map[string]any{{"queueType": "RANKED_FLEX_SR", "tier": "GOLD", "division": "I", "wins": 10, "losses": 5}, {"queueType": "RANKED_SOLO_5x5", "tier": "GOLD", "division": "I", "wins": 10, "losses": 5}}}
		case strings.Contains(r.URL.Path, "/champions"):
			payload = []map[string]any{{"id": 235, "name": "赛娜"}}
		default:
			status = 404
			payload = map[string]any{}
		}
		data, _ := json.Marshal(payload)
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})}
	f.a.lcu = f.client
	f.a.summoner = Summoner{PUUID: r161Ref(0), GameName: "Fixture0"}
	f.a.liveSnapshots.client = f.client
	f.a.liveSnapshots.response = clonePostGameSnapshot(f.snapshot)
	parent, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	t.Cleanup(func() { cancel(); f.a.stopPostGameReveal() })
	f.a.postGameReveal.now = func() time.Time { return f.now }
	f.a.postGameReveal.wait = func(ctx context.Context, d time.Duration) bool {
		f.waitCalls++
		f.now = f.now.Add(d)
		if f.stopAt == f.waitCalls {
			f.a.observePostGameReveal(parent, f.client, "ChampSelect")
		}
		return ctx.Err() == nil
	}
	return f
}
func (f *r197Fixture) run(t *testing.T) {
	t.Helper()
	s := &f.a.postGameReveal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.client = f.client
	s.snapshot = clonePostGameSnapshot(f.snapshot)
	s.ctx = ctx
	s.cancel = cancel
	s.generation = 1
	s.running = true
	s.started = true
	f.a.runPostGameReveal(ctx, f.client, 1, f.snapshot, s.wait, s.now)
}
func TestR197SennaPostGameReveal(t *testing.T) {
	for _, queue := range []int64{2400, 440} {
		t.Run(fmt.Sprint(queue), func(t *testing.T) {
			f := newR197Fixture(t, queue)
			f.game.Participants[5], f.game.Participants[9] = f.game.Participants[9], f.game.Participants[5]
			f.run(t)
			next, ok := f.a.postGameSnapshot(f.client, "EndOfGame", f.snapshot.GameID)
			if !ok || next.Players[9].Hidden || next.Players[9].GameName != "Fixture9" || next.Players[9].PlayerRef == "" || len(next.Players[9].RecentGames) == 0 {
				t.Fatalf("not revealed: %#v", next.Players)
			}
			if queue == 440 && next.Players[9].Rank == nil {
				t.Fatal("ordinary rank loader not run")
			}
			if queue == 2400 && next.Players[9].Rank != nil {
				t.Fatal("ARAM fake rank")
			}
			rows := r175Events(t, f.a, "live_roster_post_game_reveal")
			if len(rows) != 1 || rows[0]["reason"] != "ok" || rows[0]["revealed"] != float64(1) {
				t.Fatal(rows)
			}
			for i := 0; i < 9; i++ {
				if next.Players[i].ChampionID != f.snapshot.Players[i].ChampionID || next.Players[i].PlayerRef != f.snapshot.Players[i].PlayerRef {
					t.Fatal("existing roster modified")
				}
			}
			data, _ := json.Marshal(rows)
			if strings.Contains(string(data), r161Ref(9)) || strings.Contains(string(data), "Fixture9") {
				t.Fatal("identity in diagnostic")
			}
			if queue == 440 && os.Getenv("R197_WRITE_FIXTURE") == "1" {
				beforeFixture, afterFixture := clonePostGameSnapshot(f.snapshot), clonePostGameSnapshot(next)
				for i := range beforeFixture.Players {
					if !beforeFixture.Players[i].Hidden {
						ref := f.a.registerGameplayReferenceDetails(beforeFixture.Players[i].reference)
						beforeFixture.Players[i].PlayerRef = ref
						afterFixture.Players[i].PlayerRef = ref
					}
				}
				bytes, _ := json.MarshalIndent(map[string]any{"before": beforeFixture, "after": afterFixture}, "", "  ")
				if err := os.WriteFile("testdata/r197-post-game-reveal.json", append(bytes, '\n'), 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestR197MatchNotReadyRetries(t *testing.T) {
	for _, failures := range []int{1, 3} {
		t.Run(fmt.Sprint(failures), func(t *testing.T) {
			f := newR197Fixture(t, 2400)
			f.failDetails = failures
			f.run(t)
			rows := r175Events(t, f.a, "live_roster_post_game_reveal")
			want := 2
			if failures == 3 {
				want = 3
			}
			if f.detailCalls != want || len(rows) != want || rows[len(rows)-1]["attempt"] != float64(want) {
				t.Fatal(f.detailCalls, rows)
			}
			if failures == 1 && rows[1]["reason"] != "ok" || failures == 3 && rows[2]["reason"] != "match-not-ready" {
				t.Fatal(rows)
			}
			if failures == 3 {
				next, _ := f.a.postGameSnapshot(f.client, "EndOfGame", 0)
				if !next.Players[9].Hidden || f.a.postGameReveal.running {
					t.Fatal("failed match revealed or still running")
				}
				if f.now.Sub(time.Unix(0, 0)) != 120*time.Second {
					t.Fatal("retry deadlines drifted")
				}
			}
		})
	}
}
func TestR197AmbiguousAndHiddenParticipants(t *testing.T) {
	for _, mode := range []string{"ambiguous", "hidden", "already-present", "no-candidate"} {
		t.Run(mode, func(t *testing.T) {
			f := newR197Fixture(t, 2400)
			switch mode {
			case "ambiguous":
				f.game.Participants[8].ChampionID = 235
			case "hidden":
				f.game.ParticipantIdentities[9].Player.PUUID = ""
				f.game.ParticipantIdentities[9].Player.GameName = ""
			case "already-present":
				f.game.ParticipantIdentities[9].Player.PUUID = r161Ref(0)
			case "no-candidate":
				f.game.Participants[9].ChampionID = 236
			}
			f.run(t)
			next, _ := f.a.postGameSnapshot(f.client, "EndOfGame", 0)
			if !next.Players[9].Hidden {
				t.Fatal("unsafe participant revealed")
			}
			rows := r175Events(t, f.a, "live_roster_post_game_reveal")
			want := map[string]string{"ambiguous": "ambiguous", "hidden": "participant-hidden", "already-present": "no-candidate", "no-candidate": "no-candidate"}[mode]
			if len(rows) != 1 || rows[0]["reason"] != want {
				t.Fatal(rows)
			}
		})
	}
}
func TestR197InProgressNeverLoadsMatch(t *testing.T) {
	for _, phase := range []string{"InProgress", "Reconnect"} {
		f := newR197Fixture(t, 2400)
		f.a.observePostGameReveal(context.Background(), f.client, phase)
		time.Sleep(30 * time.Millisecond)
		f.mu.Lock()
		requests := append([]string(nil), f.calls...)
		detailCalls := f.detailCalls
		f.mu.Unlock()
		if detailCalls != 0 || len(requests) != 0 {
			t.Fatalf("%s requested match=%v", phase, f.calls)
		}
	}
}
func TestR197NewChampSelectStopsRetries(t *testing.T) {
	f := newR197Fixture(t, 2400)
	f.failDetails = 3
	f.stopAt = 2
	f.run(t)
	if f.detailCalls != 1 {
		t.Fatal("requests after ChampSelect", f.calls)
	}
	rows := r175Events(t, f.a, "live_roster_post_game_reveal")
	if len(rows) != 2 || rows[1]["reason"] != "stopped" {
		t.Fatal(rows)
	}
}
func TestR197SnapshotGameMismatchDoesNothing(t *testing.T) {
	f := newR197Fixture(t, 2400)
	f.sessionID++
	f.run(t)
	if f.detailCalls != 0 || f.a.postGameReveal.snapshot.GameID != 0 {
		t.Fatal("stale game processed")
	}
}
func TestR197RetryGameChangeStopsBeforeHistory(t *testing.T) {
	f := newR197Fixture(t, 2400)
	f.failDetails = 3
	wait := f.a.postGameReveal.wait
	f.a.postGameReveal.wait = func(ctx context.Context, d time.Duration) bool {
		ok := wait(ctx, d)
		if f.waitCalls == 2 {
			f.mu.Lock()
			f.sessionID++
			f.mu.Unlock()
		}
		return ok
	}
	f.run(t)
	if f.detailCalls != 1 || f.a.postGameReveal.snapshot.GameID != 0 {
		t.Fatal("old game survived retry identity change", f.calls)
	}
	rows := r175Events(t, f.a, "live_roster_post_game_reveal")
	if len(rows) != 2 || rows[1]["reason"] != "stopped" {
		t.Fatal(rows)
	}
}
func TestR197PhaseLifecycleRetainsAndCancelsSnapshot(t *testing.T) {
	f := newR197Fixture(t, 2400)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	block := make(chan struct{})
	f.a.postGameReveal.wait = func(ctx context.Context, _ time.Duration) bool { close(block); <-ctx.Done(); return false }
	f.a.observeGameplayPhase(ctx, f.client, "WaitingForStats")
	if f.a.postGameReveal.started {
		t.Fatal("started too early")
	}
	f.a.observeGameplayPhase(ctx, f.client, "PreEndOfGame")
	select {
	case <-block:
	case <-time.After(time.Second):
		t.Fatal("reveal not scheduled")
	}
	f.a.observeGameplayPhase(ctx, f.client, "EndOfGame")
	if snapshot, ok := f.a.postGameSnapshot(f.client, "EndOfGame", 0); !ok || !snapshot.Players[9].Hidden {
		t.Fatal("end transition dropped snapshot")
	}
	f.a.postGameSnapshot(f.client, "EndOfGame", 123)
	if f.a.postGameReveal.client != nil {
		t.Fatal("different game did not stop")
	}
	if f.detailCalls != 0 {
		t.Fatal("unexpected lookup before 10 sec")
	}
}
