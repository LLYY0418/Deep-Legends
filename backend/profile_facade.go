package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type facadeSkin struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	ChampionID      int64  `json:"championId"`
	ChampionName    string `json:"championName"`
	SplashPath      string `json:"splashPath,omitempty"`
	TilePath        string `json:"tilePath,omitempty"`
	Owned           bool   `json:"owned"`
	ReleaseDate     string `json:"releaseDate,omitempty"`
	ReleaseSortDate string `json:"releaseSortDate,omitempty"`
	ParentSkinID    int64  `json:"parentSkinId,omitempty"`
	IsVariant       bool   `json:"isVariant,omitempty"`
}

type facadeChallenge struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IconPath string `json:"iconPath,omitempty"`
}

type facadeSummoner struct {
	DisplayName   string `json:"displayName,omitempty"`
	GameName      string `json:"gameName,omitempty"`
	TagLine       string `json:"tagLine,omitempty"`
	ProfileIconID int64  `json:"profileIconId,omitempty"`
	SummonerLevel int64  `json:"summonerLevel,omitempty"`
}

type facadeProfile struct {
	BackgroundPath       string `json:"backgroundPath,omitempty"`
	BackgroundType       string `json:"backgroundType,omitempty"`
	BackgroundSkinID     int64  `json:"backgroundSkinId,omitempty"`
	BackgroundChampionID int64  `json:"backgroundChampionId,omitempty"`
	BackgroundSkinName   string `json:"backgroundSkinName,omitempty"`
}

type facadeChatLOL struct {
	RankedLeagueQueue    string `json:"rankedLeagueQueue,omitempty"`
	RankedLeagueTier     string `json:"rankedLeagueTier,omitempty"`
	RankedLeagueDivision string `json:"rankedLeagueDivision,omitempty"`
}

type facadeChat struct {
	Icon          int64         `json:"icon,omitempty"`
	Availability  string        `json:"availability,omitempty"`
	StatusMessage string        `json:"statusMessage,omitempty"`
	LOL           facadeChatLOL `json:"lol"`
}

type facadeState struct {
	RankBanner               string                   `json:"rankBanner,omitempty"`
	BannerAccent             string                   `json:"bannerAccent,omitempty"`
	SkinsUnavailable         bool                     `json:"skinsUnavailable,omitempty"`
	SkinOwnershipUnavailable bool                     `json:"skinOwnershipUnavailable,omitempty"`
	ProfileUnavailable       bool                     `json:"profileUnavailable,omitempty"`
	Connected                bool                     `json:"connected"`
	Summoner                 facadeSummoner           `json:"summoner"`
	Profile                  facadeProfile            `json:"profile"`
	Chat                     facadeChat               `json:"chat"`
	ChallengeSummary         map[string]any           `json:"challengeSummary"`
	Challenges               []facadeChallenge        `json:"challenges"`
	ChallengesReady          bool                     `json:"challengesReady"`
	Skins                    []facadeSkin             `json:"skins"`
	LoginReset               facadeLoginResetSettings `json:"loginReset"`
	Reason                   string                   `json:"reason,omitempty"`
}

type facadeApplyRequest struct {
	RankBanner    string                    `json:"rankBanner,omitempty"`
	BannerAccent  string                    `json:"bannerAccent,omitempty"`
	Action        string                    `json:"action"`
	SkinID        int64                     `json:"skinId,omitempty"`
	Availability  string                    `json:"availability,omitempty"`
	StatusMessage *string                   `json:"statusMessage,omitempty"`
	Queue         string                    `json:"queue,omitempty"`
	Tier          string                    `json:"tier,omitempty"`
	Division      string                    `json:"division,omitempty"`
	LoginReset    *facadeLoginResetSettings `json:"loginReset,omitempty"`
}

type facadeApplyResult struct {
	TitleRestore        string
	TitleAttemptStatus  []int
	TitleHasTitle       bool
	TitleCandidateCount int
}

type facadeTitleRestoreCandidate struct {
	Value      any
	Diagnostic string
}

type facadeTitleRestoreRefusedError struct {
	cause error
}

type facadeTitleRestoreNoCandidateError struct{}

func (e *facadeTitleRestoreRefusedError) Error() string {
	return "无法在保留头衔的前提下完成操作：客户端拒绝了所有头衔回填方式。如果你不介意头衔一并卸下，请先手动卸下头衔再执行本操作。"
}

func (e *facadeTitleRestoreRefusedError) Unwrap() error {
	return e.cause
}

func (e *facadeTitleRestoreNoCandidateError) Error() string {
	return "客户端返回的头衔数据缺少可回填的标识（itemId / contentId 都为空），为避免把你的头衔一起清掉，本次操作已中止。"
}

func (a *app) loadFacadeState(ctx context.Context) facadeState {
	return a.loadFacadeStateTriggered(ctx, "poll")
}

