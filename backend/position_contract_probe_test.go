package main

// ============================================================================
// R121-P3-1 探测单测（TEMPORARY — 与 position_contract_probe.go 同时删除）
//
// 合成 fixture 中的字段名与示例值只用于验证解析器，不是真机证据。
// 真机入口默认 Skip，仅在显式设置 R121_POSITION_PROBE=1 时运行。
// ============================================================================

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestR121PositionContractsExpandSyntheticV3(t *testing.T) {
	// 合成示例，非真机证据；example 绝不能进入探针事件。
	raw := []byte(`{
		"openapi":"3.0.0",
		"paths":{
			"/lol-lobby/v2/lobby":{"get":{"operationId":"getLobby","responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/LobbyDto"}}}}}}},
			"/lol-champ-select/v1/session":{"get":{"responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ChampSelectSessionDto"}}}}}}},
			"/lol-summoner/v1/current-summoner":{"get":{"responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/UnrelatedDto"}}}}}}}
		},
		"components":{"schemas":{
			"LobbyDto":{"type":"object","properties":{"members":{"type":"array","items":{"$ref":"#/components/schemas/LobbyMemberDto"}},"lobbyId":{"type":"string"}}},
			"LobbyMemberDto":{"type":"object","properties":{"firstPreference":{"type":"string","example":"R121_SYNTHETIC_FIRST_VALUE"},"secondPreference":{"type":"string","example":"R121_SYNTHETIC_SECOND_VALUE"},"puuid":{"type":"string","example":"R121_SYNTHETIC_PUUID_VALUE"},"slot":{"type":"integer","example":17}}},
			"ChampSelectSessionDto":{"type":"object","properties":{"members":{"type":"array","items":{"$ref":"#/components/schemas/ChampSelectMemberDto"}}}},
			"ChampSelectMemberDto":{"type":"object","properties":{"assignedPosition":{"type":"string","example":"R121_SYNTHETIC_ASSIGNED_VALUE"},"cellId":{"type":"integer"}}},
			"UnrelatedDto":{"type":"object","properties":{"displayName":{"type":"string"},"level":{"type":"integer"}}}
		}}
	}`)

	if !json.Valid(raw) {
		t.Fatal("synthetic OpenAPI fixture is invalid JSON")
	}
	records, stats := positionProbeContracts(raw)
	if !stats.ContractRead {
		t.Fatal("synthetic OpenAPI must be recognized as readable")
	}
	if stats.MatchedEndpoints != 2 || stats.MatchedPaths != 2 {
		t.Fatalf("matched endpoints/paths = %d/%d, want 2/2", stats.MatchedEndpoints, stats.MatchedPaths)
	}
	if stats.ExpandedSchemas == 0 || stats.MatchedProperties != 3 {
		t.Fatalf("schema/property counters = %d/%d, want schemas > 0 and properties 3", stats.ExpandedSchemas, stats.MatchedProperties)
	}

	lobbyFound, champFound, assignedFound := false, false, false
	for _, record := range records {
		if record["write_executed"] != false {
			t.Fatalf("write_executed must be false: %v", record)
		}
		path, _ := record["path"].(string)
		if path == "/lol-lobby/v2/lobby" && record["response_schema"] != "LobbyDto" {
			t.Fatalf("lobby response_schema = %v, want LobbyDto", record["response_schema"])
		}
		if path == "/lol-champ-select/v1/session" && record["response_schema"] != "ChampSelectSessionDto" {
			t.Fatalf("champ-select response_schema = %v, want ChampSelectSessionDto", record["response_schema"])
		}
		if path == "/lol-summoner/v1/current-summoner" {
			t.Fatalf("unrelated endpoint leaked into records: %v", record)
		}
		expanded, _ := record["expanded_schemas"].([]map[string]any)
		for _, schema := range expanded {
			name, _ := schema["schema_name"].(string)
			if name != "LobbyMemberDto" {
				continue
			}
			lobbyFound = true
			matched, _ := schema["matched_properties"].([]map[string]any)
			names := make([]string, 0, len(matched))
			for _, property := range matched {
				name, _ := property["name"].(string)
				names = append(names, name)
				if property["type"] != "string" {
					t.Fatalf("LobbyMemberDto property type = %v, want string", property["type"])
				}
			}
			sort.Strings(names)
			if strings.Join(names, ",") != "firstPreference,secondPreference" {
				t.Fatalf("LobbyMemberDto matched properties = %v", names)
			}
		}
		for _, schema := range expanded {
			if schema["schema_name"] != "ChampSelectMemberDto" {
				continue
			}
			champFound = true
			matched, _ := schema["matched_properties"].([]map[string]any)
			for _, property := range matched {
				if property["name"] == "assignedPosition" && property["type"] == "string" {
					assignedFound = true
				}
			}
		}
	}
	if !lobbyFound || !champFound || !assignedFound {
		t.Fatalf("expanded schemas missing required position properties: %v", records)
	}
	encoded, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "R121_SYNTHETIC_") {
		t.Fatalf("fixture example value leaked into events: %s", encoded)
	}
}

