package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type facadeIcon struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Year        int      `json:"year"`
	Legacy      bool     `json:"legacy"`
	Disabled    bool     `json:"disabled"`
	Owned       bool     `json:"owned"`
	Sets        []string `json:"sets"`
	SearchTerms []string `json:"searchTerms"`
	ImagePath   string   `json:"-"`
}
type facadeIconCatalog struct {
	Icons                []facadeIcon `json:"icons"`
	OwnershipUnavailable bool         `json:"iconOwnershipUnavailable"`
	Total                int          `json:"total"`
	OwnedCount           int          `json:"ownedCount"`
}
type facadeIconCache struct {
	mu      sync.Mutex
	client  *LCUClient
	at      time.Time
	catalog facadeIconCatalog
	err     error
	// Per-cache seam verifies expensive transliteration is only done on a miss.
	terms func(string) []string
}

func (a *app) loadIconCatalog(ctx context.Context, client *LCUClient) ([]facadeIcon, error) {
	catalog, err := a.loadIconCatalogResult(ctx, client)
	return catalog.Icons, err
}
func (a *app) loadIconCatalogResult(ctx context.Context, client *LCUClient) (facadeIconCatalog, error) {
	c := &a.facadeIcons
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != client {
		c.client = client
		c.at = time.Time{}
		c.catalog = facadeIconCatalog{}
		c.err = nil
	}
	ttl := 15 * time.Minute
	if c.err != nil || c.catalog.OwnershipUnavailable || len(c.catalog.Icons) == 0 {
		ttl = 5 * time.Second
	}
	if !c.at.IsZero() && time.Since(c.at) < ttl {
		return c.catalog, c.err
	}
	c.catalog, c.err = a.fetchIconCatalog(ctx, client, c.terms)
	c.at = time.Now()
	return c.catalog, c.err
}
func (a *app) fetchIconCatalog(ctx context.Context, client *LCUClient, terms func(string) []string) (facadeIconCatalog, error) {
	raw, err := client.GetBytesContext(ctx, r99ReadPaths[0])
	if err != nil {
		return facadeIconCatalog{}, err
	}
	var rows []struct {
		ID              int64    `json:"id"`
		Title           string   `json:"title"`
		Year            int      `json:"yearReleased"`
		Legacy          bool     `json:"isLegacy"`
		ImagePath       string   `json:"imagePath"`
		DisabledRegions []string `json:"disabledRegions"`
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		return facadeIconCatalog{}, err
	}
	sets := map[int64][]string{}
	raw, err = client.GetBytesContext(ctx, r99ReadPaths[1])
	if err == nil {
		var groups []struct {
			Name        string  `json:"name"`
			DisplayName string  `json:"displayName"`
			Icons       []int64 `json:"icons"`
		}
		if json.Unmarshal(raw, &groups) == nil {
			for _, group := range groups {
				name := group.DisplayName
				if name == "" {
					name = group.Name
				}
				if name != "" {
					for _, id := range group.Icons {
						sets[id] = append(sets[id], name)
					}
				}
			}
		}
	}
	if terms == nil {
		terms = championPinyinTerms
	}
	var inventory []facadeIconInventoryEntry
	inventoryErr := client.RequestJSON(ctx, http.MethodGet, "/lol-inventory/v2/inventory/SUMMONER_ICON", nil, &inventory)
	unknown := inventoryErr != nil || len(inventory) == 0
	owned := map[int64]bool{}
	if !unknown {
		for _, item := range inventory {
			if item.Owned {
				owned[item.ItemID] = true
			}
		}
	}
	ownedCount := 0
	icons := make([]facadeIcon, 0, len(rows))
	for _, row := range rows {
		if owned[row.ID] {
			ownedCount++
		}
		if row.ID <= 0 || strings.TrimSpace(row.ImagePath) == "" {
			continue
		}
		icons = append(icons, facadeIcon{ID: row.ID, Title: row.Title, Year: row.Year, Legacy: row.Legacy, Disabled: len(row.DisabledRegions) > 0, Owned: owned[row.ID], ImagePath: row.ImagePath, Sets: append([]string{}, sets[row.ID]...), SearchTerms: terms(row.Title)})
	}
	sort.Slice(icons, func(i, j int) bool { return icons[i].ID < icons[j].ID })
	if len(icons) == 0 {
		return facadeIconCatalog{}, errors.New("头像目录暂不可用")
	}
	return facadeIconCatalog{Icons: icons, OwnershipUnavailable: unknown, Total: len(rows), OwnedCount: ownedCount}, nil
}
func (a *app) knownFacadeIcon(ctx context.Context, client *LCUClient, id int64) bool {
	icons, err := a.loadIconCatalog(ctx, client)
	if err != nil {
		return false
	}
	for _, icon := range icons {
		if icon.ID == id {
			return true
		}
	}
	return false
}
func (a *app) handleFacadeIcons(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, "未连接英雄联盟客户端", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	catalog, err := a.loadIconCatalogResult(ctx, client)
	if err != nil {
		http.Error(w, "头像目录暂时读取失败，请稍后重试", http.StatusBadGateway)
		return
	}
	// Only the public DTO leaves this handler; inventory metadata remains private.
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, catalog)
}
func (a *app) writeFacadeIcon(ctx context.Context, client *LCUClient, id int64) error {
	_, err := a.writeFacadeIconResult(ctx, client, id)
	return err
}

