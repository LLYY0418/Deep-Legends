package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const autoConfigMaxBytes = 2 << 20

type autoConfigRunes struct {
	PrimaryStyleID  int64   `json:"primaryStyleId"`
	SubStyleID      int64   `json:"subStyleId"`
	SelectedPerkIDs []int64 `json:"selectedPerkIds"`
}
type autoConfigSpells struct {
	Spell1ID int64 `json:"spell1Id"`
	Spell2ID int64 `json:"spell2Id"`
}
type autoConfigStarterItem struct {
	ID    int64 `json:"id"`
	Count int64 `json:"count"`
}
type autoConfigItems struct {
	Starter     []autoConfigStarterItem `json:"starter,omitempty"`
	Core        []int64                 `json:"core"`
	Situational []int64                 `json:"situational"`
}
type autoConfigEntry struct {
	Runes     *autoConfigRunes  `json:"runes,omitempty"`
	Spells    *autoConfigSpells `json:"spells,omitempty"`
	Items     *autoConfigItems  `json:"items,omitempty"`
	UpdatedAt string            `json:"updatedAt,omitempty"`
}
type autoConfigSettings struct {
	Version   int64                                 `json:"version"`
	Enabled   bool                                  `json:"enabled"`
	Runes     bool                                  `json:"runes"`
	Spells    bool                                  `json:"spells"`
	Items     bool                                  `json:"items"`
	Champions map[string]map[string]autoConfigEntry `json:"champions"`
}
type autoConfigStore struct {
	sync.Mutex
	loaded        bool
	settings      autoConfigSettings
	runtime       autoConfigRuntime
	keys          summonerKeySettings
	phase         string
	processing    bool
	queued        bool
	catalog       *autoConfigCatalog
	catalogClient *LCUClient
	catalogKey    string
	catalogAt     time.Time
}

