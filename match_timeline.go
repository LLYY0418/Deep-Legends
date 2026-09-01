package main

// match_timeline.go 提供单场对局的“装备路线 + 技能加点”数据。
// 客户端结算战绩只包含最终装备，购买顺序与技能升级藏在对局时间线里：
//
//   - 国服：优先读取本机客户端 /lol-match-history/v1/game-timelines/{gameId}
//     （客户端优先原则）；该端点缺失或为空时回退腾讯官方 SGP 网关的
//     DETAILS 接口（帧结构与 Riot Match-V5 timeline 同构）。
//   - 韩服：Riot 官方 /lol/match/v5/matches/KR_{gameId}/timeline。
//
// 任一来源失败都返回 available=false 与中文说明，前端明确降级展示，
// 不用英雄默认加点顺序代替真实对局数据。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

/* ---------- 与 Riot / SGP / LCU 同构的时间线帧 ---------- */

type timelineEvent struct {
	Type          string `json:"type"`
	EventType     string `json:"eventType"`
	Timestamp     int64  `json:"timestamp"`
	ParticipantID int64  `json:"participantId"`
	ItemID        int64  `json:"itemId"`
	BeforeID      int64  `json:"beforeId"`
	AfterID       int64  `json:"afterId"`
	SkillSlot     int    `json:"skillSlot"`
	LevelUpType   string `json:"levelUpType"`
	rawKeys       []string
}

func (event *timelineEvent) UnmarshalJSON(data []byte) error {
	type timelineEventAlias timelineEvent
	var decoded timelineEventAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	*event = timelineEvent(decoded)
	event.rawKeys = keys
	return nil
}

type timelineFrame struct {
	Timestamp int64           `json:"timestamp"`
	Events    []timelineEvent `json:"events"`
}

/* ---------- 对外响应 ---------- */

type timelineItemEvent struct {
	ItemID int64 `json:"itemId"`
	Sold   bool  `json:"sold,omitempty"`
}

type timelineItemGroup struct {
	Minute int                 `json:"minute"`
	Events []timelineItemEvent `json:"events"`
}

type timelineSkillUp struct {
	Level int `json:"level"`
	Slot  int `json:"slot"`
}

type matchTimelineResponse struct {
	Available      bool                `json:"available"`
	Source         string              `json:"source,omitempty"`
	Detail         string              `json:"detail,omitempty"`
	Attempts       []DataSourceAttempt `json:"attempts,omitempty"`
	FallbackReason string              `json:"fallbackReason,omitempty"`
	ItemGroups     []timelineItemGroup `json:"itemGroups,omitempty"`
	SkillOrder     []timelineSkillUp   `json:"skillOrder,omitempty"`
}

/* ---------- 事件提取 ---------- */

const timelineMaxItemGroups = 60

// timelineItemRecord 是提取过程中的中间记录（购买或出售）。
type timelineItemRecord struct {
	at     int64
	itemID int64
	sold   bool
}

// extractParticipantTimeline 从时间线帧里抽出指定参与者的装备购买
// 路线（按分钟分组，撤销的购买/出售会抵消）与技能加点顺序。
func extractParticipantTimeline(frames []timelineFrame, participantID int64) ([]timelineItemGroup, []timelineSkillUp) {
	records := make([]timelineItemRecord, 0, 32)
	skills := make([]timelineSkillUp, 0, 18)
	for _, frame := range frames {
		for _, event := range frame.Events {
			if event.ParticipantID != participantID {
				continue
			}
			eventType := strings.TrimSpace(event.Type)
			if eventType == "" {
				eventType = strings.TrimSpace(event.EventType)
			}
			switch strings.ToUpper(eventType) {
			case "ITEM_PURCHASED":
				if event.ItemID > 0 {
					records = append(records, timelineItemRecord{at: event.Timestamp, itemID: event.ItemID})
				}
			case "ITEM_SOLD":
				if event.ItemID > 0 {
					records = append(records, timelineItemRecord{at: event.Timestamp, itemID: event.ItemID, sold: true})
				}
			case "ITEM_UNDO":
				// 撤销购买时 beforeId 是被退掉的装备；撤销出售时 afterId
				// 是拿回的装备。都从记录里抵消最近一条对应条目。
				if event.BeforeID > 0 {
					records = removeLastItemRecord(records, event.BeforeID, false)
				} else if event.AfterID > 0 {
					records = removeLastItemRecord(records, event.AfterID, true)
				}
			case "SKILL_LEVEL_UP":
				// EVOLVE（如卡兹克进化）不占常规加点位，单独忽略。
				if event.SkillSlot >= 1 && event.SkillSlot <= 4 && !strings.EqualFold(event.LevelUpType, "EVOLVE") {
					skills = append(skills, timelineSkillUp{Level: len(skills) + 1, Slot: event.SkillSlot})
				}
			}
		}
	}
	groups := make([]timelineItemGroup, 0, 16)
	for _, record := range records {
		minute := int(record.at / 60000)
		if length := len(groups); length > 0 && groups[length-1].Minute == minute {
			groups[length-1].Events = append(groups[length-1].Events, timelineItemEvent{ItemID: record.itemID, Sold: record.sold})
			continue
		}
		groups = append(groups, timelineItemGroup{Minute: minute, Events: []timelineItemEvent{{ItemID: record.itemID, Sold: record.sold}}})
	}
	if len(groups) > timelineMaxItemGroups {
		groups = groups[:timelineMaxItemGroups]
	}
	return groups, skills
}

