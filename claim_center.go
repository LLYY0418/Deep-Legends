package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type claimSourceState struct {
	Count  int    `json:"count"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type claimEntry struct {
	Key            string       `json:"key"`
	Source         string       `json:"source"`
	ID             string       `json:"id"`
	RewardGroupID  string       `json:"rewardGroupId,omitempty"`
	RewardGroupIDs []string     `json:"rewardGroupIds,omitempty"`
	Title          string       `json:"title"`
	Description    string       `json:"description,omitempty"`
	DateCreated    string       `json:"dateCreated,omitempty"`
	Items          []RewardItem `json:"items"`
	MinSelections  int          `json:"minSelections,omitempty"`
	MaxSelections  int          `json:"maxSelections,omitempty"`
	Historical     bool         `json:"historical"`
	NeedsChoice    bool         `json:"needsChoice"`
	ChainID        string       `json:"chainId,omitempty"`
	ChainIndex     int          `json:"chainIndex,omitempty"`
	ChainCount     int          `json:"chainCount,omitempty"`
	OverlapWith    string       `json:"overlapWith,omitempty"`
}

type claimScanResponse struct {
	Connected          bool                        `json:"connected"`
	ScannedAt          time.Time                   `json:"scannedAt,omitempty"`
	Items              []claimEntry                `json:"items"`
	Sources            map[string]claimSourceState `json:"sources"`
	HistoricalEvidence bool                        `json:"historicalEvidence"`
	Reason             string                      `json:"reason,omitempty"`
}

type claimExecuteSelection struct {
	Key        string   `json:"key"`
	Selections []string `json:"selections,omitempty"`
}

type claimExecuteResult struct {
	Key         string `json:"key"`
	OK          bool   `json:"ok"`
	StatusCode  int    `json:"statusCode,omitempty"`
	ErrorCode   string `json:"errorCode,omitempty"`
	Message     string `json:"message,omitempty"`
	Consequence string `json:"consequence,omitempty"`
}

type claimExecuteResponse struct {
	Results   []claimExecuteResult `json:"results"`
	Succeeded int                  `json:"succeeded"`
	Failed    int                  `json:"failed"`
	Scan      claimScanResponse    `json:"scan"`
}

func (a *app) handleClaimScan(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		respondJSON(w, claimScanResponse{Items: []claimEntry{}, Sources: emptyClaimSources(), Reason: "未连接英雄联盟客户端"})
		return
	}
	respondJSON(w, scanClaims(r.Context(), client))
}

func scanClaims(ctx context.Context, client *LCUClient) claimScanResponse {
	response := claimScanResponse{Connected: true, ScannedAt: time.Now().UTC(), Items: []claimEntry{}, Sources: emptyClaimSources()}
	grants, grantCapability := NewRewardsAPI(client).PendingGrantsContext(ctx)
	grantState := claimSourceState{Count: len(grants), State: grantCapability.State, Detail: grantCapability.Detail}
	response.Sources["grant"] = grantState
	for _, grant := range grants {
		entry := claimEntry{
			Key: "grant:" + grant.ID, Source: "grant", ID: grant.ID, RewardGroupID: grant.RewardGroupID,
			Title: grant.Title, Description: grant.Description, DateCreated: grant.DateCreated, Items: grant.Items,
			MinSelections: grant.MinSelections, MaxSelections: grant.MaxSelections,
		}
		entry.NeedsChoice = entry.MaxSelections > 0 && len(entry.Items) > 1
		entry.Historical = historicalDate(entry.DateCreated)
		response.Items = append(response.Items, entry)
	}

	missionEntries, missionState := scanMissionClaims(ctx, client)
	response.Sources["mission"] = missionState
	response.Items = append(response.Items, missionEntries...)
	eventEntries, eventState := scanEventClaims(ctx, client)
	response.Sources["event"] = eventState
	response.Items = append(response.Items, eventEntries...)
	markClaimOverlaps(response.Items)
	for _, entry := range response.Items {
		if entry.Historical {
			response.HistoricalEvidence = true
			break
		}
	}
	sort.SliceStable(response.Items, func(i, j int) bool {
		if response.Items[i].Historical != response.Items[j].Historical {
			return response.Items[i].Historical
		}
		if response.Items[i].DateCreated != response.Items[j].DateCreated {
			return response.Items[i].DateCreated < response.Items[j].DateCreated
		}
		return response.Items[i].Key < response.Items[j].Key
	})
	return response
}

func emptyClaimSources() map[string]claimSourceState {
	return map[string]claimSourceState{
		"grant": {State: "unavailable"}, "mission": {State: "unavailable"}, "event": {State: "unavailable"},
	}
}

func scanMissionClaims(ctx context.Context, client *LCUClient) ([]claimEntry, claimSourceState) {
	var payload any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-missions/v1/missions", nil, &payload); err != nil {
		return []claimEntry{}, claimSourceError(err)
	}
	missions := mapSliceFromPayload(payload, "missions")
	entries := make([]claimEntry, 0, len(missions))
	for _, mission := range missions {
		if strings.ToUpper(anyString(mission, "status")) != "SELECT_REWARDS" {
			continue
		}
		id := anyString(mission, "id", "missionId")
		if !safeLCUIdentifier(id) {
			continue
		}
		groups := firstMapSlice(mission, "rewardGroups", "rewards")
		groupIDs := make([]string, 0, len(groups))
		items := []RewardItem{}
		for _, group := range groups {
			if groupID := anyString(group, "id", "groupId", "rewardGroupId"); safeLCUIdentifier(groupID) {
				groupIDs = append(groupIDs, groupID)
			}
			items = append(items, collectClaimRewardItems(group)...)
		}
		metadata, _ := mission["metadata"].(map[string]any)
		chainID, chainIndex, chainCount := missionChain(metadata)
		title := claimDisplayTitle(mission, "待领取任务奖励")
		entry := claimEntry{
			Key: "mission:" + id, Source: "mission", ID: id, RewardGroupIDs: groupIDs, Title: title,
			Description: anyString(mission, "description", "shortDescription"), DateCreated: anyString(mission, "dateCreated", "completedDate"),
			Items: items, ChainID: chainID, ChainIndex: chainIndex, ChainCount: chainCount,
		}
		entry.Historical = historicalDate(entry.DateCreated)
		entries = append(entries, entry)
	}
	return entries, claimSourceState{Count: len(entries), State: "available"}
}

func scanEventClaims(ctx context.Context, client *LCUClient) ([]claimEntry, claimSourceState) {
	var payload any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-event-hub/v1/events", nil, &payload); err != nil {
		return []claimEntry{}, claimSourceError(err)
	}
	events := mapSliceFromPayload(payload, "events")
	filtered := make([]map[string]any, 0, len(events))
	for _, event := range events {
		info, _ := event["eventInfo"].(map[string]any)
		if count := anyInt(info, "unclaimedRewardCount"); count <= 0 {
			if count = anyInt(event, "unclaimedRewardCount"); count <= 0 {
				continue
			}
		}
		id := anyString(event, "id", "eventId")
		if safeLCUIdentifier(id) {
			filtered = append(filtered, event)
		}
	}
	entries := make([]claimEntry, len(filtered))
	sem := make(chan struct{}, 4)
	var wait sync.WaitGroup
	var resultMu sync.Mutex
	detailFailures := 0
	var firstDetailError error
	for index, event := range filtered {
		index, event := index, event
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			id := anyString(event, "id", "eventId")
			items := []RewardItem{}
			detailAvailable := false
			var detailError error
			for _, suffix := range []string{"items", "bonus-items"} {
				var detail any
				path := "/lol-event-hub/v1/events/" + id + "/reward-track/" + suffix
				if err := client.RequestJSON(ctx, http.MethodGet, path, nil, &detail); err == nil {
					detailAvailable = true
					items = append(items, collectUnselectedEventItems(detail)...)
				} else if detailError == nil {
					detailError = err
				}
			}
			if !detailAvailable {
				resultMu.Lock()
				detailFailures++
				if firstDetailError == nil {
					firstDetailError = detailError
				}
				resultMu.Unlock()
				return
			}
			info, _ := event["eventInfo"].(map[string]any)
			title := claimDisplayTitle(event, claimDisplayTitle(info, "未领取活动奖励"))
			endDate := anyString(info, "endDate")
			entries[index] = claimEntry{
				Key: "event:" + id, Source: "event", ID: id, Title: title,
				Description: anyString(info, "description"), DateCreated: anyString(info, "startDate"), Items: items,
				Historical: historicalDate(endDate),
			}
		}()
	}
	wait.Wait()
	if err := ctx.Err(); err != nil {
		return []claimEntry{}, claimSourceError(err)
	}
	compact := entries[:0]
	for _, entry := range entries {
		if entry.ID != "" {
			compact = append(compact, entry)
		}
	}
	if len(filtered) > 0 && len(compact) == 0 && firstDetailError != nil {
		return []claimEntry{}, claimSourceError(firstDetailError)
	}
	state := claimSourceState{Count: len(compact), State: "available"}
	if detailFailures > 0 {
		state.State = "failed"
		state.Detail = "部分活动奖励明细读取失败"
	}
	return compact, state
}

func claimSourceError(err error) claimSourceState {
	state := claimSourceState{State: "failed", Detail: "读取失败"}
	var httpErr *LCUHTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		state.State = "unsupported"
		state.Detail = "当前客户端版本未提供此接口"
	}
	return state
}

func mapSliceFromPayload(payload any, keys ...string) []map[string]any {
	if direct, ok := payload.([]any); ok {
		return mapsFromAnySlice(direct)
	}
	object, _ := payload.(map[string]any)
	for _, key := range keys {
		if list, ok := object[key].([]any); ok {
			return mapsFromAnySlice(list)
		}
	}
	return []map[string]any{}
}

func mapsFromAnySlice(values []any) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

func firstMapSlice(object map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		if values, ok := object[key].([]any); ok {
			return mapsFromAnySlice(values)
		}
	}
	return []map[string]any{}
}

func collectClaimRewardItems(value any) []RewardItem {
	result := []RewardItem{}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			id := anyString(typed, "id", "rewardId", "itemId")
			itemID := anyString(typed, "itemId")
			title := claimDisplayTitle(typed, "")
			if (id != "" || itemID != "") && (title != "" || anyInt(typed, "quantity", "count") > 0) {
				key := id + ":" + itemID
				if !seen[key] {
					seen[key] = true
					icon := sanitizeClientImagePath(anyString(typed, "iconUrl", "thumbIconPath", "iconPath"))
					result = append(result, RewardItem{ID: id, ItemID: itemID, ItemType: anyString(typed, "itemType", "type"), Title: title, Details: anyString(typed, "details", "description"), Quantity: int(anyInt(typed, "quantity", "count")), IconURL: icon})
				}
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func collectUnselectedEventItems(value any) []RewardItem {
	result := []RewardItem{}
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			state := strings.ToLower(anyString(typed, "state"))
			if state == "unselected" {
				result = append(result, collectClaimRewardItems(typed)...)
				return
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func claimDisplayTitle(object map[string]any, fallback string) string {
	for _, key := range []string{"title", "name", "displayName", "localizedName", "rewardName"} {
		if value := strings.TrimSpace(anyString(object, key)); value != "" && !rewardLocalizationPlaceholder(value) {
			return value
		}
	}
	if localizations, ok := object["localizations"].(map[string]any); ok {
		if value := strings.TrimSpace(anyString(localizations, "title", "name")); value != "" && !rewardLocalizationPlaceholder(value) {
			return value
		}
	}
	return fallback
}

func missionChain(metadata map[string]any) (string, int, int) {
	if metadata == nil {
		return "", 0, 0
	}
	chainID := anyString(metadata, "chain", "chainId")
	chain, _ := metadata["chain"].(map[string]any)
	if chainID == "" {
		chainID = anyString(chain, "id", "chainId")
	}
	index := int(anyInt(metadata, "chainIndex", "index"))
	count := int(anyInt(metadata, "chainCount", "count"))
	if index == 0 {
		index = int(anyInt(chain, "index", "current"))
	}
	if count == 0 {
		count = int(anyInt(chain, "count", "total"))
	}
	return chainID, index, count
}

func historicalDate(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		parsed, err = time.Parse("2006-01-02", value[:minInt(len(value), 10)])
	}
	return err == nil && parsed.Before(time.Date(time.Now().Year(), 1, 1, 0, 0, 0, 0, time.Local))
}

func markClaimOverlaps(entries []claimEntry) {
	grantSignatures := map[string]bool{}
	for _, entry := range entries {
		if entry.Source == "grant" {
			grantSignatures[claimSignature(entry)] = true
		}
	}
	for index := range entries {
		if entries[index].Source == "event" && grantSignatures[claimSignature(entries[index])] {
			entries[index].OverlapWith = "grant"
			signature := claimSignature(entries[index])
			for grantIndex := range entries {
				if entries[grantIndex].Source == "grant" && claimSignature(entries[grantIndex]) == signature {
					entries[grantIndex].OverlapWith = "event"
				}
			}
		}
	}
}

func claimSignature(entry claimEntry) string {
	title := strings.ToLower(strings.Join(strings.Fields(entry.Title), ""))
	itemIDs := make([]string, 0, len(entry.Items))
	for _, item := range entry.Items {
		if item.ItemID != "" {
			itemIDs = append(itemIDs, item.ItemID)
		} else if item.ID != "" {
			itemIDs = append(itemIDs, item.ID)
		}
	}
	sort.Strings(itemIDs)
	if len(itemIDs) > 0 {
		return strings.Join(itemIDs, ",")
	}
	return title
}

func (a *app) handleClaimExecute(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Items []claimExecuteSelection `json:"items"`
	}
	if err := decodeJSONRequest(r, &request, 64<<10); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(request.Items) == 0 || len(request.Items) > 100 {
		http.Error(w, "请选择 1 到 100 项奖励", http.StatusBadRequest)
		return
	}
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, "未连接英雄联盟客户端", http.StatusConflict)
		return
	}
	canonical := scanClaims(r.Context(), client)
	byKey := make(map[string]claimEntry, len(canonical.Items))
	for _, entry := range canonical.Items {
		byKey[entry.Key] = entry
	}
	response := claimExecuteResponse{Results: make([]claimExecuteResult, 0, len(request.Items))}
	for _, selected := range request.Items {
		entry, ok := byKey[selected.Key]
		result := claimExecuteResult{Key: selected.Key}
		if !ok {
			result.Message = "该条目已不存在，请重新扫描"
			result.Consequence = "它可能已被客户端处理，不影响本批次其它条目。"
			response.Failed++
			response.Results = append(response.Results, result)
			continue
		}
		err := executeClaimEntry(r.Context(), client, entry, selected.Selections)
		if err == nil {
			result.OK = true
			response.Succeeded++
		} else {
			populateClaimError(&result, err)
			response.Failed++
		}
		response.Results = append(response.Results, result)
	}
	// A fresh scan exposes the next SELECT_REWARDS node in a mission chain, but
	// never claims it automatically.
	response.Scan = scanClaims(r.Context(), client)
	respondJSON(w, response)
}

func executeClaimEntry(ctx context.Context, client *LCUClient, entry claimEntry, selections []string) error {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	switch entry.Source {
	case "grant":
		if !safeLCUIdentifier(entry.ID) || !safeLCUIdentifier(entry.RewardGroupID) {
			return errors.New("客户端返回的奖励标识无法安全使用")
		}
		allowed := map[string]bool{}
		for _, item := range entry.Items {
			if item.ID != "" {
				allowed[item.ID] = true
			}
		}
		unique := []string{}
		seen := map[string]bool{}
		for _, selection := range selections {
			if !allowed[selection] || seen[selection] {
				return errors.New("选择项不属于当前奖励组")
			}
			seen[selection] = true
			unique = append(unique, selection)
		}
		minimum, maximum := entry.MinSelections, entry.MaxSelections
		if maximum > len(entry.Items) {
			maximum = len(entry.Items)
		}
		if maximum > 0 && (len(unique) < minimum || len(unique) > maximum) {
			return fmt.Errorf("需要选择 %d 到 %d 项奖励", minimum, maximum)
		}
		path := "/lol-rewards/v1/grants/" + entry.ID + "/select"
		body := map[string]any{"grantId": entry.ID, "rewardGroupId": entry.RewardGroupID, "selections": unique}
		return client.RequestJSON(requestCtx, http.MethodPost, path, body, nil)
	case "mission":
		if !safeLCUIdentifier(entry.ID) || len(entry.RewardGroupIDs) == 0 {
			return errors.New("客户端返回的任务奖励标识无法安全使用")
		}
		path := "/lol-missions/v1/player/" + entry.ID
		return client.RequestJSON(requestCtx, http.MethodPut, path, map[string]any{"rewardGroups": entry.RewardGroupIDs}, nil)
	case "event":
		if !safeLCUIdentifier(entry.ID) {
			return errors.New("客户端返回的活动标识无法安全使用")
		}
		path := "/lol-event-hub/v1/events/" + entry.ID + "/reward-track/claim-all"
		return client.RequestJSON(requestCtx, http.MethodPost, path, nil, nil)
	default:
		return errors.New("未知奖励来源")
	}
}

func populateClaimError(result *claimExecuteResult, err error) {
	result.Message = "领取失败，请重新扫描后再试"
	result.Consequence = "本项失败不会中断其它条目。"
	var httpErr *LCUHTTPError
	if !errors.As(err, &httpErr) {
		if strings.TrimSpace(err.Error()) != "" {
			result.Message = err.Error()
		}
		return
	}
	result.StatusCode = httpErr.StatusCode
	result.ErrorCode = strings.TrimSpace(httpErr.ErrorCode)
	if strings.TrimSpace(httpErr.Message) != "" {
		result.Message = strings.TrimSpace(httpErr.Message)
	} else if result.ErrorCode != "" {
		result.Message = result.ErrorCode
	}
	if strings.Contains(strings.ToLower(result.ErrorCode+" "+result.Message), "fulfilled") || strings.Contains(strings.ToLower(result.ErrorCode+" "+result.Message), "already") {
		result.Consequence = "该条目可能已被处理，重新扫描后会消失；本批次其它条目未受影响。"
	}
}
