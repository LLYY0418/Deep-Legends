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
	Session           string            `json:"session"`
	ChampionID        int64             `json:"championId"`
	ModeKey           string            `json:"modeKey"`
	PageName          string            `json:"pageName,omitempty"`
	PageID            int64             `json:"pageId,omitempty"`
	Spells            autoConfigSpells  `json:"spellPair"`
	Results           map[string]string `json:"results"`
	Yielded           map[string]bool   `json:"yielded"`
	yieldedByChampion map[int64]map[string]bool
	observedSpells    autoConfigSpells
	spellsObserved    bool
	expectedRunes     *autoConfigRunes
	applyingRunes     bool
	client            *LCUClient
	cancel            context.CancelFunc
	generation        uint64
	observed          int64
	changedAt         time.Time
	running           bool
	skipReasons       map[string]bool
	done              bool
}

// Call with autoConfig locked. A pending hero-change job must not hide a
// player's edit made after an earlier successful write in this session.
func (r *autoConfigRuntime) observeSpells(s lcuChampSelectSession) {
	if champSelectCurrentChampion(s) != r.ChampionID {
		return
	}
	for _, p := range s.MyTeam {
		if p.CellID == nil || s.LocalPlayerCellID == nil || *p.CellID != *s.LocalPlayerCellID {
			continue
		}
		pair := autoConfigSpells{p.Spell1ID, p.Spell2ID}
		changed := r.Results["spells"] == "applied" && pair != r.Spells || r.spellsObserved && !r.done && pair != r.observedSpells
		if changed {
			if r.Yielded == nil {
				r.Yielded = map[string]bool{}
			}
			r.Yielded["spells"] = true
		}
		r.observedSpells = pair
		r.spellsObserved = true
	}
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

func autoConfigSessionMode(s lcuChampSelectSession, flow lcuGameflowSession) string {
	queue := s.QueueID
	if queue == 0 {
		queue = flow.GameData.Queue.ID
	}
	mapID := flow.GameData.Queue.MapID
	if mapID == 0 {
		mapID = flow.Map.ID
	}
	return autoConfigMode(queue, firstNonEmpty(flow.GameData.Queue.GameMode, flow.Map.GameMode), mapID)
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
func (r *autoConfigRuntime) selectChampion(id int64) {
	if r.yieldedByChampion == nil {
		r.yieldedByChampion = map[int64]map[string]bool{}
		if r.ChampionID > 0 {
			r.yieldedByChampion[r.ChampionID] = r.Yielded
		}
	}
	if r.ChampionID != id {
		r.Results = map[string]string{}
		r.PageID = 0
		r.expectedRunes = nil
		r.spellsObserved = false
		r.done = false
	}
	r.ChampionID = id
	if r.yieldedByChampion[id] == nil {
		r.yieldedByChampion[id] = map[string]bool{}
	}
	r.Yielded = r.yieldedByChampion[id]
}
func (a *app) yieldAutoConfig(kind string) {
	a.autoConfig.Lock()
	r := &a.autoConfig.runtime
	r.selectChampion(r.ChampionID)
	r.Yielded[kind] = true
	a.autoConfig.Unlock()
	a.emitAutoConfig()
}
func (a *app) yieldAutoConfigForChampion(ctx context.Context, client *LCUClient, id int64, kind string, extraKinds ...string) {
	var s lcuChampSelectSession
	if client.RequestJSON(ctx, "GET", "/lol-champ-select/v1/session", nil, &s) != nil {
		return
	}
	current := champSelectCurrentChampion(s)
	if id <= 0 {
		id = current
	}
	if id <= 0 || current > 0 && id != current {
		return
	}
	a.autoConfig.Lock()
	r := &a.autoConfig.runtime
	if r.Session != autoConfigSessionKey(s) || r.client != client {
		if r.cancel != nil {
			r.cancel()
		}
		*r = autoConfigRuntime{Session: autoConfigSessionKey(s), client: client, generation: r.generation + 1}
	}
	r.selectChampion(id)
	r.Yielded[kind] = true
	for _, extra := range extraKinds {
		r.Yielded[extra] = true
	}
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
				if phase == "EndOfGame" && previous != "EndOfGame" || previous == "ChampSelect" && canceledChampSelectPhase(phase) {
					a.goSafe("dl-rune-cleanup", func() {
						cleanupCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
						defer cancel()
						a.cleanupDLRunePages(cleanupCtx, client, previous == "ChampSelect" && canceledChampSelectPhase(phase))
					})
				}
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
		ready := r.Results["runes"] == "applied" && !r.applyingRunes
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

// Each reason is emitted at most once per champion-select session.
func (a *app) recordAutoConfigSkip(reason, modeKey string) {
	a.autoConfig.Lock()
	r := &a.autoConfig.runtime
	if r.skipReasons == nil {
		r.skipReasons = map[string]bool{}
	}
	if r.skipReasons[reason] {
		a.autoConfig.Unlock()
		return
	}
	r.skipReasons[reason] = true
	phase := a.autoConfig.phase
	a.autoConfig.Unlock()
	a.recordDiagnostic(map[string]any{"event": "auto_config_skip", "reason": reason, "modeKey": modeKey, "phase": phase})
}
func (a *app) triggerAutoConfig(parent context.Context, client *LCUClient) {
	settings := a.autoConfigSnapshot()
	a.autoConfig.Lock()
	if a.autoConfig.processing {
		a.autoConfig.queued = true
		a.autoConfig.Unlock()
		a.recordAutoConfigSkip("guard", "")
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
			a.recordAutoConfigSkip("guard", "")
			return
		}
		var flow lcuGameflowSession
		_ = client.RequestJSON(ctx, "GET", "/lol-gameflow/v1/session", nil, &flow)
		mode := autoConfigSessionMode(s, flow)
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
		r.observeSpells(s)
		r.selectChampion(id)
		r.observeSpells(s)
		changed := r.observed != id
		if changed {
			r.observed = id
			r.changedAt = time.Now()
			r.done = false
			if r.cancel != nil {
				r.cancel()
			}
		}
		reason := ""
		switch {
		case !settings.Enabled:
			reason = "disabled"
		case !settings.Runes && !settings.Spells && !settings.Items:
			reason = "category_off"
		case mode == "":
			reason = "no_mode"
		case !autoConfigLocked(s, mode):
			reason = "not_locked"
		case r.done:
			reason = "guard"
		}
		if reason != "" {
			a.autoConfig.Unlock()
			a.emitAutoConfig()
			a.recordAutoConfigSkip(reason, mode)
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
				a.recordAutoConfigSkip("guard", mode)
				return
			}
		}
		if err := a.autoConfigGuard(ctx, client, s, generation, id, mode); err != nil {
			a.recordAutoConfigSkip("guard", mode)
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
func (a *app) autoConfigGuard(ctx context.Context, client *LCUClient, original lcuChampSelectSession, generation uint64, id int64, expectedMode string) error {
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
	var flow lcuGameflowSession
	_ = client.RequestJSON(ctx, "GET", "/lol-gameflow/v1/session", nil, &flow)
	mode := autoConfigSessionMode(s, flow)
	if autoConfigSessionKey(s) != autoConfigSessionKey(original) || champSelectCurrentChampion(s) != id || mode == "" || mode != expectedMode || !autoConfigLocked(s, mode) {
		return errors.New("英雄或会话已变化")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	a.autoConfig.Lock()
	if a.autoConfig.runtime.generation != generation || a.autoConfig.runtime.client != client {
		a.autoConfig.Unlock()
		return errors.New("自动配置已取消")
	}
	a.autoConfig.runtime.observeSpells(s)
	a.autoConfig.Unlock()
	return nil
}

type autoConfigWriteGuardKey struct{}

func checkAutoConfigWrite(ctx context.Context) error {
	if guard, ok := ctx.Value(autoConfigWriteGuardKey{}).(func() error); ok {
		return guard()
	}
	return nil
}

func (a *app) applyAutoConfigRunes(ctx context.Context, client *LCUClient, generation uint64, oldPage int64, id int64, name string, runes *autoConfigRunes) (pageID int64, err error) {
	a.autoConfig.Lock()
	if a.autoConfig.runtime.generation != generation || ctx.Err() != nil {
		a.autoConfig.Unlock()
		return 0, errors.New("自动配置已取消")
	}
	a.autoConfig.runtime.applyingRunes = true
	a.autoConfig.Unlock()
	defer func() {
		a.autoConfig.Lock()
		r := &a.autoConfig.runtime
		if r.generation == generation {
			r.applyingRunes = false
			if err == nil && pageID > 0 {
				r.PageID = pageID
				r.expectedRunes = runes
				r.Results["runes"] = "applied"
			}
		}
		a.autoConfig.Unlock()
	}()
	trace := &runeApplyTrace{}
	ctx = context.WithValue(ctx, runeApplyTraceContextKey{}, trace)
	defer func() {
		outcome := "success"
		if err != nil {
			outcome = "failed"
		}
		a.recordDiagnostic(map[string]any{"event": "perk_apply_attempt", "source": "auto-config", "outcome": outcome, "dl_pages_before": trace.DLPagesBefore, "dl_pages_after": trace.DLPagesAfter, "action": trace.Action})
	}()
	return applyRunePage(ctx, client, gameplayRuneApplyRequest{ChampionID: id, ChampionName: name, Source: "自动配置", PrimaryStyleID: runes.PrimaryStyleID, SubStyleID: runes.SubStyleID, SelectedPerkIDs: runes.SelectedPerkIDs})
}

func (a *app) applyAutoConfig(ctx context.Context, client *LCUClient, s lcuChampSelectSession, generation uint64, id int64, mode, modeKey string, entry autoConfigEntry, exists bool, manualApply ...bool) {
	started := time.Now()
	skip := func(reason string) { a.recordAutoConfigSkip(reason, modeKey) }
	manual := len(manualApply) > 0 && manualApply[0]
	guard := func() error {
		if manual {
			return a.autoConfigManualGuard(ctx, client, s, id, mode)
		}
		return a.autoConfigGuard(ctx, client, s, generation, id, mode)
	}
	ctx = context.WithValue(ctx, autoConfigWriteGuardKey{}, guard)
	catalog, catErr := a.loadAutoConfigCatalog(ctx)
	settings := a.autoConfigSnapshot()
	if !manual && !settings.Enabled {
		skip("disabled")
		return
	}
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
		enabled := manual || map[string]bool{"runes": settings.Runes, "spells": settings.Spells, "items": settings.Items}[kind]
		a.autoConfig.Lock()
		yielded := !manual && a.autoConfig.runtime.Yielded[kind]
		oldPage := a.autoConfig.runtime.PageID
		a.autoConfig.Unlock()
		if !enabled || yielded || mode == "hextech" && kind == "runes" {
			results[kind] = "skipped-disabled"
			if !yielded {
				skip("category_off")
			}
			if yielded {
				results[kind] = "skipped-manual"
				skip("yielded")
			}
			continue
		}
		if !exists || (kind == "runes" && entry.Runes == nil) || (kind == "spells" && entry.Spells == nil) || (kind == "items" && entry.Items == nil) {
			results[kind] = "skipped-no-config"
			skip("no_config")
			continue
		}
		if catErr != nil {
			skip("guard")
			results[kind] = "failed"
			continue
		}
		if guard() != nil {
			skip("guard")
			return
		}
		var err error
		categoryGuard := func() error {
			if err := guard(); err != nil {
				skip("guard")
				return err
			}
			current := a.autoConfigSnapshot()
			enabled := manual || map[string]bool{"runes": current.Runes, "spells": current.Spells, "items": current.Items}[kind]
			a.autoConfig.Lock()
			yielded := !manual && a.autoConfig.runtime.Yielded[kind]
			a.autoConfig.Unlock()
			if !manual && !current.Enabled {
				skip("disabled")
				return errors.New("自动配置已关闭")
			}
			if !enabled || yielded {
				if yielded {
					skip("yielded")
				} else {
					skip("category_off")
				}
				return errors.New("玩家已切换或已关闭此项")
			}
			return nil
		}
		ctx = context.WithValue(ctx, autoConfigWriteGuardKey{}, categoryGuard)
		switch kind {
		case "runes":
			if !autoConfigRunesComplete(entry.Runes, catalog) {
				results[kind] = "skipped-incomplete"
				skip("no_config")
				continue
			}
			pageID, err = a.applyAutoConfigRunes(ctx, client, generation, oldPage, id, name, entry.Runes)
		case "spells":
			if a.validateAutoConfigCatalog(ctx, mode, autoConfigEntry{Spells: entry.Spells}, false) != nil {
				results[kind] = "skipped-incomplete"
				skip("no_config")
				continue
			}
			if categoryGuard() != nil {
				skip("guard")
				return
			}
			err = applySummonerSpells(ctx, client, entry.Spells.Spell1ID, entry.Spells.Spell2ID)
			if err == nil {
				// Later guards compare against the write just acknowledged by LCU,
				// rather than treating our new hero's different pair as a player edit.
				a.autoConfig.Lock()
				if a.autoConfig.runtime.generation == generation {
					a.autoConfig.runtime.Spells = *entry.Spells
					a.autoConfig.runtime.observedSpells = *entry.Spells
					a.autoConfig.runtime.spellsObserved = true
					a.autoConfig.runtime.Results["spells"] = "applied"
				}
				a.autoConfig.Unlock()
			}
		case "items":
			if a.validateAutoConfigCatalog(ctx, mode, autoConfigEntry{Items: entry.Items}, false) != nil {
				results[kind] = "skipped-incomplete"
				skip("no_config")
				continue
			}
			_, current, e := a.gameplayClient()
			if e != nil {
				skip("guard")
				return
			}
			mapID := int64(11)
			if mode == "aram" || mode == "hextech" {
				mapID = 12
			}
			req := gameplayItemSetApplyRequest{Storage: "recommended", Title: "[DL] " + name, ChampionID: id, MapID: mapID, Position: "other"}
			if len(entry.Items.Starter) > 0 {
				block := gameplayItemSetBlockRequest{Type: "出门装"}
				for _, item := range entry.Items.Starter {
					block.Items = append(block.Items, gameplayItemSetItemRequest{ID: item.ID, Count: int(item.Count)})
				}
				req.Blocks = append(req.Blocks, block)
			}
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
				skip("no_config")
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
		skip("guard")
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

func autoConfigDisplayedChampion(s lcuChampSelectSession) int64 {
	for _, p := range s.MyTeam {
		if p.CellID != nil && s.LocalPlayerCellID != nil && *p.CellID == *s.LocalPlayerCellID {
			if p.ChampionID > 0 {
				return p.ChampionID
			}
			return positiveChampionPickIntent(p.ChampionPickIntent)
		}
	}
	return 0
}
func (a *app) autoConfigManualGuard(ctx context.Context, client *LCUClient, original lcuChampSelectSession, id int64, mode string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	current, _, err := a.gameplayClient()
	if err != nil || current != client {
		return errors.New("客户端已变化")
	}
	var phase string
	if client.RequestJSON(ctx, "GET", "/lol-gameflow/v1/gameflow-phase", nil, &phase) != nil || phase != "ChampSelect" {
		return errors.New("只能在英雄选择阶段应用配置")
	}
	var session lcuChampSelectSession
	if err := client.RequestJSON(ctx, "GET", "/lol-champ-select/v1/session", nil, &session); err != nil {
		return err
	}
	var flow lcuGameflowSession
	if err := client.RequestJSON(ctx, "GET", "/lol-gameflow/v1/session", nil, &flow); err != nil {
		return err
	}
	if autoConfigSessionKey(session) != autoConfigSessionKey(original) || autoConfigDisplayedChampion(session) != id || autoConfigSessionMode(session, flow) != mode {
		return errors.New("英雄或模式已变化")
	}
	return nil
}
func (a *app) handleAutoConfigApply(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ChampionID int64 `json:"championId"`
	}
	if decodeAutoConfig(w, r, &request) != nil || request.ChampionID <= 0 {
		http.Error(w, "英雄无效", 400)
		return
	}
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, "请连接客户端", 409)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	var session lcuChampSelectSession
	var flow lcuGameflowSession
	if client.RequestJSON(ctx, "GET", "/lol-champ-select/v1/session", nil, &session) != nil || client.RequestJSON(ctx, "GET", "/lol-gameflow/v1/session", nil, &flow) != nil {
		http.Error(w, "读取英雄选择失败", 409)
		return
	}
	mode := autoConfigSessionMode(session, flow)
	if err := a.autoConfigManualGuard(ctx, client, session, request.ChampionID, mode); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	position := ""
	for _, p := range session.MyTeam {
		if p.CellID != nil && session.LocalPlayerCellID != nil && *p.CellID == *session.LocalPlayerCellID {
			position = p.AssignedPosition
		}
	}
	entry, key, exists := autoConfigEntryFor(a.autoConfigSnapshot(), request.ChampionID, mode, position)
	if !exists {
		http.Error(w, "当前英雄没有已保存配置", 404)
		return
	}
	a.autoConfig.Lock()
	runtime := &a.autoConfig.runtime
	if runtime.cancel != nil {
		runtime.cancel()
	}
	if runtime.Session != autoConfigSessionKey(session) || runtime.client != client {
		*runtime = autoConfigRuntime{Session: autoConfigSessionKey(session), client: client, generation: runtime.generation + 1}
	} else {
		runtime.generation++
	}
	runtime.selectChampion(request.ChampionID)
	generation := runtime.generation
	a.autoConfig.Unlock()
	a.applyAutoConfig(ctx, client, session, generation, request.ChampionID, mode, key, entry, true, true)
	a.autoConfig.Lock()
	results := map[string]string{}
	for k, v := range a.autoConfig.runtime.Results {
		results[k] = v
	}
	a.autoConfig.Unlock()
	respondJSON(w, map[string]any{"results": results})
}
