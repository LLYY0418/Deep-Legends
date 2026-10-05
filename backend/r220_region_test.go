package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestR220PlatformRoutingAndIsolation(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-r220-fixture")
	expected := map[string]string{"br1": "americas", "eun1": "europe", "euw1": "europe", "jp1": "asia", "kr": "asia", "la1": "americas", "la2": "americas", "me1": "europe", "na1": "americas", "oc1": "sea", "ru": "europe", "sg2": "sea", "tr1": "europe", "tw2": "sea", "vn2": "sea"}
	champs := newChampionProvider()
	var hosts []string
	champs.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		return proHTTPBody([]byte(`{"puuid":"r220-valid-account","gameName":"Fixture","tagLine":"TEST"}`)), nil
	})}
	root := newRiotProvider(champs)
	for region, cluster := range expected {
		p := root.forPlatform(region)
		if p.region() != region || p.platformHost() != region+".api.riotgames.com" || p.clusterHost() != cluster+".api.riotgames.com" {
			t.Fatal(region, p.region(), p.platformHost(), p.clusterHost())
		}
		if p != root.forPlatform(strings.ToUpper(region)) {
			t.Fatal("provider not reused", region)
		}
		accountCluster := cluster
		if cluster == "sea" {
			accountCluster = "asia"
		}
		if _, err := p.accountByRiotID(t.Context(), "Fixture", "TEST"); err != nil {
			t.Fatal(err)
		}
		if hosts[len(hosts)-1] != accountCluster+".api.riotgames.com" {
			t.Fatal(region, hosts)
		}
		if !validRiotMatchID(riotMatchID(region, 220)) || !publicRiotMatchCacheKey("riot-match-v4|"+riotMatchID(region, 220)) {
			t.Fatal("platform cache rejected", region)
		}
		for _, host := range []string{p.platformHost(), p.clusterHost()} {
			endpoint, err := riotRelayEndpoint("https://fixture", host, "/test")
			if err != nil || !strings.Contains(endpoint, "/r/"+strings.TrimSuffix(host, ".api.riotgames.com")+"/") {
				t.Fatal(endpoint, err)
			}
		}
	}
	if len(hosts) != len(expected) {
		t.Fatal("cross-platform account cache was reused", hosts)
	}
	jp, kr := root.forPlatform("jp1"), root.forPlatform("kr")
	before := len(kr.shortWindow)
	jp.shortWindow = []time.Time{time.Now(), time.Now()}
	if len(kr.shortWindow) != before || jp == kr {
		t.Fatal("limiter shared")
	}
	if riotIdentityKey("ranks:fixture|platform:jp1") == riotIdentityKey("ranks:fixture|platform:kr") {
		t.Fatal("disk identity collision")
	}
	if _, err := riotRelayEndpoint("https://fixture", "evil.api.riotgames.com", "/test"); err == nil {
		t.Fatal("illegal host")
	}
}

