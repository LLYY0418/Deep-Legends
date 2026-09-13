package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var focusField = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,79}$`)
var focusSensitive = regexp.MustCompile(`(?i)auth|token|password|secret|account|puuid|summoner|session|identifier|username|gameName|tagLine|device|chat`)
var focusCandidate = regexp.MustCompile(`(?i)focus|foreground|activat|window|popup|notification|ready.?check|gameflow`)

// Only structural keys and primitive boolean/numeric candidates. Strings may
// contain JWTs despite innocent-looking names; none are logged from preferences.
func focusPreferenceShape(raw []byte) map[string]any {
	fields := []map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	type level struct {
		indent int
		key    string
	}
	stack := []level{}
	skipped := -1
	lines := 0
	for scanner.Scan() {
		lines++
		line := scanner.Text()
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if skipped >= 0 {
			if indent > skipped {
				continue
			}
			skipped = -1
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || !focusField.MatchString(key) {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		if focusSensitive.MatchString(key) {
			skipped = indent
			continue
		}
		if len(stack) > 12 {
			continue
		}
		keys := []string{}
		for _, parent := range stack {
			keys = append(keys, parent.key)
		}
		keys = append(keys, key)
		field := strings.Join(keys, ".")
		value = strings.TrimSpace(value)
		row := map[string]any{"field": field, "type": "string-or-container"}
		if value == "" {
			row["type"] = "object"
			stack = append(stack, level{indent, key})
		}
		if value == "true" || value == "false" {
			row["type"] = "boolean"
			if focusCandidate.MatchString(field) {
				row["value"] = value == "true"
			}
		}
		if number, err := strconv.ParseInt(value, 10, 64); err == nil {
			row["type"] = "integer"
			if focusCandidate.MatchString(field) {
				row["value"] = number
			}
		}
		fields = append(fields, row)
		if len(fields) >= 256 {
			break
		}
	}
	return map[string]any{"fields": fields, "line_count": lines, "limit_reached": len(fields) >= 256, "parse_error": scanner.Err() != nil, "parser": "plain-yaml-key-shape-not-complete-schema"}
}

func focusHelpShape(raw []byte) map[string]any {
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return map[string]any{"parsed": false}
	}
	found := map[string]bool{}
	contracts := []map[string]any{}
	nodes := 0
	var walk func(any, int)
	walk = func(value any, depth int) {
		nodes++
		if nodes > 50000 || depth > 16 {
			return
		}
		check := func(s string) {
			// Exclude authentication paths and all unrestricted prose/example values.
			if len(s) > 180 || focusSensitive.MatchString(s) || !focusCandidate.MatchString(s) {
				return
			}
			if regexpFocusSymbol.MatchString(s) {
				found[s] = true
			}
		}
		switch v := value.(type) {
		case map[string]any:
			if len(contracts) < 160 {
				for _, field := range []string{"name", "path", "url"} {
					symbol, ok := v[field].(string)
					if !ok || !regexpFocusSymbol.MatchString(symbol) || focusSensitive.MatchString(symbol) || !focusCandidate.MatchString(symbol) {
						continue
					}
					contract := map[string]any{"symbol": symbol}
					for _, key := range []string{"method", "httpMethod", "type"} {
						value, _ := v[key].(string)
						switch value {
						case "GET", "POST", "PUT", "PATCH", "DELETE", "boolean", "integer", "number", "string", "object", "array":
							contract[key] = value
						}
					}
					for _, key := range []string{"required", "default"} {
						switch value := v[key].(type) {
						case bool:
							contract[key] = value
						case float64:
							contract[key] = value
						}
					}
					contract["default_exposed"] = contract["default"] != nil
					contracts = append(contracts, contract)
					break
				}
			}
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if focusSensitive.MatchString(key) {
					continue
				}
				check(key)
				if key == "name" || key == "path" || key == "url" {
					if text, ok := v[key].(string); ok {
						check(text)
					}
				}
				walk(v[key], depth+1)
				if nodes > 50000 {
					return
				}
			}
		case []any:
			for _, item := range v {
				walk(item, depth+1)
				if nodes > 50000 {
					return
				}
			}
		}
	}
	walk(root, 0)
	symbols := make([]string, 0, len(found))
	for symbol := range found {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	limited := len(symbols) > 160
	if limited {
		symbols = symbols[:160]
	}
	return map[string]any{"parsed": true, "candidate_symbols": symbols, "candidate_contracts": contracts, "nodes": nodes, "limit_reached": limited || nodes > 50000 || len(contracts) >= 160, "scope": "candidate-symbols-not-proven-config-switches"}
}

var regexpFocusSymbol = regexp.MustCompile(`^(?:/(?:lol-settings|lol-gameflow|lol-matchmaking|riotclient)/[A-Za-z0-9_/{}/-]+|[A-Za-z][A-Za-z0-9_-]{0,100})$`)

func focusJSONSettingsShape(raw []byte) map[string]any {
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return map[string]any{"parsed": false}
	}
	fields := []map[string]any{}
	var walk func(any, string, int)
	walk = func(value any, path string, depth int) {
		if depth > 12 || len(fields) >= 256 {
			return
		}
		row := map[string]any{"field": path}
		switch v := value.(type) {
		case map[string]any:
			keys := []string{}
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if focusField.MatchString(key) && !focusSensitive.MatchString(key) {
					walk(v[key], path+"."+key, depth+1)
				}
			}
			return
		case []any:
			row["type"] = "array"
			row["count"] = len(v)
		case bool:
			row["type"] = "boolean"
			if focusCandidate.MatchString(path) {
				row["value"] = v
			}
		case float64:
			row["type"] = "number"
			if focusCandidate.MatchString(path) {
				row["value"] = v
			}
		case nil:
			row["type"] = "null"
		default:
			row["type"] = "string-redacted"
		}
		fields = append(fields, row)
	}
	walk(root, "root", 0)
	return map[string]any{"parsed": true, "fields": fields, "limit_reached": len(fields) >= 256}
}

func (a *app) collectAcceptFocusInspection(ctx context.Context, client *LCUClient) {
	a.collectAcceptFocusInspectionAt(ctx, client, "export-not-at-accept")
}

func (a *app) collectAcceptFocusInspectionAt(ctx context.Context, client *LCUClient, timing string) {
	if client == nil {
		a.recordDiagnostic(map[string]any{"event": "accept_focus_inspection_end", "result": "client-unavailable"})
		return
	}
	if !client.acceptFocusInspection.mu.TryLock() {
		a.recordDiagnostic(map[string]any{"event": "accept_focus_inspection_end", "result": "inspection-already-active"})
		return
	}
	defer client.acceptFocusInspection.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	trace, _ := client.acceptFocusLastTrace.Load().(string)
	record := func(event map[string]any) {
		event["trace_id"] = trace
		event["snapshot_timing"] = timing
		event["observed_at_ms"] = time.Now().UnixMilli()
		a.recordDiagnostic(event)
	}
	record(client.acceptFocusHistorySnapshot())
	started := time.Now()
	defer func() {
		record(map[string]any{"event": "accept_focus_inspection_end", "duration_ms": time.Since(started).Milliseconds(), "budget_exhausted": ctx.Err() != nil, "configuration_mutated": false, "internal_call_stack_available": false, "unexposed_defaults_available": false})
	}()
	// /Help case and formats are documented by MingweiSamuel/lcu-schema's
	// update.ps1. These are read-only probes, not assumed supported by Tencent.
	for _, path := range []string{"/Help?format=Full", "/lol-settings/v2/local/lol-user-experience", "/lol-settings/v2/local/lol-notifications"} {
		requestStarted := time.Now()
		callCtx, stop := context.WithTimeout(ctx, 1200*time.Millisecond)
		raw, err := client.getBytes(callCtx, path, 8<<20, "application/json")
		stop()
		event := map[string]any{"event": "accept_focus_inspection", "source": timing, "method": "GET", "path": path, "ok": err == nil, "read_only": true, "duration_ms": time.Since(requestStarted).Milliseconds(), "response_bytes": len(raw)}
		if err != nil {
			event["error_class"] = focusErrorClass(err)
		}
		var e *LCUHTTPError
		if errors.As(err, &e) {
			event["status_code"] = e.StatusCode
		}
		if err == nil {
			if strings.HasPrefix(path, "/Help") {
				event["shape"] = focusHelpShape(raw)
			} else {
				event["shape"] = focusJSONSettingsShape(raw)
			}
		}
		record(event)
	}
	location, err := locateGameSettings(ctx, client)
	if err != nil {
		record(map[string]any{"event": "accept_focus_config", "result": "install-location-unavailable", "error_class": focusErrorClass(err)})
		return
	}
	for _, name := range []string{"LCULocalPreferences.yaml", "LCUAccountPreferences.yaml", "LeagueClientSettings.yaml"} {
		// Client Config, NOT Game/Config. File names are fixed and never logged as
		// absolute paths; refuse symlink redirection outside the client installation.
		target := filepath.Join(location.installRoot, "Config", name)
		resolved, err := filepath.EvalSymlinks(target)
		root, rootErr := filepath.EvalSymlinks(location.installRoot)
		if err != nil || rootErr != nil || !pathWithin(root, resolved) {
			reason := "outside-installation"
			if err != nil {
				reason = focusErrorClass(err)
			} else if rootErr != nil {
				reason = focusErrorClass(rootErr)
			}
			record(map[string]any{"event": "accept_focus_config", "file": name, "result": "unavailable", "error_class": reason})
			continue
		}
		file, err := os.Open(resolved)
		if err != nil {
			record(map[string]any{"event": "accept_focus_config", "file": name, "result": "open-failed", "error_class": focusErrorClass(err)})
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(file, (256<<10)+1))
		file.Close()
		event := map[string]any{"event": "accept_focus_config", "file": name, "read_only": true, "result": "ok"}
		if err != nil || len(raw) > 256<<10 {
			event["result"] = "unreadable-or-too-large"
			if err != nil {
				event["error_class"] = focusErrorClass(err)
			}
		} else {
			event["shape"] = focusPreferenceShape(raw)
		}
		record(event)
	}
}

func focusErrorClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, os.ErrNotExist):
		return "missing"
	case errors.Is(err, os.ErrPermission):
		return "permission-denied"
	default:
		return "request-or-io-error"
	}
}
