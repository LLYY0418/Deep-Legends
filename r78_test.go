package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR78ChampSelectNormalizationEnforcesModeLimitsAndIDs(t *testing.T) {
	settings := defaultChampSelectSettings()
	ranked := settings.Groups["ranked"]
	ranked.Ban.Champions["middle"] = []int64{1, 2, 2, -1, 3, 4, 5, 6, 7, 8}
	ranked.Ban.Champions["made-up"] = []int64{99}
	ranked.Ban.DelayMS = -1
	ranked.Pick.DelayMS = 20000
	settings.Groups["ranked"] = ranked
	arena := settings.Groups["arena"]
	arena.Pick.Champions["default"] = []int64{-3, -3, -2, 10, 11, 12, 13}
	settings.Groups["arena"] = arena
	settings.Groups["unknown"] = champSelectGroupConfig{}

	normalized := normalizeChampSelectSettings(settings)
	if got := normalized.Groups["ranked"].Ban.Champions["default"]; len(got) != 5 || got[0] != 1 || got[4] != 5 {
		t.Fatalf("ranked ban pool = %#v", got)
	}
	if _, ok := normalized.Groups["ranked"].Ban.Champions["made-up"]; ok {
		t.Fatal("unknown position survived normalization")
	}
	if normalized.Groups["ranked"].Ban.DelayMS != 0 || normalized.Groups["ranked"].Pick.DelayMS != 10000 {
		t.Fatalf("delays were not clamped: ban=%d pick=%d", normalized.Groups["ranked"].Ban.DelayMS, normalized.Groups["ranked"].Pick.DelayMS)
	}
	if got := normalized.Groups["arena"].Pick.Champions["default"]; len(got) != 3 || got[0] != -3 || got[1] != 10 || got[2] != 11 {
		t.Fatalf("arena pick pool = %#v", got)
	}
	if _, ok := normalized.Groups["unknown"]; ok {
		t.Fatal("unknown group survived normalization")
	}
}

func TestR78RankedFallsBackToDefaultPoolWhenPositionUnknown(t *testing.T) {
	settings := defaultChampSelectSettings()
	group := settings.Groups["ranked"]
	group.Ban.Champions["default"] = []int64{9}
	group.Pick.Champions["default"] = []int64{10}
	group.Pick.Champions["middle"] = []int64{1}
	settings.Groups["ranked"] = group
	group = normalizeChampSelectSettings(settings).Groups["ranked"]
	definition, _ := champSelectGroupDefinitionFor("ranked")
	if len(definition.Positions) != 6 || definition.Positions[5] != "default" || champSelectPositionName("default") != "未分配位置" {
		t.Fatalf("ranked positions = %#v", definition.Positions)
	}
	for _, assigned := range []string{"", "FILL", "NONE", "unknown"} {
		t.Run(assigned, func(t *testing.T) {
			for side, config := range map[string]champSelectSideConfig{"ban": group.Ban, "pick": group.Pick} {
				want := int64(9)
				if side == "pick" {
					want = 10
				}
				pool, position := champSelectConfiguredPool(config, definition, assigned, side)
				if position != "default" || len(pool) != 1 || pool[0] != want {
					t.Fatalf("%s position=%q pool=%v", side, position, pool)
				}
			}
		})
	}
}

func TestR78RankedUnassignedPositionStillBans(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "ranked", "ban", 0, []int64{9}, map[int64]champSelectGridSelectionStatus{9: {}})
	fixture.session.MyTeam[0].AssignedPosition = ""
	fixture.enableLane("ranked", "ban", "default", []int64{9})
	if request := fixture.waitAfterEvaluate(t); request.ChampionID != 9 || request.Type != "ban" {
		t.Fatalf("ranked default PATCH = %#v", request)
	}
}

func TestR78MatchmakingQueuesUseNormalGroup(t *testing.T) {
	for _, queueID := range []int64{430, 480, 490} {
		if got := champSelectGroupID(queueID, "CLASSIC", 11); got != "normal" {
			t.Errorf("queue %d = %q, want normal", queueID, got)
		}
	}
}

