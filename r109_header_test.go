package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestR109OverviewBackgroundUsesOnlySubjectAndPrefersVerifiedProfile(t *testing.T) {
	a := &app{}
	recent := []gameplayMatch{
		{CreatedAt: 30, Participants: []gameplayParticipant{{ChampionID: 999}}}, // zero IDs cannot identify subject
		{CreatedAt: 10, SubjectParticipantID: 2, Participants: []gameplayParticipant{{ParticipantID: 1, ChampionID: 55}, {ParticipantID: 2, ChampionID: 103}}},
		{CreatedAt: 20, Participants: []gameplayParticipant{{PlayerRef: "subject", ChampionID: 64, ChampionName: "盲僧"}}},
	}
	for _, tc := range []struct {
		name    string
		player  gameplayPlayer
		mastery []gameplayMastery
		want    int64
	}{
		{name: "remote without mastery", want: 64000},
		{name: "highest mastery", mastery: []gameplayMastery{{ChampionID: 1, ChampionPoints: 100}}, want: 1000},
		{name: "actual client background", player: gameplayPlayer{BackgroundSkinID: 103086, BackgroundPath: "/actual.jpg"}, want: 103086},
	} {
		t.Run(tc.name, func(t *testing.T) {
			overview := gameplayOverview{Player: tc.player, Masteries: tc.mastery, Matches: recent}
			a.completeOverviewBackground(&overview, "subject")
			if overview.Player.BackgroundSkinID != tc.want {
				t.Fatalf("background=%d want=%d", overview.Player.BackgroundSkinID, tc.want)
			}
			if len(overview.Masteries) != len(tc.mastery) {
				t.Fatal("fallback fabricated mastery data")
			}
		})
	}
	unknown := gameplayOverview{Matches: recent[:1]}
	a.completeOverviewBackground(&unknown, "subject")
	if unknown.Player.BackgroundPath != "" {
		t.Fatal("unidentified participant was used as the profile owner")
	}
}

func TestR109RiotProgressAndCompleteCarryBackgroundAndProfileWithoutExtraReads(t *testing.T) {
	for _, noMatches := range []bool{false, true} {
		t.Run(map[bool]string{false: "with matches", true: "empty mode"}[noMatches], func(t *testing.T) {
			f := r98OverviewFixture(t, false)
			transport := f.a.riot.champions.client.Transport
			f.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := ""
				switch {
				case strings.Contains(r.URL.Path, "/league/"):
					body = `[{"queueType":"RANKED_SOLO_5x5","tier":"MASTER","leaguePoints":300}]`
				case strings.Contains(r.URL.Path, "/champion-mastery/"):
					body = `[{"championId":103,"championPoints":1000,"championLevel":10}]`
				case noMatches && strings.HasSuffix(r.URL.Path, "/ids"):
					body = `[]`
				}
				if body != "" {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				}
				return transport.RoundTrip(r)
			})
			var last gameplayOverview
			frames := 0
			ctx := context.WithValue(context.Background(), riotOverviewProgressKey{}, func(p gameplayOverview) { last = p; frames++ })
			got, err := f.a.loadRiotOverview(ctx, gameplayReference{PlayerRef: "r98-subject", Region: "kr", Privacy: "PRIVATE"}, 0, 1)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []gameplayOverview{last, got} {
				if p.Player.BackgroundSkinID != 103000 || len(p.Ranks) != 1 || len(p.Masteries) != 1 {
					t.Fatalf("incomplete header: background=%d ranks=%d masteries=%d", p.Player.BackgroundSkinID, len(p.Ranks), len(p.Masteries))
				}
			}
			if frames == 0 || last.Matches == nil {
				t.Fatal("empty mode must still stream a valid profile frame")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.calls["detail"] > 1 || f.calls["matchIDs"] > 1 {
				t.Fatal("header triggered additional match queries", f.calls)
			}
		})
	}
}

func TestR109IncompleteHeaderDoesNotBecomeFreshSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, state            string
		partial, pending, want bool
	}{
		{name: "ranked-stats", state: capabilityFailed},
		{name: "champion-mastery", state: capabilityCanceled},
		{name: "ranked-stats", state: capabilityAvailable, want: true},
		{name: "champion-mastery", state: capabilityUnsupported, want: true},
		{partial: true}, {pending: true},
	} {
		cache := newOverviewQueryCache()
		flight := &overviewQueryFlight{done: make(chan struct{})}
		cache.flights["test"] = flight
		p := gameplayOverview{ProfilePending: tc.pending, Pagination: gameplayPagination{Partial: tc.partial}, Capabilities: []EndpointCapability{{Name: tc.name, State: tc.state}}}
		cache.complete("test", flight, p, nil)
		_, ok := cache.getLocked("test", time.Now())
		if ok != tc.want {
			t.Fatalf("%+v cache=%v", tc, ok)
		}
		select {
		case <-flight.done:
		default:
			t.Fatal("waiting callers were not released")
		}
	}
}

func TestR109CNMatchTimeoutKeepsReadyRankAndMastery(t *testing.T) {
	a, publicRef, _ := newGameplayOverviewSGPFixture(t, true)
	subject := strings.Repeat("o", 48)
	a.rankScores = newRankScoreCache()
	a.rankScores.put(rankScoreCacheKey(dataSourceLCU, "HN1", subject), rankScoreEntry{
		at: time.Now(), ranks: []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Tier: "MASTER"}},
		capability: EndpointCapability{Name: "ranked-stats", State: capabilityAvailable},
	})
	lcuTransport := a.lcu.http.Transport
	a.lcu.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/lol-champion-mastery/") {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"championId":103,"championLevel":20,"championPoints":10000}]`)), Request: r}, nil
		}
		return lcuTransport.RoundTrip(r)
	})
	sgpTransport := a.sgp.http.Transport
	a.sgp.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/match-history-query/") {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return sgpTransport.RoundTrip(r)
	})
	a.overviewTimeout = func(ctx context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
		return context.WithTimeout(ctx, 150*time.Millisecond)
	}
	recorder := httptest.NewRecorder()
	a.handleGameplayOverview(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"playerRef":"`+publicRef+`","count":20}`)))
	var got gameplayOverview
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Pagination.BudgetExceeded || len(got.Ranks) != 1 || len(got.Masteries) != 1 || got.Player.BackgroundSkinID != 103000 {
		t.Fatalf("timeout lost ready profile: budget=%v ranks=%d masteries=%d background=%d status=%d", got.Pagination.BudgetExceeded, len(got.Ranks), len(got.Masteries), got.Player.BackgroundSkinID, recorder.Code)
	}
	for _, cap := range got.Capabilities {
		if (cap.Name == "ranked-stats" || cap.Name == "champion-mastery") && cap.State != capabilityAvailable {
			t.Fatalf("ready capability replaced by timeout: %+v", cap)
		}
	}
}
