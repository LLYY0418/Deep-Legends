package main

// ============================================================================
// R121-P3-1 探测：位置偏好 / 分配位置 / 补位契约的一次性侦察
//
// 工单出处：WORKLIST-R121 P3-1（仓库内文件名 docs/WORKLIST-R119-AUTOFILL-LABEL-GAPS.md）
// 结论落盘处：docs/r121-position-probe-findings.md
//
// 目的只有一个：一次性回答「本机 LCU 客户端里，是否存在真正的位置偏好 /
// 分配位置 / 补位（autofill）字段」，为补位标签总开关（autofillLabelGate）的
// 去留、以及是否另起「对局进行时记录己方补位信息」工单提供依据。
//
// 两条取证路径（R121 工单 POSITION-PROBE-HELP-FALLBACK-INEFFECTIVE 之后）：
//
//  1. **端点直探（主力）**：不依赖任何契约文档，直接 GET positionProbeEndpoints
//     里那六个已知端点，扫响应 JSON 的**键名**（positionProbeResponseKeys）。
//     这是唯一能产出可判定结论的路径。
//  2. **契约清单（best-effort 补充）**：openapi v3/v2 若可用，$ref 展开能给出
//     比响应样本更完整的 schema；404 时完全不影响上面的结论。
//
// 为什么放弃原先的 /help?format=Full 文本兜底：真机实测它返回 200 与 3,022,367
// 字节，但「扫字面斜杠路径」只匹配到 1 个 token 且不在关心的命名空间里——说明
// 该端点输出的是驼峰式 RPC 名而非 REST 路径，格式无关扫描从一开始就扫不到东西。
// 详见 docs/r121-position-probe-findings.md 与本轮台账。
//
// 这不是产品功能，必须整体删除：本文件与 position_contract_probe_test.go 在
// 真机探测执行完毕、结论写入 docs/r121-position-probe-findings.md 之后立即删除，
// 不进入正式版本。**没有任何生产调用方**，唯一入口是那个默认 Skip 的测试。
//
// 安全边界（与 augment_contract_probe.go 的既有约定一致）：
//   - 只读：只发 GET，永不写客户端，事件里恒带 write_executed=false；
//   - 只记录形状（path / method / schema 名 / 键名与类型 / 响应码），**绝不记录
//     任何响应体取值**——响应里可能有召唤师名、puuid、对局数据，一个都不能落盘；
//   - 不复用也绝不修改 augmentProbePath / objectiveDiagnosticPath，
//     本文件自带独立谓词 positionProbePath / positionProbeProperty。
// ============================================================================

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	// positionProbePath 是契约扫描的路径谓词，覆盖工单点名的五个命名空间。
	positionProbePath = regexp.MustCompile(`(?i)lobby|champ-select|gameflow|end-of-game|match-history`)
	// positionProbeProperty 是属性名谓词：只有命中它的属性才会被记录名字与类型。
	// 端点直探与契约扫描共用这一条谓词，保证两条路径的「命中」口径一致。
	positionProbeProperty = regexp.MustCompile(`(?i)position|preference|pref|fill|autofill|role|lane|assigned`)
	// positionProbeSafeName 限定可写入诊断日志的名字形状（schema 名、operationId）。
	positionProbeSafeName = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
)

const (
	positionProbeName           = "r121-position-preference-inventory"
	positionProbeContractLimit  = int64(16 << 20)
	positionProbeRequestTimeout = 10 * time.Second
	positionProbeTotalTimeout   = 90 * time.Second
	positionProbePathLimit      = 512
	// 契约（openapi）展开的递归深度与属性总数上限。
	positionProbeSchemaDepth   = 6
	positionProbePropertyLimit = 400
	// 命中 operation 明细的保留上限；stats 里的计数不受该上限影响，
	// 否则「命中 0」这条否定判据会被截断污染。
	positionProbeOperationLimit = 200
	// 端点直探：响应键名扫描的深度与上限。
	//
	// 深度 3 比工单口径（「顶层 + 一层子对象」）放宽两层，原因是真实响应形状
	// 装不下：/lol-gameflow/v1/session 把队伍放在 gameData.teamOne[] 里，
	// selectedPosition 已在第三层；再留一层是给可能存在的同类包装。数组不计入
	// 深度（只穿透），因此 lobby.members[].firstPreference 仍在第 1 层。
	//
	// 关键：到达深度上限时，若被跳过的值**还有未扫描的非空结构**（map/array），
	// 必须置 Truncated——否则 `{"members":[{"profile":{"deep":{"assignedPosition":…}}}]}`
	// 这种形状会零命中且「看起来扫完整了」，被误判成 not-found（假否定）。
	positionProbeResponseDepth = 3
	// 单个响应最多扫多少个键、每个数组最多看几个元素。战绩响应可能几 MB、几十场
	// 对局，不设上限会把一次性侦察变成分钟级 CPU 占用。
	//
	// 数组上限取 10 而不是更小：lobby.members / gameflow.teamOne / champ-select.myTeam
	// 都是最多 5 人的队伍，10 能保证**关键端点整支队伍都被扫到**、不会因截断而
	// 失去下否定结论的资格；真正会超限的只有战绩那种大数组，而它是非关键端点。
	// 一旦有元素被跳过就置 Truncated，零命中便不再算否定证据。
	positionProbeResponseKeyLimit   = 4000
	positionProbeResponseArrayLimit = 10
	// 祖先键名形状异常时写进 owner 路径的占位符，避免绕过 positionProbeSafeName
	// 把任意响应键名带进诊断日志（白名单必须覆盖完整路径，不只是命中键名）。
	positionProbeRedactedSegment = "?"
)