func removeLastItemRecord(records []timelineItemRecord, itemID int64, sold bool) []timelineItemRecord {
	for index := len(records) - 1; index >= 0; index-- {
		if records[index].itemID == itemID && records[index].sold == sold {
			return append(records[:index], records[index+1:]...)
		}
	}
	return records
}

/* ---------- 时间线来源 ---------- */

// lcuGameTimeline 兼容本机客户端可能返回的两种包装形状。
type lcuGameTimeline struct {
	Frames []timelineFrame `json:"frames"`
	Info   struct {
		Frames []timelineFrame `json:"frames"`
	} `json:"info"`
	JSON struct {
		Frames []timelineFrame `json:"frames"`
	} `json:"json"`
}

func (t lcuGameTimeline) frames() []timelineFrame {
	frames, _ := t.framesWithSource()
	return frames
}

func (t lcuGameTimeline) framesWithSource() ([]timelineFrame, string) {
	if len(t.Frames) > 0 {
		return t.Frames, "Frames"
	}
	if len(t.Info.Frames) > 0 {
		return t.Info.Frames, "Info.Frames"
	}
	if len(t.JSON.Frames) > 0 {
		return t.JSON.Frames, "JSON.Frames"
	}
	// Preserve an explicitly present but empty wrapper for diagnostics. Business
	// selection above still prefers the first non-empty frame collection.
	if t.Frames != nil {
		return t.Frames, "Frames"
	}
	if t.Info.Frames != nil {
		return t.Info.Frames, "Info.Frames"
	}
	if t.JSON.Frames != nil {
		return t.JSON.Frames, "JSON.Frames"
	}
	return nil, "none"
}

