package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
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
	IDs        []string         `json:"ids"`
	Play       int              `json:"play"`
	Win        int              `json:"win"`
	TotalPlace int              `json:"total_place"`
	FirstPlace int              `json:"first_place"`
	PickRate   float64          `json:"pick_rate"`
	Builds     []opggSkillBuild `json:"builds"`
}

type opggSkillBuild struct {
	Order    []string `json:"order"`
	Play     int      `json:"play"`
	Win      int      `json:"win"`
	PickRate float64  `json:"pick_rate"`
}

type opggAugmentMetric struct {
	ID         int     `json:"id"`
	Play       int     `json:"play"`
	Win        int     `json:"win"`
	TotalPlace int     `json:"total_place"`
	FirstPlace int     `json:"first_place"`
	PickRate   float64 `json:"pick_rate"`
	WinRate    float64 `json:"win_rate"`
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

var (
	// RSC serializes the percentage number as a JSON number in current pages
	// (`[54.05,"%"]`), but older payloads quoted it. Accept both forms.
	opggDepthPercentPattern = regexp.MustCompile(`"?([0-9]+(?:\.[0-9]+)?)"?\s*(?:,\s*"%"|%)`)
	opggDepthGamesPattern   = regexp.MustCompile(`"?([0-9][0-9,]*)"?\s*(?:场|,\s*"场")`)
)

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
			Positions    []struct {
				Name     string          `json:"name"`
				Stats    opggRankedStats `json:"stats"`
				Counters []opggCounter   `json:"counters"`
			} `json:"positions"`
		} `json:"summary"`
		SummonerSpells []opggMetric            `json:"summoner_spells"`
		SkillMasteries []opggSkillMetric       `json:"skill_masteries"`
		StarterItems   []opggMetric            `json:"starter_items"`
		Boots          []opggMetric            `json:"boots"`
		CoreItems      []opggMetric            `json:"core_items"`
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

type opggPositionMode int

const (
	opggPositionRequired opggPositionMode = iota
	opggPositionLiteralNone
	opggPositionOmitted
)

type opggModeSpec struct {
	APIMode       string
	Region        string
	PositionMode  opggPositionMode
	UsesTier      bool
	HasRunes      bool
	HasAugments   bool
	HasCounters   bool
	HasBanRate    bool
	HasTopPlayers bool
}

var opggModeSpecs = map[string]opggModeSpec{
	"ranked":       {APIMode: "ranked", Region: "KR", PositionMode: opggPositionRequired, UsesTier: true, HasRunes: true, HasCounters: true, HasBanRate: true, HasTopPlayers: true},
	"aram":         {APIMode: "aram", Region: "KR", PositionMode: opggPositionLiteralNone, HasRunes: true},
	"hextech-aram": {APIMode: "aram_mayhem", Region: "CN", PositionMode: opggPositionOmitted, HasRunes: false, HasAugments: true},
	"arena":        {APIMode: "arena", Region: "global", PositionMode: opggPositionOmitted, HasAugments: true, HasBanRate: true},
	"urf":          {APIMode: "urf", Region: "KR", PositionMode: opggPositionLiteralNone, HasRunes: true},
	"nexus-blitz":  {APIMode: "nexus_blitz", Region: "KR", PositionMode: opggPositionLiteralNone, HasRunes: true, HasCounters: true},
}

func normalizeInternalChampionMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "aram-mayhem" {
		return "hextech-aram"
	}
	return value
}

func (spec opggModeSpec) requestPosition(position string) string {
	switch spec.PositionMode {
	case opggPositionLiteralNone:
		return "none"
	case opggPositionOmitted:
		return ""
	default:
		normalized, err := normalizeOPGGPosition(position)
		if err != nil {
			return ""
		}
		return strings.ToUpper(normalized)
	}
}

func opggDetailRequest(mode string, championID int, position, tier string) (opggModeSpec, string, url.Values, error) {
	mode = normalizeInternalChampionMode(mode)
	spec, ok := opggModeSpecs[mode]
	if !ok || championID <= 0 {
		return opggModeSpec{}, "", nil, errors.New("unsupported OP.GG champion mode")
	}
	if mode == "hextech-aram" {
		return opggModeSpec{}, "", nil, errors.New("hextech ARAM uses the independent Hexdata and OP.GG RSC path")
	}
	requestPosition := spec.requestPosition(position)
	if spec.PositionMode == opggPositionRequired && requestPosition == "" {
		return opggModeSpec{}, "", nil, errors.New("OP.GG position is required")
	}
	requestPath := "/api/" + spec.Region + "/champions/" + spec.APIMode + "/" + strconv.Itoa(championID)
	if spec.PositionMode != opggPositionOmitted {
		requestPath += "/" + requestPosition
	}
	var query url.Values
	if spec.UsesTier {
		if strings.TrimSpace(tier) == "" {
			tier = championCounterFallbackTier
		}
		query = url.Values{"tier": {tier}}
	}
	return spec, requestPath, query, nil
}

func (p *championProvider) loadOPGGLatestVersion(ctx context.Context, spec opggModeSpec) (string, error) {
	requestPath := "/api/" + spec.Region + "/champions/" + spec.APIMode + "/versions"
	data, err := p.fetch(ctx, opggChampionHost, requestPath, nil, championJSONMax, "application/json")
	if err != nil {
		return "", err
	}
	var payload struct {
		Data []string `json:"data"`
	}
	if json.Unmarshal(data, &payload) != nil || len(payload.Data) == 0 {
		return "", errors.New("OP.GG champion version response changed")
	}
	version := strings.TrimSpace(payload.Data[0])
	if !validQQ101PatchName(version) {
		return "", errors.New("OP.GG champion version is invalid")
	}
	return version, nil
}

func (p *championProvider) applyOPGGRequestVersion(ctx context.Context, spec opggModeSpec, query url.Values) (url.Values, string, error) {
	version, err := p.loadOPGGLatestVersion(ctx, spec)
	if err != nil {
		if isCancellation(err) {
			return query, "", err
		}
		if p.diag != nil {
			p.diag(map[string]any{"event": "opgg_version_failed", "region": spec.Region, "mode": spec.APIMode, "errorKind": championProviderErrorKind(err)})
		}
		fallbackVersion := strings.TrimSpace(p.currentPatch())
		if fallbackVersion == "" {
			fallbackVersion = time.Now().UTC().Format("2006010215")
		}
		return query, "fallback-" + fallbackVersion, nil
	}
	if query == nil {
		query = make(url.Values)
	} else {
		cloned := make(url.Values, len(query)+1)
		for key, values := range query {
			cloned[key] = append([]string(nil), values...)
		}
		query = cloned
	}
	query.Set("version", version)
	return query, version, nil
}

func parsePatchVersion(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil || major < 0 || minor < 0 {
		return 0, 0, false
	}
	return major, minor, true
}

func opggPatchIsStale(currentVersion, dataVersion string) bool {
	currentMajor, currentMinor, currentOK := parsePatchVersion(currentVersion)
	dataMajor, dataMinor, dataOK := parsePatchVersion(dataVersion)
	if !currentOK || !dataOK {
		return true
	}
	if dataMajor != currentMajor {
		return dataMajor < currentMajor
	}
	return currentMinor-dataMinor > 2
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
	if championCountersNeedFallback(response.Counters) {
		// 高分段即使只返回一两条也属于样本不足；保留已有顺序，再用翡翠以上
		// 的不同英雄补足，而不是把“非空”误判为已经完整。回退只读取 OP.GG
		// 对抗字段，不能再次等待其它出装链和高场次玩家请求。
		fallback, fallbackErr := p.loadOPGGPageCounters(ctx, champion, position, championCounterFallbackTier, response.Patch)
		if fallbackErr != nil {
			fallback, fallbackErr = p.loadStructuredCounters(ctx, mode, champion, position, championCounterFallbackTier)
		}
		if fallbackErr == nil {
			if fillMissingChampionCounters(&response.Counters, fallback) {
				response.CountersTier = championCounterFallbackTier
			}
		}
		if p.diag != nil {
			event := map[string]any{
				"event": "counter_fallback_resolved", "tier": tier, "fallback_tier": championCounterFallbackTier,
				"weak": len(response.Counters.WeakAgainst), "strong": len(response.Counters.StrongAgainst),
			}
			if fallbackErr != nil {
				event["errorKind"] = championProviderErrorKind(fallbackErr)
			}
			p.diag(event)
		}
	}
	return response, nil
}

// loadStructuredCounters reuses the cached OP.GG detail payload but parses only
// matchup rows. Sparse-tier fallback must stay independent from optional item
// and top-player providers, otherwise one slow provider can leave both matchup
// columns stuck on the same single row.
func (p *championProvider) loadStructuredCounters(ctx context.Context, mode, champion, position, tier string) (championCounterSections, error) {
	mode = normalizeInternalChampionMode(mode)
	id, _, err := p.resolveChampionID(ctx, champion)
	if err != nil {
		return championCounterSections{}, err
	}
	spec, requestPath, query, err := opggDetailRequest(mode, id, position, tier)
	if err != nil || !spec.HasCounters {
		if err == nil {
			err = errors.New("OP.GG counters are unavailable for this mode")
		}
		return championCounterSections{}, err
	}
	query, dataVersion, err := p.applyOPGGRequestVersion(ctx, spec, query)
	if err != nil {
		return championCounterSections{}, err
	}
	requestPosition := spec.requestPosition(position)
	cacheKey := opggDetailCacheKey(mode, spec.Region, id, requestPosition, tier, dataVersion)
	data, _, err := p.fetchWithMetadataCacheKey(ctx, opggChampionHost, requestPath, query, championJSONMax, "application/json", cacheKey)
	if err != nil {
		return championCounterSections{}, err
	}
	var payload opggStructuredDetail
	if json.Unmarshal(data, &payload) != nil || payload.Data.Summary.ID != id {
		return championCounterSections{}, errors.New("OP.GG champion counters response changed")
	}
	counterValues := payload.Data.Counters
	return p.structuredCountersForChampion(counterValues, id), nil
}