// positionProbeSources 是**契约清单**来源，best-effort 补充。
//
// R121 工单（WORKLIST-R121-POSITION-PROBE-HELP-FALLBACK-INEFFECTIVE）真机实测：
// 两个 openapi 来源在本机客户端上双双 404（与 R82 观测一致），而原先作为兜底的
// /help?format=Full 虽然返回 200 / 3,022,367 字节，用「扫字面斜杠路径」的策略
// 却只匹配到 1 个 token 且不在关心的命名空间里——说明该端点输出的是驼峰式 RPC
// 名而非 REST 路径字符串，格式无关扫描从一开始就扫不到东西。因此 /help 兜底已
// 按工单 P1-1 删除，本列表只保留 openapi：它 404 时不影响任何判据，200 时能顺带
// 给出完整 schema（含 $ref 展开），仍是最有信息量的来源。
var positionProbeSources = []struct {
	path string
	kind string
}{
	{"/swagger/v3/openapi.json", "openapi-v3"},
	{"/swagger/v2/swagger.json", "openapi-v2"},
}

// positionProbeEndpoints 是端点直探清单：不依赖任何契约文档，直接 GET 已知端点、
// 扫响应 JSON 的键名。这是本轮唯一能产出「可判定」结论的路径。
//
// note 逐个写明该端点在本仓库生产代码里的调用出处（工单 P1-1 验收判据 3 要求），
// 以及 404 是否属预期——404 只说明「此刻不在这个阶段」，不构成任何否定证据。
//
// critical 标记「位置偏好/补位字段最可能出现的端点」。汇总的否定判据只认
// critical 端点：如果 lobby 与 champ-select 全部 404（客户端不在大厅也不在选人
// 阶段，这是很常见的情况），那么即使 match-history 扫了几百个键零命中，也**不能**
// 得出「客户端没有位置偏好字段」——最可能藏这个字段的地方压根没看到。
var positionProbeEndpoints = []struct {
	path     string
	note     string
	critical bool
}{
	{"/lol-lobby/v2/lobby", "backend/gameplay.go:7244；大厅成员对象是位置偏好最可能的落点", true},
	{"/lol-champ-select/v1/session", "backend/gameplay.go:7226、8752；仅选人阶段有效，其余时刻 404 属预期", true},
	{"/lol-gameflow/v1/session", "backend/gameplay.go:7163；selectedPosition / selectedRole 在 gameData.teamOne[] 里", true},
	{"/lol-gameflow/v1/gameflow-phase", "backend/gameplay.go:7138、8254、8744；只返回裸阶段字符串，用来记录探测时客户端处于哪个阶段，本身不含成员对象", false},
	{"/lol-match-history/v1/products/lol/current-summoner/matches", "backend/gameplay.go:3296-3297；LCU 战绩回退路径，可顺带验证 teamPosition/individualPosition 是否真的下发", false},
	{"/lol-end-of-game/v1/eog-stats-block", "仅命名空间有出处（backend/flow_diagnostics.go:162）；该具体路径在本仓库无调用点，由工单指定，局外才有效，404 属预期且不构成证据", false},
}

var positionProbeMethods = []string{"get", "put", "patch", "post", "delete", "head", "options"}

