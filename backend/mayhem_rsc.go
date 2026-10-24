package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	opggRSCVersionPattern    = regexp.MustCompile(`/meta/images/lol/([0-9]+\.[0-9]+(?:\.[0-9]+)?)/`)
	opggRSCLinePattern       = regexp.MustCompile(`(?m)^([0-9a-f]+):(.*)$`)
	opggRSCReferencePattern  = regexp.MustCompile(`"\$L([0-9a-f]+)"`)
	opggRSCMetaIDPattern     = regexp.MustCompile(`"metaId":([0-9]+)`)
	opggRSCMetaKindIDPattern = regexp.MustCompile(`"metaType":"([^"]+)"\s*,\s*"metaId":([0-9]+)`)
	opggRSCMetaIDKindPattern = regexp.MustCompile(`"metaId":([0-9]+)\s*,\s*"metaType":"([^"]+)"`)
	opggRSCAugmentIDPattern  = regexp.MustCompile(`"metaId":([0-9]+),"metaType":"aram-augment"`)
	opggRSCSkillPattern      = regexp.MustCompile(`"skill_[0-9]+"[\s\S]{0,600}?"extraData":"([QWER])"`)
	opggRSCSkillOrderPattern = regexp.MustCompile(`"span","[0-9]+"[\s\S]{0,320}?"children":"([QWER])"`)
	opggRSCSpellPairPattern  = regexp.MustCompile(`(?m)^[0-9a-f]+:(.*"spell_0".*"spell_1".*)$`)
	opggRSCSpellMetaPattern  = regexp.MustCompile(`(?:"metaId":([0-9]+)\s*,\s*"metaType":"spell"|"metaType":"spell"\s*,\s*"metaId":([0-9]+))`)
	opggRSCMetaNamePattern   = regexp.MustCompile(`"(?:name|title)":"([^"]+)"`)
	opggRSCMetaIconPattern   = regexp.MustCompile(`"(?:src|icon|iconUrl|image_url)":"([^"]+\.(?:png|jpg|jpeg|webp))"`)
)

type mayhemRSCDetail struct {
	Build             championBuildSections
	Augments          []championMetricRow
	Citation          championSourceCitation
	FetchedAt         time.Time
	RejectedCoreItems []rscRejectedItem
}

type mayhemRSCPhases struct{ wireMS, parseMS, descriptionsMS int64 }

type rscRejectedItem struct {
	ID     int
	Reason string
}

func (p *championProvider) reportRSCRejectedItems(rows []rscRejectedItem) {
	if p.diag == nil {
		return
	}
	for _, row := range rows {
		p.diag(map[string]any{"event": "rsc_core_item_rejected", "id": row.ID, "reason": row.Reason})
	}
}

