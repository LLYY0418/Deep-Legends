package main

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// The chat service publishes party JSON inside lol.pty (not in the local
// player's /lol-lobby session). Only project counts and queue ID; never expose
// partyId, summoners, summonerPuuids, or other party/join data.
// Contract: WordlessMeteor/LoL-DIY-Programs, src/core/config/headers.py,
// friend_hovercard_header's "pty ..." fields and party_header.
type socialParty struct {
	Size, Capacity, QueueID int64
	State, CountSource      string
}

func socialPresenceInt(raw json.RawMessage, limit int64) int64 {
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, `"`) {
		if json.Unmarshal(raw, &text) != nil {
			return 0
		}
	}
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || value <= 0 || value > limit {
		return 0
	}
	return value
}

func socialPartyMemberCount(raw json.RawMessage, puuids bool) (int64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, true
	}
	var members []json.RawMessage
	if json.Unmarshal(raw, &members) != nil || len(members) > 100 {
		return 0, false
	}
	unique := make(map[string]struct{}, len(members))
	for _, member := range members {
		var key string
		if puuids {
			if json.Unmarshal(member, &key) != nil {
				return 0, false
			}
			key = strings.TrimSpace(key)
			if key == "" || strings.Trim(key, "0-") == "" {
				continue
			}
		} else {
			id := socialPresenceInt(member, 1<<63-1)
			if id == 0 {
				// Empty/zero legacy IDs coexist with the newer PUUID list.
				switch strings.TrimSpace(string(member)) {
				case "null", "0", "-1", `""`, `"0"`, `"-1"`:
					continue
				default:
					return 0, false
				}
			}
			key = strconv.FormatInt(id, 10)
		}
		unique[key] = struct{}{}
	}
	return int64(len(unique)), true
}

func parseSocialParty(raw string) socialParty {
	party := socialParty{State: "missing"}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return party
	}
	if len(raw) > 16*1024 {
		party.State = "too_large"
		return party
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil || fields == nil {
		party.State = "invalid_json_object"
		return party
	}
	party.Capacity = socialPresenceInt(fields["maxPlayers"], 100)
	party.QueueID = socialPresenceInt(fields["queueId"], 100000)
	puuids, validPuuids := socialPartyMemberCount(fields["summonerPuuids"], true)
	ids, validIDs := socialPartyMemberCount(fields["summoners"], false)
	if !validPuuids || !validIDs {
		party.State, party.CountSource = "invalid_members", "none"
		return party
	}
	party.Size, party.CountSource = puuids, "summonerPuuids"
	if puuids == 0 {
		party.Size, party.CountSource = ids, "summoners"
	}
	party.State = "complete"
	if puuids > 0 && ids > 0 && puuids != ids {
		party.Size, party.CountSource, party.State = 0, "conflict", "conflicting_member_counts"
	} else if party.Capacity > 0 && party.Size > party.Capacity {
		party.Size, party.State = 0, "count_exceeds_capacity"
	} else if party.Size == 0 {
		party.CountSource, party.State = "none", "members_unavailable"
	} else if party.Capacity == 0 {
		party.State = "capacity_unavailable"
	}
	return party
}

func socialGameStatus(raw string) string {
	switch status := strings.ToLower(strings.TrimSpace(raw)); status {
	case "ingame":
		return "inGame"
	case "championselect", "champselect":
		return "championSelect"
	case "inqueue":
		return "inQueue"
	case "spectating":
		return "spectating"
	case "outofgame", "":
		return "outOfGame"
	case "lobby", "inlobby":
		return "lobby"
	default:
		if strings.HasPrefix(status, "hosting_") {
			return "lobby"
		}
		return "unknown"
	}
}

// Queue metadata is preferred. The TFT fallback belongs to social presence,
// not the gameplay history filter (which deliberately excludes TFT).
var socialTFTQueueLabels = map[int64]string{
	1090: "匹配模式（云顶之弈）", 1100: "排位赛（云顶之弈）", 1110: "云顶之弈教学",
	1130: "狂暴模式（云顶之弈）", 1160: "双人作战（云顶之弈）",
}

