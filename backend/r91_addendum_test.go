package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR91AddendumRateLimitJSON(t *testing.T) {
	for _, status := range []int{429, 500, 503} {
		w := httptest.NewRecorder()
		writeRiotHTTPError(w, &riotStatusError{status: status, message: "fixture", retryAfter: 5})
		var body riotHTTPError
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != status || body.RetryAfter != 5 || w.Header().Get("Retry-After") != "5" || (body.Kind == "rate-limited") != (status == 429) {
			t.Fatalf("status=%d body=%+v", w.Code, body)
		}
	}
}

func TestR91AddendumCacheThreeStates(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "fixture")
	c := newChampionProvider()
	c.cache = newChampionDataCache(&localStore{root: t.TempDir()})
	var calls atomic.Int32
	c.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return proHTTPBody([]byte(`{"metadata":{"matchId":"KR_123"},"info":{"gameId":123,"participants":[{"puuid":"fixture"}]}}`)), nil
	})}
	tracker := &riotOverviewCostTracker{}
	ctx := context.WithValue(context.Background(), riotOverviewCostTrackerKey{}, tracker)
	p := newRiotProvider(c)
	for _, want := range []string{"miss", "hit", "disk"} {
		if want == "disk" {
			p = newRiotProvider(c)
		}
		_, status, err := p.matchByIDWithCache(ctx, "KR_123")
		if err != nil || status != want {
			t.Fatalf("%s %s %v", want, status, err)
		}
	}
	if calls.Load() != 1 || tracker.matchesFromMemory != 1 || tracker.matchesFromDisk != 1 || tracker.matchesFromNetwork != 1 {
		t.Fatalf("calls=%d tracker=%+v", calls.Load(), tracker)
	}
}

