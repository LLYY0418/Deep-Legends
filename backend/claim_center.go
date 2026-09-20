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
	DisplayGroup   string       `json:"displayGroup,omitempty"`
	EventID        string       `json:"eventId,omitempty"`
	EventName      string       `json:"eventName,omitempty"`
	Title          string       `json:"title"`
	Description    string       `json:"description,omitempty"`
	Detail         string       `json:"detail,omitempty"`
	DateCreated    string       `json:"dateCreated,omitempty"`
	Items          []RewardItem `json:"items"`
	MinSelections  int          `json:"minSelections,omitempty"`
	MaxSelections  int          `json:"maxSelections,omitempty"`
	Historical     bool         `json:"historical"`
	NeedsChoice    bool         `json:"needsChoice"`
	Actionable     bool         `json:"actionable"`
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
	respondJSON(w, scanClaimsObserved(r.Context(), client, a.recordDiagnostic))
}

func scanClaims(ctx context.Context, client *LCUClient) claimScanResponse {
	return scanClaimsObserved(ctx, client, nil)
}

func scanClaimsObserved(ctx context.Context, client *LCUClient, observe func(map[string]any)) claimScanResponse {
	response := claimScanResponse{Connected: true, ScannedAt: time.Now().UTC(), Items: []claimEntry{}, Sources: emptyClaimSources()}
	grants, grantCapability := NewRewardsAPI(client).PendingGrantsContext(ctx)
	grantState := claimSourceState{Count: len(grants), State: grantCapability.State, Detail: grantCapability.Detail}
	response.Sources["grant"] = grantState
	for _, grant := range grants {
		entry := claimEntry{
			Key: "grant:" + grant.ID, Source: "grant", ID: grant.ID, RewardGroupID: grant.RewardGroupID,
			DisplayGroup: grant.DisplayGroup, Title: grant.Title, Description: grant.Description, DateCreated: grant.DateCreated, Items: grant.Items,
			MinSelections: grant.MinSelections, MaxSelections: grant.MaxSelections, Actionable: true,
		}
		reason := grant.ValidationError
		if reason == "" {
			reason = grantValidationReason(entry)
		}
		if reason != "" {
			entry.Actionable = false
			entry.Detail = reason
		}
		entry.NeedsChoice = entry.MaxSelections > 0 && len(entry.Items) > entry.MaxSelections
		entry.Historical = historicalDate(entry.DateCreated)
		response.Items = append(response.Items, entry)
	}

	missionEntries, missionState := scanMissionClaims(ctx, client)
	response.Sources["mission"] = missionState
	response.Items = append(response.Items, missionEntries...)
	eventEntries, eventState := scanEventClaims(ctx, client, observe)
	response.Sources["event"] = eventState
	response.Items = append(response.Items, eventEntries...)
	markClaimOverlaps(response.Items)
	client.claimSettlements.filter(&response, time.Now())
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
	if observe != nil {
		observe(claimScanShape(response))
	}
	return response
}

// Counts only: no grant IDs, localized names, account identifiers or raw payloads.
func claimScanShape(response claimScanResponse) map[string]any {
	sources := map[string]int{}
	rows, positive, unknown, actionable, overlap, eventMapped := 0, 0, 0, 0, 0, 0
	quantities := map[string]int{}
	for _, entry := range response.Items {
		sources[entry.Source]++
		if entry.Source == "grant" && entry.EventID != "" {
			eventMapped++
		}
		if entry.Actionable {
			actionable++
		}
		if len(entry.OverlapWith) > 0 {
			overlap++
		}
		for _, item := range entry.Items {
			rows++
			if item.Quantity > 0 {
				positive++
			} else {
				unknown++
			}
			kind := strings.ToUpper(item.ItemType)
			switch kind {
			case "CURRENCY", "MATERIAL", "CHEST", "CHAMPION", "CHAMPION_SKIN", "EMOTE", "SUMMONER_ICON", "STATSTONE", "STATSTONE_SHARD":
			default:
				kind = "OTHER"
			}
			if item.Quantity > 0 {
				quantities[kind] += item.Quantity
			}
		}
	}
	return map[string]any{"event": "claim_scan_shape", "source_entries": sources, "entry_count": len(response.Items), "actionable_entries": actionable, "reward_item_rows": rows, "quantity_positive_rows": positive, "quantity_unknown_rows": unknown, "quantity_by_type": quantities, "overlap_entries": overlap, "event_mapped_grants": eventMapped}
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
			Items: items, ChainID: chainID, ChainIndex: chainIndex, ChainCount: chainCount, Actionable: true,
		}
		if len(groupIDs) == 0 || len(groupIDs) != len(groups) {
			entry.Actionable = false
			entry.Detail = "客户端未提供完整、安全的任务奖励组标识，请在客户端领取"
		}
		entry.Historical = historicalDate(entry.DateCreated)
		entries = append(entries, entry)
	}
	return entries, claimSourceState{Count: len(entries), State: "available"}
}

