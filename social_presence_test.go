package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSocialParty(t *testing.T) {
	tests := []struct {
		name, raw, state, source string
		size, capacity, queue    int64
	}{
		{"missing", "", "missing", "", 0, 0, 0},
		{"null", "null", "missing", "", 0, 0, 0},
		{"new members", `{"maxPlayers":5,"queueId":2400,"summonerPuuids":["member-a","member-b"],"summoners":[1,2]}`, "complete", "summonerPuuids", 2, 5, 2400},
		{"legacy members and string counts", `{"maxPlayers":"5","queueId":"1100","summoners":["7",7,0,-1,null]}`, "complete", "summoners", 1, 5, 1100},
		{"placeholder legacy IDs", `{"maxPlayers":5,"summoners":[0,0],"summonerPuuids":["a","b","a","", "00000000-0000-0000-0000-000000000000"]}`, "complete", "summonerPuuids", 2, 5, 0},
		{"unknown capacity", `{"queueId":450,"summoners":[7]}`, "capacity_unavailable", "summoners", 1, 0, 450},
		{"no invented solo member", `{"maxPlayers":5,"queueId":450}`, "members_unavailable", "none", 0, 5, 450},
		{"conflicting sources", `{"maxPlayers":5,"summoners":[7,8],"summonerPuuids":["a"]}`, "conflicting_member_counts", "conflict", 0, 5, 0},
		{"count overflow", `{"maxPlayers":1,"summoners":[7,8]}`, "count_exceeds_capacity", "summoners", 0, 1, 0},
		{"decimal capacity is not rounded", `{"maxPlayers":1.5,"summoners":[7]}`, "capacity_unavailable", "summoners", 1, 0, 0},
		{"unsupported schema", `{"members":[7],"maxPlayers":5}`, "members_unavailable", "none", 0, 5, 0},
		{"invalid PUUID does not silently fall back", `{"maxPlayers":5,"summonerPuuids":["a",42],"summoners":[7]}`, "invalid_members", "none", 0, 5, 0},
		{"invalid ID does not partially count", `{"maxPlayers":5,"summoners":[7,{}]}`, "invalid_members", "none", 0, 5, 0},
		{"malformed", `{"maxPlayers":`, "invalid_json_object", "", 0, 0, 0},
		{"array", `[]`, "invalid_json_object", "", 0, 0, 0},
		{"too large", strings.Repeat("x", 16385), "too_large", "", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSocialParty(tt.raw)
			if got.Size != tt.size || got.Capacity != tt.capacity || got.QueueID != tt.queue || got.State != tt.state || got.CountSource != tt.source {
				t.Fatalf("got %+v; want size=%d capacity=%d queue=%d state=%s source=%s", got, tt.size, tt.capacity, tt.queue, tt.state, tt.source)
			}
		})
	}
}

func TestSocialPresenceLobbyAndPhasePrecedence(t *testing.T) {
	party := `{"maxPlayers":5,"queueId":2400,"summonerPuuids":["a","b"]}`
	for _, tt := range []struct {
		name, availability, status, product, party, phase, label string
		size                                                     int64
	}{
		{"online lobby", "chat", "hosting_NORMAL", "", party, "lobby", "海克斯大乱斗", 2},
		{"idle chat with live party", "chat", "outOfGame", "", party, "lobby", "海克斯大乱斗", 2},
		{"ordinary online", "chat", "outOfGame", "", "", "outOfGame", "单排/双排", 0},
		{"bad party stays in lobby", "chat", "hosting_NORMAL", "", "bad-json", "lobby", "单排/双排", 0},
		{"queue overrides old party", "chat", "inQueue", "", party, "inQueue", "单排/双排", 0},
		{"select overrides old party", "chat", "championSelect", "", party, "championSelect", "单排/双排", 0},
		{"game overrides old party", "chat", " inGame ", "", party, "inGame", "单排/双排", 0},
		{"spectator overrides old party", "spectating", "spectating", "", party, "spectating", "单排/双排", 0},
		{"offline ignores party", "offline", "outOfGame", "", party, "outOfGame", "单排/双排", 0},
		{"mobile ignores party", "mobile", "outOfGame", "", party, "outOfGame", "单排/双排", 0},
		{"away ignores stale party", "away", "outOfGame", "", party, "outOfGame", "单排/双排", 0},
		{"busy does not infer a lobby", "dnd", "outOfGame", "", party, "outOfGame", "单排/双排", 0},
		{"observing availability ignores stale party", "spectating", "outOfGame", "", party, "outOfGame", "单排/双排", 0},
		{"other product ignores party", "chat", "outOfGame", "valorant", party, "outOfGame", "单排/双排", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			friends := convertFriends([]lcuChatFriend{{GameName: "测试", Availability: tt.availability, Product: tt.product,
				Lol: map[string]string{"gameStatus": tt.status, "pty": tt.party, "queueId": "420", "timeStamp": "1700000000", "championId": "64"}}}, map[int64]string{64: "盲僧"}, nil)
			got := friends[0]
			if got.GameStatus != tt.phase || got.QueueLabel != tt.label || got.PartySize != tt.size || (tt.size == 0 && got.PartyCapacity != 0 && tt.name != "bad party stays in lobby") {
				t.Fatalf("got %+v", got)
			}
			if got.GameStartedAt != 1700000000000 || got.ChampionName != "盲僧" {
				t.Fatalf("existing game details lost: %+v", got)
			}
		})
	}
}

