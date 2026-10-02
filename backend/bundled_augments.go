package main

import (
	"embed"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// The dated CommunityDragon snapshot keeps augment ID, Chinese name and artwork
// together. It is only metadata: win rates and grades still come from their
// live statistical sources. Unknown IDs remain unknown instead of being guessed.
//
//go:embed data/augment_catalog_20260924.json data/augment_icons_20260924.bin data/arena_items_20260924.json
var bundledArenaData embed.FS

type bundledIconSpan struct {
	ID     int64 `json:"id"`
	Offset int   `json:"offset"`
	Length int   `json:"length"`
}

type bundledAugmentState struct {
	rows  []gameplayAugment
	icons map[int64]bundledIconSpan
	data  []byte
}

var readBundledAugments = sync.OnceValue(func() bundledAugmentState {
	var snapshot struct {
		Augments []gameplayAugment `json:"augments"`
		Icons    []bundledIconSpan `json:"icons"`
	}
	data, err := bundledArenaData.ReadFile("data/augment_catalog_20260924.json")
	if err != nil || json.Unmarshal(data, &snapshot) != nil {
		return bundledAugmentState{}
	}
	data, err = bundledArenaData.ReadFile("data/augment_icons_20260924.bin")
	if err != nil {
		return bundledAugmentState{}
	}
	icons := make(map[int64]bundledIconSpan, len(snapshot.Icons))
	for _, icon := range snapshot.Icons {
		if icon.ID > 0 && icon.Offset >= 0 && icon.Length > 0 && icon.Length <= 512000 && icon.Offset <= len(data)-icon.Length && strings.HasPrefix(http.DetectContentType(data[icon.Offset:icon.Offset+icon.Length]), "image/png") {
			icons[icon.ID] = icon
		}
	}
	rows := make([]gameplayAugment, 0, len(snapshot.Augments))
	for _, row := range snapshot.Augments {
		if row.ID <= 0 || strings.TrimSpace(row.Name) == "" {
			continue
		}
		if _, ok := icons[row.ID]; ok {
			row.IconPath = "builtin:" + bundledAugmentPath(row.ID)
		} else {
			row.IconPath = ""
		}
		rows = append(rows, row)
	}
	return bundledAugmentState{rows: rows, icons: icons, data: data}
})

func bundledAugmentPath(id int64) string {
	return "/augments/" + strconv.FormatInt(id, 10) + ".png"
}

func bundledAugmentIconPath(id int64) string {
	if _, ok := readBundledAugments().icons[id]; ok {
		return bundledAugmentPath(id)
	}
	return ""
}

func bundledAugmentCatalog() []gameplayAugment {
	return append([]gameplayAugment(nil), readBundledAugments().rows...)
}

func preferBundledAugmentIcons(rows []gameplayAugment) []gameplayAugment {
	result := append([]gameplayAugment(nil), rows...)
	for index := range result {
		if path := bundledAugmentIconPath(result[index].ID); path != "" {
			result[index].IconPath = "builtin:" + path
			result[index].FallbackIconPath = ""
		}
	}
	return result
}

func (p *championProvider) arenaAugmentCatalogFast() []gameplayAugment {
	catalog := bundledAugmentCatalog()
	p.remoteAugmentMu.RLock()
	catalog = mergeGameplayAugmentMetadata(catalog, p.remoteAugments)
	p.remoteAugmentMu.RUnlock()
	return preferBundledAugmentIcons(catalog)
}

func bundledAugmentImage(path string) ([]byte, bool) {
	id, ok := bundledAugmentID(path)
	if !ok {
		return nil, false
	}
	state := readBundledAugments()
	span, ok := state.icons[id]
	if !ok {
		return nil, false
	}
	return state.data[span.Offset : span.Offset+span.Length], true
}

func bundledAugmentID(path string) (int64, bool) {
	const prefix = "/augments/"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, ".png") {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(path, prefix), ".png"), 10, 64)
	if err != nil || id <= 0 || path != bundledAugmentPath(id) {
		return 0, false
	}
	_, ok := readBundledAugments().icons[id]
	return id, ok
}

var readBundledArenaItems = sync.OnceValue(func() []gameplayItem {
	var snapshot struct {
		Items []communityDragonItemRaw `json:"items"`
	}
	data, err := bundledArenaData.ReadFile("data/arena_items_20260924.json")
	if err != nil || json.Unmarshal(data, &snapshot) != nil {
		return nil
	}
	return normalizeCommunityDragonItems(snapshot.Items)
})

func bundledArenaItems() []gameplayItem {
	return append([]gameplayItem(nil), readBundledArenaItems()...)
}