func (p *championProvider) loadOPGGPageCounters(ctx context.Context, champion, position, tier, patch string) (championCounterSections, error) {
	slug := strings.ToLower(strings.TrimSpace(champion))
	if !championSlugPattern.MatchString(slug) {
		return championCounterSections{}, errors.New("invalid OP.GG counter champion")
	}
	pagePosition, err := normalizeOPGGPosition(position)
	if err != nil || pagePosition == "" {
		if err == nil {
			err = errors.New("OP.GG counter position is required")
		}
		return championCounterSections{}, err
	}
	requestPath := "/zh-cn/lol/champions/" + slug + "/counters/" + pagePosition
	query := url.Values{"region": {"kr"}, "tier": {tier}, "type": {"ranked"}}
	if strings.TrimSpace(patch) != "" {
		query.Set("patch", patch)
	}
	data, err := p.fetch(ctx, opggPageHost, requestPath, query, championHTMLMax, "text/html,application/xhtml+xml")
	if err != nil {
		return championCounterSections{}, err
	}
	document, err := xhtml.Parse(strings.NewReader(string(data)))
	if err != nil {
		return championCounterSections{}, errors.New("OP.GG counter page changed")
	}
	result := parseChampionCounters(document)
	// HTML rows identify opponents by slug and image URL. Hydrate local IDs so
	// downstream recommendation cards can render the same champion portraits as
	// structured data, while retaining the page's explicit direction groups.
	p.mu.Lock()
	metadataByKey := make(map[string]championMetadata, len(p.championMeta))
	for _, meta := range p.championMeta {
		metadataByKey[strings.ToLower(strings.TrimSpace(meta.Key))] = meta
		metadataByKey[strings.ToLower(strings.TrimSpace(meta.Slug))] = meta
	}
	p.mu.Unlock()
	for _, group := range []*[]championCounterRow{&result.WeakAgainst, &result.StrongAgainst} {
		for index := range *group {
			row := &(*group)[index]
			if meta := metadataByKey[strings.ToLower(strings.TrimSpace(row.Key))]; meta.ID > 0 {
				row.ChampionID = meta.ID
				if row.Name == "" {
					row.Name = meta.NameZH
				}
			}
		}
	}
	// A few OP.GG page variants include the subject itself in both direction
	// groups. It is never a meaningful matchup and, left in place, can make the
	// same champion appear as both an advantage and a disadvantage.
	subjectID, _, _ := p.resolveChampionID(ctx, slug)
	for _, group := range []*[]championCounterRow{&result.WeakAgainst, &result.StrongAgainst} {
		filtered := (*group)[:0]
		seen := make(map[string]bool, len(*group))
		for _, row := range *group {
			key := strings.ToLower(strings.TrimSpace(row.Key))
			if subjectID > 0 && row.ChampionID == subjectID || key != "" && key == slug {
				continue
			}
			identity := key
			if identity == "" && row.ChampionID > 0 {
				identity = strconv.Itoa(row.ChampionID)
			}
			if identity != "" && seen[identity] {
				continue
			}
			if identity != "" {
				seen[identity] = true
			}
			filtered = append(filtered, row)
		}
		*group = filtered
	}
	// A partially parsed page is not authoritative. Keep the structured payload
	// available as a fallback instead of replacing a complete side with one row.
	if len(result.WeakAgainst) < 2 || len(result.StrongAgainst) < 2 {
		return championCounterSections{}, errors.New("OP.GG counter page is incomplete")
	}
	if p.diag != nil {
		p.diag(map[string]any{"event": "opgg_page_counters_resolved", "champion": slug, "position": pagePosition, "tier": tier, "weak": len(result.WeakAgainst), "strong": len(result.StrongAgainst)})
	}
	return result, nil
}

func parseOPGGDepthRows(data []byte) map[int][]championMetricRow {
	expanded := expandRSCReferences(decodeNextFlight(data))
	if !strings.Contains(expanded, `"depth_4_item_`) {
		expanded = expandRSCReferences(string(data))
	}
	result := make(map[int][]championMetricRow, 3)
	for _, depth := range []int{4, 5, 6} {
		segments := rscRowSegments(expanded, "depth_"+strconv.Itoa(depth)+"_item", 5)
		seen := make(map[int]bool, len(segments))
		for _, segment := range segments {
			itemID := 0
			for _, match := range opggRSCMetaKindIDPattern.FindAllStringSubmatch(segment, -1) {
				if strings.EqualFold(match[1], "item") {
					itemID, _ = strconv.Atoi(match[2])
					break
				}
			}
			if itemID == 0 {
				for _, match := range opggRSCMetaIDKindPattern.FindAllStringSubmatch(segment, -1) {
					if strings.EqualFold(match[2], "item") {
						itemID, _ = strconv.Atoi(match[1])
						break
					}
				}
			}
			percentMatch := opggDepthPercentPattern.FindStringSubmatch(segment)
			gamesMatch := opggDepthGamesPattern.FindStringSubmatch(segment)
			if itemID <= 0 || seen[itemID] || len(percentMatch) != 2 || len(gamesMatch) < 2 {
				continue
			}
			winRate, _ := strconv.ParseFloat(percentMatch[1], 64)
			gamesText := gamesMatch[1]
			if gamesText == "" && len(gamesMatch) > 2 {
				gamesText = gamesMatch[2]
			}
			games, _ := strconv.Atoi(strings.ReplaceAll(gamesText, ",", ""))
			// A completed item can legitimately have zero wins in a tiny sample.
			// Keep that OP.GG row as long as its percentage and game count are valid.
			if winRate < 0 || winRate > 100 || games <= 0 {
				continue
			}
			seen[itemID] = true
			result[depth] = append(result[depth], championMetricRow{
				Assets:  []championAsset{{ID: itemID, Kind: "item", Name: strconv.Itoa(itemID), Source: "ddragon"}},
				WinRate: winRate, Games: games,
			})
		}
	}
	return result
}

