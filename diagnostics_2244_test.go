package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func Test2244CustomLifecycleSuppressesAllRulesAndCancelsJobs(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			writes.Add(1)
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "fixture-token", http: server.Client()}
	runner := newWatchRunner(nil, nil)
	settings := defaultWatchSettings()
	settings.Rules.AutoAccept.Enabled = true
	settings.Rules.AutoReconnect.Enabled = true
	settings.Rules.AutoPlayAgain.Enabled = true
	settings.Rules.AutoHonor.Enabled = true
	settings.Rules.PromoteLeader.Enabled = true
	settings.Rules.AutoMatchmaking.Enabled = true
	settings.Rules.Invitations.Enabled = true
	settings.Rules.SkipCelebration.Enabled = true
	settings.Rules.PositionBroadcast.Enabled = true
	runner.apply(settings)
	runner.scheduleMarked(client, "reconnect", 50, "POST", "/must-not-fire", nil)
	runner.handleEvent(client, LCUEvent{URI: "/lol-lobby/v2/lobby", Data: json.RawMessage(`{"gameConfig":{"isCustom":true,"queueId":3100}}`)}, Summoner{})
	if !runner.customPaused() {
		t.Fatal("queue3100 custom lobby not paused")
	}
	for _, phase := range []string{"ReadyCheck", "ChampSelect", "InProgress", "Reconnect", "WaitingForStats", "PreEndOfGame", "EndOfGame"} {
		runner.handlePhase(client, phase)
	}
	for _, uri := range []string{"/lol-honor-v2/v1/ballot", "/lol-pre-end-of-game/v1/currentSequenceEvent", "/lol-lobby/v2/received-invitations"} {
		runner.handleEvent(client, LCUEvent{URI: uri, Data: json.RawMessage(`{"name":"missions-celebration"}`)}, Summoner{})
	}
	runner.handleChampSelect(client, Summoner{})
	runner.observeCustomSession(map[string]any{"gameData": map[string]any{}}, false)
	if !runner.customPaused() {
		t.Fatal("teardown lost custom state")
	}
	if runner.scheduleMarked(client, "play-again", 0, "POST", "/must-not-fire", nil) {
		t.Fatal("scheduled while custom")
	}
	time.Sleep(100 * time.Millisecond)
	if writes.Load() != 0 {
		t.Fatalf("custom writes=%d", writes.Load())
	}
	if !runner.currentWatch().Rules.AutoAccept.Enabled {
		t.Fatal("user settings changed")
	}
	runner.observeCustomSession(map[string]any{"gameConfig": map[string]any{"isCustom": false}}, true)
	if runner.customPaused() {
		t.Fatal("normal lobby did not resume")
	}
	runner.setCustomSession(true)
	runner.handlePhase(client, "None")
	if runner.customPaused() {
		t.Fatal("exit did not resume")
	}
}

func Test2244OfficialOpponentTeamSpacing(t *testing.T) {
	var event proRuneEvent
	json.Unmarshal([]byte(`{"match":{"teams":[{"id":"al-id","code":"AL"},{"id":"lgd-id","code":"LGD"}]}}`), &event)
	for _, c := range []struct{ id, name, want string }{{"al-id", "ALShanks", "AL Shanks"}, {"lgd-id", "LGDTangyuan", "LGD Tangyuan"}, {"al-id", "AL Shanks", "AL Shanks"}, {"unknown", "ALShanks", "ALShanks"}} {
		row := proRuneRow{OpponentTeamID: c.id, Opponent: proParticipant{SummonerName: c.name}}
		if got := proOpponentDisplayName(row, event); got != c.want {
			t.Fatalf("%s => %s", c.name, got)
		}
	}
}

func Test2244EventNameAndRewardGroupIdentityNotAmounts(t *testing.T) {
	var detail any
	json.Unmarshal([]byte(`[{"state":"Unlocked","rewardOptions":[{"state":"Unselected","rewardGroupId":"group-a","rewardName":"750蓝色精萃","thumbIconPath":"/lol-game-data/assets/a.png"}]}]`), &detail)
	items := collectUnselectedEventItems(detail)
	if len(items) != 1 || items[0].ID != "group-a" || items[0].Quantity != 0 {
		t.Fatalf("event reward option lost or invented quantity: %+v", items)
	}
	if got := claimDisplayTitle(map[string]any{"eventName": "测试活动"}, "fallback"); got != "测试活动" {
		t.Fatal(got)
	}
	entries := []claimEntry{
		{Source: "event", ID: "one", EventID: "one", EventName: "测试活动", RewardGroupIDs: eventRewardGroupIDs(detail), Items: items, Actionable: true},
		{Source: "grant", RewardGroupID: "group-a", Items: items, Actionable: true},
		{Source: "grant", RewardGroupID: "unrelated", Items: items, Actionable: true},
	}
	markClaimOverlaps(entries)
	if entries[1].EventID != "one" || entries[1].Title != "测试活动" || entries[2].EventID != "" || entries[0].Actionable {
		t.Fatalf("bad ownership: %+v", entries)
	}
	// An ambiguous reused group cannot establish ownership.
	entries = append(entries, claimEntry{Source: "event", EventID: "two", RewardGroupIDs: []string{"group-a"}})
	entries[1].EventID = ""
	markClaimOverlaps(entries)
	if entries[1].EventID != "" {
		t.Fatal("ambiguous event guessed")
	}
}

