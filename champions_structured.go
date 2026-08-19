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

type opggMetric struct {
	IDs        []int        `json:"ids"`
	Order      []string     `json:"order"`
	Play       int          `json:"play"`
	Win        int          `json:"win"`
	TotalPlace int          `json:"total_place"`
	FirstPlace int          `json:"first_place"`
	PickRate   float64      `json:"pick_rate"`
	WinRate    float64      `json:"win_rate"`
	Builds     []opggMetric `json:"builds"`
}

type opggSkillMetric struct {
	IDs      []string         `json:"ids"`
	Play     int              `json:"play"`
	Win      int              `json:"win"`
	PickRate float64          `json:"pick_rate"`
	Builds   []opggSkillBuild `json:"builds"`
}

type opggSkillBuild struct {
	Order    []string `json:"order"`
	Play     int      `json:"play"`
	Win      int      `json:"win"`
	PickRate float64  `json:"pick_rate"`
}

type opggAugmentMetric struct {
	ID       int     `json:"id"`
	Play     int     `json:"play"`
	Win      int     `json:"win"`
	PickRate float64 `json:"pick_rate"`
	WinRate  float64 `json:"win_rate"`
}

type opggRunePage struct {
	ID              int             `json:"id"`
	PrimaryPageID   int             `json:"primary_page_id"`
	SecondaryPageID int             `json:"secondary_page_id"`
	Play            int             `json:"play"`
	Win             int             `json:"win"`
	PickRate        float64         `json:"pick_rate"`
	Builds          []opggRuneBuild `json:"builds"`
}

type opggRuneBuild struct {
	PrimaryPageID    int   `json:"primary_page_id"`
	PrimaryRuneIDs   []int `json:"primary_rune_ids"`
	SecondaryPageID  int   `json:"secondary_page_id"`
	SecondaryRuneIDs []int `json:"secondary_rune_ids"`
	StatModIDs       []int `json:"stat_mod_ids"`
}

type opggCounter struct {
	ChampionID int `json:"champion_id"`
	Play       int `json:"play"`
	Win        int `json:"win"`
}

type opggArenaAugmentGroup struct {
	Rarity   int                 `json:"rarity"`
	Augments []opggAugmentMetric `json:"augments"`
}

type opggSynergy struct {
	ChampionID int     `json:"champion_id"`
	Play       int     `json:"play"`
	Win        int     `json:"win"`
	TotalPlace int     `json:"total_place"`
	FirstPlace int     `json:"first_place"`
	PickRate   float64 `json:"pick_rate"`
}

type opggStructuredDetail struct {
	Data struct {
		Summary struct {
			ID           int             `json:"id"`
			AverageStats opggRankedStats `json:"average_stats"`
		} `json:"summary"`
		SummonerSpells []opggMetric            `json:"summoner_spells"`
		SkillMasteries []opggSkillMetric       `json:"skill_masteries"`
		StarterItems   []opggMetric            `json:"starter_items"`
		Boots          []opggMetric            `json:"boots"`
		CoreItems      []opggMetric            `json:"core_items"`
		LastItems      []opggMetric            `json:"last_items"`
		PrismItems     []opggMetric            `json:"prism_items"`
		RunePages      []opggRunePage          `json:"rune_pages"`
		Counters       []opggCounter           `json:"counters"`
		AugmentGroup   []opggArenaAugmentGroup `json:"augment_group"`
		Synergies      []opggSynergy           `json:"synergies"`
	} `json:"data"`
	Meta struct {
		Version  string `json:"version"`
		CachedAt string `json:"cached_at"`
	} `json:"meta"`
}

// championCounterFallbackTier 是对抗与整页样本不足时的回退段位：
// OP.GG 默认统计口径，样本量最大。
const championCounterFallbackTier = "emerald_plus"

func (p *championProvider) loadDetail(ctx context.Context, mode, champion, position, tier string) (championDetailResponse, error) {
	response, err := p.loadDetailOnce(ctx, mode, champion, position, tier)
	if mode != "ranked" || tier == championCounterFallbackTier {
		return response, err
	}
	if err != nil {
		// 最强王者等高分段经常整页无样本：退回翡翠以上样本，
		// 并通过 sampleTier 告知前端标注实际口径。
		fallback, fallbackErr := p.loadDetailOnce(ctx, mode, champion, position, championCounterFallbackTier)
		if fallbackErr != nil {
			return response, err
		}
		fallback.Tier = tier
		fallback.SampleTier = championCounterFallbackTier
		return fallback, nil
	}
	if len(response.Counters.WeakAgainst) == 0 && len(response.Counters.StrongAgainst) == 0 {
		// 只有对抗样本缺失时，单独回退对抗数据。
		if fallback, fallbackErr := p.loadDetailOnce(ctx, mode, champion, position, championCounterFallbackTier); fallbackErr == nil {
			response.Counters = fallback.Counters
			response.CountersTier = championCounterFallbackTier
		}
	}
	return response, nil
}

