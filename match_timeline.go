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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

/* ---------- 与 Riot / SGP / LCU 同构的时间线帧 ---------- */

type timelineEvent struct {
	Type          string `json:"type"`
	Timestamp     int64  `json:"timestamp"`
	ParticipantID int64  `json:"participantId"`
	ItemID        int64  `json:"itemId"`
	BeforeID      int64  `json:"beforeId"`
	AfterID       int64  `json:"afterId"`
	SkillSlot     int    `json:"skillSlot"`
	LevelUpType   string `json:"levelUpType"`
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
	Available  bool                `json:"available"`
	Source     string              `json:"source,omitempty"`
	Detail     string              `json:"detail,omitempty"`
	ItemGroups []timelineItemGroup `json:"itemGroups,omitempty"`
	SkillOrder []timelineSkillUp   `json:"skillOrder,omitempty"`
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
			switch event.Type {
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
}

func (t lcuGameTimeline) frames() []timelineFrame {
	if len(t.Frames) > 0 {
		return t.Frames
	}
	return t.Info.Frames
}

// loadMatchTimelineCN 按“客户端优先，SGP 兜底”的顺序读取国服时间线。
func (a *app) loadMatchTimelineCN(ctx context.Context, client *LCUClient, serverID string, gameID int64) ([]timelineFrame, string, error) {
	var err error
	// 仅目标服务器就是当前客户端服务器时尝试 LCU；跨服对局绝不能用
	// 当前大区相同 gameId 的时间线冒充目标数据。
	if serverID == "" || !isRemoteTencentServer(client, serverID) {
		var timeline lcuGameTimeline
		err = client.GetJSON(fmt.Sprintf("/lol-match-history/v1/game-timelines/%d", gameID), &timeline)
		if err == nil && len(timeline.frames()) > 0 {
			return timeline.frames(), "lcu", nil
		}
	}
	var frames []timelineFrame
	var sgpErr error
	if serverID != "" {
		frames, sgpErr = a.sgp.gameDetailsOn(ctx, client, serverID, gameID)
	} else {
		frames, sgpErr = a.sgp.gameDetails(ctx, client, gameID)
	}
	if sgpErr == nil {
		return frames, "sgp", nil
	}
	if err == nil {
		err = sgpErr
	}
	return nil, "", err
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
	if isKR {
		regionKey = riotRegionKR
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
	cacheKey := regionKey + "|" + strconv.FormatInt(request.GameID, 10) + "|" + strconv.FormatInt(request.ParticipantID, 10)
	if cached, ok := a.matchTimelines.get(cacheKey); ok {
		respondJSON(w, cached)
		return
	}
	var frames []timelineFrame
	var source string
	var err error
	if isKR {
		if a.riot == nil {
			http.Error(w, "Riot 接口不可用", http.StatusConflict)
			return
		}
		frames, err = a.riot.matchTimeline(r.Context(), fmt.Sprintf("KR_%d", request.GameID))
		source = "riot"
	} else {
		client, _, clientErr := a.gameplayClient()
		if clientErr != nil {
			http.Error(w, clientErr.Error(), http.StatusConflict)
			return
		}
		frames, source, err = a.loadMatchTimelineCN(r.Context(), client, request.ServerID, request.GameID)
	}
	if err != nil || len(frames) == 0 {
		// 失败结果不写入缓存：下一次展开构建页时可以重试。
		respondJSON(w, matchTimelineResponse{Available: false, Detail: "这场对局暂时读取不到时间线数据"})
		return
	}
	groups, skills := extractParticipantTimeline(frames, request.ParticipantID)
	response := matchTimelineResponse{Available: len(groups) > 0 || len(skills) > 0, Source: source, ItemGroups: groups, SkillOrder: skills}
	if !response.Available {
		response.Detail = "时间线里没有该玩家的装备与技能事件"
	}
	a.matchTimelines.put(cacheKey, response)
	respondJSON(w, response)
}
