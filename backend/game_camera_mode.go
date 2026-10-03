package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Local evidence confirms locked=2 only. Free/dynamic follow the client's
// option order and remain subject to the R198 two-game Windows acceptance.
var gameCameraModeValues = map[string]int{"free": 0, "dynamic": 1, "locked": 2}

type gameSettingsPreference struct {
	Enabled    *bool  `json:"enabled"`
	CameraMode string `json:"cameraMode,omitempty"`
}

func (a *app) cameraModePreference() string {
	if a.storage == nil {
		return "none"
	}
	data, err := readLocalStoreFile(a.storage, "game-settings-sync.json")
	var value gameSettingsPreference
	if err != nil || json.Unmarshal(data, &value) != nil {
		return "none"
	}
	if _, ok := gameCameraModeValues[value.CameraMode]; !ok {
		return "none"
	}
	return value.CameraMode
}
func (a *app) saveGameSettingsPreference(enabled *bool, camera *string) error {
	a.gameSettingsPreferenceMu.Lock()
	defer a.gameSettingsPreferenceMu.Unlock()
	currentEnabled := a.keepGameSettingsEnabled()
	value := gameSettingsPreference{Enabled: &currentEnabled, CameraMode: a.cameraModePreference()}
	if enabled != nil {
		value.Enabled = enabled
	}
	if camera != nil {
		value.CameraMode = *camera
	}
	data, _ := json.Marshal(value)
	return writeLocalStoreFile(a.storage, "game-settings-sync.json", data)
}
func (a *app) handleCameraModePreference(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Mode string `json:"mode"`
	}
	if decodeJSONRequest(r, &request, 4096) != nil {
		http.Error(w, "设置格式不正确", 400)
		return
	}
	if _, ok := gameCameraModeValues[request.Mode]; !ok && request.Mode != "none" {
		http.Error(w, "镜头模式无效", 400)
		return
	}
	if a.saveGameSettingsPreference(nil, &request.Mode) != nil {
		http.Error(w, "设置保存失败", 503)
		return
	}
	respondJSON(w, map[string]string{"cameraMode": request.Mode})
	a.mu.RLock()
	client := a.lcu
	a.mu.RUnlock()
	if client != nil && request.Mode != "none" {
		a.goSafe("camera-mode-settings", func() { a.applyGameCameraMode(context.Background(), client, "settings_changed", nil) })
	}
}
func cameraStageAllowed(stage, phase string) bool {
	switch stage {
	case "champselect":
		return phase == "ChampSelect"
	case "game_start":
		return phase == "GameStart"
	case "settings_changed":
		return phase == "None" || phase == "Lobby"
	}
	return false
}
func cameraSetting(lcu map[string]map[string]any) (string, string, any, bool) {
	for section, fields := range lcu {
		if strings.EqualFold(section, "General") {
			for name, value := range fields {
				if name == "CameraMode" {
					_, ok := settingRawValue(value)
					return section, name, value, ok
				}
			}
		}
	}
	return "", "", nil, false
}
func cameraValue(text string) string {
	if n, e := strconv.Atoi(text); e == nil && n >= 0 && n <= 2 {
		return strconv.Itoa(n)
	}
	return "unknown"
}
func cameraConfigValue(all map[string]string) string {
	text := gameConfigValues(all)["general.cameramode"]
	if text == "" {
		for key, value := range all {
			if strings.EqualFold(key, "General.CameraMode") {
				text = value
				break
			}
		}
	}
	if index := strings.IndexAny(text, ";#"); index >= 0 {
		text = text[:index]
	}
	return cameraValue(strings.TrimSpace(text))
}

