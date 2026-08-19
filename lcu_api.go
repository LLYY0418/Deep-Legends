package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	capabilityAvailable   = "available"
	capabilityUnsupported = "unsupported"
	capabilityFailed      = "failed"
)

type EndpointCapability struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	State  string `json:"state"`
	Count  int    `json:"count"`
	Detail string `json:"detail,omitempty"`
}

type SummonerProfile struct {
	BackgroundSkinID       int64  `json:"backgroundSkinId,omitempty"`
	BackgroundSkinName     string `json:"backgroundSkinName,omitempty"`
	BackgroundSkinAugments string `json:"backgroundSkinAugments,omitempty"`
	Regalia                string `json:"regalia,omitempty"`
}

type LootItem struct {
	LootID               string `json:"lootId"`
	LootName             string `json:"lootName,omitempty"`
	LocalizedName        string `json:"localizedName,omitempty"`
	DisplayName          string `json:"displayName,omitempty"`
	Category             string `json:"category,omitempty"`
	SkinID               int64  `json:"skinId,omitempty"`
	SkinOwned            bool   `json:"skinOwned"`
	SkinOwnedKnown       bool   `json:"skinOwnedKnown,omitempty"`
	LocalizedDescription string `json:"localizedDescription,omitempty"`
	DisplayCategories    string `json:"displayCategories,omitempty"`
	Type                 string `json:"type,omitempty"`
	Rarity               string `json:"rarity,omitempty"`
	ItemStatus           string `json:"itemStatus,omitempty"`
	RedeemableStatus     string `json:"redeemableStatus,omitempty"`
	Count                int    `json:"count"`
	StoreItemID          int64  `json:"storeItemId,omitempty"`
	DisenchantValue      int    `json:"disenchantValue,omitempty"`
	UpgradeEssenceValue  int    `json:"upgradeEssenceValue,omitempty"`
	Asset                string `json:"asset,omitempty"`
	TilePath             string `json:"tilePath,omitempty"`
	SplashPath           string `json:"splashPath,omitempty"`
	IsSkinRelated        bool   `json:"isSkinRelated"`
	Kind                 string `json:"kind,omitempty"`
}

var lootChineseNames = map[string]string{
	"CURRENCY_CHAMPION":      "蓝色精粹",
	"CURRENCY_COSMETIC":      "橙色精粹",
	"MATERIAL_KEY":           "战利品宝箱钥匙",
	"MATERIAL_KEY_FRAGMENT":  "钥匙碎片",
	"CHEST_CHAMPION_MASTERY": "战利品宝箱",
}

var lootClientIcons = map[string]string{
	"CURRENCY_CHAMPION":      "/fe/lol-loot/assets/loot_item_icons/currency_champion.png",
	"CURRENCY_COSMETIC":      "/fe/lol-loot/assets/loot_item_icons/currency_cosmetic.png",
	"MATERIAL_KEY":           "/fe/lol-loot/assets/loot_item_icons/material_key.png",
	"MATERIAL_KEY_FRAGMENT":  "/fe/lol-loot/assets/loot_item_icons/material_key_fragment.png",
	"CHEST_CHAMPION_MASTERY": "/fe/lol-loot/assets/loot_item_icons/chest_champion_mastery.png",
}

