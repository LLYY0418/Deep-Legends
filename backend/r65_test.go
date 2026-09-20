package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR65FacadeStateOnlyExposesStableRenderFields(t *testing.T) {
	var chatReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			_, _ = w.Write([]byte(`{"backgroundSkinId":103000,"backgroundSkinName":"星之守护者","backgroundSkinAugments":"PRIVATE_AUGMENT","regalia":"PRIVATE_PROFILE_REGALIA"}`))
		case "/lol-chat/v1/me":
			version := chatReads.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"availability": "away", "statusMessage": "稳定签名", "lastSeenOnlineTimestamp": version,
				"puuid": "PRIVATE_CHAT_PUUID", "obfuscatedSummonerId": 998877,
				"lol": map[string]any{
					"rankedLeagueQueue": "RANKED_SOLO_5X5", "rankedLeagueTier": "MASTER", "rankedLeagueDivision": "I",
					"gameStatus": "outOfGame", "lastSeenOnlineTimestamp": version,
				},
			})
		case "/lol-regalia/v2/current-summoner/regalia":
			_, _ = w.Write([]byte(`{"preferredCrestType":"ranked","puuid":"PRIVATE_REGALIA_PUUID"}`))
		case "/lol-challenges/v1/summary-player-data/local-player":
			_, _ = w.Write([]byte(`{"title":{"name":"无畏先锋","contentId":"PRIVATE_TITLE_ID"},"selectedChallengesString":"1","topChallenges":[]}`))
		case "/lol-challenges/v1/challenges/local-player":
			_, _ = w.Write([]byte(`{"1":{"id":1,"name":"坚韧不拔","iconPath":"/challenge/1.png","description":"PRIVATE_DESCRIPTION"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{
		connected: true,
		lcu:       client,
		summoner: Summoner{
			SummonerID: 42, AccountID: 43, PUUID: "PRIVATE_SUMMONER_PUUID", DisplayName: "测试玩家",
			GameName: "稳定名字", TagLine: "1234", ProfileIconID: 27, SummonerLevel: 526, Privacy: "PRIVATE",
		},
	}
	first := a.loadFacadeState(context.Background())
	second := a.loadFacadeState(context.Background())
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("upstream timestamp-only change altered facade response:\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
	for _, forbidden := range []string{"lastSeenOnlineTimestamp", "puuid", "obfuscatedSummonerId", "regalia", "PRIVATE_"} {
		if strings.Contains(string(firstJSON), forbidden) {
			t.Fatalf("facade response leaked %q: %s", forbidden, firstJSON)
		}
	}
	if first.Summoner.GameName != "稳定名字" || first.Summoner.TagLine != "1234" || first.Summoner.ProfileIconID != 27 || first.Summoner.SummonerLevel != 526 {
		t.Fatalf("stable summoner projection = %#v", first.Summoner)
	}
	if first.Profile.BackgroundSkinID != 103000 || first.Profile.BackgroundSkinName != "星之守护者" {
		t.Fatalf("stable profile projection = %#v", first.Profile)
	}
	if first.Chat.Availability != "away" || first.Chat.StatusMessage != "稳定签名" || first.Chat.LOL.RankedLeagueTier != "MASTER" {
		t.Fatalf("stable chat projection = %#v", first.Chat)
	}
	title, ok := first.ChallengeSummary["title"].(map[string]any)
	if !ok || title["name"] != "无畏先锋" || len(title) != 1 {
		t.Fatalf("stable title projection = %#v", first.ChallengeSummary)
	}
	if len(first.Challenges) != 1 || first.Challenges[0].Name != "坚韧不拔" {
		t.Fatalf("challenge projection = %#v", first.Challenges)
	}
}
