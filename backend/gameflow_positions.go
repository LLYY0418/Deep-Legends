package main

import (
	"strconv"
	"strings"
)

func normalizeGameflowPosition(selectedPosition, selectedRole string) string {
	exact := func(value string) string {
		switch strings.ToUpper(strings.TrimSpace(value)) {
		case "TOP":
			return "top"
		case "JUNGLE":
			return "jungle"
		case "MIDDLE":
			return "middle"
		case "BOTTOM":
			return "bottom"
		case "UTILITY":
			return "utility"
		case "OTHER":
			return "other"
		default:
			return ""
		}
	}
	if position := exact(selectedPosition); position != "" {
		return position
	}
	first, _, _ := strings.Cut(selectedRole, ".")
	return exact(first)
}
func isStandardLivePosition(position string) bool {
	switch position {
	case "top", "jungle", "middle", "bottom", "utility":
		return true
	}
	return false
}
func liveFinalPositionShape(players []gameplayLivePlayer) (map[string]int, map[string]int) {
	sources := map[string]int{"liveclient": 0, "snapshot": 0, "gameflow": 0, "none": 0}
	counts := map[int64]map[string]int{}
	duplicates := map[string]int{}
	for _, player := range players {
		source := player.positionSource
		if source == "" || player.Position == "" {
			source = "none"
		}
		sources[source]++
		if counts[player.TeamID] == nil {
			counts[player.TeamID] = map[string]int{}
		}
		duplicates[strconv.FormatInt(player.TeamID, 10)] = 0
		if isStandardLivePosition(player.Position) {
			counts[player.TeamID][player.Position]++
		}
	}
	for team, positions := range counts {
		for _, count := range positions {
			duplicates[strconv.FormatInt(team, 10)] += max(0, count-1)
		}
	}
	return sources, duplicates
}
func gameplayLivePositionsPending(response gameplayLiveResponse) bool {
	for _, capability := range response.Capabilities {
		if capability.Name == "live-client-positions" && capability.State == capabilityPending {
			return true
		}
	}
	return false
}
