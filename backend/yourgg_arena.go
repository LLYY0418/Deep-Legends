package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type arenaFirstPlacesResponse struct {
	Source            string          `json:"source"`
	Region            string          `json:"region"`
	Patch             string          `json:"patch,omitempty"`
	FetchedAt         time.Time       `json:"fetchedAt"`
	UnavailableReason string          `json:"unavailableReason,omitempty"`
	Matches           []gameplayMatch `json:"matches"`
}

type arenaFirstPlaceFilterStats struct {
	Returned         int
	Accepted         int
	Invalid          int
	UnknownChampion  int
	ChampionMismatch int
	NotFirst         int
}

type yourGGArenaResponse struct {
	Success    bool `json:"success"`
	StatusCode int  `json:"statusCode"`
	Response   struct {
		Version string             `json:"version"`
		Builds  []yourGGArenaBuild `json:"builds"`
	} `json:"response"`
}

type yourGGArenaAggregateResponse struct {
	Success    bool `json:"success"`
	StatusCode int  `json:"statusCode"`
	Response   struct {
		Version        string                       `json:"version"`
		CoreItems      []yourGGArenaAggregateMetric `json:"coreItems"`
		PrismaticItems []yourGGArenaAggregateMetric `json:"prismaticItems"`
		TotalMatches   int                          `json:"totalMatches"`
	} `json:"response"`
}

type yourGGArenaAggregateMetric struct {
	ItemID             int     `json:"itemId"`
	Tier               string  `json:"tier"`
	Score              float64 `json:"score"`
	WinRate            float64 `json:"winRate"`
	AveragePlacement   float64 `json:"averagePlacement"`
	FirstPlacementRate float64 `json:"firstPlacementRate"`
	PickRate           float64 `json:"pickRate"`
	Matches            int     `json:"matches"`
}

type communityDragonItemRaw struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	NameTRA          string `json:"nameTRA"`
	Description      string `json:"description"`
	ShortDescription string `json:"shortDescription"`
	IconPath         string `json:"iconPath"`
	ImagePath        string `json:"imagePath"`
	PriceTotal       int64  `json:"priceTotal"`
}

// loadCommunityDragonItems resolves six-digit Arena item IDs to the localized
// name and game asset path used by the client. The provider-level copy avoids
// repeated catalog requests when a detail page loads both core and prismatic
// sections; the upstream response remains governed by the normal cache policy.
func (p *championProvider) loadCommunityDragonItems(ctx context.Context) ([]gameplayItem, error) {
	p.mu.Lock()
	if len(p.arenaItems) > 0 && time.Since(p.arenaItemsAt) < 30*time.Minute {
		items := append([]gameplayItem(nil), p.arenaItems...)
		p.mu.Unlock()
		return items, nil
	}
	p.mu.Unlock()
	data, err := p.fetch(ctx, communityDragonHost, "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json", nil, championJSONMax, "application/json")
	if err != nil {
		return nil, err
	}
	var raw []communityDragonItemRaw
	if json.Unmarshal(data, &raw) != nil || len(raw) == 0 {
		var wrapped struct {
			Items []communityDragonItemRaw `json:"items"`
		}
		if json.Unmarshal(data, &wrapped) != nil || len(wrapped.Items) == 0 {
			return nil, errors.New("CommunityDragon item catalog response changed")
		}
		raw = wrapped.Items
	}
	items := make([]gameplayItem, 0, len(raw))
	for _, source := range raw {
		if source.ID <= 0 {
			continue
		}
		name := strings.TrimSpace(source.Name)
		if name == "" {
			name = strings.TrimSpace(source.DisplayName)
		}
		if name == "" {
			name = strings.TrimSpace(source.NameTRA)
		}
		path := communityDragonGameAssetPath(firstNonEmpty(source.IconPath, source.ImagePath))
		items = append(items, gameplayItem{ID: source.ID, Name: name, Description: cleanMarkup(firstNonEmpty(source.Description, source.ShortDescription)), IconPath: path, Price: source.PriceTotal})
	}
	if len(items) == 0 {
		return nil, errors.New("CommunityDragon item catalog is empty")
	}
	p.mu.Lock()
	p.arenaItems, p.arenaItemsAt = append([]gameplayItem(nil), items...), time.Now()
	p.mu.Unlock()
	return items, nil
}