func (p *championProvider) loadOPGGDepthRows(ctx context.Context, champion, position, tier, patch string) (map[int][]championMetricRow, time.Time, error) {
	slug := strings.ToLower(strings.TrimSpace(champion))
	pagePosition, err := normalizeOPGGPosition(position)
	if err != nil || !championSlugPattern.MatchString(slug) || pagePosition == "" {
		if err == nil {
			err = errors.New("invalid OP.GG item-depth request")
		}
		return nil, time.Time{}, err
	}
	query := url.Values{"region": {"kr"}, "tier": {tier}, "type": {"ranked"}}
	if strings.TrimSpace(patch) != "" {
		query.Set("patch", patch)
	}
	requestPath := "/zh-cn/lol/champions/" + slug + "/build/" + pagePosition + "?" + query.Encode()
	cacheKey := strings.Join([]string{"v2", "opgg-depth", slug, pagePosition, tier, patch}, "|")
	loader := func(loadCtx context.Context) ([]byte, error) { return p.fetchOPGGRSCDirect(loadCtx, requestPath) }
	var data []byte
	var fetchedAt time.Time
	if p.cache != nil {
		cached, loadErr := p.cache.loadWithStatus(ctx, cacheKey, 6*time.Hour, 24*time.Hour, true, loader)
		data, fetchedAt, err = cached.data, cached.fetchedAt, loadErr
	} else {
		data, err = loader(ctx)
		fetchedAt = time.Now()
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	depths := parseOPGGDepthRows(data)
	if len(depths[4]) == 0 || len(depths[5]) == 0 {
		return nil, time.Time{}, errors.New("OP.GG item-depth response changed")
	}
	for _, depth := range []int{4, 5, 6} {
		for rowIndex := range depths[depth] {
			for assetIndex := range depths[depth][rowIndex].Assets {
				asset := &depths[depth][rowIndex].Assets[assetIndex]
				asset.Path = p.ddragonAssetPath(asset.Kind, asset.ID)
			}
		}
	}
	if p.diag != nil {
		p.diag(map[string]any{"event": "opgg_item_depths_resolved", "champion": slug, "position": pagePosition, "tier": tier, "fourth": len(depths[4]), "fifth": len(depths[5]), "sixth": len(depths[6])})
	}
	return depths, fetchedAt, nil
}

func championItemAttempt(source string, err error) DataSourceAttempt {
	attempt := DataSourceAttempt{Source: source, Outcome: dataSourceFailed, Message: safeDiagnosticReason(err)}
	message := strings.ToLower(attempt.Message)
	switch {
	case strings.Contains(message, "disabled by feature gate"):
		attempt.Outcome = dataSourceDisabled
	case strings.Contains(message, "unsupported"):
		attempt.Outcome = dataSourceModeUnsupported
	}
	return attempt
}

func itemDepthRowsAvailable(depths map[int][]championMetricRow) bool {
	return len(depths[4])+len(depths[5])+len(depths[6]) > 0
}

func itemDepthRowsReady(depths map[int][]championMetricRow) bool {
	return len(depths[4]) > 0 && len(depths[5]) > 0
}

func mergeChampionItemDepths(preferred, fallback map[int][]championMetricRow) map[int][]championMetricRow {
	result := make(map[int][]championMetricRow, 3)
	for _, depth := range []int{4, 5, 6} {
		if len(preferred[depth]) > 0 {
			result[depth] = preferred[depth]
		} else {
			result[depth] = fallback[depth]
		}
	}
	return result
}

func applyChampionItemDepths(build *championBuildSections, depths map[int][]championMetricRow) {
	if build == nil {
		return
	}
	build.FourthItems, build.FifthItems, build.SixthItems = depths[4], depths[5], depths[6]
	build.FourthSample, build.FifthSample, build.SixthSample = 0, 0, 0
	for _, row := range build.FourthItems {
		build.FourthSample += row.Games
	}
	for _, row := range build.FifthItems {
		build.FifthSample += row.Games
	}
	for _, row := range build.SixthItems {
		build.SixthSample += row.Games
	}
}

func (p *championProvider) resolveRankedItemDepths(ctx context.Context, champion, position, tier, opggPatch string, qq101Depths map[int][]championMetricRow, qq101Patch string, qq101Err error, qq101Received, qq101Deferred bool, response *championDetailResponse) {
	if response == nil {
		return
	}
	build := &response.Build
	qq101Useful := itemDepthRowsAvailable(qq101Depths)
	qq101Ready := itemDepthRowsReady(qq101Depths)
	switch {
	case !p.featureGates.enabled(featureGateQQ101):
		build.ItemAttempts = append(build.ItemAttempts, DataSourceAttempt{Source: dataSourceQQ101, Outcome: dataSourceDisabled, Message: "QQ101 数据源已由远程开关关闭"})
		build.ItemFallback = "qq101-disabled"
	case qq101Deferred:
		build.ItemAttempts = append(build.ItemAttempts, DataSourceAttempt{Source: dataSourceQQ101, Outcome: dataSourceFailed, Message: "等待 QQ101 超过详情加载预算"})
		build.ItemFallback = "qq101-timeout"
		if p.diag != nil {
			p.diag(map[string]any{"event": "qq101_item_depths_deferred", "champion": champion, "position": position, "tier": tier, "reason": "detail-budget"})
		}
	case !qq101Received:
		build.ItemAttempts = append(build.ItemAttempts, DataSourceAttempt{Source: dataSourceQQ101, Outcome: dataSourceFailed, Message: "QQ101 未返回结果"})
		build.ItemFallback = "qq101-failed"
	case qq101Err != nil:
		build.ItemAttempts = append(build.ItemAttempts, championItemAttempt(dataSourceQQ101, qq101Err))
		build.ItemFallback = "qq101-failed"
		if championProviderErrorKind(qq101Err) == "timeout" {
			build.ItemFallback = "qq101-timeout"
		} else if build.ItemAttempts[len(build.ItemAttempts)-1].Outcome == dataSourceModeUnsupported {
			build.ItemFallback = "qq101-mode-unsupported"
		}
		if p.diag != nil && !isCancellation(qq101Err) {
			p.diag(map[string]any{"event": "qq101_item_depths_failed", "champion": champion, "position": position, "tier": tier, "patch": qq101Patch, "errorKind": championProviderErrorKind(qq101Err)})
		}
	default:
		attempt := DataSourceAttempt{Source: dataSourceQQ101, Outcome: dataSourceSuccess}
		if !qq101Ready {
			attempt.Message = "部分装备阶段没有数据"
			build.ItemFallback = "qq101-partial"
		}
		build.ItemAttempts = append(build.ItemAttempts, attempt)
		if p.diag != nil {
			p.diag(map[string]any{"event": "qq101_item_depths_resolved", "champion": champion, "position": position, "tier": tier, "patch": qq101Patch, "fourth": len(qq101Depths[4]), "fifth": len(qq101Depths[5]), "sixth": len(qq101Depths[6])})
		}
	}

	if qq101Ready {
		applyChampionItemDepths(build, qq101Depths)
		build.ItemSource = "QQ101"
		build.ItemWindow = strings.TrimSpace("国服 " + qq101Patch)
		build.ItemChainStatus = "ready"
		response.Source = "OP.GG JSON + QQ101 item depths"
		response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique, "第四至第六件优先采用腾讯掌上英雄联盟国服统计")
		return
	}

	opggDepths, depthFetchedAt, opggErr := p.loadOPGGDepthRows(ctx, champion, position, tier, opggPatch)
	if opggErr == nil {
		build.ItemAttempts = append(build.ItemAttempts, DataSourceAttempt{Source: dataSourceOPGG, Outcome: dataSourceSuccess})
		merged := mergeChampionItemDepths(qq101Depths, opggDepths)
		applyChampionItemDepths(build, merged)
		build.ItemWindow = "当前版本"
		build.ItemChainStatus = "ready"
		if qq101Useful {
			build.ItemSource = "QQ101 + OP.GG"
			response.Source = "OP.GG JSON + QQ101/OP.GG item depths"
		} else {
			build.ItemSource = "OP.GG"
			response.Source = "OP.GG JSON + OP.GG RSC item depths"
		}
		response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique, "QQ101 不可用或数据不完整时显式回退到 OP.GG 当前版本统计")
		if depthFetchedAt.After(response.FetchedAt) {
			response.FetchedAt = depthFetchedAt
		}
		return
	}

	build.ItemAttempts = append(build.ItemAttempts, championItemAttempt(dataSourceOPGG, opggErr))
	if qq101Useful {
		applyChampionItemDepths(build, qq101Depths)
		build.ItemSource = "QQ101"
		build.ItemWindow = strings.TrimSpace("国服 " + qq101Patch)
	}
	build.ItemChainStatus = "unavailable"
	response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique, "QQ101 与 OP.GG 第四至第六件装备统计均不完整，核心装仍可用")
	if p.diag != nil {
		p.diag(map[string]any{"event": "opgg_item_depths_failed", "champion": champion, "position": position, "tier": tier, "errorKind": championProviderErrorKind(opggErr)})
	}
}

func fillMissingChampionCounters(current *championCounterSections, fallback championCounterSections) bool {
	if current == nil {
		return false
	}
	seenEverywhere := make(map[int]bool, len(current.WeakAgainst)+len(current.StrongAgainst))
	seenKeys := make(map[string]bool, len(current.WeakAgainst)+len(current.StrongAgainst))
	for _, rows := range [][]championCounterRow{current.WeakAgainst, current.StrongAgainst} {
		for _, row := range rows {
			seenEverywhere[row.ChampionID] = row.ChampionID > 0
			seenKeys[strings.ToLower(strings.TrimSpace(row.Key))] = strings.TrimSpace(row.Key) != ""
		}
	}
	fill := func(rows *[]championCounterRow, candidates []championCounterRow) bool {
		seen := make(map[int]bool, len(*rows))
		for _, row := range *rows {
			seen[row.ChampionID] = true
		}
		before := len(*rows)
		for _, row := range candidates {
			if len(*rows) >= 5 {
				break
			}
			key := strings.ToLower(strings.TrimSpace(row.Key))
			if row.ChampionID <= 0 && key == "" || seen[row.ChampionID] || row.ChampionID > 0 && seenEverywhere[row.ChampionID] || key != "" && seenKeys[key] {
				continue
			}
			*rows = append(*rows, row)
			if row.ChampionID > 0 {
				seen[row.ChampionID], seenEverywhere[row.ChampionID] = true, true
			}
			if key != "" {
				seenKeys[key] = true
			}
		}
		return len(*rows) > before
	}
	weakChanged := fill(&current.WeakAgainst, fallback.WeakAgainst)
	strongChanged := fill(&current.StrongAgainst, fallback.StrongAgainst)
	return weakChanged || strongChanged
}

func championCountersNeedFallback(counters championCounterSections) bool {
	if len(counters.WeakAgainst) < 2 || len(counters.StrongAgainst) < 2 {
		return true
	}
	seen := make(map[string]bool, len(counters.WeakAgainst))
	for _, row := range counters.WeakAgainst {
		key := strings.ToLower(strings.TrimSpace(row.Key))
		if key == "" && row.ChampionID > 0 {
			key = strconv.Itoa(row.ChampionID)
		}
		seen[key] = key != ""
	}
	for _, row := range counters.StrongAgainst {
		key := strings.ToLower(strings.TrimSpace(row.Key))
		if key == "" && row.ChampionID > 0 {
			key = strconv.Itoa(row.ChampionID)
		}
		if key != "" && seen[key] {
			return true
		}
	}
	return false
}

func (p *championProvider) loadDetailOnce(ctx context.Context, mode, champion, position, tier string) (championDetailResponse, error) {
	mode = normalizeInternalChampionMode(mode)
	spec, ok := opggModeSpecs[mode]
	if !ok {
		return championDetailResponse{}, errors.New("unsupported champion detail mode")
	}
	if mode == "hextech-aram" {
		return p.loadMayhemDetail(ctx, champion)
	}
	response, err := p.loadStructuredDetail(ctx, mode, champion, position, tier)
	if err != nil {
		p.reportStructuredDrift(mode, champion, err)
		return championDetailResponse{}, err
	}
	if spec.HasTopPlayers {
		response.TopPlayers = p.loadTopPlayers(ctx, champion)
	}
	if spec.HasCounters {
		// The page has explicit "优势/劣势" groups and is the authoritative
		// direction source. Prefer it whenever available, even if the structured
		// endpoint returned a non-empty but incomplete/ambiguous array.
		if counters, counterErr := p.loadOPGGPageCounters(ctx, champion, position, tier, response.Patch); counterErr == nil {
			response.Counters = counters
			response.CountersTier = tier
		} else if championCountersNeedFallback(response.Counters) && p.diag != nil {
			p.diag(map[string]any{"event": "opgg_page_counters_fallback", "champion": champion, "position": position, "tier": tier, "errorKind": championProviderErrorKind(counterErr)})
		}
	}
	return response, nil
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

func parseOPGGLeaderboardFlight(data []byte) []championTopPlayer {
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

func opggLeaderboardRiotID(cell *xhtml.Node) (string, string) {
	const summonerPath = "/zh-cn/lol/summoners/kr/"
	for _, link := range descendantElements(cell, "a") {
		href := attribute(link, "href")
		if !strings.HasPrefix(href, summonerPath) {
			continue
		}
		name, tagline := "", ""
		for _, span := range descendantElements(link, "span") {
			value := strings.TrimSpace(nodeText(span))
			if strings.HasPrefix(value, "#") {
				tagline = strings.TrimSpace(strings.TrimPrefix(value, "#"))
			} else if name == "" && value != "" {
				name = value
			}
		}
		if name != "" && tagline != "" {
			return name, tagline
		}

		// The visible spans are the preferred source, but the semantic summoner
		// link keeps the parser useful if OP.GG changes the row wrappers.
		decoded, err := url.PathUnescape(strings.TrimPrefix(href, summonerPath))
		if err != nil {
			continue
		}
		separator := strings.LastIndex(decoded, "-")
		if separator > 0 && separator < len(decoded)-1 {
			return decoded[:separator], decoded[separator+1:]
		}
	}
	return "", ""
}

func opggLeaderboardTier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	tier := ""
	for _, candidate := range []struct {
		needles []string
		value   string
	}{
		{[]string{"最强王者", "challenger"}, "challenger"},
		{[]string{"傲世宗师", "宗师", "grandmaster"}, "grandmaster"},
		{[]string{"超凡大师", "大师", "master"}, "master"},
		{[]string{"钻石", "diamond"}, "diamond"},
		{[]string{"翡翠", "emerald"}, "emerald"},
		{[]string{"铂金", "platinum"}, "platinum"},
		{[]string{"黄金", "gold"}, "gold"},
		{[]string{"白银", "silver"}, "silver"},
		{[]string{"青铜", "bronze"}, "bronze"},
		{[]string{"黑铁", "iron"}, "iron"},
	} {
		for _, needle := range candidate.needles {
			if strings.Contains(value, needle) {
				tier = candidate.value
				break
			}
		}
		if tier != "" {
			break
		}
	}
	if tier == "" {
		return ""
	}
	division := int(firstNumber(value))
	if division >= 1 && division <= 4 && tier != "master" && tier != "grandmaster" && tier != "challenger" {
		return tier + " " + strconv.Itoa(division)
	}
	return tier
}

func opggLeaderboardWinRate(cell *xhtml.Node) float64 {
	for _, span := range descendantElements(cell, "span") {
		value := strings.TrimSpace(nodeText(span))
		if strings.Contains(value, "%") {
			return firstNumber(value)
		}
	}
	return 0
}

func parseOPGGLeaderboardTable(data []byte) []championTopPlayer {
	document, err := xhtml.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil
	}
	result := make([]championTopPlayer, 0, 5)
	for _, table := range descendantElements(document, "table") {
		if caption := firstDescendant(table, "caption"); caption == nil || strings.TrimSpace(nodeText(caption)) != "Champion Table" {
			continue
		}
		tbody := firstDescendant(table, "tbody")
		for _, row := range directElements(tbody, "tr") {
			cells := directElements(row, "td")
			if len(cells) < 7 {
				continue
			}
			rank, rankErr := strconv.Atoi(strings.TrimSpace(nodeText(cells[0])))
			name, tagline := opggLeaderboardRiotID(cells[1])
			if rankErr != nil || rank <= 0 || name == "" {
				continue
			}
			player := championTopPlayer{
				Rank: rank, Name: name, Tagline: tagline,
				Tier: opggLeaderboardTier(nodeText(cells[2])), LP: strings.TrimSpace(nodeText(cells[3])),
				Games: strings.TrimSpace(nodeText(cells[5])), WinRate: opggLeaderboardWinRate(cells[6]),
			}
			if image := firstDescendant(cells[1], "img"); image != nil {
				if parsed, parseErr := url.Parse(attribute(image, "src")); parseErr == nil && strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Hostname(), opggAssetHost) && strings.HasPrefix(parsed.Path, "/meta/images/") {
					player.IconSource, player.IconPath = "opgg", parsed.Path
				}
			}
			result = append(result, player)
			if len(result) == 5 {
				return result
			}
		}
	}
	return result
}

