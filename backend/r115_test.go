package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Frozen pre-R115 semantics, including Unicode lowercasing and prefix families.
func r115LegacyScope(event LCUEvent) string {
	uri := strings.ToLower(event.URI)
	if strings.HasPrefix(uri, "/lol-champ-select/v1/session") {
		return "champselect"
	}
	for _, p := range []string{"/lol-champions/v1/inventories/", "/lol-champion-mastery/", "/lol-inventory/"} {
		if strings.HasPrefix(uri, p) {
			return "collection"
		}
	}
	if strings.HasPrefix(uri, "/lol-summoner/v1/current-summoner/summoner-profile") {
		return "summoner-profile"
	}
	if strings.HasPrefix(uri, "/lol-summoner/v1/current-summoner") {
		return "summoner"
	}
	for _, p := range []string{"/lol-loot/v1/player-loot-map", "/lol-rewards/v1/grants"} {
		if strings.HasPrefix(uri, p) {
			return "account"
		}
	}
	return ""
}
func TestR115_LCUEventRoutingParity(t *testing.T) {
	for _, uri := range []string{"", "/", "no-slash", "/unrelated/", "/lol-champ-select/v1/session", "/lol-champions/v1/inventories/", "/lol-champion-mastery/", "/lol-inventory/", "/lol-summoner/v1/current-summoner/summoner-profile", "/lol-summoner/v1/current-summoner", "/lol-loot/v1/player-loot-map", "/lol-rewards/v1/grants", "/LOL-İNVENTORY/", "/lol-ſummoner/v1/current-summoner"} {
		for _, form := range []string{uri, strings.ToUpper(uri)} {
			for _, suffix := range []string{"", "/", "/child", "?query=UPPER", "-other", "汉字"} {
				event := LCUEvent{URI: form + suffix}
				if got, want := lcuEventRefreshScope(event), r115LegacyScope(event); got != want {
					t.Errorf("%q got %q want %q", event.URI, got, want)
				}
			}
			for i := 0; i < len(form); i++ {
				event := LCUEvent{URI: form[:i]}
				if lcuEventRefreshScope(event) != r115LegacyScope(event) {
					t.Errorf("boundary %q", event.URI)
				}
			}
		}
	}
	stats := topLCUEventURIStats(map[string]lcuEventURIStat{"a": {URI: "a", LastBytes: 2, Count: 1}, "b": {URI: "b", LastBytes: 3, Count: 2}}, 1)
	data, _ := json.Marshal(stats)
	if string(data) != `[{"uri":"b","last_bytes":3,"count":2}]` {
		t.Fatalf("diagnostic contract changed: %s", data)
	}
}

