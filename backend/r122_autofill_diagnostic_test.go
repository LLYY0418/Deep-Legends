package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// R122：WORKLIST-R119-AUTOFILL-DIAGNOSTIC-EVENTS-UNTESTED 的验收测试。
//
// 缺口是「三个补位计数从汇总结构写进诊断事件」这一步完全没有护栏：删掉事件里
// 的任意一行赋值，全部既有测试仍然通过。修法是把三行赋值抽成 autofillDiagnosticFields，
// 两个事件共用同一来源，再补下面三层测试：
//
//  1. 单元层：键集合恰好三个、取值与汇总结构逐项一致（fixture 用 1/2/3 三个
//     互不相同的值，避免「全 0 也相等」把取值错配漏过去）；
//  2. 端到端（sgp_match_history_succeeded）：真跑 SGP 战绩分页，断言事件里三个键
//     的**具体数值**；
//  3. 端到端（sgp_summary_history_succeeded）：真跑 30 天窗口分支，断言三个键
//     带着**与第 2 层同口径的真实数值**（2/3/1）。
//
// （R120 复测工单「顺手改注释」：这一段原本写的是「第 3 层只断言键存在、值恒为 0，
// 因为要让详情页为空只能喂自定义对局，而自定义对局不属于 420/440、计数必然是 0」。
// 独立对抗复核（变异 M7/M8：把窗口事件的实参换成零值汇总仍全绿）证明那个口径
// 守不住，现已修正：把与第 2 层完全相同的三场 420/440 对局的 gameType 换成
// CUSTOM_GAME——isCustomGameplayMatch 把它们从可见战绩里过滤掉（窗口分支照常
// 触发），但 summarizeRiotParticipants 的队列门禁只看 QueueID，计数照算，所以
// 第 3 层钉得住真实数值。对应 TestR122SGPSummaryHistoryEventCarriesAutofillCounts。）

// r122NormalLanes 造一场十人、两个推算位置完全一致的常规对局位置表。
func r122NormalLanes() [10][2]string {
	lanes := [5]string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}
	positions := [10][2]string{}
	for index := range positions {
		positions[index] = [2]string{lanes[index%5], lanes[index%5]}
	}
	return positions
}

// r122SGPGame 造一场 SGP SUMMARY 形状的对局（外层 {"json": {...}}）。
// positions 按下标给出 teamPosition 与 individualPosition。
func r122SGPGame(gameID, queueID int64, gameType string, positions [10][2]string, playerRef string) map[string]any {
	participants := make([]map[string]any, 0, len(positions))
	for index, position := range positions {
		puuid := strings.Repeat(string(rune('a'+index)), 48)
		if index == 0 {
			puuid = playerRef
		}
		participants = append(participants, map[string]any{
			"puuid": puuid, "participantId": index + 1,
			"teamId": 100 + int64(index/5)*100, "championId": 103,
			"win": index < 5, "kills": 3, "deaths": 2, "assists": 4,
			"totalMinionsKilled": 120, "goldEarned": 9000,
			"teamPosition": position[0], "individualPosition": position[1],
		})
	}
	return map[string]any{"json": map[string]any{
		"gameId": gameID, "queueId": queueID, "gameMode": "CLASSIC", "gameType": gameType,
		"mapId": 11, "gameDuration": 1800, "gameCreation": time.Now().UnixMilli(),
		"gameEndTimestamp": time.Now().UnixMilli(),
		"participants":     participants,
		"teams": []map[string]any{
			{"teamId": 100, "win": true}, {"teamId": 200, "win": false},
		},
	}}
}

// r122EncodeGames 把若干场对局编码成 SGP 的一页响应。
func r122EncodeGames(t *testing.T, games ...map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"games": games})
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

// r122AssertAutofillKeysPresent 断言事件带着三个补位计数键（键存在性）。
func r122AssertAutofillKeysPresent(t *testing.T, event map[string]any, label string) {
	t.Helper()
	for _, key := range []string{"autofill_candidates", "position_mismatch", "autofill_no_evidence"} {
		if _, ok := event[key]; !ok {
			t.Fatalf("%s 事件缺少 %q 键：%#v", label, key, event)
		}
	}
}

// r122Number 读取事件里的数值字段（JSON 反序列化成 float64）。
func r122Number(t *testing.T, event map[string]any, key string) float64 {
	t.Helper()
	value, ok := event[key]
	if !ok {
		t.Fatalf("事件缺少 %q 键：%#v", key, event)
	}
	number, ok := value.(float64)
	if !ok {
		t.Fatalf("事件 %q = %#v (%T)，不是数值", key, value, value)
	}
	return number
}

func r122Store(t *testing.T) *localStore {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	return trackTestStore(t, &localStore{root: root})
}

