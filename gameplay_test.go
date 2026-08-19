package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type gameplayRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn gameplayRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestGameplayReferenceDoesNotExposeStablePlayerID(t *testing.T) {
	a := &app{token: "session-secret", gameplayRefs: make(map[string]string)}
	raw := strings.Repeat("p", 48)
	public := a.registerGameplayReference(raw)
	if public == "" || public == raw || strings.Contains(public, raw) {
		t.Fatalf("public reference leaked stable id: %q", public)
	}
	if resolved, ok := a.resolveGameplayReference(public); !ok || resolved != raw {
		t.Fatalf("reference did not resolve: %q, %v", resolved, ok)
	}
	a.mu.Lock()
	a.gameplayRefs = make(map[string]string)
	a.mu.Unlock()
	if _, ok := a.resolveGameplayReference(public); ok {
		t.Fatal("reference survived session reset")
	}
	if !gameplaySummonerChanged(Summoner{}, Summoner{PUUID: raw}) || gameplaySummonerChanged(Summoner{PUUID: raw}, Summoner{PUUID: raw}) {
		t.Fatal("account transition did not invalidate aliases exactly once")
	}
}

func TestGameplayOverviewRejectsUnregisteredStablePlayerID(t *testing.T) {
	a := &app{token: "session-secret", gameplayRefs: make(map[string]string)}
	request := httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"playerRef":"`+strings.Repeat("p", 48)+`","count":20}`))
	recorder := httptest.NewRecorder()
	a.handleGameplayOverview(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestGameplayHistoryUsesRequestedPageWindow(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"games":{"gameCount":87,"games":[]}}`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	_, _, total := loadGameplayHistory(client, strings.Repeat("p", 48), false, 30, 50, true)
	if gotQuery != "begIndex=30&endIndex=79" {
		t.Fatalf("query = %q, want paged window", gotQuery)
	}
	if total != 87 {
		t.Fatalf("total = %d, want 87", total)
	}
	if clampMatchCount(50) != 50 || clampMatchCount(51) != 50 || clampMatchStart(-1) != 0 {
		t.Fatal("gameplay page limits changed")
	}
}