func Test2244AuthorizedExactOPGGArchiveOnce(t *testing.T) {
	loc := migrationFixture(t)
	raw, err := os.ReadFile("testdata/diagnostics-1945/akari-user-verified.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		UID string `json:"uid"`
	}
	json.Unmarshal(raw, &fixture)
	if !authorizedOPGGItemSet(fixture.UID) {
		t.Fatal("not user's file")
	}
	target := migrationFile(t, loc, "Global/Recommended/akari.json", string(raw))
	foreign := migrationFile(t, loc, "Global/Recommended/new-patch.json", strings.ReplaceAll(string(raw), "16.17", "16.18"))
	if !authorizedOPGGRemovalPending(loc) {
		t.Fatal("not pending")
	}
	n, err := archiveRecommendedFilesMatching(context.Background(), loc, authorizedOPGGItemSet, nil)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err = os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("target remains")
	}
	if _, err = os.Stat(foreign); err != nil {
		t.Fatal("foreign removed")
	}
	if err = markAuthorizedOPGGRemoval(loc, nil); err != nil {
		t.Fatal(err)
	}
	if authorizedOPGGRemovalPending(loc) {
		t.Fatal("migration repeats after completion")
	}
	backup, _ := filepath.Glob(filepath.Join(loc.configRoot, "DeepLegendsItemSetBackups", "v1-*", "*.json"))
	if len(backup) != 1 {
		t.Fatal("backup missing")
	}
}

func Test2244OverviewAnimationUsesExactSkinAndCenteredVariant(t *testing.T) {
	skins := []Skin{{ID: 103085, CollectionVideoPath: "/lol-game-data/assets/wrong.webm"}, {ID: 103086, SplashPath: "/lol-game-data/assets/skin86/ahri_centered_86.jpg", SplashVideoPath: "/lol-game-data/assets/skin86/centered.webm", CollectionVideoPath: "/lol-game-data/assets/skin86/uncentered.webm"}}
	poster, video := overviewSkinMedia(skins, 103086)
	if !strings.Contains(poster, "skin86") || video != "/lol-game-data/assets/skin86/centered.webm" {
		t.Fatal(poster, video)
	}
	if _, video = overviewSkinMedia(skins, 103087); video != "" {
		t.Fatal("borrowed different variant")
	}
}

func Test2244CustomTransitionCancelsAdmittedHTTPWrites(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(started)
		select {
		case <-req.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	client := &LCUClient{baseURL: server.URL, token: "fixture-token", http: server.Client()}
	runner := newWatchRunner(nil, nil)
	done := make(chan error, 1)
	go func() { done <- runner.requestWatchJSON(context.Background(), client, "POST", "/in-flight", nil) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("write not started")
	}
	runner.setCustomSession(true)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("active write was not canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("write remained in flight")
	}
	if err := runner.requestWatchJSON(context.Background(), client, "POST", "/after-custom", nil); err == nil {
		t.Fatal("new write admitted")
	}
}

func Test2244ScanOfficialEventOptionsAndDeduplicateEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Path == "/lol-event-hub/v1/events" {
			w.Write([]byte(`[{"eventId":"event-2244","eventInfo":{"eventName":"活动实际名称","unclaimedRewardCount":1}}]`))
			return
		}
		w.Write([]byte(`[{"state":"Unlocked","rewardOptions":[{"state":"Unselected","rewardGroupId":"reward-2244","rewardName":"750蓝色精萃"}]}]`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "fixture-token", http: server.Client()}
	entries, state := scanEventClaims(context.Background(), client)
	if len(entries) != 1 || state.Count != 1 || entries[0].Title != "活动实际名称" || len(entries[0].Items) != 1 {
		t.Fatalf("entries=%+v state=%+v", entries, state)
	}
}
