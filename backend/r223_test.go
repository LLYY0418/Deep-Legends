package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestR223OptionalStatsAcrossRiotAndLCU(t *testing.T) {
	for _, value := range []string{`12`, `12.5`, `"12.5"`, `null`, `"broken"`, `{}`, `[]`, `1e100`, `-2`} {
		for _, field := range []string{"totalDamageTaken", "damageSelfMitigated", "totalHealsOnTeammates", "totalDamageShieldedOnTeammates", "timeCCingOthers", "damageDealtToBuildings", "turretTakedowns", "doubleKills", "tripleKills", "quadraKills", "pentaKills", "visionWardsBoughtInGame"} {
			data := []byte(fmt.Sprintf(`{"participantId":1,"kills":3,"%s":%s,"challenges":{"dragonTakedowns":%s,"baronTakedowns":%s,"riftHeraldTakedowns":%s}}`, field, value, value, value, value))
			var riot riotParticipant
			if err := json.Unmarshal(data, &riot); err != nil {
				t.Fatalf("%s=%s: %v", field, value, err)
			}
			var lcu lcuParticipant
			if err := json.Unmarshal([]byte(`{"participantId":1,"stats":`+string(data)+`}`), &lcu); err != nil {
				t.Fatalf("LCU %s=%s: %v", field, value, err)
			}
			game, err := decodeSGPHistoryGame([]byte(`{"gameId":1,"queueId":420,"participants":[` + string(data) + `]}`))
			if err != nil || len(game.Participants) != 1 {
				t.Fatalf("game dropped %s=%s: %v", field, value, err)
			}
			var lcuGame lcuGame
			if err := json.Unmarshal([]byte(`{"gameId":1,"participants":[{"participantId":1,"stats":`+string(data)+`}]}`), &lcuGame); err != nil || len(lcuGame.Participants) != 1 {
				t.Fatalf("LCU game dropped %s=%s: %v", field, value, err)
			}
			if riot.Kills != 3 || lcu.Stats.Kills != 3 {
				t.Fatal("core changed")
			}
			if value == `12.5` || value == `"12.5"` {
				if riot.Challenges.DragonTakedowns == nil || *historyIntValue(riot.Challenges.DragonTakedowns) != 13 {
					t.Fatal(riot.Challenges)
				}
			}
			lcuMatch := normalizeGameplayMatch(lcuGame, gameplayReference{}, nil, nil)
			if len(lcuMatch.Participants) != 1 {
				t.Fatal("LCU conversion dropped participant")
			}
			for _, statistic := range []*int{lcuMatch.Participants[0].DragonTakedowns, lcuMatch.Participants[0].BaronTakedowns, lcuMatch.Participants[0].RiftHeraldTakedowns} {
				switch value {
				case `12`, `12.5`, `"12.5"`:
					want := 13
					if value == `12` {
						want = 12
					}
					if statistic == nil || *statistic != want {
						t.Fatalf("LCU challenge %s: %v", value, statistic)
					}
				default:
					if statistic != nil {
						t.Fatalf("LCU invented challenge %s: %v", value, statistic)
					}
				}
			}
			if value == `null` || value == `"broken"` || value == `{}` || value == `[]` || value == `1e100` || value == `-2` {
				if historyIntValue(riot.Challenges.DragonTakedowns) != nil {
					t.Fatal("invented statistic")
				}
			}
		}
	}
	var riot riotParticipant
	if err := json.Unmarshal([]byte(`{"kills":"bad"}`), &riot); err == nil {
		t.Fatal("core corruption accepted")
	}
}

