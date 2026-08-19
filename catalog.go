package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Skin struct {
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	ChampionID            int64  `json:"championId,omitempty"`
	ChampionName          string `json:"championName,omitempty"`
	Rarity                string `json:"rarity,omitempty"`
	RarityTier            string `json:"rarityTier,omitempty"`
	RaritySubtier         string `json:"raritySubtier,omitempty"`
	RegionRarityID        int64  `json:"regionRarityId,omitempty"`
	IsLegacy              bool   `json:"isLegacy,omitempty"`
	IsVariant             bool   `json:"isVariant,omitempty"`
	ParentSkinID          int64  `json:"parentSkinId,omitempty"`
	Description           string `json:"description,omitempty"`
	SplashPath            string `json:"splashPath,omitempty"`
	TilePath              string `json:"tilePath,omitempty"`
	LoadScreenPath        string `json:"loadScreenPath,omitempty"`
	SplashVideoPath       string `json:"splashVideoPath,omitempty"`
	CollectionVideoPath   string `json:"collectionVideoPath,omitempty"`
	CardHoverVideoPath    string `json:"cardHoverVideoPath,omitempty"`
	Owned                 bool   `json:"owned"`
	PoolName              string `json:"poolName,omitempty"`
	AcquiredAt            string `json:"acquiredAt,omitempty"`
	ChampionMasteryPoints int64  `json:"championMasteryPoints,omitempty"`
	ChampionMasteryLevel  int64  `json:"championMasteryLevel,omitempty"`
}

type Chroma struct {
	ID                    int64    `json:"id"`
	Name                  string   `json:"name"`
	ParentSkinID          int64    `json:"parentSkinId,omitempty"`
	ParentSkinName        string   `json:"parentSkinName,omitempty"`
	ChampionID            int64    `json:"championId,omitempty"`
	ChampionName          string   `json:"championName,omitempty"`
	Rarity                string   `json:"rarity,omitempty"`
	RarityTier            string   `json:"rarityTier,omitempty"`
	RaritySubtier         string   `json:"raritySubtier,omitempty"`
	Description           string   `json:"description,omitempty"`
	SplashPath            string   `json:"splashPath,omitempty"`
	ParentSplashPath      string   `json:"parentSplashPath,omitempty"`
	TilePath              string   `json:"tilePath,omitempty"`
	ChromaPath            string   `json:"chromaPath,omitempty"`
	Colors                []string `json:"colors,omitempty"`
	IsPrestige            bool     `json:"isPrestige,omitempty"`
	PrestigeImageID       string   `json:"prestigeImageId,omitempty"`
	Owned                 bool     `json:"owned"`
	AcquiredAt            string   `json:"acquiredAt,omitempty"`
	ChampionMasteryPoints int64    `json:"championMasteryPoints,omitempty"`
	ChampionMasteryLevel  int64    `json:"championMasteryLevel,omitempty"`
}

type PoolIssue struct {
	Name       string   `json:"name"`
	Reason     string   `json:"reason"`
	Candidates []string `json:"candidates,omitempty"`
}

type Snapshot struct {
	Summoner    Summoner
	All         []Skin
	Owned       []Skin
	Remaining   []Skin
	PoolTotal   int
	PoolMatched int
	Issues      []PoolIssue
	Client      *LCUClient
	Ownership   []OwnershipSourceStatus
	Catalog     CatalogStats
	Account     AccountData
	Chromas     []Chroma
	ChromaState EndpointCapability
}

type OwnershipSourceStatus struct {
	Path               string  `json:"path"`
	State              string  `json:"state"`
	Count              int     `json:"count"`
	RawOwnedCount      int     `json:"rawOwnedCount,omitempty"`
	EvidenceCount      int     `json:"evidenceCount,omitempty"`
	BaseCount          int     `json:"baseCount,omitempty"`
	VariantCount       int     `json:"variantCount,omitempty"`
	UnknownCount       int     `json:"unknownCount,omitempty"`
	RentalCount        int     `json:"rentalCount,omitempty"`
	FreeToPlayCount    int     `json:"freeToPlayCount,omitempty"`
	BaseIDs            []int64 `json:"baseIds,omitempty"`
	VariantIDs         []int64 `json:"variantIds,omitempty"`
	UnknownIDs         []int64 `json:"unknownIds,omitempty"`
	RentalIDs          []int64 `json:"rentalIds,omitempty"`
	FreeToPlayIDs      []int64 `json:"freeToPlayIds,omitempty"`
	CatalogOwnedIDHash string  `json:"catalogOwnedIdHash,omitempty"`
	Detail             string  `json:"detail,omitempty"`
}

type ownershipSourceSpec struct {
	path               string
	presenceMeansOwned bool
	authoritative      bool
}

type ownershipResult struct {
	source      ownershipSourceSpec
	ids         map[int64]bool
	statusIndex int
}

type CatalogStats struct {
	SkinCount     int    `json:"skinCount"`
	ChampionCount int    `json:"championCount"`
	BaseSkinCount int    `json:"baseSkinCount"`
	Fingerprint   string `json:"fingerprint"`
}

func loadSnapshot(pool PoolManifest) (Snapshot, error) {
	client, err := discoverLCU()
	if err != nil {
		return Snapshot{}, err
	}
	return loadSnapshotWithClient(client, pool)
}