func TestHiddenPlayerUsesTheSameRankAndMatchHistoryLoaders(t *testing.T) {
	hiddenPUUID := strings.Repeat("h", 48)
	currentPUUID := strings.Repeat("c", 48)

	var game lcuGame
	game.GameID = 701
	game.GameCreation = 1_786_300_000_000
	game.GameDuration = 1_800
	game.QueueID = 420
	game.GameMode = "CLASSIC"
	identity := lcuParticipantIdentity{ParticipantID: 1}
	identity.Player.PUUID = hiddenPUUID
	participant := lcuParticipant{ParticipantID: 1, TeamID: 100, ChampionID: 64}
	participant.Stats.Kills = 8
	participant.Stats.Deaths = 2
	participant.Stats.Assists = 6
	participant.Stats.TotalMinionsKilled = 180
	participant.Stats.Win = true
	game.ParticipantIdentities = []lcuParticipantIdentity{identity}
	game.Participants = []lcuParticipant{participant}
	game.Teams = []lcuTeam{{TeamID: 100, Win: "Win"}}
	var history lcuMatchHistory
	history.Games.GameCount = 1
	history.Games.Games = []lcuGame{game}

	requested := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested[r.URL.Path]++
		switch {
		case r.URL.Path == "/lol-summoner/v2/summoners/puuid/"+hiddenPUUID:
			http.Error(w, "profile hidden", http.StatusNotFound)
		case r.URL.Path == "/lol-game-queues/v1/queues":
			_, _ = w.Write([]byte(`[{"id":420,"shortName":"单排/双排"}]`))
		case r.URL.Path == "/lol-ranked/v1/ranked-stats/"+hiddenPUUID:
			_, _ = w.Write([]byte(`{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"I","leaguePoints":55,"wins":12,"losses":8}]}`))
		case r.URL.Path == "/lol-match-history/v1/products/lol/"+hiddenPUUID+"/matches":
			_ = json.NewEncoder(w).Encode(history)
		case r.URL.Path == "/lol-champion-mastery/v1/"+hiddenPUUID+"/champion-mastery":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{token: "session-secret", connected: true, lcu: client, summoner: Summoner{PUUID: currentPUUID}, gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference)}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: hiddenPUUID, DisplayName: "隐藏玩家"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"playerRef":"`+publicRef+`","count":20}`))
	a.handleGameplayOverview(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	responseBody := recorder.Body.String()
	if strings.Contains(responseBody, hiddenPUUID) {
		t.Fatal("renderer response leaked the hidden player's stable PUUID")
	}
	var response gameplayOverview
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Player.Hidden || response.Player.DisplayName != "隐藏玩家" {
		t.Fatalf("hidden profile = %#v", response.Player)
	}
	if response.Overall.Games != 1 || response.Overall.Wins != 1 || len(response.Matches) != 1 {
		t.Fatalf("hidden player match data was suppressed: overall=%#v matches=%d", response.Overall, len(response.Matches))
	}
	if len(response.Ranks) != 1 || response.Ranks[0].Wins != 12 || response.Ranks[0].Losses != 8 {
		t.Fatalf("hidden player rank data was suppressed: %#v", response.Ranks)
	}
	if len(response.Matches[0].Participants) != 1 || !response.Matches[0].Participants[0].Hidden || response.Matches[0].Participants[0].PlayerRef == "" {
		t.Fatalf("hidden participant was not kept queryable: %#v", response.Matches[0].Participants)
	}
	for _, path := range []string{
		"/lol-ranked/v1/ranked-stats/" + hiddenPUUID,
		"/lol-match-history/v1/products/lol/" + hiddenPUUID + "/matches",
		"/lol-champion-mastery/v1/" + hiddenPUUID + "/champion-mastery",
	} {
		if requested[path] == 0 {
			t.Fatalf("hidden player did not use normal loader %s", path)
		}
	}
}

func TestChampSelectKeepsObfuscatedIdentityQueryable(t *testing.T) {
	obfuscated := "00000000-0000-0000-0000-000000000000"
	deobfuscated := "817076a9-f451-509b-9598-6813ce9117e7"
	merged := mergeChampSelectPlayers(nil, lcuChampSelectSession{TheirTeam: []lcuChampSelectPlayer{{
		ChampionID: 64, ObfuscatedPUUID: obfuscated, ObfuscatedSummonerID: 88,
		NameVisibilityType: "HIDDEN",
	}}})
	if len(merged) != 1 {
		t.Fatalf("merged players = %d", len(merged))
	}
	player := merged[0].player
	if player.PUUID != deobfuscated || player.ObfuscatedPUUID != obfuscated || player.NameVisibilityType != "HIDDEN" {
		t.Fatalf("obfuscated identity was not recovered internally: %#v", player)
	}
	reference := normalizeGameplayReference(gameplayReference{PlayerRef: player.PUUID, AlternatePlayerRef: player.ObfuscatedPUUID, AlternateSummonerID: player.ObfuscatedSummonerID})
	if reference.PlayerRef != deobfuscated || reference.AlternatePlayerRef != obfuscated {
		t.Fatalf("obfuscated player cannot receive a session alias: %#v", reference)
	}
	if deobfuscateHiddenPlayerReference("550e8400-e29b-41d4-a716-446655440000") != "d47ef2a9-16ca-114f-328e-2c759bd517e7" {
		t.Fatal("hidden player UUID compatibility vector changed")
	}
	if deobfuscateHiddenPlayerReference("not-a-uuid") != "" {
		t.Fatal("invalid hidden player reference was accepted")
	}
}

func TestHiddenPlayerFallsBackToSummonerIDBeforeLoadingStats(t *testing.T) {
	hiddenRef := strings.Repeat("x", 48)
	realPUUID := strings.Repeat("r", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-summoner/v2/summoners/puuid/" + hiddenRef:
			http.Error(w, "hidden", http.StatusNotFound)
		case "/lol-summoner/v1/summoners/77":
			_, _ = w.Write([]byte(`{"summonerId":77,"puuid":"` + realPUUID + `","profileIconId":12,"summonerLevel":99}`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	summoner, capability := loadGameplaySummoner(client, gameplayReference{PlayerRef: hiddenRef, SummonerID: 77})
	if capability.State != capabilityAvailable || summoner.PUUID != realPUUID || summoner.SummonerID != 77 {
		t.Fatalf("summoner id fallback failed: summoner=%#v capability=%#v", summoner, capability)
	}
}

func TestChampionAbilitiesUseClientDescriptionsAndSafeAssetPaths(t *testing.T) {
	httpClient := &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/lol-game-data/assets/v1/champions/103.json" {
			t.Fatalf("unexpected endpoint: %s", request.URL.Path)
		}
		body := `{"spells":[{"name":"欺诈宝珠","description":"放出并收回宝珠。","abilityIconPath":"/lol-game-data/assets/ASSETS/Characters/Ahri/HUD/Icons2D/AhriQ.png","costCoefficients":[55,65,75,85,95],"cooldownCoefficients":[7,7,7,7,7],"range":[970,970,970,970,970]},{"name":"妖异狐火","tooltip":"释放三团狐火。","imagePath":"/lol-game-data/assets/ASSETS/Characters/Ahri/HUD/Icons2D/AhriW.png"},{"name":"魅惑妖术","description":"魅惑第一个敌人。","iconPath":"https://example.com/unsafe.png"},{"name":"灵魄突袭","description":"向前突进。","iconPath":"/lol-game-data/assets/ASSETS/Characters/Ahri/HUD/Icons2D/AhriR.png"}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "test-token", http: httpClient}
	abilities, err := loadChampionAbilities(client, 103)
	if err != nil || len(abilities) != 4 {
		t.Fatalf("abilities = %#v, err=%v", abilities, err)
	}
	if abilities[0].Slot != "Q" || abilities[0].Description != "放出并收回宝珠。" || abilities[0].IconPath == "" || len(abilities[0].Costs) != 5 || len(abilities[0].Cooldowns) != 5 || len(abilities[0].Ranges) != 5 {
		t.Fatalf("Q ability was not normalized: %#v", abilities[0])
	}
	if abilities[1].Slot != "W" || abilities[1].Description != "释放三团狐火。" || abilities[1].IconPath == "" {
		t.Fatalf("W tooltip fallback failed: %#v", abilities[1])
	}
	if abilities[2].IconPath != "" {
		t.Fatalf("unsafe ability asset path was accepted: %#v", abilities[2])
	}
}

