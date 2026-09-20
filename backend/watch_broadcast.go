package main

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	watchCampPrefix     = "当前阵营位置："
	watchLineupPrefix   = "当前己方英雄："
	watchPositionPrefix = "我的分路："
)

type watchBroadcastOption struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Template string `json:"template"`
	TeamOnly bool   `json:"teamOnly,omitempty"`
}

// UI declarations and emitted prefixes share the same source of truth.
func positionBroadcastOptions() []watchBroadcastOption {
	return []watchBroadcastOption{
		{Key: "camp", Label: "阵营位置（保留）", Template: watchCampPrefix + "蓝色方 / 红色方"},
		{Key: "teamComposition", Label: "附带己方英雄（仅队伍频道）", Template: watchLineupPrefix + "英雄名、英雄名", TeamOnly: true},
		{Key: "assignedPosition", Label: "附带我的分路（客户端提供时）", Template: watchPositionPrefix + "上路 / 打野 / 中路 / 下路 / 辅助"},
	}
}

func (r *watchRunner) positionBroadcastMessage(camp string, session map[string]any, rule watchBroadcastRule) string {
	parts := []string{watchCampPrefix + camp}
	record := func(item, reason string) {
		// Record inclusion/skipping without player identities or message contents.
		r.record(map[string]any{"event": "watch_action", "action": "position-broadcast", "result": "armed", "item": item, "reason": reason})
	}
	players := mapSliceFromPayload(session, "myTeam")
	if rule.TeamComposition {
		reason := "lineup-unavailable"
		if rule.Visibility != "team" {
			reason = "team-only"
		} else if r.broadcastChampionNames != nil && len(players) > 0 && len(players) <= 5 {
			names := r.broadcastChampionNames()
			lineup := make([]string, 0, len(players))
			for _, player := range players {
				name := strings.TrimSpace(names[int64(anyInt(player, "championId"))])
				if name == "" || len([]rune(name)) > 30 || strings.ContainsAny(name, "\r\n") {
					break
				}
				lineup = append(lineup, name)
			}
			if len(lineup) == len(players) {
				parts = append(parts, watchLineupPrefix+strings.Join(lineup, "、"))
				reason = "included"
			}
		}
		record("teamComposition", reason)
	}
	if rule.AssignedPosition {
		reason := "position-unavailable"
		// Missing cell IDs are unknown, never implicitly the zero cell.
		local, err := strconv.ParseInt(fmt.Sprint(session["localPlayerCellId"]), 10, 64)
		if err == nil && local >= 0 {
			for _, player := range players {
				cell, err := strconv.ParseInt(fmt.Sprint(player["cellId"]), 10, 64)
				if err != nil || cell != local {
					continue
				}
				position := map[string]string{"TOP": "上路", "JUNGLE": "打野", "MIDDLE": "中路", "MID": "中路", "BOTTOM": "下路", "ADC": "下路", "UTILITY": "辅助", "SUPPORT": "辅助"}[strings.ToUpper(strings.TrimSpace(anyString(player, "assignedPosition")))]
				if position != "" {
					parts = append(parts, watchPositionPrefix+position)
					reason = "included"
				}
				break
			}
		}
		record("assignedPosition", reason)
	}
	return strings.Join(parts, "；")
}