func mapYourGGArenaAggregateItems(values []yourGGArenaAggregateMetric, catalog []gameplayItem) []championMetricRow {
	metadata := make(map[int64]gameplayItem, len(catalog))
	for _, item := range catalog {
		metadata[item.ID] = item
	}
	rows := make([]championMetricRow, 0, len(values))
	for _, value := range values {
		if value.ItemID <= 0 || value.Matches <= 0 {
			continue
		}
		item := metadata[int64(value.ItemID)]
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = "装备 " + strconv.Itoa(value.ItemID)
		}
		path := strings.TrimSpace(item.IconPath)
		source := "communitydragon"
		if path == "" {
			source = "ddragon"
			path = "/cdn/latest/img/item/" + strconv.Itoa(value.ItemID) + ".png"
		}
		grade := strings.ToUpper(strings.TrimSpace(value.Tier))
		rows = append(rows, championMetricRow{
			Assets: []championAsset{{ID: value.ItemID, Kind: "item", Name: name, Description: item.Description, Source: source, Path: path}},
			Tier:   grade, Grade: grade, Score: value.Score,
			WinRate: value.WinRate * 100, AveragePlacement: value.AveragePlacement,
			FirstPlaceRate: value.FirstPlacementRate * 100, PickRate: value.PickRate * 100, Games: value.Matches,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Score > rows[j].Score })
	return rows
}

func (p *championProvider) loadArenaChampionAggregate(ctx context.Context, championID int) (yourGGArenaAggregateResponse, []gameplayItem, time.Time, error) {
	if championID <= 0 {
		return yourGGArenaAggregateResponse{}, nil, time.Time{}, errors.New("invalid arena aggregate request")
	}
	requestPath := "/kr/api/arena/champions/" + strconv.Itoa(championID)
	data, fetchedAt, err := p.fetchWithMetadata(ctx, yourGGArenaHost, requestPath, nil, championJSONMax, "application/json")
	if err != nil {
		return yourGGArenaAggregateResponse{}, nil, time.Time{}, err
	}
	var payload yourGGArenaAggregateResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return yourGGArenaAggregateResponse{}, nil, time.Time{}, errors.New("YOUR.GG aggregate response changed")
	}
	if payload.Response.CoreItems == nil && payload.Response.PrismaticItems == nil {
		return yourGGArenaAggregateResponse{}, nil, time.Time{}, errors.New("YOUR.GG aggregate response is missing item sections")
	}
	items, err := p.loadCommunityDragonItems(ctx)
	if err != nil {
		if p.diag != nil {
			p.diag(map[string]any{"event": "arena_item_catalog_failed", "source": "communitydragon", "errorKind": championProviderErrorKind(err)})
		}
		items = nil
	}
	return payload, items, fetchedAt, nil
}

type yourGGArenaBuild struct {
	MatchID int64            `json:"matchId"`
	Match   yourGGArenaMatch `json:"match"`
}

type yourGGArenaMatch struct {
	MatchID       int64                    `json:"matchId"`
	MatchDate     int64                    `json:"matchDate"`
	GameTime      int64                    `json:"gameTime"`
	MatchCategory string                   `json:"matchCategory"`
	Result        string                   `json:"result"`
	Me            yourGGArenaSubject       `json:"me"`
	Participants  []yourGGArenaParticipant `json:"participants"`
}

type yourGGArenaParticipant struct {
	ChampionKey      string `json:"championKey"`
	RiotIDGameName   string `json:"riotIdGameName"`
	RiotIDTagline    string `json:"riotIdTagline"`
	SummonerID       string `json:"summonerId"`
	SubteamID        int64  `json:"subteamId"`
	SubteamPlacement int    `json:"subteamPlacement"`
	IsBot            bool   `json:"isBot"`
}