func TestR223DecodeFallbackAndPrivacy(t *testing.T) {
	data := []byte(`{"gameId":1,"queueId":420,"gameEndTimestamp":"changed","gameDuration":900,"participants":[{"participantId":1,"kills":2,"perks":"optional shape changed","puuid":"private-account"}]}`)
	game, err := decodeSGPHistoryGame(data)
	if err != nil || len(game.Participants) != 1 || game.Participants[0].Kills != 2 || game.GameDuration != 900 || !game.Participants[0].scoreMissing["damage"] {
		t.Fatal(game, err)
	}
	_, err = decodeSGPHistoryGame([]byte(`{"gameId":1,"queueId":420,"participants":[{"participantId":1,"kills":"private-value","puuid":"private-account"}]}`))
	if err == nil {
		t.Fatal("bad core survived fallback")
	}
	diagnostic := sgpGameDecodeDiagnostic(err)
	if diagnostic["field"] != "participants.kills" || diagnostic["value_type"] != "string" {
		t.Fatal(diagnostic)
	}
	diagnostic["value"] = "private-value"
	diagnostic["puuid"] = "private-account"
	encoded, _ := json.Marshal(allowSGPGameDecodeDiagnostic(diagnostic))
	if strings.Contains(string(encoded), "private") || len(allowSGPGameDecodeDiagnostic(diagnostic)) != 4 {
		t.Fatal(string(encoded))
	}
}

func TestR223SGPDoesNotRefillDecodedPage(t *testing.T) {
	for _, tags := range [][]string{nil, {"q_420"}} {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			games := make([]map[string]any, 20)
			for i := range games {
				participant := map[string]any{"puuid": strings.Repeat("p", 48), "participantId": 1, "kills": 3, "timeCCingOthers": 12.5, "challenges": map[string]any{"dragonTakedowns": "2.0"}}
				if i == 7 {
					participant["kills"] = "broken"
				}
				games[i] = map[string]any{"json": map[string]any{"gameId": i + 1, "queueId": 420, "participants": []any{participant}}}
			}
			json.NewEncoder(w).Encode(map[string]any{"games": games})
		}))
		client := &LCUClient{}
		provider := newSGPProvider()
		provider.http = server.Client()
		provider.serverBases["HN1"] = server.URL
		provider.token, provider.tokenAt, provider.tokenClient = "fixture", time.Now(), client
		observed := captureSGPObservations(provider)
		cost := &overviewLoadCost{}
		ctx := context.WithValue(t.Context(), overviewLoadCostContextKey{}, cost)
		games, consumed, more, err := provider.matchHistoryFilteredOn(ctx, client, "HN1", strings.Repeat("p", 48), 0, 20, tags, true)
		if err != nil || requests != 1 || len(games) != 19 || consumed != 20 || !more || cost.decodeFailedCount() != 1 {
			t.Fatal(len(games), consumed, more, requests, err, cost.decodeFailedCount())
		}
		failures := 0
		for _, event := range *observed {
			if event["event"] == "sgp_game_decode_failed" {
				failures++
				if event["field"] != "participants.kills" {
					t.Fatal(event)
				}
			}
		}
		if failures != 1 {
			t.Fatal(failures)
		}
		cost = &overviewLoadCost{}
		ctx = context.WithValue(t.Context(), overviewLoadCostContextKey{}, cost)
		provider.matchHistoryFilteredOn(ctx, client, "HN1", strings.Repeat("p", 48), 0, 20, tags, true)
		if requests != 1 || cost.decodeFailedCount() != 1 {
			t.Fatal("cache dropped decode evidence")
		}
		server.Close()
	}
}

func TestR223RegionAliasesAndAuthority(t *testing.T) {
	for _, tc := range [][2]string{{"JP", "jp1"}, {"KR", "kr"}, {"NA", "na1"}, {"EUW", "euw1"}, {"EUNE", "eun1"}, {"BR", "br1"}, {"LAN", "la1"}, {"LAS", "la2"}, {"OCE", "oc1"}, {"RU", "ru"}, {"TR", "tr1"}, {"ME", "me1"}, {"SG", "sg2"}, {"TW", "tw2"}, {"VN", "vn2"}, {"PH2", "sg2"}, {"TH2", "sg2"}} {
		c := newLCUClient(1, "fixture")
		c.applyPlatformArgs("--region=" + tc[0])
		c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { return proHTTPBody([]byte(`null`)), nil })}
		region, _ := clientRegionInfo(c)
		if region != tc[1] {
			t.Fatal(tc, region)
		}
	}
	for _, args := range []string{"", "--region=NA"} {
		c := newLCUClient(1, "fixture")
		c.applyPlatformArgs(args)
		c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.Contains(r.URL.Path, "LoginDataPacket") {
				return proHTTPBody([]byte(`"JP1"`)), nil
			}
			return proHTTPBody([]byte(`null`)), nil
		})}
		region, label := clientRegionInfo(c)
		if region != "jp1" || label != "日服" {
			t.Fatal(region, label)
		}
		if clientPlatformDiagnostic(c)["source"] != "login-data-packet" {
			t.Fatal(clientPlatformDiagnostic(c))
		}
	}
}

