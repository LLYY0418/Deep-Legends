package main

import (
	"bufio"
	"encoding/json"
	"sort"
	"strings"
)

func settingRawValue(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	case bool:
		if x {
			return "true", true
		}
		return "false", true
	}
	return "", false
}
func allSettingsFromJSON(data []byte) map[string]string {
	values := map[string]string{}
	var root any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if decoder.Decode(&root) != nil {
		return values
	}
	var walk func(any, string, bool, int)
	walk = func(node any, prefix string, input bool, depth int) {
		if depth > 16 {
			return
		}
		switch v := node.(type) {
		case map[string]any:
			// PersistedSettings files/sections/settings records and LCU named records.
			if name, ok := v["name"].(string); ok {
				if strings.EqualFold(name, "Input.ini") {
					input = true
				}
				if value, ok := v["value"]; ok {
					key := strings.Trim(prefix+"."+name, ".")
					if cameraSafeKey(key) {
						if text, ok := settingRawValue(value); ok {
							values[key] = watchedSettingValue(text, input)
						}
					}
					return
				}
				if strings.EqualFold(name, "Game.cfg") || strings.EqualFold(name, "Input.ini") {
					prefix = name
				} else {
					prefix = strings.Trim(prefix+"."+name, ".")
				}
			}
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if key == "name" || key == "description" {
					continue
				}
				child := v[key]
				path := strings.Trim(prefix+"."+key, ".")
				if key == "files" || key == "sections" || key == "settings" {
					path = prefix
				}
				nextInput := input || strings.EqualFold(key, "input.ini") || strings.EqualFold(key, "input") || strings.EqualFold(key, "hotkeys") || strings.EqualFold(key, "keybindings")
				switch child.(type) {
				case map[string]any, []any:
					walk(child, path, nextInput, depth+1)
				default:
					if cameraSafeKey(path) {
						if text, ok := settingRawValue(child); ok {
							values[path] = watchedSettingValue(text, nextInput)
						}
					}
				}
			}
		case []any:
			for _, child := range v {
				walk(child, prefix, input, depth+1)
			}
		}
	}
	walk(root, "", false, 0)
	return values
}
func allSettingsFromINI(data []byte, input bool) map[string]string {
	values := map[string]string{}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 256<<10+1)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		key := strings.Trim(section+"."+name, ".")
		if !ok || !cameraSafeKey(key) {
			continue
		}
		prefix := "Game.cfg."
		if input {
			prefix = "Input.ini."
		}
		values[prefix+key] = watchedSettingValue(strings.TrimSpace(value), input)
	}
	return values
}

// Input bindings are only compared by digest in memory; diagnostics emit names.
func watchedSettingValue(value string, input bool) string {
	if input {
		return itemSetDigest([]byte(value))
	}
	return value
}
