package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const collectionRetryNegativeTTL = time.Hour

var collectionRetryCacheMu sync.Mutex

const collectionRetryCacheFile = "collection-retry-negative.json"

// Only public catalog content contributes to this revision, never ownership.
func collectionCatalogRevision(skins []Skin, metadata map[string]lootMetadata) string {
	type skinIdentity struct {
		ID   int64
		Name string
	}
	rows := make([]skinIdentity, 0, len(skins))
	for _, skin := range skins {
		rows = append(rows, skinIdentity{skin.ID, skin.Name})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	data, _ := json.Marshal(struct {
		Skins    []skinIdentity
		Metadata map[string]lootMetadata
	}{rows, metadata})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func collectionBlankSample(item LootItem) map[string]any {
	kind := strings.TrimSpace(item.Category)
	if kind == "" {
		kind = "类型未知"
	}
	id := item.StoreItemID
	if id == 0 {
		id = item.SkinID
	}
	if id == 0 {
		parts := strings.FieldsFunc(item.LootID, func(r rune) bool { return r < '0' || r > '9' })
		if len(parts) > 0 {
			id, _ = strconv.ParseInt(parts[len(parts)-1], 10, 64)
		}
	}
	// Zero means the source supplied no numeric ID, not an inferred identity.
	return map[string]any{"kind": kind, "id": id}
}
func collectionBlankCacheKey(item LootItem) string {
	fields := append([]string(nil), item.rawFieldKeys...)
	sort.Strings(fields)
	data, _ := json.Marshal([]any{item.catalogRevision, item.LootID, item.LootName, item.Type, collectionBlankSample(item), fields})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func collectionBlankDiagnostics(account AccountData) (map[string]int, []map[string]any, string) {
	kinds := map[string]int{}
	samples := []map[string]any{}
	lastError := "none"
	for _, item := range account.Loot {
		if item.Blank && item.DataPending {
			sample := collectionBlankSample(item)
			kinds[sample["kind"].(string)]++
			if len(samples) < 3 {
				samples = append(samples, sample)
			}
			lastError = "empty_identity"
		}
	}
	for _, c := range account.Capabilities {
		if c.Name == "player-loot" && c.State == capabilityFailed {
			lastError = "source_failed"
		}
	}
	return kinds, samples, lastError
}
func (s *localStore) collectionRetryCache(now time.Time) map[string]time.Time {
	entries := map[string]time.Time{}
	if s != nil {
		if data, err := readLocalStoreFile(s, collectionRetryCacheFile); err == nil {
			_ = json.Unmarshal(data, &entries)
		}
	}
	if entries == nil {
		entries = map[string]time.Time{}
	}
	for key, until := range entries {
		if !now.Before(until) || until.Sub(now) > collectionRetryNegativeTTL {
			delete(entries, key)
		}
	}
	return entries
}
func (s *localStore) cacheExhaustedCollectionBlanks(account AccountData, now time.Time) error {
	if s == nil {
		return nil
	}
	collectionRetryCacheMu.Lock()
	defer collectionRetryCacheMu.Unlock()
	entries := s.collectionRetryCache(now)
	for _, item := range account.Loot {
		if item.Blank && item.DataPending {
			entries[collectionBlankCacheKey(item)] = now.Add(collectionRetryNegativeTTL)
		}
	}
	// Bound a cache of public metadata failures; never store inventory quantities.
	if len(entries) > 512 {
		keys := make([]string, 0, len(entries))
		for key := range entries {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return entries[keys[i]].Before(entries[keys[j]]) })
		for _, key := range keys[:len(keys)-512] {
			delete(entries, key)
		}
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return writeLocalStoreFile(s, collectionRetryCacheFile, data)
}
func (s *localStore) suppressCachedCollectionBlanks(account AccountData, now time.Time) (AccountData, int) {
	if s == nil {
		return account, 0
	}
	collectionRetryCacheMu.Lock()
	entries := s.collectionRetryCache(now)
	collectionRetryCacheMu.Unlock()
	loot := append([]LootItem(nil), account.Loot...)
	count := 0
	for i, item := range loot {
		if item.Blank && item.DataPending && now.Before(entries[collectionBlankCacheKey(item)]) {
			loot[i].DataPending = false
			count++
		}
	}
	if count > 0 {
		account.Loot = loot
	}
	return account, count
}
