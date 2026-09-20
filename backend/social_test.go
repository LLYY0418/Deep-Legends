package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSocialFriendsResponseUsesAnonymousPlayerReferences(t *testing.T) {
	puuid := strings.Repeat("f", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-chat/v1/friend-groups":
			_, _ = io.WriteString(w, `[{"id":1,"name":"好友","priority":1}]`)
		case "/lol-chat/v1/friends":
			_, _ = io.WriteString(w, `[{"gameName":"好友玩家","gameTag":"4321","icon":27,"groupId":1,"displayGroupId":1,"puuid":"`+puuid+`","summonerId":9988,"availability":"chat"}]`)
		case "/lol-game-data/assets/v1/queues.json":
			_, _ = io.WriteString(w, `[]`)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "lcu-secret", http: server.Client()}
	a := &app{token: "session-secret", connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleSocialFriends(recorder, httptest.NewRequest(http.MethodGet, "/api/social/friends", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if strings.Contains(body, `"puuid"`) || strings.Contains(body, `"summonerId"`) || strings.Contains(body, puuid) {
		t.Fatalf("stable player identity leaked in response: %s", body)
	}
	var response socialFriendsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Friends) != 1 || response.Friends[0].PlayerRef == "" {
		t.Fatalf("friends = %#v", response.Friends)
	}
	reference, ok := a.resolveGameplayReferenceDetails(response.Friends[0].PlayerRef)
	if !ok || reference.PlayerRef != puuid || reference.GameName != "好友玩家" || reference.TagLine != "4321" || reference.ProfileIconID != 27 {
		t.Fatalf("reference = %#v, ok = %v", reference, ok)
	}
}

func TestSocialFriendsNeverProbesSpectatorForInGameFriends(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	puuid := strings.Repeat("p", 48)
	methods := make([]string, 0)
	spectatorRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		switch r.URL.Path {
		case "/lol-chat/v1/friend-groups":
			_, _ = io.WriteString(w, `[]`)
		case "/lol-chat/v1/friends":
			_, _ = io.WriteString(w, `[{"gameName":"对局好友","gameTag":"4321","puuid":"`+puuid+`","summonerId":9988,"availability":"chat","lol":{"gameStatus":"inGame","championId":"64","queueId":"1750","timeStamp":"1700000000000"}}]`)
		case "/lol-game-data/assets/v1/queues.json":
			_, _ = io.WriteString(w, `[]`)
		case "/lol-spectator/v1/spectate/launch":
			spectatorRequests++
			_, _ = io.WriteString(w, `{"canLaunch":false,"secret":"token-value","player":"`+puuid+`"}`)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "lcu-secret", http: server.Client()}
	a := &app{token: "session-secret", connected: true, lcu: client, storage: trackTestStore(t, &localStore{root: root})}
	for range 2 {
		recorder := httptest.NewRecorder()
		a.handleSocialFriends(recorder, httptest.NewRequest(http.MethodGet, "/api/social/friends", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
	if spectatorRequests != 0 {
		t.Fatalf("spectator GET requests = %d, want none", spectatorRequests)
	}
	for _, method := range methods {
		if method != http.MethodGet {
			t.Fatalf("read-only social probe used %s", method)
		}
	}
	diagnostics, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	logText := string(diagnostics)
	if strings.Contains(logText, "lcu_spectator_read_probe") {
		t.Fatal("removed probe still logged")
	}
	for _, secret := range []string{puuid, "token-value", "对局好友", "4321", "9988"} {
		if strings.Contains(logText, secret) {
			t.Fatalf("spectator diagnostic leaked %q: %s", secret, logText)
		}
	}
}