func (a *app) loadFacadeStateTriggered(ctx context.Context, trigger string) facadeState {
	started := time.Now()
	var chatMS, challengesMS, catalogMS int64
	defer func() {
		a.recordDiagnostic(map[string]any{
			"event": "facade_load_cost", "trigger": facadeLoadTrigger(trigger),
			"chat_ms": chatMS, "challenges_ms": challengesMS, "catalog_ms": catalogMS,
			"total_ms": time.Since(started).Milliseconds(),
		})
	}()
	client, current, err := a.gameplayClient()
	if err != nil {
		return facadeState{Reason: "未连接英雄联盟客户端"}
	}
	state := facadeState{Connected: true, Summoner: projectFacadeSummoner(current), ChallengeSummary: map[string]any{"title": nil}, Challenges: []facadeChallenge{}, ChallengesReady: true, Skins: []facadeSkin{}}
	profile, profileCapability := a.loadFacadeProfile(ctx, client, current)
	state.ProfileUnavailable = profileCapability.State != capabilityAvailable
	state.Profile = projectFacadeProfile(profile)
	chatRaw := map[string]any{}
	chatStarted := time.Now()
	chatErr := client.RequestJSON(ctx, http.MethodGet, "/lol-chat/v1/me", nil, &chatRaw)
	chatMS = time.Since(chatStarted).Milliseconds()
	state.Chat = projectFacadeChat(chatRaw)
	identityShapeDiagnostic := a.claimFacadeIdentityShape(client)
	if identityShapeDiagnostic {
		go func() {
			probeCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			a.recordR99SurfaceShape(probeCtx, client)
		}()
	}
	regaliaRaw := map[string]any{}
	regaliaErr := client.RequestJSON(ctx, http.MethodGet, "/lol-regalia/v2/current-summoner/regalia", nil, &regaliaRaw)
	state.RankBanner = anyString(regaliaRaw, "preferredBannerType")
	state.BannerAccent = anyString(anyMap(chatRaw["lol"]), "bannerIdSelected")
	var challengeSummaryRaw json.RawMessage
	challengesStarted := time.Now()
	challengeSummaryErr := client.RequestJSON(ctx, http.MethodGet, "/lol-challenges/v1/summary-player-data/local-player", nil, &challengeSummaryRaw)
	challengesMS = time.Since(challengesStarted).Milliseconds()
	challengeSummary := map[string]any{}
	if challengeSummaryErr == nil {
		_ = json.Unmarshal(challengeSummaryRaw, &challengeSummary)
	}
	if accent, ok := challengeSummary["bannerId"]; ok {
		state.BannerAccent = facadeBannerIdentity(accent)
	}
	state.ChallengeSummary["title"] = projectFacadeTitle(challengeSummary["title"])
	catalogStarted := time.Now()
	challengeCatalog, challengeCatalogRaw, challengeCatalogErr := a.loadFacadeChallengeCatalog(ctx, client)
	catalogMS = time.Since(catalogStarted).Milliseconds()
	state.Challenges = facadeSelectedChallenges(challengeSummary, challengeCatalog)
	if len(state.Challenges) == 0 {
		state.Challenges = facadeTopChallenges(challengeSummary)
	}
	state.ChallengesReady = facadeChallengesReady(challengeSummary, challengeSummaryErr)
	if identityShapeDiagnostic {
		a.recordFacadeIdentityShape(chatRaw, regaliaRaw, chatErr, regaliaErr, challengeSummaryRaw, challengeSummaryErr, challengeCatalogRaw, challengeCatalogErr)
	}
	skins, source, skinErr := a.loadFacadeSkins(ctx, client)
	state.SkinsUnavailable = skinErr != nil || len(skins) == 0
	state.SkinOwnershipUnavailable = source != "collection"
	for _, skin := range skins {
		if skin.ID <= 0 {
			continue
		}
		state.Skins = append(state.Skins, projectFacadeSkin(skin))
	}
	a.applyFacadeBackdrop(ctx, client, current, &state)
	backgroundInCatalog := false
	for _, skin := range state.Skins {
		if skin.ID == state.Profile.BackgroundSkinID {
			backgroundInCatalog = true
			break
		}
	}
	a.recordDiagnostic(map[string]any{"event": "facade_skin_state", "trigger": facadeLoadTrigger(trigger), "source": source, "skin_count": len(state.Skins), "unavailable": state.SkinsUnavailable, "profile_available": !state.ProfileUnavailable, "configured_skin_id": profile.BackgroundSkinID, "background_skin_id": state.Profile.BackgroundSkinID, "background_champion_id": state.Profile.BackgroundChampionID, "background_in_catalog": backgroundInCatalog})
	if watch := a.activeWatch(); watch != nil {
		state.LoginReset = watch.currentWatch().Facade
	}
	return state
}

func projectFacadeSummoner(value Summoner) facadeSummoner {
	return facadeSummoner{
		DisplayName: value.DisplayName, GameName: value.GameName, TagLine: value.TagLine,
		ProfileIconID: value.ProfileIconID, SummonerLevel: value.SummonerLevel,
	}
}