func TestGameplaySummonerSpellsExposeDescriptionsWithoutExternalAssetPaths(t *testing.T) {
	httpClient := &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/lol-game-data/assets/v1/summoner-spells.json" {
			t.Fatalf("unexpected endpoint: %s", request.URL.Path)
		}
		body := `[{"id":4,"name":"闪现","description":"瞬间传送一小段距离。","iconPath":"/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_flash.png"},{"id":14,"name":"引燃","description":"造成持续真实伤害。","imagePath":"https://example.com/unsafe.png"}]`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "test-token", http: httpClient}
	a := &app{connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleGameplaySummonerSpells(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/summoner-spells", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Spells []gameplaySummonerSpell `json:"spells"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || len(response.Spells) != 2 {
		t.Fatalf("summoner spells response = %#v, err=%v", response, err)
	}
	if response.Spells[0].Name != "闪现" || response.Spells[0].Description == "" || response.Spells[0].IconPath == "" {
		t.Fatalf("first summoner spell was not normalized: %#v", response.Spells[0])
	}
	if response.Spells[1].IconPath != "" {
		t.Fatalf("unsafe summoner spell asset path was accepted: %#v", response.Spells[1])
	}
}

func TestGameplayItemsExposeDescriptionsWithoutExternalAssetPaths(t *testing.T) {
	httpClient := &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/lol-game-data/assets/v1/items.json" {
			t.Fatalf("unexpected endpoint: %s", request.URL.Path)
		}
		body := `[{"id":3020,"name":"法师之靴","description":"<mainText>提供移动速度与法术穿透。</mainText>","iconPath":"/lol-game-data/assets/ASSETS/Items/Icons2D/3020.png"},{"id":3158,"displayName":"明朗之靴","shortDescription":"提供技能急速。","imagePath":"https://example.com/unsafe.png"}]`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "test-token", http: httpClient}
	a := &app{connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleGameplayItems(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/items", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []gameplayItem `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || len(response.Items) != 2 {
		t.Fatalf("items response = %#v, err=%v", response, err)
	}
	if response.Items[0].Name != "法师之靴" || response.Items[0].Description == "" || response.Items[0].IconPath == "" {
		t.Fatalf("first item was not normalized: %#v", response.Items[0])
	}
	if response.Items[1].Name != "明朗之靴" || response.Items[1].Description != "提供技能急速。" || response.Items[1].IconPath != "" {
		t.Fatalf("fallback or asset validation failed: %#v", response.Items[1])
	}
}

