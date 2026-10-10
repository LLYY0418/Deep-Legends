package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The same fixture also runs against c725ba27 in a temporary checkout. These
// times measure fake-service data readiness, not real Riot or Windows latency.
func TestR266FakeServiceTimingEvidence(t *testing.T) {
	if output := os.Getenv("R266_CARD_SERVER"); output != "" {
		r266CardTimingServer(t, output)
		return
	}
	f := r98OverviewFixture(t, true)
	t.Setenv("RIOT_API_KEY", "RGAPI-00000000-0000-0000-0000-000000000000")
	f.a.overviewQueries = newOverviewQueryCache()
	f.a.proPlayers.teams = []opggProTeam{{ID: 371, Members: []opggProMember{{TeamID: 371, Nickname: "Rookie", RealName: "Song Eui-jin", Position: "middle", Authority: "PROGAMER", Summoners: []opggProAccount{{PUUID: "r98-subject", GameName: "Fixture", TagLine: "KR1", Region: "kr"}}}}}}
	started := time.Now()
	ready := map[string]int64{}
	var timingMu sync.Mutex
	old := f.a.riot.champions.client.Transport
	f.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		category := riotRelayRequestCategory(r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/ids") {
			category = "ids"
		}
		response, err := old.RoundTrip(r)
		timingMu.Lock()
		if _, exists := ready[category]; !exists && err == nil {
			ready[category] = time.Since(started).Milliseconds()
		}
		timingMu.Unlock()
		return response, err
	})
	recorder := httptest.NewRecorder()
	f.a.handleGameplayOverview(recorder, httptest.NewRequest("POST", "/api/gameplay/overview", strings.NewReader(`{"gameName":"Fixture","tagLine":"KR1","region":"kr","count":5}`)))
	if recorder.Code != 200 {
		t.Fatalf("fake overview status=%d", recorder.Code)
	}
	f.mu.Lock()
	evidence := map[string]any{"scope": "fake service; backend data-ready times", "requests": f.calls, "ready_ms": ready, "total_ms": time.Since(started).Milliseconds()}
	data, _ := json.Marshal(evidence)
	f.mu.Unlock()
	fmt.Println("R266_TIMING " + string(data))

	// Begin one foreground load and ask the actual background request path to
	// start while details are in flight. The baseline admits it; R266 must wait.
	g := r98OverviewFixture(t, true)
	var backgroundRequests atomic.Int64
	backgroundTransport := g.a.riot.champions.client.Transport
	g.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/lol/status/") {
			backgroundRequests.Add(1)
		}
		return backgroundTransport.RoundTrip(r)
	})
	finished := make(chan error, 1)
	go func() {
		_, err := g.a.loadRiotOverview(context.Background(), gameplayReference{PlayerRef: "r98-subject", Region: "kr"}, 0, 5)
		finished <- err
	}()
	<-g.detailStarted
	ctx, cancel := context.WithTimeout(withRiotBackground(context.Background()), 60*time.Millisecond)
	var out any
	before := time.Now()
	_ = g.a.riot.get(ctx, g.a.riot.platformHost(), "/lol/status/v4/platform-data", nil, &out)
	waitMS := time.Since(before).Milliseconds()
	cancel()
	if loadErr := <-finished; loadErr != nil {
		t.Fatal(loadErr)
	}
	if os.Getenv("R266_TIMING_BASELINE") != "1" && backgroundRequests.Load() != 0 {
		t.Fatal("background request started during foreground")
	}
	data, _ = json.Marshal(map[string]any{"scope": "background during foreground", "new_requests": backgroundRequests.Load(), "wait_ms": waitMS})
	fmt.Println("R266_BACKGROUND " + string(data))
	resumed := time.Now()
	resumeCtx, resumeCancel := context.WithTimeout(withRiotBackground(context.Background()), 6*time.Second)
	defer resumeCancel()
	if err := g.a.riot.get(resumeCtx, g.a.riot.platformHost(), "/lol/status/v4/platform-data", nil, &out); err != nil {
		t.Fatal(err)
	}
	resumeMS := time.Since(resumed).Milliseconds()
	if os.Getenv("R266_TIMING_BASELINE") != "1" && (resumeMS < 4900 || backgroundRequests.Load() != 1) {
		t.Fatal("background resumed before the five-second grace period")
	}
	data, _ = json.Marshal(map[string]any{"scope": "background after foreground", "requests_total": backgroundRequests.Load(), "resume_ms": resumeMS})
	fmt.Println("R266_BACKGROUND_RESUMED " + string(data))
}

