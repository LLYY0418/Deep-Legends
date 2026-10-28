package main

import (
	"encoding/json"
	"sort"
	"strings"
)

// Never retain scalar strings (names, IDs, tokens) or the response itself.
func diagnosticRankedSeasonsShape(raw json.RawMessage, source string) map[string]any {
	var payload map[string]json.RawMessage
	_ = json.Unmarshal(raw, &payload)
	var seasons any
	_ = json.Unmarshal(payload["seasons"], &seasons)
	budget := 512
	var shape func(any, int) map[string]any
	shape = func(value any, depth int) map[string]any {
		budget--
		if budget < 0 || depth > 8 {
			return map[string]any{"type": "truncated"}
		}
		out := map[string]any{}
		switch v := value.(type) {
		case nil:
			out["type"] = "null"
		case []any:
			out["type"], out["length"] = "array", len(v)
			elements := make([]map[string]any, 0, min(len(v), 64))
			for _, child := range v[:min(len(v), 64)] {
				elements = append(elements, shape(child, depth+1))
			}
			out["elements"] = elements
		case map[string]any:
			out["type"] = "object"
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if len(keys) > 64 {
				keys = keys[:64]
			}
			out["keys"] = keys
			numbers := map[string]any{}
			children := map[string]any{}
			for _, key := range keys {
				child := v[key]
				if number, ok := child.(float64); ok {
					for _, part := range []string{"win", "loss", "games", "split", "season"} {
						if strings.Contains(strings.ToLower(key), part) {
							numbers[key] = number
							break
						}
					}
				}
				switch child.(type) {
				case []any, map[string]any:
					children[key] = shape(child, depth+1)
				}
			}
			out["numbers"], out["children"] = numbers, children
		case float64:
			out["type"] = "number"
		case bool:
			out["type"] = "boolean"
		default:
			out["type"] = "string"
		}
		return out
	}
	return map[string]any{"event": "ranked_seasons_shape", "source": source, "seasons": shape(seasons, 0)}
}