func TestSocialQueueLabels(t *testing.T) {
	for _, tt := range []struct {
		id          int64
		kind, label string
	}{
		{2400, "", "海克斯大乱斗"}, {1100, "", "排位赛（云顶之弈）"},
		{1090, "", "匹配模式（云顶之弈）"}, {0, "RANKED_TFT", "排位赛（云顶之弈）"},
		{0, "ARAM_UNRANKED_5x5", "极地大乱斗"}, {0, "", ""},
		{0, "unknown-or-private-value", ""}, {99999, "RANKED_TFT", "其他模式"},
	} {
		label, _ := socialQueueLabel(tt.id, tt.kind, nil)
		if label != tt.label {
			t.Errorf("%d/%s got %q, want %q", tt.id, tt.kind, label, tt.label)
		}
	}
	if label, source := socialQueueLabel(2400, "", map[int64]string{2400: "客户端新模式名称"}); label != "客户端新模式名称" || source != "client_catalog" {
		t.Fatalf("client label must win: %q/%s", label, source)
	}
	for _, clientLabel := range []string{"排位赛", "排位赛（云顶之弈）"} {
		if label, _ := socialQueueLabel(1100, "", map[int64]string{1100: clientLabel}); label != "排位赛（云顶之弈）" {
			t.Fatalf("TFT short name should be unambiguous: %q", label)
		}
	}
}

func TestSocialLobbyResponseAndDiagnosticsStayReadOnlyAndPrivate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	party := `{"maxPlayers":5,"queueId":2400,"summonerPuuids":["private-member-a","private-member-b"],"summoners":[123456789,123456790],"partyId":"private-party","token":"private-token","private-field":"private-value"}`
	rawFriends := []lcuChatFriend{
		{GameName: "房间好友", Availability: "chat", PUUID: "private-owner", Lol: map[string]string{"gameStatus": "hosting_NORMAL", "queueId": "420", "pty": party}},
		{GameName: "异常好友", Availability: "secret-state", Lol: map[string]string{"gameStatus": "secret-phase", "pty": "private-malformed-payload"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/lol-chat/v1/friend-groups":
			_ = json.NewEncoder(w).Encode([]lcuFriendGroup{})
		case "/lol-chat/v1/friends":
			_ = json.NewEncoder(w).Encode(rawFriends)
		case "/lol-game-queues/v1/queues":
			_, _ = w.Write([]byte(`[{"id":2400,"shortName":"海克斯大乱斗"}]`))
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "private-auth", http: server.Client()}
	a := &app{token: "session", connected: true, lcu: client, storage: trackTestStore(t, &localStore{root: root})}
	for range 2 {
		recorder := httptest.NewRecorder()
		a.handleSocialFriends(recorder, httptest.NewRequest(http.MethodGet, "/api/social/friends", nil))
		if recorder.Code != 200 {
			t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
		}
		var response socialFriendsResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Friends) != 2 || response.Friends[0].PartySize != 2 || response.Friends[0].PartyCapacity != 5 || response.Friends[0].QueueLabel != "海克斯大乱斗" {
			t.Fatalf("bad lobby response: %s", recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "private-") || strings.Contains(recorder.Body.String(), "123456789") {
			t.Fatalf("party identity leaked: %s", recorder.Body.String())
		}
	}
	rawLog, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	for _, private := range []string{"private-", "123456789", "房间好友", "异常好友", "secret-state", "secret-phase"} {
		if strings.Contains(log, private) {
			t.Fatalf("diagnostic leaked %s: %s", private, log)
		}
	}
	if strings.Count(log, `"event":"social_presence_resolved"`) != 1 || !strings.Contains(log, `"party_size":2`) || !strings.Contains(log, "invalid_json_object") {
		t.Fatalf("missing or duplicated sanitized summary: %s", log)
	}
}
