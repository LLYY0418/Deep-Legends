package main

import "strings"

// Prefer the catalog's centered splash/animation, exactly as the skin collection
// does. Only use the selected skin ID; never substitute a sibling quest stage.
func overviewSkinMedia(skins []Skin, id int64) (poster, video string) {
	for _, skin := range skins {
		if skin.ID != id {
			continue
		}
		poster = sanitizeClientImagePath(skin.CenteredSplashPath)
		if poster == "" {
			poster = sanitizeClientImagePath(skin.SplashPath)
		}
		for _, path := range []string{skin.SplashVideoPath, skin.CollectionVideoPath} {
			// Collection animations are commonly uncentered. Do not replace a
			// centered poster with a different composition when playback begins.
			if overviewComposition(poster) != "" && overviewComposition(path) == overviewComposition(poster) && strings.HasPrefix(strings.ToLower(path), "/lol-game-data/assets/") && strings.HasSuffix(strings.ToLower(path), ".webm") {
				video = path
				break
			}
		}
		return
	}
	return
}

func overviewComposition(path string) string {
	path = strings.ToLower(path)
	if strings.Contains(path, "uncentered") {
		return "uncentered"
	}
	if strings.Contains(path, "centered") {
		return "centered"
	}
	return ""
}
func (a *app) applyOverviewSkinMedia(player *gameplayPlayer) {
	a.mu.RLock()
	player.BackgroundPosterPath, player.BackgroundVideoPath = overviewSkinMedia(a.allSkins, player.BackgroundSkinID)
	count := len(a.allSkins)
	a.mu.RUnlock()
	a.recordDiagnostic(map[string]any{"event": "overview_art_source", "skin_id": player.BackgroundSkinID, "catalog_count": count, "poster_available": player.BackgroundPosterPath != "", "composition": overviewComposition(player.BackgroundPosterPath), "animation_available": player.BackgroundVideoPath != ""})
}