func TestR220ClientStatusAndDiagnosticWhitelist(t *testing.T) {
	for _, tc := range []struct{ region, platform, want, label string }{{"TENCENT", "HN1", "TENCENT", "国服"}, {"JP", "JP1", "jp1", "日服"}, {"KR", "KR", "kr", "韩服"}, {"PRIVATE-REGION", "PRIVATE-PLATFORM", "", ""}} {
		c := newLCUClient(1, "fixture")
		c.region, c.rsoPlatform, c.platformProbe, c.platformSource = tc.region, tc.platform, true, "startup-args"
		a := &app{connected: true, lcu: c}
		response := httptest.NewRecorder()
		a.handleStatus(response, httptest.NewRequest("GET", "/api/status", nil))
		var status statusResponse
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if status.ClientRegion != tc.want || status.ClientRegionLabel != tc.label {
			t.Fatal(status.ClientRegion, status.ClientRegionLabel)
		}
		event := clientPlatformDiagnostic(c)
		if len(event) != 7 || event["event"] != "client_platform_resolved" || (tc.want != "" && event["source"] != "startup-args") {
			t.Fatal(event)
		}
		raw, _ := json.Marshal(event)
		if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "fixture") {
			t.Fatal(string(raw))
		}
	}
	c := newLCUClient(1, "fixture")
	c.applyPlatformArgs("--region=JP")
	c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/riotclient/command-line-args" {
			t.Fatal(r.URL.Path)
		}
		return proHTTPBody([]byte(`["--region=JP","--rso_platform_id=JP1"]`)), nil
	})}
	event := clientPlatformDiagnostic(c)
	if event["source"] != "command-line-query" || event["platform"] != "JP1" {
		t.Fatal(event)
	}
}
func TestR220HistoryRetryCadence(t *testing.T) {
	for _, status := range []int{500, 503, 404} {
		c := newLCUClient(1, "fixture")
		c.region, c.rsoPlatform = "JP", "JP1"
		calls := 0
		var delays []time.Duration
		c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls < 4 {
				return r206RelayResponse(status, []byte(`{}`)), nil
			}
			return proHTTPBody([]byte(`{"games":{"games":[]}}`)), nil
		})}
		c.historyRetrySleep = func(ctx context.Context, d time.Duration) error { delays = append(delays, d); return ctx.Err() }
		_, caps, _ := loadGameplayHistoryContext(t.Context(), c, "r220-valid-account", true, 0, 5, false)
		if status == 404 {
			if calls != 1 || len(delays) != 0 || caps[0].State == capabilityAvailable {
				t.Fatal(calls, delays, caps)
			}
		} else if calls != 4 || !reflect.DeepEqual(delays, []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second}) || caps[0].State != capabilityAvailable {
			t.Fatal(calls, delays, caps)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if transientLCUHistoryError(ctx, &LCUHTTPError{StatusCode: 500}) {
		t.Fatal("cancelled retry")
	}
}
func TestR220HistoryDecisionAndFallback(t *testing.T) {
	input := matchHistoryDataSourceInput{PlayerReferenceValid: true, LCUConnected: true, SGPAvailable: true}
	if got := resolveMatchHistoryDataSources(input, "jp1"); !reflect.DeepEqual(got.Sources, []string{dataSourceRiot, dataSourceLCU}) {
		t.Fatal(got)
	}
	if got := resolveMatchHistoryDataSources(input, "TENCENT"); !reflect.DeepEqual(got.Sources, []string{dataSourceSGP, dataSourceLCU}) {
		t.Fatal(got)
	}
	t.Setenv("RIOT_API_KEY", "RGAPI-r220-fixture")
	for _, fail := range []bool{false, true} {
		calls := []string{}
		champs := newChampionProvider()
		champs.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls = append(calls, r.URL.Host)
			if fail {
				return r206RelayResponse(404, []byte(`{}`)), nil
			}
			return proHTTPBody([]byte(`[]`)), nil
		})}
		c := newLCUClient(1, "fixture")
		c.region, c.rsoPlatform = "JP", "JP1"
		c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls = append(calls, "lcu")
			return proHTTPBody([]byte(`{"games":{"games":[]}}`)), nil
		})}
		a := &app{riot: newRiotProvider(champs)}
		_, caps, _ := a.loadDetailedMatches(t.Context(), c, gameplayReference{PlayerRef: "r220-valid-account"}, "r220-valid-account", true, 0, 5, "all", nil, nil)
		want := []string{"asia.api.riotgames.com"}
		if fail {
			want = append(want, "lcu")
		}
		if !reflect.DeepEqual(calls, want) || caps[0].State != capabilityAvailable {
			t.Fatal(calls, caps)
		}
		attempts := caps[0].Attempts
		if !fail && len(attempts) != 1 || fail && (len(attempts) != 2 || attempts[0].Message == "") {
			t.Fatal(attempts)
		}
	}
}
func TestR220CooldownPlatformIsolation(t *testing.T) {
	s := &riotRelayState{}
	s.observeCooldown("asia.api.riotgames.com", "/match", "application", 60, "jp1")
	if s.requestErrorFor("asia.api.riotgames.com", "/account", "jp1") == nil || s.requestErrorFor("asia.api.riotgames.com", "/account", "kr") != nil {
		t.Fatal("shared cluster cooldown not platform isolated")
	}
	s.cooldown("ip", 60)
	if s.requestErrorFor("kr.api.riotgames.com", "/rank", "kr") == nil {
		t.Fatal("IP safety quota must remain shared")
	}
}
func TestR220PracticeModeAndPrimaryPosition(t *testing.T) {
	for _, queue := range []int64{0, 3140} {
		if got := resolveGameplayRecommendationMode(queue, "PRACTICETOOL", 11); got.InternalMode != "ranked" || got.IsFallback {
			t.Fatal(got)
		}
	}
	if got := resolveGameplayRecommendationMode(3140, "PRACTICETOOL", 12); got.InternalMode == "ranked" {
		t.Fatal("wrong map", got)
	}
	position, source, err := resolveGameplayRecommendationPosition("", []championPositionOption{{Position: "mid", RoleRate: 90}, {Position: "top", RoleRate: 10}})
	if err != nil || position != "mid" || source != "opgg-primary" {
		t.Fatal(position, source, err)
	}
}