// positionProbeContractStats 是一次 openapi 扫描的计数与**完整性**证据。
//
// MissingSchemas 是「命中了端点、但它的响应没有可解析 schema」的 operation 数——
// 这类 operation 一个属性都展开不出来，零命中当然不能算否定证据。
// UnresolvedRefs 是 `$ref` 解析失败的次数（形状非法、指向不存在的节点、或目标
// 不是对象），同样意味着展开不完整。两者任一非零，contractComplete 即为 false。
type positionProbeContractStats struct {
	ScannedPaths       int  `json:"scanned_paths"`
	MatchedPaths       int  `json:"matched_paths"`
	MatchedEndpoints   int  `json:"matched_endpoints"`
	ExpandedSchemas    int  `json:"expanded_schemas"`
	MatchedProperties  int  `json:"matched_properties"`
	MissingSchemas     int  `json:"missing_response_schemas"`
	UnresolvedRefs     int  `json:"unresolved_refs"`
	SchemaTruncated    bool `json:"schema_truncated"`
	OperationTruncated bool `json:"operations_truncated"`
	InvalidJSON        bool `json:"invalid_json"`
	ContractRead       bool `json:"contract_read"`
}

func positionProbeMatches(path string) bool {
	if path == "" || len(path) > positionProbePathLimit {
		return false
	}
	return positionProbePath.MatchString(strings.SplitN(path, "?", 2)[0])
}

// positionProbeResolve 把 "#/components/schemas/Xxx" 这类 JSON Pointer 解析成
// 契约里的实际节点。只支持文档内引用（"#/" 开头），外部引用一律当解析失败。
func positionProbeResolve(root map[string]any, ref string) any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	var value any = root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		value = object[part]
	}
	return value
}

// positionProbeRefName 取引用末段作为 schema 名，形状不合法就返回空串（丢弃）。
func positionProbeRefName(ref string) string {
	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	if len(parts) == 0 {
		return ""
	}
	name := strings.ReplaceAll(strings.ReplaceAll(parts[len(parts)-1], "~1", "/"), "~0", "~")
	if !positionProbeSafeName.MatchString(name) {
		return ""
	}
	return name
}

// positionProbeType 只回答「这个属性是什么类型」，绝不读取示例值或默认值。
func positionProbeType(value any, root map[string]any, depth int) string {
	if depth > positionProbeSchemaDepth {
		return "unknown"
	}
	object, ok := value.(map[string]any)
	if !ok {
		return "unknown"
	}
	if typ, ok := object["type"].(string); ok {
		switch typ {
		case "array", "object", "boolean", "integer", "number", "string":
			return typ
		}
	}
	if ref, ok := object["$ref"].(string); ok {
		return positionProbeType(positionProbeResolve(root, ref), root, depth+1)
	}
	if _, ok := object["properties"]; ok {
		return "object"
	}
	if _, ok := object["items"]; ok {
		return "array"
	}
	return "unknown"
}

// positionProbeWalker 沿响应 schema 展开 $ref，收集命中关键词的属性名与类型。
// seen 做 schema 名去重，因此循环引用（A→B→A）会自然收敛。
type positionProbeWalker struct {
	root          map[string]any
	seen          map[string]bool
	out           []map[string]any
	propertyTotal int
	matchedTotal  int
	badRefs       int
	truncated     bool
}

func (w *positionProbeWalker) visit(value any, name string, depth int, record bool) {
	if depth > positionProbeSchemaDepth || w.propertyTotal >= positionProbePropertyLimit {
		w.truncated = true
		return
	}
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	if ref, ok := object["$ref"].(string); ok {
		refName := positionProbeRefName(ref)
		if refName == "" {
			// 引用形状非法（超长或含非法字符）：展开不了，如实计入不完整。
			w.badRefs++
			return
		}
		if w.seen[refName] {
			// 循环引用（A→B→A）：这是正常收敛，不算失败。
			return
		}
		resolved, isObject := positionProbeResolve(w.root, ref).(map[string]any)
		if !isObject {
			// 指向不存在的节点、外部引用、或目标不是对象：同样展开不了。
			// 静默跳过会让「零属性命中」被误当成完整扫描后的否定证据。
			w.badRefs++
			return
		}
		w.seen[refName] = true
		w.visit(resolved, refName, depth+1, true)
		return
	}
	properties, _ := object["properties"].(map[string]any)
	if len(properties) > 0 {
		record = true
	}
	var schemaRecord map[string]any
	if record {
		if name == "" {
			name = "inline"
		}
		schemaRecord = map[string]any{
			"schema_name":        name,
			"property_count":     len(properties),
			"matched_properties": []map[string]any{},
		}
		w.out = append(w.out, schemaRecord)
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if w.propertyTotal >= positionProbePropertyLimit {
			if schemaRecord != nil {
				schemaRecord["properties_truncated"] = true
			}
			w.truncated = true
			break
		}
		w.propertyTotal++
		child := properties[key]
		if schemaRecord != nil && positionProbeProperty.MatchString(key) {
			matched, _ := schemaRecord["matched_properties"].([]map[string]any)
			schemaRecord["matched_properties"] = append(matched, map[string]any{
				"name": key, "type": positionProbeType(child, w.root, 0),
			})
			w.matchedTotal++
		}
		if childObject, ok := child.(map[string]any); ok && positionProbeComplex(childObject) {
			w.visit(child, name+"."+key, depth+1, false)
		}
	}
	for _, key := range []string{"items", "additionalProperties"} {
		if child, ok := object[key]; ok {
			w.visit(child, name+"."+key, depth+1, false)
		}
	}
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		list, ok := object[key].([]any)
		if !ok {
			continue
		}
		for _, child := range list {
			w.visit(child, name+"."+key, depth+1, false)
		}
	}
}