func TestR78TradeCapabilityIndependentOfBench(t *testing.T) {
	settings := defaultChampSelectSettings()
	for id, group := range settings.Groups {
		group.Bench.Enabled, group.Bench.PreferFirst, group.Bench.HandleTrade = true, true, true
		settings.Groups[id] = group
	}
	normalized := normalizeChampSelectSettings(settings)
	for _, definition := range champSelectGroupDefinitions {
		group := normalized.Groups[definition.GroupID]
		wantTrade := definition.GroupID != "arena"
		if definition.HasTrade != wantTrade || group.Bench.HandleTrade != wantTrade {
			t.Errorf("%s trade capability=%v configured=%v", definition.GroupID, definition.HasTrade, group.Bench.HandleTrade)
		}
		if !definition.HasBench && group.Bench.Enabled {
			t.Errorf("%s retained unsupported bench controls", definition.GroupID)
		}
		if group.Bench.PreferFirst != (definition.HasBench || definition.HasTrade) {
			t.Errorf("%s lost exchange priority capability", definition.GroupID)
		}
	}
}

func TestR78PickableFailureStillSwapsBench(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "aram", "pick", 0, []int64{1}, nil)
	fixture.failPickableIDs = true
	fixture.session.BenchEnabled = true
	fixture.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 1}}
	fixture.enable("aram", "pick", []int64{1})
	settings := fixture.runner.currentWatch()
	group := settings.ChampSelect.Groups["aram"]
	group.Bench.Enabled, group.Bench.HoldMS = true, 1000
	settings.ChampSelect.Groups["aram"] = group
	fixture.runner.apply(settings)
	fixture.runner.mu.Lock()
	fixture.runner.champSelect.benchFirstSeen[1] = time.Now().Add(-2 * time.Second)
	fixture.runner.mu.Unlock()
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	select {
	case path := <-fixture.benchSwapCh:
		if path != "/lol-champ-select/v1/session/bench/swap/1" {
			t.Fatalf("bench POST = %q", path)
		}
	case <-time.After(time.Second):
		t.Fatal("pickable failure prevented bench POST")
	}
	if fixture.benchSwaps.Load() != 1 || fixture.patches.Load() != 0 {
		t.Fatalf("swaps=%d patches=%d", fixture.benchSwaps.Load(), fixture.patches.Load())
	}
}

func TestR78V2WatchMigrationPreservesRulesAndAddsDisabledChampSelect(t *testing.T) {
	root := t.TempDir()
	data := `{"schemaVersion":2,"masterEnabled":true,"rules":{"autoAccept":{"enabled":true,"delayMs":777},"autoReconnect":{"enabled":true,"delayMs":9000}}}`
	if err := os.WriteFile(filepath.Join(root, convenienceSettingsFile), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := loadWatchSettings(&localStore{root: root})
	if settings.SchemaVersion != 3 || !settings.Rules.AutoAccept.Enabled || settings.Rules.AutoAccept.DelayMS != 777 || settings.Rules.AutoReconnect.DelayMS != 9000 {
		t.Fatalf("migration changed existing rules: %#v", settings)
	}
	if settings.ChampSelect.Enabled || len(settings.ChampSelect.Groups) != len(champSelectGroupDefinitions) {
		t.Fatalf("champ select migration = %#v", settings.ChampSelect)
	}
	for id, group := range settings.ChampSelect.Groups {
		if group.Ban.Enabled || group.Pick.Enabled || group.Bench.HandleTrade || group.Bench.Enabled != (id == "aram") {
			t.Fatalf("%s was enabled by default: %#v", id, group)
		}
	}
}

func TestR78ChampSelectCandidateNormalizesLaneAndHonorsIntent(t *testing.T) {
	definition, _ := champSelectGroupDefinitionFor("normal")
	side := champSelectSideConfig{Champions: map[string][]int64{"default": {1, 2}}}
	pool, position := champSelectConfiguredPool(side, definition, "middle", "ban")
	if position != "default" || len(pool) != 2 {
		t.Fatalf("pool=%#v position=%q", pool, position)
	}
	grid := map[int64]champSelectGridChampion{
		1: {ID: 1, SelectionStatus: champSelectGridSelectionStatus{PickIntented: true}},
		2: {ID: 2},
	}
	available := champSelectIDSet([]int64{1, 2})
	if got, _ := chooseChampSelectCandidate("ban", pool, available, grid, true, false); got != 2 {
		t.Fatalf("avoid intent chose %d", got)
	}
	if got, _ := chooseChampSelectCandidate("ban", pool, available, grid, false, false); got != 1 {
		t.Fatalf("disabled avoid intent chose %d", got)
	}
}

func TestR78ChampSelectDelayNeverNegativeOrPastPhase(t *testing.T) {
	for _, fixture := range []struct{ configured, remaining, want int }{{2000, 500, 500}, {500, -1, 500}, {-3, 50, 0}, {500, 0, 0}} {
		if got := champSelectDelay(fixture.configured, fixture.remaining); got != fixture.want || got < 0 {
			t.Fatalf("delay(%d,%d)=%d want %d", fixture.configured, fixture.remaining, got, fixture.want)
		}
	}
}

func TestR78ChampSelectActionRequiresInProgress(t *testing.T) {
	cell := int64(7)
	session := lcuChampSelectSession{LocalPlayerCellID: &cell, Actions: [][]lcuChampSelectAction{{{ID: 1, ActorCellID: 7, Type: "pick", IsInProgress: false}}}}
	if _, ok := firstLocalChampSelectAction(session); ok {
		t.Fatal("inactive action was selected")
	}
	session.Actions[0][0].IsInProgress = true
	if action, ok := firstLocalChampSelectAction(session); !ok || action.ID != 1 {
		t.Fatalf("active action = %#v, %v", action, ok)
	}
}

func TestR78ChampSelectRequiresChampSelectPhase(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "normal", "pick", 0, []int64{1}, map[int64]champSelectGridSelectionStatus{1: {}})
	fixture.enable("normal", "pick", []int64{1})
	fixture.runner.handleChampSelectPhase("Lobby")
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	time.Sleep(30 * time.Millisecond)
	if fixture.patches.Load() != 0 || fixture.gridReads.Load() != 0 {
		t.Fatalf("outside ChampSelect read grid %d times and patched %d times", fixture.gridReads.Load(), fixture.patches.Load())
	}
}

