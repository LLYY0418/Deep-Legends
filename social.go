package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	spectatorReadProbePath    = "/lol-spectator/v1/spectate/launch"
	spectatorReadProbeTimeout = 1500 * time.Millisecond
)

var spectatorPresenceFieldNames = []string{
	"championId", "gameId", "gameMode", "gameQueueType", "gameStatus", "mapId", "queueId", "timeStamp",
}

// 好友数据完全来自本机客户端聊天服务，只读；分组、顺序与折叠初始态
// 均以客户端返回为准，界面不做二次编辑。

type lcuFriendGroup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Priority    int64  `json:"priority"`
	Collapsed   bool   `json:"collapsed"`
	IsMetaGroup bool   `json:"isMetaGroup"`
}

type lcuChatFriend struct {
	Availability            string            `json:"availability"`
	GameName                string            `json:"gameName"`
	GameTag                 string            `json:"gameTag"`
	Name                    string            `json:"name"`
	Icon                    int64             `json:"icon"`
	GroupID                 int64             `json:"groupId"`
	DisplayGroupID          int64             `json:"displayGroupId"`
	Note                    string            `json:"note"`
	PUUID                   string            `json:"puuid"`
	SummonerID              int64             `json:"summonerId"`
	StatusMessage           string            `json:"statusMessage"`
	Product                 string            `json:"product"`
	ProductName             string            `json:"productName"`
	LastSeenOnlineTimestamp any               `json:"lastSeenOnlineTimestamp"`
	Lol                     map[string]string `json:"lol"`
}

type socialFriendGroup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Priority    int64  `json:"priority"`
	Collapsed   bool   `json:"collapsed"`
	IsMetaGroup bool   `json:"isMetaGroup"`
}

type socialFriend struct {
	PlayerRef     string `json:"playerRef,omitempty"`
	GameName      string `json:"gameName"`
	TagLine       string `json:"tagLine,omitempty"`
	Note          string `json:"note,omitempty"`
	Icon          int64  `json:"icon,omitempty"`
	Availability  string `json:"availability"`
	StatusMessage string `json:"statusMessage,omitempty"`
	GroupID       int64  `json:"groupId"`
	DisplayGroup  int64  `json:"displayGroupId"`
	Product       string `json:"product,omitempty"`
	ProductName   string `json:"productName,omitempty"`
	LastSeenAt    string `json:"lastSeenAt,omitempty"`
	GameStatus    string `json:"gameStatus,omitempty"`
	ChampionID    int64  `json:"championId,omitempty"`
	ChampionName  string `json:"championName,omitempty"`
	QueueLabel    string `json:"queueLabel,omitempty"`
	GameStartedAt int64  `json:"gameStartedAt,omitempty"`
	reference     gameplayReference
}

type socialFriendsResponse struct {
	Groups  []socialFriendGroup `json:"groups"`
	Friends []socialFriend      `json:"friends"`
}

func (a *app) handleSocialFriends(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	var rawGroups []lcuFriendGroup
	if err := client.GetJSON("/lol-chat/v1/friend-groups", &rawGroups); err != nil {
		http.Error(w, "读取好友分组失败："+friendlyError(err), http.StatusBadGateway)
		return
	}
	var rawFriends []lcuChatFriend
	if err := client.GetJSON("/lol-chat/v1/friends", &rawFriends); err != nil {
		http.Error(w, "读取好友列表失败："+friendlyError(err), http.StatusBadGateway)
		return
	}
	a.maybeRecordFriendSpectatorReadProbe(r.Context(), client, rawFriends)
	names := a.championNames()
	queueLabels := loadQueueLabels(client)
	friends := convertFriends(rawFriends, names, queueLabels)
	for index := range friends {
		friends[index].PlayerRef = a.registerGameplayReferenceDetails(friends[index].reference)
	}
	respondJSON(w, socialFriendsResponse{
		Groups:  convertFriendGroups(rawGroups),
		Friends: friends,
	})
}

// The spectator endpoint is deliberately probed with one fixed GET only. This
// records the local client's observable read contract before any launch action
// is designed; it must never become a POST/PUT/PATCH/DELETE request here.
func (a *app) maybeRecordFriendSpectatorReadProbe(parent context.Context, client *LCUClient, friends []lcuChatFriend) {
	inGameCount, _ := friendSpectatorPresenceFields(friends)
	if a == nil || a.storage == nil || client == nil || inGameCount == 0 {
		return
	}
	a.spectatorReadProbeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(parent, spectatorReadProbeTimeout)
		defer cancel()
		a.recordDiagnostic(friendSpectatorReadProbe(ctx, client, friends))
	})
}

func friendSpectatorReadProbe(ctx context.Context, client *LCUClient, friends []lcuChatFriend) map[string]any {
	inGameCount, presenceFields := friendSpectatorPresenceFields(friends)
	event := map[string]any{
		"event":                "lcu_spectator_read_probe",
		"method":               http.MethodGet,
		"path":                 spectatorReadProbePath,
		"in_game_friend_count": inGameCount,
		"presence_fields":      presenceFields,
	}
	payload, err := client.GetBytesContext(ctx, spectatorReadProbePath)
	if err != nil {
		state, status := spectatorReadProbeErrorState(err)
		event["state"] = state
		if status > 0 {
			event["http_status"] = status
		}
		return event
	}
	shape, fields := spectatorReadProbePayloadShape(payload)
	event["state"] = "success"
	if shape == "invalid-json" {
		event["state"] = "invalid-json"
	}
	event["response_shape"] = shape
	if len(fields) > 0 {
		event["response_fields"] = fields
	}
	return event
}