func positionProbeComplex(object map[string]any) bool {
	for _, key := range []string{"$ref", "properties", "items", "additionalProperties", "allOf", "oneOf", "anyOf"} {
		if _, ok := object[key]; ok {
			return true
		}
	}
	return false
}

// positionProbeResponseSchema 取一个 operation 的响应 schema，兼容 openapi v2
// 的 responses[code].schema 与 v3 的 responses[code].content[media].schema。
// 响应码与 media type 都先排序再遍历，保证同一份契约每次产出同样的结果。
func positionProbeResponseSchema(operation map[string]any) (any, string) {
	responses, _ := operation["responses"].(map[string]any)
	codes := make([]string, 0, len(responses))
	for code := range responses {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		response, _ := responses[code].(map[string]any)
		if schema, ok := response["schema"]; ok {
			return schema, positionProbeResponseName(schema)
		}
		content, _ := response["content"].(map[string]any)
		media := make([]string, 0, len(content))
		for key := range content {
			media = append(media, key)
		}
		sort.Strings(media)
		for _, key := range media {
			item, _ := content[key].(map[string]any)
			if schema, ok := item["schema"]; ok {
				return schema, positionProbeResponseName(schema)
			}
		}
	}
	return nil, ""
}

func positionProbeResponseName(schema any) string {
	if object, ok := schema.(map[string]any); ok {
		if ref, ok := object["$ref"].(string); ok {
			return positionProbeRefName(ref)
		}
	}
	return "inline"
}

// positionProbeContracts 扫描 openapi 的 paths，对命中谓词的每个 operation 展开
// 响应 schema，产出 path/method/response_schema/matched_properties 记录。
func positionProbeContracts(raw []byte) ([]map[string]any, positionProbeContractStats) {
	var stats positionProbeContractStats
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		stats.InvalidJSON = true
		return []map[string]any{}, stats
	}
	paths, ok := root["paths"].(map[string]any)
	if !ok {
		return []map[string]any{}, stats
	}
	stats.ContractRead, stats.ScannedPaths = true, len(paths)
	keys := []string{}
	for path := range paths {
		if strings.HasPrefix(path, "/") && positionProbeMatches(path) {
			keys = append(keys, path)
		}
	}
	sort.Strings(keys)
	stats.MatchedPaths = len(keys)
	result := []map[string]any{}
	expanded := map[string]bool{}
	for _, path := range keys {
		item, _ := paths[path].(map[string]any)
		for _, method := range positionProbeMethods {
			operation, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			stats.MatchedEndpoints++
			if len(result) >= positionProbeOperationLimit {
				stats.OperationTruncated = true
				continue
			}
			entry := map[string]any{
				"path": path, "method": strings.ToUpper(method), "response_schema": "",
				"matched_properties": []map[string]any{}, "expanded_schemas": []map[string]any{},
				"write_executed": false,
			}
			if id, ok := operation["operationId"].(string); ok && positionProbeSafeName.MatchString(id) {
				entry["operation_id"] = id
			}
			schema, schemaName := positionProbeResponseSchema(operation)
			entry["response_schema"] = schemaName
			if schema != nil {
				walker := &positionProbeWalker{root: root, seen: map[string]bool{}}
				walker.visit(schema, schemaName, 0, false)
				entry["expanded_schemas"] = walker.out
				matched := []map[string]any{}
				for _, schemaItem := range walker.out {
					if properties, ok := schemaItem["matched_properties"].([]map[string]any); ok {
						matched = append(matched, properties...)
					}
					if name, ok := schemaItem["schema_name"].(string); ok && !expanded[name] {
						expanded[name] = true
					}
				}
				entry["matched_properties"] = matched
				entry["schema_truncated"] = walker.truncated
				stats.MatchedProperties += walker.matchedTotal
				stats.SchemaTruncated = stats.SchemaTruncated || walker.truncated
				stats.UnresolvedRefs += walker.badRefs
			} else {
				// 命中了端点、却拿不到可读的响应 schema：一个属性都展开不出来，
				// 这种 operation 的「零命中」不能作为否定证据，必须计入不完整。
				stats.MissingSchemas++
				entry["response_schema_missing"] = true
			}
			result = append(result, entry)
		}
	}
	stats.ExpandedSchemas = len(expanded)
	return result, stats
}