func projectFacadeProfile(value SummonerProfile) facadeProfile {
	return facadeProfile{BackgroundSkinID: value.BackgroundSkinID, BackgroundSkinName: value.BackgroundSkinName}
}

func projectFacadeChat(value map[string]any) facadeChat {
	lol := anyMap(value["lol"])
	return facadeChat{
		Icon:          firstInt(value, "icon"),
		Availability:  anyString(value, "availability"),
		StatusMessage: anyString(value, "statusMessage"),
		LOL: facadeChatLOL{
			RankedLeagueQueue:    anyString(lol, "rankedLeagueQueue"),
			RankedLeagueTier:     anyString(lol, "rankedLeagueTier"),
			RankedLeagueDivision: anyString(lol, "rankedLeagueDivision"),
		},
	}
}

func projectFacadeTitle(value any) any {
	switch typed := value.(type) {
	case string:
		if title := strings.TrimSpace(typed); title != "" {
			return title
		}
	case map[string]any:
		if name := anyString(typed, "name"); name != "" {
			return map[string]any{"name": name}
		}
	case json.Number:
		return typed
	case float64:
		return typed
	}
	return nil
}

const facadeChallengeCatalogRetryDelay = 5 * time.Second

func (a *app) loadFacadeChallengeCatalog(ctx context.Context, client *LCUClient) (map[string]facadeChallenge, json.RawMessage, error) {
	if a == nil || client == nil {
		return nil, nil, errors.New("challenge catalog unavailable")
	}
	for {
		a.facadeChallengeCatalogMu.Lock()
		if a.facadeChallengeCatalogClient != client {
			a.facadeChallengeCatalogClient = client
			a.facadeChallengeCatalogAttempted = false
			a.facadeChallengeCatalogBackoffUntil = time.Time{}
			a.facadeChallengeCatalog = nil
			a.facadeChallengeCatalogRaw = nil
			a.facadeChallengeCatalogErr = nil
			a.facadeChallengeCatalogFlight = nil
		}
		if a.facadeChallengeCatalogAttempted {
			catalog, raw, err := a.facadeChallengeCatalog, append(json.RawMessage(nil), a.facadeChallengeCatalogRaw...), a.facadeChallengeCatalogErr
			a.facadeChallengeCatalogMu.Unlock()
			return catalog, raw, err
		}
		if time.Now().Before(a.facadeChallengeCatalogBackoffUntil) {
			raw := append(json.RawMessage(nil), a.facadeChallengeCatalogRaw...)
			err := a.facadeChallengeCatalogErr
			a.facadeChallengeCatalogMu.Unlock()
			return nil, raw, err
		}
		if flight := a.facadeChallengeCatalogFlight; flight != nil {
			a.facadeChallengeCatalogMu.Unlock()
			select {
			case <-flight:
				continue
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}
		flight := make(chan struct{})
		a.facadeChallengeCatalogFlight = flight
		a.facadeChallengeCatalogMu.Unlock()

		var raw json.RawMessage
		err := client.RequestJSON(ctx, http.MethodGet, "/lol-challenges/v1/challenges/local-player", nil, &raw)
		var catalog map[string]facadeChallenge
		if err == nil {
			catalog = parseFacadeChallengeCatalog(raw)
		}

		a.facadeChallengeCatalogMu.Lock()
		if a.facadeChallengeCatalogClient == client && a.facadeChallengeCatalogFlight == flight {
			a.facadeChallengeCatalogRaw = append(json.RawMessage(nil), raw...)
			a.facadeChallengeCatalogErr = err
			if err == nil {
				a.facadeChallengeCatalog = catalog
				a.facadeChallengeCatalogAttempted = true
				a.facadeChallengeCatalogBackoffUntil = time.Time{}
			} else {
				a.facadeChallengeCatalogAttempted = false
				a.facadeChallengeCatalogBackoffUntil = time.Now().Add(facadeChallengeCatalogRetryDelay)
			}
			a.facadeChallengeCatalogFlight = nil
			close(flight)
		} else {
			close(flight)
		}
		a.facadeChallengeCatalogMu.Unlock()
		return catalog, append(json.RawMessage(nil), raw...), err
	}
}

func (a *app) clearFacadeChallengeCatalog(client *LCUClient) {
	if a == nil {
		return
	}
	a.facadeChallengeCatalogMu.Lock()
	defer a.facadeChallengeCatalogMu.Unlock()
	if client != nil && a.facadeChallengeCatalogClient != client {
		return
	}
	a.facadeChallengeCatalogClient = nil
	a.facadeChallengeCatalogAttempted = false
	a.facadeChallengeCatalogBackoffUntil = time.Time{}
	a.facadeChallengeCatalogFlight = nil
	a.facadeChallengeCatalog = nil
	a.facadeChallengeCatalogRaw = nil
	a.facadeChallengeCatalogErr = nil
}

func parseFacadeChallengeCatalog(raw json.RawMessage) map[string]facadeChallenge {
	result := map[string]facadeChallenge{}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil && object != nil {
		for id, entryRaw := range object {
			if entry, ok := parseFacadeChallenge(entryRaw, id); ok {
				result[id] = entry
			}
		}
		return result
	}
	var array []json.RawMessage
	if json.Unmarshal(raw, &array) != nil {
		return result
	}
	for _, entryRaw := range array {
		if entry, ok := parseFacadeChallenge(entryRaw, ""); ok && entry.ID != "" {
			result[entry.ID] = entry
		}
	}
	return result
}

func parseFacadeChallenge(raw json.RawMessage, fallbackID string) (facadeChallenge, bool) {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return facadeChallenge{}, false
	}
	name, _ := value["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return facadeChallenge{}, false
	}
	id := facadeChallengeID(value["id"])
	if id == "" {
		id = facadeChallengeID(value["challengeId"])
	}
	if id == "" {
		id = strings.TrimSpace(fallbackID)
	}
	iconPath, _ := value["iconPath"].(string)
	return facadeChallenge{ID: id, Name: name, IconPath: strings.TrimSpace(iconPath)}, true
}