func (p *championProvider) loadDetailOnce(ctx context.Context, mode, champion, position, tier string) (championDetailResponse, error) {
	if mode == "ranked" || mode == "arena" {
		response, err := p.loadStructuredDetail(ctx, mode, champion, position, tier)
		if err == nil {
			if mode == "ranked" {
				response.TopPlayers = p.loadTopPlayers(ctx, champion)
			}
			return response, nil
		}
		p.reportStructuredDrift(mode, champion, err)
	}
	response, err := p.loadDetailHTML(ctx, mode, champion, position, tier)
	if err == nil && mode == "ranked" {
		response.TopPlayers = p.loadTopPlayers(ctx, champion)
	}
	return response, err
}

type championTopPlayer struct {
	Rank       int     `json:"rank"`
	Name       string  `json:"name"`
	Tagline    string  `json:"tagline,omitempty"`
	IconSource string  `json:"iconSource,omitempty"`
	IconPath   string  `json:"iconPath,omitempty"`
	Tier       string  `json:"tier,omitempty"`
	LP         string  `json:"lp,omitempty"`
	Games      string  `json:"games,omitempty"`
	WinRate    float64 `json:"winRate,omitempty"`
}

type opggLeaderboardRow struct {
	Rank     int `json:"rank"`
	Summoner struct {
		GameName        string `json:"game_name"`
		Tagline         string `json:"tagline"`
		ProfileImageURL string `json:"profile_image_url"`
	} `json:"summoner"`
	LeagueStats struct {
		TierInfo struct {
			Tier string `json:"tier"`
			LP   string `json:"lp"`
		} `json:"tier_info"`
		WinRatio float64 `json:"win_ratio"`
	} `json:"league_stats"`
	MostChampionStat struct {
		Play string `json:"play"`
	} `json:"most_champion_stat"`
}

// loadTopPlayers 从 OP.GG 英雄专家榜读取该英雄场次最多的玩家（韩服，
// 钻二以上样本），失败时返回空列表，不影响详情主体。
func (p *championProvider) loadTopPlayers(ctx context.Context, champion string) []championTopPlayer {
	data, err := p.fetch(ctx, opggPageHost, "/zh-cn/lol/leaderboards/champions/"+champion, url.Values{"region": {"kr"}}, championHTMLMax, "text/html,application/xhtml+xml")
	if err != nil {
		return nil
	}
	flight := decodeNextFlight(data)
	var rows []opggLeaderboardRow
	if !extractBestArray(flight, `"data":`, func(items []json.RawMessage) int {
		count := 0
		for _, item := range items {
			if strings.Contains(string(item), `"game_name"`) && strings.Contains(string(item), `"most_champion_stat"`) {
				count++
			}
		}
		return count
	}, &rows) {
		return nil
	}
	result := make([]championTopPlayer, 0, 5)
	for _, row := range rows {
		name := strings.TrimSpace(row.Summoner.GameName)
		if name == "" {
			continue
		}
		player := championTopPlayer{
			Rank: row.Rank, Name: name, Tagline: strings.TrimSpace(row.Summoner.Tagline),
			Tier: strings.ToLower(strings.TrimSpace(row.LeagueStats.TierInfo.Tier)), LP: strings.TrimSpace(row.LeagueStats.TierInfo.LP),
			Games: strings.TrimSpace(row.MostChampionStat.Play), WinRate: row.LeagueStats.WinRatio,
		}
		if parsed, parseErr := url.Parse(row.Summoner.ProfileImageURL); parseErr == nil && strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Hostname(), opggAssetHost) && strings.HasPrefix(parsed.Path, "/meta/images/") {
			player.IconSource, player.IconPath = "opgg", parsed.Path
		}
		result = append(result, player)
		if len(result) == 5 {
			break
		}
	}
	return result
}