// positionProbeKeyHit 是一条命中的键名记录：只有键名、值类型、以及它所在的
// 对象路径。**绝不含取值**——响应体里可能有召唤师名、puuid、对局数据，
// 一个都不能落进诊断日志。
type positionProbeKeyHit struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Owner string `json:"owner"`
}

// positionProbeResponseScan 是一次响应键名扫描的结果。
type positionProbeResponseScan struct {
	// ValidJSON 为 false 表示响应体不是合法 JSON（例如 gameflow-phase 返回的是
	// 裸字符串），此时「零命中」不构成任何否定证据。
	ValidJSON bool `json:"valid_json"`
	// Scanned 是扫过的键总数，用来区分「扫了 300 个键没命中」与「压根没扫到东西」。
	Scanned   int                   `json:"scanned_keys"`
	Truncated bool                  `json:"keys_truncated"`
	Hits      []positionProbeKeyHit `json:"matched_keys"`
}

// positionProbeResponseKeys 扫描一个 JSON 响应体的**键名**，返回命中
// positionProbeProperty 的那些。这是端点直探的核心：不依赖任何契约文档，
// 直接看客户端此刻真的返回了什么形状。
//
// 深度口径见 positionProbeResponseDepth 的注释（数组穿透不计深度）。
func positionProbeResponseKeys(raw []byte) positionProbeResponseScan {
	scan := positionProbeResponseScan{Hits: []positionProbeKeyHit{}}
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return scan
	}
	scan.ValidJSON = true
	positionProbeWalkResponse(root, "", 0, &scan)
	return scan
}

func positionProbeWalkResponse(value any, owner string, depth int, scan *positionProbeResponseScan) {
	switch node := value.(type) {
	case map[string]any:
		if depth > positionProbeResponseDepth {
			return
		}
		keys := make([]string, 0, len(node))
		for key := range node {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if scan.Scanned >= positionProbeResponseKeyLimit {
				scan.Truncated = true
				return
			}
			scan.Scanned++
			// 键名形状不合法（超长、含控制字符等）就不写进日志，避免把任意
			// 响应内容带进诊断导出。谓词本身只匹配 ASCII 字母，因此这一步
			// 不会漏掉任何真正关心的字段。
			if positionProbeProperty.MatchString(key) && positionProbeSafeName.MatchString(key) {
				scan.Hits = append(scan.Hits, positionProbeKeyHit{
					Name: key, Type: positionProbeJSONType(node[key]), Owner: owner,
				})
			}
			if depth < positionProbeResponseDepth {
				positionProbeWalkResponse(node[key], positionProbeOwnerPath(owner, key), depth+1, scan)
			} else if positionProbeHasChildren(node[key]) {
				// 已到深度上限，但这个值下面还有**未扫描的非空结构**：如实标记不完整。
				// 静默跳过会让「零命中」被当成否定证据，而目标键可能就在更深处
				// （复核给的反例：{"members":[{"profile":{"deep":{"assignedPosition":…}}}]}）。
				// 反之，被跳过的值若是标量或空容器，就没有漏扫任何东西，不该标记。
				scan.Truncated = true
			}
		}
	case []any:
		// 数组穿透不计入深度：lobby.members[]、gameData.teamOne[] 里的目标键都在
		// 数组元素上。元素被上限跳过时**必须**标记 Truncated——同层兄弟元素里可能
		// 就有目标键（例如只有第 3 个成员带 assignedPosition），漏扫却报零命中
		// 会得出假的否定结论。
		for index, item := range node {
			if index >= positionProbeResponseArrayLimit {
				scan.Truncated = true
				break
			}
			positionProbeWalkResponse(item, owner+"[]", depth, scan)
		}
	}
}

// positionProbeJSONType 只回答「这个值是什么类型」，不读取也不返回取值本身。
func positionProbeJSONType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}

// positionProbeHasChildren 判断一个值下面是否还有「值得继续扫的结构」。
// 只用于深度上限处的截断判定：标量与空容器都返回 false，因为跳过它们不会
// 漏掉任何键，把它们算成截断会让关键端点永远失去下否定结论的资格。
func positionProbeHasChildren(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		return len(node) > 0
	case []any:
		return len(node) > 0
	}
	return false
}

