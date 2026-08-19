package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	specialistRuneCacheTTL      = 6 * time.Hour
	specialistRunePlayerLimit   = 2
	specialistRuneMatchIDCount  = 10
	specialistRuneMatchScanMax  = 8
	specialistRuneRequestBudget = 24
)

type specialistRuneCacheEntry struct {
	expiresAt time.Time
	runes     []gameplayRecommendationRune
}

type specialistRuneFlight struct {
	done    chan struct{}
	runes   []gameplayRecommendationRune
	waiters int
}

type specialistRequestBudget struct {
	remaining int
}

func (b *specialistRequestBudget) take() bool {
	if b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

func (a *app) handleGameplaySpecialistRunes(w http.ResponseWriter, r *http.Request) {
	championID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("championId")), 10, 64)
	if err != nil || championID <= 0 || championID > 10000 {
		http.Error(w, "推荐英雄无效", http.StatusBadRequest)
		return
	}
	if _, err := normalizeOPGGPosition(r.URL.Query().Get("position")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !riotKeyConfigured() || a.riot == nil {
		respondJSON(w, []gameplayRecommendationRune{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	metadata, err := a.championDataProvider().championMetadataByID(ctx, int(championID))
	if err != nil {
		http.Error(w, "暂时无法识别当前英雄", http.StatusNotFound)
		return
	}
	respondJSON(w, a.riot.specialistRunes(ctx, championID, metadata.Slug, metadata.NameZH))
}

func (p *riotProvider) specialistRunes(ctx context.Context, championID int64, championSlug, championName string) []gameplayRecommendationRune {
	if p == nil || p.champions == nil || !riotKeyConfigured() || championID <= 0 || strings.TrimSpace(championSlug) == "" {
		return []gameplayRecommendationRune{}
	}
	now := time.Now()
	p.specialistMu.Lock()
	if cached, ok := p.specialistCache[championID]; ok && now.Before(cached.expiresAt) {
		result := cloneSpecialistRunes(cached.runes)
		p.specialistMu.Unlock()
		return result
	}
	if flight := p.specialistFlights[championID]; flight != nil {
		flight.waiters++
		done := flight.done
		p.specialistMu.Unlock()
		select {
		case <-done:
			return cloneSpecialistRunes(flight.runes)
		case <-ctx.Done():
			return []gameplayRecommendationRune{}
		}
	}
	flight := &specialistRuneFlight{done: make(chan struct{})}
	p.specialistFlights[championID] = flight
	p.specialistMu.Unlock()

	select {
	case p.specialistSlots <- struct{}{}:
	case <-ctx.Done():
		p.finishSpecialistRuneFlight(championID, flight, nil, time.Now(), false)
		return []gameplayRecommendationRune{}
	}
	runes := p.loadSpecialistRunes(ctx, championID, championSlug, championName)
	<-p.specialistSlots
	p.finishSpecialistRuneFlight(championID, flight, runes, time.Now(), true)
	return cloneSpecialistRunes(runes)
}

func (p *riotProvider) finishSpecialistRuneFlight(championID int64, flight *specialistRuneFlight, runes []gameplayRecommendationRune, fetchedAt time.Time, cache bool) {
	result := cloneSpecialistRunes(runes)
	p.specialistMu.Lock()
	if cache {
		p.specialistCache[championID] = specialistRuneCacheEntry{expiresAt: fetchedAt.Add(specialistRuneCacheTTL), runes: result}
	}
	flight.runes = cloneSpecialistRunes(result)
	delete(p.specialistFlights, championID)
	close(flight.done)
	p.specialistMu.Unlock()
}

func (p *riotProvider) loadSpecialistRunes(ctx context.Context, championID int64, championSlug, championName string) []gameplayRecommendationRune {
	players := p.champions.loadTopPlayers(ctx, championSlug)
	if len(players) > specialistRunePlayerLimit {
		players = players[:specialistRunePlayerLimit]
	}
	result := make([]gameplayRecommendationRune, 0, len(players))
	budget := specialistRequestBudget{remaining: specialistRuneRequestBudget}
	for _, player := range players {
		if strings.TrimSpace(player.Name) == "" || strings.TrimSpace(player.Tagline) == "" || !budget.take() {
			continue
		}
		account, err := p.accountByRiotID(ctx, player.Name, player.Tagline)
		if err != nil || !budget.take() {
			continue
		}
		matchIDs, err := p.matchIDs(ctx, account.PUUID, 0, specialistRuneMatchIDCount)
		if err != nil {
			continue
		}
		if len(matchIDs) > specialistRuneMatchScanMax {
			matchIDs = matchIDs[:specialistRuneMatchScanMax]
		}
		for _, matchID := range matchIDs {
			if !budget.take() {
				return result
			}
			match, err := p.matchByID(ctx, matchID)
			if err != nil {
				continue
			}
			participant, found := specialistParticipant(match, account.PUUID)
			if !found || participant.ChampionID != championID {
				continue
			}
			if rune, ok := specialistRuneFromParticipant(championID, championName, player, match, participant, len(result)); ok {
				result = append(result, rune)
			}
			break
		}
	}
	return result
}

func specialistParticipant(match *riotMatch, puuid string) (riotParticipant, bool) {
	if match == nil || strings.TrimSpace(puuid) == "" {
		return riotParticipant{}, false
	}
	for _, participant := range match.Info.Participants {
		if participant.PUUID == puuid {
			return participant, true
		}
	}
	return riotParticipant{}, false
}

func specialistRuneFromParticipant(championID int64, championName string, player championTopPlayer, match *riotMatch, participant riotParticipant, index int) (gameplayRecommendationRune, bool) {
	selected := make([]int64, 0, 9)
	var primaryStyleID, subStyleID int64
	for styleIndex, style := range participant.Perks.Styles {
		description := strings.ToLower(strings.TrimSpace(style.Description))
		switch {
		case strings.Contains(description, "primary"):
			primaryStyleID = style.Style
		case strings.Contains(description, "sub") || strings.Contains(description, "secondary"):
			subStyleID = style.Style
		case styleIndex == 0 && primaryStyleID == 0:
			primaryStyleID = style.Style
		case subStyleID == 0:
			subStyleID = style.Style
		}
		for _, selection := range style.Selections {
			if selection.Perk > 0 {
				selected = append(selected, selection.Perk)
			}
		}
	}
	statMods := compactPositiveInt64(participant.Perks.StatPerks.Offense, participant.Perks.StatPerks.Flex, participant.Perks.StatPerks.Defense)
	selected = append(selected, statMods...)
	request := gameplayRuneApplyRequest{
		ChampionName: championName, Source: "绝活哥", ChampionID: championID,
		PrimaryStyleID: primaryStyleID, SubStyleID: subStyleID, SelectedPerkIDs: selected,
	}
	if validateRuneApplyRequest(request) != nil {
		return gameplayRecommendationRune{}, false
	}
	tier, division := specialistTier(player.Tier)
	games := specialistGames(player.Games)
	riotID := strings.TrimSpace(player.Name) + "#" + strings.TrimSpace(player.Tagline)
	result := "loss"
	if participant.Win {
		result = "win"
	}
	return gameplayRecommendationRune{
		Key: "specialist-" + strconv.Itoa(index), Title: riotID, ChampionID: championID, ChampionName: championName,
		PrimaryStyleID: primaryStyleID, SubStyleID: subStyleID, SelectedPerkIDs: selected, StatModIDs: statMods,
		Stats:      gameplayRecommendationStats{WinRate: player.WinRate, Games: games},
		PlayerName: strings.TrimSpace(player.Name), TagLine: strings.TrimSpace(player.Tagline), Tier: tier, Division: division,
		LeaguePoints: strings.TrimSpace(player.LP), ChampionGames: games, PlayedAt: normalizeEpochMillis(match.Info.GameCreation), Result: result, Region: riotRegionKR,
	}, true
}

func specialistTier(value string) (string, string) {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(parts) == 0 {
		return "", ""
	}
	division := ""
	if len(parts) > 1 {
		division = map[string]string{"1": "I", "2": "II", "3": "III", "4": "IV"}[parts[1]]
		if division == "" {
			division = strings.ToUpper(parts[1])
		}
	}
	return parts[0], division
}

func specialistGames(value string) int {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	games, _ := strconv.Atoi(digits)
	return games
}

func cloneSpecialistRunes(source []gameplayRecommendationRune) []gameplayRecommendationRune {
	result := make([]gameplayRecommendationRune, len(source))
	copy(result, source)
	for index := range result {
		result[index].SelectedPerkIDs = append([]int64(nil), source[index].SelectedPerkIDs...)
		result[index].StatModIDs = append([]int64(nil), source[index].StatModIDs...)
	}
	return result
}
