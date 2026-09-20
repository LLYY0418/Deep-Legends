package main

// This is a read-only investigation, NOT a badge-clearing implementation. Mission
// reward selection and marking notifications as read must never be conflated.
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type objectiveDiagnostics struct {
	mu               sync.Mutex
	trace            string
	until            time.Time
	sequence, events int
	collecting       bool
	last             map[string]string
}

// The websocket callback must never parse a large snapshot or wait for disk IO.
// One bounded worker per connected session preserves event order; overload is
// explicitly logged, rather than spawning unbounded goroutines.
func (a *app) objectiveEventRecorder(ctx context.Context, client *LCUClient) func(LCUEvent) {
	queue := make(chan LCUEvent, 16)
	var dropped atomic.Int64
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-queue:
				a.recordObjectiveState(client, event.URI, "websocket", event.EventType, event.Data)
				if count := dropped.Swap(0); count > 0 {
					a.recordDiagnostic(map[string]any{"event": "objective_badge_dropped", "count": count, "reason": "bounded-event-queue"})
				}
			}
		}
	}()
	return func(event LCUEvent) {
		if objectiveDiagnosticPath(event.URI) == "" || ctx.Err() != nil {
			return
		}
		if len(event.Data) > 2<<20 {
			dropped.Add(1)
			return
		}
		select {
		case queue <- event:
		default:
			dropped.Add(1)
		}
	}
}

var objectiveFieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var objectiveRelevantField = regexp.MustCompile(`(?i)view|read|seen|acknowledg|isnew|notif|badge|progress|count`)

// Only static namespaces are retained. All other URI segments are placeholders,
// even short IDs; lcuDiagnosticPath alone does not redact short opaque IDs.
func objectiveDiagnosticPath(path string) string {
	if len(path) > 512 {
		return ""
	}
	path = strings.ToLower(strings.SplitN(path, "?", 2)[0])
	for _, prefix := range []string{"/lol-missions/v1/", "/lol-notifications/v1/", "/lol-event-hub/v1/"} {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
		for i, part := range parts {
			switch part {
			case "missions", "series", "data", "player", "notifications", "events", "reward-track", "items", "bonus-items", "reward-groups", "viewed", "unread", "count":
			default:
				parts[i] = "{id}"
			}
		}
		return prefix + strings.Join(parts, "/")
	}
	return ""
}

// No response strings, mission/reward IDs, descriptions or account details.
// Field paths/types reveal the contract; booleans and progress/count numbers
// reveal before/after read state. A per-session salted reference joins records.
func objectiveStateShape(raw []byte, salt string) map[string]any {
	result := map[string]any{"bytes": len(raw)}
	if len(raw) > 2<<20 {
		result["result"] = "too-large"
		return result
	}
	var root any
	if json.Unmarshal(raw, &root) != nil {
		result["result"] = "invalid-json"
		return result
	}
	fields := map[string]map[string]int{}
	values := []map[string]any{}
	nodes, omitted := 0, 0
	var walk func(any, string, string, int)
	walk = func(value any, path, ref string, depth int) {
		nodes++
		if depth > 8 || nodes > 12000 {
			omitted++
			return
		}
		switch object := value.(type) {
		case []any:
			if path == "$" {
				result["root_count"] = len(object)
			}
			for _, child := range object {
				walk(child, path+"[]", ref, depth+1)
			}
		case map[string]any:
			if id, ok := object["id"].(string); ok && id != "" {
				digest := sha256.Sum256([]byte(salt + "|" + id))
				ref = hex.EncodeToString(digest[:8])
			}
			keys := make([]string, 0, len(object))
			for key := range object {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				child := object[key]
				if !objectiveFieldName.MatchString(key) {
					omitted++
					continue
				}
				// Only known containers are traversed; unknown field names/values
				// may themselves contain personal data and are never serialized.
				switch strings.ToLower(key) {
				case "missions", "series", "objectives", "progress", "data", "notifications", "events", "eventinfo", "items", "state", "player", "payload":
					walk(child, path+"."+key, ref, depth+1)
				default:
					if !objectiveRelevantField.MatchString(key) {
						continue
					}
					name := path + "." + key
					typ := "other"
					switch child.(type) {
					case bool:
						typ = "boolean"
					case float64:
						typ = "number"
					case string:
						typ = "string-redacted"
					case nil:
						typ = "null"
					}
					if len(fields) >= 128 && fields[name] == nil {
						omitted++
						continue
					}
					if fields[name] == nil {
						fields[name] = map[string]int{}
					}
					fields[name][typ]++
					if text, ok := child.(string); ok {
						switch text {
						case "READ", "UNREAD", "SEEN", "UNSEEN", "NEW", "ACKNOWLEDGED":
							typ = "read-state-enum"
						}
					}
					if typ == "boolean" || typ == "number" || typ == "read-state-enum" {
						if len(values) < 256 {
							values = append(values, map[string]any{"field": name, "ref": ref, "value": child})
						} else {
							omitted++
						}
					}
				}
			}
		case bool, float64:
			if path == "$" {
				result["scalar"] = object
			}
		}
	}
	walk(root, "$", "", 0)
	result["result"], result["fields"], result["values"], result["omitted"] = "ok", fields, values, omitted
	return result
}