func friendSpectatorPresenceFields(friends []lcuChatFriend) (int, []string) {
	inGameCount := 0
	present := make(map[string]bool)
	for _, friend := range friends {
		if !strings.EqualFold(strings.TrimSpace(friend.Lol["gameStatus"]), "inGame") {
			continue
		}
		inGameCount++
		for _, name := range spectatorPresenceFieldNames {
			if strings.TrimSpace(friend.Lol[name]) != "" {
				present[name] = true
			}
		}
	}
	fields := make([]string, 0, len(present))
	for _, name := range spectatorPresenceFieldNames {
		if present[name] {
			fields = append(fields, name)
		}
	}
	return inGameCount, fields
}

func spectatorReadProbeErrorState(err error) (string, int) {
	var httpErr *LCUHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusNotFound:
			return "not-found", httpErr.StatusCode
		case http.StatusMethodNotAllowed:
			return "method-not-allowed", httpErr.StatusCode
		default:
			return "http-error", httpErr.StatusCode
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout", 0
	}
	return "request-failed", 0
}

func spectatorReadProbePayloadShape(payload []byte) (string, []string) {
	if strings.TrimSpace(string(payload)) == "" {
		return "empty", nil
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return "invalid-json", nil
	}
	switch typed := value.(type) {
	case map[string]any:
		fields := make([]string, 0, len(typed))
		for name := range typed {
			if safeSpectatorProbeFieldName(name) {
				fields = append(fields, name)
			}
		}
		sort.Strings(fields)
		if len(fields) > 32 {
			fields = fields[:32]
		}
		return "object", fields
	case []any:
		return "array", nil
	default:
		return "scalar", nil
	}
}

func safeSpectatorProbeFieldName(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (index > 0 && character >= '0' && character <= '9') || (index > 0 && (character == '_' || character == '-' || character == '.')) {
			continue
		}
		return false
	}
	return true
}

func convertFriendGroups(raw []lcuFriendGroup) []socialFriendGroup {
	groups := make([]socialFriendGroup, 0, len(raw))
	for _, group := range raw {
		name := strings.TrimSpace(group.Name)
		if name == "**Default" {
			name = "默认分组"
		}
		// 客户端自带的 OFFLINE 元分组与界面底部统一的“离线”组重复，跳过。
		if name == "" || strings.EqualFold(name, "OFFLINE") {
			continue
		}
		groups = append(groups, socialFriendGroup{
			ID: group.ID, Name: name, Priority: group.Priority,
			Collapsed: group.Collapsed, IsMetaGroup: group.IsMetaGroup,
		})
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Priority < groups[j].Priority })
	return groups
}

func convertFriends(raw []lcuChatFriend, championNames map[int64]string, queueLabels map[int64]string) []socialFriend {
	friends := make([]socialFriend, 0, len(raw))
	for _, friend := range raw {
		gameName := strings.TrimSpace(friend.GameName)
		if gameName == "" {
			gameName = strings.TrimSpace(friend.Name)
		}
		if gameName == "" {
			continue
		}
		converted := socialFriend{
			GameName:      gameName,
			TagLine:       strings.TrimSpace(friend.GameTag),
			Note:          strings.TrimSpace(friend.Note),
			Icon:          friend.Icon,
			Availability:  strings.ToLower(strings.TrimSpace(friend.Availability)),
			StatusMessage: strings.TrimSpace(friend.StatusMessage),
			GroupID:       friend.GroupID,
			DisplayGroup:  friend.DisplayGroupID,
			Product:       strings.TrimSpace(friend.Product),
			ProductName:   strings.TrimSpace(friend.ProductName),
			LastSeenAt:    chatTimestamp(friend.LastSeenOnlineTimestamp),
		}
		if lol := friend.Lol; lol != nil {
			converted.GameStatus = strings.TrimSpace(lol["gameStatus"])
			if championID, err := strconv.ParseInt(strings.TrimSpace(lol["championId"]), 10, 64); err == nil && championID > 0 {
				converted.ChampionID = championID
				converted.ChampionName = championNames[championID]
			}
			if queueID, err := strconv.ParseInt(strings.TrimSpace(lol["queueId"]), 10, 64); err == nil && queueID > 0 {
				converted.QueueLabel = queueLabel(queueID, "", queueLabels)
			}
			if startedAt, err := strconv.ParseInt(strings.TrimSpace(lol["timeStamp"]), 10, 64); err == nil && startedAt > 0 {
				converted.GameStartedAt = normalizeEpochMillis(startedAt)
			}
		}
		converted.reference = gameplayReference{
			PlayerRef: friend.PUUID, SummonerID: friend.SummonerID, GameName: gameName,
			TagLine: converted.TagLine, ProfileIconID: friend.Icon,
		}
		friends = append(friends, converted)
	}
	return friends
}

// chatTimestamp 归一聊天服务的最近在线时间：可能是 ISO 字符串，也可能是
// 毫秒时间戳数字；无法解析时返回空串，前端直接省略。
func chatTimestamp(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v <= 0 {
			return ""
		}
		return time.UnixMilli(int64(v)).UTC().Format(time.RFC3339)
	}
	return ""
}