func enrichLootItems(items []LootItem, skins []Skin) []LootItem {
	byID := make(map[int64]Skin, len(skins))
	championNames := make(map[int64]string)
	for _, skin := range skins {
		byID[skin.ID] = skin
		if skin.ChampionID > 0 && strings.TrimSpace(skin.ChampionName) != "" {
			championNames[skin.ChampionID] = skin.ChampionName
		}
	}
	for index := range items {
		item := &items[index]
		for _, raw := range []string{item.LootID, item.LootName} {
			token := normalizeLootToken(raw)
			if name, ok := lootChineseNames[token]; ok {
				item.DisplayName = name
			}
			if item.Asset == "" {
				item.Asset = lootClientIcons[token]
			}
		}
		item.Category = lootCategory(*item)
		if item.Category == "皮肤" {
			item.Kind = lootSkinKind(*item)
			if skinID := lootSkinID(*item); skinID > 0 {
				if skin, ok := byID[skinID]; ok {
					item.SkinID = skinID
					item.DisplayName = skin.Name
					item.SkinOwned = skin.Owned
					item.SkinOwnedKnown = true
					if item.TilePath == "" {
						item.TilePath = skin.TilePath
					}
					if item.SplashPath == "" {
						item.SplashPath = skin.SplashPath
					}
				}
			}
		} else if item.Category == "英雄" {
			item.Kind = "英雄碎片"
			if championID := lootChampionID(*item); championID > 0 {
				if name := strings.TrimSpace(championNames[championID]); name != "" {
					item.DisplayName = name
				}
				if item.Asset == "" {
					item.Asset = fmt.Sprintf("/lol-game-data/assets/v1/champion-icons/%d.png", championID)
				}
			}
		}
		if item.DisplayName == "" {
			item.DisplayName = lootDisplayName(*item)
		}
		if strings.TrimSpace(item.DisplayName) == "" || item.DisplayName == "未命名战利品" {
			item.DisplayName = "未识别材料"
		}
	}
	return items
}

func lootSkinKind(item LootItem) string {
	combined := strings.Join([]string{normalizeLootToken(item.Type), normalizeLootToken(item.DisplayCategories), normalizeLootToken(item.LootID), normalizeLootToken(item.LootName)}, "_")
	if strings.Contains(combined, "RENTAL") || strings.Contains(combined, "SHARD") {
		return "皮肤碎片"
	}
	return "完整皮肤"
}

func lootCategory(item LootItem) string {
	combined := strings.Join([]string{normalizeLootToken(item.Type), normalizeLootToken(item.DisplayCategories), normalizeLootToken(item.LootID), normalizeLootToken(item.LootName)}, "_")
	switch {
	case item.IsSkinRelated:
		return "皮肤"
	case lootChampionID(item) > 0:
		return "英雄"
	case strings.Contains(combined, "LITTLE_LEGEND"), strings.Contains(combined, "COMPANION"), strings.Contains(combined, "TFT_COMPANION"):
		return "小小英雄"
	case strings.Contains(combined, "STATSTONE"), strings.Contains(combined, "ETERNAL"):
		return "永恒星碑"
	case strings.Contains(combined, "EMOTE"):
		return "表情"
	case strings.Contains(combined, "WARD"):
		return "守卫"
	case strings.Contains(combined, "PROFILE_ICON"), strings.Contains(combined, "SUMMONER_ICON"), strings.Contains(combined, "ICON"):
		return "图标"
	default:
		return "材料"
	}
}

func normalizeLootToken(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", " ", "_").Replace(value)
	return value
}

func lootSkinID(item LootItem) int64 {
	for _, raw := range []string{item.LootID, item.LootName} {
		token := normalizeLootToken(raw)
		if !strings.Contains(token, "SKIN") {
			continue
		}
		parts := strings.Split(token, "_")
		for index := len(parts) - 1; index >= 0; index-- {
			id, err := strconv.ParseInt(parts[index], 10, 64)
			if err == nil && id > 0 {
				return id
			}
		}
	}
	return 0
}

func lootChampionID(item LootItem) int64 {
	for _, raw := range []string{item.LootID, item.LootName} {
		token := normalizeLootToken(raw)
		if !strings.HasPrefix(token, "CHAMPION_") || strings.HasPrefix(token, "CHAMPION_SKIN_") {
			continue
		}
		parts := strings.Split(token, "_")
		for index := len(parts) - 1; index >= 1; index-- {
			id, err := strconv.ParseInt(parts[index], 10, 64)
			if err == nil && id > 0 {
				return id
			}
		}
	}
	return 0
}

