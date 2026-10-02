package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func r161Ref(index int) string { return strings.Repeat(string(rune('a'+index)), 48) }

func TestR161IncompleteRankedRosterDoesNotFreeze(t *testing.T) {
	response := gameplayLiveResponse{Available: true, GameID: 161, QueueID: 440}
	for index := 0; index < 8; index++ {
		team := int64(100)
		if index >= 3 {
			team = 200
		}
		response.Players = append(response.Players, gameplayLivePlayer{TeamID: team, HistoryState: "ok"})
	}
	if gameplayLiveSnapshotComplete(response) {
		t.Fatal("3+5 roster was frozen for the entire match")
	}
	response.Players = append(response.Players, gameplayLivePlayer{TeamID: 100, HistoryState: "ok"}, gameplayLivePlayer{TeamID: 100, HistoryState: "ok"})
	if !gameplayLiveSnapshotComplete(response) {
		t.Fatal("5+5 roster should be complete")
	}
}

func TestR161LiveClientRosterIdentityIsParsedAndCloned(t *testing.T) {
	raw := []byte(`[{"riotId":"Hidden Player#CN1","team":"ORDER","position":"JUNGLE","championName":"测试英雄"}]`)
	snapshot, shape, err := parseLiveClientPlayerList(raw)
	if err != nil || shape.PlayerCount != 1 || len(snapshot.RosterPlayers) != 1 {
		t.Fatalf("playerlist parse: shape=%#v err=%v", shape, err)
	}
	player := snapshot.RosterPlayers[0]
	if player.GameName != "Hidden Player" || player.TagLine != "CN1" || player.Team != "ORDER" || player.Position != "jungle" || player.ChampionName != "测试英雄" {
		t.Fatalf("parsed roster player = %#v", player)
	}
	clone := cloneLiveClientSnapshot(snapshot)
	clone.RosterPlayers[0].GameName = "Changed"
	if snapshot.RosterPlayers[0].GameName != "Hidden Player" {
		t.Fatal("cached playerlist was mutated through its clone")
	}
}

func TestR161RecoverMissingOpponentsFromVerifiedPlayerlist(t *testing.T) {
	transport := gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/lol-summoner/v1/alias/lookup" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("missing")), Header: make(http.Header)}, nil
		}
		var index int
		if _, err := fmt.Sscanf(r.URL.Query().Get("gameName"), "Player%d", &index); err != nil || index < 0 || index >= 10 {
			t.Errorf("unexpected alias query: %v", r.URL.Query())
			return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("bad query")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`[{"puuid":%q}]`, r161Ref(index)))), Header: make(http.Header)}, nil
	})
	client := &LCUClient{baseURL: "http://lcu.local", token: "test", http: &http.Client{Transport: transport}, region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	current := Summoner{PUUID: r161Ref(5), GameName: "Player5", TagLine: "CN1"}
	raw := make([]struct {
		player lcuLivePlayer
		team   int64
	}, 0, 10)
	for _, index := range []int{0, 1, 2, 5, 6, 7, 8, 9} {
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
		snapshot.RosterPlayers = append(snapshot.RosterPlayers, liveClientRosterPlayer{GameName: fmt.Sprintf("Player%d", index), TagLine: "CN1", Team: team, Position: "JUNGLE", ChampionName: "测试英雄"})
	}
	a := &app{}
	got, appended, attempts, reason := a.recoverClassicLiveRoster(context.Background(), client, current, raw, snapshot, map[int64]string{11: "测试英雄"}, false)
	if appended != 2 || attempts != 5 || reason != "resolved" || len(got) != 10 {
		t.Fatalf("recovery result: appended=%d attempts=%d reason=%s count=%d", appended, attempts, reason, len(got))
	}
	for _, index := range []int{3, 4} {
		found := false
		for _, entry := range got {
			if entry.player.PUUID == r161Ref(index) {
				found = entry.team == 100 && entry.player.ChampionID == 11 && entry.player.SelectedPosition == "JUNGLE"
			}
		}
		if !found {
			t.Fatalf("missing opponent %d was not recovered with its verified team", index)
		}
	}
	snapshot.RosterPlayers[4].GameName = "Player6"
	if recovered, count, _, reason := a.recoverClassicLiveRoster(context.Background(), client, current, raw, snapshot, nil, false); count != 0 || len(recovered) != 8 || reason != "team-conflict" {
		t.Fatalf("conflicting identity partly changed roster: count=%d roster=%d reason=%s", count, len(recovered), reason)
	}
	snapshot.RosterPlayers[4].GameName = "Player4"
	snapshot.RosterPlayers[5].Team = "ORDER"
	if _, count, attempts, reason := a.recoverClassicLiveRoster(context.Background(), client, current, raw, snapshot, nil, false); count != 0 || attempts != 0 || reason != "teams-unverified" {
		t.Fatalf("ambiguous team mapping accepted: count=%d attempts=%d reason=%s", count, attempts, reason)
	}
}