func (p *championProvider) loadMayhemRSCWithPhases(ctx context.Context, slug string, phases *mayhemRSCPhases) (mayhemRSCDetail, error) {
	requestPath := "/lol/modes/aram-mayhem/" + slug + "/build"
	key := strings.Join([]string{"v2", "opgg-rsc", "aram-mayhem", slug}, "|")
	// The shared item catalog is independent of this hero's RSC payload.
	// Start it now so a cold Data Dragon read does not follow the RSC wait.
	type descriptionsResult struct {
		rows map[string]championAssetDescription
		err  error
		ms   int64
	}
	descriptionsReady := make(chan descriptionsResult, 1)
	go func() {
		defer recoverPanic("mayhem.loadMayhemRSCWithPhases.1")
		defer close(descriptionsReady)

		started := time.Now()
		rows, err := p.loadStaticDescriptions(ctx)
		descriptionsReady <- descriptionsResult{rows, err, time.Since(started).Milliseconds()}
	}()
	loader := func(loadCtx context.Context) ([]byte, error) { return p.fetchOPGGRSCDirect(loadCtx, requestPath) }
	var data []byte
	var fetchedAt time.Time
	var err error
	wireStarted := time.Now()
	if p.cache != nil {
		result, loadErr := p.cache.loadWithStatus(ctx, key, 6*time.Hour, 24*time.Hour, true, loader)
		data, fetchedAt, err = result.data, result.fetchedAt, loadErr
	} else {
		data, err = loader(ctx)
		fetchedAt = time.Now()
	}
	if phases != nil {
		phases.wireMS = time.Since(wireStarted).Milliseconds()
	}
	if err != nil {
		return mayhemRSCDetail{}, err
	}
	parseStarted := time.Now()
	result, err := parseMayhemRSC(data, slug)
	if phases != nil {
		phases.parseMS = time.Since(parseStarted).Milliseconds()
	}
	if err != nil {
		return mayhemRSCDetail{}, err
	}
	result.Citation.Source, result.Citation.CanonicalURL = "OP.GG RSC", "https://op.gg"+requestPath
	result.FetchedAt = fetchedAt
	p.reportRSCRejectedItems(result.RejectedCoreItems)
	loadedDescriptions := <-descriptionsReady
	descriptions, descriptionErr := loadedDescriptions.rows, loadedDescriptions.err
	if phases != nil {
		phases.descriptionsMS = loadedDescriptions.ms
	}
	if descriptionErr != nil {
		return mayhemRSCDetail{}, fmt.Errorf("load Data Dragon item catalog: %w", descriptionErr)
	}
	validItemIDs := make(map[int]bool)
	for key := range descriptions {
		if !strings.HasPrefix(key, "item/") {
			continue
		}
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(key, "item/"), filepath.Ext(key)))
		if id > 0 {
			validItemIDs[id] = true
		}
	}
	filteredRows := result.Build.CoreItems[:0]
	for _, row := range result.Build.CoreItems {
		assets := row.Assets[:0]
		for _, asset := range row.Assets {
			if asset.Kind == "item" && validItemIDs[asset.ID] {
				assets = append(assets, asset)
				continue
			}
			if p.diag != nil {
				p.diag(map[string]any{"event": "rsc_core_item_rejected", "id": asset.ID, "reason": "not_in_ddragon_item_catalog"})
			}
		}
		row.Assets = assets
		if len(row.Assets) > 0 {
			filteredRows = append(filteredRows, row)
		}
	}
	result.Build.CoreItems = filteredRows
	if len(result.Build.CoreItems) == 0 {
		return mayhemRSCDetail{}, errors.New("OP.GG mayhem RSC core items are invalid")
	}
	for _, section := range []*[]championMetricRow{&result.Build.StarterItems, &result.Build.Boots, &result.Build.CoreItems, &result.Build.SummonerSpells} {
		for rowIndex := range *section {
			for assetIndex := range (*section)[rowIndex].Assets {
				asset := &(*section)[rowIndex].Assets[assetIndex]
				if asset.Path == "" {
					asset.Path = p.ddragonAssetPath(asset.Kind, asset.ID)
				}
			}
		}
	}
	return result, nil
}

func (p *championProvider) fetchOPGGRSCDirect(ctx context.Context, requestPath string) (data []byte, err error) {
	started := time.Now()
	defer func() {
		if p.diag != nil && !isCancellation(err) {
			p.diag(map[string]any{"event": "champion_upstream", "host": "op.gg", "kind": "rsc",
				"status": championUpstreamHTTPStatus(err), "bytes": len(data),
				"duration_ms": time.Since(started).Milliseconds(), "cache": championCacheStateMiss})
		}
	}()
	requestContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, "https://"+opggPageHost+requestPath, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/x-component")
	request.Header.Set("RSC", "1")
	request.Header.Set("User-Agent", "DeepLegends/"+version)
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("champion provider returned HTTP %d", response.StatusCode)
	}
	return readLimited(response.Body, championHTMLMax)
}

func expandRSCReferences(source string) string {
	lines := make(map[string]string)
	for _, match := range opggRSCLinePattern.FindAllStringSubmatch(source, -1) {
		// RSC uses the same $Lxx syntax for delayed JSON values and imported
		// component descriptors. Only delayed JSON rows belong in build data.
		if strings.HasPrefix(match[2], "[") {
			lines[match[1]] = match[2]
		}
	}
	result := source
	for range 4 {
		changed := false
		result = opggRSCReferencePattern.ReplaceAllStringFunc(result, func(reference string) string {
			match := opggRSCReferencePattern.FindStringSubmatch(reference)
			if len(match) != 2 {
				return reference
			}
			if line := lines[match[1]]; line != "" {
				changed = true
				return line
			}
			return reference
		})
		if !changed {
			break
		}
	}
	return result
}