func TestR78ChampSelectStrategiesAndTradePriority(t *testing.T) {
	last := champSelectSubmitRecord{ChampionID: 7, Completed: false}
	if champSelectActionCompleted("show-only", 7, 7, true, last) {
		t.Fatal("show-only unexpectedly locked")
	}
	if !champSelectActionCompleted("lock-now", 0, 7, false, champSelectSubmitRecord{}) {
		t.Fatal("lock-now did not lock")
	}
	if !champSelectActionCompleted("show-then-lock", 7, 7, true, last) {
		t.Fatal("show-then-lock did not enter its second PATCH")
	}
	pool := []int64{1, 2, 3}
	if !champSelectShouldAcceptTrade(pool, 99, 2, false) {
		t.Fatal("requester in pool should replace an out-of-pool champion")
	}
	if champSelectShouldAcceptTrade(pool, 2, 3, true) || champSelectShouldAcceptTrade(pool, 2, 1, false) {
		t.Fatal("trade priority accepted a worse or non-preferred swap")
	}
	if !champSelectShouldAcceptTrade(pool, 2, 1, true) || champSelectShouldAcceptTrade(pool, 2, 99, true) {
		t.Fatal("trade priority did not accept the earlier configured champion only")
	}
	bench := map[int64]lcuChampSelectBenchChampion{1: {ChampionID: 1}, 3: {ChampionID: 3}}
	if got := champSelectBenchTarget(pool, bench, 2, true); got != 1 {
		t.Fatalf("prefer-first bench target = %d", got)
	}
	if got := champSelectBenchTarget(pool, map[int64]lcuChampSelectBenchChampion{3: {ChampionID: 3}}, 2, true); got != 0 {
		t.Fatalf("prefer-first accepted a worse bench target %d", got)
	}
	if got := champSelectBenchTarget(pool, map[int64]lcuChampSelectBenchChampion{3: {ChampionID: 3}}, 2, false); got != 3 {
		t.Fatalf("non-priority bench target = %d", got)
	}
}

func TestR78ChampSelectSessionCacheClearsOnEnterAndLeave(t *testing.T) {
	runner := newWatchRunner(nil, nil)
	runner.champSelect.grid[1] = champSelectGridChampion{ID: 1}
	runner.champSelect.submitted[10] = champSelectSubmitRecord{ChampionID: 1}
	runner.handleChampSelectPhase("ChampSelect")
	if len(runner.champSelect.grid) != 0 || len(runner.champSelect.submitted) != 0 {
		t.Fatal("entering champion select retained a previous session")
	}
	runner.champSelect.grid[2] = champSelectGridChampion{ID: 2}
	runner.handleChampSelectPhase("InProgress")
	if len(runner.champSelect.grid) != 0 || runner.champSelect.active {
		t.Fatal("leaving champion select retained runtime state")
	}
}

