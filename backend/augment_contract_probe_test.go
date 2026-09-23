package main

// ============================================================================
// R116-探测：一次性侦察代码的测试（TEMPORARY — 与 augment_contract_probe.go 同时删除）
//
// 覆盖三件事：
//  1. augmentProbePath 谓词的匹配/不匹配用例（含大小写混合）；
//  2. 工单「对抗变异」：fixture openapi.json 里有 /lol-cherry-augments/v1/select
//     时 count > 0 且该 path 被记录；删掉它 count 归零——证明探测不是死代码；
//  3. gameplay.go 新增的 `aramMode && !arenaMode` 触发分支：海斗触发
//     sampleArenaAllGameData，斗魂仍走原分支，经典模式与 ChampSelect 不触发。
//
// 真机入口 TestR116AugmentContractProbeLiveClient 默认 Skip，只有在装了 League
// 客户端的本机上显式设置 R116_AUGMENT_PROBE=1 才会执行。
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. 谓词
// ---------------------------------------------------------------------------

func TestR116AugmentProbePathPredicate(t *testing.T) {
	match := []string{
		"/lol-cherry-augments/v1/select",
		"/lol-CHERRY/v1/x",
		"/lol-Cherry/v1/player/augments",
		"/lol-mayhem-foo",
		"/lol-augment-bar",
		"/lol-game-data/assets/v1/cherry-augments.json",
		"/lol-champ-select/v1/augment-pick",
		"/lol-kiwi-mayhem/v2/offer",
		"/cherry/v1/augment",
		"/MAYHEM/offer",
	}
	for _, path := range match {
		if !augmentProbeMatches(path) {
			t.Errorf("must match: %s", path)
		}
		if !augmentProbePath.MatchString(path) {
			t.Errorf("regexp itself must match: %s", path)
		}
	}
	reject := []string{
		"/lol-missions/v1/x",
		"/lol-missions/v1/missions",
		"/lol-missions/v1/player",
		"/lol-notifications/v1/notifications",
		"/lol-event-hub/v1/events",
		"/lol-champ-select/v1/session",
		"/lol-gameflow/v1/session",
		"/lol-summoner/v1/current-summoner",
		"/swagger/v3/openapi.json",
		"/help",
		"",
		"/" + strings.Repeat("a", 600) + "/cherry",
	}
	for _, path := range reject {
		if augmentProbeMatches(path) {
			t.Errorf("must not match: %s", path)
		}
	}
	// 谓词只看路径本体，query 里的关键字不制造自我命中（探测自身请求 /help?format=Full）。
	if augmentProbeMatches("/help?format=Full&augment=1") {
		t.Error("query string must not drive the predicate")
	}
	// 独立于任务徽章护栏：objectiveDiagnosticPath 的既有语义不能被本次探测改变。
	if objectiveDiagnosticPath("/lol-cherry-augments/v1/select") != "" {
		t.Error("objectiveDiagnosticPath must stay scoped to missions/notifications/event-hub")
	}
	if objectiveDiagnosticPath("/lol-missions/v1/player/ab") != "/lol-missions/v1/player/{id}" {
		t.Error("objectiveDiagnosticPath regression")
	}
}

// ---------------------------------------------------------------------------
// 2. fixture 与对抗变异
// ---------------------------------------------------------------------------