func facadeChallengeID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed >= 0 && typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	case json.Number:
		return string(typed)
	}
	return ""
}

func facadeSelectedChallenges(summary map[string]any, catalog map[string]facadeChallenge) []facadeChallenge {
	selectedIDs, valid := facadeSelectedChallengeIDs(summary)
	if !valid || len(selectedIDs) == 0 || len(catalog) == 0 {
		return nil
	}
	result := make([]facadeChallenge, 0, 3)
	for _, id := range selectedIDs {
		challenge, found := catalog[id]
		if !found || strings.TrimSpace(challenge.Name) == "" {
			return nil
		}
		result = append(result, challenge)
		if len(result) == 3 {
			break
		}
	}
	return result
}

func facadeChallengeIDIsValid(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func facadeTopChallenges(summary map[string]any) []facadeChallenge {
	items, ok := summary["topChallenges"].([]any)
	if !ok {
		return nil
	}
	result := make([]facadeChallenge, 0, 3)
	for _, item := range items {
		value, ok := item.(map[string]any)
		if !ok {
			continue
		}
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		challenge, ok := parseFacadeChallenge(raw, "")
		if !ok {
			continue
		}
		result = append(result, challenge)
		if len(result) == 3 {
			break
		}
	}
	return result
}

func facadeChallengesReady(summary map[string]any, summaryErr error) bool {
	if summaryErr != nil {
		return true
	}
	if facadeTitlePresent(summary["title"]) {
		return true
	}
	if top, ok := summary["topChallenges"].([]any); ok && len(top) > 0 {
		return true
	}
	selected, _ := summary["selectedChallengesString"].(string)
	return strings.TrimSpace(selected) != ""
}

func facadeTitlePresent(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case float64:
		return true
	case map[string]any:
		name, _ := typed["name"].(string)
		return strings.TrimSpace(name) != ""
	default:
		return false
	}
}

func (a *app) claimFacadeIdentityShape(client *LCUClient) bool {
	if a == nil || client == nil {
		return false
	}
	a.facadeIdentityShapeDiagnosticMu.Lock()
	defer a.facadeIdentityShapeDiagnosticMu.Unlock()
	if a.facadeIdentityShapeDiagnosticClient == client {
		return false
	}
	a.facadeIdentityShapeDiagnosticClient = client
	return true
}

func (a *app) recordFacadeIdentityShape(chat, regalia map[string]any, chatErr, regaliaErr error, summaryRaw json.RawMessage, summaryErr error, catalogRaw json.RawMessage, catalogErr error) {
	if a == nil {
		return
	}

	summaryKeys, summaryItemKeys, summaryLength := facadeJSONShape(summaryRaw)
	titleRaw, topChallengesRaw, selectedChallengesRaw, categoryProgressRaw := facadeSummaryFields(summaryRaw)
	catalogType, catalogKeyCount, catalogItemKeys := facadeChallengeCatalogShape(catalogRaw)
	event := map[string]any{
		"event":                                      "facade_identity_shape",
		"chat_top_level_keys":                        diagnosticMapKeys(chat),
		"chat_lol_keys":                              diagnosticMapKeys(anyMap(chat["lol"])),
		"chat_status":                                facadeDiagnosticStatus(chatErr),
		"regalia_keys":                               diagnosticMapKeys(regalia),
		"regalia_status":                             facadeDiagnosticStatus(regaliaErr),
		"challenge_summary_keys":                     summaryKeys,
		"challenge_summary_item_keys":                summaryItemKeys,
		"challenge_summary_length":                   summaryLength,
		"challenge_summary_status":                   facadeDiagnosticStatus(summaryErr),
		"challenge_summary_result":                   facadeDiagnosticResult(summaryErr),
		"challenge_summary_title_type":               diagnosticJSONType(titleRaw),
		"challenge_summary_title_keys":               diagnosticObjectKeySet(titleRaw),
		"challenge_summary_top_challenges_length":    diagnosticArrayLength(topChallengesRaw),
		"challenge_summary_top_challenges_item_keys": diagnosticArrayKeySet(topChallengesRaw),
		"challenge_summary_selected_type":            diagnosticJSONType(selectedChallengesRaw),
		"challenge_summary_selected_segments":        diagnosticStringSegments(selectedChallengesRaw),
		"challenge_summary_category_progress_length": diagnosticArrayLength(categoryProgressRaw),
		"challenge_catalog_status":                   facadeDiagnosticStatus(catalogErr),
		"challenge_catalog_result":                   facadeDiagnosticResult(catalogErr),
		"challenge_catalog_type":                     catalogType,
		"challenge_catalog_key_count":                catalogKeyCount,
		"challenge_catalog_item_keys":                catalogItemKeys,
	}
	if availability, ok := chat["availability"].(string); ok && strings.TrimSpace(availability) != "" {
		event["availability"] = strings.TrimSpace(availability)
	}
	lol := anyMap(chat["lol"])
	if gameStatus := diagnosticFacadeEnum(lol["gameStatus"]); gameStatus != "" {
		event["game_status"] = gameStatus
	}
	a.recordDiagnostic(event)
}

