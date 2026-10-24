package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

// The snapshot is metadata plus dated statistics from the public Chinese page.
// Removed IDs come from the linked official patch, and only apply to that patch.
// Historical match metadata deliberately retains those IDs.
//
//go:embed data/mayhem_augments_20261023.json
var currentMayhemSnapshot []byte

var readCurrentMayhem = sync.OnceValue(func() struct {
	Patch      string `json:"patch"`
	SourceDate string `json:"sourceDate"`
	SourceURL  string `json:"sourceUrl"`
	Removed    []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"removed"`
	Augments []augmentRaw `json:"augments"`
} {
	var result struct {
		Patch      string `json:"patch"`
		SourceDate string `json:"sourceDate"`
		SourceURL  string `json:"sourceUrl"`
		Removed    []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"removed"`
		Augments []augmentRaw `json:"augments"`
	}
	_ = json.Unmarshal(currentMayhemSnapshot, &result)
	return result
})

// Riot's game files use 16.21 where the public patch announcement uses 26.21.
func mayhemComparablePatch(patch string) string {
	parts := strings.Split(patch, ".")
	if len(parts) < 2 {
		return "0.0"
	}
	if parts[0] == "26" {
		parts[0] = "16"
	}
	return strings.Join(parts[:2], ".")
}

func mayhemAugmentAvailable(id int, patch string) bool {
	if mayhemComparablePatch(patch) != mayhemComparablePatch(readCurrentMayhem().Patch) {
		return true
	}
	for _, row := range readCurrentMayhem().Removed {
		if row.ID == id {
			return false
		}
	}
	return true
}

func currentMayhemRows(raw []augmentRaw, patch string) []championAugment {
	rows := make([]championAugment, 0, len(raw))
	seen := map[int]bool{}
	for _, item := range raw {
		asset, valid := remoteAsset(item.LargeIcon, item.Name)
		if !valid || item.ID <= 0 || item.Name == "" || seen[item.ID] || strings.Contains(item.Name, "BlankName") || !mayhemAugmentAvailable(item.ID, patch) {
			continue
		}
		seen[item.ID] = true
		row := championAugment{ID: item.ID, Key: item.Key, Name: item.Name, Tier: item.Tier, Grade: normalizeChampionGrade("opgg-augment-catalog", item.Tier, ""), Rarity: augmentRarity(item.Rarity), Performance: item.Performance, Popularity: item.Popular, Description: cleanMarkup(item.Desc), Tooltip: cleanMarkup(item.Tooltip), ImageSource: asset.Source, ImagePath: asset.Path}
		if row.Description == "" {
			row.Description = row.Tooltip
		}
		var heroes []augmentChampionRaw
		_ = json.Unmarshal(item.ChampionIDs, &heroes)
		byID := map[int]augmentChampionRaw{}
		for _, hero := range heroes {
			byID[hero.ID] = hero
		}
		for _, stats := range item.Champions {
			hero := byID[stats.ID]
			entry := championAugmentChampion{ID: stats.ID, Name: hero.Name, Key: hero.Key, Performance: stats.Performance, Popularity: stats.Popular}
			if image, ok := remoteAsset(hero.ImageURL, hero.Name); ok {
				entry.ImageSource, entry.ImagePath = image.Source, image.Path
			}
			row.Champions = append(row.Champions, entry)
		}
		rows = append(rows, row)
	}
	return rows
}

func bundledCurrentMayhemCatalog() championAugmentResponse {
	s := readCurrentMayhem()
	at, _ := time.Parse("2006-01-02", s.SourceDate)
	return championAugmentResponse{Source: "OP.GG（内置快照）", Mode: "aram-mayhem", FetchedAt: at, EntertainmentSample: true, Citation: &championSourceCitation{Source: "OP.GG", Patch: s.Patch, ReportDate: s.SourceDate, CanonicalURL: s.SourceURL}, Rows: currentMayhemRows(s.Augments, s.Patch), DataNotice: "暂未取得新版在线目录，显示注明日期的内置快照"}
}

