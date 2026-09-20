package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
)

// Explicit refs retain their privacy and expiry rules. A public Riot ID needs
// only OP.GG's independently validated profile; it never waits for match-v5.
func (a *app) resolveOverviewSupplement(playerRef, gameName, tagLine, region string) (gameplayReference, int) {
	if strings.TrimSpace(playerRef) != "" {
		ref, ok := a.resolveGameplayReferenceDetails(strings.TrimSpace(playerRef))
		if !ok {
			return gameplayReference{}, http.StatusNotFound
		}
		return ref, 0
	}
	gameName, tagLine = strings.TrimSpace(gameName), strings.TrimSpace(strings.TrimPrefix(tagLine, "#"))
	if !strings.EqualFold(strings.TrimSpace(region), riotRegionKR) || gameName == "" || tagLine == "" || len([]rune(gameName)) > 40 || len([]rune(tagLine)) > 12 {
		return gameplayReference{}, http.StatusBadRequest
	}
	key := fmt.Sprintf("opgg-%x", sha256.Sum256([]byte(strings.ToLower(gameName+"#"+tagLine))))
	return gameplayReference{PlayerRef: key, GameName: gameName, TagLine: tagLine, Region: riotRegionKR, OPGGIdentity: true}, 0
}

func overviewSupplementCacheIdentity(ref gameplayReference) string {
	if ref.GameName != "" && ref.TagLine != "" {
		return strings.ToLower(ref.Region + ":" + ref.GameName + "#" + ref.TagLine)
	}
	return ref.Region + ":" + ref.PlayerRef
}