func rscRowSegments(source, prefix string, limit int) []string {
	pattern := regexp.MustCompile(`"(` + regexp.QuoteMeta(prefix) + `_[0-9]+)"`)
	indexes := pattern.FindAllStringSubmatchIndex(source, -1)
	// A Next.js RSC response can expose the same logical row more than once:
	// an expanded mobile-table reference and the complete desktop-table row.
	// The old code counted occurrences, so a duplicate item_0 consumed one of
	// the five slots and item_4 was never inspected. Keep one segment per row
	// key and prefer the compact, self-contained occurrence.
	order := make([]string, 0, min(limit, len(indexes)))
	segments := make(map[string]string, min(limit, len(indexes)))
	for index, bounds := range indexes {
		end := len(source)
		if index+1 < len(indexes) {
			end = indexes[index+1][0]
		}
		if nextRow := strings.Index(source[bounds[1]:end], `["$","tr"`); nextRow >= 0 {
			end = bounds[1] + nextRow
		}
		key := source[bounds[2]:bounds[3]]
		segment := source[bounds[0]:end]
		current, exists := segments[key]
		if !exists {
			if limit > 0 && len(order) >= limit {
				continue
			}
			order = append(order, key)
		}
		if !exists || len(segment) < len(current) {
			segments[key] = segment
		}
	}
	result := make([]string, 0, len(order))
	for _, key := range order {
		result = append(result, segments[key])
	}
	return result
}

func rscRows(source, prefix string, limit int) [][]int {
	segments := rscRowSegments(source, prefix, limit)
	result := make([][]int, 0, len(segments))
	for _, segment := range segments {
		seen := make(map[int]bool)
		ids := make([]int, 0, 6)
		for _, match := range opggRSCMetaIDPattern.FindAllStringSubmatch(segment, -1) {
			id, _ := strconv.Atoi(match[1])
			if id > 0 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			result = append(result, ids)
		}
	}
	return result
}

func rscCoreItemRows(source string, limit int) ([][]int, []rscRejectedItem) {
	segments := rscRowSegments(source, "core_items", limit)
	rows := make([][]int, 0, len(segments))
	rejected := make([]rscRejectedItem, 0)
	for _, segment := range segments {
		kinds := make(map[int]string)
		for _, match := range opggRSCMetaKindIDPattern.FindAllStringSubmatch(segment, -1) {
			id, _ := strconv.Atoi(match[2])
			kinds[id] = strings.ToLower(match[1])
		}
		for _, match := range opggRSCMetaIDKindPattern.FindAllStringSubmatch(segment, -1) {
			id, _ := strconv.Atoi(match[1])
			kinds[id] = strings.ToLower(match[2])
		}
		seen := make(map[int]bool)
		ids := make([]int, 0, 6)
		for _, match := range opggRSCMetaIDPattern.FindAllStringSubmatch(segment, -1) {
			id, _ := strconv.Atoi(match[1])
			if seen[id] {
				continue
			}
			seen[id] = true
			reason := ""
			kind := kinds[id]
			if kind == "" {
				reason = "missing_meta_type"
			} else if kind != "item" {
				reason = "kind_" + kind
			}
			if reason != "" {
				rejected = append(rejected, rscRejectedItem{ID: id, Reason: reason})
				continue
			}
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			rows = append(rows, ids)
		}
	}
	return rows, rejected
}

func metricRowsFromIDs(values [][]int, kind string) []championMetricRow {
	rows := make([]championMetricRow, 0, len(values))
	for _, ids := range values {
		assets := make([]championAsset, 0, len(ids))
		for _, id := range ids {
			assets = append(assets, championAsset{ID: id, Kind: kind, Name: strconv.Itoa(id), Source: "ddragon"})
		}
		rows = append(rows, championMetricRow{Assets: assets})
	}
	return rows
}

func rscAssetMetadata(source string, id int, kind string) championAsset {
	asset := championAsset{ID: id, Kind: kind, Name: strconv.Itoa(id), Source: "ddragon"}
	needle := fmt.Sprintf(`"metaId":%d`, id)
	start := 0
	for {
		index := strings.Index(source[start:], needle)
		if index < 0 {
			break
		}
		index += start
		left, right := max(0, index-700), min(len(source), index+900)
		window := source[left:right]
		if match := opggRSCMetaNamePattern.FindStringSubmatch(window); len(match) == 2 && strings.TrimSpace(match[1]) != "" {
			asset.Name = match[1]
		}
		if match := opggRSCMetaIconPattern.FindStringSubmatch(window); len(match) == 2 {
			iconURL := strings.ReplaceAll(match[1], `\/`, "/")
			if parsed, err := url.Parse(iconURL); err == nil && parsed.Host == opggAssetHost && strings.HasPrefix(parsed.Path, "/meta/images/") {
				asset.Source, asset.Path = "opgg", parsed.Path
			}
		}
		break
	}
	return asset
}