type RewardItem struct {
	ID       string `json:"id,omitempty"`
	ItemID   string `json:"itemId,omitempty"`
	ItemType string `json:"itemType,omitempty"`
	Title    string `json:"title,omitempty"`
	Details  string `json:"details,omitempty"`
	Quantity int    `json:"quantity"`
	IconURL  string `json:"iconUrl,omitempty"`
}

type RewardGrant struct {
	ID          string       `json:"id"`
	Status      string       `json:"status"`
	DateCreated string       `json:"dateCreated,omitempty"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Items       []RewardItem `json:"items"`
}

type AccountData struct {
	Profile            SummonerProfile      `json:"profile"`
	Loot               []LootItem           `json:"loot"`
	Rewards            []RewardGrant        `json:"rewards"`
	SanctumSparks      int                  `json:"sanctumSparks"`
	SanctumSparksKnown bool                 `json:"sanctumSparksKnown"`
	Capabilities       []EndpointCapability `json:"capabilities"`
}

type ChampionMastery struct {
	ChampionID     int64 `json:"championId"`
	ChampionLevel  int64 `json:"championLevel"`
	ChampionPoints int64 `json:"championPoints"`
	LastPlayTime   int64 `json:"lastPlayTime"`
}

type SkinDetailData struct {
	PriceRP              int  `json:"priceRp,omitempty"`
	PriceKnown           bool `json:"priceKnown"`
	HasBorder            bool `json:"hasBorder"`
	BorderOwnershipKnown bool `json:"borderOwnershipKnown"`
	OwnsBorder           bool `json:"ownsBorder"`
}

type SummonerAPI struct{ client *LCUClient }
type SkinCatalogAPI struct{ client *LCUClient }
type InventoryAPI struct{ client *LCUClient }
type LootAPI struct{ client *LCUClient }
type RewardsAPI struct{ client *LCUClient }
type ChampionMasteryAPI struct{ client *LCUClient }
type StoreAPI struct{ client *LCUClient }
type SkinAppearanceAPI struct{ client *LCUClient }

func NewSummonerAPI(client *LCUClient) SummonerAPI       { return SummonerAPI{client: client} }
func NewSkinCatalogAPI(client *LCUClient) SkinCatalogAPI { return SkinCatalogAPI{client: client} }
func NewInventoryAPI(client *LCUClient) InventoryAPI     { return InventoryAPI{client: client} }
func NewLootAPI(client *LCUClient) LootAPI               { return LootAPI{client: client} }
func NewRewardsAPI(client *LCUClient) RewardsAPI         { return RewardsAPI{client: client} }
func NewChampionMasteryAPI(client *LCUClient) ChampionMasteryAPI {
	return ChampionMasteryAPI{client: client}
}
func NewStoreAPI(client *LCUClient) StoreAPI { return StoreAPI{client: client} }
func NewSkinAppearanceAPI(client *LCUClient) SkinAppearanceAPI {
	return SkinAppearanceAPI{client: client}
}

func (api SummonerAPI) Current() (Summoner, error) {
	var object map[string]any
	if err := api.client.GetJSON("/lol-summoner/v1/current-summoner", &object); err != nil {
		return Summoner{}, fmt.Errorf("current summoner: %w", err)
	}
	summoner := Summoner{
		SummonerID: firstInt(object, "summonerId"), AccountID: firstInt(object, "accountId"),
		PUUID: firstString(object, "puuid"), DisplayName: firstString(object, "displayName"),
		GameName: firstString(object, "gameName"), TagLine: firstString(object, "tagLine"),
		ProfileIconID: firstInt(object, "profileIconId"), SummonerLevel: firstInt(object, "summonerLevel"),
	}
	if summoner.SummonerID == 0 {
		return Summoner{}, errors.New("current summoner is not ready")
	}
	return summoner, nil
}

func (api SummonerAPI) Profile() (SummonerProfile, EndpointCapability) {
	const path = "/lol-summoner/v1/current-summoner/summoner-profile"
	var profile SummonerProfile
	capability := EndpointCapability{Name: "summoner-profile", Path: path}
	if err := api.client.GetJSON(path, &profile); err != nil {
		return SummonerProfile{}, optionalCapabilityError(capability, err)
	}
	capability.State = capabilityAvailable
	capability.Count = 1
	return profile, capability
}

func (api SkinCatalogAPI) Load() ([]Skin, error) {
	return loadSkinCatalog(api.client)
}

func (api InventoryAPI) OwnedSkinIDs(summonerID int64, catalog []Skin) (map[int64]bool, []OwnershipSourceStatus, error) {
	return loadOwnedSkinInventory(api.client, summonerID, catalog)
}

func (api InventoryAPI) OwnedChampionIDs(summonerID int64) (map[int64]bool, EndpointCapability) {
	paths := []string{
		fmt.Sprintf("/lol-champions/v1/inventories/%d/champions-minimal", summonerID),
		fmt.Sprintf("/lol-champions/v1/inventories/%d/champions", summonerID),
	}
	capability := EndpointCapability{Name: "owned-champions", Path: paths[0]}
	for _, path := range paths {
		data, err := api.client.GetBytes(path)
		if err != nil {
			capability = optionalCapabilityError(EndpointCapability{Name: capability.Name, Path: path}, err)
			continue
		}
		var root any
		if err := json.Unmarshal(data, &root); err != nil {
			capability.State = capabilityFailed
			capability.Detail = "客户端返回了无效的英雄库存数据"
			return map[int64]bool{}, capability
		}
		ids := extractOwnedChampionIDs(root)
		capability.Path = path
		capability.State = capabilityAvailable
		capability.Count = len(ids)
		return ids, capability
	}
	return map[int64]bool{}, capability
}

func (api InventoryAPI) SkinAcquisitionDates(summonerID int64, ownedIDs map[int64]bool) (map[int64]string, EndpointCapability) {
	paths := []string{
		fmt.Sprintf("/lol-champions/v1/inventories/%d/skins-minimal", summonerID),
		fmt.Sprintf("/lol-champions/v1/inventories/%d/champions", summonerID),
		"/lol-inventory/v2/inventory/CHAMPION_SKIN",
	}
	capability := EndpointCapability{Name: "skin-acquisition-time", Path: "3 个本机库存端点"}
	merged := map[int64]string{}
	conflicts := map[int64]bool{}
	supported := 0
	invalid := 0
	for _, path := range paths {
		data, err := api.client.GetBytes(path)
		if err != nil {
			continue
		}
		var root any
		if err := json.Unmarshal(data, &root); err != nil {
			invalid++
			continue
		}
		supported++
		for id, value := range extractSkinAcquisitionDatesForOwned(root, time.Now(), ownedIDs) {
			if existing, ok := merged[id]; ok && existing != value {
				conflicts[id] = true
				delete(merged, id)
				continue
			}
			if !conflicts[id] {
				merged[id] = value
			}
		}
	}
	capability.Count = len(merged)
	if supported == 0 {
		capability.State = capabilityUnsupported
		capability.Detail = "当前国服客户端未提供可读取的皮肤获取时间端点"
		return merged, capability
	}
	capability.State = capabilityAvailable
	capability.Detail = fmt.Sprintf("已读取 %d/3 个本机端点", supported)
	if len(merged) == 0 {
		capability.Detail += "，但没有返回可核验的获取日期；不会伪造时间顺序"
	}
	if invalid > 0 {
		capability.Detail += fmt.Sprintf("；%d 个端点数据格式无效", invalid)
	}
	if len(conflicts) > 0 {
		capability.Detail += fmt.Sprintf("；已排除 %d 个时间冲突项", len(conflicts))
	}
	return merged, capability
}

func (api ChampionMasteryAPI) All(puuid string) (map[int64]ChampionMastery, EndpointCapability) {
	path := "/lol-champion-mastery/v1/" + url.PathEscape(strings.TrimSpace(puuid)) + "/champion-mastery"
	capability := EndpointCapability{Name: "champion-mastery", Path: path}
	if strings.TrimSpace(puuid) == "" {
		capability.State = capabilityUnsupported
		capability.Detail = "当前客户端未提供玩家 PUUID"
		return map[int64]ChampionMastery{}, capability
	}
	data, err := api.client.GetBytes(path)
	if err != nil {
		return map[int64]ChampionMastery{}, optionalCapabilityError(capability, err)
	}
	masteries, err := decodeChampionMasteries(data)
	if err != nil {
		capability.State = capabilityFailed
		capability.Detail = "客户端返回了无效的英雄熟练度数据"
		return map[int64]ChampionMastery{}, capability
	}
	result := make(map[int64]ChampionMastery, len(masteries))
	for _, mastery := range masteries {
		if mastery.ChampionID > 0 {
			result[mastery.ChampionID] = mastery
		}
	}
	capability.State = capabilityAvailable
	capability.Count = len(result)
	return result, capability
}

func (api StoreAPI) SkinPrice(skinID int64) (int, bool) {
	if skinID <= 0 {
		return 0, false
	}
	data, err := api.client.GetBytes(fmt.Sprintf("/lol-store/v1/skins/%d", skinID))
	if err != nil {
		return 0, false
	}
	var root any
	if json.Unmarshal(data, &root) != nil {
		return 0, false
	}
	return extractRPPrice(root)
}

func (api SkinAppearanceAPI) BorderStatus(skin Skin) (hasBorder, ownershipKnown, owned bool) {
	data, err := api.client.GetBytes(fmt.Sprintf("/lol-game-data/assets/v1/champions/%d.json", skin.ChampionID))
	if err != nil {
		return false, false, false
	}
	var root any
	if json.Unmarshal(data, &root) != nil {
		return false, false, false
	}
	hasBorder, contentIDs := skinBorderContentIDs(root, skin.ID)
	if !hasBorder || len(contentIDs) == 0 {
		return hasBorder, false, false
	}
	sawSupportedInventory := false
	for _, path := range []string{"/lol-inventory/v2/inventory/SKIN_BORDER", "/lol-inventory/v1/inventory?inventoryTypes=SKIN_BORDER", "/lol-regalia/v3/inventory/SKIN_BORDER"} {
		inventory, requestErr := api.client.GetBytes(path)
		if requestErr != nil {
			continue
		}
		var inventoryRoot any
		if json.Unmarshal(inventory, &inventoryRoot) != nil {
			continue
		}
		sawSupportedInventory = true
		ownedIDs := extractInventoryContentIDs(inventoryRoot)
		for contentID := range contentIDs {
			if ownedIDs[contentID] {
				return true, true, true
			}
		}
	}
	return true, sawSupportedInventory, false
}

func (api LootAPI) PlayerLoot() ([]LootItem, EndpointCapability) {
	const path = "/lol-loot/v1/player-loot-map"
	var lootMap map[string]LootItem
	capability := EndpointCapability{Name: "player-loot", Path: path}
	if err := api.client.GetJSON(path, &lootMap); err != nil {
		return nil, optionalCapabilityError(capability, err)
	}
	items := make([]LootItem, 0, len(lootMap))
	for key, item := range lootMap {
		if item.LootID == "" {
			item.LootID = key
		}
		item.Asset = sanitizeClientImagePath(item.Asset)
		item.TilePath = sanitizeClientImagePath(item.TilePath)
		item.SplashPath = sanitizeClientImagePath(item.SplashPath)
		item.IsSkinRelated = isSkinLoot(item)
		if item.Count > 0 {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsSkinRelated != items[j].IsSkinRelated {
			return items[i].IsSkinRelated
		}
		left, right := lootDisplayName(items[i]), lootDisplayName(items[j])
		if left != right {
			return left < right
		}
		return items[i].LootID < items[j].LootID
	})
	capability.State = capabilityAvailable
	capability.Count = len(items)
	return items, capability
}

func (api LootAPI) SanctumSparks() (int, EndpointCapability) {
	const path = "/lol-inventory/v1/wallet/lol_blessing_token"
	capability := EndpointCapability{Name: "sanctum-sparks", Path: path}
	var payload any
	if err := api.client.GetJSON(path, &payload); err != nil {
		return 0, optionalCapabilityError(capability, err)
	}
	balance, ok := walletBalance(payload)
	if !ok || balance < 0 {
		capability.State = capabilityFailed
		capability.Detail = "客户端返回了无法识别的圣堂花火余额格式"
		return 0, capability
	}
	capability.State = capabilityAvailable
	capability.Count = balance
	return balance, capability
}

func walletBalance(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if typed >= 0 && typed == float64(int(typed)) {
			return int(typed), true
		}
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil && parsed >= 0
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil && parsed >= 0
	case map[string]any:
		for _, key := range []string{"lol_blessing_token", "balance", "amount", "quantity", "count", "value"} {
			if candidate, exists := typed[key]; exists {
				if balance, ok := walletBalance(candidate); ok {
					return balance, true
				}
			}
		}
	}
	return 0, false
}

func sanitizeClientImagePath(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, prefix := range []string{"/lol-game-data/assets/", "/fe/lol-loot/assets/loot_item_icons/", "/fe/lol-static-assets/images/currency/icons/"} {
		if strings.HasPrefix(lower, prefix) && !strings.Contains(value, "..") {
			return value
		}
	}
	return ""
}

func (api RewardsAPI) PendingGrants() ([]RewardGrant, EndpointCapability) {
	const path = "/lol-rewards/v1/grants"
	var raw []struct {
		Info struct {
			ID            string `json:"id"`
			Status        string `json:"status"`
			DateCreated   string `json:"dateCreated"`
			GrantElements []struct {
				ElementID string `json:"elementId"`
				ItemID    string `json:"itemId"`
				ItemType  string `json:"itemType"`
				Quantity  int    `json:"quantity"`
			} `json:"grantElements"`
		} `json:"info"`
		RewardGroup struct {
			Localizations struct {
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"localizations"`
			Rewards []struct {
				ID            string `json:"id"`
				ItemID        string `json:"itemId"`
				ItemType      string `json:"itemType"`
				Quantity      int    `json:"quantity"`
				Localizations struct {
					Title   string `json:"title"`
					Details string `json:"details"`
				} `json:"localizations"`
				Media struct {
					IconURL string `json:"iconUrl"`
				} `json:"media"`
			} `json:"rewards"`
		} `json:"rewardGroup"`
	}
	capability := EndpointCapability{Name: "pending-rewards", Path: path}
	if err := api.client.GetJSON(path, &raw); err != nil {
		return nil, optionalCapabilityError(capability, err)
	}
	grants := make([]RewardGrant, 0, len(raw))
	for _, source := range raw {
		if !isPendingRewardStatus(source.Info.Status) {
			continue
		}
		grant := RewardGrant{ID: source.Info.ID, Status: source.Info.Status, DateCreated: source.Info.DateCreated, Title: source.RewardGroup.Localizations.Title, Description: source.RewardGroup.Localizations.Description}
		for _, item := range source.RewardGroup.Rewards {
			grant.Items = append(grant.Items, RewardItem{ID: item.ID, ItemID: item.ItemID, ItemType: item.ItemType, Title: item.Localizations.Title, Details: item.Localizations.Details, Quantity: item.Quantity, IconURL: sanitizeAssetPath(item.Media.IconURL)})
		}
		if len(grant.Items) == 0 {
			for _, item := range source.Info.GrantElements {
				grant.Items = append(grant.Items, RewardItem{ID: item.ElementID, ItemID: item.ItemID, ItemType: item.ItemType, Quantity: item.Quantity})
			}
		}
		grants = append(grants, grant)
	}
	capability.State = capabilityAvailable
	capability.Count = len(grants)
	return grants, capability
}