// r116OpenAPIFixture 构造一份最小 openapi 文档。withCherry 为真时放入工单
// 「对抗变异」要求的 /lol-cherry-augments/v1/select；为假时同一份文档里没有它。
func r116OpenAPIFixture(withCherry bool) []byte {
	operation := func(extra map[string]any) map[string]any {
		base := map[string]any{"responses": map[string]any{"200": map[string]any{}, "404": map[string]any{}}}
		for key, value := range extra {
			base[key] = value
		}
		return base
	}
	paths := map[string]any{
		"/lol-missions/v1/missions":           map[string]any{"get": operation(nil)},
		"/lol-missions/v1/player":             map[string]any{"patch": operation(nil)},
		"/lol-notifications/v1/notifications": map[string]any{"get": operation(nil)},
		"/lol-champ-select/v1/session":        map[string]any{"get": operation(nil)},
		"/lol-gameflow/v1/session":            map[string]any{"get": operation(nil)},
		"/lol-game-data/assets/v1/perks.json": map[string]any{"get": operation(nil)},
	}
	if withCherry {
		paths["/lol-cherry-augments/v1/select"] = map[string]any{
			"get": operation(map[string]any{
				"parameters": []any{
					map[string]any{"name": "stage", "in": "query", "required": true, "schema": map[string]any{"type": "integer"}},
					map[string]any{"name": "puuid", "in": "query", "schema": map[string]any{"type": "string"}},
				},
			}),
			"post": operation(map[string]any{
				"requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/AugmentPick"},
				}}},
			}),
		}
		paths["/lol-mayhem/v1/offer"] = map[string]any{"get": operation(nil)}
	}
	root := map[string]any{
		"openapi": "3.0.0",
		"paths":   paths,
		"components": map[string]any{"schemas": map[string]any{
			"AugmentPick": map[string]any{"type": "object", "properties": map[string]any{
				"augmentId": map[string]any{"type": "integer"},
				"reroll":    map[string]any{"type": "boolean"},
			}},
		}},
	}
	raw, err := json.Marshal(root)
	if err != nil {
		panic(err)
	}
	return raw
}

