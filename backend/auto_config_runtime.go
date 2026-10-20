package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type autoConfigRuntime struct {
	Session       string            `json:"session"`
	ChampionID    int64             `json:"championId"`
	ModeKey       string            `json:"modeKey"`
	PageName      string            `json:"pageName,omitempty"`
	PageID        int64             `json:"pageId,omitempty"`
	Spells        autoConfigSpells  `json:"spellPair"`
	Results       map[string]string `json:"results"`
	Yielded       map[string]bool   `json:"yielded"`
	expectedRunes *autoConfigRunes
	client        *LCUClient
	cancel        context.CancelFunc
	generation    uint64
	observed      int64
	changedAt     time.Time
	running       bool
	done          bool
}

func autoConfigLocked(session lcuChampSelectSession, mode string) bool {
	id := champSelectCurrentChampion(session)
	if id <= 0 {
		return false
	}
	if mode == "aram" || mode == "hextech" {
		return true
	}
	if session.LocalPlayerCellID == nil {
		return false
	}
	for _, round := range session.Actions {
		for _, action := range round {
			if action.ActorCellID == *session.LocalPlayerCellID && action.Type == "pick" && action.Completed && action.ChampionID == id {
				return true
			}
		}
	}
	return false
}
func autoConfigSessionKey(s lcuChampSelectSession) string {
	cell := int64(-1)
	if s.LocalPlayerCellID != nil {
		cell = *s.LocalPlayerCellID
	}
	return fmt.Sprintf("%d:%d:%d", s.GameID, s.QueueID, cell)
}
func (a *app) resetAutoConfig() {
	a.autoConfig.Lock()
	r := &a.autoConfig.runtime
	if r.cancel != nil {
		r.cancel()
	}
	*r = autoConfigRuntime{generation: r.generation + 1}
	a.autoConfig.Unlock()
	a.emitAutoConfig()
}
func (a *app) handleAutoConfigState(w http.ResponseWriter, r *http.Request) {
	a.autoConfig.Lock()
	defer a.autoConfig.Unlock()
	respondJSON(w, a.autoConfig.runtime)
}
func (a *app) emitAutoConfig() {
	a.autoConfig.Lock()
	data, _ := json.Marshal(struct {
		Type  string            `json:"type"`
		State autoConfigRuntime `json:"state"`
	}{"auto-config", a.autoConfig.runtime})
	a.autoConfig.Unlock()
	a.broadcastEvent(string(data))
}
func (a *app) yieldAutoConfig(kind string) {
	a.autoConfig.Lock()
	r := &a.autoConfig.runtime
	if r.Yielded == nil {
		r.Yielded = map[string]bool{}
	}
	r.Yielded[kind] = true
	a.autoConfig.Unlock()
	a.emitAutoConfig()
}
func (a *app) observeAutoConfigEvent(ctx context.Context, client *LCUClient, event LCUEvent) {
	uri := strings.ToLower(event.URI)
	if uri == "/lol-gameflow/v1/gameflow-phase" {
		var phase string
		if json.Unmarshal(event.Data, &phase) == nil {
			a.autoConfig.Lock()
			previous := a.autoConfig.phase
			a.autoConfig.phase = phase
			a.autoConfig.Unlock()
			if phase == "ChampSelect" {
				if previous != "ChampSelect" {
					a.resetAutoConfig()
				}
				a.goSafe("auto-config-keys", func() { a.refreshSummonerKeys(ctx, client) })
				a.triggerAutoConfig(ctx, client)
			} else if phase == "GameStart" || phase == "InProgress" || phase == "Reconnect" {
				// Cancel writes while keeping the applied configuration visible for this game.
				a.autoConfig.Lock()
				r := &a.autoConfig.runtime
				if r.cancel != nil {
					r.cancel()
				}
				r.generation++
				a.autoConfig.Unlock()
			} else {
				a.resetAutoConfig()
			}
		}
		return
	}
	if strings.HasPrefix(uri, "/lol-perks/v1/currentpage") || strings.HasPrefix(uri, "/lol-perks/v1/pages/") {
		a.autoConfig.Lock()
		r := &a.autoConfig.runtime
		pageID := r.PageID
		expected := r.expectedRunes
		ready := r.done && r.Results["runes"] == "applied" && !a.autoConfig.processing
		a.autoConfig.Unlock()
		if ready {
			var page struct {
				ID int64 `json:"id"`
				autoConfigRunes
			}
			_ = json.Unmarshal(event.Data, &page)
			if page.ID == 0 {
				_ = json.Unmarshal(event.Data, &page.ID)
			}
			changed := strings.HasPrefix(uri, "/lol-perks/v1/currentpage") && page.ID > 0 && page.ID != pageID
			if page.ID == pageID && expected != nil && len(page.SelectedPerkIDs) > 0 {
				changed = page.PrimaryStyleID != expected.PrimaryStyleID || page.SubStyleID != expected.SubStyleID || !reflect.DeepEqual(page.SelectedPerkIDs, expected.SelectedPerkIDs)
			}
			if page.ID == pageID && event.EventType == "Delete" {
				changed = true
			}
			if changed {
				a.yieldAutoConfig("runes")
			}
		}
	}
	if uri == "/lol-champ-select/v1/session" {
		var session lcuChampSelectSession
		if json.Unmarshal(event.Data, &session) == nil {
			a.autoConfig.Lock()
			r := &a.autoConfig.runtime
			id := champSelectCurrentChampion(session)
			if r.client == client && r.Session == autoConfigSessionKey(session) && id != r.observed {
				r.observed = id
				r.changedAt = time.Now()
				r.done = false
				if r.cancel != nil {
					r.cancel()
				}
			}
			a.autoConfig.Unlock()
		}
		a.triggerAutoConfig(ctx, client)
	}
}
func (a *app) triggerAutoConfig(parent context.Context, client *LCUClient) {
	settings := a.autoConfigSnapshot()
	if !settings.Enabled {
		return
	}
	a.autoConfig.Lock()
	if a.autoConfig.processing {
		a.autoConfig.queued = true
		a.autoConfig.Unlock()
		return
	}
	a.autoConfig.processing = true
	a.autoConfig.Unlock()
	a.goSafe("auto-config-observe", func() {
		defer func() {
			a.autoConfig.Lock()
			a.autoConfig.processing = false
			queued := a.autoConfig.queued
			a.autoConfig.queued = false
			a.autoConfig.Unlock()
			if queued && parent.Err() == nil {
				a.triggerAutoConfig(parent, client)
			}
		}()
		ctx, cancel := context.WithTimeout(parent, 12*time.Second)
		defer cancel()
		var s lcuChampSelectSession
		if client.RequestJSON(ctx, "GET", "/lol-champ-select/v1/session", nil, &s) != nil {
			return
		}
		var flow lcuGameflowSession
		_ = client.RequestJSON(ctx, "GET", "/lol-gameflow/v1/session", nil, &flow)
		queue := s.QueueID
		if queue == 0 {
			queue = flow.GameData.Queue.ID
		}
		mapID := flow.GameData.Queue.MapID
		if mapID == 0 {
			mapID = flow.Map.ID
		}
		gameMode := firstNonEmpty(flow.GameData.Queue.GameMode, flow.Map.GameMode)
		mode := autoConfigMode(queue, gameMode, mapID)
		if mode == "" {
			return
		}
		id := champSelectCurrentChampion(s)
		key := autoConfigSessionKey(s)
		a.autoConfig.Lock()
		r := &a.autoConfig.runtime
		if r.Session != key || r.client != client {
			generation := r.generation + 1
			if r.cancel != nil {
				r.cancel()
			}
			*r = autoConfigRuntime{Session: key, client: client, generation: generation, running: true, Results: map[string]string{}, Yielded: map[string]bool{}}
		}
		// Client edits after a successful write yield only that category for the rest of this session.
		for _, p := range s.MyTeam {
			if p.CellID != nil && s.LocalPlayerCellID != nil && *p.CellID == *s.LocalPlayerCellID && r.Results["spells"] == "applied" && (p.Spell1ID != r.Spells.Spell1ID || p.Spell2ID != r.Spells.Spell2ID) {
				r.Yielded["spells"] = true
			}
		}
		changed := r.observed != id
		if changed {
			r.observed = id
			r.changedAt = time.Now()
			r.done = false
			if r.cancel != nil {
				r.cancel()
			}
		}
		if r.done || !autoConfigLocked(s, mode) {
			a.autoConfig.Unlock()
			return
		}
		delay := time.Duration(0)
		if mode == "aram" || mode == "hextech" {
			delay = 1500*time.Millisecond - time.Since(r.changedAt)
		}
		generation := r.generation
		r.cancel = cancel
		a.autoConfig.Unlock()
		if delay > 0 {
			if !waitWatchDelay(ctx, delay) {
				return
			}
		}
		if err := a.autoConfigGuard(ctx, client, s, generation, id); err != nil {
			return
		}
		position := ""
		for _, p := range s.MyTeam {
			if p.CellID != nil && s.LocalPlayerCellID != nil && *p.CellID == *s.LocalPlayerCellID {
				position = p.AssignedPosition
			}
		}
		entry, modeKey, exists := autoConfigEntryFor(settings, id, mode, position)
		a.applyAutoConfig(ctx, client, s, generation, id, mode, modeKey, entry, exists)
	})
}
func (a *app) autoConfigGuard(ctx context.Context, client *LCUClient, original lcuChampSelectSession, generation uint64, id int64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	a.autoConfig.Lock()
	valid := a.autoConfig.runtime.generation == generation && a.autoConfig.runtime.client == client
	a.autoConfig.Unlock()
	if !valid || !a.autoConfigSnapshot().Enabled {
		return errors.New("自动配置已取消")
	}
	latest, _, err := a.gameplayClient()
	if err != nil || latest != client {
		return errors.New("客户端已变化")
	}
	var phase string
	if client.RequestJSON(ctx, "GET", "/lol-gameflow/v1/gameflow-phase", nil, &phase) != nil || phase != "ChampSelect" {
		return errors.New("阶段已变化")
	}
	var s lcuChampSelectSession
	if err = client.RequestJSON(ctx, "GET", "/lol-champ-select/v1/session", nil, &s); err != nil {
		return err
	}
	if autoConfigSessionKey(s) != autoConfigSessionKey(original) || champSelectCurrentChampion(s) != id || !autoConfigLocked(s, autoConfigMode(s.QueueID, "", 0)) {
		return errors.New("英雄或会话已变化")
	}
	return nil
}

