package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type autoConfigItem struct {
	ID                int64           `json:"id"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	IconPath          string          `json:"iconPath"`
	Price             int64           `json:"priceTotal"`
	Categories        []string        `json:"categories"`
	From              []int64         `json:"from"`
	To                []int64         `json:"to"`
	InStore           bool            `json:"inStore"`
	DisplayInItemSets bool            `json:"displayInItemSets"`
	RequiredChampion  string          `json:"requiredChampion"`
	RequiredAlly      string          `json:"requiredAlly"`
	Maps              map[string]bool `json:"maps"`
}
type autoConfigSpell struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IconPath    string   `json:"iconPath"`
	GameModes   []string `json:"gameModes"`
	Cooldown    float64  `json:"cooldown"`
}
type autoConfigCatalog struct {
	ClientConnected bool                `json:"clientConnected"`
	ItemsStatus     string              `json:"itemsStatus"`
	SpellsStatus    string              `json:"spellsStatus"`
	EventGameMode   string              `json:"eventGameMode,omitempty"`
	Styles          []gameplayPerkStyle `json:"styles"`
	StatModSlots    []gameplayPerkSlot  `json:"statModSlots"`
	Perks           []gameplayPerk      `json:"perks"`
	Items           []autoConfigItem    `json:"items"`
	Spells          []autoConfigSpell   `json:"spells"`
}

func (a *app) loadAutoConfigCatalog(ctx context.Context) (autoConfigCatalog, error) {
	client, _, _ := a.gameplayClient()
	key, _ := clientCatalogKey(client)
	a.autoConfig.Lock()
	if a.autoConfig.catalog != nil && a.autoConfig.catalogClient == client && a.autoConfig.catalogKey == key && time.Since(a.autoConfig.catalogAt) < 30*time.Minute {
		out := *a.autoConfig.catalog
		a.autoConfig.Unlock()
		return out, nil
	}
	a.autoConfig.Unlock()
	if client == nil {
		key = "ddragon"
	}
	perks, err := a.cachedGameplayPerkCatalog(ctx, key, func() (gameplayPerkCatalogResponse, error) { return a.loadGameplayPerkCatalog(ctx, client) })
	out := autoConfigCatalog{Styles: perks.Styles, StatModSlots: perks.StatModSlots, Perks: perks.Perks, Items: []autoConfigItem{}, Spells: []autoConfigSpell{}, ClientConnected: client != nil, ItemsStatus: "disconnected", SpellsStatus: "disconnected"}
	if err != nil {
		return out, err
	}
	// The disk rune catalog can predate shard descriptions. Enrich a copy so
	// readers of the shared cache never observe an in-place mutation.
	out.StatModSlots = append([]gameplayPerkSlot(nil), perks.StatModSlots...)
	for index := range out.StatModSlots {
		slot := &out.StatModSlots[index]
		slot.Perks = append([]gameplayPerk(nil), slot.Perks...)
		for perkIndex := range slot.Perks {
			perk := &slot.Perks[perkIndex]
			if perk.LongDescription == "" && perk.ShortDescription == "" {
				perk.LongDescription = runeShardDescription(int(perk.ID))
			}
		}
	}
	// Only the current client's pool authorizes automatic writes. Missing metadata fails closed.
	if client != nil {
		out.ItemsStatus, out.SpellsStatus = "failed", "failed"
		if err := a.clientCatalogJSON(ctx, client, "/lol-game-data/assets/v1/items.json", &out.Items); err == nil && len(out.Items) > 0 {
			out.ItemsStatus = "ready"
		} else {
			out.Items = []autoConfigItem{}
		}
		if err := a.clientCatalogJSON(ctx, client, "/lol-game-data/assets/v1/summoner-spells.json", &out.Spells); err == nil && len(out.Spells) > 0 {
			out.SpellsStatus = "ready"
		} else {
			out.Spells = []autoConfigSpell{}
		}
	}
	if a.champions != nil {
		descriptions, _ := a.champions.loadStaticDescriptions(ctx)
		for i := range out.Items {
			item := &out.Items[i]
			if len(item.Maps) == 0 {
				item.Maps = descriptions["item/"+strconv.FormatInt(item.ID, 10)+".png"].Maps
			}
		}
	}
	if len(out.Spells) > 0 && len(out.Items) > 0 {
		a.autoConfig.Lock()
		a.autoConfig.catalog = &out
		a.autoConfig.catalogClient = client
		a.autoConfig.catalogKey = key
		a.autoConfig.catalogAt = time.Now()
		a.autoConfig.Unlock()
	}
	return out, nil
}
func (a *app) handleAutoConfigCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := a.loadAutoConfigCatalog(r.Context())
	if err != nil {
		http.Error(w, "当前版本目录读取失败，请连接客户端后重试", 503)
		return
	}
	catalog.EventGameMode = a.currentAutoConfigGameMode(r.Context())
	respondJSON(w, catalog)
}
func autoConfigGameMode(mode string) string {
	switch mode {
	case "aram":
		return "ARAM"
	case "hextech":
		return "KIWI"
	case "urf":
		return "URF"
	case "event":
		return ""
	default:
		return "CLASSIC"
	}
}
func autoConfigSpellAllowed(s autoConfigSpell, mode string, currentMode ...string) bool {
	if s.ID <= 0 {
		return false
	}
	if mode == "hextech" && s.ID == 3 {
		return false
	}
	if (mode == "aram" || mode == "hextech") && (s.ID == 11 || s.ID == 12) {
		return false
	}
	want := autoConfigGameMode(mode)
	if mode == "event" && len(currentMode) > 0 {
		want = strings.ToUpper(currentMode[0])
	}
	for _, m := range s.GameModes {
		m = strings.ToUpper(m)
		if m == want || (mode == "hextech" && m == "ARAM_MAYHEM") {
			return true
		}
	}
	return false
}

// No current official mode-specific starting-gold rule was verifiable on 2026-10-24.
// R300 fallback: Summoner's Rift 500, Howling Abyss 1400, including URF/event.
// Official mode references checked: https://www.leagueoflegends.com/en-us/news/game-updates/patch-26-21-notes/
func autoConfigStarterBudget(mode string) int64 {
	if mode == "aram" || mode == "hextech" {
		return 1400
	}
	return 500
}
func autoConfigStarterAllowed(i autoConfigItem, mode string) bool {
	mapID := "11"
	if mode == "aram" || mode == "hextech" {
		mapID = "12"
	}
	return i.ID > 0 && i.InStore && i.DisplayInItemSets && i.RequiredChampion == "" && i.RequiredAlly == "" && i.Maps[mapID] && i.Price >= 0 && i.Price <= autoConfigStarterBudget(mode)
}
func autoConfigConsumable(i autoConfigItem) bool {
	for _, c := range i.Categories {
		if c == "Consumable" {
			return true
		}
	}
	return false
}
func autoConfigItemAllowed(i autoConfigItem, mode string) bool {
	if !i.InStore || !i.DisplayInItemSets || i.RequiredChampion != "" || i.RequiredAlly != "" {
		return false
	}
	mapID := "11"
	if mode == "aram" || mode == "hextech" {
		mapID = "12"
	}
	if !i.Maps[mapID] {
		return false
	}
	boots := false
	for _, c := range i.Categories {
		if c == "Consumable" || c == "Trinket" {
			return false
		}
		if c == "Boots" {
			for _, id := range i.From {
				if id == 1001 {
					boots = true
				}
			}
		}
	}
	for _, category := range i.Categories {
		if category == "Lane" || category == "Jungle" {
			return boots
		}
	}
	return boots || (len(i.To) == 0 && len(i.From) > 0)
}
func autoConfigRunesComplete(r *autoConfigRunes, c autoConfigCatalog) bool {
	if r == nil || len(r.SelectedPerkIDs) != 9 {
		return false
	}
	var p, q *gameplayPerkStyle
	for i := range c.Styles {
		s := &c.Styles[i]
		if s.ID == r.PrimaryStyleID {
			p = s
		}
		if s.ID == r.SubStyleID {
			q = s
		}
	}
	if p == nil || q == nil || p == q || len(p.Slots) < 4 || len(q.Slots) < 4 || len(c.StatModSlots) < 3 {
		return false
	}
	has := func(slot gameplayPerkSlot, id int64) bool {
		for _, perk := range slot.Perks {
			if perk.ID == id {
				return true
			}
		}
		return false
	}
	for row := 0; row < 4; row++ {
		if !has(p.Slots[row], r.SelectedPerkIDs[row]) {
			return false
		}
	}
	rows := map[int]bool{}
	for _, id := range r.SelectedPerkIDs[4:6] {
		found := false
		for row := 1; row < 4; row++ {
			if has(q.Slots[row], id) && !rows[row] {
				rows[row] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for row := 0; row < 3; row++ {
		if !has(c.StatModSlots[row], r.SelectedPerkIDs[6+row]) {
			return false
		}
	}
	return true
}
func (a *app) validateAutoConfigCatalog(ctx context.Context, mode string, e autoConfigEntry, complete bool) error {
	if !complete && e.Spells == nil && e.Items == nil {
		return nil
	}
	c, err := a.loadAutoConfigCatalog(ctx)
	if err != nil {
		return err
	}
	if complete && e.Runes != nil && !autoConfigRunesComplete(e.Runes, c) {
		return autoConfigError("符文")
	}
	if e.Spells != nil {
		gameMode := ""
		if mode == "event" {
			gameMode = a.currentAutoConfigGameMode(ctx)
		}
		valid := map[int64]bool{}
		for _, s := range c.Spells {
			if autoConfigSpellAllowed(s, mode, gameMode) {
				valid[s.ID] = true
			}
		}
		if !valid[e.Spells.Spell1ID] || !valid[e.Spells.Spell2ID] {
			return autoConfigError("召唤师技能")
		}
	}
	if e.Items != nil {
		starterCatalog := map[int64]autoConfigItem{}
		for _, item := range c.Items {
			starterCatalog[item.ID] = item
		}
		total := int64(0)
		for _, item := range e.Items.Starter {
			meta, ok := starterCatalog[item.ID]
			if !ok || !autoConfigStarterAllowed(meta, mode) || item.Count < 1 || item.Count > 99 || item.Count > 1 && !autoConfigConsumable(meta) {
				return autoConfigError("出门装")
			}
			total += meta.Price * item.Count
		}
		if total > autoConfigStarterBudget(mode) {
			return errors.New("超出出门预算")
		}
		valid := map[int64]bool{}
		for _, i := range c.Items {
			if autoConfigItemAllowed(i, mode) {
				valid[i.ID] = true
			}
		}
		for _, ids := range [][]int64{e.Items.Core, e.Items.Situational} {
			for _, id := range ids {
				if !valid[id] {
					return autoConfigError("装备")
				}
			}
		}
	}
	return nil
}

// Keep raw long descriptions in the editor catalog; markup is escaped/formatted by the renderer.

func (a *app) currentAutoConfigGameMode(ctx context.Context) string {
	client, _, err := a.gameplayClient()
	if err != nil {
		return ""
	}
	var flow lcuGameflowSession
	if client.RequestJSON(ctx, "GET", "/lol-gameflow/v1/session", nil, &flow) != nil {
		return ""
	}
	mode := firstNonEmpty(flow.GameData.Queue.GameMode, flow.Map.GameMode)
	if autoConfigMode(flow.GameData.Queue.ID, mode, flow.Map.ID) == "event" {
		return mode
	}
	return ""
}