func TestR78ChampSelectExhaustedEmitsAndDoesNotPatch(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "normal", "ban", 0, []int64{}, map[int64]champSelectGridSelectionStatus{1: {IsBanned: true}})
	fixture.enable("normal", "ban", []int64{1})
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	select {
	case event := <-fixture.events:
		if event != "watch:failed:champselect-ban:exhausted" {
			t.Fatalf("event = %q", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing exhausted event")
	}
	if fixture.patches.Load() != 0 {
		t.Fatalf("patches = %d", fixture.patches.Load())
	}
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	if fixture.gridReads.Load() != 2 {
		t.Fatalf("empty all-grid snapshot was read %d times", fixture.gridReads.Load())
	}
}

func TestR78ChampSelectSubmissionDeduplicatesAndUsesSecondIntentSafeChoice(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "normal", "ban", 0, []int64{1, 2}, map[int64]champSelectGridSelectionStatus{1: {PickIntented: true}, 2: {}})
	fixture.enable("normal", "ban", []int64{1, 2})
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	request := fixture.waitPatch(t)
	if request.ChampionID != 2 || request.Completed {
		t.Fatalf("patch = %#v", request)
	}
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	time.Sleep(30 * time.Millisecond)
	if fixture.patches.Load() != 1 {
		t.Fatalf("duplicate patches = %d", fixture.patches.Load())
	}
	if fixture.gridReads.Load() != 3 { // Two evaluations plus a fresh preflight.
		t.Fatalf("all-grid reads = %d", fixture.gridReads.Load())
	}
}

func TestR78ChampSelectUserTakeoverCancelsAutomation(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "normal", "pick", 99, []int64{1}, map[int64]champSelectGridSelectionStatus{1: {}})
	fixture.enable("normal", "pick", []int64{1})
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	select {
	case event := <-fixture.events:
		if event != "watch:canceled:champselect-pick" {
			t.Fatalf("event = %q", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing takeover cancellation")
	}
	if fixture.patches.Load() != 0 {
		t.Fatalf("patches = %d", fixture.patches.Load())
	}
}

func TestR78ARAMNeverSubmitsBan(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "aram", "ban", 0, []int64{1}, map[int64]champSelectGridSelectionStatus{1: {}})
	settings := fixture.runner.currentWatch().ChampSelect
	settings.Enabled = true
	group := settings.Groups["aram"]
	group.Ban.Enabled = true
	group.Ban.Champions["default"] = []int64{1}
	settings.Groups["aram"] = group
	fixture.runner.evaluateChampSelect(fixture.client, settings)
	time.Sleep(30 * time.Millisecond)
	if fixture.patches.Load() != 0 {
		t.Fatalf("ARAM ban patches = %d", fixture.patches.Load())
	}
}

func TestR78ArenaWithoutBanActionStillPicks(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "arena", "pick", 0, []int64{1}, map[int64]champSelectGridSelectionStatus{1: {}})
	fixture.enable("arena", "pick", []int64{1})
	if request := fixture.waitAfterEvaluate(t); request.ChampionID != 1 {
		t.Fatalf("arena pick PATCH = %#v", request)
	}
	if fixture.bannableReads.Load() != 0 {
		t.Fatalf("arena session without ban actions read bannable endpoint %d times", fixture.bannableReads.Load())
	}
}

func TestR78CustomPracticeChampSelectUsesItsOwnPauseGate(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "practice", "pick", 0, []int64{1}, map[int64]champSelectGridSelectionStatus{1: {}})
	fixture.enable("practice", "pick", []int64{1})
	fixture.runner.mu.Lock()
	fixture.runner.customSession = true
	fixture.runner.mu.Unlock()
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	if request := fixture.waitPatch(t); request.ChampionID != 1 {
		t.Fatalf("custom practice PATCH = %#v", request)
	}
}

type r78PatchRequest struct {
	ChampionID int64  `json:"championId"`
	Completed  bool   `json:"completed"`
	Type       string `json:"type"`
}

type r78ChampSelectFixture struct {
	session         *lcuChampSelectSession
	failPickableIDs bool
	benchSwapCh     chan string
	benchSwaps      atomic.Int32
	runner          *watchRunner
	client          *LCUClient
	events          chan string
	patchCh         chan r78PatchRequest
	patches         atomic.Int32
	gridReads       atomic.Int32
	bannableReads   atomic.Int32
}

