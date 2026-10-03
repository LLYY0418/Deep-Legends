package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *app) keepGameSettingsEnabled() bool {
	if a.storage == nil {
		return true
	}
	data, err := readLocalStoreFile(a.storage, "game-settings-sync.json")
	if err != nil {
		return true
	}
	var settings struct {
		Enabled *bool `json:"enabled"`
	}
	if json.Unmarshal(data, &settings) != nil || settings.Enabled == nil {
		return false
	}
	return *settings.Enabled
}
func (a *app) handleGameSettingsSyncPreference(w http.ResponseWriter, r *http.Request) {
	var value struct {
		Enabled *bool `json:"enabled"`
	}
	if decodeJSONRequest(r, &value, 4096) != nil || value.Enabled == nil {
		http.Error(w, "设置格式不正确", 400)
		return
	}
	if a.saveGameSettingsPreference(value.Enabled, nil) != nil {
		http.Error(w, "设置保存失败", 503)
		return
	}
	respondJSON(w, map[string]any{"enabled": *value.Enabled})
}
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
func (a *app) scheduleGameSettingsSyncLocked(ctx context.Context, client *LCUClient, generation uint64) {
	s := &a.gameSettingsWatch
	wait := s.wait
	if wait == nil {
		wait = gameSettingsWait
	}
	a.goSafe("game-settings-sync-delay", func() {
		if !wait(ctx, 5*time.Second) {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if ctx.Err() == nil && s.client == client && s.generation == generation && s.ended {
			s.enqueueLocked(gameSettingsWatchJob{stage: "sync_after_5s", generation: generation, delayed: true})
		}
	})
}
func (a *app) runGameSettingsSyncJob(ctx context.Context, client *LCUClient, job gameSettingsWatchJob) {
	s := &a.gameSettingsWatch
	valid := func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.client == client && s.generation == job.generation && s.ended && ctx.Err() == nil
	}
	if !valid() {
		return
	}
	s.mu.Lock()
	before := locatedGameSnapshot(s.start).AllValues
	location := s.location
	s.mu.Unlock()
	a.syncGameSettings(ctx, client, location, before, valid)
}
func (a *app) syncGameSettings(parent context.Context, client *LCUClient, location settingsLocation, before map[string]string, valid func() bool) {
	a.gameSettingsWriteMu.Lock()
	defer a.gameSettingsWriteMu.Unlock()
	event := map[string]any{"camera_mode_before": "unknown", "camera_mode_after": "unknown", "event": "game_settings_sync", "changed_count": 0, "patched_keys": []string{}, "result": "lcu_unavailable"}
	defer func() { a.recordDiagnostic(event) }()
	if !a.keepGameSettingsEnabled() {
		event["result"] = "skipped_disabled"
		return
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if client == nil {
		return
	}
	if location.file == "" {
		var err error
		location, err = locateGameSettings(ctx, client)
		if err != nil {
			return
		}
	}
	snapshots := readGameSettingsWatchFiles(location, "sync_after_5s")
	var target gameSettingsFileSnapshot
	for _, s := range snapshots {
		if s.Located {
			target = s
		}
		if s.Exists && s.ReadOnly && (s.Located || s.PathKind == settingsWatchPathKind(location, filepath.Join(location.configRoot, "game.cfg"))) {
			event["result"] = "skipped_locked"
			return
		}
	}
	if target.Result != "ok" {
		event["stage"] = "file_read"
		return
	}
	old, next := gameConfigValues(before), gameConfigValues(target.AllValues)
	event["camera_mode_before"] = cameraConfigValue(before)
	event["camera_mode_after"] = cameraConfigValue(target.AllValues)
	changed := map[string]string{}
	for key, value := range next {
		if previous, ok := old[key]; ok && previous != value {
			changed[key] = value
		}
	}
	event["changed_count"] = len(changed)
	if len(changed) == 0 {
		event["result"] = "skipped_no_change"
		return
	}
	var payload json.RawMessage
	if client.RequestJSON(ctx, http.MethodGet, "/lol-game-settings/v1/game-settings", nil, &payload) != nil {
		return
	}
	lcu := decodeLCUSettings(payload)
	if len(lcu) == 0 {
		return
	}
	patch := map[string]map[string]any{}
	keys := []string{}
	for section, fields := range lcu {
		for name, value := range fields {
			key := strings.ToLower(section + "." + name)
			text, hasChange := changed[key]
			if !hasChange || !settingMatches(old[key], value) || settingMatches(text, value) {
				continue
			}
			replacement, ok := settingFromFile(text, value)
			if !ok {
				continue
			}
			if patch[section] == nil {
				patch[section] = map[string]any{}
			}
			patch[section][name] = replacement
			keys = append(keys, "Game.cfg."+section+"."+name)
		}
	}
	if len(keys) == 0 {
		event["result"] = "skipped_no_change"
		return
	}
	sort.Strings(keys)
	event["patched_keys"] = keys
	if !a.keepGameSettingsEnabled() {
		event["result"] = "skipped_disabled"
		return
	}
	if valid != nil && !valid() {
		event["stage"] = "stale_game"
		return
	}
	// Recheck the located file immediately before the only write; the user may
	// have locked it or the game may have replaced it while the LCU GET ran.
	_, info, err := safeSettingsFile(location)
	if err != nil || gameSettingsReadOnly(location.file, info) {
		event["result"] = "skipped_locked"
		return
	}
	if info.ModTime().UTC().Format(time.RFC3339Nano) != target.Mtime || info.Size() != target.Size {
		event["result"] = "verify_failed"
		event["stage"] = "file_changed"
		return
	}
	stable, err := os.ReadFile(location.file)
	if err != nil || itemSetDigest(stable)[:8] != target.Hash {
		event["result"] = "verify_failed"
		event["stage"] = "file_changed"
		return
	}
	if err := client.RequestJSON(ctx, http.MethodPatch, "/lol-game-settings/v1/game-settings", patch, nil); err != nil {
		event["stage"] = "patch"
		return
	}
	var verified json.RawMessage
	if client.RequestJSON(ctx, http.MethodGet, "/lol-game-settings/v1/game-settings", nil, &verified) != nil {
		event["result"] = "verify_failed"
		return
	}
	actual := decodeLCUSettings(verified)
	for section, fields := range patch {
		for name, value := range fields {
			expected, _ := settingRawValue(value)
			if !settingMatches(expected, actual[section][name]) {
				event["result"] = "verify_failed"
				return
			}
		}
	}
	called, saveResult, saveErr := saveLCUGameSettings(ctx, client, valid)
	_ = called
	event["save_result"] = saveResult
	if saveErr != nil {
		event["result"] = "verify_failed"
		return
	}
	event["result"] = "ok"
}