func facadeSummaryFields(raw json.RawMessage) (json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage) {
	var summary map[string]json.RawMessage
	if json.Unmarshal(raw, &summary) != nil {
		return nil, nil, nil, nil
	}
	return summary["title"], summary["topChallenges"], summary["selectedChallengesString"], summary["categoryProgress"]
}

func diagnosticStringSegments(raw json.RawMessage) int {
	var value string
	if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return 0
	}
	return len(strings.Split(value, ","))
}

func facadeChallengeCatalogShape(raw json.RawMessage) (string, int, []string) {
	catalogType := diagnosticJSONType(raw)
	switch catalogType {
	case "object":
		var entries map[string]json.RawMessage
		if json.Unmarshal(raw, &entries) != nil {
			return "invalid", 0, nil
		}
		for _, entry := range entries {
			return catalogType, len(entries), diagnosticKeySet(entry)
		}
		return catalogType, 0, nil
	case "array":
		var entries []json.RawMessage
		if json.Unmarshal(raw, &entries) != nil {
			return "invalid", 0, nil
		}
		if len(entries) == 0 {
			return catalogType, 0, nil
		}
		return catalogType, 0, diagnosticKeySet(entries[0])
	default:
		return catalogType, 0, nil
	}
}

func diagnosticJSONType(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "missing"
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "invalid"
	}
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case bool:
		return "boolean"
	default:
		return "unknown"
	}
}

func diagnosticObjectKeySet(raw json.RawMessage) []string {
	if diagnosticJSONType(raw) != "object" {
		return nil
	}
	return diagnosticKeySet(raw)
}

func diagnosticFacadeEnum(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 48 {
		return ""
	}
	for _, char := range text {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return ""
	}
	return text
}

func anyMap(value any) map[string]any {
	if result, ok := value.(map[string]any); ok {
		return result
	}
	return nil
}

func diagnosticMapKeys(value map[string]any) []string {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return diagnosticKeySet(raw)
}

func facadeJSONShape(raw json.RawMessage) ([]string, []string, int) {
	if len(raw) == 0 {
		return nil, nil, 0
	}
	return diagnosticKeySet(raw), diagnosticArrayKeySet(raw), diagnosticArrayLength(raw)
}

func diagnosticArrayKeySet(raw json.RawMessage) []string {
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return nil
	}
	return diagnosticKeyUnion(entries)
}

func diagnosticArrayLength(raw json.RawMessage) int {
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return 0
	}
	return len(entries)
}

func facadeDiagnosticStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var httpErr *LCUHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode
	}
	return 0
}

func facadeDiagnosticResult(err error) string {
	if err == nil {
		return "ok"
	}
	if facadeDiagnosticStatus(err) == http.StatusNotFound {
		return "not-found"
	}
	return "failed"
}

func facadeLoadTrigger(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "manual", "sse", "poll":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "poll"
	}
}

func (a *app) handleFacadeState(w http.ResponseWriter, r *http.Request) {
	trigger := facadeLoadTrigger(r.URL.Query().Get("trigger"))
	value, err := a.cachedFacadeState(r.Context(), trigger != "poll", trigger)
	if err != nil {
		http.Error(w, "生涯资料暂时读取失败，请重试", http.StatusGatewayTimeout)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, value)
}

