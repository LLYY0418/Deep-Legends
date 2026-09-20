package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
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
	respondJSON(w, map[string]any{"banners": banners, "bannerOwnershipUnavailable": unknown, "writeSupported": true, "writeStatus": "supported"})
}

// The client profile editor passes selectedBannerId (regalia.id) as bannerAccent.
// idSecondary is the rank tier for built-in rank banners, not a cosmetic ID.
// REGALIA_BANNER_SLOT accepts PATCH but is not the career banner's source.
const facadeChallengeSummaryPath = "/lol-challenges/v1/summary-player-data/local-player"
const facadeChallengePreferencesPath = "/lol-challenges/v1/update-player-preferences/"

func writeFacadeBanner(ctx context.Context, client *LCUClient, target string) error {
	var catalog []facadeRegalia
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-game-data/assets/v1/regalia.json", nil, &catalog); err != nil {
		return err
	}
	accent := ""
	for _, item := range catalog {
		if item.ID == target && item.Type == "kBanner" && item.Selectable && item.ID != "1" && item.ID != "2" {
			accent = item.ID
			break
		}
	}
	if accent == "" {
		return errFacadeInvalid
	}
	preferences, err := facadeBannerPreferences(ctx, client)
	if err != nil {
		return err
	}
	preferences["bannerAccent"] = accent
	return writeFacadeBannerPreferences(ctx, client, preferences)
}

// Snapshot exactly the preferences used by the client editor so changing the
// banner cannot clear tokens, title, or crest. Probes retain this for restore.
func facadeBannerPreferences(ctx context.Context, client *LCUClient) (map[string]any, error) {
	var summary map[string]any
	if err := client.RequestJSON(ctx, http.MethodGet, facadeChallengeSummaryPath, nil, &summary); err != nil {
		return nil, err
	}
	accent, present := summary["bannerId"]
	if !present {
		return nil, errors.New("客户端尚未提供当前旗帜状态，已停止写入")
	}
	body := map[string]any{"bannerAccent": accent}
	var ids []any
	if selected, valid := facadeSelectedChallengeIDs(summary); valid {
		for _, id := range selected {
			value, _ := strconv.ParseInt(id, 10, 64)
			ids = append(ids, value)
		}
	} else if top, ok := summary["topChallenges"].([]any); ok {
		for _, item := range top {
			if item == nil {
				ids = append(ids, int64(-1))
				continue
			}
			id := facadeChallengeID(anyMap(item)["id"])
			if id == "" {
				return nil, errors.New("客户端挑战展示数据格式无效，已停止写入")
			}
			value, _ := strconv.ParseInt(id, 10, 64)
			ids = append(ids, value)
		}
	} else {
		return nil, errors.New("客户端挑战展示数据格式无效，已停止写入")
	}
	if len(ids) > 0 {
		for len(ids) < 3 {
			ids = append(ids, int64(-1))
		}
		body["challengeIds"] = ids
	}
	if id, ok := facadeTitleItemID(summary); ok {
		body["title"] = strconv.FormatInt(id, 10)
	} else if facadeSummaryHasTitle(summary) {
		return nil, &facadeTitleRestoreNoCandidateError{}
	}
	if value, ok := summary["crestId"]; ok {
		body["crestBorder"] = value
	}
	if value, ok := summary["prestigeCrestBorderLevel"]; ok {
		body["prestigeCrestBorderLevel"] = value
	}
	return body, nil
}

func writeFacadeBannerPreferences(ctx context.Context, client *LCUClient, body map[string]any) error {
	_, err := writeFacadeBannerPreferencesResult(ctx, client, body)
	return err
}

func writeFacadeBannerPreferencesResult(ctx context.Context, client *LCUClient, body map[string]any) (int, error) {
	status, err := r99Request(ctx, client, http.MethodPost, facadeChallengePreferencesPath, body, nil)
	if err != nil {
		return status, err
	}
	expected := facadeBannerIdentity(body["bannerAccent"])
	for attempt := 0; attempt <= 10; attempt++ {
		var observed map[string]any
		err := client.RequestJSON(ctx, http.MethodGet, facadeChallengeSummaryPath, nil, &observed)
		value, present := observed["bannerId"]
		if err == nil && present && facadeBannerIdentity(value) == expected {
			return status, nil
		}
		if attempt < 10 {
			if err := waitRiotDelay(ctx, 200*time.Millisecond); err != nil {
				return status, err
			}
		}
	}
	return status, errors.New("客户端尚未确认装备所选旗帜，已保留实际装备状态；接口返回成功不代表装备已生效")
}

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