func (a *app) recordObjectiveState(client *LCUClient, uri, source, eventType string, raw []byte) {
	path := objectiveDiagnosticPath(uri)
	if path == "" {
		return
	}
	d := &client.objectiveDiagnostics
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.trace == "" || (source == "websocket" && (time.Now().After(d.until) || d.events >= 300)) {
		return
	}
	shape := objectiveStateShape(raw, d.trace)
	encoded, _ := json.Marshal(shape)
	hash := sha256.Sum256(encoded)
	signature := hex.EncodeToString(hash[:8])
	previous := d.last[path]
	if source == "websocket" && previous == signature {
		return
	}
	d.sequence++
	if source == "websocket" {
		d.events++
	}
	d.last[path] = signature
	switch eventType {
	case "Create", "Update", "Delete":
	default:
		eventType = "snapshot"
	}
	a.recordDiagnostic(map[string]any{"event": "objective_badge_state", "trace_id": d.trace, "sequence": d.sequence, "path": path, "source": source, "event_type": eventType, "shape": shape, "state_digest": signature, "previous_digest": previous, "changed": previous != "" && previous != signature, "capture_limit_reached": d.events == 300})
}

// Swagger is returned by the running client, not a guessed web API version.
// Read both v2 and v3 formats. Emit static request property names/types only,
// resolving local refs; never persist the complete schema or example/defaults.
func objectiveContractSchema(value any, root map[string]any, depth int) any {
	if depth > 5 {
		return "depth-limit"
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	if ref, ok := m["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
		var target any = root
		for _, segment := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			obj, ok := target.(map[string]any)
			if !ok {
				return nil
			}
			target = obj[segment]
		}
		return objectiveContractSchema(target, root, depth+1)
	}
	out := map[string]any{}
	if typ, ok := m["type"].(string); ok {
		switch typ {
		case "array", "object", "boolean", "integer", "number", "string":
			out["type"] = typ
		}
	}
	if props, ok := m["properties"].(map[string]any); ok {
		fields := map[string]any{}
		keys := make([]string, 0, len(props))
		for key := range props {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if len(fields) >= 80 {
				break
			}
			if objectiveFieldName.MatchString(key) {
				fields[key] = objectiveContractSchema(props[key], root, depth+1)
			}
		}
		out["properties"] = fields
	}
	if items, ok := m["items"]; ok {
		out["items"] = objectiveContractSchema(items, root, depth+1)
	}
	return out
}