func optionalCapabilityError(capability EndpointCapability, err error) EndpointCapability {
	var httpErr *LCUHTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
		capability.State = capabilityUnsupported
		capability.Detail = "当前客户端版本未提供此只读接口"
		return capability
	}
	capability.State = capabilityFailed
	capability.Detail = "读取失败；不影响皮肤剩余计算"
	return capability
}

func isSkinLoot(item LootItem) bool {
	normalize := func(value string) string {
		return strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"), " ", "_"))
	}
	typeName := normalize(item.Type)
	categories := normalize(item.DisplayCategories)
	lootID := normalize(item.LootID)
	for _, value := range []string{typeName, categories} {
		if value == "SKIN" || value == "CHAMPION_SKIN" || value == "SKIN_SHARD" || strings.HasPrefix(value, "CHAMPION_SKIN_") {
			return true
		}
	}
	return strings.HasPrefix(lootID, "CHAMPION_SKIN_") || strings.HasPrefix(lootID, "SKIN_SHARD_")
}

func lootDisplayName(item LootItem) string {
	if strings.TrimSpace(item.LocalizedName) != "" {
		return strings.TrimSpace(item.LocalizedName)
	}
	if strings.TrimSpace(item.LootName) != "" {
		return strings.TrimSpace(item.LootName)
	}
	return item.LootID
}