func rscAssetRarity(source string, id int) string {
	needle := fmt.Sprintf(`"metaId":%d`, id)
	index := strings.Index(source, needle)
	if index < 0 {
		return "unknown"
	}
	window := strings.ToLower(source[max(0, index-700):min(len(source), index+900)])
	switch {
	case strings.Contains(window, "prismatic") || strings.Contains(window, "棱彩"):
		return "prismatic"
	case strings.Contains(window, "gold") || strings.Contains(window, "黄金"):
		return "gold"
	case strings.Contains(window, "silver") || strings.Contains(window, "白银"):
		return "silver"
	default:
		return "unknown"
	}
}

func rscSpellRows(source string) [][]int {
	result := make([][]int, 0, 2)
	for _, match := range opggRSCSpellPairPattern.FindAllStringSubmatch(source, -1) {
		if len(match) != 2 {
			continue
		}
		ids := make([]int, 0, 2)
		seen := make(map[int]bool)
		for _, idMatch := range opggRSCSpellMetaPattern.FindAllStringSubmatch(match[1], -1) {
			value := idMatch[1]
			if value == "" {
				value = idMatch[2]
			}
			id, _ := strconv.Atoi(value)
			if id > 0 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
			if len(ids) == 2 {
				break
			}
		}
		if len(ids) == 2 {
			result = append(result, ids)
		}
		if len(result) == 2 {
			break
		}
	}
	return result
}

func parseMayhemRSC(data []byte, slug string) (mayhemRSCDetail, error) {
	expanded := expandRSCReferences(decodeNextFlight(data))
	if expanded == "" {
		expanded = expandRSCReferences(string(data))
	}
	result := mayhemRSCDetail{}
	if match := opggRSCVersionPattern.FindStringSubmatch(expanded); len(match) == 2 {
		result.Citation.Patch = match[1]
	}
	result.Build.StarterItems = metricRowsFromIDs(rscRows(expanded, "starter_items", 2), "item")
	result.Build.Boots = metricRowsFromIDs(rscRows(expanded, "boots", 2), "item")
	coreItems, rejectedCoreItems := rscCoreItemRows(expanded, 5)
	result.Build.CoreItems = metricRowsFromIDs(coreItems, "item")
	result.RejectedCoreItems = rejectedCoreItems
	result.Build.SummonerSpells = metricRowsFromIDs(rscSpellRows(expanded), "spell")
	priority := make([]string, 0, 3)
	for _, match := range opggRSCSkillPattern.FindAllStringSubmatch(expanded, -1) {
		if len(match) == 2 && !containsString(priority, match[1]) {
			priority = append(priority, match[1])
		}
		if len(priority) == 3 {
			break
		}
	}
	allOrder := opggRSCSkillOrderPattern.FindAllStringSubmatch(expanded, -1)
	if len(allOrder) > 15 {
		allOrder = allOrder[len(allOrder)-15:]
	}
	order := make([]string, 0, 15)
	for _, match := range allOrder {
		if len(match) == 2 {
			order = append(order, match[1])
		}
		if len(order) == 15 {
			break
		}
	}
	if len(priority) > 0 {
		assets := make([]championAsset, len(priority))
		for index, key := range priority {
			assets[index] = championAsset{Kind: "ability", Name: key}
		}
		result.Build.Skills = []championMetricRow{{Assets: assets, SkillPriority: priority, SkillOrder: order}}
	}
	seen := make(map[int]bool)
	for _, match := range opggRSCAugmentIDPattern.FindAllStringSubmatch(expanded, -1) {
		id, _ := strconv.Atoi(match[1])
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		asset := rscAssetMetadata(expanded, id, "augment")
		result.Augments = append(result.Augments, championMetricRow{Assets: []championAsset{asset}, Rarity: rscAssetRarity(expanded, id)})
	}
	if len(result.Build.CoreItems) == 0 || len(result.Build.StarterItems) == 0 || len(result.Build.Boots) == 0 || len(result.Build.Skills) == 0 || len(result.Build.SummonerSpells) == 0 {
		return result, errors.New("OP.GG mayhem RSC is incomplete")
	}
	return result, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