func scanEventClaims(ctx context.Context, client *LCUClient, observers ...func(map[string]any)) ([]claimEntry, claimSourceState) {
	var observe func(map[string]any)
	if len(observers) > 0 {
		observe = observers[0]
	}
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
		id := anyString(event, "eventId", "id")
		if safeLCUIdentifier(id) {
			filtered = append(filtered, event)
		}
	}
	entries := make([]claimEntry, len(filtered))
	sem := make(chan struct{}, 4)
	var wait sync.WaitGroup
	var resultMu sync.Mutex
	stateCounts := map[string]int{}
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
			id := anyString(event, "eventId", "id")
			items := []RewardItem{}
			groups := []string{}
			detailAvailable := false
			var detailError error
			for _, suffix := range []string{"items", "bonus-items"} {
				var detail any
				path := "/lol-event-hub/v1/events/" + id + "/reward-track/" + suffix
				if err := client.RequestJSON(ctx, http.MethodGet, path, nil, &detail); err == nil {
					detailAvailable = true
					counts := eventRewardStateCounts(detail)
					resultMu.Lock()
					for state, count := range counts {
						stateCounts[state] += count
					}
					resultMu.Unlock()
					items = append(items, collectUnselectedEventItems(detail)...)
					groups = append(groups, eventRewardGroupIDs(detail)...)
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
			seenItems := map[string]bool{}
			uniqueItems := items[:0]
			for _, item := range items {
				raw, _ := json.Marshal(item)
				key := string(raw)
				if !seenItems[key] {
					seenItems[key] = true
					uniqueItems = append(uniqueItems, item)
				}
			}
			items = uniqueItems
			info, _ := event["eventInfo"].(map[string]any)
			title := claimDisplayTitle(event, claimDisplayTitle(info, "未领取活动奖励"))
			endDate := anyString(info, "endDate")
			if len(items) == 0 && len(groups) == 0 {
				return
			}
			entry := claimEntry{
				Key: "event:" + id, Source: "event", ID: id, Title: title, EventID: id, EventName: title, RewardGroupIDs: groups,
				Description: anyString(info, "description"), DateCreated: anyString(info, "startDate"), Items: items,
				Historical: historicalDate(endDate), Actionable: len(items) > 0,
			}
			entries[index] = entry
		}()
	}
	wait.Wait()
	if observe != nil && len(stateCounts) > 0 {
		observe(map[string]any{"event": "event_reward_state_shape", "state_counts": stateCounts})
	}
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
	actionableCount := 0
	for _, entry := range compact {
		if entry.Actionable {
			actionableCount++
		}
	}
	state := claimSourceState{Count: actionableCount, State: "available"}
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
			id := anyString(typed, "id", "rewardId", "itemId", "rewardGroupId")
			itemID := anyString(typed, "itemId")
			title := claimDisplayTitle(typed, "")
			if (id != "" || itemID != "") && (title != "" || anyInt(typed, "quantity", "count") > 0) {
				icon := sanitizeClientImagePath(anyString(typed, "iconUrl", "thumbIconPath", "iconPath"))
				item := RewardItem{ID: id, ItemID: itemID, ItemType: anyString(typed, "itemType", "type"), Title: title, Details: anyString(typed, "details", "description"), Quantity: int(anyInt(typed, "quantity", "count")), IconURL: icon}
				encoded, _ := json.Marshal(item)
				key := string(encoded)
				if !seen[key] {
					seen[key] = true
					result = append(result, item)
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

func eventRewardStateCounts(value any) map[string]int {
	result := map[string]int{}
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			if raw, ok := typed["state"].(string); ok {
				state := strings.TrimSpace(raw)
				if state != "" {
					result[state]++
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

func claimDisplayTitle(object map[string]any, fallback string) string {
	for _, key := range []string{"eventName", "title", "name", "displayName", "localizedName", "rewardName"} {
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

// Reward amounts are not identities: two unrelated events can award 750 BE.
func markClaimOverlaps(entries []claimEntry) {
	owners := map[string][]int{}
	for i, entry := range entries {
		if entry.Source != "event" {
			continue
		}
		seen := map[string]bool{}
		for _, group := range entry.RewardGroupIDs {
			if group != "" && !seen[group] {
				owners[group] = append(owners[group], i)
				seen[group] = true
			}
		}
	}
	for i := range entries {
		if entries[i].Source != "grant" {
			continue
		}
		matches := owners[entries[i].RewardGroupID]
		for _, index := range matches {
			entries[index].OverlapWith = "grant"
			entries[index].Actionable = false
			entries[index].Detail = "奖励组已在奖励账本中逐项展示，避免重复领取"
		}
		if len(matches) != 1 {
			continue
		}
		event := &entries[matches[0]]
		entries[i].EventID, entries[i].EventName = event.EventID, event.EventName
		entries[i].Title = event.EventName
		// The event claim-all and the grant select act on the same entitlement.
		// Prefer the existing per-grant selection path; never submit both.
		event.OverlapWith = "grant"
		event.Actionable = false
		event.Detail = "同一活动奖励已在奖励账本中逐项展示"
	}
}

func eventRewardGroupIDs(value any) []string {
	result := []string{}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		switch row := value.(type) {
		case []any:
			for _, child := range row {
				walk(child)
			}
		case map[string]any:
			if id := anyString(row, "rewardGroupId"); id != "" && !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
			for _, child := range row {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func claimSignature(entry claimEntry) string {
	items := make([]string, 0, len(entry.Items))
	for _, item := range entry.Items {
		id := item.ItemID
		if id == "" {
			id = item.ID
		}
		if id == "" {
			return ""
		}
		encoded, _ := json.Marshal([]any{id, strings.ToUpper(item.ItemType), item.Quantity})
		items = append(items, string(encoded))
	}
	sort.Strings(items)
	return strings.Join(items, ",")
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
	// Include waiting for another batch in the handler's total budget.
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	traceID := newDiagnosticTrace("claim")
	itemStarted := make([]time.Time, len(request.Items))
	itemEnded := make([]bool, len(request.Items))
	for index := range request.Items {
		itemStarted[index] = time.Now()
		a.recordDiagnostic(map[string]any{"event": "claim_item", "trace_id": traceID, "stage": "begin", "index": index})
	}
	finishItem := func(index int, timedOut bool) {
		if itemEnded[index] {
			return
		}
		itemEnded[index] = true
		a.recordDiagnostic(map[string]any{"event": "claim_item", "trace_id": traceID, "stage": "end", "index": index, "duration_ms": time.Since(itemStarted[index]).Milliseconds(), "timed_out": timedOut || errors.Is(ctx.Err(), context.DeadlineExceeded)})
	}
	// Include mutex wait and the canonical scan, where the observed stalls occur.
	defer func() {
		for index := range request.Items {
			finishItem(index, false)
		}
	}()
	if err := lockClaimsContext(ctx, &client.claimExecutionMu); err != nil {
		http.Error(w, "领取等待超时，请重新扫描后重试", http.StatusGatewayTimeout)
		return
	}
	defer client.claimExecutionMu.Unlock()
	canonical := scanClaimsObserved(ctx, client, a.recordDiagnostic)
	if ctx.Err() != nil {
		http.Error(w, "领取扫描超时，请重新扫描后重试", http.StatusGatewayTimeout)
		return
	}
	byKey := make(map[string]claimEntry, len(canonical.Items))
	for _, entry := range canonical.Items {
		byKey[entry.Key] = entry
	}
	seen := make(map[string]bool, len(request.Items))
	response := claimExecuteResponse{Results: make([]claimExecuteResult, 0, len(request.Items))}
	for index, selected := range request.Items {
		itemTimedOut := false
		func() {
			defer func() {
				finishItem(index, itemTimedOut)
			}()
			entry, ok := byKey[selected.Key]
			result := claimExecuteResult{Key: selected.Key}
			if seen[selected.Key] {
				result.Message = "本批次包含重复条目，已阻止重复领取"
				response.Failed++
				response.Results = append(response.Results, result)
				return
			}
			seen[selected.Key] = true
			if !ok {
				result.Message = "该条目已不存在，请重新扫描"
				result.Consequence = "它可能已被客户端处理，不影响本批次其它条目。"
				response.Failed++
				response.Results = append(response.Results, result)
				return
			}
			var err error
			if ctx.Err() != nil {
				populateClaimError(&result, ctx.Err())
				response.Failed++
				response.Results = append(response.Results, result)
				return
			}
			if !entry.Actionable {
				reason := entry.Detail
				if reason == "" {
					reason = "客户端未提供可安全领取的奖励明细"
				}
				err = errors.New(reason)
			} else {
				err = executeClaimEntry(ctx, client, entry, selected.Selections)
			}
			if err == nil {
				client.claimSettlements.remember(entry, time.Now())
				result.OK = true
				response.Succeeded++
			} else {
				itemTimedOut = diagnosticErrorKind(err) == "timeout"
				populateClaimError(&result, err)
				response.Failed++
			}
			response.Results = append(response.Results, result)
			a.recordDiagnostic(map[string]any{"event": "claim_action", "source": entry.Source, "ok": result.OK, "status_code": result.StatusCode, "reward_count": len(entry.Items)})
		}()
	}
	// Confirm only the affected mission source to expose its next chain node.
	// Settlements suppress successful grant/event writes until the final UI scan.
	response.Scan = canonical
	for _, selected := range request.Items {
		if byKey[selected.Key].Source == "mission" && ctx.Err() == nil {
			missions, source := scanMissionClaims(ctx, client)
			if source.State == "available" {
				kept := []claimEntry{}
				for _, entry := range response.Scan.Items {
					if entry.Source != "mission" {
						kept = append(kept, entry)
					}
				}
				response.Scan.Items = append(kept, missions...)
				response.Scan.Sources["mission"] = source
			}
			break
		}
	}
	client.claimSettlements.filter(&response.Scan, time.Now())
	if ctx.Err() != nil {
		response.Scan.Reason = "领取超时，列表状态待重新扫描确认"
	}
	respondJSON(w, response)
}

// Validate client metadata before advertising an action and again before writing.
// Never repair an inconsistent selection strategy by silently changing its bounds.
func grantValidationReason(entry claimEntry) string {
	if !safeLCUIdentifier(entry.ID) || !safeLCUIdentifier(entry.RewardGroupID) {
		return "客户端返回的奖励标识无法安全使用"
	}
	if len(entry.Items) == 0 {
		return "客户端未提供奖励明细，请在客户端领取"
	}
	if entry.MinSelections < 0 || entry.MaxSelections < 0 || entry.MinSelections > entry.MaxSelections || entry.MaxSelections > len(entry.Items) {
		return "客户端返回的奖励选择范围不完整，请在客户端领取"
	}
	if entry.MaxSelections > 0 {
		seen := make(map[string]bool, len(entry.Items))
		for _, item := range entry.Items {
			if strings.TrimSpace(item.ID) == "" || seen[item.ID] {
				return "客户端奖励选项标识缺失或重复，请在客户端领取"
			}
			seen[item.ID] = true
		}
	}
	return ""
}

func executeClaimEntry(ctx context.Context, client *LCUClient, entry claimEntry, selections []string) error {
	if entry.Source == "event" && !entry.Actionable {
		return errors.New("该活动奖励客户端未提供可领取明细，请在游戏客户端内领取")
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	switch entry.Source {
	case "grant":
		if reason := grantValidationReason(entry); reason != "" {
			return errors.New(reason)
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
		if maximum > 0 && len(entry.Items) == maximum && len(unique) == 0 {
			for _, item := range entry.Items {
				if item.ID == "" {
					unique = nil
					break
				}
				unique = append(unique, item.ID)
			}
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
	if errors.Is(err, context.DeadlineExceeded) {
		result.Message = "领取超时，请重新扫描确认后重试"
		result.ErrorCode = "timeout"
		return
	}
	if errors.Is(err, context.Canceled) {
		result.Message = "领取请求已取消，请重新扫描确认状态"
		result.ErrorCode = "canceled"
		return
	}
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

func lockClaimsContext(ctx context.Context, mu *sync.Mutex) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if mu.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