func summarizeTimelineEventTypes(frames []timelineFrame) (int, []string) {
	types := make(map[string]struct{})
	eventCount := 0
	for _, frame := range frames {
		for _, event := range frame.Events {
			eventCount++
			eventType := strings.TrimSpace(event.Type)
			if eventType == "" {
				eventType = strings.TrimSpace(event.EventType)
			}
			if eventType != "" {
				types[strings.ToUpper(eventType)] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(types))
	for eventType := range types {
		result = append(result, eventType)
	}
	sort.Strings(result)
	if len(result) > 10 {
		result = result[:10]
	}
	return eventCount, result
}

func timelineHasParticipantEvents(frames []timelineFrame) bool {
	for _, frame := range frames {
		for _, event := range frame.Events {
			if event.ParticipantID > 0 {
				return true
			}
		}
	}
	return false
}

func sampleTimelineEventKeys(frames []timelineFrame, limit int) [][]string {
	if limit <= 0 {
		return nil
	}
	type candidate struct {
		keys     []string
		priority bool
	}
	candidates := make([]candidate, 0, limit)
	for _, frame := range frames {
		for _, event := range frame.Events {
			if len(event.rawKeys) == 0 {
				continue
			}
			eventType := strings.TrimSpace(event.Type)
			if eventType == "" {
				eventType = strings.TrimSpace(event.EventType)
			}
			eventType = strings.ToUpper(eventType)
			candidates = append(candidates, candidate{keys: event.rawKeys, priority: strings.Contains(eventType, "ITEM_") || strings.Contains(eventType, "SKILL_")})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority
		}
		return len(candidates[i].keys) > len(candidates[j].keys)
	})
	samples := make([][]string, 0, min(limit, len(candidates)))
	for _, item := range candidates {
		samples = append(samples, append([]string(nil), item.keys...))
		if len(samples) >= limit {
			break
		}
	}
	return samples
}

func parseTimelineFrames(data []byte) ([]timelineFrame, error) {
	var wrapped lcuGameTimeline
	wrappedErr := json.Unmarshal(data, &wrapped)
	if wrappedErr == nil && len(wrapped.frames()) > 0 {
		return wrapped.frames(), nil
	}
	var bare []timelineFrame
	bareErr := json.Unmarshal(data, &bare)
	if bareErr == nil && len(bare) > 0 {
		return bare, nil
	}
	if wrappedErr != nil && bareErr != nil {
		return nil, fmt.Errorf("时间线响应无法解析（包装对象: %v；裸数组: %w）", wrappedErr, bareErr)
	}
	return nil, fmt.Errorf("时间线响应没有 frames")
}

// loadMatchTimelineCNDecision keeps LCU-to-SGP replacement explicit because
// the two endpoints are compatible timeline datasets, not interchangeable HTTP retries.
func (a *app) loadMatchTimelineCNDecision(ctx context.Context, client *LCUClient, serverID string, gameID int64) ([]timelineFrame, string, []DataSourceAttempt, string, error) {
	decision := resolveTimelineDataSources(timelineDataSourceInput{
		LCUConnected: client != nil,
		RemoteServer: isRemoteTencentServer(client, serverID),
		SGPAvailable: a.sgp != nil,
	})
	attempts := make([]DataSourceAttempt, 0, len(decision.Sources))
	fallbackReason := ""
	if len(decision.Sources) > 0 && decision.Sources[0] == dataSourceSGP {
		fallbackReason = decision.Reason
	}
	var lcuErr error
	// 仅目标服务器就是当前客户端服务器时尝试 LCU；跨服对局绝不能用
	// 当前大区相同 gameId 的时间线冒充目标数据。
	if len(decision.Sources) > 0 && decision.Sources[0] == dataSourceLCU {
		var data []byte
		data, lcuErr = client.GetBytesContext(ctx, fmt.Sprintf("/lol-match-history/v1/game-timelines/%d", gameID))
		if isCancellation(lcuErr) {
			return nil, "", attempts, fallbackReason, lcuErr
		}
		if lcuErr == nil {
			var frames []timelineFrame
			frames, lcuErr = parseTimelineFrames(data)
			if lcuErr == nil {
				if timelineHasParticipantEvents(frames) {
					attempts = append(attempts, DataSourceAttempt{Source: dataSourceLCU, Outcome: dataSourceSuccess})
					return frames, dataSourceLCU, attempts, fallbackReason, nil
				}
				eventCount, eventTypes := summarizeTimelineEventTypes(frames)
				a.recordDiagnostic(map[string]any{
					"event": "match_timeline_lcu_incomplete", "source": "lcu",
					"frames": len(frames), "events": eventCount,
					"event_types": eventTypes, "event_key_samples": sampleTimelineEventKeys(frames, 3),
				})
				lcuErr = fmt.Errorf("时间线事件缺少 participantId，改用 SGP 回退")
			}
		}
		outcome := dataSourceFailed
		if strings.Contains(safeDiagnosticReason(lcuErr), "participantId") || strings.Contains(safeDiagnosticReason(lcuErr), "frames") {
			outcome = dataSourceModeUnsupported
		}
		attempts = append(attempts, DataSourceAttempt{Source: dataSourceLCU, Outcome: outcome, Message: safeDiagnosticReason(lcuErr)})
		fallbackReason = map[string]string{dataSourceFailed: "lcu-failed", dataSourceModeUnsupported: "lcu-mode-unsupported"}[outcome]
	}
	var frames []timelineFrame
	var sgpErr error
	if a.sgp == nil {
		sgpErr = fmt.Errorf("SGP 时间线数据源不可用")
		attempts = append(attempts, DataSourceAttempt{Source: dataSourceSGP, Outcome: dataSourceDisabled, Message: sgpErr.Error()})
		if lcuErr != nil {
			return nil, "", attempts, fallbackReason, fmt.Errorf("LCU: %s；SGP: %s", safeDiagnosticReason(lcuErr), safeDiagnosticReason(sgpErr))
		}
		return nil, "", attempts, fallbackReason, sgpErr
	} else if serverID != "" {
		frames, sgpErr = a.sgp.gameDetailsOn(ctx, client, serverID, gameID)
	} else {
		frames, sgpErr = a.sgp.gameDetails(ctx, client, gameID)
	}
	if isCancellation(sgpErr) {
		return nil, "", attempts, fallbackReason, sgpErr
	}
	if sgpErr == nil {
		attempts = append(attempts, DataSourceAttempt{Source: dataSourceSGP, Outcome: dataSourceSuccess})
		return frames, dataSourceSGP, attempts, fallbackReason, nil
	}
	attempts = append(attempts, DataSourceAttempt{Source: dataSourceSGP, Outcome: dataSourceFailed, Message: safeDiagnosticReason(sgpErr)})
	if lcuErr != nil {
		return nil, "", attempts, fallbackReason, fmt.Errorf("LCU: %s；SGP: %s", safeDiagnosticReason(lcuErr), safeDiagnosticReason(sgpErr))
	}
	return nil, "", attempts, fallbackReason, sgpErr
}

func (a *app) loadMatchTimelineCN(ctx context.Context, client *LCUClient, serverID string, gameID int64) ([]timelineFrame, string, error) {
	frames, source, _, _, err := a.loadMatchTimelineCNDecision(ctx, client, serverID, gameID)
	return frames, source, err
}

/* ---------- 缓存与 HTTP 端点 ---------- */

const matchTimelineCacheMax = 120

type matchTimelineCache struct {
	mu      sync.Mutex
	entries map[string]matchTimelineResponse
	order   []string
}

func newMatchTimelineCache() *matchTimelineCache {
	return &matchTimelineCache{entries: make(map[string]matchTimelineResponse)}
}

func (c *matchTimelineCache) get(key string) (matchTimelineResponse, bool) {
	if c == nil {
		return matchTimelineResponse{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	return entry, ok
}

func (c *matchTimelineCache) put(key string, value matchTimelineResponse) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
		for len(c.order) > matchTimelineCacheMax {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.entries[key] = value
}

type matchTimelineRequest struct {
	GameID        int64  `json:"gameId"`
	ParticipantID int64  `json:"participantId"`
	Region        string `json:"region"`
	ServerID      string `json:"serverId"`
	PlayerRef     string `json:"playerRef"`
}

func (a *app) handleGameplayMatchTimeline(w http.ResponseWriter, r *http.Request) {
	var request matchTimelineRequest
	if err := decodeJSONRequest(r, &request, 4<<10); err != nil {
		http.Error(w, "查询参数无效", http.StatusBadRequest)
		return
	}
	if request.GameID <= 0 || request.ParticipantID <= 0 {
		http.Error(w, "查询参数无效", http.StatusBadRequest)
		return
	}
	isKR := strings.EqualFold(strings.TrimSpace(request.Region), riotRegionKR)
	regionKey := "cn"
	diagnosticRegion := "cn"
	if isKR {
		regionKey = riotRegionKR
		diagnosticRegion = riotRegionKR
		if strings.TrimSpace(request.ServerID) != "" {
			http.Error(w, "韩服时间线不能指定国服服务器", http.StatusBadRequest)
			return
		}
	} else {
		var reference gameplayReference
		var refOK bool
		if strings.TrimSpace(request.PlayerRef) != "" {
			reference, refOK = a.resolveGameplayReferenceDetails(request.PlayerRef)
			if !refOK || strings.EqualFold(reference.Region, riotRegionKR) {
				http.Error(w, "玩家引用无效或与服务器不一致", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(request.ServerID) == "" {
				request.ServerID = reference.ServerID
			}
		}
		if strings.TrimSpace(request.ServerID) != "" {
			serverID, ok := normalizeTencentServerID(request.ServerID)
			if !ok {
				http.Error(w, "国服服务器无效", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(request.PlayerRef) != "" && (!refOK || reference.ServerID == "" || reference.ServerID != serverID) {
				http.Error(w, "玩家引用与所选服务器不一致", http.StatusBadRequest)
				return
			}
			request.ServerID = serverID
			regionKey = "cn:" + serverID
		}
		// ServerID 仍为空时仅兼容旧版当前大区请求；带玩家引用的跨服
		// 请求已从后端引用恢复作用域，无法通过省略字段回落当前大区。
	}
	cacheSources := []string{dataSourceRiot}
	if !isKR {
		cacheSources = []string{dataSourceLCU, dataSourceSGP}
		if request.ServerID != "" {
			if client, _, clientErr := a.gameplayClient(); clientErr == nil && isRemoteTencentServer(client, request.ServerID) {
				cacheSources = []string{dataSourceSGP}
			}
		}
	}
	for _, cacheSource := range cacheSources {
		if cached, ok := a.matchTimelines.get(matchTimelineCacheKey(cacheSource, regionKey, request.GameID, request.ParticipantID)); ok {
			respondJSON(w, cached)
			return
		}
	}
	var frames []timelineFrame
	var source string
	var attempts []DataSourceAttempt
	var fallbackReason string
	var err error
	if isKR {
		if a.riot == nil {
			http.Error(w, "Riot 接口不可用", http.StatusConflict)
			return
		}
		frames, err = a.riot.matchTimeline(r.Context(), fmt.Sprintf("KR_%d", request.GameID))
		source = dataSourceRiot
		if err == nil {
			attempts = []DataSourceAttempt{{Source: dataSourceRiot, Outcome: dataSourceSuccess}}
		} else if !isCancellation(err) {
			attempts = []DataSourceAttempt{{Source: dataSourceRiot, Outcome: dataSourceFailed, Message: safeDiagnosticReason(err)}}
		}
	} else {
		client, _, clientErr := a.gameplayClient()
		if clientErr != nil {
			http.Error(w, clientErr.Error(), http.StatusConflict)
			return
		}
		frames, source, attempts, fallbackReason, err = a.loadMatchTimelineCNDecision(r.Context(), client, request.ServerID, request.GameID)
	}
	if err != nil || len(frames) == 0 {
		if isCancellation(err) {
			return
		}
		// 失败结果不写入缓存：下一次展开构建页时可以重试。
		detail := "这场对局暂时读取不到时间线数据"
		if err != nil {
			detail += "：" + safeDiagnosticReason(err)
		}
		a.recordDiagnostic(map[string]any{"event": "match_timeline_failed", "region": diagnosticRegion, "source": source, "reason": safeDiagnosticReason(err), "attempts": attempts, "fallback_reason": fallbackReason})
		respondJSON(w, matchTimelineResponse{Available: false, Detail: detail, Attempts: attempts, FallbackReason: fallbackReason})
		return
	}
	groups, skills := extractParticipantTimeline(frames, request.ParticipantID)
	response := matchTimelineResponse{Available: len(groups) > 0 || len(skills) > 0, Source: source, Attempts: attempts, FallbackReason: fallbackReason, ItemGroups: groups, SkillOrder: skills}
	if !response.Available {
		response.Detail = "时间线里没有该玩家的装备与技能事件"
		eventCount, eventTypes := summarizeTimelineEventTypes(frames)
		a.recordDiagnostic(map[string]any{
			"event": "match_timeline_empty", "region": diagnosticRegion, "source": source,
			"frames": len(frames), "events": eventCount, "event_types": eventTypes,
			"event_key_samples": sampleTimelineEventKeys(frames, 3),
		})
	} else {
		a.recordDiagnostic(map[string]any{"event": "match_timeline_succeeded", "region": diagnosticRegion, "source": source, "frames": len(frames), "item_groups": len(groups), "skill_ups": len(skills), "attempts": attempts, "fallback_reason": fallbackReason})
	}
	// 空结果通常意味着客户端返回了不兼容的包装或参与者编号；不缓存它，
	// 让用户再次展开时可以在客户端更新后重试，并保留诊断事件供真机分析。
	if response.Available {
		a.matchTimelines.put(matchTimelineCacheKey(source, regionKey, request.GameID, request.ParticipantID), response)
	}
	respondJSON(w, response)
}

func matchTimelineCacheKey(source, regionKey string, gameID, participantID int64) string {
	key := regionKey + "|" + strconv.FormatInt(gameID, 10) + "|" + strconv.FormatInt(participantID, 10)
	return sourceScopedKey(source, key)
}