func (a *app) handleFacadeApply(w http.ResponseWriter, r *http.Request) {
	var request facadeApplyRequest
	if err := decodeJSONRequest(r, &request, 32<<10); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	client, current, err := a.gameplayClient()
	if err != nil {
		a.recordDiagnostic(facadeApplyDiagnostic(request.Action, "not-connected", facadeApplyResult{}))
		http.Error(w, "未连接英雄联盟客户端", http.StatusConflict)
		return
	}
	if !a.facadeProbeMu.TryLock() {
		http.Error(w, "生涯操作正在进行", http.StatusConflict)
		return
	}
	defer a.facadeProbeMu.Unlock()
	a.noteFacadeManualAction()
	a.invalidateFacadeView()
	defer a.invalidateFacadeView()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	applyResult, err := a.applyFacadeActionResultDetails(ctx, client, current, request)
	if err != nil {
		status := http.StatusBadGateway
		result := "failed"
		if errors.Is(err, errFacadeInvalid) {
			status = http.StatusBadRequest
			result = "invalid"
		}
		diagnostic := facadeApplyDiagnostic(request.Action, result, applyResult)
		var httpErr *LCUHTTPError
		if errors.As(err, &httpErr) {
			diagnostic["status_code"] = httpErr.StatusCode
		}
		a.recordDiagnostic(diagnostic)
		http.Error(w, err.Error(), status)
		return
	}
	diagnostic := facadeApplyDiagnostic(request.Action, "ok", applyResult)
	next := a.loadFacadeStateTriggered(r.Context(), "manual")
	if request.Action == "background" {
		diagnostic["requested_skin_id"] = request.SkinID
		diagnostic["observed_skin_id"] = next.Profile.BackgroundSkinID
		diagnostic["profile_available"] = !next.ProfileUnavailable
		diagnostic["background_confirmed"] = !next.ProfileUnavailable && next.Profile.BackgroundSkinID == request.SkinID
	}
	a.recordDiagnostic(diagnostic)
	respondJSON(w, next)
}

var errFacadeInvalid = errors.New("生涯操作参数无效")

func facadeDiagnosticAction(action string) string {
	switch action {
	case "rank-banner", "background", "chat", "rank", "login-reset", "clear-border", "clear-challenges", "clear-title", "clear-emotes", "clear-objectives":
		return action
	default:
		return "unknown"
	}
}

func facadeApplyDiagnostic(action, result string, applyResult facadeApplyResult) map[string]any {
	diagnostic := map[string]any{
		"event": "facade_apply", "action": facadeDiagnosticAction(action), "result": result,
		"title_attempt_status":  append([]int{}, applyResult.TitleAttemptStatus...),
		"title_attempt_count":   len(applyResult.TitleAttemptStatus),
		"title_has_title":       applyResult.TitleHasTitle,
		"title_candidate_count": applyResult.TitleCandidateCount,
	}
	if applyResult.TitleRestore != "" {
		diagnostic["title_restore"] = facadeTitleRestoreDiagnostic(applyResult.TitleRestore)
	}
	return diagnostic
}

func (a *app) applyFacadeAction(ctx context.Context, client *LCUClient, current Summoner, request facadeApplyRequest) error {
	_, err := a.applyFacadeActionResult(ctx, client, current, request)
	return err
}

func (a *app) applyFacadeActionResult(ctx context.Context, client *LCUClient, current Summoner, request facadeApplyRequest) (string, error) {
	result, err := a.applyFacadeActionResultDetails(ctx, client, current, request)
	return result.TitleRestore, err
}

func (a *app) applyFacadeActionResultDetails(ctx context.Context, client *LCUClient, current Summoner, request facadeApplyRequest) (facadeApplyResult, error) {
	switch request.Action {
	case "rank-banner":
		return facadeApplyResult{}, writeFacadeRankBanner(ctx, client, request.RankBanner)
	case "clear-objectives":
		return facadeApplyResult{}, a.clearObjectiveBadge(ctx, client)
	case "background":
		if request.SkinID <= 0 || !a.knownFacadeSkin(ctx, client, request.SkinID) {
			return facadeApplyResult{}, errFacadeInvalid
		}
		return facadeApplyResult{}, client.RequestJSON(ctx, http.MethodPost, "/lol-summoner/v1/current-summoner/summoner-profile", map[string]any{"key": "backgroundSkinId", "value": request.SkinID}, nil)
	case "chat":
		body := map[string]any{}
		if request.Availability != "" {
			if !validAvailability(request.Availability) {
				return facadeApplyResult{}, errFacadeInvalid
			}
			body["availability"] = request.Availability
		}
		if request.StatusMessage != nil {
			message := strings.TrimSpace(*request.StatusMessage)
			if len(message) > 200 {
				return facadeApplyResult{}, errFacadeInvalid
			}
			body["statusMessage"] = message
		}
		if len(body) == 0 {
			return facadeApplyResult{}, errFacadeInvalid
		}
		return facadeApplyResult{}, client.RequestJSON(ctx, http.MethodPut, "/lol-chat/v1/me", body, nil)
	case "rank":
		lol, err := facadeRankPayload(request.Queue, request.Tier, request.Division)
		if err != nil {
			return facadeApplyResult{}, err
		}
		return facadeApplyResult{}, client.RequestJSON(ctx, http.MethodPut, "/lol-chat/v1/me", map[string]any{"lol": lol}, nil)
	case "login-reset":
		if request.LoginReset == nil || a.activeWatch() == nil {
			return facadeApplyResult{}, errFacadeInvalid
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
				return facadeApplyResult{}, err
			}
		}
		if err := saveWatchSettings(a.storage, settings); err != nil {
			return facadeApplyResult{}, err
		}
		a.activeWatch().apply(settings)
		return facadeApplyResult{}, nil
	case "clear-border":
		if current.SummonerLevel < 526 {
			return facadeApplyResult{}, errors.New("召唤师等级不足 526，无法卸下头像框")
		}
		var regalia map[string]any
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-regalia/v2/current-summoner/regalia", nil, &regalia); err != nil {
			return facadeApplyResult{}, err
		}
		return facadeApplyResult{}, client.RequestJSON(ctx, http.MethodPut, "/lol-regalia/v2/current-summoner/regalia", map[string]any{"preferredCrestType": "prestige", "preferredBannerType": regalia["preferredBannerType"], "selectedPrestigeCrest": 22}, nil)
	case "clear-challenges":
		return a.writeChallengePreferences(ctx, client, map[string]any{"challengeIds": []any{}})
	case "clear-title":
		return a.writeChallengePreferences(ctx, client, map[string]any{"title": ""})
	case "clear-emotes":
		return facadeApplyResult{}, clearFacadeEmotes(ctx, client)
	default:
		return facadeApplyResult{}, errFacadeInvalid
	}
}