// loadTopPlayers 从 OP.GG 英雄专家榜读取该英雄场次最多的玩家（韩服，
// 钻二以上样本），失败时返回空列表，不影响详情主体。
func (p *championProvider) loadTopPlayers(ctx context.Context, champion string) []championTopPlayer {
	return p.loadTopPlayersForPosition(ctx, champion, "")
}

func (p *championProvider) loadTopPlayersForPosition(ctx context.Context, champion, position string) []championTopPlayer {
	query := url.Values{"region": {"kr"}}
	if position != "" {
		query.Set("position", position)
	}
	data, err := p.fetch(ctx, opggPageHost, "/zh-cn/lol/leaderboards/champions/"+champion, query, championHTMLMax, "text/html,application/xhtml+xml")
	if err != nil {
		return nil
	}
	result := parseOPGGLeaderboardTable(data)
	source := "html-table"
	if len(result) == 0 {
		result = parseOPGGLeaderboardFlight(data)
		source = "next-flight"
	}
	if p.diag != nil {
		p.diag(map[string]any{"event": "load_top_players_shape", "champion": champion, "parsed": len(result), "source": source})
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
	p.diag(map[string]any{"event": "champion_structured_fallback", "mode": mode, "error_kind": championProviderErrorKind(err)})
}

func (p *championProvider) resolveChampionID(ctx context.Context, champion string) (int, championMetadata, error) {
	lookup := strings.ToLower(strings.TrimSpace(champion))
	if numericID, err := strconv.Atoi(lookup); err == nil && numericID > 0 && numericID <= 10000 {
		p.mu.Lock()
		metadata := p.championMeta[numericID]
		p.mu.Unlock()
		return numericID, metadata, nil
	}
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
	mode = normalizeInternalChampionMode(mode)
	id, metadata, err := p.resolveChampionID(ctx, champion)
	if err != nil {
		return championDetailResponse{}, err
	}
	if strings.TrimSpace(metadata.Slug) != "" {
		champion = metadata.Slug
	}
	if mode == "ranked" {
		p.startQQ101Probe(ctx, champion, id, tier, position)
	}
	type qq101PositionsResult struct {
		positions []championPositionOption
		patch     string
		err       error
	}
	type qq101BuildResult struct {
		depths map[int][]championMetricRow
		patch  string
		err    error
	}
	var qq101PositionsC chan qq101PositionsResult
	var qq101BuildC chan qq101BuildResult
	var cancelQQ101 context.CancelFunc
	var qq101Deadline time.Time
	if mode == "ranked" && p.featureGates.enabled(featureGateQQ101) {
		waitBudget := p.qq101Wait
		if waitBudget <= 0 {
			waitBudget = qq101DetailWait
		}
		qq101Deadline = time.Now().Add(waitBudget)
		qq101Ctx, cancel := context.WithDeadline(ctx, qq101Deadline)
		cancelQQ101 = cancel
		defer cancel()
		qq101PositionsC = make(chan qq101PositionsResult, 1)
		go func() {
			positions, patch, loadErr := p.loadQQ101Positions(qq101Ctx, id, tier)
			qq101PositionsC <- qq101PositionsResult{positions: positions, patch: patch, err: loadErr}
		}()
		qq101BuildC = make(chan qq101BuildResult, 1)
		go func() {
			depths, patch, loadErr := p.loadQQ101BuildRows(qq101Ctx, id, tier, position)
			qq101BuildC <- qq101BuildResult{depths: depths, patch: patch, err: loadErr}
		}()
	}
	spec, requestPath, query, err := opggDetailRequest(mode, id, position, tier)
	if err != nil {
		return championDetailResponse{}, err
	}
	query, dataVersion, err := p.applyOPGGRequestVersion(ctx, spec, query)
	if err != nil {
		return championDetailResponse{}, err
	}
	requestPosition := spec.requestPosition(position)
	cacheKey := opggDetailCacheKey(mode, spec.Region, id, requestPosition, tier, dataVersion)
	data, fetchedAt, err := p.fetchWithMetadataCacheKey(ctx, opggChampionHost, requestPath, query, championJSONMax, "application/json", cacheKey)
	if err != nil {
		return championDetailResponse{}, err
	}
	var payload opggStructuredDetail
	if json.Unmarshal(data, &payload) != nil || payload.Data.Summary.ID != id {
		return championDetailResponse{}, errors.New("OP.GG champion detail response changed")
	}
	currentVersion := p.currentPatch()
	response := championDetailResponse{
		Mode: mode, Region: spec.Region, Tier: tier, Position: strings.ToLower(requestPosition), Patch: payload.Meta.Version,
		CurrentPatch: currentVersion, IsStale: opggPatchIsStale(currentVersion, payload.Meta.Version),
		Source: "OP.GG JSON", FetchedAt: fetchedAt, EntertainmentSample: spec.HasAugments,
	}
	stats := payload.Data.Summary.AverageStats
	response.Stats = championDetailStats{
		WinRate:  firstPositive(ratePercent(stats.WinRate), percentOf(stats.Win, stats.Play)),
		PickRate: ratePercent(stats.PickRate), BanRate: ratePercent(stats.BanRate),
	}
	if spec.PositionMode == opggPositionRequired {
		response.Positions = make([]championPositionOption, 0, len(payload.Data.Summary.Positions))
		for _, raw := range payload.Data.Summary.Positions {
			positionName := strings.ToLower(strings.TrimSpace(raw.Name))
			upstreamName, ok := championPositionNames[positionName]
			if !ok || upstreamName == "" {
				continue
			}
			tierValue, rankValue := opggTierRank(raw.Stats)
			response.Positions = append(response.Positions, championPositionOption{
				Position: positionName,
				WinRate:  ratePercent(raw.Stats.WinRate),
				PickRate: ratePercent(raw.Stats.PickRate),
				BanRate:  ratePercent(raw.Stats.BanRate),
				RoleRate: ratePercent(raw.Stats.RoleRate),
				Play:     raw.Stats.Play,
				Tier:     tierValue,
				Rank:     rankValue,
			})
		}
	}
	var qq101Positions *qq101PositionsResult
	var qq101Build *qq101BuildResult
	var positionsDeferred, buildDeferred bool
	if qq101PositionsC != nil || qq101BuildC != nil {
		remaining := time.Until(qq101Deadline)
		if remaining < 0 {
			remaining = 0
		}
		timer := time.NewTimer(remaining)
		budgetExpired := false
		for qq101PositionsC != nil || qq101BuildC != nil {
			// Prefer results already produced at the deadline over the timer signal.
			if qq101PositionsC != nil {
				select {
				case official := <-qq101PositionsC:
					qq101Positions = &official
					qq101PositionsC = nil
					continue
				default:
				}
			}
			if qq101BuildC != nil {
				select {
				case build := <-qq101BuildC:
					qq101Build = &build
					qq101BuildC = nil
					continue
				default:
				}
			}
			select {
			case official := <-qq101PositionsC:
				qq101Positions = &official
				qq101PositionsC = nil
			case build := <-qq101BuildC:
				qq101Build = &build
				qq101BuildC = nil
			case <-timer.C:
				positionsDeferred = qq101PositionsC != nil
				buildDeferred = qq101BuildC != nil
				qq101PositionsC = nil
				qq101BuildC = nil
				budgetExpired = true
			case <-ctx.Done():
				if !budgetExpired {
					timer.Stop()
				}
				return championDetailResponse{}, ctx.Err()
			}
		}
		if !budgetExpired {
			timer.Stop()
		}
		cancelQQ101()
	}
	if qq101Positions != nil {
		if qq101Positions.err == nil {
			response.Positions = mergeQQ101PositionShares(response.Positions, qq101Positions.positions)
			response.PositionsSource = "QQ101"
			response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique, "分路占比采用腾讯掌上英雄联盟官方统计")
			if p.diag != nil {
				p.diag(map[string]any{"event": "qq101_positions_resolved", "champion": champion, "tier": tier, "patch": qq101Positions.patch, "positions": len(qq101Positions.positions)})
			}
		} else if p.diag != nil && !isCancellation(qq101Positions.err) {
			p.diag(map[string]any{"event": "qq101_positions_failed", "champion": champion, "tier": tier, "patch": qq101Positions.patch, "errorKind": championProviderErrorKind(qq101Positions.err)})
		}
	} else if positionsDeferred && p.diag != nil {
		p.diag(map[string]any{"event": "qq101_positions_deferred", "champion": champion, "tier": tier, "reason": "detail-budget"})
	}
	response.Build.Skills = p.structuredSkills(payload.Data.SkillMasteries)
	if spec.HasAugments {
		response.Build.Boots = p.structuredMetricsForKind(payload.Data.Boots, "item", "boots", 7)
		response.Build.CoreItems = p.structuredMetricsForKind(payload.Data.CoreItems, "item", "core", 15)
		response.Build.PrismItems = p.structuredMetricsForKind(payload.Data.PrismItems, "item", "prism", 10)
	} else {
		// 这三类直接采用 OP.GG 的推荐顺序。前端常规各展示两条，第三条仅用于
		// 某一类不足两条时补齐侧栏总计六条，不再用本地样本门槛删掉真实推荐。
		response.Build.SummonerSpells = p.structuredRecommendationMetrics(payload.Data.SummonerSpells, "spell", "spell", 3)
		response.Build.StarterItems = p.structuredRecommendationMetrics(payload.Data.StarterItems, "item", "starter", 3)
		response.Build.Boots = p.structuredRecommendationMetrics(payload.Data.Boots, "item", "boots", 3)
		// 核心装是 OP.GG 已排序的推荐列表。保留完整的上游 15 条，
		// 折叠展示数量由消费端决定，不能在数据投影层提前丢行。
		response.Build.CoreItems = p.structuredRecommendationMetrics(payload.Data.CoreItems, "item", "core", championCoreRecommendationLimit)
		if mode == "ranked" {
			var qq101Depths map[int][]championMetricRow
			var qq101Patch string
			var qq101Err error
			if qq101Build != nil {
				qq101Depths, qq101Patch, qq101Err = qq101Build.depths, qq101Build.patch, qq101Build.err
			}
			p.resolveRankedItemDepths(ctx, metadata.Slug, position, tier, payload.Meta.Version, qq101Depths, qq101Patch, qq101Err, qq101Build != nil, buildDeferred, &response)
		} else {
			depths, depthFetchedAt, depthErr := p.loadOPGGDepthRows(ctx, metadata.Slug, position, tier, payload.Meta.Version)
			response.Build.ItemSource = "OP.GG"
			response.Build.ItemWindow = "当前版本"
			if depthErr == nil {
				applyChampionItemDepths(&response.Build, depths)
				response.Build.ItemChainStatus = "ready"
				response.Source = "OP.GG JSON + OP.GG RSC item depths"
				if depthFetchedAt.After(response.FetchedAt) {
					response.FetchedAt = depthFetchedAt
				}
			} else {
				response.Build.ItemChainStatus = "unavailable"
				response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique, "OP.GG 当前版本第四至第六件装备统计暂不可用，核心装仍可用")
				if p.diag != nil {
					p.diag(map[string]any{"event": "opgg_item_depths_failed", "champion": metadata.Slug, "position": position, "tier": tier, "errorKind": championProviderErrorKind(depthErr)})
				}
			}
		}
	}
	if spec.HasRunes {
		response.Runes = p.structuredRunes(ctx, payload.Data.RunePages)
	}
	if spec.HasCounters {
		response.Counters = p.structuredCountersForChampion(payload.Data.Counters, id)
	}
	if spec.HasAugments {
		response.ArenaStats = arenaChampionStats{
			Tier: stats.Tier, Rank: firstPositiveInt(stats.TierData.Rank, stats.Rank), RankPrevPatch: stats.TierData.RankPrevPatch,
			Games: stats.Play, KDA: stats.KDA,
			AveragePlacement: arenaAverage(float64(stats.TotalPlace), stats.Play), FirstPlaceRate: percentOf(stats.FirstPlace, stats.Play),
			PickRate: ratePercent(stats.PickRate), WinRate: percentOf(stats.Win, stats.Play), BanRate: ratePercent(stats.BanRate),
		}
		response.ArenaAugmentGroups = p.structuredArenaAugmentGroups(ctx, payload.Data.AugmentGroup)
		applyLocalArenaAugmentGrades(response.ArenaAugmentGroups)
		// Preserve every quality group for the live page; the client applies a
		// per-quality display limit so no rarity disappears due to global sorting.
		response.ArenaAugments = flattenArenaAugmentGroups(response.ArenaAugmentGroups, 0)
		response.TeamCompositions = p.structuredSynergies(id, payload.Data.Synergies)
		aggregate, itemCatalog, aggregateFetchedAt, aggregateErr := p.loadArenaChampionAggregate(ctx, id)
		if aggregateErr == nil {
			coreItems := mapYourGGArenaAggregateItems(aggregate.Response.CoreItems, itemCatalog)
			prismItems := mapYourGGArenaAggregateItems(aggregate.Response.PrismaticItems, itemCatalog)
			if len(coreItems) == 0 && len(prismItems) == 0 {
				aggregateErr = errors.New("YOUR.GG aggregate item sections are empty")
			} else {
				if len(coreItems) > 0 {
					response.Build.CoreItems = coreItems
				}
				if len(prismItems) > 0 {
					response.Build.PrismItems = prismItems
				}
				response.Source = "OP.GG JSON + YOUR.GG aggregate"
				response.FetchedAt = aggregateFetchedAt
				response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique, "核心物品与棱彩装备采用 YOUR.GG 聚合数据；胜率、选用率、吃鸡率由 0~1 小数换算为百分比，等级沿用上游 OP/S/A/B/C/D/F")
			}
		}
		if aggregateErr != nil {
			applyLocalAugmentGrades(response.Build.CoreItems)
			applyLocalAugmentGrades(response.Build.PrismItems)
			response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique, "YOUR.GG 核心物品聚合暂不可用，核心物品回退 OP.GG 并使用本地综合评分分位")
			if p.diag != nil {
				p.diag(map[string]any{"event": "arena_aggregate_fallback", "source": "your.gg", "errorKind": championProviderErrorKind(aggregateErr)})
			}
		}
		response.Build = normalizeArenaBuild(response.Build)
	}
	p.decorateDetailAssets(ctx, champion, &response)
	if len(response.Build.CoreItems) == 0 && len(response.Build.Boots) == 0 && len(response.Build.Skills) == 0 {
		return championDetailResponse{}, errors.New("OP.GG structured detail is incomplete")
	}
	return response, nil
}