func loadSnapshotWithClient(client *LCUClient, pool PoolManifest) (Snapshot, error) {
	summoner, err := NewSummonerAPI(client).Current()
	if err != nil {
		return Snapshot{Client: client}, err
	}

	all, err := NewSkinCatalogAPI(client).Load()
	if err != nil {
		return Snapshot{}, err
	}
	chromas, chromaCatalogErr := loadChromaCatalog(client, all)
	ownedIDs, ownershipSources, ownershipErr := NewInventoryAPI(client).OwnedSkinIDs(summoner.SummonerID, all)
	if ownershipErr != nil {
		return Snapshot{Summoner: summoner, All: all, Client: client, Ownership: ownershipSources, Catalog: buildCatalogStats(all)}, ownershipErr
	}
	ownedChampionIDs, championCapability := NewInventoryAPI(client).OwnedChampionIDs(summoner.SummonerID)
	applySkinOwnership(all, ownedIDs, ownedChampionIDs)
	acquiredAt, acquisitionCapability := NewInventoryAPI(client).SkinAcquisitionDates(summoner.SummonerID, ownedIDs)
	masteries, masteryCapability := NewChampionMasteryAPI(client).All(summoner.PUUID)
	for i := range all {
		if all[i].Owned {
			all[i].AcquiredAt = acquiredAt[all[i].ID]
		}
		if mastery, ok := masteries[all[i].ChampionID]; ok {
			all[i].ChampionMasteryPoints = mastery.ChampionPoints
			all[i].ChampionMasteryLevel = mastery.ChampionLevel
		}
	}
	chromaState := EndpointCapability{Name: "owned-chromas", Path: "本机炫彩目录与库存"}
	if chromaCatalogErr != nil {
		chromaState.State = capabilityFailed
		chromaState.Detail = "客户端炫彩目录不可用；普通皮肤和三合一结果不受影响"
		chromas = nil
	} else {
		chromaOwnedIDs, ownershipCapability := loadOwnedChromaIDs(client, summoner.SummonerID, chromas)
		chromaState = ownershipCapability
		chromaDates, _ := NewInventoryAPI(client).SkinAcquisitionDates(summoner.SummonerID, chromaOwnedIDs)
		for i := range chromas {
			chromas[i].Owned = chromaOwnedIDs[chromas[i].ID]
			if chromas[i].Owned {
				chromas[i].AcquiredAt = chromaDates[chromas[i].ID]
			}
			if mastery, ok := masteries[chromas[i].ChampionID]; ok {
				chromas[i].ChampionMasteryPoints = mastery.ChampionPoints
				chromas[i].ChampionMasteryLevel = mastery.ChampionLevel
			}
		}
		sortChromas(chromas)
	}

	if len(pool.Names) == 0 {
		return Snapshot{}, errors.New("pool manifest has no entries")
	}
	matched, issues := matchPoolManifest(pool, all)
	annotatePoolMembership(all, matched)
	owned := ownedCollectionSkins(all)
	var remaining []Skin
	for _, skin := range matched {
		if !ownedIDs[skin.ID] {
			remaining = append(remaining, skin)
		}
	}
	sortSkins(owned)
	sortSkins(remaining)
	var displayAll []Skin
	for _, skin := range all {
		if !isBaseSkin(skin) {
			displayAll = append(displayAll, skin)
		}
	}
	sortSkins(displayAll)

	profile, profileCapability := NewSummonerAPI(client).Profile()
	for _, skin := range all {
		if skin.ID == profile.BackgroundSkinID {
			profile.BackgroundSkinName = skin.Name
			break
		}
	}
	loot, lootCapability := NewLootAPI(client).PlayerLoot()
	loot = enrichLootItems(loot, all)
	sanctumSparks, sanctumCapability := NewLootAPI(client).SanctumSparks()
	rewards, rewardsCapability := NewRewardsAPI(client).PendingGrants()

	return Snapshot{
		Summoner:    summoner,
		All:         displayAll,
		Owned:       owned,
		Remaining:   remaining,
		PoolTotal:   len(pool.Names),
		PoolMatched: len(matched),
		Issues:      issues,
		Client:      client,
		Ownership:   ownershipSources,
		Catalog:     buildCatalogStats(all),
		Chromas:     chromas,
		ChromaState: chromaState,
		Account: AccountData{
			Profile: profile, Loot: loot, Rewards: rewards, SanctumSparks: sanctumSparks, SanctumSparksKnown: sanctumCapability.State == capabilityAvailable,
			Capabilities: []EndpointCapability{profileCapability, lootCapability, sanctumCapability, rewardsCapability, championCapability, acquisitionCapability, masteryCapability, chromaState},
		},
	}, nil
}

// applySkinOwnership deliberately requires evidence for each non-base skin ID.
// Quest/tier variants are independent inventory objects and must never inherit
// ownership merely because their parent skin is owned.
func applySkinOwnership(skins []Skin, ownedSkinIDs, ownedChampionIDs map[int64]bool) {
	for i := range skins {
		skins[i].Owned = ownedSkinIDs[skins[i].ID]
		if isBaseSkin(skins[i]) {
			skins[i].Owned = ownedChampionIDs[skins[i].ChampionID]
		}
	}
}

func ownedCollectionSkins(catalog []Skin) []Skin {
	owned := make([]Skin, 0, len(catalog))
	for _, skin := range catalog {
		if skin.Owned && !isBaseSkin(skin) && !skin.IsVariant {
			owned = append(owned, skin)
		}
	}
	return owned
}

func annotatePoolMembership(all, matched []Skin) {
	poolNames := make(map[int64]string, len(matched))
	for _, skin := range matched {
		poolNames[skin.ID] = skin.PoolName
	}
	for i := range all {
		all[i].PoolName = poolNames[all[i].ID]
	}
}

func loadSkinCatalog(client *LCUClient) ([]Skin, error) {
	paths := []string{"/lol-game-data/assets/v1/skins.json", "/lol-game-data/v1/skins.json"}
	var data []byte
	var err error
	for _, path := range paths {
		data, err = client.GetBytes(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("skin catalog unavailable: %w", err)
	}
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("skin catalog decode: %w", err)
	}
	objects := catalogObjects(root)
	byID := map[int64]Skin{}
	for _, object := range objects {
		id := firstInt(object, "id", "skinId", "itemId")
		name := firstString(object, "name", "displayName")
		if id <= 0 || name == "" {
			continue
		}
		if isChromaCatalogObject(object) {
			continue
		}
		championID := firstInt(object, "championId", "championID")
		if championID == 0 {
			championID = id / 1000
		}
		if !validCatalogSkinIdentity(id, championID) {
			continue
		}
		skin := Skin{
			ID:                  id,
			Name:                strings.TrimSpace(name),
			ChampionID:          championID,
			ChampionName:        strings.TrimSpace(firstString(object, "championName", "championDisplayName")),
			Rarity:              firstString(object, "rarity", "rarityGemPath"),
			RegionRarityID:      firstInt(object, "regionRarityId", "regionRarityID"),
			IsLegacy:            hasTrueFlag(object, "isLegacy"),
			Description:         firstString(object, "description"),
			SplashPath:          sanitizeAssetPath(firstString(object, "uncenteredSplashPath", "splashPath")),
			TilePath:            sanitizeAssetPath(firstString(object, "tilePath")),
			LoadScreenPath:      sanitizeAssetPath(firstString(object, "loadScreenPath")),
			SplashVideoPath:     sanitizeAssetPath(firstString(object, "splashVideoPath", "previewVideoUrl")),
			CollectionVideoPath: sanitizeAssetPath(firstString(object, "collectionSplashVideoPath")),
			CardHoverVideoPath:  sanitizeAssetPath(firstString(object, "collectionCardHoverVideoPath")),
		}
		skin.RarityTier, skin.RaritySubtier = classifySkinRarity(object, skin)
		if skin.ChampionName == "" {
			skin.ChampionName = lastWord(skin.Name)
		}
		if existing, ok := byID[id]; ok {
			if normalizeName(existing.Name) != normalizeName(skin.Name) || existing.ChampionID != skin.ChampionID {
				return nil, fmt.Errorf("skin catalog has conflicting records for ID %d: %q and %q", id, existing.Name, skin.Name)
			}
			byID[id] = mergeSkin(existing, skin)
		} else {
			byID[id] = skin
		}
		for _, variant := range questSkinVariants(object, skin) {
			if existing, ok := byID[variant.ID]; ok {
				if normalizeName(existing.Name) != normalizeName(variant.Name) {
					return nil, fmt.Errorf("skin catalog has conflicting variant records for ID %d: %q and %q", variant.ID, existing.Name, variant.Name)
				}
				byID[variant.ID] = mergeSkin(existing, variant)
			} else {
				byID[variant.ID] = variant
			}
		}
	}
	champions := map[int64]bool{}
	bases := map[int64]bool{}
	for _, skin := range byID {
		champions[skin.ChampionID] = true
		if skin.ID == skin.ChampionID*1000 {
			bases[skin.ChampionID] = true
		}
	}
	if len(byID) < 1000 || len(champions) < 100 {
		return nil, fmt.Errorf("skin catalog is incomplete: %d skins across %d champions", len(byID), len(champions))
	}
	if len(bases) != len(champions) {
		return nil, fmt.Errorf("skin catalog is incomplete: %d of %d champions have a base skin", len(bases), len(champions))
	}
	out := make([]Skin, 0, len(byID))
	for _, skin := range byID {
		out = append(out, skin)
	}
	return out, nil
}