func TestR223UnknownPlatformRetriesAndPreservesEnum(t *testing.T) {
	c := newLCUClient(1, "fixture")
	c.applyPlatformArgs("--region=SECRET")
	ready := false
	requests := 0
	c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if ready && strings.Contains(r.URL.Path, "LoginDataPacket") {
			return proHTTPBody([]byte(`"JP1"`)), nil
		}
		return proHTTPBody([]byte(`null`)), nil
	})}
	region, _ := clientRegionInfo(c)
	if region != "" {
		t.Fatal(region)
	}
	first := requests
	event := clientPlatformDiagnostic(c)
	if event["region"] != "other" || event["has_region_arg"] != true || event["has_platform_arg"] != false {
		t.Fatal(event)
	}
	if requests != first {
		t.Fatal("backoff ignored")
	}
	ready = true
	c.retryPlatformAfterIdentityReady()
	region, _ = clientRegionInfo(c)
	if region != "jp1" {
		t.Fatal(region)
	}
	first = requests
	c.resolvePlatform(true)
	if requests != first {
		t.Fatal("resolved platform reprobed")
	}
}

func TestR223UnknownLCUHistoryRetries(t *testing.T) {
	c := newLCUClient(1, "fixture")
	requests := 0
	delays := []time.Duration{}
	c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "/matches") {
			return proHTTPBody([]byte(`null`)), nil
		}
		requests++
		if requests < 4 {
			response := proHTTPBody([]byte(`{}`))
			response.StatusCode = 503
			return response, nil
		}
		return proHTTPBody([]byte(`{}`)), nil
	})}
	c.historyRetrySleep = func(ctx context.Context, d time.Duration) error { delays = append(delays, d); return nil }
	var out any
	if err := getLCUHistoryWithRetry(t.Context(), c, "/matches", &out); err != nil || requests != 4 || fmt.Sprint(delays) != "[3s 6s 12s]" {
		t.Fatal(requests, delays, err)
	}
}