// Opt-in evidence server uses actual overview/OP.GG handlers and embedded web
// assets on both releases. It never contacts an external upstream.
func r266CardTimingServer(t *testing.T, output string) {
	f := r98OverviewFixture(t, true)
	a, cp := f.a, f.a.riot.champions
	a.token = "r266-local-fixture"
	a.champions, a.opgg, a.overviewQueries = cp, newOPGGInsights(), newOverviewQueryCache()
	a.proPlayers.teams = []opggProTeam{{ID: 371, Members: []opggProMember{{TeamID: 371, Nickname: "Rookie", RealName: "Song Eui-jin", Position: "middle", Authority: "PROGAMER", Summoners: []opggProAccount{{PUUID: "subject-puuid-0000001", GameName: "Fixture", TagLine: "KR1", Region: "kr"}}}}}}
	cp.championMeta[126] = championMetadata{ID: 126, Key: "Jayce", NameZH: "杰斯"}
	old := cp.client.Transport
	cp.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "op.gg" {
			if r.URL.Path != "/zh-cn/lol/summoners/kr/Fixture-KR1" {
				return nil, fmt.Errorf("unexpected fake OP.GG path")
			}
			f.mu.Lock()
			f.calls["opgg_"+r.Method]++
			f.mu.Unlock()
			time.Sleep(40 * time.Millisecond)
			data := summaryFixtureHTML("subject-puuid-0000001", 33)
			if r.Method == "POST" {
				data = summaryFixtureResponse()
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
		}
		response, err := old.RoundTrip(r)
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			response.Body = io.NopCloser(strings.NewReader(strings.ReplaceAll(string(body), "r98-subject", "subject-puuid-0000001")))
			var data string
			switch {
			case strings.Contains(r.URL.Path, "/league/"):
				data = `[{"queueType":"RANKED_SOLO_5x5","tier":"CHALLENGER","rank":"I","leaguePoints":900,"wins":100,"losses":50}]`
			case strings.Contains(r.URL.Path, "/champion-mastery/"):
				data = `[{"championId":1,"championLevel":10,"championPoints":90000}]`
			case strings.Contains(r.URL.Path, "/matches/KR_"):
				body, _ := io.ReadAll(response.Body)
				data = strings.Replace(string(body), `"championId":1`, `"teamPosition":"MIDDLE","individualPosition":"MIDDLE","championId":1`, 1)
			}
			if data != "" {
				response.Body.Close()
				response.Body = io.NopCloser(strings.NewReader(data))
				response.ContentLength = int64(len(data))
			}
		}
		return response, err
	})
	files, err := fs.Sub(embedded, "web")
	if err != nil {
		t.Fatal(err)
	}
	static := newStaticAssetHandler(files)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.handleBootstrap(static))
	mux.Handle("GET /", static)
	mux.HandleFunc("POST /api/gameplay/overview", a.authorized(a.handleGameplayOverview))
	mux.HandleFunc("POST /api/gameplay/season-summary", a.authorized(a.handleOPGGSeasonSummary))
	server := httptest.NewServer(securityHeaders(mux))
	defer server.Close()
	ready, _ := json.Marshal(map[string]any{"baseUrl": server.URL, "token": a.token, "bootstrapUrl": server.URL})
	if err := os.WriteFile(output, ready, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(output + ".done"); err == nil {
			f.mu.Lock()
			data, _ := json.Marshal(f.calls)
			f.mu.Unlock()
			fmt.Println("R266_CARD_REQUESTS " + string(data))
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("fake card timing browser did not finish")
}