func (p *championProvider) structuredMetrics(values []opggMetric, assetKind string, limit int) []championMetricRow {
	return p.structuredMetricsForKind(values, assetKind, assetKind, limit)
}

type structuredMetricCandidate struct {
	row   championMetricRow
	games int
}

func (p *championProvider) structuredMetricCandidate(value opggMetric, assetKind string) (structuredMetricCandidate, bool) {
	if len(value.IDs) == 0 || value.Play <= 0 {
		return structuredMetricCandidate{}, false
	}
	assets := make([]championAsset, 0, len(value.IDs))
	for _, id := range value.IDs {
		if id <= 0 {
			continue
		}
		assets = append(assets, championAsset{ID: id, Kind: assetKind, Name: strconv.Itoa(id), Source: "ddragon", Path: p.ddragonAssetPath(assetKind, id)})
	}
	if len(assets) == 0 {
		return structuredMetricCandidate{}, false
	}
	return structuredMetricCandidate{row: championMetricRow{
		Assets: assets, PickRate: ratePercent(value.PickRate), WinRate: firstPositive(ratePercent(value.WinRate), percentOf(value.Win, value.Play)), Games: value.Play,
		AveragePlacement: arenaAverage(float64(value.TotalPlace), value.Play), FirstPlaceRate: percentOf(value.FirstPlace, value.Play),
	}, games: value.Play}, true
}

func (p *championProvider) structuredRecommendationMetrics(values []opggMetric, assetKind, diagnosticKind string, limit int) []championMetricRow {
	result := make([]championMetricRow, 0, min(limit, len(values)))
	for _, value := range values {
		candidate, ok := p.structuredMetricCandidate(value, assetKind)
		if !ok {
			continue
		}
		result = append(result, candidate.row)
		if len(result) == limit {
			break
		}
	}
	if p.diag != nil {
		p.diag(map[string]any{"event": "structured_recommendations", "kind": diagnosticKind, "in": len(values), "returned": len(result)})
	}
	return result
}

// structuredMetricsForKind keeps the established absolute and relative sample
// gate, but does not hide an entire recommendation section when every upstream
// row falls below it. The fallback remains ordered by sample size.
func (p *championProvider) structuredMetricsForKind(values []opggMetric, assetKind, gateKind string, limit int) []championMetricRow {
	leadingSample := 0
	for _, value := range values {
		if value.Play > leadingSample {
			leadingSample = value.Play
		}
	}
	candidates := make([]structuredMetricCandidate, 0, min(limit, len(values)))
	passed := make([]structuredMetricCandidate, 0, min(limit, len(values)))
	droppedBelowFloor := 0
	for _, value := range values {
		if value.Play < structuredMetricMinimumGames {
			droppedBelowFloor++
		}
		candidate, ok := p.structuredMetricCandidate(value, assetKind)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
		if structuredSampleAllowedForKind(gateKind, value.Play, leadingSample) {
			passed = append(passed, candidate)
		}
	}
	resultCandidates := passed
	usedFallback := len(resultCandidates) == 0 && len(candidates) > 0
	if usedFallback {
		resultCandidates = append([]structuredMetricCandidate(nil), candidates...)
		sort.SliceStable(resultCandidates, func(i, j int) bool { return resultCandidates[i].games > resultCandidates[j].games })
	}
	result := make([]championMetricRow, 0, min(limit, len(resultCandidates)))
	for _, candidate := range resultCandidates {
		result = append(result, candidate.row)
		if len(result) == limit {
			break
		}
	}
	if p.diag != nil {
		p.diag(map[string]any{
			"event": "structured_metrics_gate", "kind": gateKind, "in": len(values), "out": len(passed),
			"leading": leadingSample, "dropped_below_floor": droppedBelowFloor, "returned": len(result), "fallback": usedFallback,
		})
	}
	return result
}

