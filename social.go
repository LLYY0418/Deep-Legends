package main

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
	PUUID         string `json:"puuid,omitempty"`
	SummonerID    int64  `json:"summonerId,omitempty"`
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
}

type socialFriendsResponse struct {
	Groups  []socialFriendGroup `json:"groups"`
	Friends []socialFriend      `json:"friends"`
}

func (a *app) handleSocialFriends(w http.ResponseWriter, _ *http.Request) {
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
	names := a.championNames()
	queueLabels := loadQueueLabels(client)
	respondJSON(w, socialFriendsResponse{
		Groups:  convertFriendGroups(rawGroups),
		Friends: convertFriends(rawFriends, names, queueLabels),
	})
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
			PUUID:         friend.PUUID,
			SummonerID:    friend.SummonerID,
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