func (a *app) writeChallengePreferences(ctx context.Context, client *LCUClient, changes map[string]any) (facadeApplyResult, error) {
	var summary map[string]any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-challenges/v1/summary-player-data/local-player", nil, &summary); err != nil {
		return facadeApplyResult{TitleRestore: "not-set"}, err
	}
	selectedIDs, valid := facadeSelectedChallengeIDs(summary)
	if !valid {
		return facadeApplyResult{TitleRestore: "not-set"}, errors.New("客户端挑战展示数据格式无效，已停止写入")
	}
	challengeIDs := make([]any, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		value, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return facadeApplyResult{TitleRestore: "not-set"}, errors.New("客户端挑战展示数据格式无效，已停止写入")
		}
		challengeIDs = append(challengeIDs, value)
	}
	var chat map[string]any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-chat/v1/me", nil, &chat); err != nil {
		return facadeApplyResult{TitleRestore: "not-set"}, err
	}
	lol := anyMap(chat["lol"])
	candidates, hasTitle := facadeTitleRestorePlan(summary, lol)
	result := facadeApplyResult{TitleHasTitle: hasTitle, TitleCandidateCount: len(candidates)}
	body := map[string]any{
		"challengeIds": challengeIDs,
		"bannerAccent": anyString(lol, "bannerIdSelected"),
	}
	if accent, ok := summary["bannerId"]; ok {
		body["bannerAccent"] = accent
	}
	if value, ok := summary["crestId"]; ok {
		body["crestBorder"] = value
	}
	if value, ok := summary["prestigeCrestBorderLevel"]; ok {
		body["prestigeCrestBorderLevel"] = value
	}
	_, changesTitle := changes["title"]
	for key, value := range changes {
		body[key] = value
	}
	if changesTitle {
		result.TitleRestore = "not-set"
		err := client.RequestJSON(ctx, http.MethodPost, "/lol-challenges/v1/update-player-preferences/", body, nil)
		if err == nil {
			return result, nil
		}
		result.TitleAttemptStatus = appendFacadeTitleAttemptStatus(result.TitleAttemptStatus, err)
		if title := changes["title"]; title == "" {
			body["title"] = nil
			err = client.RequestJSON(ctx, http.MethodPost, "/lol-challenges/v1/update-player-preferences/", body, nil)
			if err != nil {
				result.TitleAttemptStatus = appendFacadeTitleAttemptStatus(result.TitleAttemptStatus, err)
			}
		}
		return result, err
	}

	if !hasTitle {
		err := client.RequestJSON(ctx, http.MethodPost, "/lol-challenges/v1/update-player-preferences/", body, nil)
		result.TitleRestore = "not-set"
		return result, err
	}

	if len(candidates) == 0 {
		result.TitleRestore = "no-candidate"
		return result, &facadeTitleRestoreNoCandidateError{}
	}
	result.TitleRestore = "refused"
	var lastErr error
	for _, candidate := range candidates {
		body["title"] = candidate.Value
		lastErr = client.RequestJSON(ctx, http.MethodPost, "/lol-challenges/v1/update-player-preferences/", body, nil)
		if lastErr == nil {
			result.TitleRestore = candidate.Diagnostic
			return result, nil
		}
		result.TitleAttemptStatus = appendFacadeTitleAttemptStatus(result.TitleAttemptStatus, lastErr)
		if !facadeHTTP4xx(lastErr) {
			result.TitleRestore = candidate.Diagnostic
			return result, lastErr
		}
	}
	return result, &facadeTitleRestoreRefusedError{cause: lastErr}
}