// positionProbeOwnerPath 拼接命中键所在的对象路径。祖先键名同样要过
// positionProbeSafeName：否则白名单只保护了命中键名本身，形如
// `{"private name": {"assignedPosition": …}}` 的响应会把带空格的原始键名
// 经 owner 路径带进诊断日志。
func positionProbeOwnerPath(owner, key string) string {
	segment := key
	if !positionProbeSafeName.MatchString(key) {
		segment = positionProbeRedactedSegment
	}
	if owner == "" {
		return segment
	}
	return owner + "." + segment
}

func positionProbeTraceID() string {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(nonce)
}

// collectPositionContractProbe 是本次探测的唯一执行入口（临时入口）。
//
// ⚠️ 一次性侦察代码：真机探测完成、结论写入 docs/r121-position-probe-findings.md
// 后，本函数连同整个文件必须删除，不得进入正式版本。**不接入任何生产路径。**
//
// 触发方式（仅本机、需人工显式开启，绝不自动运行）：
//
//	R121_POSITION_PROBE=1 go test ./backend -run TestR121PositionContractProbeLiveClient -v
//
// 返回值是已写入诊断日志的事件负载，便于本机入口直接归档成一份导出文件。
func (a *app) collectPositionContractProbe(ctx context.Context, client *LCUClient, source string) ([]map[string]any, bool) {
	if a == nil || client == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, positionProbeTotalTimeout)
	defer cancel()
	trace := positionProbeTraceID()
	a.recordDiagnostic(map[string]any{
		"event": "position_contract_probe_start", "probe": positionProbeName, "trace_id": trace,
		"source": source, "mode": "read-only", "direct_endpoints": len(positionProbeEndpoints),
		"contract_sources": len(positionProbeSources), "path_predicate": positionProbePath.String(),
		"property_predicate": positionProbeProperty.String(), "contract_limit_bytes": positionProbeContractLimit,
		"response_key_depth": positionProbeResponseDepth, "response_key_limit": positionProbeResponseKeyLimit,
		"temporary_reconnaissance_code": true,
	})
	events := []map[string]any{}
	endpointSummaries := []map[string]any{}
	contractSummaries := []map[string]any{}
	// 端点直探（主力）的计数。
	endpointsScanned, criticalConclusive, endpointsWithHits := 0, 0, 0
	endpointKeys, endpointHits := 0, 0
	// 契约清单（best-effort 补充）的计数。
	structuredRead, contractHits := false, 0
	contractConclusive := false
	var scannedPaths, matchedPaths, contractEndpoints, expandedSchemas, contractProperties int

	// ---- 第一段：端点直探 ----------------------------------------------------
	// 不依赖任何契约文档：直接 GET 已知端点，扫响应 JSON 的键名。R121 工单的
	// 根因就是「契约清单这条路在本机走不通」（openapi 双 404、/help 的格式与
	// 「扫字面斜杠路径」的假设对不上），所以这一段必须能独立给出可判定结论。
	for _, spec := range positionProbeEndpoints {
		if ctx.Err() != nil {
			break
		}
		status, result, raw, err := positionProbeGet(ctx, client, spec.path)
		event := map[string]any{
			"event": "position_endpoint_probe", "probe": positionProbeName, "trace_id": trace,
			"source": source, "method": "GET", "endpoint_path": spec.path, "endpoint_note": spec.note,
			"endpoint_critical": spec.critical, "status": status, "result": result,
			"body_bytes": len(raw), "write_executed": false, "temporary_reconnaissance_code": true,
		}
		scan := positionProbeResponseScan{Hits: []positionProbeKeyHit{}}
		if err == nil && len(raw) > 0 {
			scan = positionProbeResponseKeys(raw)
			event["valid_json"] = scan.ValidJSON
			event["scanned_keys"] = scan.Scanned
			event["keys_truncated"] = scan.Truncated
			event["matched_keys"] = scan.Hits
			event["matched_key_count"] = len(scan.Hits)
			endpointKeys += scan.Scanned
			endpointHits += len(scan.Hits)
		}
		// 判据护栏一：只有「合法 JSON 且真的扫到了键」才算扫描完成。裸字符串响应
		// （gameflow-phase 返回 "ChampSelect" 这种）ValidJSON 为 true 但 Scanned
		// 为 0，那种「零命中」不是证据，必须排除，否则会得出假的否定结论。
		scanned := scan.ValidJSON && scan.Scanned > 0
		// 判据护栏二：扫描被键数上限截断时，零命中同样不是证据——目标键可能就在
		// 没扫到的那部分里。战绩响应（几十场 × 十人 × 几十个字段）在真机上必然
		// 触发 positionProbeResponseKeyLimit，所以这不是理论情况。
		conclusive := scanned && !scan.Truncated
		event["endpoint_scanned"] = scanned
		event["negative_conclusive"] = conclusive && len(scan.Hits) == 0
		if scanned {
			endpointsScanned++
			if len(scan.Hits) > 0 {
				endpointsWithHits++
			}
		}
		if conclusive && spec.critical {
			criticalConclusive++
		}
		a.recordDiagnostic(event)
		events = append(events, event)
		endpointSummaries = append(endpointSummaries, map[string]any{
			"endpoint_path": spec.path, "endpoint_critical": spec.critical, "status": status,
			"result": result, "endpoint_scanned": scanned, "scanned_keys": scan.Scanned,
			"keys_truncated": scan.Truncated, "matched_key_count": len(scan.Hits),
			"negative_conclusive": conclusive && len(scan.Hits) == 0,
		})
	}

	// ---- 第二段：契约清单（best-effort，404 完全不影响上面的结论）------------
	for _, spec := range positionProbeSources {
		if ctx.Err() != nil {
			break
		}
		status, result, raw, err := positionProbeGet(ctx, client, spec.path)
		event := map[string]any{
			"event": "position_contract_probe", "probe": positionProbeName, "trace_id": trace,
			"source": source, "method": "GET", "source_path": spec.path, "contract_kind": spec.kind,
			"status": status, "result": result, "body_bytes": len(raw), "write_executed": false,
			"operation_limit": positionProbeOperationLimit, "temporary_reconnaissance_code": true,
		}
		// 契约来源的判据必须用**属性命中数**（MatchedProperties），不能用端点数
		// （MatchedEndpoints）：后者只说明「契约里存在这个路径」，与「这个路径的
		// schema 里有没有位置字段」毫无关系。用错会让「openapi 里有
		// /lol-lobby/v2/lobby」被误判成「找到了位置偏好字段」。
		count, contractRead, contractProps := 0, false, 0
		contractComplete := false
		if err == nil && len(raw) > 0 {
			records, stats := positionProbeContracts(raw)
			count, contractRead, contractProps = stats.MatchedEndpoints, stats.ContractRead, stats.MatchedProperties
			// 契约被截断时零命中不能当否定证据：目标属性可能就在没展开的那部分里。
			// 契约「完整」的判据：读到了、schema 与 operation 明细都没被截断、每个
			// 命中端点都有可解析的响应 schema、且所有 $ref 都解析成功。任一条不满足，
			// 「零属性命中」就不能作为否定证据——那只是没展开出来，不是确实没有。
			contractComplete = stats.ContractRead && !stats.SchemaTruncated && !stats.OperationTruncated &&
				stats.MissingSchemas == 0 && stats.UnresolvedRefs == 0
			event["scanned_paths"] = stats.ScannedPaths
			event["matched_paths"] = stats.MatchedPaths
			event["matched_endpoints"] = stats.MatchedEndpoints
			event["expanded_schemas"] = stats.ExpandedSchemas
			event["matched_properties"] = stats.MatchedProperties
			event["schema_truncated"] = stats.SchemaTruncated
			event["operations_truncated"] = stats.OperationTruncated
			event["missing_response_schemas"] = stats.MissingSchemas
			event["unresolved_refs"] = stats.UnresolvedRefs
			event["operations"] = records
			event["invalid_json"] = stats.InvalidJSON
			scannedPaths += stats.ScannedPaths
			matchedPaths += stats.MatchedPaths
			contractEndpoints += stats.MatchedEndpoints
			expandedSchemas += stats.ExpandedSchemas
			contractProperties += stats.MatchedProperties
		}
		sourceConclusive := contractComplete && contractProps == 0
		event["contract_read"] = contractRead
		event["contract_complete"] = contractComplete
		event["negative_conclusive"] = sourceConclusive
		if contractRead {
			structuredRead = true
			contractHits += contractProps
		}
		if sourceConclusive {
			contractConclusive = true
		}
		a.recordDiagnostic(event)
		events = append(events, event)
		contractSummaries = append(contractSummaries, map[string]any{
			"source_path": spec.path, "contract_kind": spec.kind, "status": status, "result": result,
			"count": count, "matched_properties": contractProps, "contract_read": contractRead,
			"contract_complete": contractComplete, "negative_conclusive": sourceConclusive,
		})
	}

	verdict, reason := positionProbeVerdict(endpointHits, contractHits, criticalConclusive, contractConclusive)
	final := map[string]any{
		"event": "position_contract_probe_summary", "probe": positionProbeName, "trace_id": trace,
		"source": source, "verdict": verdict, "inconclusive_reason": reason,
		"endpoints": endpointSummaries, "sources": contractSummaries,
		// endpoints_probed 用实际产出的事件数，不用 len(positionProbeEndpoints)：
		// ctx 中途取消时循环会 break，固定值会谎报「六个端点都探过了」。
		"endpoints_probed": len(endpointSummaries), "endpoints_scanned": endpointsScanned,
		"endpoints_critical_conclusive": criticalConclusive, "endpoints_with_hits": endpointsWithHits,
		"endpoint_keys_scanned": endpointKeys, "endpoint_matched_keys": endpointHits,
		"contract_read_structured": structuredRead, "contract_matched_properties": contractHits,
		"contract_conclusive": contractConclusive, "scanned_paths": scannedPaths,
		"matched_paths": matchedPaths, "matched_endpoints": contractEndpoints,
		"expanded_schemas": expandedSchemas, "matched_properties": contractProperties,
		// negative_conclusive_all 保留旧字段名（便于与上一轮真机日志对照），
		// 但口径已收紧：只认「证据完整且零命中」，见 positionProbeVerdict。
		// 以下三行刻意写成单键并对齐，与 augment_contract_probe.go 的同款形状一致，
		// 避免 gofmt 因单键行与多键行混排而重排空白。
		"negative_conclusive_all":       verdict == positionProbeVerdictNotFound,
		"write_executed":                false,
		"temporary_reconnaissance_code": true,
	}
	a.recordDiagnostic(final)
	events = append(events, final)
	return events, true
}

