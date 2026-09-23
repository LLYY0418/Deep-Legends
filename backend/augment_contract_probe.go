package main

// ============================================================================
// R116-探测：一次性侦察代码（TEMPORARY RECONNAISSANCE — 探测结束后整体删除）
//
// 工单：docs/history/worklists/R116-探测-海克斯选择端点与局内数据形状探测-工单.md
// 结论落盘处：docs/r116-probe-findings.md
//
// 目的只有一个：一次性回答「本机 LCU 客户端的接口契约里，是否存在任何暴露
// 海克斯（斗魂 CHERRY / 海斗 KIWI·ARAM_MAYHEM）三选一候选项的端点」，为
// R116-D 的 P1-1 去留提供真实依据。
//
// 这不是产品功能，必须整体删除：
//   - 本文件与 augment_contract_probe_test.go 在真机探测执行完毕、结论写入
//     docs/r116-probe-findings.md 之后立即删除，不进入正式版本；
//   - gameplay.go 里同批新增的 `aramMode && !arenaMode` 触发分支按工单 P1
//     第 4 条一并评估保留/删除。
//
// 安全边界（与 objective_diagnostics.go 的既有约定一致）：
//   - 只读：只发 GET，永不写客户端，事件里恒带 write_executed=false；
//   - 只记录契约形状（path / method / 参数名 / 字段名与类型 / 响应码），
//     不记录任何响应体取值、账号标识、召唤师名或对局数据；
//   - 不复用也绝不修改 objectiveDiagnosticPath（任务徽章功能的既有护栏），
//     本文件自带独立谓词 augmentProbePath。
// ============================================================================

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// augmentProbePath 是本次探测专用的路径谓词，独立于 objectiveDiagnosticPath。
// 工单原文指定：`(?i)cherry|augment|mayhem`。匹配的是路径本体（query 之前），
// 因此 `/help?format=Full` 这类探测自身的请求路径不会自我命中。
var augmentProbePath = regexp.MustCompile(`(?i)cherry|augment|mayhem`)

// /help?format=Full 是 functions/events/types JSON，不能用 REST 路径 token 判定。
var augmentProbeHelpValue = regexp.MustCompile(`^[A-Za-z0-9_./:{}-]{1,512}$`)

