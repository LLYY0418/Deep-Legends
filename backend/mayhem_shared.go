package main

import (
	"strings"
)

type championSourceCitation struct {
	Source       string `json:"source"`
	Patch        string `json:"patch,omitempty"`
	ReportDate   string `json:"reportDate,omitempty"`
	CanonicalURL string `json:"canonicalUrl"`
}

type championRarityStage struct {
	Stage     int     `json:"stage"`
	Silver    float64 `json:"silver"`
	Gold      float64 `json:"gold"`
	Prismatic float64 `json:"prismatic"`
	Games     int     `json:"games"`
}

type championAugmentDetailResponse struct {
	ChampionStages map[string][]championAugmentChampion `json:"championStages,omitempty"`
	Stages         []aramkitAtlasStage                  `json:"stages,omitempty"`
	Combinations   []championMetricRow                  `json:"combinations,omitempty"`
	Items          []championMetricRow                  `json:"items,omitempty"`
	// Raw, checked guide text before metadata fallback; used by match build cards.
	effectDescription string
	ID                int                     `json:"id"`
	Slug              string                  `json:"slug"`
	Source            string                  `json:"source"`
	Description       string                  `json:"description,omitempty"`
	Citation          *championSourceCitation `json:"citation,omitempty"`
	// R128 §2.3：前端不展示（海克斯图鉴详情不再渲染口径说明）；字段保留供诊断与既有后端测试使用。
	MeasurementTechnique string                    `json:"measurementTechnique,omitempty"`
	Champions            []championAugmentChampion `json:"champions"`
}

type championAugmentRarityResponse struct {
	Source   string                  `json:"source"`
	Citation *championSourceCitation `json:"citation,omitempty"`
	// R128 §2.3：前端不展示（全服品质分布面板不再渲染口径说明）；字段保留供诊断与既有后端测试使用。
	MeasurementTechnique string                `json:"measurementTechnique,omitempty"`
	Stages               []championRarityStage `json:"stages"`
}

// Augment IDs are unique across the whole CommunityDragon catalog (655 entries,
// zero collisions between the Cherry and ARAM_ namespaces), so metadata is
// always looked up by ID. The icon directory (/Cherry/ vs /Kiwi/) looks like a
// mode boundary but is not one: 217 of the 370 Mayhem augments ship /Cherry/
// icons and Arena's 393 艾卡西亚的陷落 ships a /Kiwi/ icon. Filtering on it used
// to drop names (leaving raw IDs like "1022"), icons and rarities.
func gameplayAugmentIndexAll(catalog []gameplayAugment) map[int]gameplayAugment {
	result := make(map[int]gameplayAugment, len(catalog))
	for _, item := range catalog {
		if item.ID > 0 {
			result[int(item.ID)] = item
		}
	}
	return result
}

func normalizeAugmentRarity(value any) string {
	switch typed := value.(type) {
	case int:
		return map[int]string{0: "silver", 1: "gold", 2: "prismatic", 4: "event"}[typed]
	case string:
		key := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(typed), "k"))
		if key == "eventchoice" {
			return "event"
		}
		if key == "silver" || key == "gold" || key == "prismatic" || key == "event" {
			return key
		}
	}
	return "unknown"
}

func mergeMayhemAugmentRows(primary, fallback []championMetricRow, limit int) []championMetricRow {
	capacity := len(primary) + len(fallback)
	if limit > 0 {
		capacity = min(limit, capacity)
	}
	result := make([]championMetricRow, 0, capacity)
	seen := make(map[int]bool, len(primary)+len(fallback))
	for _, rows := range [][]championMetricRow{primary, fallback} {
		for _, row := range rows {
			if len(row.Assets) == 0 || row.Assets[0].ID <= 0 || seen[row.Assets[0].ID] {
				continue
			}
			seen[row.Assets[0].ID] = true
			result = append(result, row)
			if limit > 0 && len(result) == limit {
				return result
			}
		}
	}
	return result
}

func augmentMetadataImage(iconPath string) (string, string) {
	if strings.HasPrefix(iconPath, "opgg:") {
		path := strings.TrimPrefix(iconPath, "opgg:")
		if _, ok := validateChampionAssetPath("opgg", path); ok {
			return "opgg", path
		}
		return "", ""
	}
	if strings.HasPrefix(iconPath, "builtin:") {
		path := strings.TrimPrefix(iconPath, "builtin:")
		if _, ok := bundledAugmentID(path); ok {
			return "builtin", path
		}
		return "", ""
	}
	if path := communityDragonGameAssetPath(iconPath); path != "" {
		return "communitydragon", path
	}
	return "", ""
}