type autoConfigWriteGuardKey struct{}

func checkAutoConfigWrite(ctx context.Context) error {
	if guard, ok := ctx.Value(autoConfigWriteGuardKey{}).(func() error); ok {
		return guard()
	}
	return nil
}
func (a *app) applyAutoConfig(ctx context.Context, client *LCUClient, s lcuChampSelectSession, generation uint64, id int64, mode, modeKey string, entry autoConfigEntry, exists bool) {
	started := time.Now()
	guard := func() error { return a.autoConfigGuard(ctx, client, s, generation, id) }
	ctx = context.WithValue(ctx, autoConfigWriteGuardKey{}, guard)
	catalog, catErr := a.loadAutoConfigCatalog(ctx)
	settings := a.autoConfigSnapshot()
	results := map[string]string{}
	pageID := int64(0)
	name := bundledChampionNames()[id]
	if currentName := a.championNames()[id]; currentName != "" {
		name = currentName
	}
	if a.champions != nil {
		a.champions.mu.Lock()
		if meta, ok := a.champions.championMeta[int(id)]; ok && meta.NameZH != "" {
			name = meta.NameZH
		}
		a.champions.mu.Unlock()
	}
	if name == "" {
		name = fmt.Sprint(id)
	}
	pageName := runePageName(name, "自动配置")
	for _, kind := range []string{"runes", "spells", "items"} {
		enabled := map[string]bool{"runes": settings.Runes, "spells": settings.Spells, "items": settings.Items}[kind]
		a.autoConfig.Lock()
		yielded := a.autoConfig.runtime.Yielded[kind]
		oldPage := a.autoConfig.runtime.PageID
		a.autoConfig.Unlock()
		if !enabled || yielded || mode == "hextech" && kind == "runes" {
			results[kind] = "skipped-disabled"
			continue
		}
		if !exists || (kind == "runes" && entry.Runes == nil) || (kind == "spells" && entry.Spells == nil) || (kind == "items" && entry.Items == nil) {
			results[kind] = "skipped-no-config"
			continue
		}
		if catErr != nil {
			results[kind] = "failed"
			continue
		}
		if guard() != nil {
			return
		}
		var err error
		categoryGuard := func() error {
			if err := guard(); err != nil {
				return err
			}
			current := a.autoConfigSnapshot()
			enabled := map[string]bool{"runes": current.Runes, "spells": current.Spells, "items": current.Items}[kind]
			a.autoConfig.Lock()
			yielded := a.autoConfig.runtime.Yielded[kind]
			a.autoConfig.Unlock()
			if !enabled || yielded {
				return errors.New("玩家已切换或已关闭此项")
			}
			return nil
		}
		ctx = context.WithValue(ctx, autoConfigWriteGuardKey{}, categoryGuard)
		switch kind {
		case "runes":
			if !autoConfigRunesComplete(entry.Runes, catalog) {
				results[kind] = "skipped-incomplete"
				continue
			}
			if oldPage > 0 {
				var page lcuRunePage
				if client.RequestJSON(ctx, "GET", fmt.Sprintf("/lol-perks/v1/pages/%d", oldPage), nil, &page) == nil && page.ID == oldPage && strings.HasPrefix(page.Name, runePagePrefix) && (page.IsDeletable == nil || *page.IsDeletable) {
					if guard() != nil {
						return
					}
					err = client.RequestJSON(ctx, "DELETE", fmt.Sprintf("/lol-perks/v1/pages/%d", oldPage), nil, nil)
				}
			}
			if err == nil {
				r := entry.Runes
				pageID, err = applyRunePage(ctx, client, gameplayRuneApplyRequest{ChampionID: id, ChampionName: name, Source: "自动配置", PrimaryStyleID: r.PrimaryStyleID, SubStyleID: r.SubStyleID, SelectedPerkIDs: r.SelectedPerkIDs})
			}
		case "spells":
			if a.validateAutoConfigCatalog(ctx, mode, autoConfigEntry{Spells: entry.Spells}, false) != nil {
				results[kind] = "skipped-incomplete"
				continue
			}
			if categoryGuard() != nil {
				return
			}
			err = applySummonerSpells(ctx, client, entry.Spells.Spell1ID, entry.Spells.Spell2ID)
		case "items":
			if a.validateAutoConfigCatalog(ctx, mode, autoConfigEntry{Items: entry.Items}, false) != nil {
				results[kind] = "skipped-incomplete"
				continue
			}
			_, current, e := a.gameplayClient()
			if e != nil {
				return
			}
			mapID := int64(11)
			if mode == "aram" || mode == "hextech" {
				mapID = 12
			}
			req := gameplayItemSetApplyRequest{Storage: "recommended", Title: "[DL] " + name, ChampionID: id, MapID: mapID, Position: "other"}
			for i, ids := range [][]int64{entry.Items.Core, entry.Items.Situational} {
				if len(ids) == 0 {
					continue
				}
				block := gameplayItemSetBlockRequest{Type: []string{"核心出装", "备选"}[i]}
				for _, item := range ids {
					block.Items = append(block.Items, gameplayItemSetItemRequest{ID: item, Count: 1})
				}
				req.Blocks = append(req.Blocks, block)
			}
			if len(req.Blocks) == 0 {
				results[kind] = "skipped-incomplete"
				continue
			}
			a.itemSetMu.Lock()
			_, _, _, err = applyRecommendedItemSet(ctx, client, current, req, nil, categoryGuard)
			a.itemSetMu.Unlock()
		}
		if err != nil {
			results[kind] = "failed"
		} else {
			results[kind] = "applied"
		}
	}
	a.autoConfig.Lock()
	r := &a.autoConfig.runtime
	if r.generation != generation {
		a.autoConfig.Unlock()
		return
	}
	r.ChampionID = id
	r.ModeKey = modeKey
	r.PageName = pageName
	if pageID > 0 {
		r.PageID = pageID
	}
	if entry.Spells != nil {
		r.Spells = *entry.Spells
	}
	r.expectedRunes = entry.Runes
	r.Results = results
	r.done = true
	a.autoConfig.Unlock()
	a.emitAutoConfig()
	a.recordDiagnostic(map[string]any{"event": "auto_config_apply", "modeKey": modeKey, "results": results, "duration_ms": time.Since(started).Milliseconds(), "yielded": a.autoConfigYieldSnapshot()})
}

func (a *app) autoConfigYieldSnapshot() map[string]bool {
	a.autoConfig.Lock()
	defer a.autoConfig.Unlock()
	out := map[string]bool{}
	for k, v := range a.autoConfig.runtime.Yielded {
		out[k] = v
	}
	return out
}