type yourGGArenaSubject struct {
	yourGGArenaParticipant
	Level       int               `json:"level"`
	Kills       int               `json:"kills"`
	Deaths      int               `json:"deaths"`
	Assists     int               `json:"assists"`
	KDA         float64           `json:"kda"`
	CS          int               `json:"cs"`
	CSPerMinute float64           `json:"csPerMinute"`
	Gold        int               `json:"gold"`
	Damage      int               `json:"damage"`
	DamageTaken int               `json:"damageTaken"`
	WardPlaced  int               `json:"wardPlaced"`
	WardKills   int               `json:"wardKills"`
	Items       []yourGGArenaIcon `json:"items"`
	Augments    []yourGGArenaIcon `json:"augments"`
}

type yourGGArenaIcon struct {
	ID int64 `json:"id"`
}

func (p *championProvider) waitForYourGG(ctx context.Context) error {
	p.yourGGMu.Lock()
	defer p.yourGGMu.Unlock()
	if wait := time.Second - time.Since(p.yourGGLast); !p.yourGGLast.IsZero() && wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	p.yourGGLast = time.Now()
	return nil
}

func (p *championProvider) loadArenaFirstPlaces(ctx context.Context, championID, limit int) (arenaFirstPlacesResponse, error) {
	if championID <= 0 || limit <= 0 {
		return arenaFirstPlacesResponse{}, errors.New("invalid arena first-place request")
	}
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	requestPath := "/kr/api/arena/champions/" + strconv.Itoa(championID) + "/top-builds"
	data, fetchedAt, err := p.fetchWithMetadata(ctx, yourGGArenaHost, requestPath, query, championJSONMax, "application/json")
	if err != nil {
		p.reportArenaFirstPlacesFailure(arenaFirstPlacesErrorKind(err))
		return arenaFirstPlacesResponse{}, err
	}
	var payload yourGGArenaResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		p.reportArenaFirstPlacesFailure("invalid-payload")
		return arenaFirstPlacesResponse{}, errors.New("YOUR.GG 返回的数据格式暂时无法识别")
	}
	if !payload.Success || (payload.StatusCode != 0 && payload.StatusCode != 200) {
		p.reportArenaFirstPlacesFailure("invalid-payload")
		return arenaFirstPlacesResponse{}, errors.New("YOUR.GG 返回的数据格式暂时无法识别")
	}
	lookup := p.yourGGChampionLookup()
	inferYourGGSubjectChampion(lookup, payload.Response.Builds, championID)
	matches, stats := filterYourGGArenaBuilds(payload.Response.Builds, lookup, championID, limit)
	unavailableReason := arenaFirstUnavailableReason(stats, len(matches))
	p.reportArenaFirstPlaces(stats, len(matches))
	return arenaFirstPlacesResponse{
		Source: "YOUR.GG", Region: "KR", Patch: payload.Response.Version,
		FetchedAt: fetchedAt, UnavailableReason: unavailableReason, Matches: matches,
	}, nil
}

func (p *championProvider) reportArenaFirstPlaces(stats arenaFirstPlaceFilterStats, rowsOut int) {
	if p.diag != nil {
		p.diag(map[string]any{
			"event": "arena_first_places", "source": "your.gg", "returned": stats.Returned, "accepted": stats.Accepted, "rowsOut": rowsOut,
			"invalid": stats.Invalid, "unknownChampion": stats.UnknownChampion, "championMismatch": stats.ChampionMismatch, "notFirst": stats.NotFirst,
		})
	}
}

func inferYourGGSubjectChampion(lookup map[string]championMetadata, builds []yourGGArenaBuild, championID int) {
	unknown := make(map[string]string)
	for _, build := range builds {
		key := strings.TrimSpace(build.Match.Me.ChampionKey)
		normalized := strings.ToLower(key)
		if normalized != "" {
			if _, known := lookup[normalized]; !known {
				unknown[normalized] = key
			}
		}
	}
	if len(unknown) != 1 {
		return
	}
	for normalized, key := range unknown {
		lookup[normalized] = championMetadata{ID: championID, Key: key, Slug: normalized, NameZH: key}
	}
}