// reportStructuredDrift records structured-API failures in the local
// diagnostics log so schema drift is visible instead of silently degrading to
// the HTML fallback. Entries are throttled per champion+mode to avoid spam.
func (p *championProvider) reportStructuredDrift(mode, champion string, err error) {
	if p.diag == nil || err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	key := mode + "|" + champion
	now := time.Now()
	p.mu.Lock()
	if p.diagLast == nil {
		p.diagLast = make(map[string]time.Time)
	}
	if last, ok := p.diagLast[key]; ok && now.Sub(last) < 30*time.Minute {
		p.mu.Unlock()
		return
	}
	p.diagLast[key] = now
	p.mu.Unlock()
	p.diag(map[string]any{"event": "champion_structured_fallback", "mode": mode, "champion": champion, "error": err.Error()})
}

func (p *championProvider) resolveChampionID(ctx context.Context, champion string) (int, championMetadata, error) {
	lookup := strings.ToLower(strings.TrimSpace(champion))
	p.mu.Lock()
	id, metadata := p.championIDs[lookup], championMetadata{}
	if id > 0 {
		metadata = p.championMeta[id]
	}
	p.mu.Unlock()
	if id == 0 {
		if _, err := p.loadCatalog(ctx); err != nil {
			return 0, championMetadata{}, err
		}
		p.mu.Lock()
		id, metadata = p.championIDs[lookup], p.championMeta[p.championIDs[lookup]]
		p.mu.Unlock()
	}
	if id <= 0 {
		return 0, championMetadata{}, errors.New("unknown champion")
	}
	return id, metadata, nil
}

func (p *championProvider) championMetadataByID(ctx context.Context, championID int) (championMetadata, error) {
	if championID <= 0 {
		return championMetadata{}, errors.New("unknown champion")
	}
	p.mu.Lock()
	metadata := p.championMeta[championID]
	p.mu.Unlock()
	if metadata.ID == 0 {
		if _, err := p.loadCatalog(ctx); err != nil {
			return championMetadata{}, err
		}
		p.mu.Lock()
		metadata = p.championMeta[championID]
		p.mu.Unlock()
	}
	if metadata.ID == 0 || strings.TrimSpace(metadata.Slug) == "" {
		return championMetadata{}, errors.New("unknown champion")
	}
	return metadata, nil
}

func (p *championProvider) loadStructuredDetail(ctx context.Context, mode, champion, position, tier string) (championDetailResponse, error) {
	id, _, err := p.resolveChampionID(ctx, champion)
	if err != nil {
		return championDetailResponse{}, err
	}
	requestPath := "/api/KR/champions/ranked/" + strconv.Itoa(id) + "/" + position
	query := url.Values{"tier": {tier}}
	region := "KR"
	if mode == "arena" {
		requestPath = "/api/global/champions/arena/" + strconv.Itoa(id)
		query = nil
		region = "GLOBAL"
	}
	data, err := p.fetch(ctx, opggChampionHost, requestPath, query, championJSONMax, "application/json")
	if err != nil {
		return championDetailResponse{}, err
	}
	var payload opggStructuredDetail
	if json.Unmarshal(data, &payload) != nil || payload.Data.Summary.ID != id {
		return championDetailResponse{}, errors.New("OP.GG champion detail response changed")
	}
	response := championDetailResponse{
		Mode: mode, Region: region, Tier: tier, Position: position, Patch: payload.Meta.Version,
		Source: "OP.GG JSON", FetchedAt: time.Now(), EntertainmentSample: mode == "arena",
	}
	stats := payload.Data.Summary.AverageStats
	response.Stats = championDetailStats{
		WinRate:  firstPositive(ratePercent(stats.WinRate), percentOf(stats.Win, stats.Play)),
		PickRate: ratePercent(stats.PickRate), BanRate: ratePercent(stats.BanRate),
	}
	response.Build.SummonerSpells = p.structuredMetrics(payload.Data.SummonerSpells, "spell", 5)
	response.Build.Skills = p.structuredSkills(payload.Data.SkillMasteries)
	response.Build.StarterItems = p.structuredMetrics(payload.Data.StarterItems, "item", 5)
	response.Build.Boots = p.structuredMetrics(payload.Data.Boots, "item", 5)
	response.Build.CoreItems = p.structuredMetrics(payload.Data.CoreItems, "item", 5)
	response.Build.PrismItems = p.structuredMetrics(payload.Data.PrismItems, "item", 8)
	late := p.structuredMetrics(payload.Data.LastItems, "item", 15)
	response.Build.FourthItems, response.Build.FifthItems, response.Build.SixthItems = splitLateItems(late)
	if mode == "ranked" {
		response.Runes = p.structuredRunes(ctx, payload.Data.RunePages)
		response.Counters = p.structuredCounters(payload.Data.Counters)
	} else {
		response.ArenaStats = arenaChampionStats{
			AveragePlacement: arenaAverage(float64(stats.TotalPlace), stats.Play), FirstPlaceRate: percentOf(stats.FirstPlace, stats.Play),
			PickRate: ratePercent(stats.PickRate), WinRate: percentOf(stats.Win, stats.Play), BanRate: ratePercent(stats.BanRate),
		}
		response.ArenaAugments = p.structuredArenaAugments(ctx, payload.Data.AugmentGroup)
		response.TeamCompositions = p.structuredSynergies(id, payload.Data.Synergies)
		response.Build = normalizeArenaBuild(response.Build)
		// OP.GG 的 JSON 提供单搭档协同；若页面缓存中存在精确三人组合，优先补齐为完整队伍。
		if htmlResponse, htmlErr := p.loadDetailHTML(ctx, mode, champion, position, tier); htmlErr == nil {
			if len(htmlResponse.TeamCompositions) > 0 {
				response.TeamCompositions = htmlResponse.TeamCompositions
			}
			if len(response.ArenaAugments) == 0 {
				response.ArenaAugments = htmlResponse.ArenaAugments
			}
		}
	}
	p.decorateDetailAssets(ctx, champion, &response)
	if len(response.Build.CoreItems) == 0 && len(response.Build.Boots) == 0 && len(response.Build.Skills) == 0 {
		return championDetailResponse{}, errors.New("OP.GG structured detail is incomplete")
	}
	return response, nil
}