func socialQueueLabel(id int64, queueType string, labels map[int64]string) (string, string) {
	if label := strings.TrimSpace(labels[id]); id > 0 && label != "" {
		if socialTFTQueueLabels[id] != "" && !strings.Contains(label, "云顶") && !strings.Contains(strings.ToLower(label), "tft") && !strings.Contains(strings.ToLower(label), "teamfight") {
			label += "（云顶之弈）"
		}
		return label, "client_catalog"
	}
	if definition, ok := supportedQueueDefinition(id); ok {
		return definition.Name, "queue_definition"
	}
	if label := socialTFTQueueLabels[id]; label != "" {
		return label, "tft_queue"
	}
	// A valid, unknown queue ID must not be relabelled using a stale type.
	if id > 0 {
		return queueLabel(id, "", nil), "unknown_queue"
	}
	labelsByType := map[string]string{
		"RANKED_SOLO_5X5": "单排/双排", "RANKED_FLEX_SR": "灵活组排",
		"NORMAL": "匹配模式", "NORMAL_5X5_BLIND": "匹配模式（盲选）", "NORMAL_5X5_DRAFT": "匹配模式（征召）",
		"ARAM_UNRANKED_5X5": "极地大乱斗", "ARAM": "极地大乱斗", "ARAM_MAYHEM": "海克斯大乱斗", "KIWI": "海克斯大乱斗",
		"NORMAL_TFT": "匹配模式（云顶之弈）", "RANKED_TFT": "排位赛（云顶之弈）",
		"RANKED_TFT_TURBO": "狂暴模式（云顶之弈）", "RANKED_TFT_DOUBLE_UP": "双人作战（云顶之弈）",
		"CUSTOM_GAME": "自定义对局",
	}
	if label := labelsByType[strings.ToUpper(strings.TrimSpace(queueType))]; label != "" {
		return label, "queue_type"
	}
	return "", "unavailable"
}

// Fixed enums and counts only: no player names, identity values, raw pty, error
// text, unknown field names, or other untrusted presence strings in diagnostics.
type socialPresenceDiagnostic struct {
	Availability  string `json:"availability"`
	Phase         string `json:"phase"`
	PartyState    string `json:"party_state"`
	CountSource   string `json:"count_source"`
	QueueID       int64  `json:"queue_id"`
	QueueSource   string `json:"queue_source"`
	LabelSource   string `json:"label_source"`
	PartySize     int64  `json:"party_size"`
	PartyCapacity int64  `json:"party_capacity"`
	PartyApplied  bool   `json:"party_applied"`
	OtherProduct  bool   `json:"other_product"`
}

func applySocialPresence(friend *socialFriend, lol map[string]string, labels map[int64]string) {
	party := parseSocialParty(lol["pty"])
	phase := socialGameStatus(lol["gameStatus"])
	queueID := socialPresenceInt(json.RawMessage(strconv.Quote(lol["queueId"])), 100000)
	queueSource := "lol"
	otherProduct := friend.Product != "" && !strings.EqualFold(friend.Product, "league_of_legends")
	chatting := friend.Availability == "chat" || friend.Availability == "online"
	available := chatting || friend.Availability == "dnd"
	if chatting && !otherProduct && phase == "outOfGame" && party.Size > 0 {
		phase = "lobby"
	}
	applyParty := available && !otherProduct && phase == "lobby"
	if applyParty {
		friend.PartySize, friend.PartyCapacity = party.Size, party.Capacity
		if party.QueueID > 0 {
			queueID, queueSource = party.QueueID, "pty"
		}
	}
	label, labelSource := socialQueueLabel(queueID, lol["gameQueueType"], labels)
	friend.GameStatus, friend.QueueLabel = phase, label
	availability := friend.Availability
	switch availability {
	case "chat", "online", "away", "dnd", "mobile", "offline", "none", "spectating", "":
	default:
		availability = "unknown"
	}
	friend.presence = socialPresenceDiagnostic{
		Availability: availability, Phase: phase, PartyState: party.State, CountSource: party.CountSource,
		QueueID: queueID, QueueSource: queueSource, LabelSource: labelSource,
		PartySize: party.Size, PartyCapacity: party.Capacity, PartyApplied: applyParty, OtherProduct: otherProduct,
	}
}

func (a *app) recordSocialPresenceDiagnostics(friends []socialFriend) {
	variants := make(map[string]int)
	for _, friend := range friends {
		raw, _ := json.Marshal(friend.presence)
		variants[string(raw)]++
	}
	keys := make([]string, 0, len(variants))
	for key := range variants {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, map[string]any{"presence": json.RawMessage(key), "count": variants[key]})
	}
	a.recordDiagnostic(map[string]any{"event": "social_presence_resolved", "friend_count": len(friends), "variants": rows})
}