func arenaFirstPlacesErrorKind(err error) string {
	value := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(value, "timeout") || strings.Contains(value, "deadline"):
		return "timeout"
	case strings.Contains(value, "http 403"):
		return "forbidden"
	case strings.Contains(value, "http 404"):
		return "not-found"
	case strings.Contains(value, "empty response"):
		return "empty-response"
	default:
		return "unavailable"
	}
}

func (p *championProvider) reportArenaFirstPlacesFailure(kind string) {
	if p.diag != nil {
		p.diag(map[string]any{"event": "arena_first_places", "source": "your.gg", "outcome": "error", "errorKind": kind})
	}
}

func filterYourGGArenaBuilds(builds []yourGGArenaBuild, lookup map[string]championMetadata, championID, limit int) ([]gameplayMatch, arenaFirstPlaceFilterStats) {
	stats := arenaFirstPlaceFilterStats{Returned: len(builds)}
	matches := make([]gameplayMatch, 0, min(limit, len(builds)))
	for _, build := range builds {
		raw := build.Match
		if raw.MatchID == 0 {
			raw.MatchID = build.MatchID
		}
		match, ok := convertYourGGArenaMatch(raw, lookup)
		if !ok {
			stats.Invalid++
			continue
		}
		if match.Participants[0].ChampionID <= 0 {
			stats.UnknownChampion++
			continue
		}
		if match.Participants[0].ChampionID != int64(championID) {
			stats.ChampionMismatch++
			continue
		}
		if match.Participants[0].Placement != 1 {
			stats.NotFirst++
			continue
		}
		stats.Accepted++
		if len(matches) < limit {
			matches = append(matches, match)
		}
	}
	return matches, stats
}

func arenaFirstUnavailableReason(stats arenaFirstPlaceFilterStats, rowsOut int) string {
	if rowsOut > 0 {
		return ""
	}
	if stats.Returned == 0 {
		return "empty"
	}
	if stats.UnknownChampion > 0 {
		return "champion-metadata-mismatch"
	}
	if stats.ChampionMismatch > 0 {
		return "champion-mismatch"
	}
	if stats.NotFirst > 0 {
		return "no-first-place"
	}
	return "invalid-upstream-data"
}

func (p *championProvider) yourGGChampionLookup() map[string]championMetadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	lookup := make(map[string]championMetadata, len(p.championMeta)*2)
	for _, meta := range p.championMeta {
		lookup[strings.ToLower(meta.Key)] = meta
		lookup[strings.ToLower(meta.Slug)] = meta
	}
	return lookup
}