// loadChromaCatalog intentionally keeps chromas outside the ordinary Skin catalog.
// Chroma IDs must never participate in the 554-item reroll pool calculation.
func loadChromaCatalog(client *LCUClient, skins []Skin) ([]Chroma, error) {
	paths := []string{"/lol-game-data/assets/v1/skins.json", "/lol-game-data/v1/skins.json"}
	var data []byte
	var err error
	for _, endpoint := range paths {
		data, err = client.GetBytes(endpoint)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("chroma catalog unavailable: %w", err)
	}
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("chroma catalog decode: %w", err)
	}
	objects := catalogObjects(root)
	skinByID := make(map[int64]Skin, len(skins))
	for _, skin := range skins {
		skinByID[skin.ID] = skin
	}
	contentToSkin := map[string]Skin{}
	for _, object := range objects {
		if skin, ok := skinByID[firstInt(object, "id", "skinId", "itemId")]; ok {
			if contentID := strings.TrimSpace(firstString(object, "contentId", "contentID")); contentID != "" {
				contentToSkin[strings.ToLower(contentID)] = skin
			}
		}
	}
	byID := map[int64]Chroma{}
	for _, object := range objects {
		if entry, ok := independentArtworkChroma(object, skinByID, contentToSkin); ok {
			entry = enrichPrestigeChroma(entry)
			if existing, exists := byID[entry.ID]; exists && (existing.ParentSkinID != entry.ParentSkinID || normalizeName(existing.Name) != normalizeName(entry.Name)) {
				return nil, fmt.Errorf("chroma catalog has conflicting records for ID %d", entry.ID)
			}
			byID[entry.ID] = entry
		}
		objectID := firstInt(object, "id", "skinId", "itemId")
		parent, hasParent := skinByID[objectID]
		rawChromas, _ := object["chromas"].([]any)
		for _, raw := range rawChromas {
			chromaObject, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			resolvedParent := parent
			if related := strings.ToLower(strings.TrimSpace(firstString(chromaObject, "relatedPrimeContentId", "relatedPrimeContentID"))); related != "" {
				if relatedSkin, ok := contentToSkin[related]; ok {
					resolvedParent = relatedSkin
				}
			}
			if !hasParent && resolvedParent.ID == 0 {
				continue
			}
			id := firstInt(chromaObject, "id", "chromaId", "itemId")
			name := strings.TrimSpace(firstString(chromaObject, "name", "displayName"))
			if id <= 0 || name == "" || resolvedParent.ChampionID <= 0 {
				continue
			}
			entry := Chroma{
				ID: id, Name: name, ParentSkinID: resolvedParent.ID, ParentSkinName: resolvedParent.Name,
				ChampionID: resolvedParent.ChampionID, ChampionName: resolvedParent.ChampionName,
				Rarity: resolvedParent.Rarity, RarityTier: resolvedParent.RarityTier, RaritySubtier: resolvedParent.RaritySubtier,
				Description:      firstString(chromaObject, "description"),
				SplashPath:       sanitizeAssetPath(firstString(chromaObject, "uncenteredSplashPath", "splashPath", "collectionSplashPath")),
				ParentSplashPath: resolvedParent.SplashPath,
				TilePath:         sanitizeAssetPath(firstString(chromaObject, "tilePath", "chromaPath")),
				ChromaPath:       sanitizeAssetPath(firstString(chromaObject, "chromaPath", "tilePath")),
				Colors:           stringValues(chromaObject["colors"]),
			}
			entry = enrichPrestigeChroma(entry)
			if existing, exists := byID[id]; exists && (existing.ParentSkinID != entry.ParentSkinID || normalizeName(existing.Name) != normalizeName(entry.Name)) {
				return nil, fmt.Errorf("chroma catalog has conflicting records for ID %d", id)
			}
			byID[id] = entry
		}
	}
	if len(byID) < 1000 {
		return nil, fmt.Errorf("chroma catalog is incomplete: %d entries", len(byID))
	}
	out := make([]Chroma, 0, len(byID))
	for _, chroma := range byID {
		out = append(out, chroma)
	}
	sortChromas(out)
	return out, nil
}

// independentArtworkChroma accepts only an explicitly linked chroma record that
// also carries its own splash art. Ordinary nested chromas usually have appearance
// tiles instead of splash art; Tencent project_jade classic artwork has splash art but no chroma
// identity or parent relationship, so neither can satisfy both requirements.
func independentArtworkChroma(object map[string]any, skinByID map[int64]Skin, contentToSkin map[string]Skin) (Chroma, bool) {
	classification := strings.ToLower(strings.TrimSpace(firstString(object, "skinClassification", "classification", "type")))
	parentID := firstInt(object, "parentSkinId", "parentSkinID", "baseSkinId", "baseSkinID")
	explicitChroma := hasTrueFlag(object, "isChroma", "chroma") || strings.Contains(classification, "chroma") || strings.Contains(classification, "recolor") || parentID > 0
	if !explicitChroma {
		return Chroma{}, false
	}
	splashPath := sanitizeAssetPath(firstString(object, "uncenteredSplashPath", "splashPath", "collectionSplashPath"))
	if splashPath == "" {
		return Chroma{}, false
	}
	parent, ok := skinByID[parentID]
	if !ok {
		related := strings.ToLower(strings.TrimSpace(firstString(object, "relatedPrimeContentId", "relatedPrimeContentID")))
		parent, ok = contentToSkin[related]
	}
	id := firstInt(object, "id", "chromaId", "itemId")
	name := strings.TrimSpace(firstString(object, "name", "displayName"))
	if !ok || parent.ID <= 0 || parent.ChampionID <= 0 || id <= 0 || id == parent.ID || name == "" {
		return Chroma{}, false
	}
	return Chroma{
		ID: id, Name: name, ParentSkinID: parent.ID, ParentSkinName: parent.Name,
		ChampionID: parent.ChampionID, ChampionName: parent.ChampionName,
		Rarity: parent.Rarity, RarityTier: parent.RarityTier, RaritySubtier: parent.RaritySubtier,
		Description: firstString(object, "description"), SplashPath: splashPath, ParentSplashPath: parent.SplashPath,
		TilePath:   sanitizeAssetPath(firstString(object, "tilePath", "chromaPath")),
		ChromaPath: sanitizeAssetPath(firstString(object, "chromaPath", "tilePath")),
		Colors:     stringValues(object["colors"]), IsPrestige: true,
	}, true
}

