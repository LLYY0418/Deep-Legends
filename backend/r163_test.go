package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestR163TenPlayerQueueAndSnapshotCompleteness(t *testing.T) {
	for _, queueID := range []int64{420, 440, 2300, 2400, 3270} {
		if !liveTenPlayerRosterQueue(queueID) {
			t.Fatalf("queue %d must require a complete roster", queueID)
		}
	}
	for _, queueID := range []int64{420, 440, 2300, 2400, 3270} {
		if !liveAnonymousRosterQueue(queueID) {
			t.Fatalf("queue %d must accept verified anonymous slots", queueID)
		}
	}
	if liveTenPlayerRosterQueue(1700) || liveAnonymousRosterQueue(1700) {
		t.Fatal("Arena must keep its own roster rules")
	}
	response := gameplayLiveResponse{Available: true, QueueID: 2400}
	for index := 0; index < 9; index++ {
		team := int64(100)
		if index >= 5 {
			team = 200
		}
		response.Players = append(response.Players, gameplayLivePlayer{TeamID: team, HistoryState: "unavailable"})
	}
	if gameplayLiveSnapshotComplete(response) {
		t.Fatal("5+4 Hextech ARAM roster was frozen")
	}
	response.Players = append(response.Players, gameplayLivePlayer{TeamID: 200, HistoryState: "unavailable"})
	if !gameplayLiveSnapshotComplete(response) {
		t.Fatal("5+5 Hextech ARAM roster should be complete")
	}
}

func TestR163PositionlessHextechPlayerlistRequiresAllTen(t *testing.T) {
	for _, count := range []int{9, 10} {
		entries := make([]string, 0, count)
		for index := 0; index < count; index++ {
			team := "ORDER"
			if index >= 5 {
				team = "CHAOS"
			}
			identity := fmt.Sprintf(`"riotId":"Player%d#CN1"`, index)
			if index == 9 {
				identity = `"riotId":"","riotIdGameName":"","riotIdTagLine":""`
			}
			entries = append(entries, fmt.Sprintf(`{%s,"team":%q,"position":"OTHER"}`, identity, team))
		}
		a := &app{liveClientPlayerList: func(context.Context) ([]byte, int, error) {
			return []byte("[" + strings.Join(entries, ",") + "]"), http.StatusOK, nil
		}}
		snapshot, _ := a.liveClientSnapshotForGame(context.Background(), 163, "InProgress", 0, 2400)
		if got := len(snapshot.RosterPlayers); got != count && count == 10 || got != 0 && count == 9 {
			t.Fatalf("%d-entry probe returned %d entries", count, got)
		}
		if a.liveClientProbe.Succeeded != (count == 10) {
			t.Fatalf("%d-entry probe success = %v", count, a.liveClientProbe.Succeeded)
		}
	}
}

func TestR163AnonymousSlotOnlyOnVerifiedShortTeam(t *testing.T) {
	lookupFails := false
	transport := gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/lol-summoner/v1/alias/lookup" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("missing")), Header: make(http.Header)}, nil
		}
		var index int
		if _, err := fmt.Sscanf(request.URL.Query().Get("gameName"), "Player%d", &index); err != nil || index < 0 || index >= 10 {
			t.Errorf("unexpected alias query: %v", request.URL.Query())
			return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("bad query")), Header: make(http.Header)}, nil
		}
		if lookupFails && index == 8 {
			return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("lookup failed")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`[{"puuid":%q}]`, r161Ref(index)))), Header: make(http.Header)}, nil
	})
	client := &LCUClient{baseURL: "http://lcu.local", token: "test", http: &http.Client{Transport: transport}, region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	current := Summoner{PUUID: r161Ref(0), GameName: "Player0", TagLine: "CN1"}
	raw := make([]struct {
		player lcuLivePlayer
		team   int64
	}, 0, 9)
	for index := 0; index < 9; index++ {
		team := int64(100)
		if index >= 5 {
			team = 200
		}
		raw = append(raw, struct {
			player lcuLivePlayer
			team   int64
		}{lcuLivePlayer{PUUID: r161Ref(index)}, team})
	}
	snapshot := liveClientSnapshot{}
	for index := 0; index < 10; index++ {
		team := "ORDER"
		if index >= 5 {
			team = "CHAOS"
		}
		player := liveClientRosterPlayer{GameName: fmt.Sprintf("Player%d", index), TagLine: "CN1", Team: team}
		if index == 9 {
			player.GameName = ""
			player.TagLine = ""
			player.ChampionName = "测试英雄"
		}
		snapshot.RosterPlayers = append(snapshot.RosterPlayers, player)
	}
	a := &app{}
	got, appended, _, reason := a.recoverClassicLiveRoster(context.Background(), client, current, raw, snapshot, map[int64]string{11: "测试英雄"}, true)
	if appended != 1 || reason != "anonymous-placeholder" || len(got) != 10 || got[9].team != 200 || got[9].player.PUUID != "" || got[9].player.GameName != "" || got[9].player.NameVisibilityType != "HIDDEN" || got[9].player.ChampionID != 11 {
		t.Fatalf("anonymous recovery: count=%d reason=%s last=%#v", appended, reason, got[len(got)-1])
	}
	if got, appended, _, reason := a.recoverClassicLiveRoster(context.Background(), client, current, raw, snapshot, nil, false); appended != 0 || len(got) != 9 || reason != "partial" {
		t.Fatalf("disabled anonymous path created anonymous slot: count=%d reason=%s", appended, reason)
	}
	lookupFails = true
	if got, appended, _, reason := a.recoverClassicLiveRoster(context.Background(), client, current, raw, snapshot, nil, true); appended != 0 || len(got) != 9 || reason != "partial" {
		t.Fatalf("alias failure created anonymous slot: count=%d reason=%s", appended, reason)
	}
	lookupFails = false
	snapshot.RosterPlayers[4].GameName = ""
	snapshot.RosterPlayers[4].TagLine = ""
	snapshot.RosterPlayers[9].GameName = "Player9"
	snapshot.RosterPlayers[9].TagLine = "CN1"
	if got, appended, _, reason := a.recoverClassicLiveRoster(context.Background(), client, current, raw, snapshot, nil, true); appended != 1 || len(got) != 10 || reason != "resolved" || got[9].player.PUUID != r161Ref(9) {
		t.Fatalf("wrong-side anonymous slot displaced named recovery: count=%d reason=%s", appended, reason)
	}
}