func (a *app) writeFacadeIconResult(ctx context.Context, client *LCUClient, id int64) (string, error) {
	if id <= 0 || !a.knownFacadeIcon(ctx, client, id) {
		return "", errFacadeInvalid
	}
	status, err := r99Request(ctx, client, http.MethodPut, "/lol-summoner/v1/current-summoner/icon", map[string]any{"profileIconId": id}, nil)
	if err != nil {
		var rejected *LCUHTTPError
		if errors.As(err, &rejected) && (rejected.StatusCode == 401 || rejected.StatusCode == 403) {
			// Chat presence has a separate icon field and supports unowned icons.
			// This does not unlock or replace the official summoner profile icon.
			if chatErr := writeFacadeChatIcon(ctx, client, id); chatErr == nil {
				return "chat", nil
			}
			return "", fmt.Errorf("客户端拒绝更换生涯头像（%d），聊天头像也未确认切换：%w", rejected.StatusCode, err)
		}
		return "", err
	}
	if status != http.StatusCreated {
		return "", errors.New("头像写入未返回 201，请在客户端核对后重试")
	}
	next, err := (SummonerAPI{client: client, ctx: ctx}).Current()
	for attempt := 0; attempt < 3 && (err != nil || next.ProfileIconID != id); attempt++ {
		if waitRiotDelay(ctx, 200*time.Millisecond) != nil {
			break
		}
		next, err = (SummonerAPI{client: client, ctx: ctx}).Current()
	}
	if err != nil {
		return "", errors.New("头像请求已提交，客户端资料暂时无法回读，请重新读取核对")
	}
	if next.ProfileIconID != id {
		return "", errors.New("头像请求已提交，但客户端尚未确认，请重新读取核对")
	}
	_, err = a.applySummonerIdentity(client, next, time.Now())
	return "profile", err
}

func writeFacadeChatIcon(ctx context.Context, client *LCUClient, id int64) error {
	if err := client.RequestJSON(ctx, http.MethodPut, "/lol-chat/v1/me", map[string]any{"icon": id}, nil); err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		var chat map[string]any
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-chat/v1/me", nil, &chat); err == nil && firstInt(chat, "icon") == id {
			return nil
		}
		if attempt < 3 && waitRiotDelay(ctx, 200*time.Millisecond) != nil {
			break
		}
	}
	return errors.New("聊天头像请求已提交，但客户端没有确认更改")
}

func writeFacadeRankBanner(ctx context.Context, client *LCUClient, value string) error {
	if value != "lastSeasonHighestRank" && value != "blank" {
		return errFacadeInvalid
	}
	var current map[string]any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-regalia/v2/current-summoner/regalia", nil, &current); err != nil {
		return err
	}
	if current["preferredCrestType"] == nil || current["selectedPrestigeCrest"] == nil {
		return errors.New("客户端边框偏好无法读取，已停止修改段位旗")
	}
	return client.RequestJSON(ctx, http.MethodPut, "/lol-regalia/v2/current-summoner/regalia", map[string]any{"preferredBannerType": value, "preferredCrestType": current["preferredCrestType"], "selectedPrestigeCrest": current["selectedPrestigeCrest"]}, nil)
}