func (p *championProvider) structuredMetrics(values []opggMetric, kind string, limit int) []championMetricRow {
	result := make([]championMetricRow, 0, min(limit, len(values)))
	for _, value := range values {
		if len(value.IDs) == 0 || value.Play <= 0 {
			continue
		}
		assets := make([]championAsset, 0, len(value.IDs))
		for _, id := range value.IDs {
			if id <= 0 {
				continue
			}
			assets = append(assets, championAsset{ID: id, Kind: kind, Name: strconv.Itoa(id), Source: "ddragon", Path: p.ddragonAssetPath(kind, id)})
		}
		if len(assets) == 0 {
			continue
		}
		result = append(result, championMetricRow{Assets: assets, PickRate: ratePercent(value.PickRate), WinRate: firstPositive(ratePercent(value.WinRate), percentOf(value.Win, value.Play)), Games: value.Play})
		if len(result) == limit {
			break
		}
	}
	return result
}

func (p *championProvider) ddragonAssetPath(kind string, id int) string {
	p.mu.Lock()
	patch := p.patch
	p.mu.Unlock()
	if !validDDragonVersion(patch) {
		patch = "latest"
	}
	directory := kind
	if kind == "spell" {
		// 数字 ID 的实际文件名由静态目录装饰阶段替换；这个占位不依赖客户端。
		return "/cdn/" + patch + "/img/spell/" + strconv.Itoa(id) + ".png"
	}
	return "/cdn/" + patch + "/img/" + directory + "/" + strconv.Itoa(id) + ".png"
}

func (p *championProvider) structuredSkills(values []opggSkillMetric) []championMetricRow {
	result := make([]championMetricRow, 0, min(3, len(values)))
	for _, value := range values {
		if len(value.Builds) == 0 && len(value.IDs) == 0 {
			continue
		}
		priority := make([]string, 0, len(value.IDs))
		assets := make([]championAsset, 0, len(value.IDs))
		priority = append(priority, value.IDs...)
		for _, key := range priority {
			assets = append(assets, championAsset{Kind: "skill", Name: key, Source: "ddragon", Path: "/cdn/img/champion-skill-placeholder.png"})
		}
		order, play, win, pick := []string(nil), value.Play, value.Win, value.PickRate
		if len(value.Builds) > 0 {
			order, play, win, pick = value.Builds[0].Order, value.Builds[0].Play, value.Builds[0].Win, value.Builds[0].PickRate
		}
		if len(priority) > 0 {
			result = append(result, championMetricRow{Assets: assets, SkillPriority: priority, SkillOrder: order, PickRate: ratePercent(pick), WinRate: percentOf(win, play), Games: play})
		}
		if len(result) == 3 {
			break
		}
	}
	return result
}