// The slow details cannot finish until the HTTP client has received a preview.
// This proves one 20-ID pipeline actually streams, rather than two JSON requests.
func TestR91AddendumOneRequestStreamsFiveThenTwenty(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "fixture")
	c := newChampionProvider()
	c.cache = newChampionDataCache(nil)
	c.championMeta = map[int]championMetadata{1: {NameZH: "安妮"}}
	gate := make(chan struct{})
	defer func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	}()
	var details, lists atomic.Int32
	c.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := "[]"
		switch {
		case strings.Contains(r.URL.Path, "/summoner/"):
			body = `{"puuid":"r91-subject-identity","summonerLevel":100}`
		case strings.HasSuffix(r.URL.Path, "/ids"):
			lists.Add(1)
			if r.URL.Query().Get("count") != "20" {
				t.Errorf("wrong count %s", r.URL.RawQuery)
			}
			ids := []string{}
			for i := 1; i <= 20; i++ {
				ids = append(ids, fmt.Sprintf("KR_%d", i))
			}
			raw, _ := json.Marshal(ids)
			body = string(raw)
		case strings.Contains(r.URL.Path, "/lol/match/v5/matches/"):
			n := details.Add(1)
			if n > 5 {
				select {
				case <-gate:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			}
			id := strings.TrimPrefix(r.URL.Path, "/lol/match/v5/matches/KR_")
			body = fmt.Sprintf(`{"metadata":{"matchId":"KR_%s"},"info":{"gameId":%s,"queueId":420,"gameDuration":1800,"participants":[{"puuid":"r91-subject-identity","participantId":1,"teamId":100,"championId":1}]}}`, id, id)
		case strings.Contains(r.URL.Path, "/league/"), strings.Contains(r.URL.Path, "/champion-mastery/"):
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		return proHTTPBody([]byte(body)), nil
	})}
	a := &app{riot: newRiotProvider(c), overviewQueries: newOverviewQueryCache()}
	ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: "r91-subject-identity", GameName: "Fixture", TagLine: "KR1", Region: "kr", Privacy: "PRIVATE"})
	server := httptest.NewServer(http.HandlerFunc(a.handleGameplayOverview))
	defer server.Close()
	req, _ := http.NewRequest("POST", server.URL, strings.NewReader(fmt.Sprintf(`{"playerRef":%q,"count":20}`, ref)))
	req.Header.Set("Accept", "application/x-ndjson")
	client := server.Client()
	client.Timeout = 10 * time.Second
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	if !scanner.Scan() {
		t.Fatalf("preview missing: %v", scanner.Err())
	}
	var frame struct {
		Type     string           `json:"type"`
		Overview gameplayOverview `json:"overview"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
		t.Fatalf("%v: status=%d body=%s", err, res.StatusCode, scanner.Bytes())
	}
	if frame.Type != "progress" || len(frame.Overview.Matches) != 5 {
		t.Fatalf("preview %s count=%d", frame.Type, len(frame.Overview.Matches))
	}
	if bytes.Contains(scanner.Bytes(), []byte(`"puuid"`)) || frame.Overview.Player.PlayerRef == "r91-subject-identity" {
		t.Fatal("private identity leaked in stream")
	}
	close(gate)
	for scanner.Scan() {
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Type != "progress" {
			break
		}
		if len(frame.Overview.Matches) < 5 || len(frame.Overview.Matches) > 20 {
			t.Fatal("invalid cumulative preview")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if frame.Type != "complete" || len(frame.Overview.Matches) != 20 || details.Load() != 20 || lists.Load() != 1 {
		t.Fatalf("type=%s matches=%d details=%d lists=%d", frame.Type, len(frame.Overview.Matches), details.Load(), lists.Load())
	}
}

func TestR91AddendumGameStartGroupingAndCarryover(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	a := r90Fixture(t, players, raw)
	var probes atomic.Int32
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { probes.Add(1); return nil, 0, errors.New("loading") }
	got := a.loadGameplayLive(context.Background(), a.lcu, a.summoner, "GameStart")
	if got.ArenaGrouped || !got.ArenaGroupingUnavailable || probes.Load() != 0 {
		t.Fatalf("GameStart grouping=%t source=%s probes=%d", got.ArenaGrouped, got.ArenaGroupSource, probes.Load())
	}
	// Remove the middle member, retaining original identity-to-block assignments.
	next := got
	next.Phase = "InProgress"
	next.ArenaGrouped = false
	next.Players = append(append([]gameplayLivePlayer{}, got.Players[:4]...), got.Players[5:]...)
	rawPlayers := make([]lcuLivePlayer, len(next.Players))
	for i := range next.Players {
		next.Players[i].ArenaGroup = ""
		rawPlayers[i].PUUID = next.Players[i].reference.PlayerRef
	}
	a.applyArenaLiveGrouping(a.lcu, a.summoner, &next, rawPlayers, liveClientSnapshot{})
	if next.ArenaGrouped || next.Players[4].ArenaGroup != "" || next.Players[5].ArenaGroup != "" {
		t.Fatal("carryover lost boundaries")
	}
	next.GameID++
	next.ArenaGrouped = false
	a.applyArenaLiveGrouping(a.lcu, a.summoner, &next, rawPlayers, liveClientSnapshot{})
	if next.ArenaGrouped || !next.ArenaGroupingUnavailable || !next.ArenaGroupingRetryable {
		t.Fatal("new game borrowed old grouping or missing failure")
	}
	reasons := map[string]bool{}
	for _, e := range r90Events(t, a, "arena_group_order_rejected") {
		reasons[fmt.Sprint(e["reason"])] = true
	}
	if !reasons["order-inference-disabled"] {
		t.Fatal(reasons)
	}
	if gameplayLiveSnapshotComplete(next) {
		t.Fatal("retryable failure stopped probing")
	}
}

func TestR91AddendumForcedShapeAndUnattemptedPhase(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	a := r90Fixture(t, players, raw)
	a.lcuGameflowShapeDiagnosticKeys = map[string]struct{}{lcuSessionShapeDiagnosticKey("GameStart", "CHERRY", 1750): {}}
	for _, id := range []int{901, 901, 902} {
		a.recordLCUGameflowSessionShape([]byte(fmt.Sprintf(`{"gameData":{"gameId":%d,"teamOne":[{"puuid":"private","teamParticipantId":1}]}}`, id)), "GameStart", "CHERRY", 1750)
	}
	events := r90Events(t, a, "lcu_gameflow_session_shape")
	if len(events) != 2 || events[0]["forced_sample"] != true || events[0]["team_one_length"] != float64(1) {
		t.Fatal(events)
	}
	// Missing game IDs still receive one forced sample on each GameStart entry.
	for range 2 {
		a.observeArenaGroupingPhase(a.lcu, "ChampSelect")
		a.observeArenaGroupingPhase(a.lcu, "GameStart")
		a.recordLCUGameflowSessionShape([]byte(`{"gameData":{}}`), "GameStart", "CHERRY", 1750)
	}
	if len(r90Events(t, a, "lcu_gameflow_session_shape")) != 4 {
		t.Fatal("zero game ID suppressed a later Loading sample")
	}
	a.observeArenaGroupingPhase(a.lcu, "ChampSelect")
	a.noteArenaPhaseRoster(a.lcu, &gameplayLiveResponse{Phase: "ChampSelect", QueueID: 1750, GameID: 901})
	a.observeArenaGroupingPhase(a.lcu, "GameStart")
	a.observeArenaGroupingPhase(a.lcu, "InProgress")
	rejected := r90Events(t, a, "arena_group_order_rejected")
	if len(rejected) != 1 || rejected[0]["reason"] != "phase-not-attempted" {
		t.Fatal(rejected)
	}
	a.markArenaGroupingAttempt(a.lcu, &gameplayLiveResponse{Phase: "InProgress", QueueID: 1750, GameID: 901})
	a.observeArenaGroupingPhase(a.lcu, "Reconnect")
	if len(r90Events(t, a, "arena_group_order_rejected")) != 1 {
		t.Fatal("attempted phase misreported")
	}
}

func TestR91AddendumWildcardHoverClearKeepsArmedLock(t *testing.T) {
	for _, newID := range []int64{0, 104} {
		t.Run(strconv.FormatInt(newID, 10), func(t *testing.T) {
			f := newExecutionFixture(t)
			f.session.QueueID = 1750
			f.session.Timer.Phase = "BAN_PICK"
			s := f.r.currentWatch()
			g := s.ChampSelect.Groups["arena"]
			g.Ban.Enabled = true
			g.Ban.DelayMS = 0
			g.Ban.Strategy = "lock-now"
			g.Ban.Champions["default"] = []int64{141}
			s.ChampSelect.Groups["arena"] = g
			f.r.apply(s)
			f.tick(t) // wildcard must hover first
			s = f.r.currentWatch()
			g = s.ChampSelect.Groups["arena"]
			g.Ban.DelayMS = 120
			s.ChampSelect.Groups["arena"] = g
			f.r.apply(s)
			f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			f.r.mu.Lock()
			pending := f.r.pending["champselect-ban"]
			f.r.mu.Unlock()
			if pending == nil {
				t.Fatal("lock not armed")
			}
			f.mu.Lock()
			f.session.Actions[0][0].ChampionID = newID
			f.mu.Unlock()
			f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			waitTakeoverIdle(t, f)
			if newID == 0 {
				if f.count() != 2 || f.last().Body["completed"] != true {
					t.Fatalf("lock lost: %+v", f.patches)
				}
			} else if f.count() != 1 || f.r.champSelectSnapshot().BanStates["141"] != "manual-takeover" {
				t.Fatal("different nonzero choice ignored")
			}
			if newID == 0 {
				f.mu.Lock()
				found := false
				for _, e := range f.events {
					if e["reason"] == "hover-cleared-by-client" || e["result"] == "hover-cleared-by-client" {
						found = true
					}
				}
				f.mu.Unlock()
				if !found {
					t.Fatal("clear diagnostic missing")
				}
			}
		})
	}
}

func TestR91AddendumSaveReevaluatesCurrentPhase(t *testing.T) {
	for _, phase := range []string{"Reconnect", "ReadyCheck", "EndOfGame"} {
		t.Run(phase, func(t *testing.T) {
			runner := newWatchRunner(nil, nil)
			defer runner.apply(defaultWatchSettings())
			var reads atomic.Int32
			client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/lol-gameflow/v1/gameflow-phase" {
					reads.Add(1)
					return proHTTPBody([]byte(strconv.Quote(phase))), nil
				}
				return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			})}}
			runner.handlePhase(client, phase)
			settings := runner.currentWatch()
			settings.MasterEnabled = true
			settings.Rules.AutoReconnect.Enabled = true
			settings.Rules.AutoReconnect.DelayMS = 10000
			settings.Rules.AutoAccept.Enabled = true
			settings.Rules.AutoAccept.DelayMS = 10000
			settings.Rules.AutoPlayAgain.Enabled = true
			a := &app{watch: runner, connected: true, lcu: client, storage: trackTestStore(t, &localStore{root: t.TempDir()})}
			raw, _ := json.Marshal(settings)
			w := httptest.NewRecorder()
			a.handleWatchRules(w, httptest.NewRequest("POST", "/api/watch/rules", bytes.NewReader(raw)))
			action := map[string]string{"Reconnect": "reconnect", "ReadyCheck": "accept", "EndOfGame": "play-again"}[phase]
			runner.mu.Lock()
			armed := runner.pending[action] != nil
			runner.mu.Unlock()
			if w.Code != 200 || reads.Load() == 0 || !armed {
				t.Fatalf("status=%d reads=%d action=%s armed=%t", w.Code, reads.Load(), action, armed)
			}
		})
	}
}

func TestR91AddendumUpstreamRetryAfterSurvives(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "fixture")
	c := newChampionProvider()
	var calls atomic.Int32
	c.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := calls.Add(1)
		delay := "1"
		if n == 3 {
			delay = "30"
		}
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{delay}}, Body: io.NopCloser(strings.NewReader("quota"))}, nil
	})}
	p := newRiotProvider(c)
	var out any
	err := p.getLimited(context.Background(), riotClusterHost, "/fixture", nil, &out, 1024)
	var quota *riotStatusError
	if !errors.As(err, &quota) || quota.retryAfter != 30 || calls.Load() != 3 {
		t.Fatalf("calls=%d err=%+v", calls.Load(), err)
	}
	w := httptest.NewRecorder()
	writeRiotHTTPError(w, err)
	if w.Header().Get("Retry-After") != "30" {
		t.Fatal(w.Header())
	}
}

func TestR91AddendumRetryableRosterDoesNotBlockEarlyProbe(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	a := r90Fixture(t, players[:17], raw)
	var calls atomic.Int32
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { calls.Add(1); return nil, 0, errors.New("loading") }
	first := a.cachedGameplayLive(context.Background(), a.lcu, a.summoner, "InProgress")
	if !first.ArenaGroupingRetryable {
		t.Fatal("fixture not retryable")
	}
	a.liveSnapshots.mu.Lock()
	a.liveSnapshots.at = time.Now().Add(-4 * time.Second)
	a.liveSnapshots.mu.Unlock()
	a.liveClientProbeMu.Lock()
	a.liveClientProbe.LastAttempt = time.Now().Add(-4 * time.Second)
	a.liveClientProbeMu.Unlock()
	a.cachedGameplayLive(context.Background(), a.lcu, a.summoner, "InProgress")
	if calls.Load() != 2 {
		t.Fatal("20 second roster cache suppressed early probe", calls.Load())
	}
}

// Independent adversarial check for P9: saving watch settings while the
// current phase is ChampSelect (not Reconnect/ReadyCheck/EndOfGame - i.e. no
// rule should match this phase at all) must not spuriously arm ANY pending
// rule action, even though all three rules are enabled in the saved request.
// The R91-addendum ledger's own test only exercises the three phases where a
// rule SHOULD arm; this fills the negative-case gap so a regression that
// widens the phase-reevaluation switch (e.g. `case "Reconnect", "ChampSelect":`)
// is actually caught instead of silently passing.
func TestR91AddendumSaveDuringChampSelectArmsNoUnrelatedRule(t *testing.T) {
	phase := "ChampSelect"
	runner := newWatchRunner(nil, nil)
	defer runner.apply(defaultWatchSettings())
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/lol-gameflow/v1/gameflow-phase" {
			return proHTTPBody([]byte(strconv.Quote(phase))), nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}}
	runner.handlePhase(client, phase)
	settings := runner.currentWatch()
	settings.MasterEnabled = true
	settings.Rules.AutoReconnect.Enabled = true
	settings.Rules.AutoReconnect.DelayMS = 10000
	settings.Rules.AutoAccept.Enabled = true
	settings.Rules.AutoAccept.DelayMS = 10000
	settings.Rules.AutoPlayAgain.Enabled = true
	a := &app{watch: runner, connected: true, lcu: client, storage: trackTestStore(t, &localStore{root: t.TempDir()})}
	raw, _ := json.Marshal(settings)
	w := httptest.NewRecorder()
	a.handleWatchRules(w, httptest.NewRequest("POST", "/api/watch/rules", bytes.NewReader(raw)))
	runner.mu.Lock()
	reconnectArmed := runner.pending["reconnect"] != nil
	acceptArmed := runner.pending["accept"] != nil
	playAgainArmed := runner.pending["play-again"] != nil
	runner.mu.Unlock()
	if w.Code != 200 {
		t.Fatalf("save failed status=%d", w.Code)
	}
	if reconnectArmed || acceptArmed || playAgainArmed {
		t.Fatalf("REGRESSION: save during ChampSelect spuriously armed a rule: reconnect=%t accept=%t play-again=%t", reconnectArmed, acceptArmed, playAgainArmed)
	}
}
