package main

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

func gameSettingsChangedKeys(before, after map[string]string, location settingsLocation, current Summoner) ([]string, int) {
	changes := gameSettingsCameraChanges(before, after)
	names := make([]string, 0, len(changes))
	for name := range changes {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := []string{}
	for _, name := range names {
		if !cameraSafeKey(name) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(name), "input.ini.") {
			rows = append(rows, name)
			continue
		}
		values := map[string]string{}
		for _, side := range []string{"before", "after"} {
			value, _ := cameraDiagnosticValue(changes[name][side])
			values[side] = value
		}
		sanitizeGameSettingsCameraValues(values, location, current)
		keys := map[string]string{name: values["after"]}
		sanitizeGameSettingsCameraValues(keys, location, current)
		if _, ok := keys[name]; !ok {
			continue
		}
		rows = append(rows, name+": "+values["before"]+" → "+values["after"])
	}
	truncated := max(0, len(rows)-60)
	if len(rows) > 60 {
		rows = rows[:60]
	}
	return rows, truncated
}
func gameConfigValues(all map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range all {
		if strings.HasPrefix(strings.ToLower(key), "game.cfg.") {
			out[strings.ToLower(key[len("Game.cfg."):])] = value
		}
	}
	return out
}
func decodeLCUSettings(data []byte) map[string]map[string]any {
	var raw map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if decoder.Decode(&raw) != nil {
		return nil
	}
	if wrapper, ok := raw["Game.cfg"].(map[string]any); ok {
		raw = wrapper
	}
	out := map[string]map[string]any{}
	for section, value := range raw {
		if !cameraSafeKey(section) || strings.Contains(strings.ToLower(section), "input") || strings.EqualFold(section, "hotkeys") || strings.EqualFold(section, "keybindings") {
			continue
		}
		fields, ok := value.(map[string]any)
		if !ok {
			continue
		}
		for name, v := range fields {
			if !cameraSafeKey(name) {
				continue
			}
			if _, ok := settingRawValue(v); !ok {
				continue
			}
			if out[section] == nil {
				out[section] = map[string]any{}
			}
			out[section][name] = v
		}
	}
	return out
}
func settingFromFile(text string, old any) (any, bool) {
	switch old.(type) {
	case bool:
		value, err := strconv.ParseBool(text)
		return value, err == nil
	case json.Number:
		var value json.Number
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		err := decoder.Decode(&value)
		return value, err == nil
	case string:
		return text, true
	}
	return nil, false
}
func settingMatches(text string, value any) bool {
	next, ok := settingFromFile(text, value)
	if !ok {
		return false
	}
	if _, ok := value.(json.Number); ok {
		x, e1 := strconv.ParseFloat(text, 64)
		raw, _ := settingRawValue(value)
		y, e2 := strconv.ParseFloat(raw, 64)
		return e1 == nil && e2 == nil && x == y
	}
	a, _ := settingRawValue(next)
	b, _ := settingRawValue(value)
	return a == b
}
func lcuMatchesConfig(lcu map[string]map[string]any, all map[string]string) bool {
	values := gameConfigValues(all)
	count := 0
	for section, fields := range lcu {
		for name, value := range fields {
			text, ok := values[strings.ToLower(section+"."+name)]
			if !ok || !settingMatches(text, value) {
				return false
			}
			count++
		}
	}
	return count > 0
}
func locatedGameSnapshot(snapshots map[string]gameSettingsFileSnapshot) gameSettingsFileSnapshot {
	for _, s := range snapshots {
		if s.Located {
			return s
		}
	}
	return gameSettingsFileSnapshot{}
}
func matchLCUSettingsFile(payload []byte, start, end map[string]gameSettingsFileSnapshot, current []gameSettingsFileSnapshot) string {
	lcu := decodeLCUSettings(payload)
	if lcuMatchesConfig(lcu, locatedGameSnapshot(end).AllValues) {
		return "B"
	}
	if lcuMatchesConfig(lcu, locatedGameSnapshot(start).AllValues) {
		return "A"
	}
	if start == nil && end == nil {
		for _, s := range current {
			if s.Located && lcuMatchesConfig(lcu, s.AllValues) {
				return "A"
			}
		}
	}
	return "neither"
}