// Both writes use this serialization, so a settlement sync and preference
// action cannot overwrite each other while they are being verified.
func (a *app) applyGameCameraMode(parent context.Context, client *LCUClient, stage string, valid func() bool) {
	target := a.cameraModePreference()
	value, enabled := gameCameraModeValues[target]
	if !enabled || client == nil {
		return
	}
	a.gameSettingsWriteMu.Lock()
	defer a.gameSettingsWriteMu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	allowed := func() bool {
		if ctx.Err() != nil || a.cameraModePreference() != target || valid != nil && !valid() {
			return false
		}
		a.mu.RLock()
		connected := a.lcu == client
		a.mu.RUnlock()
		if !connected {
			return false
		}
		var phase string
		return client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase) == nil && cameraStageAllowed(stage, phase)
	}
	if !allowed() {
		return
	}
	event := map[string]any{"event": "game_camera_mode_apply", "stage": stage, "target": target, "target_value": value, "lcu_before": "unknown", "lcu_after": "unknown", "lcu_result": "unavailable", "file_before": map[string]string{}, "file_after": map[string]string{}, "file_result": "unchanged", "save_called": false}
	defer func() { a.recordDiagnostic(event) }()
	var payload json.RawMessage
	if client.RequestJSON(ctx, http.MethodGet, "/lol-game-settings/v1/game-settings", nil, &payload) == nil {
		section, name, old, ok := cameraSetting(decodeLCUSettings(payload))
		if ok {
			text, _ := settingRawValue(old)
			event["lcu_before"] = cameraValue(text)
			event["lcu_after"] = cameraValue(text)
			if settingMatches(strconv.Itoa(value), old) {
				event["lcu_result"] = "unchanged"
			} else if replacement, ok := settingFromFile(strconv.Itoa(value), old); ok && allowed() {
				patch := map[string]map[string]any{section: {name: replacement}}
				if client.RequestJSON(ctx, http.MethodPatch, "/lol-game-settings/v1/game-settings", patch, nil) == nil {
					event["lcu_result"] = "verify_failed"
					var verified json.RawMessage
					if client.RequestJSON(ctx, http.MethodGet, "/lol-game-settings/v1/game-settings", nil, &verified) == nil {
						_, _, actual, exists := cameraSetting(decodeLCUSettings(verified))
						text, _ := settingRawValue(actual)
						event["lcu_after"] = cameraValue(text)
						if exists && settingMatches(strconv.Itoa(value), actual) {
							called, _, err := saveLCUGameSettings(ctx, client, allowed)
							event["save_called"] = called
							if err == nil {
								event["lcu_result"] = "ok"
							}
						}
					}
				}
			}
		}
	}
	if stage == "settings_changed" {
		event["file_skipped"] = "settings_changed"
		return
	}
	if !allowed() {
		event["file_skipped"] = "stopped"
		return
	}
	running := a.cameraProcessRunning
	if running == nil {
		running = gameCameraProcessRunning
	}
	safeToWrite := func() bool {
		if !allowed() {
			return false
		}
		active, err := running()
		return err == nil && !active
	}
	if !safeToWrite() {
		event["file_skipped"] = "game_running"
		return
	}
	location, err := locateGameSettings(ctx, client)
	if err != nil {
		event["file_result"] = "not_found"
		return
	}
	results := map[string]bool{}
	before := event["file_before"].(map[string]string)
	after := event["file_after"].(map[string]string)
	for _, name := range []string{"PersistedSettings.json", "game.cfg"} {
		file := location
		file.file = filepath.Join(location.configRoot, name)
		b, n, result := applyCameraModeFile(file, value, safeToWrite)
		before[name] = b
		after[name] = n
		results[result] = true
	}
	for _, result := range []string{"write_failed", "read_only", "not_found", "ok", "unchanged"} {
		if results[result] {
			event["file_result"] = result
			break
		}
	}
	if results["read_only"] {
		event["file_skipped"] = "read_only"
	}
}
func applyCameraModeFile(location settingsLocation, target int, valid func() bool) (string, string, string) {
	if !settingsWatchNoSymlinks(location, location.file) {
		return "unknown", "unknown", "write_failed"
	}
	file, info, err := safeSettingsFile(location)
	if os.IsNotExist(err) {
		return "unknown", "unknown", "not_found"
	}
	if err != nil || info.Size() > 256<<10 {
		return "unknown", "unknown", "write_failed"
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "unknown", "unknown", "write_failed"
	}
	next, before, err := replaceCameraModeBytes(data, strings.EqualFold(filepath.Ext(file), ".json"), target)
	if err != nil {
		return "unknown", "unknown", "write_failed"
	}
	if gameSettingsReadOnly(file, info) {
		return before, before, "read_only"
	}
	if string(data) == string(next) {
		return before, before, "unchanged"
	}
	temp, err := os.CreateTemp(filepath.Dir(file), ".deep-legends-camera-*")
	if err != nil {
		return before, before, "write_failed"
	}
	name := temp.Name()
	defer os.Remove(name)
	err = temp.Chmod(info.Mode().Perm())
	if err == nil {
		_, err = temp.Write(next)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return before, before, "write_failed"
	}
	// Recheck link components, identity, permissions and bytes immediately before
	// atomic replacement; never follow links or replace a concurrently changed file.
	if !valid() {
		return before, before, "write_failed"
	}
	_, current, err := safeSettingsFile(location)
	stable, readErr := os.ReadFile(file)
	if err != nil || readErr != nil || !settingsWatchNoSymlinks(location, location.file) || !os.SameFile(info, current) || gameSettingsReadOnly(file, current) || string(stable) != string(data) {
		return before, before, "write_failed"
	}
	if os.Rename(name, file) != nil {
		return before, before, "write_failed"
	}
	// Confirm the installed bytes, including all settings outside CameraMode.
	verified, readErr := os.ReadFile(file)
	if readErr != nil || !settingsWatchNoSymlinks(location, location.file) || string(verified) != string(next) {
		return before, "unknown", "write_failed"
	}
	return before, strconv.Itoa(target), "ok"
}
func saveLCUGameSettings(ctx context.Context, client *LCUClient, valid func() bool) (bool, string, error) {
	var schema struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if client.RequestJSON(ctx, http.MethodGet, "/swagger/v3/openapi.json", nil, &schema) != nil {
		return false, "not_advertised", nil
	}
	operation := schema.Paths["/lol-game-settings/v1/save"]["post"]
	if len(operation) == 0 {
		return false, "not_advertised", nil
	}
	var post struct {
		RequestBody struct {
			Required bool `json:"required"`
		} `json:"requestBody"`
		Parameters []struct {
			Required bool `json:"required"`
		} `json:"parameters"`
	}
	if json.Unmarshal(operation, &post) != nil {
		return false, "not_advertised", nil
	}
	if post.RequestBody.Required {
		return false, "unsupported_schema", nil
	}
	for _, p := range post.Parameters {
		if p.Required {
			return false, "unsupported_schema", nil
		}
	}
	if valid != nil && !valid() {
		return false, "failed", errors.New("settings stage changed")
	}
	if err := client.RequestJSON(ctx, http.MethodPost, "/lol-game-settings/v1/save", nil, nil); err != nil {
		return true, "failed", err
	}
	return true, "ok", nil
}