const (
	// Tiny long-tail samples are not actionable recommendations. Requiring both
	// an absolute floor and a share of the leading row keeps the gate stable
	// across popular and niche modes.
	structuredMetricMinimumGames = 50
	structuredMetricHeadRatio    = 0.01
	structuredSpellMinimumGames  = 15
	structuredSpellHeadRatio     = 0.0
)

func structuredSampleAllowed(play, leadingSample int) bool {
	return play >= structuredMetricMinimumGames && leadingSample > 0 && float64(play)/float64(leadingSample) >= structuredMetricHeadRatio
}

func structuredSampleAllowedForKind(kind string, play, leadingSample int) bool {
	if kind == "spell" {
		return play >= structuredSpellMinimumGames && (structuredSpellHeadRatio == 0 || leadingSample > 0 && float64(play)/float64(leadingSample) >= structuredSpellHeadRatio)
	}
	return structuredSampleAllowed(play, leadingSample)
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
			result = append(result, championMetricRow{
				Assets: assets, SkillPriority: priority, SkillOrder: order, PickRate: ratePercent(pick), WinRate: percentOf(win, play), Games: play,
				AveragePlacement: arenaAverage(float64(value.TotalPlace), value.Play), FirstPlaceRate: percentOf(value.FirstPlace, value.Play),
			})
		}
		if len(result) == 3 {
			break
		}
	}
	return result
}

func arenaAverage(value float64, total int) float64 {
	if value <= 0 || total <= 0 {
		return 0
	}
	return value / float64(total)
}

func (p *championProvider) structuredCounters(values []opggCounter) championCounterSections {
	return p.structuredCountersForChampion(values, 0)
}

func (p *championProvider) structuredCountersForChampion(values []opggCounter, subjectID int) championCounterSections {
	rows := make([]championCounterRow, 0, len(values))
	droppedNoMeta, droppedNoPlay, droppedSubject, droppedDuplicate := 0, 0, 0, 0
	seenIDs := make(map[int]bool, len(values))
	p.mu.Lock()
	metadata := make(map[int]championMetadata, len(p.championMeta))
	for id, item := range p.championMeta {
		metadata[id] = item
	}
	p.mu.Unlock()
	for _, value := range values {
		if subjectID > 0 && value.ChampionID == subjectID {
			droppedSubject++
			continue
		}
		if value.ChampionID > 0 && seenIDs[value.ChampionID] {
			droppedDuplicate++
			continue
		}
		meta := metadata[value.ChampionID]
		if meta.ID == 0 {
			droppedNoMeta++
			continue
		}
		if value.Play <= 0 {
			droppedNoPlay++
			continue
		}
		seenIDs[value.ChampionID] = true
		rows = append(rows, championCounterRow{ChampionID: value.ChampionID, Key: meta.Key, Name: meta.NameZH, ImageSource: meta.ImageSource, ImagePath: meta.ImagePath, WinRate: percentOf(value.Win, value.Play), Games: value.Play})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].WinRate < rows[j].WinRate })
	result := championCounterSections{}
	weakCount := min(5, (len(rows)+1)/2)
	strongCount := min(5, len(rows)/2)
	if len(rows) == 1 {
		weakCount, strongCount = 0, 0
		if rows[0].WinRate < 50 {
			weakCount = 1
		} else if rows[0].WinRate > 50 {
			strongCount = 1
		}
	}
	result.WeakAgainst = append(result.WeakAgainst, rows[:weakCount]...)
	for index := len(rows) - 1; index >= len(rows)-strongCount; index-- {
		result.StrongAgainst = append(result.StrongAgainst, rows[index])
	}
	if p.diag != nil {
		p.diag(map[string]any{"event": "counters_shape", "in": len(values), "out": len(rows), "dropped_no_meta": droppedNoMeta, "dropped_no_play": droppedNoPlay, "dropped_subject": droppedSubject, "dropped_duplicate": droppedDuplicate})
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

func (p *championProvider) structuredArenaAugmentGroups(ctx context.Context, groups []opggArenaAugmentGroup) []arenaAugmentGroup {
	catalog, err := p.loadCommunityDragonAugments(ctx)
	if err != nil {
		if p.diag != nil {
			p.diag(map[string]any{"event": "arena_augment_catalog_failed", "source": "communitydragon", "errorKind": championProviderErrorKind(err)})
		}
		catalog = nil
	}
	result := arenaAugmentGroups(groups, catalog)
	if p.diag != nil {
		unknown := 0
		for _, group := range result {
			for _, row := range group.Rows {
				if row.Rarity == "unknown" {
					unknown++
				}
			}
		}
		if unknown > 0 {
			p.diag(map[string]any{"event": "arena_augment_rarity_unknown", "rows": unknown})
		}
	}
	if p.diag != nil {
		metadata := make(map[int64]gameplayAugment, len(catalog))
		for _, item := range catalog {
			metadata[item.ID] = item
		}
		augmentsIn, rowsOut, missingMeta := 0, 0, 0
		for _, group := range groups {
			augmentsIn += len(group.Augments)
			for _, item := range group.Augments {
				if item.ID <= 0 {
					continue
				}
				meta, ok := metadata[int64(item.ID)]
				if !ok || communityDragonGameAssetPath(meta.IconPath) == "" || strings.TrimSpace(meta.Name) == "" {
					missingMeta++
				}
			}
		}
		for _, group := range result {
			rowsOut += len(group.Rows)
		}
		p.diag(map[string]any{"event": "arena_augments_built", "groupsIn": len(groups), "augmentsIn": augmentsIn, "groupsOut": len(result), "rowsOut": rowsOut, "missingMeta": missingMeta})
	}
	return result
}

func (p *championProvider) loadCommunityDragonAugments(ctx context.Context) ([]gameplayAugment, error) {
	data, err := p.fetch(ctx, communityDragonHost, "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json", nil, 1<<20, "application/json")
	if err != nil {
		return nil, err
	}
	var raw []gameplayAugmentRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	base := normalizeGameplayAugments(raw)
	arenaData, arenaErr := p.fetch(ctx, communityDragonHost, "/latest/cdragon/arena/zh_cn.json", nil, 1<<20, "application/json")
	if arenaErr != nil {
		p.reportMayhemAugmentCatalogShape(base)
		return base, nil
	}
	var arenaPayload struct {
		Augments []struct {
			ID           int64                      `json:"id"`
			Name         string                     `json:"name"`
			Desc         string                     `json:"desc"`
			Tooltip      string                     `json:"tooltip"`
			IconLarge    string                     `json:"iconLarge"`
			Rarity       int                        `json:"rarity"`
			DataValues   map[string]json.RawMessage `json:"dataValues"`
			Calculations map[string]json.RawMessage `json:"calculations"`
		} `json:"augments"`
	}
	if json.Unmarshal(arenaData, &arenaPayload) != nil {
		p.reportMayhemAugmentCatalogShape(base)
		return base, nil
	}
	supplement := make([]gameplayAugment, 0, len(arenaPayload.Augments))
	for _, item := range arenaPayload.Augments {
		if item.ID <= 0 {
			continue
		}
		iconPath := strings.TrimSpace(strings.ReplaceAll(item.IconLarge, "\\", "/"))
		if !strings.HasPrefix(strings.ToLower(iconPath), "assets/") || strings.Contains(iconPath, "..") {
			iconPath = ""
		} else {
			iconPath = "/lol-game-data/assets/" + iconPath
		}
		iconPath, fallbackIconPath := normalizeAugmentIconPaths(iconPath)
		description := renderArenaAugmentDescription(item.Desc, item.DataValues, item.Calculations)
		if description == "" {
			description = renderArenaAugmentDescription(item.Tooltip, item.DataValues, item.Calculations)
		}
		supplement = append(supplement, gameplayAugment{
			ID: item.ID, Name: strings.TrimSpace(item.Name), Description: description,
			Rarity: arenaCDragonRarity(item.Rarity), IconPath: iconPath, FallbackIconPath: fallbackIconPath,
		})
	}
	catalog := mergeGameplayAugmentMetadata(base, supplement)
	p.reportMayhemAugmentCatalogShape(catalog)
	return catalog, nil
}

var (
	arenaIconPlaceholderPattern     = regexp.MustCompile(`%i:[^%]*%`)
	arenaValuePlaceholderPattern    = regexp.MustCompile(`@([A-Za-z][A-Za-z0-9_]*)(?:\s*\*\s*([0-9.]+))?@`)
	arenaDescriptionResidualPattern = regexp.MustCompile(`%i:|@[A-Za-z][A-Za-z0-9_.: ]{0,80}@`)
	arenaTemplatePattern            = regexp.MustCompile(`\{\{[^{}]*\}\}`)
	arenaLineBreakPattern           = regexp.MustCompile(`(?i)<br\s*/?>`)
	// Cross-augment references such as @spell.Augment_ShadowRunner:MSAmount@
	// point at another augment's spell block, which this payload does not ship.
	// The live client's per-round counters (@f1@, @f2@) are unresolvable for the
	// same reason and are pruned by the same sentence rule.
	arenaSpellReferencePattern = regexp.MustCompile(`@[A-Za-z][A-Za-z0-9_.: *]{0,80}@`)
	arenaCJKSpacePattern       = regexp.MustCompile(`([\p{Han}：，。、（）])[ \t]+|[ \t]+([\p{Han}：，。、（）])`)
)

// Riot's mStat enum as used by Arena augment calculations. Each entry below is
// pinned by the surrounding Chinese copy of the augment that uses it (for
// example 会心防守 scales 会心防御 by CritDefendRatio × mStat 9 = 暴击几率).
// Unknown stats deliberately fall through as unresolvable rather than guess.
var arenaCalculationStatNames = map[int]string{
	0:  "法术强度",
	2:  "攻击力",
	7:  "移动速度",
	9:  "暴击几率",
	11: "技能急速",
	12: "最大生命值",
	34: "治疗和护盾强度",
}

// Keyword references CommunityDragon leaves unexpanded in the localized dump.
var arenaTemplateKeywords = map[string]string{"item_keyword_onhit": "攻击特效"}

const (
	// Placeholder that survives every substitution attempt. Sentences carrying
	// it are dropped so the UI never shows "获得护盾值" with the number missing.
	arenaUnresolvedMarker = "\x00"
	// Arena characters cap at level 18, so per-level formulas are rendered as
	// a level 1 ~ level 18 range exactly like the in-game tooltip.
	arenaMaximumChampionLevel = 18
)

const communityDragonMayhemMinimumAugments = 120

// Icon directories are shared assets, so IDs are the safe boundary for this
// diagnostic. Report a catalog shrink before metadata silently disappears.
func (p *championProvider) reportMayhemAugmentCatalogShape(catalog []gameplayAugment) {
	count := 0
	for _, item := range catalog {
		if item.ID >= 1000 {
			count++
		}
	}
	if count >= communityDragonMayhemMinimumAugments || p.diag == nil {
		return
	}
	p.diag(map[string]any{
		"event":         "communitydragon_mayhem_catalog_incomplete",
		"actual":        count,
		"minimum":       communityDragonMayhemMinimumAugments,
		"discriminator": "id_ge_1000",
	})
}

// renderArenaAugmentDescription turns CommunityDragon's Arena augment template
// into display copy. Numbers come from dataValues when the key is present and
// from the sibling calculations map otherwise — 13 of the placeholders used by
// desc/tooltip exist only as GameCalculation formulas, and dropping them
// silently is what left sentences like "获得护盾值" without a number.
func renderArenaAugmentDescription(value string, dataValues, calculations map[string]json.RawMessage) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	value = arenaLineBreakPattern.ReplaceAllString(value, "\n")
	value = arenaTemplatePattern.ReplaceAllStringFunc(value, func(token string) string {
		key := strings.ToLower(strings.TrimSpace(strings.Trim(token, "{}")))
		return arenaTemplateKeywords[key]
	})
	value = arenaIconPlaceholderPattern.ReplaceAllString(value, "")
	value = arenaValuePlaceholderPattern.ReplaceAllStringFunc(value, func(token string) string {
		return arenaPlaceholderText(token, dataValues, calculations)
	})
	value = arenaSpellReferencePattern.ReplaceAllString(value, arenaUnresolvedMarker)
	value = arenaDropUnresolvedSentences(value)
	return arenaTidySpacing(cleanMarkup(value))
}

