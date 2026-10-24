package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// All callers share the same read/modify/write gate, including end-of-game cleanup.
var dlRunePagesMu sync.Mutex

func editableDLPage(p lcuRunePage) bool {
	return p.ID > 0 && strings.HasPrefix(p.Name, runePagePrefix) && (p.IsDeletable == nil || *p.IsDeletable) && (p.IsEditable == nil || *p.IsEditable)
}

func applyRunePage(ctx context.Context, client *LCUClient, request gameplayRuneApplyRequest) (int64, error) {
	dlRunePagesMu.Lock()
	defer dlRunePagesMu.Unlock()
	trace := runeApplyTraceFromContext(ctx)
	var pages []lcuRunePage
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-perks/v1/pages", nil, &pages); err != nil {
		return 0, err
	}
	var owned []lcuRunePage
	protected := 0
	for _, page := range pages {
		if strings.HasPrefix(page.Name, runePagePrefix) && !editableDLPage(page) {
			protected++
		}
		if editableDLPage(page) {
			owned = append(owned, page)
		}
	}
	if trace != nil {
		trace.DLPagesBefore = len(owned) + protected
		trace.DLPagesAfter = len(owned) + protected
	}
	if protected > 0 {
		return 0, errors.New("现有 [DL] 符文页不可编辑，请在客户端处理后重试")
	}
	pageID := int64(0)
	action := "updated"
	if len(owned) > 0 {
		pageID = owned[0].ID
		for _, page := range owned[1:] {
			if err := checkAutoConfigWrite(ctx); err != nil {
				return 0, err
			}
			if err := client.RequestJSON(ctx, http.MethodDelete, fmt.Sprintf("/lol-perks/v1/pages/%d", page.ID), nil, nil); err != nil {
				return 0, err
			}
			if trace != nil {
				trace.DLPagesAfter--
				trace.Action = "deleted_extra"
			}
		}
		if len(owned) > 1 {
			action = "deleted_extra"
		}
	} else {
		canAdd, err := canAddRunePage(ctx, client)
		if err != nil {
			return 0, err
		}
		if !canAdd {
			return 0, errRunePageLimit
		}
		if err := checkAutoConfigWrite(ctx); err != nil {
			return 0, err
		}
		var raw json.RawMessage
		if err := client.RequestJSON(ctx, http.MethodPost, "/lol-perks/v1/pages/", map[string]any{"name": runePageName(request.ChampionName, request.Source), "isEditable": true, "primaryStyleId": strconv.FormatInt(request.PrimaryStyleID, 10)}, &raw); err != nil {
			return 0, err
		}
		recordRuneApplyResponse(trace, "create-page", raw)
		var page struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return 0, err
		}
		pageID = page.ID
		if pageID <= 0 {
			return 0, errors.New("客户端没有返回符文页编号")
		}
		action = "created"
		if trace != nil {
			trace.DLPagesAfter = 1
			trace.Action = action
		}
	}
	body := map[string]any{"id": pageID, "isRecommendationOverride": false, "isTemporary": false, "name": runePageName(request.ChampionName, request.Source), "primaryStyleId": request.PrimaryStyleID, "subStyleId": request.SubStyleID, "selectedPerkIds": request.SelectedPerkIDs, "recommendationChampionId": request.ChampionID}
	if err := checkAutoConfigWrite(ctx); err != nil {
		return 0, err
	}
	var raw json.RawMessage
	if err := client.RequestJSON(ctx, http.MethodPut, fmt.Sprintf("/lol-perks/v1/pages/%d", pageID), body, &raw); err != nil {
		return 0, err
	}
	recordRuneApplyResponse(trace, "update-page", raw)
	if err := checkAutoConfigWrite(ctx); err != nil {
		return 0, err
	}
	if err := client.RequestJSON(ctx, http.MethodPut, "/lol-perks/v1/currentpage", pageID, &raw); err != nil {
		return 0, err
	}
	recordRuneApplyResponse(trace, "set-current-page", raw)
	if trace != nil {
		trace.DLPagesAfter = 1
		trace.Action = action
	}
	return pageID, nil
}

func canceledChampSelectPhase(phase string) bool {
	return phase == "Lobby" || phase == "Matchmaking" || phase == "None"
}

func (a *app) cleanupDLRunePages(ctx context.Context, client *LCUClient, canceled ...bool) {
	dlRunePagesMu.Lock()
	defer dlRunePagesMu.Unlock()
	result := "success"
	deleted := 0
	defer func() {
		a.recordDiagnostic(map[string]any{"event": "perk_page_cleanup", "result": result, "deleted": deleted})
	}()
	var pages []lcuRunePage
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-perks/v1/pages", nil, &pages); err != nil {
		result = "read-failed"
		return
	}
	for _, page := range pages {
		if !editableDLPage(page) {
			continue
		}
		var phase string
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase); err != nil || (phase != "EndOfGame" && !(len(canceled) > 0 && canceled[0] && canceledChampSelectPhase(phase))) {
			result = "phase-changed"
			return
		}
		if err := client.RequestJSON(ctx, http.MethodDelete, fmt.Sprintf("/lol-perks/v1/pages/%d", page.ID), nil, nil); err != nil {
			result = "delete-failed"
			continue
		}
		deleted++
	}
}