func TestR220LocalOverviewRetryBudget(t *testing.T) {
	var budget time.Duration
	a := &app{overviewTimeout: func(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
		budget = d
		return context.WithCancel(ctx)
	}}
	request := httptest.NewRequest("GET", "/api/gameplay/overview", nil)
	request.Header.Set("Accept", "application/x-ndjson")
	a.handleGameplayOverview(httptest.NewRecorder(), request)
	if budget != 180*time.Second {
		t.Fatal("retry budget truncated", budget)
	}
}
func TestR220JPHistoryUsesMatchDetailsAndRegionScopedTags(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-r220-fixture")
	champs := newChampionProvider()
	var hosts, paths []string
	champs.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/ids") {
			return proHTTPBody([]byte(`["JP1_220"]`)), nil
		}
		return proHTTPBody([]byte(`{"metadata":{"matchId":"JP1_220"},"info":{"gameId":220,"gameDuration":1800,"gameMode":"CLASSIC","gameType":"MATCHED_GAME","mapId":11,"queueId":420,"participants":[{"participantId":1,"puuid":"r220-valid-account","teamId":100,"championId":268,"individualPosition":"MIDDLE","win":true},{"participantId":2,"puuid":"r220-other-account","teamId":200,"championId":69,"individualPosition":"MIDDLE","win":false}]}}`)), nil
	})}
	a := &app{riot: newRiotProvider(champs)}
	matches, _, err := a.loadClientRiotHistory(t.Context(), "jp1", "r220-valid-account", 0, 5, "all", nil, nil)
	if err != nil || len(matches) != 1 || matches[0].GameID != 220 {
		t.Fatal(matches, err)
	}
	if !reflect.DeepEqual(hosts, []string{"asia.api.riotgames.com", "asia.api.riotgames.com"}) || !strings.HasSuffix(paths[1], "JP1_220") {
		t.Fatal(hosts, paths)
	}
	if matchTagScope(matches[0]) != "jp1" {
		t.Fatal("JP match tags collided", matchTagScope(matches[0]))
	}
	if id, valid := normalizeExpectedGameID("JP1_220"); !valid || id != "220" {
		t.Fatal(id, valid)
	}
	if _, valid := normalizeExpectedGameID("ZZ1_220"); valid {
		t.Fatal("unknown platform accepted")
	}
}

func TestR220MatchTierDiskCacheIsPlatformScoped(t *testing.T) {
	a := &app{storage: &localStore{root: t.TempDir()}}
	a.writeMatchTierCache(220, &matchTiersResponse{Tier: "GOLD"}, "kr")
	a.writeMatchTierCache(220, &matchTiersResponse{Tier: "DIAMOND"}, "jp1")
	restarted := &app{storage: a.storage}
	for region, want := range map[string]string{"kr": "GOLD", "jp1": "DIAMOND"} {
		value, ok := restarted.readMatchTierCache(220, region)
		if !ok || value.Tier != want {
			t.Fatal(region, value, ok)
		}
	}
	if _, ok := restarted.readMatchTierCache(220, "na1"); ok {
		t.Fatal("cross-platform tier cache hit")
	}
}

func TestR220RiotFailureAttemptsKeepSpecificSafeReasons(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{{errRiotKeyMissing, "未配置"}, {errRiotRelayQuotaExhausted, "当日额度"}, {errRiotNotFound, "404"}, {context.DeadlineExceeded, "超时"}, {&riotStatusError{status: 429, message: "fixture"}, "429"}} {
		if message := riotHistoryFailureMessage(tc.err); !strings.Contains(message, tc.want) {
			t.Fatal(message, tc.want)
		}
	}
}