func arenaPlaceholderText(token string, dataValues, calculations map[string]json.RawMessage) string {
	match := arenaValuePlaceholderPattern.FindStringSubmatch(token)
	if len(match) < 3 {
		return arenaUnresolvedMarker
	}
	name := match[1]
	if strings.HasSuffix(name, "KeyBind") {
		if key := map[string]string{"spell1KeyBind": "Q", "spell2KeyBind": "W", "spell3KeyBind": "E"}[name]; key != "" {
			return key
		}
		return arenaUnresolvedMarker
	}
	multiplier := 1.0
	if match[2] != "" {
		parsed, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			return arenaUnresolvedMarker
		}
		multiplier = parsed
	}
	if number, ok := arenaDataValueNumber(dataValues[name]); ok {
		return formatArenaValue(number * multiplier)
	}
	if text := arenaDataValueString(dataValues[name]); text != "" {
		return text
	}
	if text := arenaCalculationText(calculations[name], dataValues, multiplier); text != "" {
		return text
	}
	return arenaUnresolvedMarker
}

// arenaDropUnresolvedSentences removes only the sentence that still carries an
// unresolvable placeholder, keeping the rest of the paragraph intact.
func arenaDropUnresolvedSentences(value string) string {
	if !strings.Contains(value, arenaUnresolvedMarker) {
		return value
	}
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if !strings.Contains(line, arenaUnresolvedMarker) {
			continue
		}
		var rebuilt strings.Builder
		for _, sentence := range arenaSentences(line) {
			if strings.Contains(sentence, arenaUnresolvedMarker) {
				continue
			}
			rebuilt.WriteString(sentence)
		}
		lines[index] = rebuilt.String()
	}
	return strings.Join(lines, "\n")
}

func arenaSentences(line string) []string {
	result := make([]string, 0, 4)
	start := 0
	for index, symbol := range line {
		switch symbol {
		case '。', '！', '？', '；':
			end := index + utf8.RuneLen(symbol)
			result = append(result, line[start:end])
			start = end
		}
	}
	if start < len(line) {
		result = append(result, line[start:])
	}
	return result
}

// Removing an icon placeholder leaves a stray ASCII space between Chinese
// glyphs ("获得 1500金币"); collapse those without touching Latin runs.
func arenaTidySpacing(value string) string {
	for {
		replaced := arenaCJKSpacePattern.ReplaceAllString(value, "$1$2")
		if replaced == value {
			return value
		}
		value = replaced
	}
}

type arenaValueTerm struct {
	low     float64
	high    float64
	isRange bool
}

type arenaScaleTerm struct {
	coefficient float64
	label       string
}

// arenaCalculationText renders a GameCalculation the way the client does:
// flat terms first, champion-stat scaling appended as "(+X%属性)". It returns
// an empty string whenever any part depends on live game state (buff counters)
// or on an unmapped stat, so the caller can drop the sentence instead of
// printing a half-formed number.
func arenaCalculationText(raw json.RawMessage, dataValues map[string]json.RawMessage, multiplier float64) string {
	if len(raw) == 0 {
		return ""
	}
	var calculation map[string]json.RawMessage
	if json.Unmarshal(raw, &calculation) != nil {
		return ""
	}
	var parts []json.RawMessage
	if json.Unmarshal(calculation["mFormulaParts"], &parts) != nil || len(parts) == 0 {
		return ""
	}
	values, scales, ok := arenaCalculationTerms(parts, dataValues, 0)
	if !ok {
		return ""
	}
	if factor, has := arenaCalculationNumberPart(calculation["mMultiplier"]); has {
		multiplier *= factor
	}
	percent := false
	if json.Unmarshal(calculation["mDisplayAsPercent"], &percent) == nil && percent {
		multiplier *= 100
	} else {
		percent = false
	}
	texts := make([]string, 0, len(values))
	for _, term := range values {
		low, high := term.low*multiplier, term.high*multiplier
		if !term.isRange || formatArenaValue(low) == formatArenaValue(high) {
			if !arenaNegligible(low) {
				texts = append(texts, formatArenaValue(low))
			}
			continue
		}
		texts = append(texts, formatArenaValue(low)+"~"+formatArenaValue(high))
	}
	result := strings.Join(texts, "+")
	if percent && result != "" {
		result += "%"
	}
	tail := make([]string, 0, len(scales))
	for _, scale := range scales {
		share := scale.coefficient * 100
		if arenaNegligible(share) {
			continue
		}
		tail = append(tail, formatArenaValue(share)+"%"+scale.label)
	}
	if result == "" && len(tail) > 0 {
		result, tail = tail[0], tail[1:]
	}
	for _, item := range tail {
		result += "(+" + item + ")"
	}
	return result
}

func arenaCalculationTerms(parts []json.RawMessage, dataValues map[string]json.RawMessage, depth int) ([]arenaValueTerm, []arenaScaleTerm, bool) {
	if depth > 3 {
		return nil, nil, false
	}
	values := make([]arenaValueTerm, 0, len(parts))
	scales := make([]arenaScaleTerm, 0, len(parts))
	for _, raw := range parts {
		var part map[string]json.RawMessage
		if json.Unmarshal(raw, &part) != nil {
			return nil, nil, false
		}
		var kind string
		_ = json.Unmarshal(part["__type"], &kind)
		switch kind {
		case "NamedDataValueCalculationPart":
			var key string
			_ = json.Unmarshal(part["mDataValue"], &key)
			number, ok := arenaDataValueNumber(dataValues[key])
			if !ok {
				return nil, nil, false
			}
			values = append(values, arenaValueTerm{low: number})
		case "NumberCalculationPart":
			number, ok := arenaRawFloat(part["mNumber"])
			if !ok {
				return nil, nil, false
			}
			values = append(values, arenaValueTerm{low: number})
		case "ByCharLevelInterpolationCalculationPart":
			start, startOK := arenaRawFloat(part["mStartValue"])
			end, endOK := arenaRawFloat(part["mEndValue"])
			if !startOK || !endOK {
				return nil, nil, false
			}
			values = append(values, arenaValueTerm{low: start, high: end, isRange: true})
		case "ByCharLevelBreakpointsCalculationPart":
			start, end, ok := arenaBreakpointRange(part)
			if !ok {
				return nil, nil, false
			}
			values = append(values, arenaValueTerm{low: start, high: end, isRange: true})
		case "StatByCoefficientCalculationPart":
			label, labelOK := arenaCalculationStatLabel(part)
			coefficient, has := arenaRawFloat(part["mCoefficient"])
			if !labelOK || !has {
				return nil, nil, false
			}
			scales = append(scales, arenaScaleTerm{coefficient: coefficient, label: label})
		case "StatByNamedDataValueCalculationPart":
			label, labelOK := arenaCalculationStatLabel(part)
			var key string
			_ = json.Unmarshal(part["mDataValue"], &key)
			coefficient, has := arenaDataValueNumber(dataValues[key])
			if !labelOK || !has {
				return nil, nil, false
			}
			scales = append(scales, arenaScaleTerm{coefficient: coefficient, label: label})
		case "AbilityResourceByCoefficientCalculationPart":
			coefficient, has := arenaRawFloat(part["mCoefficient"])
			if !has {
				return nil, nil, false
			}
			scales = append(scales, arenaScaleTerm{coefficient: coefficient, label: "最大法力值"})
		case "SumOfSubPartsCalculationPart":
			var subparts []json.RawMessage
			if json.Unmarshal(part["mSubparts"], &subparts) != nil {
				return nil, nil, false
			}
			subValues, subScales, ok := arenaCalculationTerms(subparts, dataValues, depth+1)
			if !ok {
				return nil, nil, false
			}
			values, scales = append(values, subValues...), append(scales, subScales...)
		default:
			// Hashed part type carrying an explicit Min/Max data value pair.
			// Other hashed types (for example base + per-level pairs) are left
			// unresolved on purpose: guessing their arithmetic prints a number
			// that looks authoritative and is wrong.
			low, high, ok := arenaMinMaxDataValues(part, dataValues)
			if !ok {
				return nil, nil, false
			}
			values = append(values, arenaValueTerm{low: low, high: high, isRange: true})
		}
	}
	return values, scales, true
}