func stringValues(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result
}

func loadOwnedChromaIDs(client *LCUClient, summonerID int64, catalog []Chroma) (map[int64]bool, EndpointCapability) {
	valid := make(map[int64]bool, len(catalog))
	for _, chroma := range catalog {
		valid[chroma.ID] = true
	}
	type source struct {
		path     string
		presence bool
	}
	sources := []source{
		{path: fmt.Sprintf("/lol-champions/v1/inventories/%d/skins-minimal", summonerID)},
		{path: fmt.Sprintf("/lol-champions/v1/inventories/%d/champions", summonerID)},
		{path: "/lol-inventory/v2/inventory/CHAMPION_SKIN", presence: true},
		{path: "/lol-inventory/v2/inventory/CHROMA", presence: true},
		{path: "/lol-inventory/v1/inventory?inventoryTypes=CHROMA", presence: true},
	}
	owned := map[int64]bool{}
	supported := 0
	invalid := 0
	for _, candidate := range sources {
		data, err := client.GetBytes(candidate.path)
		if err != nil {
			continue
		}
		var root any
		if err := json.Unmarshal(data, &root); err != nil {
			invalid++
			continue
		}
		supported++
		var ids map[int64]bool
		if candidate.presence {
			ids = extractChromaPresenceIDs(root)
		} else {
			ids = extractOwnedEvidence(root, false).OwnedIDs
		}
		for id := range ids {
			if valid[id] {
				owned[id] = true
			}
		}
	}
	capability := EndpointCapability{Name: "owned-chromas", Path: "5 个本机炫彩库存候选", Count: len(owned)}
	if supported == 0 {
		capability.State = capabilityUnsupported
		capability.Detail = "当前国服客户端未提供可核验的炫彩库存端点"
		return owned, capability
	}
	capability.State = capabilityAvailable
	capability.Detail = fmt.Sprintf("已从 %d/5 个本机端点核对炫彩所有权", supported)
	if invalid > 0 {
		capability.Detail += fmt.Sprintf("；%d 个端点数据格式无效", invalid)
	}
	return owned, capability
}

func extractChromaPresenceIDs(value any) map[int64]bool {
	result := map[int64]bool{}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			id, inventoryType := inventorySkinIdentity(typed)
			if id == 0 {
				id = firstInt(typed, "id", "chromaId")
			}
			allowedType := inventoryType == "CHROMA" || inventoryType == "CHAMPION_SKIN" || inventoryType == ""
			quantityOK := true
			if quantity, ok := numericValue(typed["quantity"]); ok {
				quantityOK = quantity > 0
			}
			explicitOwned, evidence, conflict := ownershipDecision(typed)
			if id > 0 && allowedType && quantityOK && !freeToPlayOwnership(typed) && (!evidence || (explicitOwned && !conflict)) {
				result[id] = true
			}
			for key, child := range typed {
				normalizedKey := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
				switch normalizedKey {
				case "items", "inventory", "inventoryitems", "chromas", "ownedchromas", "championskins", "skins", "data":
					walk(child)
				}
			}
		}
	}
	walk(value)
	return result
}

func sortChromas(chromas []Chroma) {
	sort.Slice(chromas, func(i, j int) bool {
		if chromas[i].IsPrestige != chromas[j].IsPrestige {
			return chromas[i].IsPrestige
		}
		if chromas[i].ChampionID != chromas[j].ChampionID {
			return chromas[i].ChampionID < chromas[j].ChampionID
		}
		if chromas[i].ParentSkinID != chromas[j].ParentSkinID {
			return chromas[i].ParentSkinID < chromas[j].ParentSkinID
		}
		return chromas[i].ID < chromas[j].ID
	})
}

func classifySkinRarity(object map[string]any, skin Skin) (string, string) {
	tierByRegion := map[int64]string{
		10: "圣堂", 11: "卓越", 8: "神话", 9: "终极", 5: "传说",
		7: "限定", 4: "史诗", 3: "王者", 2: "勇士", 1: "典藏",
	}
	tier := tierByRegion[skin.RegionRarityID]
	if tier == "" {
		switch strings.ToLower(strings.TrimSpace(skin.Rarity)) {
		case "kexalted", "exalted":
			tier = "圣堂"
		case "ktranscendent", "transcendent":
			tier = "卓越"
		case "kmythic", "mythic":
			tier = "神话"
		case "kultimate", "ultimate":
			tier = "终极"
		case "klegendary", "legendary":
			tier = "传说"
		case "kepic", "epic":
			tier = "史诗"
		default:
			tier = "未分级"
		}
	}
	if tier != "神话" {
		return tier, ""
	}
	search := strings.ToLower(skin.Name + " " + strings.Join(catalogEmblemNames(object), " "))
	switch {
	case strings.Contains(search, "prestige") || strings.Contains(skin.Name, "至臻"):
		return tier, "至臻"
	case strings.Contains(search, "hextech") || strings.Contains(skin.Name, "海克斯科技"):
		return tier, "海克斯系列"
	case strings.Contains(skin.Name, "MVP T1"):
		return tier, "总决赛FMVP系列"
	case strings.Contains(skin.Name, "殿堂传奇") || strings.Contains(search, "hol"):
		return tier, "殿堂系列"
	case strings.Contains(skin.Name, " T1 ") || strings.HasPrefix(skin.Name, "T1 ") || strings.Contains(skin.Name, "MVP") || strings.Contains(skin.Name, "战队"):
		return tier, "战队系列"
	case strings.Contains(skin.Name, "水晶"):
		return tier, "水晶系列"
	case strings.Contains(skin.Name, "灰烬"):
		return tier, "灰烬系列"
	case strings.Contains(skin.Name, "周年"):
		return tier, "周年系列"
	case strings.Contains(search, "mythic"):
		return tier, "神话幻想"
	default:
		return tier, "强行神话"
	}
}

