package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type facadeSkin struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ChampionID   int64  `json:"championId"`
	ChampionName string `json:"championName"`
	SplashPath   string `json:"splashPath,omitempty"`
	TilePath     string `json:"tilePath,omitempty"`
	Owned        bool   `json:"owned"`
}

type facadeState struct {
	Connected  bool                     `json:"connected"`
	Summoner   Summoner                 `json:"summoner"`
	Profile    SummonerProfile          `json:"profile"`
	Chat       map[string]any           `json:"chat"`
	Regalia    map[string]any           `json:"regalia"`
	Skins      []facadeSkin             `json:"skins"`
	LoginReset facadeLoginResetSettings `json:"loginReset"`
	Reason     string                   `json:"reason,omitempty"`
}

type facadeApplyRequest struct {
	Action        string                    `json:"action"`
	SkinID        int64                     `json:"skinId,omitempty"`
	Availability  string                    `json:"availability,omitempty"`
	StatusMessage *string                   `json:"statusMessage,omitempty"`
	Queue         string                    `json:"queue,omitempty"`
	Tier          string                    `json:"tier,omitempty"`
	Division      string                    `json:"division,omitempty"`
	LoginReset    *facadeLoginResetSettings `json:"loginReset,omitempty"`
}

func (a *app) loadFacadeState(ctx context.Context) facadeState {
	client, current, err := a.gameplayClient()
	if err != nil {
		return facadeState{Reason: "未连接英雄联盟客户端"}
	}
	state := facadeState{Connected: true, Summoner: current, Chat: map[string]any{}, Regalia: map[string]any{}, Skins: []facadeSkin{}}
	profile, _ := NewSummonerAPI(client).Profile()
	state.Profile = profile
	_ = client.RequestJSON(ctx, http.MethodGet, "/lol-chat/v1/me", nil, &state.Chat)
	_ = client.RequestJSON(ctx, http.MethodGet, "/lol-regalia/v2/current-summoner/regalia", nil, &state.Regalia)
	a.mu.RLock()
	for _, skin := range a.allSkins {
		if skin.ID <= 0 || skin.IsVariant {
			continue
		}
		state.Skins = append(state.Skins, facadeSkin{ID: skin.ID, Name: skin.Name, ChampionID: skin.ChampionID, ChampionName: skin.ChampionName, SplashPath: skin.SplashPath, TilePath: skin.TilePath, Owned: skin.Owned})
	}
	a.mu.RUnlock()
	if watch := a.activeWatch(); watch != nil {
		state.LoginReset = watch.currentWatch().Facade
	}
	return state
}

func (a *app) handleFacadeState(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, a.loadFacadeState(r.Context()))
}

func (a *app) handleFacadeApply(w http.ResponseWriter, r *http.Request) {
	var request facadeApplyRequest
	if err := decodeJSONRequest(r, &request, 32<<10); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	client, current, err := a.gameplayClient()
	if err != nil {
		http.Error(w, "未连接英雄联盟客户端", http.StatusConflict)
		return
	}
	a.noteFacadeManualAction()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := a.applyFacadeAction(ctx, client, current, request); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errFacadeInvalid) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	respondJSON(w, a.loadFacadeState(r.Context()))
}

var errFacadeInvalid = errors.New("门面操作参数无效")

func (a *app) applyFacadeAction(ctx context.Context, client *LCUClient, current Summoner, request facadeApplyRequest) error {
	switch request.Action {
	case "background":
		if request.SkinID <= 0 || !a.knownFacadeSkin(request.SkinID) {
			return errFacadeInvalid
		}
		return client.RequestJSON(ctx, http.MethodPost, "/lol-summoner/v1/current-summoner/summoner-profile", map[string]any{"key": "backgroundSkinId", "value": request.SkinID}, nil)
	case "chat":
		body := map[string]any{}
		if request.Availability != "" {
			if !validAvailability(request.Availability) {
				return errFacadeInvalid
			}
			body["availability"] = request.Availability
		}
		if request.StatusMessage != nil {
			message := strings.TrimSpace(*request.StatusMessage)
			if len(message) > 200 {
				return errFacadeInvalid
			}
			body["statusMessage"] = message
		}
		if len(body) == 0 {
			return errFacadeInvalid
		}
		return client.RequestJSON(ctx, http.MethodPut, "/lol-chat/v1/me", body, nil)
	case "rank":
		lol, err := facadeRankPayload(request.Queue, request.Tier, request.Division)
		if err != nil {
			return err
		}
		return client.RequestJSON(ctx, http.MethodPut, "/lol-chat/v1/me", map[string]any{"lol": lol}, nil)
	case "login-reset":
		if request.LoginReset == nil || a.activeWatch() == nil {
			return errFacadeInvalid
		}
		settings := a.activeWatch().currentWatch()
		settings.Facade = normalizeWatchSettings(watchSettings{Rules: settings.Rules, Facade: *request.LoginReset}).Facade
		settings.SchemaVersion = watchSettingsVersion
		settings.MasterEnabled = a.activeWatch().currentWatch().MasterEnabled
		if settings.Facade.RankEnabled {
			queue := anyString(settings.Facade.Rank, "rankedLeagueQueue")
			tier := anyString(settings.Facade.Rank, "rankedLeagueTier")
			division := anyString(settings.Facade.Rank, "rankedLeagueDivision")
			if _, err := facadeRankPayload(queue, tier, division); err != nil {
				return err
			}
		}
		if err := saveWatchSettings(a.storage, settings); err != nil {
			return err
		}
		a.activeWatch().apply(settings)
		return nil
	case "clear-border":
		if current.SummonerLevel < 526 {
			return errors.New("召唤师等级不足 526，无法卸下头像框")
		}
		var regalia map[string]any
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-regalia/v2/current-summoner/regalia", nil, &regalia); err != nil {
			return err
		}
		return client.RequestJSON(ctx, http.MethodPut, "/lol-regalia/v2/current-summoner/regalia", map[string]any{"preferredCrestType": "prestige", "preferredBannerType": regalia["preferredBannerType"], "selectedPrestigeCrest": 22}, nil)
	case "clear-challenges":
		var chat map[string]any
		_ = client.RequestJSON(ctx, http.MethodGet, "/lol-chat/v1/me", nil, &chat)
		lol, _ := chat["lol"].(map[string]any)
		return client.RequestJSON(ctx, http.MethodPost, "/lol-challenges/v1/update-player-preferences/", map[string]any{"challengeIds": []any{}, "bannerAccent": anyString(lol, "bannerIdSelected")}, nil)
	case "previous-banner":
		return client.RequestJSON(ctx, http.MethodPost, "/lol-challenges/v1/update-player-preferences/", map[string]any{"bannerAccent": "2"}, nil)
	case "clear-emotes":
		return clearFacadeEmotes(ctx, client)
	default:
		return errFacadeInvalid
	}
}