func TestR115_BroadcastContentsAndGuards(t *testing.T) {
	for _, tc := range []struct {
		name        string
		rule        watchBroadcastRule
		id          string
		pauseAt     string
		fail        bool
		want        string
		writes      int
		messageType string
	}{
		{name: "legacy", id: "room", want: "当前阵营位置：蓝色方", writes: 1, messageType: "celebration"},
		{name: "combined", id: "room", rule: watchBroadcastRule{Visibility: "team", TeamComposition: true, AssignedPosition: true}, want: "当前阵营位置：蓝色方；当前己方英雄：瑞兹、阿狸；我的分路：中路", writes: 1, messageType: "chat"},
		{name: "invalid visibility", id: "room", rule: watchBroadcastRule{Visibility: "invalid", TeamComposition: true, AssignedPosition: true}, want: "当前阵营位置：蓝色方；我的分路：中路", writes: 1, messageType: "celebration"},
		{name: "unsafe id", id: "../messages"},
		{name: "pause before reads", id: "room", pauseAt: "start"},
		{name: "pause before write", id: "room", pauseAt: "/lol-chat/v1/conversations"},
		{name: "uncertain write", id: "room", fail: true, want: "当前阵营位置：蓝色方", writes: 1, messageType: "celebration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := newWatchRunner(nil, nil)
			runner.broadcastChampionNames = func() map[int64]string { return map[int64]string{13: "瑞兹", 103: "阿狸"} }
			if tc.pauseAt == "start" {
				runner.mu.Lock()
				runner.customSession = true
				runner.mu.Unlock()
			}
			var writes int
			var sent map[string]any
			reads := 0
			client := &LCUClient{baseURL: "http://fixture", token: "test", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "fixture" {
					t.Fatalf("external request: %s", req.URL.Host)
				}
				body := "{}"
				reads++
				switch req.URL.Path {
				case "/lol-champ-select/v1/session":
					body = `{"queueId":450,"benchEnabled":true,"localPlayerCellId":7,"myTeam":[{"cellId":7,"team":100,"championId":13,"assignedPosition":"MIDDLE","puuid":"must-not-send","gameName":"private"},{"cellId":8,"team":100,"championId":103}]}`
				case "/lol-chat/v1/me":
					body = `{"id":"self"}`
				case "/lol-chat/v1/conversations":
					b, _ := json.Marshal([]map[string]string{{"id": tc.id, "type": "championSelect"}})
					body = string(b)
				case "/lol-chat/v1/conversations/room/messages":
					writes++
					json.NewDecoder(req.Body).Decode(&sent)
					if tc.fail {
						return nil, errors.New("uncertain response")
					}
				default:
					t.Fatalf("unexpected endpoint: %s", req.URL.Path)
				}
				if tc.pauseAt == req.URL.Path {
					runner.mu.Lock()
					runner.customSession = true
					runner.mu.Unlock()
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}}
			got := runner.broadcastPosition(client, Summoner{SummonerID: 115}, tc.rule)
			if tc.name == "unsafe id" && got != "failed" {
				t.Fatalf("unsafe conversation not rejected at discovery: %s", got)
			}
			if writes != tc.writes {
				t.Fatalf("writes=%d want=%d result=%s", writes, tc.writes, got)
			}
			if writes > 0 && (sent["body"] != tc.want || sent["type"] != tc.messageType) {
				t.Fatalf("unexpected message: %v", sent)
			}
			if tc.fail && got != "write_failed" {
				t.Fatalf("uncertain POST was retryable: %s", got)
			}
			if tc.pauseAt == "start" && reads != 0 {
				t.Fatal("paused broadcast performed reads")
			}
		})
	}
}
func TestR115_BroadcastDefaultsAndMissingData(t *testing.T) {
	s := defaultWatchSettings()
	json.Unmarshal([]byte(`{"rules":{"positionBroadcast":{"enabled":true,"visibility":"team"}}}`), &s)
	if s.Rules.PositionBroadcast.TeamComposition {
		t.Fatal("upgrade enabled new messages")
	}
	if !s.Rules.PositionBroadcast.AssignedPosition {
		t.Fatal("R139: assigned position must default on, it is no longer user-configurable")
	}
	s.Rules.PositionBroadcast.Visibility = "garbage"
	if normalizeWatchSettings(s).Rules.PositionBroadcast.Visibility != "self" {
		t.Fatal("invalid visibility did not fall back")
	}
	runner := newWatchRunner(nil, nil)
	for _, raw := range []string{`{}`, `{"myTeam":[{"cellId":0,"assignedPosition":"MIDDLE"}]}`, `{"localPlayerCellId":0,"myTeam":[{"assignedPosition":"MIDDLE"}]}`, `{"localPlayerCellId":0,"myTeam":[{"cellId":0,"assignedPosition":"UNKNOWN","championId":0}]}`, `{"localPlayerCellId":"invalid","myTeam":[{"cellId":0,"assignedPosition":"TOP"}]}`, `{"localPlayerCellId":0,"myTeam":[{"cellId":0.5,"assignedPosition":"TOP"}]}`} {
		var session map[string]any
		json.Unmarshal([]byte(raw), &session)
		if got := runner.positionBroadcastMessage("红色方", session, watchBroadcastRule{Visibility: "team", TeamComposition: true, AssignedPosition: true}); got != "当前阵营位置：红色方" {
			t.Fatalf("missing data fabricated: %s", got)
		}
	}
	options := positionBroadcastOptions()
	if options[0].Template != watchCampPrefix+"蓝色方 / 红色方" || !strings.HasPrefix(options[1].Template, watchLineupPrefix) || !strings.HasPrefix(options[2].Template, watchPositionPrefix) {
		t.Fatal("declaration drift")
	}
}