func TestR121PositionProbeRefCycleBounded(t *testing.T) {
	raw := []byte(`{
		"openapi":"3.0.0",
		"paths":{"/lol-lobby/v2/lobby":{"get":{"responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/A"}}}}}}}},
		"components":{"schemas":{
			"A":{"type":"object","properties":{"b":{"$ref":"#/components/schemas/B"}}},
			"B":{"type":"object","properties":{"a":{"$ref":"#/components/schemas/A"}}}
		}}
	}`)
	records, stats := positionProbeContracts(raw)
	if len(records) != 1 || stats.SchemaTruncated {
		t.Fatalf("cycle traversal was not bounded cleanly: records=%d stats=%+v", len(records), stats)
	}
	expanded, _ := records[0]["expanded_schemas"].([]map[string]any)
	if len(expanded) != 2 {
		t.Fatalf("expanded schema count = %d, want 2 (A and B): %v", len(expanded), expanded)
	}
}

// TestR121PositionContractProbeLiveClient 是仅本机可触发的临时入口。
func TestR121PositionContractProbeLiveClient(t *testing.T) {
	if os.Getenv("R121_POSITION_PROBE") != "1" {
		t.Skip("opt-in local LCU reconnaissance: set R121_POSITION_PROBE=1 on the machine running the League client")
	}
	client, report, err := discoverLCUDetailed()
	if err != nil {
		t.Logf("LCU discovery failed (%s / %s): %v", report.Result, report.Detail, err)
		if lockfile := strings.TrimSpace(os.Getenv("R121_POSITION_PROBE_LOCKFILE")); lockfile != "" {
			client, err = clientFromLockfile(lockfile)
			if err != nil {
				t.Fatalf("lockfile fallback failed: %v", err)
			}
			client.source = "lockfile"
		} else {
			t.Fatal("客户端未连接：请先登录 League 客户端，或用 R121_POSITION_PROBE_LOCKFILE 指定 lockfile")
		}
	}
	defer client.Close()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{connected: true, lcu: client, storage: store}
	// 外层比探测自身的 positionProbeTotalTimeout（90s）宽松，让内层超时成为唯一约束。
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	events, ok := a.collectPositionContractProbe(ctx, client, "live-local")
	if !ok {
		t.Fatal("probe did not run")
	}
	diagnosticLog, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if output := strings.TrimSpace(os.Getenv("R121_POSITION_PROBE_OUTPUT")); output != "" {
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
	verdict := ""
	endpointEvents, contractEvents := 0, 0
	for _, event := range events {
		if event["write_executed"] != false {
			t.Errorf("write_executed must stay false: %v", event)
		}
		switch event["event"] {
		case "position_endpoint_probe":
			endpointEvents++
		case "position_contract_probe":
			contractEvents++
		case "position_contract_probe_summary":
			verdict, _ = event["verdict"].(string)
		}
		encoded, _ := json.Marshal(event)
		if len(encoded) > 6000 {
			encoded = append(encoded[:6000], []byte("…(truncated)")...)
		}
		t.Logf("%s", encoded)
	}
	if endpointEvents != len(positionProbeEndpoints) || contractEvents != len(positionProbeSources) {
		t.Errorf("事件不完整：endpoint %d/%d、contract %d/%d（可能整体超时或 ctx 被取消）",
			endpointEvents, len(positionProbeEndpoints), contractEvents, len(positionProbeSources))
	}
	switch verdict {
	case positionProbeVerdictFound:
		t.Log("结论：found —— 扫到了命中谓词的键，把 matched_keys 抄进 docs/r121-position-probe-findings.md 的结论表")
	case positionProbeVerdictNotFound:
		t.Log("结论：not-found —— 关键端点已被真正扫描且零命中，可以写「该端点响应里没有位置偏好/补位字段」")
	case positionProbeVerdictInconclusive:
		t.Error("结论：inconclusive —— 关键端点都没有可读响应，本轮不构成任何结论。请在客户端处于大厅或选人阶段时重跑；inconclusive 绝不等于「没有该字段」")
	default:
		t.Errorf("summary 里 verdict 缺失或非法：%q", verdict)
	}
}

// ---------------------------------------------------------------------------
// R121 工单（POSITION-PROBE-HELP-FALLBACK-INEFFECTIVE）P1-1 验收判据 1 与 2：
// 不依赖真实客户端，用合成响应证明「键名扫描逻辑本身有效」。
//
// 判据 2（对抗变异）：把 positionProbeProperty 换成必然扫不到东西的假谓词
// （例如 `regexp.MustCompile("zzz_no_such_key")`），下面第一个测试必须 FAIL；
// 把 positionProbeResponseDepth 从 2 改成 1，第二个测试的 gameflow 用例必须 FAIL。
// ---------------------------------------------------------------------------

// r121HitIndex 把扫描结果压成 name → (type, owner) 便于断言。
func r121HitIndex(scan positionProbeResponseScan) map[string][]positionProbeKeyHit {
	index := map[string][]positionProbeKeyHit{}
	for _, hit := range scan.Hits {
		index[hit.Name] = append(index[hit.Name], hit)
	}
	return index
}

func TestR121PositionResponseKeysScanSyntheticLobbyBody(t *testing.T) {
	// 合成示例，非真机证据。所有 R121_SYNTHETIC_ 开头的**取值**都绝不能出现在
	// 扫描结果里——那是本测试最重要的一条断言（安全边界：只记键名与类型）。
	lobby := []byte(`{
		"lobbyId": "R121_SYNTHETIC_LOBBY_ID",
		"members": [
			{"firstPreference": "R121_SYNTHETIC_FIRST", "secondPreference": "R121_SYNTHETIC_SECOND",
			 "puuid": "R121_SYNTHETIC_PUUID", "slot": 3, "isLeader": true},
			{"firstPreference": "R121_SYNTHETIC_FIRST_B", "puuid": "R121_SYNTHETIC_PUUID_B"}
		],
		"gameConfig": {"queueId": 420, "mapId": 11},
		"invitationCount": 0
	}`)

	scan := positionProbeResponseKeys(lobby)
	if !scan.ValidJSON {
		t.Fatal("合法 JSON 响应必须被识别为 ValidJSON")
	}
	if scan.Scanned == 0 {
		t.Fatal("一个键都没扫到：扫描逻辑无效（判据 1 不成立）")
	}
	if scan.Truncated {
		t.Fatalf("这么小的响应不该触发键数上限：%#v", scan)
	}

	index := r121HitIndex(scan)
	// 命中：members[] 里两个成员的位置偏好键。
	for _, name := range []string{"firstPreference", "secondPreference"} {
		hits, ok := index[name]
		if !ok {
			t.Fatalf("%q 必须被扫出（谓词含 preference）；实际命中 %#v", name, scan.Hits)
		}
		for _, hit := range hits {
			if hit.Type != "string" {
				t.Fatalf("%q 的类型 = %q, want string", name, hit.Type)
			}
			if hit.Owner != "members[]" {
				t.Fatalf("%q 的 owner = %q, want members[]（数组穿透后应标出所在对象路径）", name, hit.Owner)
			}
		}
	}
	if got := len(index["firstPreference"]); got != 2 {
		t.Fatalf("firstPreference 命中次数 = %d, want 2（两个成员各一次）", got)
	}
	// 不该命中：这些键既不含位置语义，也证明扫描器没有「见键就记」。
	for _, banned := range []string{"lobbyId", "members", "puuid", "slot", "isLeader", "gameConfig", "queueId", "mapId", "invitationCount"} {
		if _, ok := index[banned]; ok {
			t.Fatalf("%q 不该命中位置谓词，却出现在结果里：%#v", banned, scan.Hits)
		}
	}
	// 安全边界：取值一律不得外泄。
	encoded, err := json.Marshal(scan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "R121_SYNTHETIC_") {
		t.Fatalf("响应取值泄漏进了扫描结果：%s", encoded)
	}
}

func TestR121PositionResponseKeysReachNestedArraysAndRejectBareStrings(t *testing.T) {
	// gameflow 的真实形状：目标键在 gameData.teamOne[] 里，也就是第三层。
	// 这是 positionProbeResponseDepth 必须为 2 的原因——按工单原口径「顶层 +
	// 一层子对象」永远扫不到它。把常量改成 1，本用例必须 FAIL。
	gameflow := []byte(`{
		"phase": "R121_SYNTHETIC_PHASE",
		"gameData": {
			"queue": {"id": 420},
			"teamOne": [
				{"selectedPosition": "R121_SYNTHETIC_POS", "selectedRole": "R121_SYNTHETIC_ROLE",
				 "summonerName": "R121_SYNTHETIC_NAME", "championId": 103},
				{"selectedPosition": "R121_SYNTHETIC_POS_B"}
			]
		}
	}`)
	scan := positionProbeResponseKeys(gameflow)
	if !scan.ValidJSON {
		t.Fatal("gameflow 响应必须是合法 JSON")
	}
	index := r121HitIndex(scan)
	for _, name := range []string{"selectedPosition", "selectedRole"} {
		hits, ok := index[name]
		if !ok {
			t.Fatalf("%q 必须从 gameData.teamOne[] 里被扫出；实际命中 %#v", name, scan.Hits)
		}
		if hits[0].Owner != "gameData.teamOne[]" {
			t.Fatalf("%q 的 owner = %q, want gameData.teamOne[]", name, hits[0].Owner)
		}
	}
	// summonerName / phase 是身份与取值类字段，不该命中，也不该泄漏。
	for _, banned := range []string{"summonerName", "phase", "championId", "queue"} {
		if _, ok := index[banned]; ok {
			t.Fatalf("%q 不该命中位置谓词", banned)
		}
	}
	if encoded, _ := json.Marshal(scan); strings.Contains(string(encoded), "R121_SYNTHETIC_") {
		t.Fatalf("响应取值泄漏进了扫描结果：%s", encoded)
	}

	// 数组只穿透前 positionProbeResponseArrayLimit 个元素，且**必须**标记截断：
	// 同层兄弟元素里可能就有目标键（例如只有第 3 个成员带 assignedPosition），
	// 漏扫却报零命中会得出假的否定结论。
	wideElements := strings.Repeat(`{"assignedPosition":"x"},`, positionProbeResponseArrayLimit+1) + `{"assignedPosition":"x"}`
	wideScan := positionProbeResponseKeys([]byte(`{"members":[` + wideElements + `]}`))
	if got := len(r121HitIndex(wideScan)["assignedPosition"]); got != positionProbeResponseArrayLimit {
		t.Fatalf("数组穿透元素数 = %d, want %d（上限必须生效，否则战绩那种大响应会拖慢一次性侦察）", got, positionProbeResponseArrayLimit)
	}
	if !wideScan.Truncated {
		t.Fatal("有数组元素被跳过时必须置 Truncated，否则零命中会被误当成否定证据")
	}

	// 反过来：元素数不超过上限时不能被标记为截断。真实队伍最多 5 人，上限取 10
	// 就是为了让关键端点整支队伍都被扫到、保住下否定结论的资格。
	narrowScan := positionProbeResponseKeys([]byte(`{"members":[{"assignedPosition":"x"},{"assignedPosition":"y"},{"assignedPosition":"z"}]}`))
	if narrowScan.Truncated {
		t.Fatalf("3 个元素不该触发截断：%#v", narrowScan)
	}
	if got := len(r121HitIndex(narrowScan)["assignedPosition"]); got != 3 {
		t.Fatalf("3 个元素应全部扫到，实际 %d", got)
	}

	// 裸字符串响应（/lol-gameflow/v1/gameflow-phase 返回的就是这种）：它是合法
	// JSON，但一个键都没有。collect 里的扫描完成判据因此必须要求 Scanned > 0，
	// 否则这种「零命中」会被误当成否定证据。
	bare := positionProbeResponseKeys([]byte(`"ChampSelect"`))
	if !bare.ValidJSON || bare.Scanned != 0 || len(bare.Hits) != 0 {
		t.Fatalf("裸字符串响应 = %#v, want ValidJSON=true / Scanned=0 / 无命中", bare)
	}
	// 非法 JSON 同样不构成任何证据。
	if broken := positionProbeResponseKeys([]byte(`<html>not json</html>`)); broken.ValidJSON || broken.Scanned != 0 {
		t.Fatalf("非法 JSON 响应 = %#v, want ValidJSON=false / Scanned=0", broken)
	}
}

// TestR121PositionProbeVerdictSeparatesReadFromConclusive 锁住三态判据。
// 上一轮真机的教训就是「读到了东西」被当成「查过了、确实没有」：/help 返回
// 200 与 3 MB 内容，contract_read_any=true，但结论其实是 inconclusive。
func TestR121PositionProbeVerdictSeparatesReadFromConclusive(t *testing.T) {
	tests := []struct {
		name               string
		endpointHits       int
		contractHits       int
		criticalConclusive int
		contractConclusive bool
		want               string
		wantReason         bool
	}{
		{"上一轮真机形状：只有非关键来源可读、零命中", 0, 0, 0, false, positionProbeVerdictInconclusive, true},
		{"什么都没读到", 0, 0, 0, false, positionProbeVerdictInconclusive, true},
		{"关键端点证据完整且零命中 → 可下否定结论", 0, 0, 1, false, positionProbeVerdictNotFound, false},
		{"契约完整读取且零属性命中 → 可下否定结论", 0, 0, 0, true, positionProbeVerdictNotFound, false},
		{"端点有命中 → 肯定结论", 3, 0, 1, false, positionProbeVerdictFound, false},
		{"只有非关键端点命中也算肯定结论", 2, 0, 0, false, positionProbeVerdictFound, false},
		{"契约属性有命中 → 肯定结论", 0, 4, 0, false, positionProbeVerdictFound, false},
		// 下面这条是 P1 缺陷的回归锁：契约里存在目标端点（MatchedEndpoints > 0）
		// 但属性零命中、且 schema 未完整展开时，既不能判 found（端点存在 ≠ 字段
		// 存在），也不能判 not-found（没完整展开）——只能是 inconclusive。
		{"契约只有端点命中、属性零命中且不完整 → 仍不可判定", 0, 0, 0, false, positionProbeVerdictInconclusive, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verdict, reason := positionProbeVerdict(test.endpointHits, test.contractHits, test.criticalConclusive, test.contractConclusive)
			if verdict != test.want {
				t.Fatalf("verdict = %q, want %q（reason=%q）", verdict, test.want, reason)
			}
			if test.wantReason && reason == "" {
				t.Fatal("inconclusive 必须给出原因，否则读日志的人无从判断下一步")
			}
			if !test.wantReason && reason != "" {
				t.Fatalf("可判定的结论不该带 inconclusive 原因，got %q", reason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 不完整的扫描绝不能变成否定证据（复核第二轮抓到的两个 P1）。
// 下面五个用例分别锁住：深度超限且下面还有结构、深度超限但下面是标量、
// owner 路径脱敏、契约缺 response schema、$ref 解析失败。
// 对抗变异：把 walker 里的 `scan.Truncated = true`（深度分支）删掉 → 用例 1 FAIL；
// 把 positionProbeOwnerPath 的 SafeName 校验删掉 → 用例 3 FAIL；
// 把 positionProbeContracts 里的 `stats.MissingSchemas++` / `stats.UnresolvedRefs += walker.badRefs`
// 删掉 → 用例 4 / 5 FAIL。
// ---------------------------------------------------------------------------

func TestR121PositionProbeIncompleteScansNeverBecomeNegativeEvidence(t *testing.T) {
	// 1) 深度超限、且被跳过的值下面还有非空结构 → 必须置 Truncated。
	//    目标键 assignedPosition 在第 5 层（members[].profile.deep.deeper），
	//    超出 positionProbeResponseDepth；静默跳过会得到「零命中 + 看起来扫完整了」，
	//    进而被 critical 端点判据当成 not-found（假否定）。
	deep := positionProbeResponseKeys([]byte(`{"members":[{"profile":{"deep":{"deeper":{"assignedPosition":"R121_SYNTHETIC_DEEP"}}}}]}`))
	if len(deep.Hits) != 0 {
		t.Fatalf("第 5 层的键不该被扫到（深度上限 %d）：%#v", positionProbeResponseDepth, deep.Hits)
	}
	if !deep.Truncated {
		t.Fatal("深度超限且下面还有未扫描结构时必须置 Truncated，否则零命中会被当成否定证据")
	}

	// 2) 深度超限、但被跳过的值是标量 → 没有漏扫任何键，不该置 Truncated。
	//    这一条同样重要：若把「深度受限」一律当成截断，任何真实响应都会被判
	//    不完整，关键端点将永远失去下否定结论的资格。
	flat := positionProbeResponseKeys([]byte(`{"members":[{"profile":{"deep":{"puuid":"R121_SYNTHETIC_PUUID"}}}]}`))
	if flat.Truncated {
		t.Fatalf("被跳过的值是标量时不该置 Truncated：%#v", flat)
	}
	if !flat.ValidJSON || flat.Scanned == 0 {
		t.Fatalf("这个响应应当被正常扫描：%#v", flat)
	}

	// 3) owner 路径必须脱敏：祖先键名不过白名单时用占位符，不能把原始键名带进日志。
	redacted := positionProbeResponseKeys([]byte(`{"private name":{"assignedPosition":"R121_SYNTHETIC_TOP"}}`))
	if len(redacted.Hits) != 1 {
		t.Fatalf("应命中 1 个键，实际 %#v", redacted.Hits)
	}
	if redacted.Hits[0].Owner != positionProbeRedactedSegment {
		t.Fatalf("owner = %q, want %q（祖先键名含空格，必须脱敏）", redacted.Hits[0].Owner, positionProbeRedactedSegment)
	}
	if encoded, _ := json.Marshal(redacted); strings.Contains(string(encoded), "private name") {
		t.Fatalf("未过白名单的祖先键名泄漏进了扫描结果：%s", encoded)
	}
	if encoded, _ := json.Marshal(redacted); strings.Contains(string(encoded), "R121_SYNTHETIC_") {
		t.Fatalf("响应取值泄漏进了扫描结果：%s", encoded)
	}

	// 4) 契约里命中了端点、但响应没有可解析 schema → 计入 MissingSchemas。
	//    collect 的 contractComplete 依赖它：一个属性都展开不出来时，
	//    「零属性命中」不能作为否定证据。
	_, noSchema := positionProbeContracts([]byte(`{"openapi":"3.0.0","paths":{"/lol-lobby/v2/lobby":{"get":{"responses":{"200":{"description":"ok"}}}}}}`))
	if noSchema.MatchedEndpoints != 1 {
		t.Fatalf("MatchedEndpoints = %d, want 1（这个 fixture 应当命中端点）", noSchema.MatchedEndpoints)
	}
	if noSchema.MissingSchemas != 1 {
		t.Fatalf("MissingSchemas = %d, want 1：没有响应 schema 必须被记为不完整", noSchema.MissingSchemas)
	}
	if noSchema.MatchedProperties != 0 {
		t.Fatalf("MatchedProperties = %d, want 0", noSchema.MatchedProperties)
	}

	// 5) $ref 指向不存在的节点 → 计入 UnresolvedRefs（同样让 contractComplete 为 false）。
	_, badRef := positionProbeContracts([]byte(`{"openapi":"3.0.0","paths":{"/lol-lobby/v2/lobby":{"get":{"responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/DoesNotExist"}}}}}}}}}`))
	if badRef.UnresolvedRefs != 1 {
		t.Fatalf("UnresolvedRefs = %d, want 1：解析不了的 $ref 必须被记为不完整", badRef.UnresolvedRefs)
	}
	if badRef.MatchedProperties != 0 {
		t.Fatalf("MatchedProperties = %d, want 0", badRef.MatchedProperties)
	}
}