func (a *app) knownFacadeSkin(skinID int64) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, skin := range a.allSkins {
		if skin.ID == skinID {
			return true
		}
	}
	return false
}

func validAvailability(value string) bool {
	switch value {
	case "chat", "away", "dnd", "mobile", "offline", "online", "spectating", "inGame":
		return true
	default:
		return false
	}
}

func facadeRankPayload(queue, tier, division string) (map[string]any, error) {
	queue = strings.ToUpper(strings.TrimSpace(queue))
	tier = strings.ToUpper(strings.TrimSpace(tier))
	division = strings.ToUpper(strings.TrimSpace(division))
	if queue != "RANKED_SOLO_5X5" && queue != "RANKED_FLEX_SR" {
		return nil, errFacadeInvalid
	}
	validTier := map[string]bool{"UNRANKED": true, "IRON": true, "BRONZE": true, "SILVER": true, "GOLD": true, "PLATINUM": true, "EMERALD": true, "DIAMOND": true, "MASTER": true, "GRANDMASTER": true, "CHALLENGER": true}
	if !validTier[tier] {
		return nil, errFacadeInvalid
	}
	payload := map[string]any{"rankedLeagueQueue": queue, "rankedLeagueTier": tier}
	if tier != "MASTER" && tier != "GRANDMASTER" && tier != "CHALLENGER" && tier != "UNRANKED" {
		if division != "I" && division != "II" && division != "III" && division != "IV" {
			return nil, errFacadeInvalid
		}
		payload["rankedLeagueDivision"] = division
	}
	return payload, nil
}

func clearFacadeEmotes(ctx context.Context, client *LCUClient) error {
	var loadouts []map[string]any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-loadouts/v4/loadouts/scope/account", nil, &loadouts); err != nil {
		return err
	}
	if len(loadouts) == 0 {
		return errors.New("客户端没有返回表情轮盘配置")
	}
	id := anyString(loadouts[0], "id")
	current, _ := loadouts[0]["loadout"].(map[string]any)
	if !safeLCUIdentifier(id) || len(current) == 0 {
		return errors.New("客户端返回的表情轮盘格式无法识别")
	}
	cleared := map[string]any{}
	for key := range current {
		if strings.HasPrefix(strings.ToUpper(key), "EMOTES_") {
			cleared[key] = map[string]any{"inventoryType": "EMOTE", "itemId": -1}
		}
	}
	if len(cleared) == 0 {
		return errors.New("客户端没有返回可清理的表情位")
	}
	path := "/lol-loadouts/v4/loadouts/" + id
	return client.RequestJSON(ctx, http.MethodPatch, path, map[string]any{"loadout": cleared}, nil)
}

func (a *app) noteFacadeManualAction() {
	a.facadeMu.Lock()
	a.facadeManualVersion++
	a.facadeMu.Unlock()
}

func (a *app) scheduleFacadeLoginReset(ctx context.Context, client *LCUClient) {
	watch := a.activeWatch()
	if watch == nil {
		return
	}
	reset := watch.currentWatch().Facade
	if !reset.StatusMessageEnabled && !reset.RankEnabled {
		return
	}
	a.facadeMu.Lock()
	version := a.facadeManualVersion
	a.facadeMu.Unlock()
	go func() {
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		a.facadeMu.Lock()
		preempted := version != a.facadeManualVersion
		a.facadeMu.Unlock()
		if preempted {
			return
		}
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		if reset.StatusMessageEnabled {
			if err := client.RequestJSON(requestCtx, http.MethodPut, "/lol-chat/v1/me", map[string]any{"statusMessage": reset.StatusMessage}, nil); err != nil {
				a.broadcastEvent("watch:failed:facade-status-message")
			}
		}
		if reset.RankEnabled {
			queue := anyString(reset.Rank, "rankedLeagueQueue")
			tier := anyString(reset.Rank, "rankedLeagueTier")
			division := anyString(reset.Rank, "rankedLeagueDivision")
			if lol, err := facadeRankPayload(queue, tier, division); err == nil {
				if err := client.RequestJSON(requestCtx, http.MethodPut, "/lol-chat/v1/me", map[string]any{"lol": lol}, nil); err != nil {
					a.broadcastEvent("watch:failed:facade-rank")
				}
			}
		}
	}()
}

func facadeImagePath(value string) string {
	if value == "" {
		return ""
	}
	return "/api/image?path=" + value
}

func facadeLevelLabel(level int64) string {
	return strconv.FormatInt(level, 10)
}
