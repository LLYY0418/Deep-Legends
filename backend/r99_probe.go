package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var r99ReadPaths = []string{
	"/lol-game-data/assets/v1/summoner-icons.json",
	"/lol-game-data/assets/v1/summoner-icon-sets.json",
	"/lol-game-data/assets/v1/regalia.json",
	"/lol-inventory/v2/inventory/SUMMONER_ICON",
	"/lol-regalia/v3/inventory/REGALIA_BANNER",
	"/lol-regalia/v3/inventory/REGALIA_CREST",
	"/lol-regalia/v3/inventory/REGALIA_BORDER",
	"/lol-inventory/v2/inventory/REGALIA_BANNER",
	"/lol-loadouts/v4/loadouts/scope/account",
	"/lol-regalia/v2/current-summoner/regalia",
	"/lol-banners/v1/current-summoner/flags",
	"/lol-banners/v1/current-summoner/flags/equipped",
	"/lol-banners/v1/current-summoner/frames/equipped",
}

// Field names are useful evidence, but even identifier field names are excluded
// by the R99 diagnostic contract. Aliases preserve shape without their values.
func r99SafeKey(key string) string {
	for _, pair := range [][2]string{{"puuid", "stable-ref"}, {"summonerid", "player-ref"}, {"accountid", "account-ref"}, {"uuid", "opaque-ref"}, {"contentid", "content-ref"}, {"itemid", "item-ref"}, {"purchasedate", "purchase-time"}, {"gamename", "display-name"}} {
		if strings.Contains(strings.ToLower(key), pair[0]) {
			return pair[1]
		}
	}
	if len(key) > 80 {
		return "other-field"
	}
	for _, r := range key {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return "other-field"
		}
	}
	return key
}
func r99Keys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for k := range value {
		keys = append(keys, r99SafeKey(k))
	}
	sort.Strings(keys)
	return keys
}
func r99Enums(rows []any, key string) []string {
	values := map[string]bool{}
	for _, row := range rows {
		if value := anyString(anyMap(row), key); value != "" {
			values[r99SafeKey(value)] = true
		}
	}
	out := make([]string, 0, len(values))
	for v := range values {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func r99JSONType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case []any:
		return "array"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "object"
	}
}
func r99KeyShape(key string) string {
	if _, err := strconv.ParseUint(key, 10, 64); err == nil {
		return "numeric"
	}
	if len(key) == 36 && strings.Count(key, "-") == 4 {
		return "guid"
	}
	return "other"
}
func r99ReadShape(index int, value any) map[string]any {
	e := map[string]any{"json_type": r99JSONType(value)}
	rows, _ := value.([]any)
	obj := anyMap(value)
	if rows != nil {
		e["count"] = len(rows)
		if len(rows) > 0 {
			e["fields"] = r99Keys(anyMap(rows[0]))
		}
	} else {
		e["fields"] = r99Keys(obj)
	}
	switch index {
	case 0:
		n := 0
		for _, r := range rows {
			if anyString(anyMap(r), "title") != "" {
				n++
			}
		}
		e["named_count"] = n
	case 2:
		e["types"] = r99Enums(rows, "regaliaType")
		selected, local := 0, 0
		types := map[string]bool{}
		for _, r := range rows {
			m := anyMap(r)
			if m["isSelectable"] == true {
				selected++
			}
			if m["isTencentOnly"] == true {
				local++
			}
			types[r99JSONType(m["id"])] = true
		}
		e["selectable_count"], e["local_count"], e["id_types"] = selected, local, types
	case 3:
		e["ownership_types"] = r99Enums(rows, "ownershipType")
		n := 0
		for _, r := range rows {
			if anyMap(r)["owned"] == true {
				n++
			}
		}
		e["owned_count"] = n
	case 4, 5, 6, 7:
		shapes := map[string]int{}
		fields := map[string]bool{}
		n := 0
		if rows != nil {
			for _, v := range rows {
				m := anyMap(v)
				for _, f := range r99Keys(m) {
					fields[f] = true
				}
				if m["isOwned"] == true {
					n++
				}
			}
		}
		for k, v := range obj {
			shapes[r99KeyShape(k)]++
			m := anyMap(v)
			for _, f := range r99Keys(m) {
				fields[f] = true
			}
			if m["isOwned"] == true {
				n++
			}
		}
		// Top-level keys here are inventory identifiers, never schema field names.
		delete(e, "fields")
		e["key_count"], e["key_shapes"], e["value_fields"], e["owned_count"] = len(obj), shapes, fields, n
	case 8:
		e["scopes"] = r99Enums(rows, "scope")
		slots := map[string][]string{}
		for _, row := range rows {
			for k, v := range anyMap(anyMap(row)["loadout"]) {
				slots[r99SafeKey(k)] = r99Keys(anyMap(v))
			}
		}
		e["slots"] = slots
	case 9:
		for _, key := range []string{"bannerType", "preferredBannerType", "crestType", "preferredCrestType"} {
			if s := anyString(obj, key); s != "" {
				e[key] = r99SafeKey(s)
			}
		}
	case 10:
		e["themes"] = r99Enums(rows, "theme")
	}
	return e
}
func (a *app) recordR99SurfaceShape(ctx context.Context, client *LCUClient) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for index, path := range r99ReadPaths {
		// R101 real-client evidence closed R7/R8 and the entire lol-banners fallback.
		// Keep historical indices stable for the archived R99 diagnostic matrix.
		if index == 6 || index == 7 || index >= 10 {
			continue
		}
		wg.Add(1)
		go func(index int, path string) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			status := &atomic.Int64{}
			requestCtx = context.WithValue(requestCtx, lcuResponseStatusKey{}, status)
			raw, err := client.GetBytesContext(requestCtx, path)
			cancel()
			var value any
			valid := err == nil && json.Unmarshal(raw, &value) == nil
			e := map[string]any{}
			if valid {
				e = r99ReadShape(index, value)
			}
			e["event"], e["probe"], e["status"], e["valid_json"] = "r99_read_probe", index+1, status.Load(), valid
			a.recordDiagnostic(e)
		}(index, path)
	}
	wg.Wait()
}
func (a *app) handleFacadeProbe(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, "未连接英雄联盟客户端", http.StatusConflict)
		return
	}
	if !a.facadeProbeMu.TryLock() {
		http.Error(w, "检测正在进行", http.StatusConflict)
		return
	}
	defer a.facadeProbeMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	// Changing a profile icon can reset the client's background and chat icon,
	// even when the icon itself is restored. A diagnostic must never do that.
	a.recordR99SurfaceShape(ctx, client)
	respondJSON(w, map[string]any{"mode": "read-only", "writeTested": false})
}