// 第 1 层：共享函数的键集合与取值。
// 变异：把任一键改名（例如改回 autofill_swapped / autofill_tagged）、删掉任一键、
// 或把某个键的取值换成另一个字段 → 本测试 FAIL。
func TestR122AutofillDiagnosticFieldsCarryExactlyThreeNeutralKeys(t *testing.T) {
	fields := autofillDiagnosticFields(participantCompletenessSummary{
		AutofillCandidates: 1, PositionMismatch: 2, AutofillNoEvidence: 3,
	})
	want := map[string]any{
		"autofill_candidates":  1,
		"position_mismatch":    2,
		"autofill_no_evidence": 3,
	}
	if len(fields) != len(want) {
		t.Fatalf("键数量 = %d, want %d（键集合必须恰好是三个中性名）：%#v", len(fields), len(want), fields)
	}
	for key, expected := range want {
		got, ok := fields[key]
		if !ok {
			t.Fatalf("缺少键 %q：%#v", key, fields)
		}
		if got != expected {
			t.Fatalf("fields[%q] = %#v, want %#v（三个值刻意互不相同，取值错配必须被抓到）", key, got, expected)
		}
	}
	// 零值也必须产出全部三个键：真机上一场都没有候选时，键缺失与计数为 0
	// 是两件不同的事，诊断导出不能少字段。
	zero := autofillDiagnosticFields(participantCompletenessSummary{})
	if len(zero) != len(want) {
		t.Fatalf("零值汇总的键数量 = %d, want %d：%#v", len(zero), len(want), zero)
	}
	for key := range want {
		if value, ok := zero[key]; !ok {
			t.Fatalf("零值汇总缺少键 %q", key)
		} else if value != 0 {
			t.Fatalf("零值汇总的 %q = %#v, want 0", key, value)
		}
	}
}

// r122AutofillGames 造三场对局，让 summarizeRiotParticipants 恰好产出
// **候选 2 / 位置不一致 3 / 整场降级 1**——三个互不相同且都非零的数，这样
// 「取值错配」与「传了零值汇总」两类变异都无处可藏。
//
// gameType 决定这三场在界面上可不可见：
//
//	MATCHED_GAME → 可见，用于详情页（sgp_match_history_*）断言；
//	CUSTOM_GAME  → 被 isCustomGameplayMatch 过滤（matches 为空），但计数只看
//	               QueueID，因此三个数依然非零。这是唯一能同时满足「触发 30 天
//	               窗口分支（要求 len(matches)==0）」与「窗口事件数值可断言」
//	               的形状。
func r122AutofillGames(t *testing.T, playerRef, gameType string) []map[string]any {
	t.Helper()
	// 第 1 场（420）：下标 8 两个推算位置不一致（mismatch），下标 9 个人位置缺失
	// （候选）。有效个人位置 9/10 = 90%，恰好过覆盖率门槛。
	first := r122NormalLanes()
	first[8] = [2]string{"MIDDLE", "BOTTOM"}
	first[9] = [2]string{"UTILITY", ""}
	// 第 2 场（440）：两个 mismatch + 一个 Invalid 候选。
	second := r122NormalLanes()
	second[5] = [2]string{"BOTTOM", "TOP"}
	second[6] = [2]string{"TOP", "MIDDLE"}
	second[7] = [2]string{"JUNGLE", "Invalid"}
	// 第 3 场（420）：全场没有个人位置 → 整场降级，计一次 no_evidence。
	third := r122NormalLanes()
	for index := range third {
		third[index] = [2]string{third[index][0], ""}
	}
	return []map[string]any{
		r122SGPGame(1001, 420, gameType, first, playerRef),
		r122SGPGame(1002, 440, gameType, second, playerRef),
		r122SGPGame(1003, 420, gameType, third, playerRef),
	}
}

// r122AssertAutofillCounts 断言一个诊断事件带着三个键、且数值恰好是 2 / 3 / 1。
func r122AssertAutofillCounts(t *testing.T, event map[string]any, label string) {
	t.Helper()
	r122AssertAutofillKeysPresent(t, event, label)
	wants := map[string]float64{
		"autofill_candidates":  2,
		"position_mismatch":    3,
		"autofill_no_evidence": 1,
	}
	for key, want := range wants {
		if got := r122Number(t, event, key); got != want {
			t.Fatalf("%s 事件的 %s = %v, want %v（值必须来自汇总结构，不能错配、不能是零值）", label, key, got, want)
		}
	}
}

// 第 2 层：sgp_match_history_succeeded 带三个键，且数值与真实对局形状一致。
// 变异：删掉详情页事件里的 autofillDiagnosticFields 合并循环 → 本测试 FAIL。
func TestR122SGPMatchHistoryEventCarriesAutofillCounts(t *testing.T) {
	if autofillLabelGate {
		t.Fatal("本用例必须在标签总开关关闭的前提下运行（R121 P1-1 的默认状态）")
	}
	playerRef := strings.Repeat("d", 48)

	page := r122EncodeGames(t, r122AutofillGames(t, playerRef, "MATCHED_GAME")...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/match-history-query/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("startIndex") != "0" {
			_, _ = w.Write([]byte(`{"games":[]}`))
			return
		}
		_, _ = io.WriteString(w, page)
	}))
	defer server.Close()

	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	store := r122Store(t)
	a := &app{sgp: provider, storage: store}

	matches, capabilities, _ := a.loadDetailedMatches(context.Background(), client, gameplayReference{ServerID: "HN1"}, playerRef, false, 0, 20, "all", nil, nil)
	if len(matches) != 3 {
		t.Fatalf("SGP 战绩路径没有正常返回：matches = %d, capabilities = %#v", len(matches), capabilities)
	}
	// 开关关闭 → 一个标签都不下发，JSON 里连 autofill 键都没有。
	for index, match := range matches {
		for position, participant := range match.Participants {
			if participant.Autofill {
				t.Fatalf("matches[%d].Participants[%d].Autofill 必须为 false（开关关闭）", index, position)
			}
		}
	}
	payload, err := json.Marshal(matches)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "autofill") {
		t.Fatalf("开关关闭时响应里不得出现 autofill 键：%s", payload)
	}

	event := r116dDiagnosticEvents(t, store, "sgp_match_history_succeeded")[0]
	r122AssertAutofillCounts(t, event, "sgp_match_history_succeeded")
}

