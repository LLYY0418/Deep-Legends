package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
)

var champSelectSubsetPaths = []string{
	"/lol-lobby-team-builder/champ-select/v1/subset-champion-list",
	"/lol-champ-select/v1/subset-champion-list",
}

type lcuHelpOperation struct{ Method, Path, Name string }

// /help has varied between arrays and named function maps. Keep only explicit
// HTTP operations and static local paths; never execute arbitrary help text.
func lcuHelpOperations(raw []byte) []lcuHelpOperation {
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	var result []lcuHelpOperation
	var walk func(any, string)
	walk = func(value any, key string) {
		switch node := value.(type) {
		case []any:
			for _, child := range node {
				walk(child, "")
			}
		case map[string]any:
			name := firstString(node, "name")
			if name == "" {
				name = key
			}
			path := firstString(node, "path", "url", "uri", "endpoint")
			method := strings.ToUpper(firstString(node, "method", "httpMethod", "http_method", "verb"))
			for _, label := range []string{name, key} {
				parts := strings.Fields(label)
				if len(parts) == 2 && strings.HasPrefix(parts[1], "/") {
					method = strings.ToUpper(parts[0])
					path = parts[1]
				}
			}
			if path == "" && strings.HasPrefix(name, "/") {
				path = name
			}
			if strings.HasPrefix(path, "/lol-") && !strings.ContainsAny(path, "{}?#") && validateLCURequestPath(path) == nil && (method == "GET" || method == "POST") {
				op := lcuHelpOperation{method, path, name}
				if !slices.Contains(result, op) {
					result = append(result, op)
				}
			}
			for k, child := range node {
				walk(child, k)
			}
		}
	}
	walk(root, "")
	slices.SortFunc(result, func(a, b lcuHelpOperation) int { return strings.Compare(a.Path, b.Path) })
	return result
}

func champSelectSubsetHelpPaths(raw []byte) []string {
	paths := []string{}
	for _, op := range lcuHelpOperations(raw) {
		label := strings.ToLower(op.Path + " " + op.Name)
		if op.Method == http.MethodGet && strings.Contains(label, "subset") && (strings.Contains(label, "champ-select") || strings.Contains(label, "team-builder")) && !slices.Contains(paths, op.Path) {
			paths = append(paths, op.Path)
		}
	}
	return paths
}

func champSelectSubsetIDs(raw []byte) ([]int64, string) {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || rows == nil {
		return nil, "invalid-list"
	}
	ids := make([]int64, 0, len(rows))
	shape := "empty-list"
	for _, row := range rows {
		var id int64
		kind := "integer-array"
		if json.Unmarshal(row, &id) != nil {
			var object struct {
				ChampionID *int64 `json:"championId"`
			}
			if json.Unmarshal(row, &object) != nil || object.ChampionID == nil {
				return nil, "invalid-champion-id"
			}
			id = *object.ChampionID
			kind = "object-array"
		}
		if id <= 0 || shape != "empty-list" && shape != kind {
			return nil, "invalid-champion-id"
		}
		shape = kind
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, shape
}

func (r *watchRunner) champSelectSubsetRead(ctx context.Context, client *LCUClient, path string) ([]int64, error) {
	var status atomic.Int64
	raw, err := client.GetBytesContext(context.WithValue(ctx, lcuResponseStatusKey{}, &status), path)
	ids, parsed := champSelectSubsetIDs(raw)
	if err != nil {
		parsed = "read-" + diagnosticErrorKind(err)
	}
	valid := err == nil && status.Load() == http.StatusOK && ids != nil
	r.record(map[string]any{"event": "subset_probe", "path": path, "status_code": status.Load(), "parse_result": parsed, "id_count": len(ids), "valid": valid})
	if !valid {
		return nil, errors.New("subset response unavailable")
	}
	return ids, nil
}

func (r *watchRunner) champSelectPickAvailability(ctx context.Context, client *LCUClient, session lcuChampSelectSession, api string) ([]int64, string, error) {
	if !session.AllowSubsetChampionPicks {
		source := api + "/pickable-champion-ids"
		var ids []int64
		err := client.RequestJSON(ctx, http.MethodGet, source, nil, &ids)
		return ids, source, err
	}
	r.mu.Lock()
	runtime := r.champSelect.runtimeID
	if r.champSelect.subsetClient != client {
		r.champSelect.subsetClient = client
		r.champSelect.subsetEndpoint = ""
		r.champSelect.subsetDiscovered = false
	}
	path, discovered := r.champSelect.subsetEndpoint, r.champSelect.subsetDiscovered
	r.mu.Unlock()
	if discovered {
		if path == "" {
			return nil, "unavailable", errors.New("subset unavailable")
		}
		ids, err := r.champSelectSubsetRead(ctx, client, path)
		if err != nil {
			return nil, "unavailable", err
		}
		return ids, path, nil
	}
	raw, helpErr := client.GetBytesContext(ctx, "/help")
	paths := champSelectSubsetHelpPaths(raw)
	r.record(map[string]any{"event": "subset_help_matches", "paths": append([]string{}, paths...), "result": diagnosticStageError(helpErr, "read")})
	for _, candidate := range champSelectSubsetPaths {
		if !slices.Contains(paths, candidate) {
			paths = append(paths, candidate)
		}
	}
	var ids []int64
	for _, candidate := range paths {
		var err error
		ids, err = r.champSelectSubsetRead(ctx, client, candidate)
		if err == nil {
			path = candidate
			break
		}
	}
	r.mu.Lock()
	if r.champSelect.runtimeID != runtime {
		r.mu.Unlock()
		return nil, "unavailable", errors.New("subset session changed")
	}
	r.champSelect.subsetEndpoint, r.champSelect.subsetDiscovered = path, true
	r.mu.Unlock()
	if path == "" {
		return nil, "unavailable", errors.New("subset unavailable")
	}
	return ids, path, nil
}

func (r *watchRunner) champSelectSubsetTrace(session lcuChampSelectSession, ids []int64, source, reason string) {
	if !session.AllowSubsetChampionPicks {
		return
	}
	r.champDiagnostic("subset", reason, champSelectDecision{}, map[string]any{"subset_mode": true, "subset_ids": append([]int64{}, ids...), "subset_source": source})
}