// augmentProbeSafeToken 限定可写入诊断日志的 token 形状（响应码、根对象键名）。
var augmentProbeSafeToken = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,64}$`)

const (
	augmentProbeName = "r116-augment-endpoint-inventory"
	// 契约文档体积上限：照抄 objective_diagnostics.go:377-380 的 16 MiB 限流写法。
	augmentProbeContractLimit = int64(16 << 20)
	// operations 明细的保留上限。注意 count 统计的是全部命中 operation，
	// 不受该上限影响，否则「count == 0」这条判据会被截断污染。
	augmentProbeOperationLimit = 200
	augmentProbeParameterLimit = 20
	augmentProbeResponseLimit  = 12
	augmentProbeRootKeyLimit   = 40
	// 真机样本合计 5,783 个条目。超限时不得下否定结论。
	augmentProbeHelpElementLimit = 10000
	augmentProbeHelpNodeLimit    = 100000
	// 单次请求与整轮探测的超时。这是一次性人工侦察，不在任何实时链路上，
	// 因此比既有诊断的 1500ms 宽松，以保证 3 MB 契约能完整读完。
	augmentProbeRequestTimeout = 10 * time.Second
	augmentProbeTotalTimeout   = 45 * time.Second
	// 路径字面量的可记录长度上限，与 objectiveDiagnosticPath 一致。
	augmentProbePathLimit = 512
)

// augmentProbeSources 是探测的契约来源，按权威度排序。
// 前两个是工单指定的 openapi.json（主来源）；第三个是 R82 真机观测证明
// 本机唯一 200 的契约文档，作为兜底，避免把 404 误读成「确证无端点」。
var augmentProbeSources = []struct {
	path string
	kind string
}{
	{"/swagger/v3/openapi.json", "openapi-v3"},
	{"/swagger/v2/swagger.json", "openapi-v2"},
	{"/help?format=Full", "help-full"},
}

// augmentProbeMatches 判定一个契约路径是否属于本次探测关心的命名空间。
func augmentProbeMatches(path string) bool {
	if path == "" || len(path) > augmentProbePathLimit {
		return false
	}
	return augmentProbePath.MatchString(strings.SplitN(path, "?", 2)[0])
}

// augmentProbeContractStats 是一次 openapi 扫描的计数证据。
type augmentProbeContractStats struct {
	ScannedPaths      int  `json:"scanned_paths"`
	MatchedPaths      int  `json:"matched_paths"`
	MatchedOperations int  `json:"count"`
	Retained          int  `json:"retained"`
	Truncated         bool `json:"operations_truncated"`
	RejectedPaths     int  `json:"rejected_paths"`
	InvalidJSON       bool `json:"invalid_json"`
}

// augmentProbeContracts 复用 objectiveContractSchema，对 openapi 的 paths 做
// 全量扫描（不限定 /lol-missions/ 等前缀），只把谓词命中的 operation 汇总成
// 与 objective_badge_contracts 同构的 path/method/parameters/body 记录。
func augmentProbeContracts(raw []byte) ([]map[string]any, augmentProbeContractStats) {
	var stats augmentProbeContractStats
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		stats.InvalidJSON = true
		return []map[string]any{}, stats
	}
	paths, _ := root["paths"].(map[string]any)
	stats.ScannedPaths = len(paths)
	keys := make([]string, 0, 8)
	for key := range paths {
		// 只有形状可信的绝对路径才允许把字面量写进日志；其余仍计数，
		// 保证「count == 0」不会因为拒绝记录而变成假阴性。
		if !strings.HasPrefix(key, "/") || len(key) > augmentProbePathLimit {
			stats.RejectedPaths++
			continue
		}
		if !augmentProbeMatches(key) {
			continue
		}
		stats.MatchedPaths++
		keys = append(keys, key)
	}
	sort.Strings(keys)
	operations := make([]map[string]any, 0, len(keys))
	for _, path := range keys {
		item, _ := paths[path].(map[string]any)
		for _, method := range []string{"get", "put", "patch", "post", "delete", "head", "options"} {
			operation, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			stats.MatchedOperations++
			if len(operations) >= augmentProbeOperationLimit {
				stats.Truncated = true
				continue
			}
			operations = append(operations, augmentProbeOperation(path, method, operation, root))
		}
	}
	stats.Retained = len(operations)
	return operations, stats
}

// augmentProbeOperation 提取单个 operation 的静态契约形状。
// 字段名与 objective_badge_contracts 保持一致：path / method / parameters / body。
func augmentProbeOperation(path, method string, operation, root map[string]any) map[string]any {
	entry := map[string]any{"path": path, "method": strings.ToUpper(method)}
	parameters := []map[string]any{}
	list, _ := operation["parameters"].([]any)
	for _, item := range list {
		parameter, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := parameter["name"].(string)
		if !objectiveFieldName.MatchString(name) || len(parameters) >= augmentProbeParameterLimit {
			continue
		}
		record := map[string]any{"name": name, "schema": objectiveContractSchema(parameter["schema"], root, 0)}
		if location, ok := parameter["in"].(string); ok && augmentProbeSafeToken.MatchString(location) {
			record["in"] = location
		}
		if required, ok := parameter["required"].(bool); ok {
			record["required"] = required
		}
		parameters = append(parameters, record)
	}
	entry["parameters"] = parameters
	body, _ := operation["requestBody"].(map[string]any)
	content, _ := body["content"].(map[string]any)
	application, _ := content["application/json"].(map[string]any)
	if schema := objectiveContractSchema(application["schema"], root, 0); schema != nil {
		entry["body"] = schema
	}
	if responses, ok := operation["responses"].(map[string]any); ok {
		codes := make([]string, 0, len(responses))
		for code := range responses {
			if augmentProbeSafeToken.MatchString(code) {
				codes = append(codes, code)
			}
		}
		sort.Strings(codes)
		if len(codes) > augmentProbeResponseLimit {
			codes = codes[:augmentProbeResponseLimit]
		}
		entry["response_codes"] = codes
	}
	return entry
}

type augmentProbeHelpStats struct {
	Lengths        map[string]int
	Scanned        map[string]int
	ElementKeys    map[string][]string
	Count          int
	StaticCount    int
	Truncated      bool
	LimitReached   bool
	InvalidJSON    bool
	InvalidElement bool
	Complete       bool
}

// 仅记录契约名字或去掉 query/host 的路径，不记录响应体中的其他取值。
func augmentProbeHelpSafeValue(raw string) string {
	value := strings.TrimSpace(raw)
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return ""
		}
		value = parsed.Path
	}
	value = strings.SplitN(value, "?", 2)[0]
	if !augmentProbeHelpValue.MatchString(value) || !augmentProbePath.MatchString(value) {
		return ""
	}
	return value
}

func augmentProbeStaticCatalog(value string) bool {
	return strings.Contains(strings.ToLower(value), "cherry-augments.json") || strings.HasSuffix(strings.ToLower(value), "/perks.json")
}

// 扫描 help-full 的三个真实数组；任何结构缺失或扫描上限都会撤销否定资格。
func augmentProbeHelpDocument(raw []byte) ([]map[string]any, []map[string]any, augmentProbeHelpStats) {
	stats := augmentProbeHelpStats{Lengths: map[string]int{}, Scanned: map[string]int{}, ElementKeys: map[string][]string{}}
	matches, static := []map[string]any{}, []map[string]any{}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil || root == nil {
		stats.InvalidJSON = true
		return matches, static, stats
	}
	arraysPresent := true
	totalElements, totalNodes := 0, 0
	for _, group := range []string{"functions", "events", "types"} {
		items, ok := root[group].([]any)
		if !ok {
			arraysPresent = false
			continue
		}
		stats.Lengths[group] = len(items)
		keys := map[string]struct{}{}
		for _, item := range items {
			if totalElements >= augmentProbeHelpElementLimit {
				stats.LimitReached = true
				break
			}
			totalElements++
			stats.Scanned[group]++
			object, ok := item.(map[string]any)
			if !ok {
				stats.InvalidElement = true
				continue
			}
			for key := range object {
				if augmentProbeSafeToken.MatchString(key) {
					keys[key] = struct{}{}
				}
			}
			var visit func(any, int)
			visit = func(value any, depth int) {
				totalNodes++
				if totalNodes > augmentProbeHelpNodeLimit || depth > 16 {
					stats.LimitReached = true
					return
				}
				switch typed := value.(type) {
				case map[string]any:
					for key, child := range typed {
						if key == "name" && depth == 0 || strings.EqualFold(key, "url") || strings.EqualFold(key, "path") || strings.EqualFold(key, "uri") {
							if rawValue, ok := child.(string); ok {
								if safe := augmentProbeHelpSafeValue(rawValue); safe != "" {
									entry := map[string]any{"array": group, "field": key, "value": safe}
									if augmentProbeStaticCatalog(safe) {
										stats.StaticCount++
										if len(static) < augmentProbeOperationLimit {
											static = append(static, entry)
										} else {
											stats.Truncated = true
										}
									} else {
										stats.Count++
										if len(matches) < augmentProbeOperationLimit {
											matches = append(matches, entry)
										} else {
											stats.Truncated = true
										}
									}
								}
							}
						}
						if totalNodes <= augmentProbeHelpNodeLimit {
							visit(child, depth+1)
						}
					}
				case []any:
					for _, child := range typed {
						if totalNodes <= augmentProbeHelpNodeLimit {
							visit(child, depth+1)
						}
					}
				}
			}
			visit(object, 0)
		}
		for key := range keys {
			stats.ElementKeys[group] = append(stats.ElementKeys[group], key)
		}
		sort.Strings(stats.ElementKeys[group])
	}
	stats.Complete = arraysPresent && !stats.InvalidElement && !stats.LimitReached && !stats.Truncated
	for _, group := range []string{"functions", "events", "types"} {
		if stats.Scanned[group] != stats.Lengths[group] {
			stats.Complete = false
		}
	}
	return matches, static, stats
}

// augmentProbeRootShape 记录契约文档根对象的形状（键名 + 值类型 + 元素个数），
// 用于回答「200 但读不懂」这类情况。R82 第 4.2 节的第 1 条建议：先打形状，别再猜。
// 只有键名与类型，没有任何取值。
func augmentProbeRootShape(raw []byte) map[string]any {
	shape := map[string]any{}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		var array []any
		if json.Unmarshal(raw, &array) == nil {
			shape["json_root"] = "array"
			shape["json_root_count"] = len(array)
		} else {
			shape["json_root"] = "unparsed"
		}
		return shape
	}
	shape["json_root"] = "object"
	keys := make([]string, 0, len(root))
	for key := range root {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	types := map[string]any{}
	truncated := false
	for index, key := range keys {
		if index >= augmentProbeRootKeyLimit {
			truncated = true
			break
		}
		if !augmentProbeSafeToken.MatchString(key) {
			continue
		}
		switch value := root[key].(type) {
		case map[string]any:
			types[key] = "object:" + strconv.Itoa(len(value))
		case []any:
			types[key] = "array:" + strconv.Itoa(len(value))
		case string:
			types[key] = "string"
		case bool:
			types[key] = "boolean"
		case float64:
			types[key] = "number"
		case nil:
			types[key] = "null"
		default:
			types[key] = "other"
		}
	}
	shape["root_keys"] = keys[:min(len(keys), augmentProbeRootKeyLimit)]
	shape["root_key_count"] = len(keys)
	shape["root_keys_truncated"] = truncated
	shape["root_shape"] = types
	return shape
}

func augmentProbeTraceID() string {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(nonce)
}

// collectAugmentContractProbe 是本次探测的唯一执行入口（临时入口）。
//
// ⚠️ 一次性侦察代码：真机探测完成、结论写入 docs/r116-probe-findings.md 后，
// 本函数连同整个文件必须删除，不得进入正式版本。
//
// 触发方式（仅本机、需人工显式开启，绝不自动运行）：
//
//	R116_AUGMENT_PROBE=1 go test ./backend -run TestR116AugmentContractProbeLiveClient -v
//
// 见 augment_contract_probe_test.go 里的 TestR116AugmentContractProbeLiveClient。
// 返回值是已写入诊断日志的事件负载，便于本机入口直接归档成一份导出文件。
func (a *app) collectAugmentContractProbe(ctx context.Context, client *LCUClient, source string) ([]map[string]any, bool) {
	if a == nil || client == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, augmentProbeTotalTimeout)
	defer cancel()
	trace := augmentProbeTraceID()
	a.recordDiagnostic(map[string]any{
		"event": "augment_contract_probe_start", "probe": augmentProbeName, "trace_id": trace,
		"source": source, "mode": "read-only", "sources": len(augmentProbeSources),
		"path_predicate": augmentProbePath.String(), "contract_limit_bytes": augmentProbeContractLimit,
		"temporary_reconnaissance_code": true,
	})
	events := []map[string]any{}
	summary := []map[string]any{}
	readAny, negativeAny := false, false
	for _, spec := range augmentProbeSources {
		if ctx.Err() != nil {
			break
		}
		requestCtx, stop := context.WithTimeout(ctx, augmentProbeRequestTimeout)
		raw, err := client.getBytes(requestCtx, spec.path, augmentProbeContractLimit, "application/json")
		stop()
		status, result := http.StatusOK, "ok"
		if err != nil {
			status, result = 0, "unavailable"
			var httpErr *LCUHTTPError
			if errors.As(err, &httpErr) {
				status = httpErr.StatusCode
			}
			if errors.Is(err, errResponseLimitExceeded) {
				result = "too-large"
			}
		}
		event := map[string]any{
			"event": "augment_contract_probe", "probe": augmentProbeName, "trace_id": trace,
			"source": source, "method": "GET", "source_path": spec.path, "contract_kind": spec.kind,
			"status": status, "result": result, "body_bytes": len(raw), "write_executed": false,
			"operation_limit": augmentProbeOperationLimit, "temporary_reconnaissance_code": true,
		}
		count, contractRead := 0, false
		if err == nil && len(raw) > 0 {
			if spec.kind == "help-full" {
				matched, static, stats := augmentProbeHelpDocument(raw)
				count = stats.Count
				event["count"] = count
				event["matched_items"] = matched
				event["static_catalog_hits"] = static
				event["static_catalog_count"] = stats.StaticCount
				event["matches_truncated"] = stats.Truncated
				event["scan_limit_reached"] = stats.LimitReached
				event["invalid_json"] = stats.InvalidJSON
				event["invalid_element"] = stats.InvalidElement
				for _, group := range []string{"functions", "events", "types"} {
					event[group+"_length"] = stats.Lengths[group]
					event[group+"_scanned"] = stats.Scanned[group]
					event[group+"_element_keys"] = stats.ElementKeys[group]
				}
				contractRead = stats.Complete
				for key, value := range augmentProbeRootShape(raw) {
					event[key] = value
				}
			} else {
				operations, stats := augmentProbeContracts(raw)
				count = stats.MatchedOperations
				event["count"] = stats.MatchedOperations
				event["scanned_paths"] = stats.ScannedPaths
				event["matched_paths"] = stats.MatchedPaths
				event["rejected_paths"] = stats.RejectedPaths
				event["operations"] = operations
				event["operations_retained"] = stats.Retained
				event["operations_truncated"] = stats.Truncated
				event["invalid_json"] = stats.InvalidJSON
				contractRead = !stats.InvalidJSON && stats.ScannedPaths > 0
			}
		}
		// 判据护栏（写进事件本身，避免读日志的人把 404 当成「确证无端点」）：
		// 只有真的读到契约内容时，count == 0 才构成否定证据。
		event["contract_read"] = contractRead
		event["negative_conclusive"] = contractRead && count == 0
		if contractRead {
			readAny = true
			negativeAny = negativeAny || count == 0
		}
		a.recordDiagnostic(event)
		events = append(events, event)
		summary = append(summary, map[string]any{
			"source_path": spec.path, "contract_kind": spec.kind, "status": status,
			"result": result, "count": count, "contract_read": contractRead,
			"negative_conclusive": contractRead && count == 0,
		})
	}
	final := map[string]any{
		"event": "augment_contract_probe_summary", "probe": augmentProbeName, "trace_id": trace,
		"source": source, "sources": summary, "contract_read_any": readAny,
		// 全部可读来源都为 0 命中，才允许写「确证客户端没有海克斯端点」。
		"negative_conclusive_all":       readAny && negativeAny && augmentProbeAllZero(summary),
		"write_executed":                false,
		"temporary_reconnaissance_code": true,
	}
	a.recordDiagnostic(final)
	events = append(events, final)
	return events, true
}

// augmentProbeAllZero 判定所有「真的读到契约」的来源是否都零命中。
func augmentProbeAllZero(summary []map[string]any) bool {
	for _, entry := range summary {
		read, _ := entry["contract_read"].(bool)
		if !read {
			continue
		}
		if count, ok := entry["count"].(int); ok && count > 0 {
			return false
		}
	}
	return true
}