// r116ProbeServer 模拟本机 LCU：openapi 可 200/404，/help 可带或不带海克斯路径。
// 任何非 GET 请求都会让测试失败（探测必须只读）。
func r116ProbeServer(t *testing.T, openapi []byte, openapiStatus int, help []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	writes := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
			t.Errorf("probe must be read-only, got %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/swagger/v3/openapi.json":
			if openapiStatus != http.StatusOK {
				http.Error(w, `{"message":"not found"}`, openapiStatus)
				return
			}
			_, _ = w.Write(openapi)
		case "/swagger/v2/swagger.json":
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case "/help":
			if len(help) == 0 {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write(help)
		default:
			http.Error(w, `{"message":"not available in R116 fixture"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server, writes
}

func r116ProbeApp(t *testing.T, server *httptest.Server) (*app, *localStore) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	return &app{storage: store}, store
}

func r116ProbeEvents(t *testing.T, store *localStore, event string) []map[string]any {
	t.Helper()
	raw, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	events := []map[string]any{}
	for _, line := range strings.Split(string(raw), "\n") {
		var parsed map[string]any
		if json.Unmarshal([]byte(line), &parsed) == nil && parsed["event"] == event {
			events = append(events, parsed)
		}
	}
	return events
}

func r116ProbeBySource(events []map[string]any, kind string) map[string]any {
	for _, event := range events {
		if event["contract_kind"] == kind {
			return event
		}
	}
	return nil
}

// TestR116AugmentContractProbeMutation 是工单 P0「对抗变异」要求的正反两跑：
// fixture 里有 /lol-cherry-augments/v1/select → count > 0 且该 path 被记录；
// 把它删掉 → count == 0。证明探测不是死代码。
func TestR116AugmentContractProbeMutation(t *testing.T) {
	for _, withCherry := range []bool{true, false} {
		t.Run("cherry="+strconv.FormatBool(withCherry), func(t *testing.T) {
			server, writes := r116ProbeServer(t, r116OpenAPIFixture(withCherry), http.StatusOK, nil)
			a, store := r116ProbeApp(t, server)
			client := &LCUClient{baseURL: server.URL, token: "private-token", http: server.Client()}
			returned, ok := a.collectAugmentContractProbe(context.Background(), client, "unit-test")
			if !ok || len(returned) == 0 {
				t.Fatal("probe did not run")
			}
			events := r116ProbeEvents(t, store, "augment_contract_probe")
			openapi := r116ProbeBySource(events, "openapi-v3")
			if openapi == nil {
				t.Fatalf("missing openapi-v3 event: %v", events)
			}
			count := int(openapi["count"].(float64))
			if withCherry {
				if count == 0 {
					t.Fatalf("dead probe: cherry fixture produced count=0: %v", openapi)
				}
				if count != 3 { // GET+POST /lol-cherry-augments/v1/select + GET /lol-mayhem/v1/offer
					t.Fatalf("count = %d, want 3: %v", count, openapi)
				}
				encoded, _ := json.Marshal(openapi["operations"])
				for _, evidence := range []string{`/lol-cherry-augments/v1/select`, `"method":"GET"`, `"method":"POST"`, `/lol-mayhem/v1/offer`, `"parameters"`, `"stage"`, `"body"`, `"augmentId"`, `"reroll"`} {
					if !strings.Contains(string(encoded), evidence) {
						t.Fatalf("missing %s in %s", evidence, encoded)
					}
				}
				if openapi["matched_paths"].(float64) != 2 || openapi["scanned_paths"].(float64) != 8 {
					t.Fatalf("scan counters wrong: %v", openapi)
				}
				if openapi["negative_conclusive"] != false || openapi["contract_read"] != true {
					t.Fatalf("judgement flags wrong: %v", openapi)
				}
			} else {
				if count != 0 {
					t.Fatalf("count must be zero without the cherry path, got %d: %v", count, openapi)
				}
				if openapi["contract_read"] != true || openapi["negative_conclusive"] != true {
					t.Fatalf("a readable contract with zero hits must be conclusive: %v", openapi)
				}
				if int(openapi["scanned_paths"].(float64)) != 6 {
					t.Fatalf("full scan must still see every unrelated path: %v", openapi)
				}
				encoded, _ := json.Marshal(openapi["operations"])
				if strings.Contains(string(encoded), "cherry") || strings.Contains(string(encoded), "mayhem") {
					t.Fatalf("phantom hit: %s", encoded)
				}
			}
			// 探测绝不能把任务徽章命名空间收进来（谓词独立于 objectiveDiagnosticPath）。
			encoded, _ := json.Marshal(events)
			for _, unrelated := range []string{"/lol-missions/v1/", "/lol-notifications/v1/", "private-token"} {
				if strings.Contains(string(encoded), unrelated) {
					t.Fatalf("probe leaked unrelated namespace or credential %s", unrelated)
				}
			}
			if writes.Load() != 0 {
				t.Fatal("probe wrote to the client")
			}
			for _, event := range events {
				if event["write_executed"] != false {
					t.Fatalf("write_executed must stay false: %v", event)
				}
			}
			summary := r116ProbeEvents(t, store, "augment_contract_probe_summary")
			if len(summary) != 1 {
				t.Fatalf("missing summary: %v", summary)
			}
			if summary[0]["negative_conclusive_all"] != (withCherry == false) {
				t.Fatalf("summary judgement wrong: %v", summary[0])
			}
		})
	}
}

// TestR116AugmentContractProbeUnreadableContractIsNotConclusive 复现 R82 的真机观测：
// /swagger/v3/openapi.json 与 v2 都 404，只有 /help?format=Full 返回 200。
// 此时 count == 0 绝不能被读成「确证客户端没有海克斯端点」。
func TestR116AugmentContractProbeUnreadableContractIsNotConclusive(t *testing.T) {
	help := []byte(`{"functions":[{"uri":"/lol-champ-select/v1/session"},{"uri":"/lol-cherry-augments/v1/select"},{"uri":"/lol-missions/v1/missions"}]}`)
	server, writes := r116ProbeServer(t, r116OpenAPIFixture(true), http.StatusNotFound, help)
	a, store := r116ProbeApp(t, server)
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	if _, ok := a.collectAugmentContractProbe(context.Background(), client, "unit-test"); !ok {
		t.Fatal("probe did not run")
	}
	events := r116ProbeEvents(t, store, "augment_contract_probe")
	openapi := r116ProbeBySource(events, "openapi-v3")
	if openapi["status"].(float64) != 404 || openapi["result"] != "unavailable" {
		t.Fatalf("404 not recorded: %v", openapi)
	}
	if openapi["contract_read"] != false || openapi["negative_conclusive"] != false {
		t.Fatalf("404 must never be conclusive: %v", openapi)
	}
	if _, present := openapi["count"]; present {
		t.Fatalf("unreadable source must not publish a count: %v", openapi)
	}
	fallback := r116ProbeBySource(events, "help-full")
	if fallback == nil {
		t.Fatalf("missing help fallback event: %v", events)
	}
	if int(fallback["count"].(float64)) != 1 {
		t.Fatalf("format-independent fallback missed the cherry path: %v", fallback)
	}
	matched, _ := json.Marshal(fallback["matched_paths"])
	if !strings.Contains(string(matched), "/lol-cherry-augments/v1/select") {
		t.Fatalf("matched path not recorded: %s", matched)
	}
	if strings.Contains(string(matched), "/lol-missions/v1/missions") {
		t.Fatalf("unrelated namespace recorded: %s", matched)
	}
	if fallback["root_keys"] == nil || fallback["json_root"] != "object" {
		t.Fatalf("root shape not recorded (R82 4.2-1 requires shape before guessing): %v", fallback)
	}
	summary := r116ProbeEvents(t, store, "augment_contract_probe_summary")
	if summary[0]["contract_read_any"] != true || summary[0]["negative_conclusive_all"] != false {
		t.Fatalf("summary must reflect the readable fallback: %v", summary[0])
	}
	if writes.Load() != 0 {
		t.Fatal("probe wrote to the client")
	}
}

// TestR116AugmentContractProbeZeroEverywhereIsConclusive 是「确证无端点」这条判据的
// 正向样例：三份来源都可读且零命中时，才允许写 negative_conclusive_all。
func TestR116AugmentContractProbeZeroEverywhereIsConclusive(t *testing.T) {
	help := []byte(`{"functions":[{"uri":"/lol-missions/v1/missions"},{"uri":"/lol-champ-select/v1/session"}]}`)
	server, _ := r116ProbeServer(t, r116OpenAPIFixture(false), http.StatusOK, help)
	a, store := r116ProbeApp(t, server)
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	if _, ok := a.collectAugmentContractProbe(context.Background(), client, "unit-test"); !ok {
		t.Fatal("probe did not run")
	}
	summary := r116ProbeEvents(t, store, "augment_contract_probe_summary")
	if len(summary) != 1 || summary[0]["negative_conclusive_all"] != true {
		t.Fatalf("readable zero-hit contract must be conclusive: %v", summary)
	}
	events := r116ProbeEvents(t, store, "augment_contract_probe")
	if openapi := r116ProbeBySource(events, "openapi-v2"); openapi["contract_read"] != false {
		t.Fatalf("404 source must not count as read: %v", openapi)
	}
}

// TestR116AugmentProbeTextScanIsBounded 保证 3 MB 级契约文本的兜底扫描有硬上限，
// 且明细被截断时 count 仍然是全量命中数（判据不被截断污染）。
func TestR116AugmentProbeTextScanIsBounded(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(`{"functions":[`)
	for i := 0; i < augmentProbeTextPathLimit+40; i++ {
		if i > 0 {
			builder.WriteString(",")
		}
		fmt.Fprintf(&builder, `{"uri":"/lol-augment-%d/v1/offer"}`, i)
	}
	builder.WriteString(`]}`)
	matched, stats := augmentProbeTextPaths([]byte(builder.String()))
	if stats.MatchedPaths != augmentProbeTextPathLimit+40 {
		t.Fatalf("count must stay complete under truncation: %d", stats.MatchedPaths)
	}
	if len(matched) != augmentProbeTextPathLimit || !stats.Truncated {
		t.Fatalf("retained list must be bounded: %d truncated=%t", len(matched), stats.Truncated)
	}
	if _, over := augmentProbeTextPaths(make([]byte, 0)); over.Tokens != 0 {
		t.Fatal("empty body must scan nothing")
	}
}

// ---------------------------------------------------------------------------
// 3. gameplay.go 的 aramMode 触发分支
// ---------------------------------------------------------------------------

func r116LiveFixturePlayers(count int) []map[string]any {
	players := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		players = append(players, map[string]any{
			"puuid":              fmt.Sprintf("r116-player-%02d", i+1),
			"championId":         i + 1,
			"cellId":             i,
			"teamParticipantId":  i + 1,
			"gameName":           fmt.Sprintf("R116P%02d", i+1),
			"tagLine":            "CN1",
			"summonerName":       "",
			"nameVisibilityType": "UNHIDDEN",
		})
	}
	return players
}

// r116LiveFixture 是一个只服务 gameflow/champ-select 的假 LCU，并把
// liveclientdata 的两个钩子都替换成本地计数器，用来观测 sampleArenaAllGameData
// 是否真的被触发（可观测副作用：钩子调用 + gameID 去重 map + 诊断事件）。
func r116LiveFixture(t *testing.T, gameMode string, queueID, mapID, gameID int64, players int) (*app, *atomic.Int32) {
	t.Helper()
	roster := r116LiveFixturePlayers(players)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{
				"gameId":  gameID,
				"queue":   map[string]any{"id": queueID, "mapId": mapID, "gameMode": gameMode},
				"teamOne": roster,
				"teamTwo": []any{},
			}})
		case "/lol-champ-select/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"gameId": gameID, "queueId": queueID, "localPlayerCellId": 0,
				"myTeam": roster, "theirTeam": []any{},
			})
		case "/lol-lobby/v2/lobby":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameConfig": map[string]any{
				"queueId": queueID, "mapId": mapID, "gameMode": gameMode,
			}})
		default:
			http.Error(w, "not available in R116 fixture", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	calls := &atomic.Int32{}
	a := &app{
		connected:            true,
		lcu:                  &LCUClient{baseURL: server.URL, token: "test", http: server.Client()},
		summoner:             Summoner{PUUID: "r116-player-01"},
		storage:              trackTestStore(t, &localStore{root: root}),
		liveClientPlayerList: func(context.Context) ([]byte, int, error) { return []byte(`[]`), http.StatusOK, nil },
		liveClientAllGameData: func(context.Context) ([]byte, int, error) {
			calls.Add(1)
			return []byte(`{"allPlayers":[{"items":[{"itemID":1,"slot":0,"count":1}]}],"activePlayer":{"level":7}}`), http.StatusOK, nil
		},
	}
	return a, calls
}

// r116WaitForSampling 等待 `go a.sampleArenaAllGameData(...)` 落地。触发是异步的，
// 因此正向断言以「诊断日志里出现该 gameID 的 live_client_allgamedata_shape」为准，
// 不能只看钩子计数（去重键先写、事件后写）；反向断言在有界窗口内确认钩子与去重表
// 都没有被写入。两个可观测副作用都来自既有实现，未改动 arena_truth_diagnostics.go。
func r116WaitForSampling(t *testing.T, a *app, calls *atomic.Int32, gameID int64, want bool) []map[string]any {
	t.Helper()
	key := strconv.FormatInt(gameID, 10)
	claimed := func() bool {
		a.liveClientAllGameDataMu.Lock()
		defer a.liveClientAllGameDataMu.Unlock()
		_, ok := a.liveClientAllGameDataKeys[key]
		return ok
	}
	if !want {
		deadline := time.Now().Add(750 * time.Millisecond)
		for time.Now().Before(deadline) {
			if calls.Load() > 0 || claimed() {
				t.Fatalf("gameID %s was sampled but must not be (calls=%d)", key, calls.Load())
			}
			time.Sleep(5 * time.Millisecond)
		}
		return nil
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		events := r90Events(t, a, "live_client_allgamedata_shape")
		if len(events) > 0 {
			if !claimed() {
				t.Fatalf("shape event recorded without claiming gameID %s", key)
			}
			return events
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("sampleArenaAllGameData never produced a shape event for gameID %s (calls=%d claimed=%t)", key, calls.Load(), claimed())
	return nil
}

func TestR116AramModeTriggersArenaAllGameDataSampling(t *testing.T) {
	cases := []struct {
		name     string
		gameMode string
		queueID  int64
		mapID    int64
		phase    string
		players  int
		want     bool
	}{
		{"海斗 ARAM_MAYHEM InProgress", "ARAM_MAYHEM", 1900, 12, "InProgress", 10, true},
		{"海斗 KIWI GameStart", "KIWI", 1900, 12, "GameStart", 10, true},
		{"海斗 ARAM_MAYHEM Reconnect", "ARAM_MAYHEM", 1900, 12, "Reconnect", 10, true},
		{"普通大乱斗 ARAM 也扩大覆盖", "ARAM", 450, 12, "InProgress", 10, true},
		{"海斗 ChampSelect 不触发", "ARAM_MAYHEM", 1900, 12, "ChampSelect", 10, false},
		{"经典模式不触发", "CLASSIC", 420, 11, "InProgress", 10, false},
		{"斗魂仍走原 arena 分支", "CHERRY", 1750, 30, "InProgress", 8, true},
	}
	for index, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// 每个子测试有独立的 app 与去重表；gameID 只用来把事件与本次触发对上。
			gameID := int64(91601 + index)
			a, calls := r116LiveFixture(t, testCase.gameMode, testCase.queueID, testCase.mapID, gameID, testCase.players)
			response := a.loadGameplayLive(context.Background(), a.lcu, a.summoner, testCase.phase)
			if !response.Available {
				t.Fatalf("fixture not available: %+v", response.Capabilities)
			}
			if response.GameMode != testCase.gameMode {
				t.Fatalf("gameMode = %q, want %q", response.GameMode, testCase.gameMode)
			}
			events := r116WaitForSampling(t, a, calls, gameID, testCase.want)
			if !testCase.want {
				if len(events) != 0 {
					t.Fatalf("unexpected shape events: %v", events)
				}
				return
			}
			if len(events) != 1 {
				t.Fatalf("expected exactly one shape event, got %d: %v", len(events), events)
			}
			if int(events[0]["game_id"].(float64)) != int(gameID) || events[0]["success"] != true {
				t.Fatalf("shape event wrong: %v", events[0])
			}
			encoded, _ := json.Marshal(events[0])
			if !strings.Contains(string(encoded), "items") || !strings.Contains(string(encoded), "itemID") {
				t.Fatalf("shape lost the item fields the probe exists for: %s", encoded)
			}
			if testCase.gameMode == "CHERRY" {
				// 斗魂必须仍由既有 arena 分支驱动：小队提示只在 if arenaMode 里赋值，
				// 而海斗分支要求 !arenaMode，两者互斥，能证明触发来自原分支。
				if response.ArenaMySquadNotice == "" {
					t.Fatal("arena branch no longer runs for CHERRY")
				}
			}
		})
	}
}

// TestR116AramBranchSourceGuardForArenaSampling 是「对抗变异」的源码侧护栏：工单要求把判断写成
// !aramMode 时测试必须失败。行为测试已经会因为海斗不触发而失败，这里再加一道
// 文本护栏，避免有人把变异写成同样能触发的等价形式而悄悄改变语义。
func TestR116AramBranchSourceGuardForArenaSampling(t *testing.T) {
	data, err := os.ReadFile("gameplay.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	required := "if aramMode && !arenaMode && (phase == \"GameStart\" || phase == \"InProgress\" || phase == \"Reconnect\") {\n\t\tgo a.sampleArenaAllGameData(ctx, response.GameID)\n\t}"
	if !strings.Contains(source, required) {
		t.Error("gameplay.go lost the R116 aramMode sampling branch (or it was mutated)")
	}
	for _, mutation := range []string{"if !aramMode && !arenaMode", "if aramMode && arenaMode"} {
		if strings.Contains(source, mutation) {
			t.Errorf("mutation survived in gameplay.go: %s", mutation)
		}
	}
	// 既有 arena 分支一行都不许改。
	arena := "if arenaMode && (phase == \"GameStart\" || phase == \"InProgress\" || phase == \"Reconnect\") {"
	if strings.Count(source, arena) != 1 {
		t.Error("existing arena branch was modified")
	}
}

// ---------------------------------------------------------------------------
// 4. 真机临时入口（仅本机、需显式开启；探测结论落盘后随文件一起删除）
// ---------------------------------------------------------------------------

// TestR116AugmentContractProbeLiveClient 是工单 P0 第 3 条要求的「仅本机可触发的临时入口」。
//
// ⚠️ 一次性侦察代码：真机探测执行完、结论写进 docs/r116-probe-findings.md 之后，
// 本测试与 augment_contract_probe.go 必须整体删除，不得进入正式版本。
//
// 用法（在装了 League 客户端并已登录的 Windows 机器上，仓库根目录执行）：
//
//	R116_AUGMENT_PROBE=1 R116_AUGMENT_PROBE_OUTPUT=docs/r116-validation/augment-contract-probe.jsonl \
//	  go test ./backend -run TestR116AugmentContractProbeLiveClient -v -timeout 5m
//
// 可选：R116_AUGMENT_PROBE_LOCKFILE=<lockfile 绝对路径>（客户端发现失败时手工指定）。
// 只发 GET，只读契约文档，不写客户端、不上传任何账号数据。
func TestR116AugmentContractProbeLiveClient(t *testing.T) {
	if os.Getenv("R116_AUGMENT_PROBE") != "1" {
		t.Skip("opt-in local LCU reconnaissance: set R116_AUGMENT_PROBE=1 on the machine running the League client")
	}
	client, report, err := discoverLCUDetailed()
	if err != nil {
		t.Logf("LCU discovery failed (%s / %s): %v", report.Result, report.Detail, err)
		if lockfile := strings.TrimSpace(os.Getenv("R116_AUGMENT_PROBE_LOCKFILE")); lockfile != "" {
			client, err = clientFromLockfile(lockfile)
			if err != nil {
				t.Fatalf("lockfile fallback failed: %v", err)
			}
			client.source = "lockfile"
		} else {
			t.Fatal("客户端未连接：请先登录 League 客户端，或用 R116_AUGMENT_PROBE_LOCKFILE 指定 lockfile")
		}
	}
	defer client.Close()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{connected: true, lcu: client, storage: store}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	events, ok := a.collectAugmentContractProbe(ctx, client, "live-local")
	if !ok {
		t.Fatal("probe did not run")
	}
	diagnosticLog, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if output := strings.TrimSpace(os.Getenv("R116_AUGMENT_PROBE_OUTPUT")); output != "" {
		if dir := filepath.Dir(output); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(output, diagnosticLog, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("诊断日志已导出：%s（%d 字节）", output, len(diagnosticLog))
	}
	readAny := false
	for _, event := range events {
		encoded, _ := json.Marshal(event)
		text := string(encoded)
		if len(text) > 6000 {
			text = text[:6000] + "…(truncated)"
		}
		t.Logf("%s", text)
		if event["event"] == "augment_contract_probe_summary" {
			if value, ok := event["contract_read_any"].(bool); ok {
				readAny = value
			}
		}
	}
	if !readAny {
		t.Error("三个契约来源一个都没读到（R82 曾观测到 openapi.json 404）：本轮探测不构成任何结论，" +
			"请把 status/result 抄进 docs/r116-probe-findings.md 并改用 /help?format=Full 的形状记录判读")
	}
}