func splitLateItems(values []championMetricRow) ([]championMetricRow, []championMetricRow, []championMetricRow) {
	parts := [3][]championMetricRow{}
	for index, value := range values {
		parts[index%3] = append(parts[index%3], value)
	}
	return parts[0], parts[1], parts[2]
}

func arenaAverage(value float64, total int) float64 {
	if value <= 0 || total <= 0 {
		return 0
	}
	return value / float64(total)
}

func (p *championProvider) structuredCounters(values []opggCounter) championCounterSections {
	rows := make([]championCounterRow, 0, len(values))
	p.mu.Lock()
	metadata := make(map[int]championMetadata, len(p.championMeta))
	for id, item := range p.championMeta {
		metadata[id] = item
	}
	p.mu.Unlock()
	for _, value := range values {
		meta := metadata[value.ChampionID]
		if meta.ID == 0 || value.Play <= 0 {
			continue
		}
		rows = append(rows, championCounterRow{ChampionID: value.ChampionID, Key: meta.Key, Name: meta.NameZH, ImageSource: meta.ImageSource, ImagePath: meta.ImagePath, WinRate: percentOf(value.Win, value.Play), Games: value.Play})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].WinRate < rows[j].WinRate })
	result := championCounterSections{}
	for _, row := range rows {
		if len(result.WeakAgainst) < 5 {
			result.WeakAgainst = append(result.WeakAgainst, row)
		}
	}
	for index := len(rows) - 1; index >= 0 && len(result.StrongAgainst) < 5; index-- {
		result.StrongAgainst = append(result.StrongAgainst, rows[index])
	}
	return result
}

func (p *championProvider) structuredRunes(ctx context.Context, values []opggRunePage) []championRunePage {
	p.mu.Lock()
	patch := p.patch
	p.mu.Unlock()
	data, err := p.fetch(ctx, dataDragonHost, "/cdn/"+patch+"/data/zh_CN/runesReforged.json", nil, championJSONMax, "application/json")
	if err != nil {
		return nil
	}
	var styles []ddragonRuneStyle
	if json.Unmarshal(data, &styles) != nil {
		return nil
	}
	styleByID := make(map[int]ddragonRuneStyle, len(styles))
	for _, style := range styles {
		styleByID[style.ID] = style
	}
	result := make([]championRunePage, 0, min(2, len(values)))
	for _, value := range values {
		if len(value.Builds) == 0 {
			continue
		}
		build := value.Builds[0]
		selected := make(map[int]bool)
		for _, id := range append(append(append([]int{}, build.PrimaryRuneIDs...), build.SecondaryRuneIDs...), build.StatModIDs...) {
			selected[id] = true
		}
		primary, okPrimary := styleByID[build.PrimaryPageID]
		secondary, okSecondary := styleByID[build.SecondaryPageID]
		if !okPrimary || !okSecondary {
			continue
		}
		page := championRunePage{PrimaryStyle: runeStyleAsset(primary), SubStyle: runeStyleAsset(secondary), PickRate: ratePercent(value.PickRate), WinRate: percentOf(value.Win, value.Play), Games: value.Play}
		page.PrimarySlots = runeStyleSlots(primary, selected, &page.Selected)
		// 副系不显示基石行：游戏中副系只能选两个小符文。
		if subSlots := runeStyleSlots(secondary, selected, &page.Selected); len(subSlots) > 1 {
			page.SubSlots = subSlots[1:]
		} else {
			page.SubSlots = subSlots
		}
		page.ShardSlots = runeShardSlots(build.StatModIDs, &page.Selected)
		result = append(result, page)
		if len(result) == 2 {
			break
		}
	}
	return result
}

func runeStyleAsset(style ddragonRuneStyle) championAsset {
	return championAsset{ID: style.ID, Kind: "perkStyle", Name: style.Name, Source: "ddragon", Path: "/cdn/img/" + strings.TrimPrefix(style.Icon, "/")}
}