// 第 3 层：sgp_summary_history_succeeded 也带这三个键，且数值与详情页事件一致。
// 变异：删掉窗口事件的合并循环、或把它的实参换成零值汇总 → 本测试 FAIL。
func TestR122SGPSummaryHistoryEventCarriesAutofillCounts(t *testing.T) {
	if autofillLabelGate {
		t.Fatal("本用例必须在标签总开关关闭的前提下运行")
	}
	playerRef := strings.Repeat("w", 48)

	// 窗口分支的进入条件是「详情页返回 0 场可见对局」。这里用与详情页测试
	// **完全相同**的三场对局，只把 gameType 换成 CUSTOM_GAME：它们会被
	// isCustomGameplayMatch 过滤掉（matches 为空），但 QueueID 仍是 420/440，
	// 所以 summarizeRiotParticipants 照常计数。于是窗口事件既能被触发、
	// 又能断言真实数值 2/3/1，而不是只能断言「键存在、值为 0」。
	page := r122EncodeGames(t, r122AutofillGames(t, playerRef, "CUSTOM_GAME")...)

	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/match-history-query/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("startIndex") != "0" {
			_, _ = w.Write([]byte(`{"games":[]}`))
			return
		}
		_, _ = io.WriteString(w, page)
	}))
	defer sgpServer.Close()
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/lol-ranked/v1/current-ranked-stats":
			_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"EMERALD","division":"II","leaguePoints":40,"wins":30,"losses":20}]}`)
		default:
			_, _ = io.WriteString(w, `[]`)
		}
	}))
	defer lcuServer.Close()

	client := &LCUClient{
		baseURL: lcuServer.URL, token: "test", http: lcuServer.Client(),
		region: "TENCENT", rsoPlatform: "HN1", platformProbe: true,
	}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN1"] = sgpServer.URL
	provider.token, provider.tokenAt, provider.tokenClient = "entitlements", time.Now(), client
	provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
	store := r122Store(t)
	a := &app{
		sgp: provider, lcu: client, connected: true, storage: store,
		summoner: Summoner{PUUID: playerRef, GameName: "当前玩家"}, lpTracker: newLPTracker(nil),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}

	result := make(chan gameplayOverview, 1)
	go func() {
		result <- a.loadGameplayOverview(context.Background(), client, a.summoner, gameplayReference{PlayerRef: playerRef, ServerID: "HN1"}, 0, 20, "all", false)
	}()
	var overview gameplayOverview
	select {
	case overview = <-result:
	case <-time.After(30 * time.Second):
		t.Fatal("loadGameplayOverview 未在 30 秒内返回")
	}
	// 前置条件必须真的成立：三场都是 CUSTOM_GAME，会被 isCustomGameplayMatch
	// 过滤掉，于是 matches 为空——这正是 shouldLoadOverviewHistory 要求
	// len(matches)==0 才走 30 天窗口分支的条件。若这里非空，说明窗口事件根本
	// 不该出现，后面的断言就是在测另一条路径了。
	if len(overview.Matches) != 0 {
		t.Fatalf("详情页本该被自定义对局过滤成 0 场，实际 %d 场：窗口分支的前置条件不成立", len(overview.Matches))
	}

	// 复用仓库既有的诊断事件读取辅助函数（gameplay_r116d_test.go）：它在找不到
	// 指定事件时直接 Fatal，所以这里不用再判空。
	//
	// 关键：summarizeRiotParticipants 只按 QueueID 统计（420/440 就计数），而
	// isCustomGameplayMatch 只看 gameType，所以「queueId=420 + CUSTOM_GAME」这种
	// 对局在界面上不可见、在计数里却非零。窗口事件因此能断言**真实数值** 2/3/1，
	// 而不只是键存在——把 windowDiagnostic 的实参换成零值汇总这类变异也会打红。
	window := r116dDiagnosticEvents(t, store, "sgp_summary_history_succeeded")[0]
	r122AssertAutofillCounts(t, window, "sgp_summary_history_succeeded")
	// 同一页数据也会走详情页事件，两个事件必须给出同一组数（共享同一函数）。
	detail := r116dDiagnosticEvents(t, store, "sgp_match_history_succeeded")[0]
	r122AssertAutofillCounts(t, detail, "sgp_match_history_succeeded")
}
