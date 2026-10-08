package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestR244OldLiveHistoryAndHeaderShareRows(t *testing.T) {
	now := time.Now()
	ref := r161Ref(1)
	for _, count := range []int{10, 3, 0} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			matches := []gameplayMatch{}
			for i := 0; i < 16; i++ {
				queue, result := int64(450), "win"
				if i < count {
					queue = 420
				}
				if i == 15 {
					queue, result = 420, "remake"
				}
				matches = append(matches, gameplayMatch{GameID: int64(i + 1), QueueID: queue, CreatedAt: now.Add(-60*24*time.Hour - time.Duration(i)*time.Minute).UnixMilli(), Result: result, SubjectParticipantID: 1, Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: ref, Win: true, Kills: 2, Deaths: 1, Assists: 3}}})
			}
			stats, games := liveRecentPlayerStats(matches, ref, 420)
			diagnostic := liveHistoryFreshnessDiagnostic(livePlayerMatchesResult{Matches: matches}, ref, 420, 100, 0, true, now)
			if len(games) != count || stats.Games != count || diagnostic["header_games"] != count || diagnostic["shown_count"] != count {
				t.Fatal(stats, games, diagnostic)
			}
			if _, stale := diagnostic["window_days"]; stale {
				t.Fatal(diagnostic)
			}
			age := diagnostic["oldest_shown_age_days"].(*float64)
			if count > 0 && (age == nil || *age < 60) || count == 0 && age != nil {
				t.Fatal(diagnostic)
			}
		})
	}
}
func TestR244UnfilteredHistoryStopsAfterTwoPages(t *testing.T) {
	f := r180Fixture(t)
	ref := r161Ref(1)
	f.a.setQueueFilterCapability("HN1", "solo:q_420", queueFilterCapabilityUnsupported)
	calls := 0
	f.a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		offset, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
		if offset != (calls-1)*30 || calls > 2 || r.URL.Query().Get("count") != "30" {
			t.Fatal(r.URL)
		}
		count := 4
		if calls == 2 {
			count = 6
		}
		games := []any{}
		for i := 0; i < 30; i++ {
			queue := int64(450)
			if i < count {
				queue = 420
			}
			game := f.sgpGame(int64(offset+i+1), queue, ref)
			game["gameCreation"] = f.now - int64(60*24*time.Hour/time.Millisecond) - int64(offset+i)*60000
			games = append(games, map[string]any{"json": game})
		}
		return r178JSON(map[string]any{"games": games}, 200), nil
	})
	got := f.a.loadLivePlayerMatches(context.Background(), f.c, gameplayReference{ServerID: "HN1"}, ref, false, nil, 420)
	if calls != 2 || len(recentLiveMatchesForPlayer(got.Matches, ref, 420, time.Now())) != 10 || got.Evidence.StopReason != "enough" || f.lcuCalls.Load() != 0 {
		t.Fatal(calls, got.Evidence, f.lcuCalls.Load())
	}
}
func TestR244SGPAuthoritativeShortEmptyAndSelfCatchUp(t *testing.T) {
	for _, count := range []int{0, 3, 10} {
		for _, self := range []bool{false, true} {
			f := r180Fixture(t)
			ref := r161Ref(1)
			if self {
				ref = f.a.summoner.PUUID
			}
			f.a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				games := []any{}
				for i := 0; i < count; i++ {
					games = append(games, map[string]any{"json": f.sgpGame(int64(i+1), 440, ref)})
				}
				return r178JSON(map[string]any{"games": games}, 200), nil
			})
			got := f.a.loadLivePlayerMatchesForPrevious(context.Background(), f.c, gameplayReference{ServerID: "HN1"}, ref, self, nil, 440, 0)
			if f.lcuCalls.Load() != 0 || got.Source != "sgp" || len(got.Matches) != count {
				t.Fatal(count, self, got, f.lcuCalls.Load())
			}
		}
	}
	f := r180Fixture(t)
	ref := f.a.summoner.PUUID
	got := f.a.loadLivePlayerMatchesForPrevious(context.Background(), f.c, gameplayReference{ServerID: "HN1"}, ref, true, nil, 440, 101)
	if f.lcuCalls.Load() != 1 || got.Source != "lcu+sgp" || !got.Evidence.LCURequested {
		t.Fatal(got, f.lcuCalls.Load())
	}
	f = r180Fixture(t)
	ref = f.a.summoner.PUUID
	got = f.a.loadLivePlayerMatchesForPrevious(context.Background(), f.c, gameplayReference{ServerID: "HN1"}, ref, true, nil, 440, 112)
	if f.lcuCalls.Load() != 0 || got.Source != "sgp" {
		t.Fatal(got, f.lcuCalls.Load())
	}
}
func TestR244SGPFailureFallsBackLCU(t *testing.T) {
	f := r180Fixture(t)
	f.sgpFailed.Store(true)
	got := f.a.loadLivePlayerMatches(context.Background(), f.c, gameplayReference{ServerID: "HN1"}, r161Ref(1), false, nil, 440)
	if f.lcuCalls.Load() != 1 || got.Source != "lcu" || len(got.Matches) == 0 {
		t.Fatal(got, f.lcuCalls.Load())
	}
}
func TestR244LightDiscoveryAndOneSweepPerLaunch(t *testing.T) {
	a := &app{}
	stage, commands, sweeps := 0, 0, 0
	a.coldDiscovery.snapshot = func() (processQueryResult, error) {
		if stage == 0 {
			return processQueryResult{}, nil
		}
		return processQueryResult{ProcessCount: 1}, nil
	}
	a.coldDiscovery.commands = func() (processQueryResult, error) {
		commands++
		q := processQueryResult{ProcessCount: 1, CommandLines: []string{`LeagueClientUx.exe --install-directory="` + t.TempDir() + `"`}}
		if stage == 2 {
			q.CommandLines = nil
			q.Unreadable = 1
		}
		return q, nil
	}
	a.coldDiscovery.candidates = func([]string) []string { sweeps++; return []string{filepath.Join(t.TempDir(), "missing-lockfile")} }
	for i := 0; i < 4; i++ {
		_, r, _ := a.discoverColdLCU()
		if r.LockfilesChecked != 0 || r.Sweep || commands != 0 {
			t.Fatal(r, commands)
		}
	}
	stage = 1
	_, r, _ := a.discoverColdLCU()
	if r.Sweep || sweeps != 0 || r.LockfilesChecked != 1 {
		t.Fatal(r, sweeps)
	}
	stage = 2
	for i := 0; i < 4; i++ {
		_, r, _ := a.discoverColdLCU()
		if r.Sweep != (i == 0) || sweeps != 1 || i > 0 && r.LockfilesChecked != 0 {
			t.Fatal(i, r, sweeps)
		}
	}
	stage = 0
	a.discoverColdLCU()
	stage = 2
	_, r, _ = a.discoverColdLCU()
	if sweeps != 2 || !r.Sweep {
		t.Fatal(r, sweeps)
	}
	p50, p95 := a.discoverySnapshotPercentiles()
	if p95 >= 20 || p50 > p95 {
		t.Fatal(p50, p95)
	}
}
func TestR244ProbeReasonsAndTimeout(t *testing.T) {
	c := newLCUClient(1, "test")
	if c.http.Timeout != 8*time.Second {
		t.Fatal(c.http.Timeout)
	}
	for _, tc := range []struct {
		code       int
		body, kind string
	}{{404, `{}`, "http-404"}, {200, `{}`, "no-summoner"}, {200, `"not-json-object"`, "no-summoner"}, {200, `{"summonerId":1}`, ""}} {
		c.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) > 1500*time.Millisecond {
				t.Error("probe lacks short deadline")
			}
			return r178JSON(json.RawMessage(tc.body), tc.code), nil
		})
		_, open, err := c.discoveryProbe(context.Background())
		if !open || discoveryProbeErrorKind(err) != tc.kind {
			t.Fatal(open, err)
		}
	}
	for _, tc := range []struct {
		err  error
		kind string
	}{{errors.New("connection refused"), "refused"}, {context.DeadlineExceeded, "timeout"}, {errors.New("tls handshake failed"), "tls"}} {
		if discoveryProbeErrorKind(tc.err) != tc.kind {
			t.Fatal(tc)
		}
	}
}
func TestR244ColdConnectionSequenceAndSummonerEvent(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	eventAt := make(chan time.Time, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.Error(w, "starting", 404)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
		eventAt <- time.Now()
		conn.WriteJSON([]any{8, "OnJsonApiEvent", map[string]any{"uri": "/lol-summoner/v1/current-summoner", "eventType": "Update", "data": map[string]any{"summonerId": 44, "puuid": r161Ref(0)}}})
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	client := newLCUClient(port, "fixture")
	defer client.Close()
	a := r175App(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	delays := []time.Duration{}
	a.runConnectionManagerWith(ctx, connectionLoopOps{
		discover: func() (*LCUClient, LCUDiscoveryStatus, error) {
			attempts++
			now := time.Now()
			r := LCUDiscoveryStatus{AttemptAt: now}
			switch attempts {
			case 1, 2, 3:
				r.Result = "process-not-found"
				return nil, r, errLCUNotFound
			case 4:
				r.ProcessCount = 1
				r.Result = "credentials-unreadable"
				r.CommandLineCount = 1
				r.CommandLineAt = now
				return nil, r, errLCUCredentialsUnreadable
			case 5:
				r.ProcessCount = 1
				r.Result = "probe-failed"
				r.CredentialCandidates = 1
				r.CredentialsAt = now
				r.ProbeErrorKind = "refused"
				return nil, r, errLCUProbeFailed
			default:
				r.ProcessCount = 1
				r.Result = "probe-failed"
				r.PortOpen = true
				r.PortOpenAt = now
				r.ProbeErrorKind = "no-summoner"
				return client, r, errLCUProbeFailed
			}
		},
		identity: func(c *LCUClient) bool {
			at := <-eventAt
			if time.Since(at) > 100*time.Millisecond {
				t.Error("event delayed", time.Since(at))
			}
			c.mu.RLock()
			seed := c.discoverySummoner
			c.mu.RUnlock()
			if seed == nil || seed.SummonerID != 44 {
				t.Error(seed)
			}
			return true
		},
		session: func(context.Context, *LCUClient) error {
			a.observeColdLaunchMilestone("overview_first_card_ms", time.Now())
			a.observeColdLaunchMilestone("overlay_hidden_ms", time.Now())
			a.observeColdLaunchMilestone("overlay_hidden_ms", time.Now())
			cancel()
			return nil
		},
		wait: func(_ context.Context, d time.Duration) bool { delays = append(delays, d); return true },
	})
	if fmt.Sprint(delays) != fmt.Sprint([]time.Duration{time.Second, time.Second, time.Second, time.Second, 500 * time.Millisecond}) {
		t.Fatal(delays)
	}
	events := r175Events(t, a, "client_cold_launch_timeline")
	if len(events) != 1 {
		t.Fatal(events)
	}
	for _, name := range coldLaunchMilestones {
		if _, ok := events[0][name]; !ok {
			t.Fatal(name, events)
		}
	}
	if events[0]["client_running_at_app_start"] != false || events[0]["process_ms"] != float64(0) || events[0]["summoner_ready_ms"].(float64) < 0 {
		t.Fatal(events)
	}
}
func TestR244OverviewParallelAndEarlyMatches(t *testing.T) {
	a, _, _ := newGameplayOverviewSGPFixture(t, true)
	a.summoner.PUUID = strings.Repeat("o", 48)
	started := time.Now()
	phases := newOverviewPhaseTimings(started)
	old := a.sgp.http.Transport
	var count atomic.Int32
	a.sgp.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/match-history-query/") {
			count.Add(1)
			time.Sleep(500 * time.Millisecond)
		}
		return old.RoundTrip(r)
	})
	var progressAt time.Time
	var firstCardRequests int32
	ctx := context.WithValue(context.Background(), overviewPhasesContextKey{}, phases)
	ctx = context.WithValue(ctx, localOverviewProgressKey{}, func(partial gameplayOverview) {
		progressAt = time.Now()
		firstCardRequests = count.Load()
		if len(partial.Matches) == 0 || strings.Contains(partial.Player.PlayerRef, a.summoner.PUUID) {
			t.Error("bad early page", partial)
		}
	})
	response := a.loadGameplayOverview(ctx, a.lcu, a.summoner, gameplayReference{}, 0, 20, "all", false)
	elapsed := time.Since(started)
	if elapsed >= 800*time.Millisecond || progressAt.IsZero() || len(response.Matches) == 0 || firstCardRequests != 1 {
		t.Fatal(elapsed, progressAt, firstCardRequests, count.Load())
	}
	snapshot := phases.snapshot(time.Now())
	spans := snapshot["spans"].(map[string][][2]int64)
	if spans["recent_ranked"][0][0] > spans["detailed_matches"][0][0]+50 {
		t.Fatal(snapshot)
	}
}
func TestR244OverviewProgressDoesNotMutateAggregation(t *testing.T) {
	a, _, _ := newGameplayOverviewSGPFixture(t, true)
	a.summoner.PUUID = strings.Repeat("o", 48)
	ctx := context.WithValue(context.Background(), localOverviewProgressKey{}, func(gameplayOverview) {})
	got := a.loadGameplayOverview(ctx, a.lcu, a.summoner, gameplayReference{}, 0, 20, "all", false)
	if got.Overall.Games != 1 || got.Overall.Wins != 1 {
		t.Fatal("progress modified raw identities", got.Overall)
	}
}
func TestR244OverviewFlightReplaysEarlyCard(t *testing.T) {
	flight := &overviewQueryFlight{}
	var calls atomic.Int32
	ctx := context.WithValue(context.Background(), localOverviewProgressKey{}, func(gameplayOverview) { calls.Add(1) })
	done := flight.subscribeOverview(ctx)
	flight.publishOverview(gameplayOverview{})
	done()
	done = flight.subscribeOverview(ctx)
	done()
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
}