func defaultAutoConfig() autoConfigSettings {
	return autoConfigSettings{Version: 1, Runes: true, Spells: true, Items: true, Champions: map[string]map[string]autoConfigEntry{}}
}
func autoConfigMode(queue int64, mode string, mapID int64) string {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if queue == 1700 || queue == 1710 || queue == 1750 || mode == "CHERRY" {
		return ""
	}
	if queue == 2300 || queue == 2400 || queue == 3270 || mode == "KIWI" || mode == "ARAM_MAYHEM" {
		return "hextech"
	}
	switch queue {
	case 420, 440:
		return "ranked"
	case 450, 930, 3220, 720:
		return "aram"
	case 900, 1900:
		return "urf"
	case 400, 430, 480, 490, 700, 820, 830, 840, 850, 860, 870, 880, 890, 950, 960:
		return "normal"
	case 1300, 1400:
		return "event"
	}
	if queue == 0 && mapID == 11 && mode == "" {
		return "normal"
	}
	switch mode {
	case "ARAM":
		return "aram"
	case "URF":
		return "urf"
	case "ULTBOOK", "NEXUSBLITZ":
		return "event"
	case "CLASSIC":
		if mapID == 0 || mapID == 11 {
			return "normal"
		}
	}
	if champSelectGroupID(queue, mode, mapID) == "event" {
		return "event"
	}
	return ""
}
func validAutoConfigMode(mode string) bool {
	switch mode {
	case "ranked", "ranked-top", "ranked-jungle", "ranked-middle", "ranked-bottom", "ranked-utility", "normal", "aram", "hextech", "urf", "event":
		return true
	}
	return false
}
func validateAutoConfigEntry(mode string, e autoConfigEntry) error {
	if !validAutoConfigMode(mode) {
		return errors.New("模式无效")
	}
	if mode == "hextech" && e.Runes != nil {
		return errors.New("海克斯大乱斗没有符文")
	}
	if e.Runes != nil {
		r := e.Runes
		if r.PrimaryStyleID <= 0 || r.SubStyleID <= 0 || r.PrimaryStyleID == r.SubStyleID || len(r.SelectedPerkIDs) > 9 {
			return errors.New("符文配置无效")
		}
		for _, id := range r.SelectedPerkIDs {
			if id < 0 || id > 100000 {
				return errors.New("符文编号无效")
			}
		}
	}
	if e.Spells != nil {
		s := e.Spells
		if s.Spell1ID <= 0 || s.Spell2ID <= 0 || s.Spell1ID == s.Spell2ID || s.Spell1ID > 100000 || s.Spell2ID > 100000 {
			return errors.New("召唤师技能无效或重复")
		}
	}
	if e.Items != nil {
		if len(e.Items.Core) > 6 || len(e.Items.Situational) > 6 || len(e.Items.Starter) > 6 {
			return errors.New("每组最多6件装备")
		}
		starterSeen := map[int64]bool{}
		for _, item := range e.Items.Starter {
			if item.ID <= 0 || item.ID > 1000000 || item.Count < 1 || item.Count > 99 || starterSeen[item.ID] {
				return errors.New("出门装备编号或数量无效")
			}
			starterSeen[item.ID] = true
		}
		seen := map[int64]bool{}
		for _, ids := range [][]int64{e.Items.Core, e.Items.Situational} {
			for _, id := range ids {
				if id <= 0 || id > 1000000 || seen[id] {
					return errors.New("装备编号无效或重复")
				}
				seen[id] = true
			}
		}
	}
	return nil
}
func validateAutoConfig(s autoConfigSettings) error {
	if s.Version < 1 || len(s.Champions) > 1000 {
		return errors.New("配置版本或英雄数量无效")
	}
	for id, modes := range s.Champions {
		n, err := strconv.Atoi(id)
		if err != nil || n <= 0 || n > 10000 || len(modes) > 11 {
			return errors.New("英雄配置无效")
		}
		for mode, e := range modes {
			if err := validateAutoConfigEntry(mode, e); err != nil {
				return err
			}
		}
	}
	return nil
}
func (a *app) loadAutoConfigLocked() {
	s := &a.autoConfig
	if s.loaded {
		return
	}
	s.loaded = true
	s.settings = defaultAutoConfig()
	if a.storage == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(a.storage.root, "auto-config.json"))
	if os.IsNotExist(err) {
		return
	}
	var settings autoConfigSettings
	if err != nil || len(data) > autoConfigMaxBytes || json.Unmarshal(data, &settings) != nil || validateAutoConfig(settings) != nil {
		a.recordDiagnostic(map[string]any{"event": "auto_config_storage", "result": "invalid-defaults"})
		return
	}
	if settings.Champions == nil {
		settings.Champions = map[string]map[string]autoConfigEntry{}
	}
	s.settings = settings
}
func (a *app) autoConfigSnapshot() autoConfigSettings {
	a.autoConfig.Lock()
	defer a.autoConfig.Unlock()
	a.loadAutoConfigLocked()
	data, _ := json.Marshal(a.autoConfig.settings)
	var out autoConfigSettings
	_ = json.Unmarshal(data, &out)
	return out
}
func (a *app) saveAutoConfigLocked(next autoConfigSettings) error {
	if err := validateAutoConfig(next); err != nil {
		return err
	}
	next.Version = a.autoConfig.settings.Version + 1
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if len(data) > autoConfigMaxBytes {
		return errors.New("配置超过2MB")
	}
	if a.storage != nil {
		if err = atomicWriteFile(filepath.Join(a.storage.root, "auto-config.json"), data, 0600); err != nil {
			return err
		}
	}
	a.autoConfig.settings = next
	return nil
}
func decodeAutoConfig(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, autoConfigMaxBytes)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func (a *app) handleAutoConfigSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		respondJSON(w, a.autoConfigSnapshot())
		return
	}
	var next autoConfigSettings
	if err := decodeAutoConfig(w, r, &next); err != nil {
		http.Error(w, "配置格式无效", 400)
		return
	}
	a.autoConfig.Lock()
	defer a.autoConfig.Unlock()
	a.loadAutoConfigLocked()
	if next.Version != a.autoConfig.settings.Version {
		http.Error(w, "配置已在其它页面更新，请重新读取", 409)
		return
	}
	if err := a.saveAutoConfigLocked(next); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	respondJSON(w, a.autoConfig.settings)
}
func (a *app) handleAutoConfigChampion(w http.ResponseWriter, r *http.Request) {
	id, mode := r.PathValue("id"), r.PathValue("modeKey")
	n, err := strconv.Atoi(id)
	if err != nil || n <= 0 || n > 10000 || !validAutoConfigMode(mode) {
		http.Error(w, "英雄或模式无效", 400)
		return
	}
	var entry autoConfigEntry
	var version int64
	if r.Method != http.MethodDelete {
		if decodeAutoConfig(w, r, &entry) != nil {
			http.Error(w, "配置格式无效", 400)
			return
		}
		if err = validateAutoConfigEntry(mode, entry); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		previous := a.autoConfigSnapshot()
		version = previous.Version
		stored := previous.Champions[id][mode]
		// Preserve obsolete values already stored for an untouched category.
		// A changed spell pair or equipment list must pass the current catalog.
		changed := entry
		if reflect.DeepEqual(entry.Spells, stored.Spells) {
			changed.Spells = nil
		}
		if reflect.DeepEqual(entry.Items, stored.Items) {
			changed.Items = nil
		}
		if err = a.validateAutoConfigCatalog(r.Context(), mode, changed, false); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		entry.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	a.autoConfig.Lock()
	defer a.autoConfig.Unlock()
	a.loadAutoConfigLocked()
	if r.Method != http.MethodDelete && a.autoConfig.settings.Version != version {
		http.Error(w, "配置已更新，请重新读取后保存", http.StatusConflict)
		return
	}
	next := a.autoConfig.settings
	// Clone before mutation: a failed disk write must leave the live configuration intact.
	data, _ := json.Marshal(next)
	next = autoConfigSettings{}
	_ = json.Unmarshal(data, &next)
	if next.Champions[id] == nil {
		next.Champions[id] = map[string]autoConfigEntry{}
	}
	if r.Method == http.MethodDelete {
		delete(next.Champions[id], mode)
		if len(next.Champions[id]) == 0 {
			delete(next.Champions, id)
		}
	} else {
		next.Champions[id][mode] = entry
	}
	if err = a.saveAutoConfigLocked(next); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	respondJSON(w, a.autoConfig.settings)
}
func (a *app) handleAutoConfigCopy(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ChampionID int64    `json:"championId"`
		From       string   `json:"from"`
		Targets    []string `json:"targets"`
	}
	if decodeAutoConfig(w, r, &request) != nil || len(request.Targets) > 11 {
		http.Error(w, "复制参数无效", 400)
		return
	}
	next := a.autoConfigSnapshot()
	version := next.Version
	id := strconv.FormatInt(request.ChampionID, 10)
	e, ok := next.Champions[id][request.From]
	if !ok {
		http.Error(w, "请先保存源配置", 400)
		return
	}
	for _, mode := range request.Targets {
		copy := e
		if mode == "hextech" {
			copy.Runes = nil
		}
		if err := validateAutoConfigEntry(mode, copy); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := a.validateAutoConfigCatalog(r.Context(), mode, copy, false); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		copy.UpdatedAt = time.Now().Format(time.RFC3339)
		next.Champions[id][mode] = copy
	}
	a.autoConfig.Lock()
	defer a.autoConfig.Unlock()
	if a.autoConfig.settings.Version != version {
		http.Error(w, "配置已更新，请重试复制", 409)
		return
	}
	if err := a.saveAutoConfigLocked(next); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	respondJSON(w, a.autoConfig.settings)
}
func autoConfigEntryFor(s autoConfigSettings, id int64, mode, position string) (autoConfigEntry, string, bool) {
	m := s.Champions[strconv.FormatInt(id, 10)]
	if mode == "ranked" && position != "" {
		key := "ranked-" + strings.ToLower(position)
		if e, ok := m[key]; ok {
			return e, key, true
		}
	}
	e, ok := m[mode]
	return e, mode, ok
}
func autoConfigError(kind string) error {
	return fmt.Errorf("%s已失效或不适用于当前模式", kind)
}