// positionProbeGet 发一个只读 GET，并把错误分类成诊断可用的状态。
// 永远不写客户端；返回的 raw 只用于键名与结构扫描，取值一律不落日志。
func positionProbeGet(ctx context.Context, client *LCUClient, path string) (int, string, []byte, error) {
	requestCtx, stop := context.WithTimeout(ctx, positionProbeRequestTimeout)
	raw, err := client.getBytes(requestCtx, path, positionProbeContractLimit, "application/json")
	stop()
	if err == nil {
		return http.StatusOK, "ok", raw, nil
	}
	status, result := 0, "unavailable"
	var httpErr *LCUHTTPError
	if errors.As(err, &httpErr) {
		status = httpErr.StatusCode
	}
	if errors.Is(err, errResponseLimitExceeded) {
		result = "too-large"
	}
	return status, result, raw, err
}

const (
	positionProbeVerdictFound        = "found"
	positionProbeVerdictNotFound     = "not-found"
	positionProbeVerdictInconclusive = "inconclusive"
)

// positionProbeVerdict 把本轮证据收敛成三态结论。
//
// R121 工单 §0 的核心教训是：上一轮真机 `contract_read_any=true`（/help 确实
// 返回了 200 和 3 MB 内容），但结论其实是「压根没查到」，日志却容易被读成
// 「查了、确实没有」。这里把「读到东西」与「能下结论」彻底分开。
//
// 四个入参是两组彼此独立的证据，各自分「肯定」与「可作否定依据」两面：
//
//	endpointHits       端点响应里命中谓词的键总数（肯定证据）
//	contractHits       契约 schema 里命中谓词的属性总数（肯定证据）
//	criticalConclusive 证据完整的**关键端点**个数：200 + 合法 JSON + 扫到键 + 未截断
//	contractConclusive 是否有契约来源被完整读取且零命中
//
// 三态：
//
//	found        —— 任一组肯定证据 > 0；
//	not-found    —— 没有肯定证据，但至少一路否定依据成立；
//	inconclusive —— 既没命中，也没有任何能支撑否定结论的完整扫描。
//
// 必须判 inconclusive 而不能判 not-found 的三种情况：
//   - 只有非关键端点可读（例如只拿到战绩响应）：位置偏好最可能落在大厅 / 选人
//     对象里，那两个没看到就不能说「客户端没有这个字段」；
//   - 关键端点的扫描被数组元素上限或键数上限截断：目标键可能就在没扫到的部分里；
//   - 契约里存在目标端点、但 schema 未完整展开：同理。
//
// 注意「契约里存在目标端点」本身**不是**肯定证据：那只代表路径存在，与 schema
// 里有没有位置字段无关。上一版实现把 MatchedEndpoints 当成了命中数，会让
// 「openapi 里有 /lol-lobby/v2/lobby」被误判成 found，现已改为只看属性命中数。
func positionProbeVerdict(endpointHits, contractHits, criticalConclusive int, contractConclusive bool) (string, string) {
	if endpointHits > 0 || contractHits > 0 {
		return positionProbeVerdictFound, ""
	}
	if criticalConclusive > 0 || contractConclusive {
		return positionProbeVerdictNotFound, ""
	}
	return positionProbeVerdictInconclusive, "关键端点（大厅 / 选人 / 对局会话）都没有完整可读的响应，契约文档也未读到或未完整展开；本轮无法就位置偏好下任何结论，请在客户端处于大厅或选人阶段时重跑"
}
