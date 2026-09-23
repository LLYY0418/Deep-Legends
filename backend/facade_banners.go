package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

// R101 real R5/R8 comparison: inventory/v2 returns four false owned values.
// Only regalia/v3 is authoritative for banner ownership.
const facadeBannerInventoryPath = "/lol-regalia/v3/inventory/REGALIA_BANNER"

type facadeBannerInventoryEntry struct {
	Owned bool            `json:"isOwned"`
	Items json.RawMessage `json:"items"`
}
type facadeRegalia struct {
	ID          string `json:"id"`
	IDSecondary string `json:"idSecondary"`
	Type        string `json:"regaliaType"`
	Selectable  bool   `json:"isSelectable"`
	TencentOnly bool   `json:"isTencentOnly"`
	Name        string `json:"localizedName"`
	ContentID   string `json:"contentId"`
	AssetPath   string `json:"assetPath"`
}
type facadeBanner struct {
	ID          string   `json:"id"`
	IDSecondary string   `json:"idSecondary"`
	Name        string   `json:"localizedName"`
	TencentOnly bool     `json:"isTencentOnly"`
	Owned       bool     `json:"owned"`
	ImagePath   string   `json:"imagePath,omitempty"`
	SearchTerms []string `json:"searchTerms,omitempty"`
}

func loadFacadeBanners(ctx context.Context, client *LCUClient) ([]facadeBanner, bool, error) {
	var catalog []facadeRegalia
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-game-data/assets/v1/regalia.json", nil, &catalog); err != nil {
		return nil, true, err
	}
	var inventory map[string]facadeBannerInventoryEntry
	err := client.RequestJSON(ctx, http.MethodGet, facadeBannerInventoryPath, nil, &inventory)
	unknown := err != nil || len(inventory) == 0
	banners := make([]facadeBanner, 0)
	for _, item := range catalog {
		if item.Type != "kBanner" || !item.Selectable || item.ID == "1" || item.ID == "2" {
			continue
		}
		imagePath := item.AssetPath
		if !strings.HasPrefix(strings.ToLower(imagePath), "/lol-game-data/assets/") || strings.ContainsAny(imagePath, "?#") || strings.Contains(imagePath, "..") {
			imagePath = ""
		}
		banners = append(banners, facadeBanner{ID: item.ID, IDSecondary: item.IDSecondary, Name: item.Name, TencentOnly: item.TencentOnly, Owned: !unknown && inventory[item.ID].Owned, ImagePath: imagePath, SearchTerms: championPinyinTerms(item.Name)})
	}
	sort.SliceStable(banners, func(i, j int) bool { return banners[i].Name < banners[j].Name })
	return banners, unknown, nil
}
func (a *app) handleFacadeBanners(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, "未连接英雄联盟客户端", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	banners, unknown, err := loadFacadeBanners(ctx, client)
	if err != nil {
		http.Error(w, "旗帜目录暂时读取失败，请稍后重试", http.StatusBadGateway)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, map[string]any{"banners": banners, "bannerOwnershipUnavailable": unknown})
}

// R123：生涯旗帜写入已退场，目录与拥有状态仅只读浏览；客户端编辑器的 bannerAccent 写入路径不再使用。
// idSecondary is the rank tier for built-in rank banners, not a cosmetic ID.
// REGALIA_BANNER_SLOT accepts PATCH but is not the career banner's source.
const facadeChallengeSummaryPath = "/lol-challenges/v1/summary-player-data/local-player"

func facadeBannerIdentity(value any) string {
	raw, _ := json.Marshal(value)
	return facadeBannerItemID(raw)
}

func facadeBannerItemID(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}