func facadeSelectedChallengeIDs(summary map[string]any) ([]string, bool) {
	selected, ok := summary["selectedChallengesString"].(string)
	if !ok {
		return nil, false
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return []string{}, true
	}
	parts := strings.Split(selected, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" || !facadeChallengeIDIsValid(id) {
			return nil, false
		}
		result = append(result, id)
	}
	return result, true
}

func facadeTitleItemID(summary map[string]any) (int64, bool) {
	title := anyMap(summary["title"])
	id := facadeChallengeID(title["itemId"])
	if id == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(id, 10, 64)
	return value, err == nil && value > 0
}

func facadeSummaryHasTitle(summary map[string]any) bool {
	_, hasTitle := facadeTitleRestorePlan(summary, nil)
	return hasTitle
}

// facadeTitleRestorePlan is intentionally broader than projectFacadeTitle:
// the latter requires a displayable name for the UI, while this plan preserves
// every verified identifier and treats a name-only title as real but unsafe to
// rewrite. That name-only shape is handled by the no-candidate guard.
func facadeTitleRestorePlan(summary, chatLOL map[string]any) ([]facadeTitleRestoreCandidate, bool) {
	candidates := make([]facadeTitleRestoreCandidate, 0, 3)
	hasTitle := false
	if itemID, ok := facadeTitleItemID(summary); ok {
		candidates = append(candidates, facadeTitleRestoreCandidate{Value: strconv.FormatInt(itemID, 10), Diagnostic: "item-id-string"})
		hasTitle = true
	}
	title := anyMap(summary["title"])
	if contentID := anyString(title, "contentId"); contentID != "" {
		candidates = append(candidates, facadeTitleRestoreCandidate{Value: contentID, Diagnostic: "content-id"})
		hasTitle = true
	}
	if selected := anyString(chatLOL, "playerTitleSelected"); selected != "" {
		candidates = append(candidates, facadeTitleRestoreCandidate{Value: selected, Diagnostic: "player-title-selected"})
		hasTitle = true
	}
	if anyString(title, "name") != "" {
		hasTitle = true
	}
	if !hasTitle {
		switch typed := summary["title"].(type) {
		case string:
			hasTitle = strings.TrimSpace(typed) != ""
		case json.Number:
			hasTitle = typed.String() != "" && typed.String() != "0"
		case float64:
			hasTitle = typed > 0
		}
	}
	return candidates, hasTitle
}

func appendFacadeTitleAttemptStatus(statuses []int, err error) []int {
	if status := facadeDiagnosticStatus(err); status > 0 {
		return append(statuses, status)
	}
	return statuses
}

func facadeHTTP4xx(err error) bool {
	var httpErr *LCUHTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode >= 400 && httpErr.StatusCode < 500
}

func facadeTitleRestoreDiagnostic(value string) string {
	switch value {
	case "item-id-string", "content-id", "player-title-selected", "refused", "no-candidate", "not-set":
		return value
	default:
		return "not-set"
	}
}

func (a *app) knownFacadeSkin(ctx context.Context, client *LCUClient, skinID int64) bool {
	// Validate against exactly the catalog shown by the career editor, including
	// its client-scoped fallback before Collection has ever been opened.
	skins, _, err := a.loadFacadeSkins(ctx, client)
	if err != nil {
		return false
	}
	for _, skin := range skins {
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

// The career editor must work before the user opens Collection. This cache
// contains public catalog metadata only; ownership is never inferred from it.
func (a *app) loadFacadeSkins(ctx context.Context, client *LCUClient) ([]Skin, string, error) {
	a.mu.RLock()
	skins := append([]Skin(nil), a.allSkinsWithBase...)
	a.mu.RUnlock()
	if len(skins) > 0 {
		return skins, "collection", nil
	}
	a.facadeSkinCatalogMu.Lock()
	defer a.facadeSkinCatalogMu.Unlock()
	if a.facadeSkinCatalogClient != client {
		a.facadeSkinCatalogClient = client
		a.facadeSkinCatalog = nil
		a.facadeSkinCatalogAt = time.Time{}
		a.facadeSkinCatalogErr = nil
	}
	ttl := 15 * time.Minute
	if len(a.facadeSkinCatalog) == 0 {
		ttl = 5 * time.Second
	}
	if time.Since(a.facadeSkinCatalogAt) < ttl {
		return a.facadeSkinCatalog, "lcu-catalog", a.facadeSkinCatalogErr
	}
	catalogCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	skins, err := loadSkinCatalogContext(catalogCtx, client)
	// A superseded UI request must not poison the cache for the next request.
	if ctx.Err() != nil {
		return a.facadeSkinCatalog, "lcu-catalog", ctx.Err()
	}
	a.facadeSkinCatalogAt = time.Now()
	a.facadeSkinCatalogErr = err
	if err == nil {
		a.facadeSkinCatalog = skins
	}
	a.recordDiagnostic(map[string]any{"event": "facade_skin_catalog", "count": len(skins), "success": err == nil, "cancelled": errors.Is(err, context.Canceled), "timeout": errors.Is(err, context.DeadlineExceeded)})
	return a.facadeSkinCatalog, "lcu-catalog", err
}
