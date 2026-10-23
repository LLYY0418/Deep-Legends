package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type summonerKeySettings struct {
	Keys      [2]string `json:"keys"`
	FlashSlot *int      `json:"flashSlot"`
	loaded    bool
}

var summonerBindingPart = regexp.MustCompile(`\[([^\]]+)\]`)

func formatSummonerBinding(raw string) string {
	first := strings.Split(raw, ",")[0]
	parts := summonerBindingPart.FindAllStringSubmatch(first, -1)
	out := []string{}
	for _, p := range parts {
		v := strings.TrimSpace(p[1])
		switch strings.ToLower(v) {
		case "shift":
			v = "Shift"
		case "ctrl", "control":
			v = "Ctrl"
		case "alt":
			v = "Alt"
		default:
			v = strings.ToUpper(v)
		}
		if v != "" && v != "<UNBOUND>" {
			out = append(out, v)
		}
	}
	return strings.Join(out, "+")
}
func summonerKeysFromJSON(data []byte) [2]string {
	values := map[string]string{}
	var root any
	if json.Unmarshal(data, &root) != nil {
		return [2]string{}
	}
	var walk func(any, string)
	walk = func(node any, path string) {
		switch n := node.(type) {
		case map[string]any:
			name, _ := n["name"].(string)
			if name != "" {
				path += "." + name
			}
			if value, ok := n["value"].(string); ok {
				values[strings.ToLower(path)] = value
			}
			for key, value := range n {
				if key == "name" || key == "value" {
					continue
				}
				if str, ok := value.(string); ok {
					values[strings.ToLower(path+"."+key)] = str
				} else {
					walk(value, path+"."+key)
				}
			}
		case []any:
			for _, value := range n {
				walk(value, path)
			}
		}
	}
	walk(root, "")
	return summonerKeysFromValues(values)
}
func summonerKeysFromValues(values map[string]string) [2]string {
	out := [2]string{}
	for i, suffix := range []string{"1", "2"} {
		for _, event := range []string{"evtCastAvatarSpell", "evtSmartCastAvatarSpell", "evtSmartPlusSelfCastAvatarSpell", "evtNormalCastAvatarSpell"} {
			for key, value := range values {
				if strings.Contains(strings.ToLower(key), "gameevents") && strings.HasSuffix(strings.ToLower(key), strings.ToLower(event+suffix)) {
					if v := formatSummonerBinding(value); v != "" {
						if out[i] == "" {
							out[i] = v
						}
						if !strings.Contains(v, "+") {
							out[i] = v
							break
						}
					}
				}
			}
			if out[i] != "" && !strings.Contains(out[i], "+") {
				break
			}
		}
	}
	return out
}
func (a *app) loadSummonerKeysLocked() {
	k := &a.autoConfig.keys
	if k.loaded {
		return
	}
	if a.storage != nil {
		if data, err := os.ReadFile(filepath.Join(a.storage.root, "summoner-keys.json")); err == nil && len(data) < 4096 {
			_ = json.Unmarshal(data, k)
		}
	}
	k.loaded = true
	if k.FlashSlot != nil && (*k.FlashSlot < 0 || *k.FlashSlot > 1) {
		k.FlashSlot = nil
	}
}
func (a *app) saveSummonerKeysLocked() error {
	if a.storage == nil {
		return nil
	}
	data, _ := json.Marshal(a.autoConfig.keys)
	return atomicWriteFile(filepath.Join(a.storage.root, "summoner-keys.json"), data, 0600)
}
func (a *app) refreshSummonerKeys(ctx context.Context, client *LCUClient) {
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var raw json.RawMessage
	keys := [2]string{}
	if client.RequestJSON(ctx, http.MethodGet, "/lol-game-settings/v1/input-settings", nil, &raw) == nil {
		keys = summonerKeysFromJSON(raw)
	}
	if keys[0] == "" || keys[1] == "" {
		if location, err := locateGameSettings(ctx, client); err == nil {
			for _, file := range gameSettingsCandidates(location) {
				if !strings.EqualFold(filepath.Base(file), "input.ini") || !settingsWatchNoSymlinks(location, file) {
					continue
				}
				if data, err := os.ReadFile(file); err == nil && len(data) <= 256<<10 {
					fallback := summonerKeysFromValues(allSettingsFromINI(data, false))
					for i := range keys {
						if keys[i] == "" {
							keys[i] = fallback[i]
						}
					}
				}
			}
		}
	}
	a.autoConfig.Lock()
	defer a.autoConfig.Unlock()
	a.loadSummonerKeysLocked()
	for i, key := range keys {
		if key != "" {
			a.autoConfig.keys.Keys[i] = key
		}
	}
	_ = a.saveSummonerKeysLocked()
}
func (a *app) handleSummonerKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		client, _, _ := a.gameplayClient()
		a.refreshSummonerKeys(r.Context(), client)
	}
	a.autoConfig.Lock()
	defer a.autoConfig.Unlock()
	a.loadSummonerKeysLocked()
	if r.Method == http.MethodPut {
		var request struct {
			FlashSlot *int `json:"flashSlot"`
		}
		if decodeAutoConfig(w, r, &request) != nil || (request.FlashSlot != nil && (*request.FlashSlot < 0 || *request.FlashSlot > 1)) {
			http.Error(w, "闪现位置无效", 400)
			return
		}
		old := a.autoConfig.keys.FlashSlot
		a.autoConfig.keys.FlashSlot = request.FlashSlot
		if err := a.saveSummonerKeysLocked(); err != nil {
			a.autoConfig.keys.FlashSlot = old
			http.Error(w, "保存失败", 500)
			return
		}
	}
	respondJSON(w, a.autoConfig.keys)
}
func reorderFlashSpells(first, second int64, slot *int) (int64, int64) {
	if slot != nil && ((*slot == 0 && second == 4) || (*slot == 1 && first == 4)) {
		return second, first
	}
	return first, second
}