func parseCurrentMayhemCatalog(page []byte, at time.Time) (championAugmentResponse, error) {
	decoded := decodeNextFlight(page)
	var raw []augmentRaw
	if !extractBestArray(decoded, `"data":`, func(candidate []json.RawMessage) int {
		score := 0
		for _, entry := range candidate {
			var row augmentRaw
			if json.Unmarshal(entry, &row) == nil && row.ID > 0 && row.Name != "" && row.LargeIcon != "" {
				score++
			}
		}
		return score
	}, &raw) {
		return championAugmentResponse{}, errors.New("OP.GG augment directory changed")
	}
	match := opggRSCVersionPattern.FindStringSubmatch(decoded)
	if len(match) != 2 {
		return championAugmentResponse{}, errors.New("OP.GG augment version unavailable")
	}
	rows := currentMayhemRows(raw, match[1])
	if len(rows) < 40 {
		return championAugmentResponse{}, errors.New("OP.GG augment directory incomplete")
	}
	return championAugmentResponse{Source: "OP.GG", Mode: "aram-mayhem", FetchedAt: at, EntertainmentSample: true, Citation: &championSourceCitation{Source: "OP.GG", Patch: match[1], CanonicalURL: "https://op.gg/zh-cn/lol/modes/aram-mayhem"}, Rows: rows}, nil
}

func (p *championProvider) loadAugments(ctx context.Context) (championAugmentResponse, error) {
	if result, err := p.loadARAMKitAugments(ctx); err == nil {
		p.rememberAugmentCatalog(result)
		return result, nil
	}
	result := bundledCurrentMayhemCatalog()
	for i := range result.Rows {
		result.Rows[i].Grade = ""
		result.Rows[i].WinRate = 0
		result.Rows[i].Popularity = 0
		result.Rows[i].Performance = 0
		result.Rows[i].PerformanceUnavailable = true
		result.Rows[i].Champions = nil
	}
	return result, nil
}

func currentMayhemMetadataRows() []gameplayAugment {
	rows := make([]gameplayAugment, 0, len(readCurrentMayhem().Augments))
	for _, item := range readCurrentMayhem().Augments {
		asset, ok := remoteAsset(item.LargeIcon, item.Name)
		if !ok {
			continue
		}
		rows = append(rows, gameplayAugment{ID: int64(item.ID), Name: item.Name, Description: cleanMarkup(item.Desc), Rarity: augmentRarity(item.Rarity), IconPath: "opgg:" + asset.Path})
	}
	return rows
}

func (p *championProvider) enrichCurrentMayhemDetail(ctx context.Context, detail *championDetailResponse) {
	if comparePatchVersions(mayhemComparablePatch(detail.Patch), mayhemComparablePatch(readCurrentMayhem().Patch)) < 0 {
		detail.IsStale = true
		detail.DataNotice = "该来源仍是旧版本数据，请参考标注的数据版本"
		return
	}
	catalog, ready := p.rememberedAugmentCatalog()
	if !ready {
		catalog = bundledCurrentMayhemCatalog()
		catalog.DataNotice = ""
	}
	if catalog.Citation != nil && comparePatchVersions(mayhemComparablePatch(detail.Patch), mayhemComparablePatch(catalog.Citation.Patch)) > 0 {
		catalog, _ = p.loadAugments(ctx)
	}
	if catalog.Citation == nil || mayhemComparablePatch(catalog.Citation.Patch) != mayhemComparablePatch(detail.Patch) {
		detail.DataNotice = "该版本的海克斯效果资料尚未同步"
		return
	}
	byID := map[int]championAugment{}
	for _, row := range catalog.Rows {
		byID[row.ID] = row
	}
	filtered := make([]championMetricRow, 0, len(detail.RecommendedAugments))
	for _, row := range detail.RecommendedAugments {
		if len(row.Assets) == 0 || !mayhemAugmentAvailable(row.Assets[0].ID, detail.Patch) {
			continue
		}
		for i := range row.Assets {
			if meta, ok := byID[row.Assets[i].ID]; ok {
				row.Assets[i].Name = meta.Name
				row.Assets[i].Description = meta.Description
				row.Assets[i].Source = meta.ImageSource
				row.Assets[i].Path = meta.ImagePath
				row.Rarity = meta.Rarity
			}
		}
		filtered = append(filtered, row)
	}
	detail.RecommendedAugments = filtered
	if catalog.DataNotice != "" {
		detail.DataNotice = catalog.DataNotice
	}
}

func (p *championProvider) loadCurrentAugmentDetail(ctx context.Context, id int) (championAugmentDetailResponse, error) {
	if result, err := p.loadARAMKitAugmentDetail(ctx, id); err == nil {
		return result, nil
	}
	catalog, err := p.loadAugments(ctx)
	if err != nil {
		return championAugmentDetailResponse{}, err
	}
	for _, row := range catalog.Rows {
		if row.ID == id {
			return championAugmentDetailResponse{ID: id, Slug: row.Key, Source: catalog.Source, Description: row.Description, effectDescription: row.Description, Citation: catalog.Citation, Champions: row.Champions}, nil
		}
	}
	return championAugmentDetailResponse{}, errors.New("该海克斯不在当前版本图鉴中")
}
