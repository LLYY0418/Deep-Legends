package main

import (
	"embed"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Data Dragon 16.19.1 Chinese names cover the 173 IDs current at this release.
// They remove a cold catalog network wait from the first overview. A known
// different patch still uses the live catalog to avoid stale new-champion names.
//
//go:embed data/champion_names_16.19.1_zh_cn.json data/champion_icons_16.19.1.bin
var bundledChampionNamesData embed.FS

type bundledChampionState struct {
	names map[int64]string
	icons map[int64]bundledIconSpan
	data  []byte
}

var readBundledChampionNames = sync.OnceValue(func() bundledChampionState {
	data, err := bundledChampionNamesData.ReadFile("data/champion_names_16.19.1_zh_cn.json")
	if err != nil {
		return bundledChampionState{}
	}
	var snapshot struct {
		Names map[string]string `json:"names"`
		Icons []bundledIconSpan `json:"icons"`
	}
	if json.Unmarshal(data, &snapshot) != nil {
		return bundledChampionState{}
	}
	names := make(map[int64]string, len(snapshot.Names))
	for rawID, name := range snapshot.Names {
		if id, err := strconv.ParseInt(rawID, 10, 64); err == nil && id > 0 && name != "" {
			names[id] = name
		}
	}
	data, err = bundledChampionNamesData.ReadFile("data/champion_icons_16.19.1.bin")
	if err != nil {
		return bundledChampionState{names: names}
	}
	icons := make(map[int64]bundledIconSpan, len(snapshot.Icons))
	for _, icon := range snapshot.Icons {
		if icon.ID > 0 && icon.Offset >= 0 && icon.Length > 0 && icon.Length <= 180000 && icon.Offset <= len(data)-icon.Length && strings.HasPrefix(http.DetectContentType(data[icon.Offset:icon.Offset+icon.Length]), "image/png") {
			icons[icon.ID] = icon
		}
	}
	return bundledChampionState{names: names, icons: icons, data: data}
})

func bundledChampionNames() map[int64]string {
	state := readBundledChampionNames()
	result := make(map[int64]string, len(state.names))
	for id, name := range state.names {
		result[id] = name
	}
	return result
}

func bundledChampionIcon(path string) ([]byte, bool) {
	const prefix = "/lol-game-data/assets/v1/champion-icons/"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, ".png") {
		return nil, false
	}
	id, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(path, prefix), ".png"), 10, 64)
	if err != nil || id <= 0 || path != prefix+strconv.FormatInt(id, 10)+".png" {
		return nil, false
	}
	state := readBundledChampionNames()
	span, ok := state.icons[id]
	if !ok {
		return nil, false
	}
	return state.data[span.Offset : span.Offset+span.Length], true
}

func (p *championProvider) bundledChampionVersionCompatible() bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	patch := p.patch
	p.mu.Unlock()
	return patch == "" || strings.HasPrefix(patch, "16.19.")
}
