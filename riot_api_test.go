package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRiotAccountRequestDoesNotDoubleEncodeKoreanNames(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	var gotURL string
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		gotURL = request.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"puuid":"abc","gameName":"노 갱","tagLine":"0518"}`)),
			Request:    request,
		}, nil
	})}
	champions.clientMu.Unlock()
	account, err := newRiotProvider(champions).accountByRiotID(context.Background(), "노 갱", "0518")
	if err != nil {
		t.Fatal(err)
	}
	if account.GameName != "노 갱" || account.TagLine != "0518" {
		t.Fatalf("account = %+v", account)
	}
	escaped := url.PathEscape("노 갱")
	if escaped == "" || strings.Contains(gotURL, "%25") {
		t.Fatalf("path was double-encoded: %s (once-escaped name %q)", gotURL, escaped)
	}
	if !strings.Contains(gotURL, escaped) {
		t.Fatalf("request URL %s missing once-escaped name %q", gotURL, escaped)
	}
}

func TestRiotMatchIDsFilteredAddsQueueAndTypeQuery(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	var requests []*http.Request
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.Clone(request.Context()))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`["KR_1"]`)),
			Request:    request,
		}, nil
	})}
	champions.clientMu.Unlock()
	provider := newRiotProvider(champions)
	if _, err := provider.matchIDsFiltered(context.Background(), "puuid", 4, 30, 420, "ranked"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	query := requests[0].URL.Query()
	if query.Get("start") != "4" || query.Get("count") != "30" || query.Get("queue") != "420" || query.Get("type") != "ranked" {
		t.Fatalf("filtered match query = %v", query)
	}
	requests = nil
	if _, err := provider.matchIDsFiltered(context.Background(), "puuid", 0, 30, 0, ""); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 {
		t.Fatalf("unfiltered requests = %d, want 1", len(requests))
	}
	query = requests[0].URL.Query()
	if query.Get("queue") != "" || query.Get("type") != "" {
		t.Fatalf("unfiltered match query unexpectedly contains filters: %v", query)
	}
}

func TestRiotProfileCapabilitiesTreatCancellationAsCanceled(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})}
	champions.clientMu.Unlock()
	provider := newRiotProvider(champions)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, capability := provider.loadRiotRanks(ctx, "player"); capability.State != capabilityCanceled {
		t.Fatalf("rank capability after cancellation = %#v", capability)
	}
	if _, capability := provider.loadRiotMasteries(ctx, "player", nil); capability.State != capabilityCanceled {
		t.Fatalf("mastery capability after cancellation = %#v", capability)
	}
}

func TestRiotMatchConversionPreservesControlWardZero(t *testing.T) {
	controlWardsBought := 0
	info := riotMatchInfo{
		GameID:       99,
		GameDuration: 1800,
		QueueID:      420,
		Participants: []riotParticipant{{
			ParticipantID: 1, TeamID: 100, PUUID: "subject", ChampionID: 64,
			VisionWardsBoughtInGame: &controlWardsBought,
		}},
	}

	match := convertRiotMatchInfo(&info, "subject", map[int64]string{64: "李青"}, nil, riotRegionKR, "")
	if len(match.Participants) != 1 {
		t.Fatalf("participants = %#v", match.Participants)
	}
	got := match.Participants[0].ControlWardsBought
	if got == nil || *got != 0 {
		t.Fatalf("control ward zero was lost: %#v", got)
	}
}

func TestOpggParseSearchResultReadsSummoners(t *testing.T) {
	payload := "0:{\"a\":1}\n1:{\"summoners\":[{\"gameName\":\"Hide on bush\",\"tagline\":\"KR1\"},{\"gameName\":\"노 갱\",\"tagline\":\"0518\"}]}\n"
	got := opggParseSearchResult([]byte(payload))
	if len(got) != 2 || got[0].GameName != "Hide on bush" || got[0].TagLine != "KR1" || got[1].GameName != "노 갱" || got[1].TagLine != "0518" {
		t.Fatalf("parsed %+v", got)
	}
	if opggParseSearchResult([]byte("not-json")) != nil {
		t.Fatal("expected empty result for invalid payload")
	}
}

func TestRiotOverviewCapsMatchDetailConcurrencyAtFour(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	const detailCount = 8
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	champions := newChampionProvider()
	champions.mu.Lock()
	champions.championMeta = map[int]championMetadata{1: {NameZH: "黑暗之女"}}
	champions.mu.Unlock()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := "[]"
		switch {
		case strings.Contains(request.URL.Path, "/lol/summoner/v4/summoners/by-puuid/"):
			body = `{"puuid":"subject","profileIconId":1,"summonerLevel":100}`
		case strings.HasSuffix(request.URL.Path, "/ids"):
			body = `["KR_1","KR_2","KR_3","KR_4","KR_5","KR_6","KR_7","KR_8"]`
		case strings.Contains(request.URL.Path, "/lol/league/v4/entries/"):
			body = "[]"
		case strings.Contains(request.URL.Path, "/lol/champion-mastery/v4/"):
			body = "[]"
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/"):
			active := inFlight.Add(1)
			for {
				seen := maxInFlight.Load()
				if active <= seen || maxInFlight.CompareAndSwap(seen, active) {
					break
				}
			}
			time.Sleep(80 * time.Millisecond)
			inFlight.Add(-1)
			body = `{"metadata":{"matchId":"fixture"},"info":{"gameId":1,"queueId":420,"gameDuration":1800,"participants":[{"puuid":"subject","participantId":1,"teamId":100,"championId":1,"win":true}]}}`
		default:
			t.Fatalf("unexpected Riot request: %s", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	champions.clientMu.Unlock()

	a := &app{riot: newRiotProvider(champions)}
	overview, err := a.loadRiotOverview(context.Background(), gameplayReference{PlayerRef: "subject", Region: riotRegionKR}, 0, detailCount)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Matches) != detailCount {
		t.Fatalf("matches = %d, want %d", len(overview.Matches), detailCount)
	}
	if got := maxInFlight.Load(); got == 0 || got > 4 {
		t.Fatalf("maximum concurrent Riot match detail requests = %d, want 1..4", got)
	}
}