func questSkinVariants(object map[string]any, parent Skin) []Skin {
	quest, ok := object["questSkinInfo"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := quest["tiers"].([]any)
	if !ok {
		return nil
	}
	variants := make([]Skin, 0, len(raw))
	for _, value := range raw {
		tier, ok := value.(map[string]any)
		if !ok {
			continue
		}
		id := firstInt(tier, "id")
		name := strings.TrimSpace(firstString(tier, "name", "shortName"))
		if id <= 0 || id == parent.ID || name == "" {
			continue
		}
		variant := parent
		variant.ID = id
		variant.Name = name
		variant.IsVariant = true
		variant.ParentSkinID = parent.ID
		variant.Description = firstString(tier, "description")
		variant.SplashPath = sanitizeAssetPath(firstString(tier, "uncenteredSplashPath", "splashPath"))
		variant.TilePath = sanitizeAssetPath(firstString(tier, "tilePath"))
		variant.LoadScreenPath = sanitizeAssetPath(firstString(tier, "loadScreenPath"))
		variant.SplashVideoPath = sanitizeAssetPath(firstString(tier, "splashVideoPath"))
		variant.CollectionVideoPath = sanitizeAssetPath(firstString(tier, "collectionSplashVideoPath"))
		variant.CardHoverVideoPath = sanitizeAssetPath(firstString(tier, "collectionCardHoverVideoPath"))
		if rarity := strings.TrimSpace(firstString(tier, "rarity")); rarity != "" {
			variant.Rarity = rarity
		}
		if regionRarityID := firstInt(tier, "regionRarityId", "regionRarityID"); regionRarityID > 0 {
			variant.RegionRarityID = regionRarityID
		}
		variant.RarityTier, variant.RaritySubtier = classifySkinRarity(tier, variant)
		variants = append(variants, variant)
	}
	return variants
}

func catalogEmblemNames(object map[string]any) []string {
	raw, ok := object["emblems"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, value := range raw {
		if emblem, ok := value.(map[string]any); ok {
			if name := strings.TrimSpace(firstString(emblem, "name")); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

func loadOwnedSkinIDs(client *LCUClient, summonerID int64, catalog []Skin) (map[int64]bool, []string) {
	ids, statuses, err := loadOwnedSkinInventory(client, summonerID, catalog)
	var details []string
	for _, status := range statuses {
		if status.State != "success" && status.State != "unsupported" {
			details = append(details, status.Path+": "+status.Detail)
		}
	}
	if err != nil {
		details = append(details, err.Error())
		return map[int64]bool{}, details
	}
	return ids, details
}

func loadOwnedSkinInventory(client *LCUClient, summonerID int64, catalog []Skin) (map[int64]bool, []OwnershipSourceStatus, error) {
	sources := []ownershipSourceSpec{
		{path: fmt.Sprintf("/lol-champions/v1/inventories/%d/skins-minimal", summonerID), authoritative: true},
		{path: fmt.Sprintf("/lol-champions/v1/inventories/%d/champions", summonerID), authoritative: true},
		{path: "/lol-inventory/v2/inventory/CHAMPION_SKIN", presenceMeansOwned: true},
		{path: "/lol-inventory/v1/inventory?inventoryTypes=CHAMPION_SKIN", presenceMeansOwned: true},
	}
	validCatalogIDs := make(map[int64]bool, len(catalog))
	baseCatalogIDs := make(map[int64]bool)
	variantCatalogIDs := make(map[int64]bool)
	for _, skin := range catalog {
		if isBaseSkin(skin) {
			baseCatalogIDs[skin.ID] = true
			continue
		}
		validCatalogIDs[skin.ID] = true
		if skin.IsVariant {
			variantCatalogIDs[skin.ID] = true
		}
	}
	var authoritative []ownershipResult
	var presence []ownershipResult
	var statuses []OwnershipSourceStatus
	for _, source := range sources {
		statusPath := strings.ReplaceAll(source.path, strconv.FormatInt(summonerID, 10), "{summonerId}")
		data, err := client.GetBytes(source.path)
		if err != nil {
			var httpErr *LCUHTTPError
			if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
				statuses = append(statuses, OwnershipSourceStatus{Path: statusPath, State: "unsupported", Detail: "endpoint unavailable in this client"})
			} else {
				statuses = append(statuses, OwnershipSourceStatus{Path: statusPath, State: "failed", Detail: "request failed"})
			}
			continue
		}
		var root any
		if err := json.Unmarshal(data, &root); err != nil {
			statuses = append(statuses, OwnershipSourceStatus{Path: statusPath, State: "invalid", Detail: "invalid JSON"})
			continue
		}
		extraction := extractOwnedEvidence(root, source.presenceMeansOwned)
		validated := map[int64]bool{}
		baseOwned := map[int64]bool{}
		variantOwned := map[int64]bool{}
		unknownOwned := map[int64]bool{}
		for id := range extraction.OwnedIDs {
			if validCatalogIDs[id] {
				validated[id] = true
				if variantCatalogIDs[id] {
					variantOwned[id] = true
				}
			} else if baseCatalogIDs[id] {
				baseOwned[id] = true
			} else if id > 0 {
				unknownOwned[id] = true
			}
		}
		validatedEvidence := 0
		for id := range extraction.DecidedIDs {
			if validCatalogIDs[id] {
				validatedEvidence++
			}
		}
		if source.authoritative {
			minimumEvidence := len(validCatalogIDs) * 80 / 100
			if minimumEvidence < 1 {
				minimumEvidence = 1
			}
			if validatedEvidence < minimumEvidence {
				statuses = append(statuses, ownershipSourceStatus(statusPath, "invalid", validated, extraction, baseOwned, variantOwned, unknownOwned, fmt.Sprintf("explicit ownership coverage is incomplete: %d/%d", validatedEvidence, len(validCatalogIDs)), validatedEvidence))
				continue
			}
		}
		statusIndex := len(statuses)
		status := ownershipSourceStatus(statusPath, "success", validated, extraction, baseOwned, variantOwned, unknownOwned, "", validatedEvidence)
		if len(unknownOwned) > 0 {
			status.State = "warning"
			status.Detail = fmt.Sprintf("已忽略 %d 个目录外 ID（炫彩或库存附属项）；完整 ID 已写入本地诊断", len(unknownOwned))
		}
		statuses = append(statuses, status)
		result := ownershipResult{source: source, ids: validated, statusIndex: statusIndex}
		if source.authoritative {
			authoritative = append(authoritative, result)
		} else {
			presence = append(presence, result)
		}
	}
	if len(authoritative) > 0 {
		if inventory, ok := corroboratingPresenceSuperset(authoritative, presence); ok {
			for _, candidate := range authoritative {
				if !equalIDSets(inventory.ids, candidate.ids) {
					missing, extra := idSetDifferenceCounts(inventory.ids, candidate.ids)
					markOwnershipSource(&statuses[candidate.statusIndex], "warning", fmt.Sprintf("explicit source is fully contained by verified inventory: inventory adds %d, explicit-only %d", missing, extra))
				}
			}
			if statuses[inventory.statusIndex].Detail == "" {
				statuses[inventory.statusIndex].Detail = "已用完整 CHAMPION_SKIN 库存补齐显式收藏接口遗漏"
			}
			return inventory.ids, statuses, nil
		}
		baselineIndex, support, tied := bestCorroboratedAuthoritative(authoritative, presence)
		if tied {
			return map[int64]bool{}, statuses, errors.New("ownership evidence contains equally supported conflicting explicit sources")
		}
		baseline := authoritative[baselineIndex]
		hasExplicitConflict := false
		for _, candidate := range authoritative {
			if !equalIDSets(baseline.ids, candidate.ids) {
				hasExplicitConflict = true
			}
		}
		if hasExplicitConflict && support == 0 {
			for _, candidate := range authoritative {
				if !equalIDSets(baseline.ids, candidate.ids) {
					missing, extra := idSetDifferenceCounts(baseline.ids, candidate.ids)
					markOwnershipSource(&statuses[candidate.statusIndex], "conflict", fmt.Sprintf("differs from the primary explicit source: missing %d, extra %d", missing, extra))
				}
			}
			return map[int64]bool{}, statuses, errors.New("authoritative ownership sources disagree without a corroborating source")
		}
		for _, candidate := range authoritative {
			if !equalIDSets(baseline.ids, candidate.ids) {
				missing, extra := idSetDifferenceCounts(baseline.ids, candidate.ids)
				markOwnershipSource(&statuses[candidate.statusIndex], "warning", fmt.Sprintf("explicit audit differs: missing %d, extra %d; corroborated source retained", missing, extra))
			}
		}
		for _, candidate := range presence {
			if !equalIDSets(baseline.ids, candidate.ids) {
				missing, extra := idSetDifferenceCounts(baseline.ids, candidate.ids)
				markOwnershipSource(&statuses[candidate.statusIndex], "warning", fmt.Sprintf("presence-only audit differs: missing %d, extra %d; explicit full-coverage source retained", missing, extra))
			}
		}
		return baseline.ids, statuses, nil
	}
	if len(presence) < 2 {
		return map[int64]bool{}, statuses, fmt.Errorf("owned skin inventory needs an explicit full-coverage source or two agreeing presence sources; got %d presence sources", len(presence))
	}
	baseline := presence[0]
	for _, candidate := range presence[1:] {
		if !equalIDSets(baseline.ids, candidate.ids) {
			missing, extra := idSetDifferenceCounts(baseline.ids, candidate.ids)
			statuses[candidate.statusIndex].State = "conflict"
			statuses[candidate.statusIndex].Detail = fmt.Sprintf("presence sources differ: missing %d, extra %d", missing, extra)
			return map[int64]bool{}, statuses, fmt.Errorf("presence ownership sources disagree: %s=%d, %s=%d", baseline.source.path, len(baseline.ids), candidate.source.path, len(candidate.ids))
		}
	}
	return baseline.ids, statuses, nil
}

func corroboratingPresenceSuperset(authoritative, presence []ownershipResult) (ownershipResult, bool) {
	if len(authoritative) < 2 {
		return ownershipResult{}, false
	}
	var best ownershipResult
	found := false
	for _, inventory := range presence {
		containsAll := true
		for _, source := range authoritative {
			for id := range source.ids {
				if !inventory.ids[id] {
					containsAll = false
					break
				}
			}
			if !containsAll {
				break
			}
		}
		if containsAll && (!found || len(inventory.ids) < len(best.ids)) {
			best, found = inventory, true
		}
	}
	return best, found
}

func bestCorroboratedAuthoritative(authoritative, presence []ownershipResult) (int, int, bool) {
	bestIndex := 0
	bestSupport := -1
	tied := false
	for index, candidate := range authoritative {
		support := 0
		for _, audit := range presence {
			if equalIDSets(candidate.ids, audit.ids) {
				support++
			}
		}
		if support > bestSupport {
			bestIndex, bestSupport, tied = index, support, false
		} else if support == bestSupport && !equalIDSets(candidate.ids, authoritative[bestIndex].ids) {
			tied = support > 0
		}
	}
	return bestIndex, bestSupport, tied
}

func markOwnershipSource(status *OwnershipSourceStatus, state, detail string) {
	status.State = state
	if status.Detail == "" {
		status.Detail = detail
	} else {
		status.Detail += "; " + detail
	}
}

func extractOwnedIDs(value any, presenceMeansOwned bool) map[int64]bool {
	return extractOwnedEvidence(value, presenceMeansOwned).OwnedIDs
}

type ownedEvidence struct {
	OwnedIDs      map[int64]bool
	DecidedIDs    map[int64]bool
	RentalIDs     map[int64]bool
	FreeToPlayIDs map[int64]bool
}

func extractOwnedEvidence(value any, presenceMeansOwned bool) ownedEvidence {
	result := ownedEvidence{OwnedIDs: map[int64]bool{}, DecidedIDs: map[int64]bool{}, RentalIDs: map[int64]bool{}, FreeToPlayIDs: map[int64]bool{}}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			owned, hasEvidence, conflict := ownershipDecision(typed)
			id, inventoryType := inventorySkinIdentity(typed)
			if !presenceMeansOwned && id == 0 {
				id = firstInt(typed, "id")
			}
			if presenceMeansOwned {
				owned = (inventoryType == "" || inventoryType == "CHAMPION_SKIN") && id > 0 && !hasTrueFlag(typed, "freeToPlay", "f2p", "isFreeToPlay")
				hasEvidence = owned
				if quantity, ok := numericValue(typed["quantity"]); ok && quantity <= 0 {
					owned = false
				}
				if explicitOwned, explicit, explicitConflict := ownershipDecision(typed); explicit && (!explicitOwned || explicitConflict) {
					owned = false
					hasEvidence = true
				}
			}
			if hasEvidence && !conflict && id > 0 {
				result.DecidedIDs[id] = true
				if owned {
					result.OwnedIDs[id] = true
				}
			}
			if id > 0 && rentalOwnership(typed) {
				result.RentalIDs[id] = true
			}
			if id > 0 && freeToPlayOwnership(typed) {
				result.FreeToPlayIDs[id] = true
			}
			for key, child := range typed {
				switch strings.ToLower(key) {
				case "skins", "skin", "chromas", "championskins", "items", "entries", "inventory", "inventoryitems", "champions", "ownedskins", "data", "payload", "records", "results", "content":
					walk(child)
				}
			}
		}
	}
	walk(value)
	return result
}

func ownershipSourceStatus(path, state string, validated map[int64]bool, extraction ownedEvidence, baseOwned, variantOwned, unknownOwned map[int64]bool, detail string, evidenceCount int) OwnershipSourceStatus {
	return OwnershipSourceStatus{
		Path: path, State: state, Count: len(validated), RawOwnedCount: len(extraction.OwnedIDs), EvidenceCount: evidenceCount,
		BaseCount: len(baseOwned), VariantCount: len(variantOwned), UnknownCount: len(unknownOwned),
		RentalCount: len(extraction.RentalIDs), FreeToPlayCount: len(extraction.FreeToPlayIDs),
		BaseIDs: sortedIDKeys(baseOwned), VariantIDs: sortedIDKeys(variantOwned), UnknownIDs: sortedIDKeys(unknownOwned),
		RentalIDs: sortedIDKeys(extraction.RentalIDs), FreeToPlayIDs: sortedIDKeys(extraction.FreeToPlayIDs),
		CatalogOwnedIDHash: idSetHash(validated), Detail: detail,
	}
}

func sortedIDKeys(ids map[int64]bool) []int64 {
	result := make([]int64, 0, len(ids))
	for id := range ids {
		if id > 0 {
			result = append(result, id)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func idSetHash(ids map[int64]bool) string {
	hash := sha256.New()
	for _, id := range sortedIDKeys(ids) {
		_, _ = fmt.Fprintf(hash, "%d,", id)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func rentalOwnership(object map[string]any) bool {
	if rental, ok := object["rental"].(map[string]any); ok && hasTrueFlag(rental, "rented") {
		return true
	}
	if ownership, ok := object["ownership"].(map[string]any); ok {
		if rental, ok := ownership["rental"].(map[string]any); ok && hasTrueFlag(rental, "rented") {
			return true
		}
	}
	return false
}

func freeToPlayOwnership(object map[string]any) bool {
	if hasTrueFlag(object, "freeToPlay", "f2p", "isFreeToPlay", "freeToPlayReward") {
		return true
	}
	if ownership, ok := object["ownership"].(map[string]any); ok {
		return hasTrueFlag(ownership, "freeToPlay", "f2p", "isFreeToPlay", "freeToPlayReward")
	}
	return false
}

func inventorySkinIdentity(object map[string]any) (int64, string) {
	inventoryType := strings.ToUpper(firstString(object, "inventoryType", "type"))
	id := firstInt(object, "skinId", "itemId")
	if key, ok := object["itemKey"].(map[string]any); ok {
		if id == 0 {
			id = firstInt(key, "skinId", "itemId", "id")
		}
		if inventoryType == "" {
			inventoryType = strings.ToUpper(firstString(key, "inventoryType", "type"))
		}
	}
	return id, inventoryType
}

func idSetDifferenceCounts(reference, candidate map[int64]bool) (missing, extra int) {
	for id := range reference {
		if !candidate[id] {
			missing++
		}
	}
	for id := range candidate {
		if !reference[id] {
			extra++
		}
	}
	return missing, extra
}

func catalogObjects(root any) []map[string]any {
	var out []map[string]any
	appendDirect := func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, child := range typed {
				if object, ok := child.(map[string]any); ok {
					out = append(out, object)
				}
			}
		case map[string]any:
			out = append(out, typed)
		}
	}
	switch typed := root.(type) {
	case []any:
		appendDirect(typed)
	case map[string]any:
		if skins, ok := typed["skins"]; ok {
			appendDirect(skins)
			return out
		}
		for _, child := range typed {
			appendDirect(child)
		}
	}
	return out
}

func mergeSkin(existing, candidate Skin) Skin {
	if existing.ChampionName == "" {
		existing.ChampionName = candidate.ChampionName
	}
	if existing.Rarity == "" {
		existing.Rarity = candidate.Rarity
	}
	if existing.RarityTier == "" || existing.RarityTier == "未分级" {
		existing.RarityTier = candidate.RarityTier
	}
	if existing.RaritySubtier == "" {
		existing.RaritySubtier = candidate.RaritySubtier
	}
	if existing.RegionRarityID == 0 {
		existing.RegionRarityID = candidate.RegionRarityID
	}
	existing.IsLegacy = existing.IsLegacy || candidate.IsLegacy
	if existing.Description == "" {
		existing.Description = candidate.Description
	}
	if existing.SplashPath == "" {
		existing.SplashPath = candidate.SplashPath
	}
	if existing.TilePath == "" {
		existing.TilePath = candidate.TilePath
	}
	if existing.LoadScreenPath == "" {
		existing.LoadScreenPath = candidate.LoadScreenPath
	}
	if existing.SplashVideoPath == "" {
		existing.SplashVideoPath = candidate.SplashVideoPath
	}
	if existing.CollectionVideoPath == "" {
		existing.CollectionVideoPath = candidate.CollectionVideoPath
	}
	if existing.CardHoverVideoPath == "" {
		existing.CardHoverVideoPath = candidate.CardHoverVideoPath
	}
	if !existing.IsVariant && candidate.IsVariant {
		existing.IsVariant = true
		existing.ParentSkinID = candidate.ParentSkinID
	}
	return existing
}

func equalIDSets(left, right map[int64]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for id := range left {
		if !right[id] {
			return false
		}
	}
	return true
}

func ownershipDecision(object map[string]any) (owned bool, hasEvidence bool, conflict bool) {
	var signals []bool
	if boolean, ok := object["owned"].(bool); ok {
		signals = append(signals, boolean)
	}
	if status := strings.ToUpper(firstString(object, "ownershipStatus", "status")); status != "" {
		switch status {
		case "OWNED", "PURCHASED":
			signals = append(signals, true)
		case "AVAILABLE", "LOCKED", "NOT_OWNED", "UNOWNED", "RENTED", "F2P":
			signals = append(signals, false)
		}
	}
	if ownership, ok := object["ownership"].(map[string]any); ok {
		if boolean, ok := ownership["owned"].(bool); ok {
			signals = append(signals, boolean)
		}
	}
	if hasTrueFlag(object, "freeToPlay", "f2p", "isFreeToPlay") {
		signals = append(signals, false)
	}
	if len(signals) == 0 {
		return false, false, false
	}
	decision := signals[0]
	for _, signal := range signals[1:] {
		if signal != decision {
			return false, true, true
		}
	}
	return decision, true, false
}

func hasTrueFlag(object map[string]any, keys ...string) bool {
	for _, key := range keys {
		if boolean, ok := object[key].(bool); ok && boolean {
			return true
		}
	}
	return false
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		number, err := strconv.ParseFloat(typed, 64)
		return number, err == nil
	default:
		return 0, false
	}
}

func matchPool(poolNames []string, skins []Skin) ([]Skin, []PoolIssue) {
	exact := map[string][]Skin{}
	for _, skin := range skins {
		if isBaseSkin(skin) {
			continue
		}
		key := normalizeName(skin.Name)
		exact[key] = append(exact[key], skin)
	}
	matchedByID := map[int64]Skin{}
	var issues []PoolIssue
	for _, sourceName := range poolNames {
		canonical := canonicalPoolName(sourceName)
		key := normalizeName(canonical)
		candidates := exact[key]
		if len(candidates) != 1 {
			issue := PoolIssue{Name: sourceName, Reason: "未唯一匹配到客户端皮肤 ID"}
			suggestions := candidates
			if len(suggestions) == 0 {
				suggestions = fuzzyCandidates(canonical, skins)
			}
			for _, candidate := range suggestions {
				issue.Candidates = append(issue.Candidates, candidate.Name)
			}
			issues = append(issues, issue)
			continue
		}
		skin := candidates[0]
		if existing, duplicate := matchedByID[skin.ID]; duplicate {
			issues = append(issues, PoolIssue{
				Name:       sourceName,
				Reason:     "与另一奖池名称映射到同一皮肤 ID",
				Candidates: []string{existing.PoolName, skin.Name},
			})
			continue
		}
		skin.PoolName = sourceName
		matchedByID[skin.ID] = skin
	}
	matched := make([]Skin, 0, len(matchedByID))
	for _, skin := range matchedByID {
		matched = append(matched, skin)
	}
	sortSkins(matched)
	sort.Slice(issues, func(i, j int) bool { return issues[i].Name < issues[j].Name })
	return matched, issues
}

func matchPoolManifest(pool PoolManifest, skins []Skin) ([]Skin, []PoolIssue) {
	if len(pool.Entries) == 0 {
		return matchPool(pool.Names, skins)
	}
	byID := make(map[int64]Skin, len(skins))
	for _, skin := range skins {
		if !isBaseSkin(skin) && validCatalogSkinIdentity(skin.ID, skin.ChampionID) {
			byID[skin.ID] = skin
		}
	}
	matched := make([]Skin, 0, len(pool.Entries))
	issues := make([]PoolIssue, 0)
	for _, entry := range pool.Entries {
		skin, ok := byID[entry.ID]
		if !ok {
			issues = append(issues, PoolIssue{Name: entry.Name, Reason: fmt.Sprintf("客户端普通皮肤目录中不存在稳定 ID %d", entry.ID)})
			continue
		}
		skin.PoolName = entry.Name
		matched = append(matched, skin)
	}
	sortSkins(matched)
	sort.Slice(issues, func(i, j int) bool { return issues[i].Name < issues[j].Name })
	return matched, issues
}

func fuzzyCandidates(name string, skins []Skin) []Skin {
	target := []rune(normalizeName(name))
	if len(target) == 0 {
		return nil
	}
	champion := normalizeName(lastWord(name))
	threshold := 2
	if len(target) >= 12 {
		threshold = 3
	}
	best := threshold + 1
	var candidates []Skin
	for _, skin := range skins {
		if isBaseSkin(skin) {
			continue
		}
		candidateName := normalizeName(skin.Name)
		if champion != "" && !strings.HasSuffix(candidateName, champion) {
			continue
		}
		distance := levenshtein(target, []rune(candidateName))
		if distance < best && distance <= threshold {
			best = distance
			candidates = []Skin{skin}
		} else if distance == best && distance <= threshold {
			candidates = append(candidates, skin)
		}
	}
	return candidates
}

func canonicalPoolName(name string) string {
	return strings.TrimSpace(strings.TrimRight(name, "?？"))
}

func normalizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.In(r, unicode.Han) {
			return r
		}
		return -1
	}, value)
}

func lastWord(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, ra := range a {
		current[0] = i + 1
		for j, rb := range b {
			cost := 0
			if ra != rb {
				cost = 1
			}
			current[j+1] = minInt(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			switch typed := value.(type) {
			case string:
				return typed
			case json.Number:
				return typed.String()
			}
		}
	}
	return ""
}

func firstInt(object map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := object[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int64(typed)
		case json.Number:
			parsed, _ := typed.Int64()
			return parsed
		case string:
			parsed, _ := strconv.ParseInt(typed, 10, 64)
			return parsed
		case int64:
			return typed
		case int:
			return int64(typed)
		}
	}
	return 0
}

func sanitizeAssetPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" {
		return ""
	}
	index := strings.Index(strings.ToLower(path), "/lol-game-data/assets/")
	if index >= 0 {
		return path[index:]
	}
	return ""
}

