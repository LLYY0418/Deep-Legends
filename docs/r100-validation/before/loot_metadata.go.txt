package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

type lootMetadataCatalog struct {
	name       string
	clientPath string
	publicPath string
	parse      func([]byte) (map[string]lootMetadata, error)
}

// These are separate client catalogs: loot.json does not contain ward/icon
// names, and some legacy chests/materials only have loot_name_* translations.
var lootMetadataCatalogs = []lootMetadataCatalog{
	{"loot", "/lol-game-data/assets/v1/loot.json", "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/loot.json", parseLootCatalog},
	{"wards", "/lol-game-data/assets/v1/ward-skins.json", "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/ward-skins.json", parseLootWardCatalog},
	{"icons", "/lol-game-data/assets/v1/summoner-icons.json", "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/summoner-icons.json", parseLootIconCatalog},
	{"translations", "/fe/lol-loot/trans.json", "/latest/plugins/rcp-fe-lol-loot/global/zh_cn/trans.json", parseLootTranslations},
}

// All catalogs share the caller's deadline and load concurrently. Prefer the
// connected client's locale/version; the public fallback uses the existing
// disk cache. Only fixed, read-only catalog URLs leave this function.
func loadLootMetadata(ctx context.Context, client *LCUClient, provider *championProvider, observe func(map[string]any)) map[string]lootMetadata {
	type result struct {
		entries map[string]lootMetadata
		source  string
	}
	results := make([]result, len(lootMetadataCatalogs))
	var group sync.WaitGroup
	for index, catalog := range lootMetadataCatalogs {
		group.Add(1)
		go func(index int, catalog lootMetadataCatalog) {
			defer group.Done()
			results[index].source = "unavailable"
			if client != nil {
				localCtx, cancel := context.WithTimeout(ctx, time.Second)
				data, err := client.GetBytesContext(localCtx, catalog.clientPath)
				cancel()
				if err == nil {
					if entries, err := catalog.parse(data); err == nil {
						results[index] = result{entries, "client"}
						return
					}
				}
			}
			if provider != nil {
				data, err := provider.fetch(ctx, communityDragonHost, catalog.publicPath, nil, championCacheMaxEntry, "application/json")
				if err == nil {
					if entries, err := catalog.parse(data); err == nil {
						results[index] = result{entries, "communitydragon"}
					}
				}
			}
		}(index, catalog)
	}
	group.Wait()
	metadata := make(map[string]lootMetadata)
	for index, result := range results {
		// Game-data entries take precedence; legacy translations fill gaps.
		for id, entry := range result.entries {
			current := metadata[id]
			if current.Name == "" {
				current.Name = entry.Name
			}
			if current.Description == "" {
				current.Description = entry.Description
			}
			if current.Image == "" {
				current.Image = entry.Image
			}
			metadata[id] = current
		}
		if observe != nil {
			// Catalog sizes are public metadata, never player IDs or quantities.
			observe(map[string]any{"event": "loot_metadata_source", "catalog": lootMetadataCatalogs[index].name,
				"source": result.source, "entries": len(result.entries)})
		}
	}
	return metadata
}

func parseLootWardCatalog(data []byte) (map[string]lootMetadata, error) {
	var wards []struct {
		ID            *int64 `json:"id"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		WardImagePath string `json:"wardImagePath"`
	}
	if err := json.Unmarshal(data, &wards); err != nil {
		return nil, err
	}
	result := make(map[string]lootMetadata)
	for _, ward := range wards {
		if ward.ID == nil || *ward.ID < 0 || strings.TrimSpace(ward.Name) == "" {
			continue
		}
		entry := lootMetadata{strings.TrimSpace(ward.Name), strings.TrimSpace(ward.Description), sanitizeClientImagePath(ward.WardImagePath)}
		for _, prefix := range []string{"WARD_SKIN_", "WARD_SKIN_RENTAL_", "WARDSKIN_", "WARDSKIN_RENTAL_"} {
			result[prefix+strconv.FormatInt(*ward.ID, 10)] = entry
		}
	}
	return requireLootMetadataEntries(result)
}

func parseLootIconCatalog(data []byte) (map[string]lootMetadata, error) {
	var icons []struct {
		ID        *int64 `json:"id"`
		Title     string `json:"title"`
		ImagePath string `json:"imagePath"`
	}
	if err := json.Unmarshal(data, &icons); err != nil {
		return nil, err
	}
	result := make(map[string]lootMetadata)
	for _, icon := range icons {
		if icon.ID == nil || *icon.ID < 0 || strings.TrimSpace(icon.Title) == "" {
			continue
		}
		entry := lootMetadata{Name: strings.TrimSpace(icon.Title), Image: sanitizeClientImagePath(icon.ImagePath)}
		for _, prefix := range []string{"SUMMONER_ICON_", "SUMMONERICON_"} {
			result[prefix+strconv.FormatInt(*icon.ID, 10)] = entry
		}
	}
	return requireLootMetadataEntries(result)
}

func parseLootTranslations(data []byte) (map[string]lootMetadata, error) {
	var translations map[string]string
	if err := json.Unmarshal(data, &translations); err != nil {
		return nil, err
	}
	result := make(map[string]lootMetadata)
	for key, value := range translations {
		token := normalizeLootToken(key)
		if !strings.HasPrefix(token, "LOOT_NAME_") {
			continue
		}
		id := strings.TrimPrefix(token, "LOOT_NAME_")
		name := strings.TrimSpace(value)
		if id != "" && !lootNameMissing(LootItem{LootID: id}, name) && !strings.ContainsAny(name, "{}") {
			result[id] = lootMetadata{Name: name}
		}
	}
	for key, value := range translations {
		id := strings.TrimPrefix(normalizeLootToken(key), "LOOT_DESCRIPTION_")
		if entry, ok := result[id]; ok {
			entry.Description = strings.TrimSpace(value)
			result[id] = entry
		}
	}
	return requireLootMetadataEntries(result)
}

func requireLootMetadataEntries(entries map[string]lootMetadata) (map[string]lootMetadata, error) {
	if len(entries) == 0 {
		return nil, errors.New("loot metadata catalog has no usable entries")
	}
	return entries, nil
}
