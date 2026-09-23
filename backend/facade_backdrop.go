package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

// The profile editor reads a customization preference. The client's actual
// career page binds to collections/backdrop, which also resolves automatic
// recently-played, mastery and icon backgrounds when that preference is zero.
type facadeBackdrop struct {
	SummonerID int64  `json:"summonerId"`
	ChampionID int64  `json:"championId"`
	Image      string `json:"backdropImage"`
	Type       string `json:"backdropType"`
}

func (a *app) applyFacadeBackdrop(ctx context.Context, client *LCUClient, current Summoner, state *facadeState) {
	if current.SummonerID <= 0 {
		return
	}
	var backdrop facadeBackdrop
	err := client.RequestJSON(ctx, http.MethodGet, "/lol-collections/v1/inventories/"+strconv.FormatInt(current.SummonerID, 10)+"/backdrop", nil, &backdrop)
	if err != nil || backdrop.SummonerID != current.SummonerID {
		return
	}
	if backdrop.Type == "recently-played" || backdrop.Type == "highest-mastery" {
		state.Profile.BackgroundChampionID = backdrop.ChampionID
	}
	imagePath := sanitizeClientImagePath(backdrop.Image)
	if imagePath == "" {
		return
	}
	state.Profile.BackgroundPath = imagePath
	state.Profile.BackgroundType = backdrop.Type
	state.ProfileUnavailable = false
	// Match real catalog paths first. The automatic champion backdrops use base
	// artwork; an explicit skin preference remains distinct from those modes.
	var matched *facadeSkin
	for i := range state.Skins {
		skin := &state.Skins[i]
		if strings.EqualFold(skin.SplashPath, imagePath) || strings.EqualFold(skin.TilePath, imagePath) {
			matched = skin
			break
		}
	}
	if matched == nil && (backdrop.Type == "recently-played" || backdrop.Type == "highest-mastery") {
		for i := range state.Skins {
			if state.Skins[i].ID == backdrop.ChampionID*1000 {
				matched = &state.Skins[i]
				break
			}
		}
	}
	if matched != nil {
		state.Profile.BackgroundSkinID = matched.ID
		state.Profile.BackgroundSkinName = matched.Name
		state.Profile.BackgroundChampionID = matched.ChampionID
	} else if backdrop.Type != "specified-skin" {
		state.Profile.BackgroundSkinID = 0
		state.Profile.BackgroundSkinName = "客户端当前背景"
	}
	a.recordDiagnostic(map[string]any{"event": "facade_backdrop_read", "backdrop_type": backdrop.Type,
		"background_skin_id": state.Profile.BackgroundSkinID, "background_champion_id": state.Profile.BackgroundChampionID,
		"catalog_matched": matched != nil, "image_available": true})
}