func isPendingRewardStatus(status string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	return normalized != "" && normalized != "CLAIMED" && normalized != "FULFILLED" && normalized != "COMPLETED" && normalized != "EXPIRED" && normalized != "REVOKED"
}

func decodeChampionMasteries(data []byte) ([]ChampionMastery, error) {
	var direct []ChampionMastery
	if err := json.Unmarshal(data, &direct); err == nil {
		return direct, nil
	}
	var wrapper struct {
		Masteries []ChampionMastery `json:"masteries"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Masteries, nil
}

func extractOwnedChampionIDs(value any) map[int64]bool {
	result := map[int64]bool{}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			_, isSkin := typed["championId"]
			id := firstInt(typed, "id")
			owned, evidence, conflict := ownershipDecision(typed)
			if !isSkin && id > 0 && id < 60000 && owned && evidence && !conflict {
				result[id] = true
			}
			for key, child := range typed {
				switch strings.ToLower(key) {
				case "champions", "items", "inventory", "data":
					walk(child)
				}
			}
		}
	}
	walk(value)
	return result
}

func extractSkinAcquisitionDates(value any, now time.Time) map[int64]string {
	return extractSkinAcquisitionDatesForOwned(value, now, nil)
}

func extractSkinAcquisitionDatesForOwned(value any, now time.Time, ownedIDs map[int64]bool) map[int64]string {
	result := map[int64]string{}
	conflicts := map[int64]bool{}
	latestAllowed := now.Add(24 * time.Hour).UnixMilli()
	const earliestAllowed int64 = 1230768000000 // 2009-01-01 UTC
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			owned, evidence, conflict := ownershipDecision(typed)
			id, _ := inventorySkinIdentity(typed)
			if id == 0 {
				id = firstInt(typed, "id")
			}
			verifiedOwned := owned && evidence && !conflict
			if len(ownedIDs) > 0 {
				verifiedOwned = ownedIDs[id]
			}
			if verifiedOwned {
				purchaseDate := skinPurchaseDateMillis(typed)
				if id > 0 && purchaseDate >= earliestAllowed && purchaseDate <= latestAllowed {
					formatted := time.UnixMilli(purchaseDate).UTC().Format(time.RFC3339)
					if existing, ok := result[id]; ok && existing != formatted {
						conflicts[id] = true
						delete(result, id)
					} else if !conflicts[id] {
						result[id] = formatted
					}
				}
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func skinPurchaseDateMillis(object map[string]any) int64 {
	if value := firstPurchaseDateMillis(object); value > 0 {
		return value
	}
	ownership, ok := object["ownership"].(map[string]any)
	if !ok {
		return 0
	}
	if value := firstPurchaseDateMillis(ownership); value > 0 {
		return value
	}
	if rental, ok := ownership["rental"].(map[string]any); ok {
		return firstPurchaseDateMillis(rental)
	}
	return 0
}

func firstPurchaseDateMillis(object map[string]any) int64 {
	accepted := map[string]bool{
		"purchased": true, "purchasedate": true, "purchasedatemillis": true, "acquisitiondate": true,
		"acquireddate": true, "dateacquired": true, "ownedsince": true,
	}
	for key, value := range object {
		normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
		if !accepted[normalized] {
			continue
		}
		if parsed, ok := purchaseDateMillis(value); ok {
			return parsed
		}
	}
	return 0
}

func purchaseDateMillis(value any) (int64, bool) {
	var number int64
	switch typed := value.(type) {
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false
		}
		number = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		number = parsed
	case int64:
		number = typed
	case int:
		number = int64(typed)
	case string:
		text := strings.TrimSpace(typed)
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			number = parsed
		} else if parsedTime, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsedTime.UnixMilli(), true
		} else {
			return 0, false
		}
	default:
		return 0, false
	}
	if number > 0 && number < 100000000000 {
		number *= 1000
	} else if number >= 100000000000000000 {
		number /= 1000000
	} else if number >= 100000000000000 {
		number /= 1000
	}
	return number, number > 0
}

func extractRPPrice(value any) (int, bool) {
	type candidate struct {
		priority int
		value    int
	}
	var candidates []candidate
	add := func(priority int, raw any) {
		number, ok := numericValue(raw)
		price := int(number)
		if ok && number == float64(price) && price > 0 && price < 100000 {
			candidates = append(candidates, candidate{priority: priority, value: price})
		}
	}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			for key, raw := range typed {
				normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
				switch normalized {
				case "rp", "rpcost", "priceinrp", "rpprice":
					add(0, raw)
				}
			}
			currency := strings.ToUpper(firstString(typed, "currency", "currencyType", "type"))
			if currency == "RP" || currency == "RIOT_POINTS" || currency == "RIOTPOINTS" {
				for _, key := range []string{"basePrice", "price", "cost", "amount"} {
					if raw, ok := typed[key]; ok {
						add(1, raw)
						break
					}
				}
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	if len(candidates) == 0 {
		return 0, false
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].priority < candidates[j].priority })
	bestPriority := candidates[0].priority
	bestValue := candidates[0].value
	for _, item := range candidates[1:] {
		if item.priority != bestPriority {
			break
		}
		if item.value != bestValue {
			return 0, false
		}
	}
	return bestValue, true
}

func skinBorderContentIDs(value any, skinID int64) (bool, map[string]bool) {
	contentIDs := map[string]bool{}
	hasBorder := false
	var inspectBorder func(any)
	inspectBorder = func(current any) {
		switch typed := current.(type) {
		case []any:
			for _, child := range typed {
				inspectBorder(child)
			}
		case map[string]any:
			if strings.TrimSpace(firstString(typed, "borderPath")) != "" {
				hasBorder = true
				if id := normalizedContentID(typed["contentId"]); id != "" {
					contentIDs[id] = true
				}
			}
			for _, child := range typed {
				inspectBorder(child)
			}
		}
	}
	for _, object := range catalogObjects(value) {
		if firstInt(object, "id", "skinId") != skinID {
			continue
		}
		if augments, ok := object["skinAugments"]; ok {
			inspectBorder(augments)
		}
		break
	}
	return hasBorder, contentIDs
}

func extractInventoryContentIDs(value any) map[string]bool {
	result := map[string]bool{}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			if quantity, ok := numericValue(typed["quantity"]); !ok || quantity > 0 {
				for _, key := range []string{"contentId", "itemId", "id"} {
					if id := normalizedContentID(typed[key]); id != "" {
						result[id] = true
					}
				}
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func normalizedContentID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(typed))
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	case json.Number:
		return typed.String()
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	}
	return ""
}