func isBaseSkin(skin Skin) bool {
	return skin.ChampionID > 0 && skin.ID == skin.ChampionID*1000
}

func isChromaCatalogObject(object map[string]any) bool {
	if hasTrueFlag(object, "isChroma", "chroma") {
		return true
	}
	classification := strings.ToLower(strings.TrimSpace(firstString(object, "skinClassification", "classification", "type")))
	if strings.Contains(classification, "chroma") {
		return true
	}
	return firstInt(object, "parentSkinId", "parentSkinID", "baseSkinId", "baseSkinID") > 0
}

func validCatalogSkinIdentity(skinID, championID int64) bool {
	return skinID > 0 && skinID < 1_000_000 && championID > 0 && championID < 1000 && skinID/1000 == championID
}

func sortSkins(skins []Skin) {
	sort.Slice(skins, func(i, j int) bool {
		if skins[i].ChampionID != skins[j].ChampionID {
			return skins[i].ChampionID < skins[j].ChampionID
		}
		return skins[i].ID < skins[j].ID
	})
}

func buildCatalogStats(skins []Skin) CatalogStats {
	champions := map[int64]bool{}
	baseCount := 0
	ordered := append([]Skin(nil), skins...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	hash := sha256.New()
	for _, skin := range ordered {
		if skin.IsVariant {
			continue
		}
		champions[skin.ChampionID] = true
		if isBaseSkin(skin) {
			baseCount++
		}
		_, _ = fmt.Fprintf(hash, "%d\x00%d\x00%s\n", skin.ID, skin.ChampionID, normalizeName(skin.Name))
	}
	return CatalogStats{
		SkinCount: len(skins), ChampionCount: len(champions), BaseSkinCount: baseCount,
		Fingerprint: fmt.Sprintf("%x", hash.Sum(nil)),
	}
}
