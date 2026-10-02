package main

import (
	"slices"
	"strings"
	"testing"
)

func TestR153AllGameDataSkinNameIsKnownShapeKey(t *testing.T) {
	shape := liveClientAllGameDataShape([]byte(`{"allPlayers":[{"skinName":"星之守护者","rawSkinName":"Skin_Internal"}]}`))
	arrays := shape["arrays"].(map[string]any)
	players := arrays["$.allPlayers"].([]map[string]any)
	keys := players[0]["element_keys"].([]string)
	if !slices.Contains(keys, "skinName") || slices.Contains(keys, "<unknown-key>") {
		t.Fatalf("skinName was masked by the shape allowlist: %v", keys)
	}

	// R116 §1.5 的斗魂玩家 19 键，逐项核对安全字段表。
	for _, key := range strings.Fields("championName isBot isDead items level position rawChampionName rawSkinName respawnTimer riotId riotIdGameName riotIdTagLine runes scores skinID skinName summonerName summonerSpells team") {
		if got := arenaShapeKey(key); got != key {
			t.Errorf("known playerlist field %s became %s", key, got)
		}
	}
	if got := arenaShapeKey("privateDynamicField"); got != "<unknown-key>" {
		t.Fatalf("unknown field leaked into diagnostics: %s", got)
	}
}