func TestR223CancelledLaunchStopsImmediately(t *testing.T) {
	calls := 0
	result, err := launchClientCandidates(clientInstallation{launchCandidates: []clientLaunchCandidate{{Source: "launcher"}, {Source: "tcls"}}}, func(clientLaunchCandidate) (clientLaunchFailure, error) {
		calls++
		return clientLaunchFailure{Stage: "shell-execute", ErrorCode: 1223}, errors.New("cancelled")
	})
	if err != nil || !result.Cancelled || calls != 1 || result.Source != "" {
		t.Fatal(result, calls, err)
	}
	a := &app{clientInstallations: func() []clientInstallation {
		return []clientInstallation{{ID: "tcls", Available: true, executable: "fixture"}}
	}, clientLauncher: func(clientInstallation) (clientLaunchResult, error) { return result, nil }}
	response := httptest.NewRecorder()
	a.handleClientLaunch(response, httptest.NewRequest("POST", "/api/client-launch", strings.NewReader(`{"id":"tcls"}`)))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"cancelled":true`) || !a.beginClientLaunch("tcls") {
		t.Fatal(response.Code, response.Body.String())
	}
}

func TestR223NoDirectLeagueCandidatesAndRiotPaths(t *testing.T) {
	root := t.TempDir()
	items := buildDetectedClientInstallations([]string{root}, nil, nil, func(string) bool { return true })
	for _, item := range items {
		for _, candidate := range item.candidates() {
			if strings.Contains(candidate.executable, "LeagueClient.exe") {
				t.Fatal(candidate)
			}
		}
	}
	candidates := riotCandidatesFromJSON([]byte(`{"rc_live":"D:/Riot Games/Riot Client/RiotClientServices.exe","rc_default":"C:/Riot Games/Riot Client/RiotClientServices.exe","other":"C:/Tencent/RiotClientServices.exe","nested":{"path":"E:/Riot Games/Riot Client/RiotClientServices.exe"}}`), []string{"F:/Riot Games/Riot Client/RiotClientServices.exe"})
	if len(candidates) != 4 || candidates[0].Source != "rc_default" || candidates[1].Source != "rc_live" || candidates[2].Source != "installs-json-other" || candidates[3].Source != "drive-guess" {
		t.Fatal(candidates)
	}
	if classifyRiotProductInstall([]byte("product_install_full_path: 'D:/WeGameApps/英雄联盟'"), func(string) bool { return true }) != "tencent" {
		t.Fatal("Tencent product accepted")
	}
	if classifyRiotProductInstall([]byte("product_install_full_path: 'C:/Riot Games/League of Legends'"), func(string) bool { return false }) != "missing" {
		t.Fatal("missing product accepted")
	}
}

// Synthetic contract fixture: it is deliberately not labelled a captured
// Tencent response. The Windows SUMMARY sample remains a separate acceptance.
func TestR223TwentyTenPlayerGamesKeepSeasonPeersAndCurves(t *testing.T) {
	subject := strings.Repeat("s", 48)
	page := []map[string]any{}
	for game := 1; game <= 20; game++ {
		participants := []map[string]any{}
		for i := 1; i <= 10; i++ {
			puuid := strings.Repeat(fmt.Sprintf("p%d", i), 24)
			if i == 1 {
				puuid = subject
			}
			participants = append(participants, map[string]any{"participantId": i, "puuid": puuid, "teamId": 100 + 100*((i-1)/5), "championId": 13, "teamPosition": []string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}[(i-1)%5], "kills": i, "deaths": 2, "assists": 4, "win": i <= 5, "totalDamageDealtToChampions": 10000, "totalDamageTaken": "12345.5", "goldEarned": 10000, "totalMinionsKilled": 100, "neutralMinionsKilled": 20, "visionScore": 12, "timeCCingOthers": 12.5, "challenges": map[string]any{"dragonTakedowns": "2.0", "baronTakedowns": 1.5}})
		}
		page = append(page, map[string]any{"json": map[string]any{"gameId": game, "queueId": 420, "mapId": 11, "gameMode": "CLASSIC", "gameDuration": 900, "gameCreation": seasonStartS26.Add(time.Hour).UnixMilli(), "participants": participants}})
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		json.NewEncoder(w).Encode(map[string]any{"games": page})
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "fixture", time.Now(), client
	games, consumed, _, err := provider.matchHistoryOn(t.Context(), client, "HN1", subject, 0, 20, false)
	if err != nil || len(games) != 20 || consumed != 20 || requests != 1 {
		t.Fatal(len(games), consumed, requests, err)
	}
	stats := map[int64]*gameplaySeasonChampionStat{}
	matches := []gameplayMatch{}
	for _, info := range games {
		if len(info.Participants) != 10 || info.Participants[0].TimeCCingOthers == nil || *historyIntValue(info.Participants[0].TimeCCingOthers) != 13 {
			t.Fatal("participant/stat dropped")
		}
		seasonStatsAccumulate(stats, nil, info, subject, seasonStartS26.UnixMilli())
		matches = append(matches, riotConvertMatch(&riotMatch{Info: *info}, subject, nil, nil))
	}
	rows := seasonStatsFinalize(stats, nil)
	if len(rows) != 1 || rows[0].Games != 20 {
		t.Fatal(rows)
	}
	if len(recentPlayers(matches, subject, seasonStartS26.UnixMilli())) == 0 {
		t.Fatal("peer history empty")
	}
	pf := map[string]json.RawMessage{}
	for i := 1; i <= 10; i++ {
		pf[fmt.Sprint(i)] = json.RawMessage(`{"totalGold":10000,"minionsKilled":100,"jungleMinionsKilled":20,"damageStats":{"totalDamageDoneToChampions":10000}}`)
	}
	curves := computeMatchTagsWithRules(matches[0], []timelineFrame{{Timestamp: 0, ParticipantFrames: pf}, {Timestamp: 900000, ParticipantFrames: pf}}, currentMatchKeywordModel)
	if len(curves) != 10 {
		t.Fatal("missing curve rows", len(curves))
	}
	for _, curve := range curves {
		if len(curve.Checkpoints) != 15 {
			t.Fatal("curve unavailable", curve)
		}
	}
}

func TestR223ChallengesWholeShapeIsOptional(t *testing.T) {
	for _, shape := range []string{`null`, `[]`, `[1,2]`, `"changed"`, `42`, `true`, `{}`} {
		game, err := decodeSGPHistoryGame([]byte(`{"gameId":1,"queueId":420,"participants":[{"participantId":1,"kills":3,"challenges":` + shape + `}]}`))
		if err != nil || len(game.Participants) != 1 || game.Participants[0].Kills != 3 || historyIntValue(game.Participants[0].Challenges.DragonTakedowns) != nil {
			t.Fatal(shape, game, err)
		}
		var lcu lcuGame
		if err := json.Unmarshal([]byte(`{"gameId":1,"participants":[{"participantId":1,"stats":{"kills":3,"challenges":`+shape+`}}]}`), &lcu); err != nil {
			t.Fatal(shape, err)
		}
		match := normalizeGameplayMatch(lcu, gameplayReference{}, nil, nil)
		if len(match.Participants) != 1 || match.Participants[0].Kills != 3 || match.Participants[0].DragonTakedowns != nil {
			t.Fatal("LCU challenges shape", shape, match)
		}
	}
}

func TestR223ForeignAndUnknownClientCannotUseExplicitTencentReferences(t *testing.T) {
	for _, region := range []string{"JP", ""} {
		client := &LCUClient{region: region, rsoPlatform: map[string]string{"JP": "JP1"}[region]}
		a := &app{lcu: client, gameplayRefs: map[string]string{}, gameplayRefDetails: map[string]gameplayReference{}, sgp: newSGPProvider(), mayhemRatings: newAramkitTestClient(aramkitRoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("foreign/unknown ARAMKit request")
			return nil, nil
		}))}
		ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: strings.Repeat("p", 48), GameName: "Fixture", TagLine: "CN", ServerID: "HN1"})
		response := httptest.NewRecorder()
		a.handleGameplayMayhemRating(response, httptest.NewRequest("GET", "/api/gameplay/mayhem-rating?playerRef="+ref, nil))
		if response.Code != 200 || strings.Contains(response.Body.String(), `"available":true`) {
			t.Fatal(response.Code, response.Body.String())
		}
		a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("foreign/unknown Tencent SGP request")
			return nil, nil
		})}
		_, progress, _, _ := a.loadSeasonChampionStatsWithHistoryCache(t.Context(), client, gameplayReference{ServerID: "HN1"}, Summoner{}, strings.Repeat("p", 48), nil, true)
		if !progress.Unavailable {
			t.Fatal(progress)
		}
		scan := &seasonScanState{}
		a.seasonScanPagesWithHistoryCache(t.Context(), client, "HN1", strings.Repeat("p", 48), scan, 1, false)
		if !scan.interrupted {
			t.Fatal("unknown season scan claimed completion")
		}
	}
}

func TestR223LaunchLocksArePerEntry(t *testing.T) {
	a := &app{}
	if !a.beginClientLaunch("tcls") || a.beginClientLaunch("tcls") || !a.beginClientLaunch("riot") {
		t.Fatal("entry lock leaked to alternative")
	}
	a.finishClientLaunch("tcls", false)
	if !a.beginClientLaunch("tcls") {
		t.Fatal("cancelled entry stayed locked")
	}
	a.finishClientLaunch("tcls", true)
	if a.beginClientLaunch("tcls") {
		t.Fatal("same entry cooldown lost")
	}
	a.finishClientLaunch("riot", false)
	if !a.beginClientLaunch("riot") {
		t.Fatal("cooldown leaked to alternative")
	}
}