func runeStyleSlots(style ddragonRuneStyle, selected map[int]bool, chosen *[]championAsset) [][]championAsset {
	result := make([][]championAsset, 0, len(style.Slots))
	for _, slot := range style.Slots {
		row := make([]championAsset, 0, len(slot.Runes))
		for _, rune := range slot.Runes {
			asset := championAsset{ID: rune.ID, Kind: "perk", Name: rune.Name, Description: cleanMarkup(firstNonEmpty(rune.LongDesc, rune.ShortDesc)), Source: "ddragon", Path: "/cdn/img/" + strings.TrimPrefix(rune.Icon, "/"), Active: selected[rune.ID]}
			row = append(row, asset)
			if asset.Active {
				*chosen = append(*chosen, asset)
			}
		}
		result = append(result, row)
	}
	return result
}

// runeShardSlots 按行位置匹配属性碎片：stat_mod_ids 依次对应进攻、灵活、
// 防御三行，同一碎片 ID（如适应之力）会出现在多行，用 ID 集合判断会导致
// 一行点亮多个，这里必须逐行比对。
func runeShardSlots(statModIDs []int, chosen *[]championAsset) [][]championAsset {
	ids := [][]int{{5005, 5008, 5007}, {5008, 5010, 5001}, {5011, 5013, 5001}}
	result := make([][]championAsset, 0, len(ids))
	for rowIndex, slot := range ids {
		row := make([]championAsset, 0, len(slot))
		for _, id := range slot {
			path, _ := dataDragonRuneShardPath(id)
			active := rowIndex < len(statModIDs) && statModIDs[rowIndex] == id
			asset := championAsset{ID: id, Kind: "perk", Name: runeShardName(id), Description: runeShardDescription(id), Source: "ddragon", Path: path, Active: active}
			row = append(row, asset)
			if asset.Active {
				*chosen = append(*chosen, asset)
			}
		}
		result = append(result, row)
	}
	return result
}

func runeShardName(id int) string {
	return map[int]string{5001: "成长生命值", 5005: "攻击速度", 5007: "技能急速", 5008: "适应之力", 5010: "移动速度", 5011: "生命值", 5013: "韧性"}[id]
}

func (p *championProvider) structuredArenaAugments(ctx context.Context, groups []opggArenaAugmentGroup) []championMetricRow {
	metadata := make(map[int]championAugment)
	if catalog, err := p.loadAugments(ctx); err == nil {
		for _, item := range catalog.Rows {
			metadata[item.ID] = item
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	result := make([]championMetricRow, 0, 15)
	for _, group := range groups {
		for _, item := range group.Augments {
			if item.ID <= 0 {
				continue
			}
			id := item.ID
			meta, ok := metadata[id]
			if !ok || meta.ImagePath == "" {
				continue
			}
			asset := championAsset{ID: id, Kind: "aram-augment", Name: meta.Name, Description: firstNonEmpty(meta.Tooltip, meta.Description), Source: meta.ImageSource, Path: meta.ImagePath}
			result = append(result, championMetricRow{Assets: []championAsset{asset}, PickRate: ratePercent(item.PickRate), WinRate: firstPositive(ratePercent(item.WinRate), percentOf(item.Win, item.Play)), Games: item.Play})
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].PickRate > result[j].PickRate })
	if len(result) > 15 {
		result = result[:15]
	}
	return result
}

func (p *championProvider) structuredSynergies(subjectID int, values []opggSynergy) []arenaTeamComposition {
	p.mu.Lock()
	metadata := make(map[int]championMetadata, len(p.championMeta))
	for id, item := range p.championMeta {
		metadata[id] = item
	}
	p.mu.Unlock()
	subject := metadata[subjectID]
	result := make([]arenaTeamComposition, 0, min(10, len(values)))
	for _, value := range values {
		mate := metadata[value.ChampionID]
		if subject.ID == 0 || mate.ID == 0 || value.Play <= 0 {
			continue
		}
		result = append(result, arenaTeamComposition{Champions: []arenaTeamChampion{arenaTeamChampionFromMeta(subject), arenaTeamChampionFromMeta(mate)}, AveragePlacement: arenaAverage(float64(value.TotalPlace), value.Play), FirstPlaceRate: percentOf(value.FirstPlace, value.Play), PickRate: ratePercent(value.PickRate), WinRate: percentOf(value.Win, value.Play), Games: value.Play})
		if len(result) == 10 {
			break
		}
	}
	return result
}

func arenaTeamChampionFromMeta(meta championMetadata) arenaTeamChampion {
	return arenaTeamChampion{ID: meta.ID, Key: meta.Key, Name: meta.NameZH, ImageSource: meta.ImageSource, ImagePath: meta.ImagePath}
}