func TestR244SelfCatchUpPublishesOtherPlayersBeforeLCU(t *testing.T) {
	f := r180Fixture(t)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	old := f.c.http.Transport
	f.c.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/lol-match-history/") {
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return old.RoundTrip(r)
		}
		team := []any{}
		for i := 0; i < 5; i++ {
			team = append(team, map[string]any{"cellId": i, "puuid": r161Ref(i), "gameName": fmt.Sprint("Player", i), "selectedPosition": "TOP"})
		}
		switch {
		case r.URL.Path == "/lol-gameflow/v1/session":
			return r178JSON(map[string]any{"gameData": map[string]any{"gameId": 244, "queue": map[string]any{"id": 440, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": team}}, 200), nil
		case r.URL.Path == "/lol-champ-select/v1/session":
			return r178JSON(map[string]any{"gameId": 244, "queueId": 440, "localPlayerCellId": 0, "myTeam": team}, 200), nil
		case r.URL.Path == "/lol-lobby/v2/lobby":
			return r178JSON(map[string]any{"gameConfig": map[string]any{"queueId": 440, "mapId": 11, "gameMode": "CLASSIC"}}, 200), nil
		case strings.HasPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
			return r178JSON(map[string]any{"puuid": strings.TrimPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/"), "gameName": "Fixture"}, 200), nil
		default:
			return r178JSON(map[string]any{}, 404), nil
		}
	})
	f.a.observeLiveHistoryGame(f.c, "InProgress", 101, 440)
	var otherFirst bool
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, liveProgressPublisherKey{}, func(value gameplayLiveResponse) {
		otherReady, selfReady := 0, false
		for _, player := range value.Players {
			if len(player.RecentGames) > 0 {
				if player.IsCurrent {
					selfReady = true
				} else {
					otherReady++
				}
			}
		}
		if otherReady == 4 && !selfReady {
			otherFirst = true
			unblock()
		}
	})
	result := f.a.loadGameplayLive(ctx, f.c, Summoner{PUUID: f.a.summoner.PUUID}, "ChampSelect")
	if !otherFirst || len(result.Players) != 5 || f.lcuCalls.Load() != 1 {
		t.Fatal(otherFirst, len(result.Players), f.lcuCalls.Load())
	}
	costs := r175Events(t, f.a, "live_load_cost")
	if len(costs) != 1 || costs[0]["lcu_players"] != float64(1) || costs[0]["first_player_ms"].(float64) > 1200 {
		t.Fatal(costs)
	}
}
