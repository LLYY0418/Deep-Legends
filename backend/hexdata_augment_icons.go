package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

const observedHexdataAugmentsKey = "normalized-augments-v1|hexdata-observed"

var hexdataAugmentIconPattern = regexp.MustCompile(`^/assets/augments/icons/[0-9]+\.[0-9]+/[A-Za-z0-9_-]+\.png$`)

// Only Hexdata's observed augment icon directory may be fetched through the
// browser-facing proxy. No absolute URL, query, traversal or other asset type.
func hexdataAugmentIconPath(value string) string {
	value = strings.TrimSpace(value)
	if hexdataAugmentIconPattern.MatchString(value) {
		return value
	}
	return ""
}

func augmentMetadataImage(iconPath string) (string, string) {
	if strings.HasPrefix(iconPath, "builtin:") {
		path := strings.TrimPrefix(iconPath, "builtin:")
		if _, ok := bundledAugmentID(path); ok {
			return "builtin", path
		}
		return "", ""
	}
	if strings.HasPrefix(iconPath, "hexdata:") {
		if path := hexdataAugmentIconPath(strings.TrimPrefix(iconPath, "hexdata:")); path != "" {
			return "hexdata", path
		}
		return "", ""
	}
	if path := communityDragonGameAssetPath(iconPath); path != "" {
		return "communitydragon", path
	}
	return "", ""
}

func (p *championProvider) loadObservedHexdataAugmentsLocked() {
	if p.observedLoaded {
		return
	}
	p.observedLoaded = true
	p.observedAugments = make(map[int64]gameplayAugment)
	if p.cache == nil {
		return
	}
	entry, err := p.cache.readDisk(observedHexdataAugmentsKey)
	if err != nil || !time.Now().Before(entry.StaleUntil) {
		return
	}
	var saved []gameplayAugment
	if json.Unmarshal(entry.Data, &saved) != nil {
		return
	}
	for _, item := range saved {
		if item.ID > 0 && strings.HasPrefix(item.IconPath, "hexdata:") && hexdataAugmentIconPath(strings.TrimPrefix(item.IconPath, "hexdata:")) != "" {
			p.observedAugments[item.ID] = item
		}
	}
}

func (p *championProvider) observedHexdataAugments() []gameplayAugment {
	p.observedAugmentMu.Lock()
	defer p.observedAugmentMu.Unlock()
	p.loadObservedHexdataAugmentsLocked()
	return p.observedHexdataAugmentsLocked()
}

func (p *championProvider) observedHexdataAugmentsLocked() []gameplayAugment {
	ids := make([]int64, 0, len(p.observedAugments))
	for id := range p.observedAugments {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	items := make([]gameplayAugment, 0, len(ids))
	for _, id := range ids {
		items = append(items, p.observedAugments[id])
	}
	return items
}

// Hero JSON is the only observed source; the public Hexdata bulk endpoint does
// not provide data. Persist an ID index without adding a network request.
func (p *championProvider) observeHexdataAugments(rows []hexdataAugmentRowV2) {
	p.observedAugmentMu.Lock()
	p.loadObservedHexdataAugmentsLocked()
	changed := false
	for _, row := range rows {
		path := hexdataAugmentIconPath(row.AugmentIconURL)
		if row.AugmentID <= 0 || path == "" {
			continue
		}
		rarity := hexdataRarityLabel(row.Rarity)
		if rarity == "unknown" {
			rarity = ""
		}
		item := gameplayAugment{
			ID: int64(row.AugmentID), Name: strings.TrimSpace(row.AugmentName),
			Description: strings.TrimSpace(row.AugmentDescription), Rarity: rarity,
			IconPath: "hexdata:" + path,
		}
		if p.observedAugments[item.ID] != item {
			p.observedAugments[item.ID] = item
			changed = true
		}
	}
	if changed && p.cache != nil {
		p.observedWriteDirty = true
		if !p.observedWriteActive {
			p.observedWriteActive = true
			goSafe("hexdata_augment_icons.observeHexdataAugments.1", func() { p.persistObservedHexdataAugments() })
		}
	}
	p.observedAugmentMu.Unlock()
}

func (p *championProvider) persistObservedHexdataAugments() {
	for {
		p.observedAugmentMu.Lock()
		if !p.observedWriteDirty {
			p.observedWriteActive = false
			p.observedAugmentMu.Unlock()
			return
		}
		p.observedWriteDirty = false
		items := p.observedHexdataAugmentsLocked()
		p.observedAugmentMu.Unlock()
		data, err := json.Marshal(items)
		if err != nil || len(data) == 0 {
			continue
		}
		now := time.Now()
		sum := sha256.Sum256(data)
		entry := championCacheEnvelope{
			Schema: championCacheSchema, Key: observedHexdataAugmentsKey,
			FetchedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour), StaleUntil: now.Add(90 * 24 * time.Hour),
			Hash: hex.EncodeToString(sum[:]), Data: data,
		}
		if p.cache.dir != "" {
			_ = p.cache.writeDisk(entry)
		}
	}
}