// arenaBreakpointRange walks the per-level table to level 18 exactly like the
// client: a base rate from level 2, replaced at "at and after" breakpoints, plus
// one-off bonuses at their own level.
func arenaBreakpointRange(part map[string]json.RawMessage) (float64, float64, bool) {
	start, ok := arenaRawFloat(part["mLevel1Value"])
	if !ok {
		return 0, 0, false
	}
	rate, _ := arenaRawFloat(part["mInitialBonusPerLevel"])
	var breakpoints []struct {
		Level                   int      `json:"mLevel"`
		BonusPerLevelAtAndAfter *float64 `json:"mBonusPerLevelAtAndAfter"`
		AdditionalBonus         *float64 `json:"mAdditionalBonusAtThisLevel"`
	}
	if len(part["mBreakpoints"]) > 0 && json.Unmarshal(part["mBreakpoints"], &breakpoints) != nil {
		return 0, 0, false
	}
	value := start
	for level := 2; level <= arenaMaximumChampionLevel; level++ {
		for _, breakpoint := range breakpoints {
			if breakpoint.Level != level {
				continue
			}
			if breakpoint.BonusPerLevelAtAndAfter != nil {
				rate = *breakpoint.BonusPerLevelAtAndAfter
			}
			if breakpoint.AdditionalBonus != nil {
				value += *breakpoint.AdditionalBonus
			}
		}
		value += rate
	}
	return start, value, true
}

func arenaMinMaxDataValues(part map[string]json.RawMessage, dataValues map[string]json.RawMessage) (float64, float64, bool) {
	var minimum, maximum string
	for key, raw := range part {
		if key == "__type" {
			continue
		}
		var name string
		if json.Unmarshal(raw, &name) != nil || name == "" {
			return 0, 0, false
		}
		switch {
		case strings.HasSuffix(name, "Min"):
			minimum = name
		case strings.HasSuffix(name, "Max"):
			maximum = name
		default:
			return 0, 0, false
		}
	}
	if minimum == "" || maximum == "" {
		return 0, 0, false
	}
	low, lowOK := arenaDataValueNumber(dataValues[minimum])
	high, highOK := arenaDataValueNumber(dataValues[maximum])
	if !lowOK || !highOK {
		return 0, 0, false
	}
	return low, high, true
}

func arenaCalculationStatLabel(part map[string]json.RawMessage) (string, bool) {
	stat := 0
	if len(part["mStat"]) > 0 && json.Unmarshal(part["mStat"], &stat) != nil {
		return "", false
	}
	label, ok := arenaCalculationStatNames[stat]
	if !ok {
		return "", false
	}
	formula := 0
	_ = json.Unmarshal(part["mStatFormula"], &formula)
	if formula == 2 && stat == 2 {
		label = "额外" + label
	}
	return label, true
}

func arenaRawFloat(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var value float64
	if json.Unmarshal(raw, &value) != nil {
		return 0, false
	}
	return value, true
}

// Anything that would render as 0 after formatting carries no information and
// only adds noise like "(+0%法术强度)".
func arenaNegligible(value float64) bool {
	return math.Abs(value) < 0.05
}

func arenaCalculationNumberPart(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var part map[string]json.RawMessage
	if json.Unmarshal(raw, &part) != nil {
		return 0, false
	}
	var kind string
	if json.Unmarshal(part["__type"], &kind) != nil || kind != "NumberCalculationPart" {
		return 0, false
	}
	return arenaRawFloat(part["mNumber"])
}

// Seven-element arrays are indexed by augment level and index 0 is a stale
// duplicate slot; index 1 is the shipped level-1 value.
func arenaDataValueNumber(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var values []float64
	if json.Unmarshal(raw, &values) == nil {
		if len(values) > 1 {
			return values[1], true
		}
		if len(values) == 1 {
			return values[0], true
		}
		return 0, false
	}
	return arenaRawFloat(raw)
}

func arenaDataValueString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	return ""
}

// Float32 data values arrive as 0.030000001, so round to one decimal before
// deciding whether the number is whole — otherwise 3% renders as "3.0%".
func formatArenaValue(value float64) string {
	rounded := math.Round(value*10) / 10
	if math.Abs(rounded-math.Round(rounded)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(rounded)), 10)
	}
	return strconv.FormatFloat(rounded, 'f', 1, 64)
}

func arenaCDragonRarity(value int) string {
	return normalizeAugmentRarity(value)
}

func mergeGameplayAugmentMetadata(base, supplement []gameplayAugment) []gameplayAugment {
	result := append([]gameplayAugment(nil), base...)
	indexByID := make(map[int64]int, len(result))
	for index, item := range result {
		indexByID[item.ID] = index
	}
	for _, extra := range supplement {
		index, ok := indexByID[extra.ID]
		if !ok {
			indexByID[extra.ID] = len(result)
			result = append(result, extra)
			continue
		}
		item := &result[index]
		if strings.TrimSpace(item.Name) == "" {
			item.Name = extra.Name
		}
		if strings.TrimSpace(item.Description) == "" {
			item.Description = extra.Description
		}
		if strings.TrimSpace(item.Rarity) == "" {
			item.Rarity = extra.Rarity
		}
		if strings.TrimSpace(item.IconPath) == "" {
			item.IconPath = extra.IconPath
		}
		if strings.TrimSpace(item.FallbackIconPath) == "" {
			item.FallbackIconPath = extra.FallbackIconPath
		}
	}
	return result
}

func communityDragonGameAssetPath(value string) string {
	const prefix = "/lol-game-data/assets/"
	value = sanitizeAssetPath(value)
	if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return ""
	}
	relative := strings.ToLower(value[len(prefix):])
	if len(relative) > len("assets/") && strings.EqualFold(relative[:len("assets/")], "assets/") {
		return "/latest/game/" + relative
	}
	return "/latest/plugins/rcp-be-lol-game-data/global/default/" + relative
}

func arenaAugmentGroups(groups []opggArenaAugmentGroup, catalog []gameplayAugment) []arenaAugmentGroup {
	metadata := make(map[int64]gameplayAugment, len(catalog))
	for _, item := range catalog {
		metadata[item.ID] = item
	}
	result := make([]arenaAugmentGroup, 0, len(groups))
	for _, group := range groups {
		rows := make([]championMetricRow, 0, len(group.Augments))
		for _, item := range group.Augments {
			if item.ID <= 0 {
				continue
			}
			id := int64(item.ID)
			meta, ok := metadata[id]
			iconPath := communityDragonGameAssetPath(meta.IconPath)
			name := meta.Name
			if strings.TrimSpace(name) == "" {
				name = arenaAugmentFallbackName(item.ID)
			}
			asset := championAsset{ID: int(id), Kind: "arena-augment", Name: name, Description: meta.Description}
			if ok && iconPath != "" {
				asset.Source = "communitydragon"
				asset.Path = iconPath
				asset.FallbackPath = communityDragonGameAssetPath(meta.FallbackIconPath)
			}
			rarityCode, rarityOK := map[int]int{1: 0, 4: 1, 8: 2}[group.Rarity]
			rarity := "unknown"
			if rarityOK {
				rarity = normalizeAugmentRarity(rarityCode)
			}
			rows = append(rows, championMetricRow{
				Assets: []championAsset{asset}, Rarity: rarity, PickRate: ratePercent(item.PickRate), WinRate: firstPositive(ratePercent(item.WinRate), percentOf(item.Win, item.Play)), Games: item.Play,
				AveragePlacement: arenaAverage(float64(item.TotalPlace), item.Play), FirstPlaceRate: percentOf(item.FirstPlace, item.Play),
			})
		}
		if len(rows) > 0 {
			result = append(result, arenaAugmentGroup{Rarity: group.Rarity, Rows: rows})
		}
	}
	return result
}

func arenaAugmentFallbackName(id int) string {
	// Keep the legacy placeholder for entries missing from the localized
	// catalog so older payloads remain compatible; known legacy IDs get their
	// real localized name instead of exposing a misleading numeric label.
	if name, ok := map[int]string{2008: "负重爆气"}[id]; ok {
		return name
	}
	return "海克斯 " + strconv.Itoa(id)
}

func flattenArenaAugmentGroups(groups []arenaAugmentGroup, limit int) []championMetricRow {
	rows := make([]championMetricRow, 0)
	for _, group := range groups {
		rows = append(rows, group.Rows...)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].PickRate > rows[j].PickRate })
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func applyLocalArenaAugmentGrades(groups []arenaAugmentGroup) {
	type rowRef struct {
		group int
		row   int
	}
	refs := make([]rowRef, 0)
	for groupIndex := range groups {
		for rowIndex := range groups[groupIndex].Rows {
			refs = append(refs, rowRef{group: groupIndex, row: rowIndex})
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		left := groups[refs[i].group].Rows[refs[i].row]
		right := groups[refs[j].group].Rows[refs[j].row]
		return left.PickRate > right.PickRate
	})
	graded := make([]championMetricRow, len(refs))
	for index, ref := range refs {
		graded[index] = groups[ref.group].Rows[ref.row]
	}
	applyLocalAugmentGrades(graded)
	for index, ref := range refs {
		groups[ref.group].Rows[ref.row].Grade = graded[index].Grade
	}
}

func arenaAugmentRows(groups []opggArenaAugmentGroup, catalog []gameplayAugment) []championMetricRow {
	// Keep every quality group available to callers; presentation layers apply
	// their own per-quality cap after rarity has been resolved.
	return flattenArenaAugmentGroups(arenaAugmentGroups(groups, catalog), 0)
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
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
