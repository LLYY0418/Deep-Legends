package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Remote profiles do not expose their selected client background, and some
// servers do not expose mastery. Reuse this player's latest loaded match as a
// final artwork fallback; never query more matches just to decorate the header.
func (a *app) completeOverviewBackground(overview *gameplayOverview, playerRef string) {
	player := &overview.Player
	applyMasteryBackgroundFallback(player, overview.Masteries)
	if player.BackgroundPath == "" {
		var latest int64
		for _, match := range overview.Matches {
			for _, participant := range match.Participants {
				isSubject := (playerRef != "" && participant.PlayerRef == playerRef) ||
					(match.SubjectParticipantID > 0 && participant.ParticipantID == match.SubjectParticipantID)
				if !isSubject || participant.ChampionID <= 0 || (player.BackgroundPath != "" && match.CreatedAt <= latest) {
					continue
				}
				latest = match.CreatedAt
				player.BackgroundSkinID = participant.ChampionID * 1000
				player.BackgroundSkinName = participant.ChampionName
				player.BackgroundSource = "gtimg"
				player.BackgroundPath = fmt.Sprintf("%s%d.jpg", gtimgSkinArtworkPrefix, player.BackgroundSkinID)
			}
		}
	}
	a.applyOverviewSkinMedia(player)
}

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

// Public artwork metadata must not depend on opening Collection. Reuse the
// career catalog cache, scoped to the connected client, without loading loot or
// owned inventory. The overview starts this read alongside its other requests.
func (a *app) loadOverviewBackground(ctx context.Context, client *LCUClient, current Summoner, isCurrent bool) gameplayPlayer {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	type catalogResult struct {
		skins  []Skin
		source string
		err    error
	}
	catalogCh := make(chan catalogResult, 1)
	go func() {
		defer a.recoverPanic("overview.public-art-catalog")
		defer close(catalogCh)
		skins, source, err := a.loadFacadeSkins(ctx, client)
		catalogCh <- catalogResult{skins, source, err}
	}()
	player := gameplayPlayer{}
	var backdrop facadeBackdrop
	backdropAvailable := false
	if isCurrent {
		profile, capability := a.loadFacadeProfile(ctx, client, current)
		if capability.State != capabilityAvailable {
			player.BackgroundSkinID, player.BackgroundSkinName, player.BackgroundSource, player.BackgroundPath = a.currentProfileBackground()
		} else {
			a.applySummonerProfile(client, profile, capability)
			player.BackgroundSkinID, player.BackgroundSkinName, player.BackgroundSource, player.BackgroundPath = profileBackground(profile)
		}
		if current.SummonerID > 0 {
			err := client.RequestJSON(ctx, http.MethodGet, "/lol-collections/v1/inventories/"+strconv.FormatInt(current.SummonerID, 10)+"/backdrop", nil, &backdrop)
			backdropAvailable = err == nil && backdrop.SummonerID == current.SummonerID
		}
	}
	var catalog catalogResult
	select {
	case result, ok := <-catalogCh:
		if ok {
			catalog = result
		}
	case <-ctx.Done():
	}
	if backdropAvailable {
		state := facadeState{Profile: facadeProfile{BackgroundSkinID: player.BackgroundSkinID, BackgroundSkinName: player.BackgroundSkinName}}
		for _, skin := range catalog.skins {
			state.Skins = append(state.Skins, projectFacadeSkin(skin))
		}
		a.applyFacadeBackdropValue(backdrop, &state)
		if state.Profile.BackgroundPath != "" {
			player.BackgroundSkinID, player.BackgroundSkinName = state.Profile.BackgroundSkinID, state.Profile.BackgroundSkinName
			player.BackgroundPosterPath = state.Profile.BackgroundPath
			if player.BackgroundSkinID > 0 {
				player.BackgroundSource = "gtimg"
				player.BackgroundPath = fmt.Sprintf("%s%d.jpg", gtimgSkinArtworkPrefix, player.BackgroundSkinID)
			} else {
				player.BackgroundSource = ""
				player.BackgroundPath = state.Profile.BackgroundPath
			}
		}
	}

	a.recordDiagnostic(map[string]any{"event": "overview_background_read", "current": isCurrent, "catalog_source": catalog.source, "catalog_count": len(catalog.skins), "catalog_available": len(catalog.skins) > 0 && catalog.err == nil, "cancelled": ctx.Err() != nil, "skin_id": player.BackgroundSkinID, "backdrop_available": player.BackgroundPosterPath != ""})
	return player
}

func (a *app) cachedOverviewSkinMedia(client *LCUClient, id int64) (poster, video string, count int, source string) {
	a.mu.RLock()
	skins := a.allSkinsWithBase
	if len(skins) == 0 {
		skins = a.allSkins
	}
	poster, video = overviewSkinMedia(skins, id)
	count, source = len(skins), "collection"
	a.mu.RUnlock()
	if poster != "" {
		return
	}
	// Status and completed overview reads must not wait behind a catalog fetch.
	if !a.facadeSkinCatalogMu.TryLock() {
		return
	}
	defer a.facadeSkinCatalogMu.Unlock()
	if a.facadeSkinCatalogClient == client && client != nil {
		poster, video = overviewSkinMedia(a.facadeSkinCatalog, id)
		count, source = len(a.facadeSkinCatalog), "lcu-catalog"
	}
	return
}

func (a *app) applyOverviewSkinMedia(player *gameplayPlayer) {
	a.mu.RLock()
	client := a.lcu
	a.mu.RUnlock()
	poster, video, count, source := a.cachedOverviewSkinMedia(client, player.BackgroundSkinID)
	if poster != "" {
		player.BackgroundPosterPath, player.BackgroundVideoPath = poster, video
	}
	a.recordDiagnostic(map[string]any{"event": "overview_art_source", "skin_id": player.BackgroundSkinID, "catalog_count": count, "catalog_source": source, "poster_available": player.BackgroundPosterPath != "", "fallback_available": player.BackgroundPath != "", "composition": overviewComposition(player.BackgroundPosterPath), "animation_available": player.BackgroundVideoPath != ""})
}
