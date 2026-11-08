package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func arenaSelfFameTestEvent(group string, fame int, thresholds []int, uri string) LCUEvent {
	data, _ := json.Marshal(map[string]any{
		"groupId":  group,
		"counters": []map[string]any{{"counterValue": fame}},
		"milestones": func() []map[string]any {
			rows := make([]map[string]any, 0, len(thresholds))
			for _, value := range thresholds {
				rows = append(rows, map[string]any{"triggerValue": value})
			}
			return rows
		}(),
	})
	if uri == "" {
		uri = "/lol-progression/v1/groups/" + group
	}
	return LCUEvent{URI: uri, EventType: "Update", Data: data}
}

func arenaSelfFameTestApp(t *testing.T) (*app, *LCUClient) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	a.summoner = Summoner{PUUID: "self-puuid", SummonerID: 42}
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	return a, client
}

var arenaSelfFameTestThresholds = []int{100, 1600, 3600, 6100, 9100, 12600, 16600, 21600, 26600, 31600, 37600, 47600}

func TestR318ArenaSelfFameEventParsing(t *testing.T) {
	group, fame, thresholds, ok := arenaSelfFameEventValue(arenaSelfFameTestEvent(arenaSelfFameGroup, 97312, arenaSelfFameTestThresholds, ""))
	if !ok || group != arenaSelfFameGroup || fame != 97312 || len(thresholds) != 12 {
		t.Fatalf("parse self fame = %q %d %d %v", group, fame, len(thresholds), ok)
	}
	if level := arenaSelfFameLevel(fame, thresholds); level != 12 {
		t.Fatalf("level(97312)=%d, want 12", level)
	}
	// 错误的 group 不参与本人名望。
	if _, _, _, ok := arenaSelfFameEventValue(arenaSelfFameTestEvent("11111111-2222-3333-4444-555555555555", 10, []int{1}, "")); !ok {
		t.Fatal("generic event should still parse for group check")
	}
	if group, _, _, ok := arenaSelfFameEventValue(arenaSelfFameTestEvent("11111111-2222-3333-4444-555555555555", 10, []int{1}, "")); !ok || group == arenaSelfFameGroup {
		t.Fatal("wrong group must not match the self group")
	}
	// 缺字段：没有 counters / milestones。
	missing, _ := json.Marshal(map[string]any{"groupId": arenaSelfFameGroup})
	if _, _, _, ok := arenaSelfFameEventValue(LCUEvent{Data: missing}); ok {
		t.Fatal("missing counters should be rejected")
	}
	emptyMilestones, _ := json.Marshal(map[string]any{"groupId": arenaSelfFameGroup, "counters": []map[string]any{{"counterValue": 5}}})
	if _, _, _, ok := arenaSelfFameEventValue(LCUEvent{Data: emptyMilestones}); ok {
		t.Fatal("missing milestones should be rejected")
	}
	// 重复门槛也应拒绝。
	if _, _, _, ok := arenaSelfFameEventValue(arenaSelfFameTestEvent(arenaSelfFameGroup, 5, []int{100, 100}, "")); ok {
		t.Fatal("duplicate thresholds should be rejected")
	}
}

func TestR318ArenaSelfFameRejectsForeignPlayerEvent(t *testing.T) {
	a, client := arenaSelfFameTestApp(t)
	data, _ := json.Marshal(map[string]any{"groupId": arenaSelfFameGroup, "puuid": "someone-else", "counters": []map[string]any{{"counterValue": 97312}}, "milestones": []map[string]any{{"triggerValue": 100}}})
	a.observeArenaSelfFameEvent(client, LCUEvent{URI: "/lol-progression/v1/groups/" + arenaSelfFameGroup, Data: data})
	if got := a.ownArenaSelfFame(client); got != nil {
		t.Fatalf("foreign player event wrote self fame: %+v", got)
	}
}

func TestR318ArenaSelfFamePersistsAndReadsOwnOnly(t *testing.T) {
	a, client := arenaSelfFameTestApp(t)
	event := arenaSelfFameTestEvent(arenaSelfFameGroup, 97312, arenaSelfFameTestThresholds, "")
	a.observeArenaSelfFameEvent(client, event)
	fame := a.ownArenaSelfFame(client)
	if fame == nil || fame.Fame != 97312 || fame.Level != 12 {
		t.Fatalf("own fame = %+v", fame)
	}
	// 错误 group 不落盘、不改变已有值。
	a.observeArenaSelfFameEvent(client, arenaSelfFameTestEvent("11111111-2222-3333-4444-555555555555", 1, []int{1}, ""))
	if again := a.ownArenaSelfFame(client); again == nil || again.Fame != 97312 {
		t.Fatalf("foreign group changed own fame: %+v", again)
	}
	// 重新读取（模拟重启）仍能拿到。
	records := readArenaSelfFameRecords(a.storage)
	if len(records) != 1 {
		t.Fatalf("records = %d", len(records))
	}
	// 不同账号 / 平台互不影响。
	other := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	a.summoner = Summoner{PUUID: "other-puuid", SummonerID: 43}
	if got := a.ownArenaSelfFame(other); got != nil {
		t.Fatalf("other account should not inherit fame: %+v", got)
	}
}

func TestR318ArenaSelfFameRetiredCleanupKeepsNewFile(t *testing.T) {
	dir := t.TempDir()
	store := &localStore{root: dir}
	raw := []byte(`{"k":{"account":"a","platform":"p","fame":97312,"level":12,"observedAt":"2026-11-01T00:00:00Z"}}`)
	if err := os.WriteFile(filepath.Join(dir, arenaSelfFameFile), raw, 0600); err != nil {
		t.Fatal(err)
	}
	removeRetiredArenaCache(store)
	if _, err := os.Stat(filepath.Join(dir, arenaSelfFameFile)); err != nil {
		t.Fatal("retired cleanup removed the new self fame file:", err)
	}
}