func TestR139_BroadcastAssignedPositionLegacyOffIsNormalized(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	old := []byte(`{"rules":{"positionBroadcast":{"enabled":true,"visibility":"team","teamComposition":false,"assignedPosition":false}}}`)
	if err := os.WriteFile(filepath.Join(store.root, convenienceSettingsFile), old, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := loadWatchSettings(store)
	if !loaded.Rules.PositionBroadcast.AssignedPosition {
		t.Fatal("R139: old saved false value must be normalized to true")
	}
	if loaded.Rules.PositionBroadcast.TeamComposition || loaded.Rules.PositionBroadcast.Visibility != "team" {
		t.Fatalf("other broadcast settings changed: %+v", loaded.Rules.PositionBroadcast)
	}
	loaded.Rules.PositionBroadcast.AssignedPosition = false
	if err := saveWatchSettings(store, loaded); err != nil {
		t.Fatal(err)
	}
	if !loadWatchSettings(store).Rules.PositionBroadcast.AssignedPosition {
		t.Fatal("R139: save must not persist a false assigned-position value")
	}
}
func TestR115_BroadcastUncertainWriteIsNotRetried(t *testing.T) {
	var writes atomic.Int32
	done := make(chan struct{}, 1)
	r := newWatchRunner(nil, func(event string) {
		if event == "watch:failed:position-broadcast" {
			done <- struct{}{}
		}
	})
	s := defaultWatchSettings()
	s.Rules.PositionBroadcast.Enabled = true
	s.ChampSelect.Enabled = false
	r.apply(s)
	r.handleChampSelectPhase("ChampSelect")
	client := &LCUClient{baseURL: "http://fixture", token: "test", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{}`
		switch req.URL.Path {
		case "/lol-champ-select/v1/session":
			body = `{"queueId":450,"benchEnabled":true,"myTeam":[{"cellId":0,"team":100}]}`
		case "/lol-chat/v1/me":
			body = `{"id":"self"}`
		case "/lol-chat/v1/conversations":
			body = `[{"id":"room","type":"championSelect"}]`
		case "/lol-chat/v1/conversations/room/messages":
			writes.Add(1)
			return nil, errors.New("response lost")
		default:
			t.Errorf("unexpected endpoint %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	r.handleChampSelect(client, Summoner{})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("no write")
	}
	// Wait until the action worker actually exits, not merely until it emitted.
	deadline := time.Now().Add(4 * time.Second)
	for {
		r.mu.Lock()
		pending := r.pending["position-broadcast"] != nil
		r.mu.Unlock()
		if !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker retried or hung")
		}
		time.Sleep(time.Millisecond)
	}
	for range 100 {
		r.handleChampSelect(client, Summoner{})
	}
	if writes.Load() != 1 {
		t.Fatalf("duplicate POST: %d", writes.Load())
	}
	r.handleChampSelectPhase("InProgress")
}

func TestR115_AssetSingleflight(t *testing.T) {
	a := &app{}
	entered, release := make(chan struct{}), make(chan struct{})
	var loads atomic.Int32
	loader := func(ctx context.Context) ([]byte, error) {
		if loads.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
			return []byte("cached"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	owner := make(chan struct{})
	go func() { defer close(owner); a.loadAsset(context.Background(), "shared", 64, 0, loader) }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.loadAsset(ctx, "shared", 64, 0, loader)
	close(release)
	<-owner
	if !errors.Is(err, context.Canceled) || loads.Load() != 1 {
		t.Fatalf("singleflight cancellation: %v loads=%d", err, loads.Load())
	}
}