func TestNormalizeGameplayAugmentsUsesChineseNamesAndSafeAssets(t *testing.T) {
	raw := []gameplayAugmentRaw{
		{ID: 2087, Name: "Eureka", NameTRA: " 大法师 ", Description: "<mainText>根据法力值<br>获得法术强度。</mainText>", Rarity: " kPrismatic ", AugmentSmallIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Eureka_small.png"},
		{ID: 1205, Name: "ADAPt", SimpleNameTRA: "物理转魔法", Tooltip: "<b>转化额外攻击力</b>", Rarity: "kSilver", IconPath: "https://example.com/unsafe.png"},
		{ID: 2087, NameTRA: "重复项", AugmentSmallIconPath: "/lol-game-data/assets/duplicate.png"},
		{ID: 0, NameTRA: "无效项"},
		{ID: 9999},
	}

	augments := normalizeGameplayAugments(raw)
	if len(augments) != 2 {
		t.Fatalf("augments = %#v", augments)
	}
	if augments[0].ID != 1205 || augments[0].Name != "物理转魔法" || augments[0].Description != "转化额外攻击力" || augments[0].IconPath != "" {
		t.Fatalf("fallback fields or unsafe asset handling changed: %#v", augments[0])
	}
	if augments[1].ID != 2087 || augments[1].Name != "大法师" || augments[1].Description != "根据法力值\n获得法术强度。" || augments[1].Rarity != "kPrismatic" || augments[1].IconPath == "" {
		t.Fatalf("Chinese augment normalization changed: %#v", augments[1])
	}
}

func TestRequestJSONAuthenticatesAndRejectsUnsafePaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("riot:test-token"))
		if r.Method != http.MethodPut || r.URL.Path != "/lol-perks/v1/pages/7" || r.Header.Get("Authorization") != wantAuth {
			t.Fatalf("unexpected request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["primaryStyleId"] != float64(8000) {
			t.Fatalf("unexpected JSON body: %#v, %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	var response struct {
		OK bool `json:"ok"`
	}
	if err := client.RequestJSON(context.Background(), http.MethodPut, "/lol-perks/v1/pages/7", map[string]any{"primaryStyleId": 8000}, &response); err != nil || !response.OK {
		t.Fatalf("RequestJSON failed: response=%#v err=%v", response, err)
	}
	for _, path := range []string{"relative", "//remote/path", "/lol-perks/../summoner", "/%2e%2e/private", `/lol-perks\v1\pages`} {
		if err := client.RequestJSON(context.Background(), http.MethodGet, path, nil, nil); err == nil {
			t.Fatalf("unsafe path accepted: %q", path)
		}
	}
	if err := client.RequestJSON(context.Background(), http.MethodConnect, "/lol-perks/v1/pages", nil, nil); err == nil {
		t.Fatal("unsupported method was accepted")
	}
}

func TestApplyRunePageCreatesNewPageAndMakesItCurrent(t *testing.T) {
	requests := make([]string, 0, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":true}`))
		case "POST /lol-perks/v1/pages/":
			var body struct {
				Name           string `json:"name"`
				IsEditable     bool   `json:"isEditable"`
				PrimaryStyleID string `json:"primaryStyleId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "[DL] 李青 · OPGG" || !body.IsEditable || body.PrimaryStyleID != "8000" {
				t.Fatalf("unexpected new rune page body: %#v, %v", body, err)
			}
			_, _ = w.Write([]byte(`{"id":7}`))
		case "PUT /lol-perks/v1/pages/7":
			var body struct {
				ID              int64   `json:"id"`
				Name            string  `json:"name"`
				SelectedPerkIDs []int64 `json:"selectedPerkIds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID != 7 || body.Name != "[DL] 李青 · OPGG" || len(body.SelectedPerkIDs) != 9 {
				t.Fatalf("unexpected rune page body: %#v, %v", body, err)
			}
			w.WriteHeader(http.StatusNoContent)
		case "PUT /lol-perks/v1/currentpage":
			var pageID int64
			if err := json.NewDecoder(r.Body).Decode(&pageID); err != nil || pageID != 7 {
				t.Fatalf("current page body = %d, %v", pageID, err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	request := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{8001, 8002, 8003, 8004, 8101, 8102, 5001, 5002, 5003}}
	pageID, err := applyRunePage(context.Background(), client, request)
	if err != nil || pageID != 7 {
		t.Fatalf("applyRunePage = %d, %v", pageID, err)
	}
	want := []string{"GET /lol-perks/v1/inventory", "POST /lol-perks/v1/pages/", "PUT /lol-perks/v1/pages/7", "PUT /lol-perks/v1/currentpage"}
	if strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatalf("request order = %#v, want %#v", requests, want)
	}
}

func TestApplyRunePageFullWithOnlyUserPagesNeverDeletes(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":11,"name":"我的符文页","isDeletable":true,"lastModified":100}]`))
		default:
			t.Fatalf("user rune page was touched: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	request := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{8001, 8002, 8003, 8004, 8101, 8102, 5001, 5002, 5003}}
	pageID, err := applyRunePage(context.Background(), client, request)
	if pageID != 0 || !errors.Is(err, errRunePageLimit) || requests != 2 {
		t.Fatalf("applyRunePage = %d, %v with %d requests", pageID, err, requests)
	}
}

func TestApplyRunePageFullDeletesOnlyOldestOwnedPageThenCreates(t *testing.T) {
	requests := make([]string, 0, 7)
	inventoryReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestKey := r.Method + " " + r.URL.Path
		requests = append(requests, requestKey)
		switch requestKey {
		case "GET /lol-perks/v1/inventory":
			inventoryReads++
			_, _ = fmt.Fprintf(w, `{"canAddCustomPage":%t}`, inventoryReads > 1)
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":9,"name":"用户页","isDeletable":true,"lastModified":1},{"id":22,"name":"[DL] 新页","isDeletable":true,"lastModified":200},{"id":33,"name":"[DL] 最旧页","lastModified":100}]`))
		case "DELETE /lol-perks/v1/pages/33":
			w.WriteHeader(http.StatusNoContent)
		case "POST /lol-perks/v1/pages/":
			_, _ = w.Write([]byte(`{"id":7}`))
		case "PUT /lol-perks/v1/pages/7", "PUT /lol-perks/v1/currentpage":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected rune page request: %s", requestKey)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	request := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{8001, 8002, 8003, 8004, 8101, 8102, 5001, 5002, 5003}}
	pageID, err := applyRunePage(context.Background(), client, request)
	if err != nil || pageID != 7 {
		t.Fatalf("applyRunePage = %d, %v", pageID, err)
	}
	want := []string{"GET /lol-perks/v1/inventory", "GET /lol-perks/v1/pages", "DELETE /lol-perks/v1/pages/33", "GET /lol-perks/v1/inventory", "POST /lol-perks/v1/pages/", "PUT /lol-perks/v1/pages/7", "PUT /lol-perks/v1/currentpage"}
	if strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatalf("request order = %#v, want %#v", requests, want)
	}
}

func TestApplyRunePageSortsMissingLastModifiedByID(t *testing.T) {
	inventoryReads := 0
	deleted := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			inventoryReads++
			_, _ = fmt.Fprintf(w, `{"canAddCustomPage":%t}`, inventoryReads > 1)
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":9,"name":"[DL] 九"},{"id":4,"name":"[DL] 四"}]`))
		case "DELETE /lol-perks/v1/pages/4":
			deleted = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		case "POST /lol-perks/v1/pages/":
			_, _ = w.Write([]byte(`{"id":7}`))
		case "PUT /lol-perks/v1/pages/7", "PUT /lol-perks/v1/currentpage":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected rune page request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	request := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}}
	if pageID, err := applyRunePage(context.Background(), client, request); err != nil || pageID != 7 || deleted != "/lol-perks/v1/pages/4" {
		t.Fatalf("applyRunePage=%d,%v deleted=%q", pageID, err, deleted)
	}
}

func TestApplyRunePagePrefixMatchMustBeExact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":11,"name":"我的[DL]页","isDeletable":true,"lastModified":100}]`))
		default:
			t.Fatalf("contains-only rune page was touched: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	_, err := applyRunePage(context.Background(), client, gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}})
	if !errors.Is(err, errRunePageLimit) {
		t.Fatalf("applyRunePage error = %v, want errRunePageLimit", err)
	}
}

func TestApplyRunePageSkipsExplicitlyNonDeletableOwnedPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":11,"name":"[DL] 李青 · 绝活哥","isDeletable":false,"lastModified":100}]`))
		default:
			t.Fatalf("non-deletable rune page was touched: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	_, err := applyRunePage(context.Background(), client, gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}})
	if !errors.Is(err, errRunePageLimit) {
		t.Fatalf("applyRunePage error = %v, want errRunePageLimit", err)
	}
}

func TestApplyRunePageRecycleLoopStopsAfterFiveDeletes(t *testing.T) {
	inventoryReads := 0
	deleted := make([]string, 0, runePageRecycleLimit)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-perks/v1/inventory":
			inventoryReads++
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case r.Method == http.MethodGet && r.URL.Path == "/lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":1,"name":"[DL] 一","lastModified":1},{"id":2,"name":"[DL] 二","lastModified":2},{"id":3,"name":"[DL] 三","lastModified":3},{"id":4,"name":"[DL] 四","lastModified":4},{"id":5,"name":"[DL] 五","lastModified":5},{"id":6,"name":"[DL] 六","lastModified":6}]`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/lol-perks/v1/pages/"):
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request after recycle limit: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	_, err := applyRunePage(context.Background(), client, gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}})
	if !errors.Is(err, errRunePageLimit) || len(deleted) != runePageRecycleLimit || inventoryReads != runePageRecycleLimit+1 {
		t.Fatalf("applyRunePage error=%v deleted=%#v inventoryReads=%d", err, deleted, inventoryReads)
	}
	if got := strings.Join(deleted, "|"); got != "/lol-perks/v1/pages/1|/lol-perks/v1/pages/2|/lol-perks/v1/pages/3|/lol-perks/v1/pages/4|/lol-perks/v1/pages/5" {
		t.Fatalf("deleted pages = %s", got)
	}
}

func TestRuneApplyValidationAndReplayActionValidation(t *testing.T) {
	valid := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}}
	if err := validateRuneApplyRequest(valid); err != nil {
		t.Fatalf("valid rune request rejected: %v", err)
	}
	invalid := valid
	invalid.ChampionName = strings.Repeat("长", 49)
	if err := validateRuneApplyRequest(invalid); err == nil {
		t.Fatal("rune page name whose prefix pushes it over 60 characters was accepted")
	}
	invalid = valid
	invalid.SelectedPerkIDs = []int64{1, 2, 3, 4, 5, 5}
	if err := validateRuneApplyRequest(invalid); err == nil {
		t.Fatal("duplicate perk ids were accepted")
	}
	valid.SelectedPerkIDs = []int64{8001, 8002, 8003, 8004, 8101, 8102, 5008, 5008, 5001}
	if err := validateRuneApplyRequest(valid); err != nil {
		t.Fatalf("duplicate stat shards were rejected: %v", err)
	}
	a := &app{}
	recorder := httptest.NewRecorder()
	a.handleGameplayReplayAction(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/replay", strings.NewReader(`{"gameId":42,"action":"watch/../../private"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("replay injection status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRuneApplyHandlerNeverWritesOutsideChampionSelect(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/lol-gameflow/v1/gameflow-phase" {
			t.Fatalf("unexpected write outside champion select: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`"Lobby"`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	body := `{"championName":"李青","source":"OPGG","championId":64,"primaryStyleId":8000,"subStyleId":8100,"selectedPerkIds":[1,2,3,4,5,6]}`
	recorder := httptest.NewRecorder()
	a.handleGameplayRuneApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/runes/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusConflict || requests != 1 {
		t.Fatalf("status/requests = %d/%d, want %d/1", recorder.Code, requests, http.StatusConflict)
	}
}

func TestRuneApplyHandlerExplainsClientPageLimit(t *testing.T) {
	requests := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "/lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case "/lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":11,"name":"用户页","isDeletable":true}]`))
		default:
			t.Fatalf("user rune page was modified: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	body := `{"championName":"李青","source":"OPGG","championId":64,"primaryStyleId":8000,"subStyleId":8100,"selectedPerkIds":[1,2,3,4,5,6]}`
	recorder := httptest.NewRecorder()
	a.handleGameplayRuneApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/runes/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), runePageLimitMessage) {
		t.Fatalf("status/body = %d/%q", recorder.Code, recorder.Body.String())
	}
	want := []string{"GET /lol-gameflow/v1/gameflow-phase", "GET /lol-perks/v1/inventory", "GET /lol-perks/v1/pages"}
	if strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatalf("request order = %#v, want %#v", requests, want)
	}
}

func TestGameplayItemSetValidation(t *testing.T) {
	position, err := normalizeGameplayItemSetPosition("support")
	if err != nil || position != "utility" {
		t.Fatalf("position = %q, %v", position, err)
	}
	valid := gameplayItemSetApplyRequest{
		ChampionID: 64, MapID: 11, Position: position,
		Blocks: []gameplayItemSetBlockRequest{{Type: "出门装", Items: []gameplayItemSetItemRequest{{ID: 1055, Count: 1}, {ID: 2003, Count: 1}}}},
	}
	if err := validateGameplayItemSetRequest(valid); err != nil {
		t.Fatalf("valid item set rejected: %v", err)
	}
	invalid := valid
	invalid.Blocks = []gameplayItemSetBlockRequest{{Type: "重复装备", Items: []gameplayItemSetItemRequest{{ID: 1055, Count: 1}, {ID: 1055, Count: 1}}}}
	if err := validateGameplayItemSetRequest(invalid); err == nil {
		t.Fatal("duplicate item IDs were accepted")
	}
	invalid = valid
	invalid.Position = "unknown"
	if _, err := normalizeGameplayItemSetPosition(invalid.Position); err == nil {
		t.Fatal("unknown position was accepted")
	}
}

func TestGameplayRecommendationsAdaptCompleteOPGGData(t *testing.T) {
	detail := championDetailResponse{
		Stats: championDetailStats{WinRate: 52.3, PickRate: 8.4, BanRate: 4.1},
		Counters: championCounterSections{
			StrongAgainst: []championCounterRow{{ChampionID: 24, Name: "贾克斯"}},
			WeakAgainst:   []championCounterRow{{ChampionID: 122, Name: "德莱厄斯"}},
		},
		Runes: []championRunePage{{
			PrimaryStyle: championAsset{ID: 8000, Name: "精密"}, SubStyle: championAsset{ID: 8400, Name: "坚决"},
			Selected:   []championAsset{{ID: 8010}, {ID: 9111}, {ID: 9104}, {ID: 8299}, {ID: 8446}, {ID: 8453}, {ID: 5008}, {ID: 5008}, {ID: 5001}},
			ShardSlots: [][]championAsset{{{ID: 5008, Active: true}}, {{ID: 5008, Active: true}}, {{ID: 5001, Active: true}}},
			PickRate:   75.9, WinRate: 51.2, Games: 3521,
		}},
		Build: championBuildSections{
			SummonerSpells: []championMetricRow{{Assets: []championAsset{{ID: 4}, {ID: 12}}, PickRate: 78.2, WinRate: 54.1, Games: 29884}},
			Skills:         []championMetricRow{{SkillPriority: []string{"Q", "E", "W"}, SkillOrder: []string{"Q", "W", "E"}, PickRate: 61.2, WinRate: 53.6, Games: 21384}},
			StarterItems:   []championMetricRow{{Assets: []championAsset{{ID: 1054}, {ID: 2003}}}},
			Boots:          []championMetricRow{{Assets: []championAsset{{ID: 3047}}}},
			CoreItems:      []championMetricRow{{Assets: []championAsset{{ID: 6630}, {ID: 3071}, {ID: 3053}}}},
		},
	}
	result := gameplayRecommendationsFromChampionDetail(164, "top", detail)
	if result.Hero.WinRate != 52.3 || len(result.Hero.StrongAgainst) != 1 || result.Hero.StrongAgainst[0].ChampionID != 24 {
		t.Fatalf("hero recommendation = %#v", result.Hero)
	}
	if len(result.Runes.OPGG) != 1 || result.Runes.OPGG[0].ChampionID != 164 || len(result.Runes.OPGG[0].SelectedPerkIDs) != 9 || len(result.Runes.OPGG[0].StatModIDs) != 3 || result.Runes.OPGG[0].StatModIDs[0] != 5008 || result.Runes.OPGG[0].StatModIDs[1] != 5008 {
		t.Fatalf("rune recommendation = %#v", result.Runes.OPGG)
	}
	if len(result.Runes.Specialists) != 0 || len(result.Runes.Pros) != 0 || len(result.Build.SpellOptions) != 1 || len(result.Build.ItemRoutes) != 1 || len(result.Build.ItemRoutes[0].IDs) != 3 || strings.Join(result.Build.SkillPriority, ",") != "Q,E,W" {
		t.Fatalf("build recommendation = %#v", result.Build)
	}
	if position, err := normalizeOPGGPosition("utility"); err != nil || position != "support" {
		t.Fatalf("normalized position = %q, %v", position, err)
	}
}

func TestItemSetApplyPreservesUserSetsAndVerifiesIdempotently(t *testing.T) {
	playerRef := strings.Repeat("p", 48)
	document := lcuItemSetDocument{
		"accountId":   json.RawMessage(`456`),
		"timestamp":   json.RawMessage(`123456`),
		"futureField": json.RawMessage(`{"preserved":true}`),
		"itemSets":    json.RawMessage(`[{"uid":"user-owned","title":"我的装备","type":"custom","map":"any","mode":"any","associatedChampions":[],"associatedMaps":[],"blocks":[],"preferredItemSlots":[],"unknown":"keep-me"}]`),
	}
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "GET /lol-champ-select/v1/session":
			_, _ = w.Write([]byte(`{"myTeam":[{"summonerId":123,"puuid":"` + playerRef + `","championId":64}]}`))
		case "GET /lol-item-sets/v1/item-sets/123/sets":
			_ = json.NewEncoder(w).Encode(document)
		case "PUT /lol-item-sets/v1/item-sets/123/sets":
			putCount++
			var updated lcuItemSetDocument
			if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
				t.Fatalf("decode item set PUT: %v", err)
			}
			document = updated
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 123, AccountID: 456, PUUID: playerRef}}
	body := `{"title":"李青 · 打野 OPGG 推荐","championId":64,"mapId":11,"position":"jungle","blocks":[{"type":"出门装","items":[{"id":1055,"count":1}]},{"type":"出装路线","items":[{"id":6630,"count":1},{"id":3071,"count":1}]}]}`
	for attempt := 0; attempt < 2; attempt++ {
		recorder := httptest.NewRecorder()
		a.handleGameplayItemSetApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/item-sets/apply", strings.NewReader(body)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d: %s", attempt, recorder.Code, recorder.Body.String())
		}
		var response struct {
			Applied  bool `json:"applied"`
			Verified bool `json:"verified"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || !response.Applied || !response.Verified {
			t.Fatalf("attempt %d response = %#v, %v", attempt, response, err)
		}
	}
	if putCount != 2 {
		t.Fatalf("PUT count = %d, want 2", putCount)
	}
	if string(document["futureField"]) != `{"preserved":true}` {
		t.Fatalf("top-level unknown field changed: %s", document["futureField"])
	}
	var sets []json.RawMessage
	if err := json.Unmarshal(document["itemSets"], &sets); err != nil || len(sets) != 2 {
		t.Fatalf("item sets = %d, %v", len(sets), err)
	}
	var userSet map[string]json.RawMessage
	if err := json.Unmarshal(sets[0], &userSet); err != nil || string(userSet["unknown"]) != `"keep-me"` {
		t.Fatalf("user set was not preserved: %#v, %v", userSet, err)
	}
	created, found, err := findLCUItemSet(document, itemSetUID(64, "jungle"))
	if err != nil || !found || created.Title != "Deep Legends · 李青 · 打野 OPGG 推荐" || len(created.Blocks) != 2 || len(created.AssociatedChampions) != 1 || created.AssociatedChampions[0] != 64 {
		t.Fatalf("created item set = %#v, %v, %v", created, found, err)
	}
}

func TestItemSetApplyNeverReadsOrWritesSetsOutsideChampionSelect(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/lol-gameflow/v1/gameflow-phase" {
			t.Fatalf("unexpected item-set access outside champion select: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`"Lobby"`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 123, PUUID: strings.Repeat("p", 48)}}
	body := `{"title":"测试","championId":64,"mapId":11,"position":"jungle","blocks":[{"type":"出门装","items":[{"id":1055,"count":1}]}]}`
	recorder := httptest.NewRecorder()
	a.handleGameplayItemSetApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/item-sets/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusConflict || requests != 1 {
		t.Fatalf("status/requests = %d/%d, want %d/1", recorder.Code, requests, http.StatusConflict)
	}
}

func TestSecurityHeadersRemainStrictForGameplayPages(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	wantCSP := "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'"
	if recorder.Header().Get("Content-Security-Policy") != wantCSP || recorder.Header().Get("Referrer-Policy") != "no-referrer" || recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers changed: %#v", recorder.Header())
	}
}