func TestR318ArenaSelfFameAttachesToCurrentPlayerOnly(t *testing.T) {
	a, client := arenaSelfFameTestApp(t)
	a.observeArenaSelfFameEvent(client, arenaSelfFameTestEvent(arenaSelfFameGroup, 97312, arenaSelfFameTestThresholds, ""))
	response := gameplayLiveResponse{Players: []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{IsCurrent: true}}, {gameplayPlayer: gameplayPlayer{IsCurrent: false}}}}
	a.attachArenaSelfFame(client, &response)
	if response.Players[0].ArenaFame == nil || response.Players[0].ArenaFame.Level != 12 {
		t.Fatalf("self row = %+v", response.Players[0].ArenaFame)
	}
	if response.Players[1].ArenaFame != nil {
		t.Fatal("other players must not carry fame")
	}
}

func TestR318ArenaSelfFameDiagnosticHasNoAccount(t *testing.T) {
	a, client := arenaSelfFameTestApp(t)
	a.observeArenaSelfFameEvent(client, arenaSelfFameTestEvent(arenaSelfFameGroup, 97312, arenaSelfFameTestThresholds, ""))
	events := readDiagnosticEvents(t, a.storage, "arena_self_fame_observed")
	if len(events) != 1 {
		t.Fatalf("diagnostic events = %d", len(events))
	}
	event := events[0]
	if event["fame"] != float64(97312) || event["level"] != float64(12) {
		t.Fatalf("diagnostic = %+v", event)
	}
	for _, key := range []string{"account", "puuid", "platform"} {
		if _, exists := event[key]; exists {
			t.Fatalf("diagnostic leaked %s: %+v", key, event)
		}
	}
}

func TestR318AugmentUnifiedCatalogMergesFieldsAndCandidates(t *testing.T) {
	provider := &championProvider{}
	lcu := []gameplayAugment{{ID: 1019, Name: "本机名称", Rarity: "kSilver", IconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/1019_large.png"}}
	provider.remoteAugments = []gameplayAugment{{ID: 1019, Name: "CDragon 名称", Description: "CDragon 描述", IconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/1019_small.png"}}
	provider.onlineAugments = []gameplayAugment{{ID: 3176, Name: "新海克斯", Rarity: "gold", IconPath: "opgg:/meta/images/lol/latest/aram-augment/New_large.png"}}
	rows := provider.unifiedAugmentCatalog(lcu)
	byID := map[int64]gameplayAugment{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	row := byID[1019]
	if row.Name != "本机名称" {
		t.Fatalf("name priority = %q", row.Name)
	}
	if row.Description != "CDragon 描述" {
		t.Fatalf("description priority = %q", row.Description)
	}
	if row.Rarity != "silver" {
		t.Fatalf("rarity = %q", row.Rarity)
	}
	if len(row.IconCandidates) < 3 || row.IconCandidates[0].Source != "lcu" || row.IconCandidates[1].Source != "communitydragon" {
		t.Fatalf("candidates = %+v", row.IconCandidates)
	}
	if row.IconCandidates[0].URL != "/api/image?path=%2Flol-game-data%2Fassets%2FASSETS%2FUX%2FKiwi%2FAugments%2FIcons%2F1019_large.png" {
		t.Fatalf("lcu candidate URL = %q", row.IconCandidates[0].URL)
	}
	if _, ok := byID[1019]; !ok || byID[1019].Unknown {
		t.Fatal("known row marked unknown")
	}
	newRow := byID[3176]
	if newRow.Name != "新海克斯" || len(newRow.IconCandidates) == 0 || newRow.IconCandidates[0].Source != "opgg" {
		t.Fatalf("online row = %+v", newRow)
	}
	requested := filterAugmentIDs(rows, []int64{1019, 999999})
	if len(requested) != 2 || !requested[1].Unknown || requested[1].ID != 999999 {
		t.Fatalf("unknown filter = %+v", requested)
	}
}

func TestR318AugmentIconURLCandidates(t *testing.T) {
	cases := map[string]string{
		"builtin:/augments/1019.png":                      "/augments/1019.png",
		"lcu|/lol-game-data/assets/ASSETS/UX/Kiwi/x.png":  "/api/image?path=%2Flol-game-data%2Fassets%2FASSETS%2FUX%2FKiwi%2Fx.png",
		"communitydragon|/latest/game/assets/x_large.png": "/api/champion-asset?source=communitydragon&path=%2Flatest%2Fgame%2Fassets%2Fx_large.png",
		"opgg|/meta/images/lol/latest/aram-augment/x.png": "/api/champion-asset?source=opgg&path=%2Fmeta%2Fimages%2Flol%2Flatest%2Faram-augment%2Fx.png",
	}
	for input, want := range cases {
		source, path := "", input
		if parts := strings.SplitN(input, "|", 2); len(parts) == 2 {
			source, path = parts[0], parts[1]
		}
		if got := augmentIconURL(source, path); got != want {
			t.Fatalf("augmentIconURL(%q,%q)=%q want %q", source, path, got, want)
		}
	}
	if got := augmentIconURL("opgg", "https://opgg-static.akamaized.net/meta/images/lol/latest/aram-augment/x.png"); got == "" {
		t.Fatal("absolute OP.GG URL should be accepted")
	}
}
