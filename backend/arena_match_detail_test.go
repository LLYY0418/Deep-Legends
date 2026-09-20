package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestValidArenaRiotMatchID(t *testing.T) {
	for _, matchID := range []string{"KR_1", "KR_1234567890"} {
		if !validArenaRiotMatchID(matchID) {
			t.Fatalf("valid match ID %q was rejected", matchID)
		}
	}
	for _, matchID := range []string{"", "KR_", " KR_1", "KR_1 ", "NA1_123", "kr_123", "KR_1/../x", "KR_1%2F..%2Fx", "KR_12?x=1"} {
		if validArenaRiotMatchID(matchID) {
			t.Fatalf("invalid match ID %q was accepted", matchID)
		}
	}
}

func TestArenaMatchDetailIsMemoryCachedAndPublicizesPlayerReferences(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	const firstPUUID = "first-player-private-puuid-1234567890"
	const secondPUUID = "second-player-private-puuid-1234567890"
	raw := riotMatch{}
	raw.Metadata.MatchID = "KR_123"
	raw.Info.GameID = 123
	raw.Info.GameCreation = 1_725_000_000_000
	raw.Info.GameDuration = 900
	raw.Info.QueueID = 1700
	raw.Info.GameMode = "CHERRY"
	raw.Info.GameType = "MATCHED_GAME"
	raw.Info.MapID = 30
	raw.Info.Participants = []riotParticipant{
		{ParticipantID: 1, PUUID: firstPUUID, RiotIDGameName: "Winner", RiotIDTagline: "KR1", ChampionID: 799, ChampLevel: 18, PlayerSubteamID: 1, SubteamPlacement: 1, Kills: 12, Deaths: 4, Assists: 9, GoldEarned: 16000, TotalDamageDealtToChampions: 80000},
		{ParticipantID: 2, PUUID: secondPUUID, RiotIDGameName: "Mate", RiotIDTagline: "KR1", ChampionID: 111, ChampLevel: 17, PlayerSubteamID: 1, SubteamPlacement: 1, Kills: 4, Deaths: 6, Assists: 18, GoldEarned: 12000, TotalDamageDealtToChampions: 30000},
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	champions := newChampionProvider()
	champions.championMeta[799] = championMetadata{ID: 799, NameZH: "安蓓萨"}
	champions.championMeta[111] = championMetadata{ID: 111, NameZH: "深海泰坦"}
	var upstreamCalls atomic.Int32
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamCalls.Add(1)
		if request.URL.Host != riotClusterHost || request.URL.Path != "/lol/match/v5/matches/KR_123" {
			t.Fatalf("unexpected Riot request %s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(payload))), Request: request}, nil
	})}
	champions.clientMu.Unlock()
	events := make([]map[string]any, 0, 2)
	champions.diag = func(event map[string]any) { events = append(events, event) }
	a := &app{
		token: "arena-detail-test", champions: champions, riot: newRiotProvider(champions),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}

	requestMatch := func() (gameplayMatch, string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/champions/arena/match/KR_123", nil)
		request.SetPathValue("matchId", "KR_123")
		recorder := httptest.NewRecorder()
		a.handleArenaMatchDetail(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
		}
		var match gameplayMatch
		if err := json.Unmarshal(recorder.Body.Bytes(), &match); err != nil {
			t.Fatal(err)
		}
		return match, recorder.Body.String()
	}

	first, firstBody := requestMatch()
	second, secondBody := requestMatch()
	if upstreamCalls.Load() != 1 {
		t.Fatalf("Riot upstream calls = %d, want 1", upstreamCalls.Load())
	}
	for _, body := range []string{firstBody, secondBody} {
		for _, forbidden := range []string{firstPUUID, secondPUUID, `"puuid"`} {
			if strings.Contains(strings.ToLower(body), strings.ToLower(forbidden)) {
				t.Fatalf("response leaked %q: %s", forbidden, body)
			}
		}
	}
	if len(first.Participants) != 2 || len(second.Participants) != 2 {
		t.Fatalf("participants = %d / %d", len(first.Participants), len(second.Participants))
	}
	for index := range first.Participants {
		firstRef := first.Participants[index].PlayerRef
		secondRef := second.Participants[index].PlayerRef
		if !strings.HasPrefix(firstRef, "player_") || firstRef != secondRef {
			t.Fatalf("public player refs = %q / %q", firstRef, secondRef)
		}
	}
	if len(events) != 2 {
		t.Fatalf("diagnostic count = %d", len(events))
	}
	allowed := map[string]bool{"event": true, "source": true, "status": true, "duration_ms": true, "cache": true, "outcome": true, "errorKind": true}
	for index, event := range events {
		for key := range event {
			if !allowed[key] {
				t.Fatalf("event %d has unexpected field %q: %#v", index, key, event)
			}
		}
		if len(event) != len(allowed) || event["event"] != "arena_match_detail" || event["source"] != "riot" || event["status"] != http.StatusOK || event["outcome"] != "success" {
			t.Fatalf("event %d = %#v", index, event)
		}
	}
	if events[0]["cache"] != "miss" || events[1]["cache"] != "hit" {
		t.Fatalf("cache diagnostics = %#v", events)
	}
}

func TestArenaMatchDetailRejectsInvalidIDBeforeRiotRequest(t *testing.T) {
	champions := newChampionProvider()
	events := make([]map[string]any, 0, 1)
	champions.diag = func(event map[string]any) { events = append(events, event) }
	a := &app{champions: champions}
	request := httptest.NewRequest(http.MethodGet, "/api/champions/arena/match/NA1_123", nil).WithContext(context.Background())
	request.SetPathValue("matchId", "NA1_123")
	recorder := httptest.NewRecorder()
	a.handleArenaMatchDetail(recorder, request)
	if recorder.Code != http.StatusBadRequest || len(events) != 1 || events[0]["errorKind"] != "invalid-match-id" {
		t.Fatalf("response = %d %q, events = %#v", recorder.Code, recorder.Body.String(), events)
	}
}

func TestArenaMatchDetailWithoutAPIKeyFailsClosed(t *testing.T) {
	previousCipher := riotAPIKeyCipher
	riotAPIKeyCipher = ""
	t.Cleanup(func() { riotAPIKeyCipher = previousCipher })
	t.Setenv("RIOT_API_KEY", "")

	champions := newChampionProvider()
	var upstreamCalls atomic.Int32
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamCalls.Add(1)
		return nil, errors.New("Riot request must not run without an API key")
	})}
	champions.clientMu.Unlock()
	events := make([]map[string]any, 0, 1)
	champions.diag = func(event map[string]any) { events = append(events, event) }
	a := &app{champions: champions, riot: newRiotProvider(champions)}
	request := httptest.NewRequest(http.MethodGet, "/api/champions/arena/match/KR_123", nil)
	request.SetPathValue("matchId", "KR_123")
	recorder := httptest.NewRecorder()

	a.handleArenaMatchDetail(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "没有注入 Riot API Key") {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("Riot upstream calls = %d, want 0", upstreamCalls.Load())
	}
	if len(events) != 1 || events[0]["status"] != http.StatusServiceUnavailable || events[0]["errorKind"] != "not-configured" {
		t.Fatalf("diagnostic events = %#v", events)
	}
}