func objectiveContracts(raw []byte) []map[string]any {
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	paths, _ := root["paths"].(map[string]any)
	keys := make([]string, 0, len(paths))
	for key := range paths {
		if objectiveDiagnosticPath(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := []map[string]any{}
	for _, path := range keys {
		operations, _ := paths[path].(map[string]any)
		for _, method := range []string{"get", "put", "patch", "post", "delete"} {
			op, ok := operations[method].(map[string]any)
			if !ok {
				continue
			}
			if len(result) >= 120 {
				return result
			}
			entry := map[string]any{"path": objectiveDiagnosticPath(path), "method": strings.ToUpper(method)}
			params := []map[string]any{}
			list, _ := op["parameters"].([]any)
			for _, p := range list {
				param, ok := p.(map[string]any)
				if !ok {
					continue
				}
				name, _ := param["name"].(string)
				if !objectiveFieldName.MatchString(name) || len(params) >= 20 {
					continue
				}
				item := map[string]any{"name": name, "schema": objectiveContractSchema(param["schema"], root, 0)}
				if required, ok := param["required"].(bool); ok {
					item["required"] = required
				}
				params = append(params, item)
			}
			entry["parameters"] = params
			body, _ := op["requestBody"].(map[string]any)
			content, _ := body["content"].(map[string]any)
			application, _ := content["application/json"].(map[string]any)
			if schema := objectiveContractSchema(application["schema"], root, 0); schema != nil {
				entry["body"] = schema
			}
			result = append(result, entry)
		}
	}
	return result
}

func (a *app) collectObjectiveDiagnostics(ctx context.Context, client *LCUClient, source string) bool {
	if client == nil || a.storage == nil {
		return false
	}
	d := &client.objectiveDiagnostics
	d.mu.Lock()
	if d.collecting {
		d.mu.Unlock()
		return false
	}
	d.collecting = true
	if d.trace == "" {
		nonce := make([]byte, 12)
		if _, err := rand.Read(nonce); err != nil {
			d.collecting = false
			d.mu.Unlock()
			return false
		}
		d.trace = hex.EncodeToString(nonce)
		d.last = map[string]string{}
	}
	if source != "export" {
		d.until = time.Now().Add(2 * time.Minute)
		d.events = 0
		d.last = map[string]string{}
	}
	trace := d.trace
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.collecting = false
		if source != "export" {
			d.until = time.Now().Add(2 * time.Minute)
		}
		d.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	a.recordDiagnostic(map[string]any{"event": "objective_badge_capture", "trace_id": trace, "source": source, "mode": "read-only", "event_window_seconds": 120, "event_limit": 300})
	for _, path := range []string{"/lol-missions/v1/missions", "/lol-missions/v1/series", "/lol-missions/v1/data", "/lol-notifications/v1/notifications", "/swagger/v3/openapi.json", "/swagger/v2/swagger.json"} {
		if ctx.Err() != nil {
			break
		}
		limit := int64(2 << 20)
		if strings.HasPrefix(path, "/swagger/") {
			limit = 16 << 20
		}
		requestCtx, stop := context.WithTimeout(ctx, 1500*time.Millisecond)
		raw, err := client.getBytes(requestCtx, path, limit, "application/json")
		stop()
		status := 200
		result := "ok"
		if err != nil {
			result = "unavailable"
			status = 0
			var httpErr *LCUHTTPError
			if errors.As(err, &httpErr) {
				status = httpErr.StatusCode
			}
		}
		a.recordDiagnostic(map[string]any{"event": "objective_badge_probe", "trace_id": trace, "method": "GET", "path": path, "status": status, "result": result})
		if err != nil {
			continue
		}
		if strings.HasPrefix(path, "/swagger/") {
			contracts := objectiveContracts(raw)
			a.recordDiagnostic(map[string]any{"event": "objective_badge_contracts", "trace_id": trace, "source_path": path, "count": len(contracts), "operations": contracts, "write_executed": false})
			if len(contracts) > 0 {
				break
			}
		} else {
			a.recordObjectiveState(client, path, source, "snapshot", raw)
		}
	}
	return true
}

func (a *app) handleObjectiveDiagnostics(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	client := a.lcu
	connected := a.connected
	a.mu.RUnlock()
	if !connected || client == nil {
		http.Error(w, "请先连接客户端", http.StatusServiceUnavailable)
		return
	}
	if !a.collectObjectiveDiagnostics(r.Context(), client, "manual") {
		http.Error(w, "采集进行中或日志暂不可用，请稍后重试", http.StatusConflict)
		return
	}
	a.diagnosticLogMu.RLock()
	logError := a.diagnosticLogErr
	a.diagnosticLogMu.RUnlock()
	if logError != "" {
		http.Error(w, "角标诊断日志写入失败，请检查日志存储", http.StatusInternalServerError)
		return
	}
	respondJSON(w, map[string]any{"ok": true, "readOnly": true, "captureSeconds": 120})
}