func convertYourGGArenaMatch(raw yourGGArenaMatch, champions map[string]championMetadata) (gameplayMatch, bool) {
	if raw.MatchID <= 0 || raw.Me.SubteamID <= 0 || raw.Me.SubteamPlacement <= 0 {
		return gameplayMatch{}, false
	}
	result := gameplayMatch{
		GameID: raw.MatchID, CreatedAt: normalizeEpochMillis(raw.MatchDate), Duration: raw.GameTime,
		QueueID: 1700, QueueLabel: "斗魂竞技场", ModeGroup: "arena", GameMode: "CHERRY",
		GameType: "MATCHED_GAME", MapID: 30, Result: "win", SubjectParticipantID: 1,
	}
	subject := yourGGGameplayParticipant(raw.Me.yourGGArenaParticipant, champions, 1)
	subject.ChampionLevel = raw.Me.Level
	subject.Kills, subject.Deaths, subject.Assists = raw.Me.Kills, raw.Me.Deaths, raw.Me.Assists
	subject.KDA = raw.Me.KDA
	if subject.KDA <= 0 {
		subject.KDA = ratio(subject.Kills+subject.Assists, subject.Deaths)
	}
	subject.CS, subject.LaneCS, subject.CSPerMinute = raw.Me.CS, raw.Me.CS, raw.Me.CSPerMinute
	if subject.CSPerMinute <= 0 {
		subject.CSPerMinute = perMinute(subject.CS, raw.GameTime)
	}
	subject.Gold, subject.Damage, subject.DamageTaken = raw.Me.Gold, raw.Me.Damage, raw.Me.DamageTaken
	subject.WardsPlaced, subject.WardsKilled = raw.Me.WardPlaced, raw.Me.WardKills
	for _, item := range raw.Me.Items {
		if item.ID > 0 {
			subject.ItemIDs = append(subject.ItemIDs, item.ID)
		}
	}
	for _, augment := range raw.Me.Augments {
		if augment.ID > 0 {
			subject.AugmentIDs = append(subject.AugmentIDs, augment.ID)
		}
	}
	result.Participants = append(result.Participants, subject)
	for _, rawParticipant := range raw.Participants {
		if sameYourGGArenaPlayer(raw.Me.yourGGArenaParticipant, rawParticipant) {
			continue
		}
		participantID := int64(len(result.Participants) + 1)
		result.Participants = append(result.Participants, yourGGGameplayParticipant(rawParticipant, champions, participantID))
	}
	teamByID := make(map[int64]gameplayTeam)
	for _, participant := range result.Participants {
		team := teamByID[participant.SubteamID]
		team.TeamID = participant.SubteamID
		team.Win = team.Win || participant.Placement == 1
		team.Kills += participant.Kills
		team.Gold += participant.Gold
		team.Damage += participant.Damage
		team.DamageTaken += participant.DamageTaken
		team.VisionScore += participant.VisionScore
		team.CS += participant.CS
		teamByID[participant.SubteamID] = team
	}
	for _, team := range teamByID {
		result.Teams = append(result.Teams, team)
	}
	sort.Slice(result.Teams, func(i, j int) bool { return result.Teams[i].TeamID < result.Teams[j].TeamID })
	return result, true
}

func yourGGGameplayParticipant(raw yourGGArenaParticipant, champions map[string]championMetadata, participantID int64) gameplayParticipant {
	gameName := strings.TrimSpace(raw.RiotIDGameName)
	tagLine := strings.TrimSpace(raw.RiotIDTagline)
	hidden := gameName == ""
	displayName := gameName
	if displayName == "" {
		displayName = "隐藏玩家"
	} else if tagLine != "" {
		displayName += "#" + tagLine
	}
	meta := champions[strings.ToLower(strings.TrimSpace(raw.ChampionKey))]
	championName := meta.NameZH
	if championName == "" {
		championName = strings.TrimSpace(raw.ChampionKey)
	}
	return gameplayParticipant{
		ParticipantID: participantID, TeamID: raw.SubteamID,
		DisplayName: displayName, GameName: gameName, TagLine: tagLine,
		ChampionID: int64(meta.ID), ChampionName: championName,
		PerkIDs: []int64{}, ItemIDs: []int64{},
		Position: "other", Win: raw.SubteamPlacement == 1, Hidden: hidden || raw.IsBot,
		SubteamID: raw.SubteamID, Placement: raw.SubteamPlacement,
	}
}

func sameYourGGArenaPlayer(subject, candidate yourGGArenaParticipant) bool {
	if subject.SummonerID != "" && candidate.SummonerID != "" && subject.SummonerID == candidate.SummonerID {
		return true
	}
	gameName := strings.TrimSpace(subject.RiotIDGameName)
	return gameName != "" && strings.EqualFold(gameName, strings.TrimSpace(candidate.RiotIDGameName)) &&
		strings.EqualFold(strings.TrimSpace(subject.RiotIDTagline), strings.TrimSpace(candidate.RiotIDTagline)) &&
		strings.EqualFold(strings.TrimSpace(subject.ChampionKey), strings.TrimSpace(candidate.ChampionKey)) &&
		subject.SubteamID > 0 && subject.SubteamID == candidate.SubteamID
}