func (fixture *r78ChampSelectFixture) enable(groupID, side string, champions []int64) {
	fixture.enableLane(groupID, side, "default", champions)
}

func (fixture *r78ChampSelectFixture) enableLane(groupID, side, lane string, champions []int64) {
	settings := fixture.runner.currentWatch()
	settings.ChampSelect.Enabled = true
	group := settings.ChampSelect.Groups[groupID]
	config := &group.Pick
	if side == "ban" {
		config = &group.Ban
	}
	config.Enabled = true
	config.DelayMS = 0
	config.Champions[lane] = append([]int64(nil), champions...)
	settings.ChampSelect.Groups[groupID] = group
	fixture.runner.apply(settings)
}

func (fixture *r78ChampSelectFixture) waitPatch(t *testing.T) r78PatchRequest {
	t.Helper()
	select {
	case request := <-fixture.patchCh:
		return request
	case <-time.After(time.Second):
		t.Fatal("missing PATCH")
		return r78PatchRequest{}
	}
}

func (fixture *r78ChampSelectFixture) waitAfterEvaluate(t *testing.T) r78PatchRequest {
	t.Helper()
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	return fixture.waitPatch(t)
}

func newR78ChampSelectFixture(t *testing.T, groupID, actionType string, actionChampionID int64, available []int64, statuses map[int64]champSelectGridSelectionStatus) *r78ChampSelectFixture {
	t.Helper()
	queueID := int64(400)
	switch groupID {
	case "ranked":
		queueID = 440
	case "aram":
		queueID = 450
	case "practice":
		queueID = 3100
	case "arena":
		queueID = 1700
	}
	cell := int64(7)
	session := lcuChampSelectSession{
		QueueID: queueID, LocalPlayerCellID: &cell,
		Actions: [][]lcuChampSelectAction{{{ID: 42, ActorCellID: cell, Type: actionType, ChampionID: actionChampionID, IsInProgress: true}}},
		MyTeam:  []lcuChampSelectPlayer{{CellID: &cell, AssignedPosition: "middle"}},
		Timer:   lcuChampSelectTimer{Phase: "BAN_PICK", AdjustedTimeLeftInPhase: 5000, TotalTimeInPhase: 30000},
	}
	grid := make([]champSelectGridChampion, 0, len(statuses))
	for id, status := range statuses {
		grid = append(grid, champSelectGridChampion{ID: id, Owned: true, SelectionStatus: status})
	}
	gridJSON, _ := json.Marshal(grid)
	availableJSON, _ := json.Marshal(available)
	events := make(chan string, 16)
	fixture := &r78ChampSelectFixture{session: &session, events: events, patchCh: make(chan r78PatchRequest, 8), benchSwapCh: make(chan string, 8)}
	fixture.runner = newWatchRunner(nil, func(event string) {
		if strings.HasPrefix(event, "watch:failed:champselect-") || strings.HasPrefix(event, "watch:canceled:champselect-") {
			events <- event
		}
	})
	fixture.runner.handleChampSelectPhase("ChampSelect")
	fixture.client = &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := "{}"
		status := http.StatusOK
		switch request.URL.Path {
		case "/lol-champ-select/v1/session":
			sessionJSON, _ := json.Marshal(fixture.session)
			body = string(sessionJSON)
		case "/lol-champ-select/v1/all-grid-champions":
			fixture.gridReads.Add(1)
			body = string(gridJSON)
		case "/lol-champ-select/v1/pickable-champion-ids":
			body = string(availableJSON)
			if fixture.failPickableIDs {
				status = http.StatusInternalServerError
			}
		case "/lol-champ-select/v1/bannable-champion-ids":
			fixture.bannableReads.Add(1)
			body = string(availableJSON)
		default:
			if request.Method == http.MethodPatch && strings.HasPrefix(request.URL.Path, "/lol-champ-select/v1/session/actions/") {
				var payload r78PatchRequest
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Errorf("decode PATCH: %v", err)
				}
				fixture.patches.Add(1)
				fixture.patchCh <- payload
				status = http.StatusNoContent
				body = ""
			} else if request.Method == http.MethodPost && strings.HasPrefix(request.URL.Path, "/lol-champ-select/v1/session/bench/swap/") {
				fixture.benchSwaps.Add(1)
				fixture.benchSwapCh <- request.URL.Path
				status = http.StatusNoContent
				body = ""
			} else {
				t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
				status = http.StatusNotFound
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}}
	return fixture
}
