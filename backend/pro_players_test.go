package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func proFixtureAccount(name, puuid, tier string, division, lp int) opggProAccount {
	rank := json.RawMessage("null")
	if tier != "" {
		rank = json.RawMessage(fmt.Sprintf(`{"tier":%q,"division":%d,"lp":%d}`, tier, division, lp))
	}
	return opggProAccount{Region: "kr", PUUID: puuid, GameName: name, TagLine: "KR1", UpdatedAt: "2026-09-08T10:00:00+09:00", Rank: rank}
}
func proFixtureMember(team int, name, realName string, accounts ...opggProAccount) opggProMember {
	return opggProMember{TeamID: team, Nickname: name, RealName: realName, Authority: "PROGAMER", Position: "MID", Summoners: accounts}
}
func proFixtureHTML(region string, overrides ...opggProTeam) []byte {
	teams := map[int]opggProTeam{}
	for _, roster := range proRoster {
		teams[roster.OPGGID] = opggProTeam{ID: roster.OPGGID, Name: roster.Name, ShortName: roster.Code, Members: []opggProMember{}}
	}
	for _, team := range overrides {
		teams[team.ID] = team
	}
	var flight strings.Builder
	index := 1
	for _, team := range teams {
		data, _ := json.Marshal([]any{"$", "$L1", fmt.Sprint(team.ID), map[string]any{"region": region, "team": team}})
		fmt.Fprintf(&flight, "%x:%s\n", index, data)
		index++
	}
	// Split in the middle of JSON as real Next.js does.
	text := flight.String()
	split := len(text) / 2
	var page strings.Builder
	for _, part := range []string{text[:split], text[split:]} {
		value, _ := json.Marshal(part)
		fmt.Fprintf(&page, "<script>self.__next_f.push([1,%s])</script>", value)
	}
	return []byte(page.String())
}

func TestProDirectoryParsesAllFlightRecordsAndRequiresKR(t *testing.T) {
	team := opggProTeam{ID: 632, Name: "Bilibili Gaming", ShortName: "BLG", Members: []opggProMember{proFixtureMember(632, "knight", "Zhuo Ding (卓定)", proFixtureAccount("one", "puuid1", "CHALLENGER", 1, 1000))}}
	rows, err := parseOPGGProPlayers(proFixtureHTML("kr", team))
	if err != nil || len(rows) != 6 {
		t.Fatalf("teams=%d err=%v", len(rows), err)
	}
	result := (&app{token: "test"}).buildProPlayers(rows, proRoster)
	if result.PlayerCount != 33 || result.AccountCount != 1 || result.MissingCount != 32 {
		t.Fatalf("unexpected counts: players=%d accounts=%d missing=%d", result.PlayerCount, result.AccountCount, result.MissingCount)
	}
	for _, body := range [][]byte{nil, []byte("<html>challenge</html>"), proFixtureHTML("na", team), bytes.Repeat([]byte("x"), proPlayersMaxBytes+1)} {
		if _, err := parseOPGGProPlayers(body); err == nil {
			t.Fatal("invalid/NA directory accepted")
		}
	}
}

func TestProRosterIdentityMergeExcludesAcademyCoachesHistoricalAndCollisions(t *testing.T) {
	roster := []proRosterTeam{{Code: "BLG", OPGGID: 632, Players: []proRosterPlayer{{Name: "knight", Position: "middle", Names: []string{"Zhuo Ding", "卓定"}}, {Name: "ON", Position: "utility", Names: []string{"Luo Wen-Jun", "骆文俊"}}, {Name: "Missing", Position: "top", Names: []string{"No Data"}}}}, {Code: "IG", OPGGID: 371, Players: []proRosterPlayer{{AllowTeams: []int{858}, Name: "Rookie", Position: "middle", Names: []string{"Song Eui-jin", "송의진"}}}}}
	one := proFixtureAccount("old name", "stable-one", "MASTER", 1, 200)
	newer := proFixtureAccount("BLG 온", "stable-one", "CHALLENGER", 1, 700)
	newer.UpdatedAt = "2026-09-08T12:00:00+09:00"
	conflict := proFixtureAccount("conflict", "shared", "GOLD", 1, 10)
	coachAuthority := proFixtureMember(632, "knight", "Zhuo Ding", proFixtureAccount("coach1", "coach1", "", 0, 0))
	coachAuthority.Authority = "COACH"
	coachPosition := proFixtureMember(632, "knight", "Zhuo Ding", proFixtureAccount("coach2", "coach2", "", 0, 0))
	coachPosition.Position = "COACH"
	source := []opggProTeam{
		{ID: 632, Name: "Bilibili Gaming", Members: []opggProMember{
			proFixtureMember(632, "knight", "Zhuo Ding (卓定)", one, conflict),
			proFixtureMember(632, "Knight", "Zhuo Ding (卓定)", newer, proFixtureAccount("other", "two", "DIAMOND", 2, 99)),
			proFixtureMember(632, "knight", "Different Person", proFixtureAccount("impostor", "bad", "CHALLENGER", 1, 2000)),
			proFixtureMember(632, "ON", "Luo Wen-Jun (骆文俊)", conflict, proFixtureAccount("support", "support", "GOLD", 1, 10)),
			proFixtureMember(632, "Biubiu", "Yu Lei-Xin", proFixtureAccount("former", "former", "", 0, 0)), coachAuthority, coachPosition}},
		{ID: 101, Name: "Bilibili Gaming Junior", Members: []opggProMember{proFixtureMember(101, "Knight", "Zhuo Ding", proFixtureAccount("academy", "academy", "CHALLENGER", 1, 3000))}},
		{ID: 858, Name: "Ninjas in Pyjamas", Members: []opggProMember{proFixtureMember(858, "Rookie", "Song Eui-jin (송의진)", proFixtureAccount("rookie", "rookie", "MASTER", 1, 900))}},
	}
	a := &app{token: "private-session"}
	result := a.buildProPlayers(source, roster)
	knight := result.Teams[0].Players[0]
	if len(knight.Accounts) != 2 || knight.Accounts[0].GameName != "BLG 온" || knight.Status != "partial" {
		t.Fatalf("bad knight merge: %+v", knight)
	}
	if len(result.Teams[0].Players[1].Accounts) != 1 || result.Teams[0].Players[1].Accounts[0].GameName != "support" {
		t.Fatal("account nickname incorrectly interpreted as owner")
	}
	if result.Teams[0].Players[2].Status != "missing" || result.Teams[1].Players[0].Accounts[0].GameName != "rookie" {
		t.Fatal("missing player or historical team reconciliation lost")
	}
	encoded, _ := json.Marshal(result)
	for _, secret := range []string{"stable-one", "puuid", "acct_id", "summoner_id", "private-session", "playerRef", "academy", "impostor", "former", "coach1", "coach2"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("excluded/private data leaked: %s", secret)
		}
	}
}

func TestProAccountRankingAndMissingRank(t *testing.T) {
	cases := []struct {
		tier         string
		division, lp int
		want         string
	}{
		{"CHALLENGER", 1, 500, "ranked"}, {"MASTER", 0, 1900, "ranked"}, {"DIAMOND", 1, 0, "ranked"}, {"DIAMOND", 2, 99, "ranked"}, {"EMERALD", 1, 100, "ranked"}, {"UNKNOWN", 1, 40, "unavailable"}, {"DIAMOND", 0, 0, "unavailable"}, {"CHALLENGER", 1, -1, "unavailable"}, {"", 0, 0, "unavailable"}, {"UNRANKED", 0, 0, "unranked"},
	}
	accounts := []proAccount{}
	for i, tc := range cases {
		raw := proFixtureAccount(fmt.Sprint(i), "id", tc.tier, tc.division, tc.lp)
		account, valid := normalizeProAccount(raw)
		if !valid || account.RankStatus != tc.want {
			t.Fatalf("%+v => %+v valid=%v", tc, account, valid)
		}
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool { return proAccountLess(accounts[i], accounts[j]) })
	for i, name := range []string{"0", "1", "2", "3", "4", "9"} {
		if accounts[i].GameName != name {
			t.Fatalf("unexpected order: %+v", accounts)
		}
	}
	raw := proFixtureAccount("NA", "na", "CHALLENGER", 1, 999)
	raw.Region = "na"
	if _, valid := normalizeProAccount(raw); valid {
		t.Fatal("NA account accepted")
	}
	raw.Region = "kr"
	raw.Rank = nil
	account, valid := normalizeProAccount(raw)
	if !valid || account.RankStatus != "unavailable" {
		t.Fatal("missing rank reported as unranked")
	}
	raw = proFixtureAccount("ladder", "ladder", "MASTER", 1, 500)
	raw.LadderRank, raw.LadderRankKnown = 3714, true
	account, valid = normalizeProAccount(raw)
	if !valid || !account.LadderRankKnown || account.LadderRank != 3714 {
		t.Fatalf("ladder rank was not preserved: %+v", account)
	}
}

func TestParseOPGGLadderRankUsesRequestedHighlightedRow(t *testing.T) {
	body := []byte(`<table><tbody><tr id="other-KR1"><td>1</td></tr><tr id="빈 스토리-KR1" class="highlight"><td class="text-gray-400">3,714</td><td>빈 스토리#KR1</td></tr></tbody></table>`)
	rank, err := parseOPGGLadderRank(body, "빈 스토리", "KR1")
	if err != nil || rank != 3714 {
		t.Fatalf("rank=%d err=%v", rank, err)
	}
	for _, invalid := range [][]byte{
		nil,
		[]byte(`<tr id="other-KR1"><td>3714</td></tr>`),
		[]byte(`<tr id="빈 스토리-KR1"><td>unknown</td></tr>`),
		[]byte(`<tr id="빈 스토리-KR1"><td>0</td></tr>`),
		bytes.Repeat([]byte("x"), proLadderMaxBytes+1),
	} {
		if _, err := parseOPGGLadderRank(invalid, "빈 스토리", "KR1"); err == nil {
			t.Fatal("invalid ladder response accepted")
		}
	}
}

func TestFetchAndEnrichProLadderRankUsesFixedKRLeaderboard(t *testing.T) {
	var calls atomic.Int32
	provider := newChampionProvider()
	provider.clientMu.Lock()
	provider.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if !validProLadderURL(request.URL) || request.URL.Query().Get("summoner") != "fixtureaccount-KR1" {
			t.Fatalf("unexpected ladder request: %s", request.URL)
		}
		if request.Header.Get("Accept") != "text/html,application/xhtml+xml" || request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" || request.Header.Get("X-Riot-Token") != "" {
			t.Fatalf("unexpected ladder headers: %v", request.Header)
		}
		return proHTTPBody([]byte(`<table><tr id="fixtureaccount-KR1"><td>42</td><td>fixtureaccount</td></tr></table>`)), nil
	})}
	provider.clientMu.Unlock()

	account := proFixtureAccount("fixtureaccount", "fixture", "CHALLENGER", 1, 1200)
	source := []opggProTeam{{ID: 632, Name: "Bilibili Gaming", Members: []opggProMember{proFixtureMember(632, "Bin", "Chen Ze-Bin (陈泽彬)", account)}}}
	enrichProLadderRanks(context.Background(), provider, source)
	result := new(app).buildProPlayers(source, proRoster)
	got := result.Teams[0].Players[0].Accounts
	if calls.Load() != 1 || len(got) != 1 || !got[0].LadderRankKnown || got[0].LadderRank != 42 || result.LadderRankPartial {
		t.Fatalf("calls=%d account=%+v partial=%v", calls.Load(), got, result.LadderRankPartial)
	}
	if _, err := fetchOPGGLadderRank(context.Background(), provider, "bad#name", "KR1"); err == nil {
		t.Fatal("invalid Riot ID accepted")
	}
}

func newProMockApp(t *testing.T, fetch func(*http.Request) (*http.Response, error)) *app {
	t.Helper()
	p := newChampionProvider()
	// P2-2（R120）：loadProPlayers 发布目录之后还会用后台 goroutine 继续补种子与阶梯，
	// 这些 goroutine 会活过测试结束再回调这个 transport。此时任何 t.Log/t.Error 都会
	// 触发 "Log in goroutine after test has completed" panic，把整个测试包打红，而失败
	// 会被记在当时正在跑的另一个测试头上——这就是「单跑 40/40 通过、整包负载下偶发
	// 红」的形状。所以测试结束后只记录事实、不再动 t。
	var finished atomic.Bool
	t.Cleanup(func() { finished.Store(true) })
	p.clientMu.Lock()
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == proSupplementHost {
			name := strings.TrimPrefix(r.URL.Path, "/player/")
			_, player, ok := proSupplementPlayer(name)
			if !ok {
				if !finished.Load() {
					t.Errorf("unreviewed supplement: %s", name)
				}
				// 未复核的名字没有可用的 Names[0]，继续走下去会在测试结束后 panic。
				return nil, errors.New("unreviewed pro identity page")
			}
			return proHTTPBody([]byte(fmt.Sprintf("<h1>%s</h1><table><tr><td>Name</td><td>%s</td></tr></table><div><h4>Accounts</h4><table></table></div>", player.Name, player.Names[0]))), nil
		}
		return fetch(r)
	})}
	p.clientMu.Unlock()
	return &app{token: "test-session", champions: p}
}
func proHTTPBody(body []byte) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), ContentLength: int64(len(body))}
}

func TestProCacheSingleflightRefreshBackoffAndStale(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	a := newProMockApp(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "op.gg" && strings.HasPrefix(r.URL.Path, "/zh-cn/lol/summoners/kr/") {
			return proHTTPBody([]byte("<html>profile unavailable</html>")), nil
		}
		calls.Add(1)
		if r.URL.Host != opggPageHost || r.URL.Path != proPlayersPath || r.URL.Query().Get("region") != "kr" {
			t.Errorf("unexpected upstream: %s", r.URL)
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Riot-Token") != "" {
			t.Error("private credentials sent")
		}
		if fail.Load() {
			return nil, errors.New("private upstream error")
		}
		time.Sleep(15 * time.Millisecond)
		return proHTTPBody(proFixtureHTML("kr")), nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			teams, _, err := a.loadProPlayers(context.Background(), true)
			if err != nil || len(teams) != 42 {
				t.Errorf("load=%d err=%v", len(teams), err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("expected one load, got %d", calls.Load())
	}
	_, _, _ = a.loadProPlayers(context.Background(), true)
	if calls.Load() != 1 {
		t.Fatal("refresh bypasses cooldown")
	}
	waitProEnrichment(t, a)
	fail.Store(true)
	a.proPlayers.mu.Lock()
	a.proPlayers.attemptedAt = time.Now().Add(-proPlayersRetry - time.Second)
	a.proPlayers.fetchedAt = time.Now().Add(-proPlayersTTL - time.Minute)
	a.proPlayers.mu.Unlock()
	teams, at, err := a.loadProPlayers(context.Background(), false)
	if err == nil || len(teams) != 42 || time.Since(at) < proPlayersTTL {
		t.Fatal("failed load didn't retain explicitly stale data")
	}
	w := httptest.NewRecorder()
	a.handleProPlayers(w, httptest.NewRequest("GET", "/api/pro-players", nil))
	var response proPlayersResponse
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	if w.Code != 200 || !response.Stale || response.Unavailable || strings.Contains(w.Body.String(), "private upstream error") {
		t.Fatalf("bad stale response: %s", w.Body.String())
	}
	a.proPlayers.mu.Lock()
	a.proPlayers.fetchedAt = time.Now().Add(-proPlayersMaxStale - time.Hour)
	a.proPlayers.mu.Unlock()
	teams, _, _ = a.loadProPlayers(context.Background(), false)
	if len(teams) != 0 {
		t.Fatal("unbounded stale accounts")
	}
	if calls.Load() != 2 {
		t.Fatal("failed requests did not back off")
	}
}

func waitProEnrichment(t *testing.T, a *app) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.proPlayers.mu.Lock()
		updating := a.proPlayers.updating
		a.proPlayers.mu.Unlock()
		if !updating {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("background directory enrichment did not finish")
}

func TestProDirectoryPublishesBeforeSlowSupplements(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	a := &app{champions: newChampionProvider()}
	// P2-2（R120）：后台补种子/阶梯用的是 a.proRefreshContext（生产里是 runtimeContext），
	// 而且刻意不随调用方 ctx 取消（「一个调用方离开页面不能取消别人共享的加载」）。
	// 测试必须自己给它一个可取消的 ctx 并在结束时取消，否则这些 goroutine 会带着
	// 90 秒预算活到后面的测试里去，把失败记到别的测试头上。
	refreshCtx, cancelRefresh := context.WithCancel(context.Background())
	defer cancelRefresh()
	a.proRefreshContext = refreshCtx
	var directoryCalls, supplementCalls, otherCalls, supplementDone atomic.Int32
	a.champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == proSupplementHost {
			supplementCalls.Add(1)
			select {
			case <-release:
			case <-r.Context().Done():
			}
			// P1-1（R120 复测）：完成计数必须在 return 之前落定。补充源 handler 自己
			// 带 12 秒超时 × 2 次重试（pro_players_supplement.go:103），「release 不关
			// 它就永远不返回」不成立——生产代码若改成发布前等补充源，约 24 秒后这里
			// 会经由 r.Context().Done() 返回并把计数抬上去，下面的断言就能抓到。
			supplementDone.Add(1)
			return nil, errors.New("supplement unavailable")
		}
		// 只数「目录」这一个请求。原来任何非补充源请求都被计成一次目录请求，于是
		// 后台补种子/阶梯打到 op.gg 的 /zh-cn/lol/summoners/... 也算进去，断言就变成
		// 「后台请求有没有恰好在三次轮询之间落地」的时序赌博：GOMAXPROCS=1 下
		// -race -count=100 里 27 次假红，报的全是 poll refetched directory。
		if r.URL.Path == proPlayersPath {
			directoryCalls.Add(1)
		} else {
			otherCalls.Add(1)
		}
		return proHTTPBody(proFixtureHTML("kr")), nil
	})}
	// 原来这里是 1 秒墙钟预算。要证明的是「目录先发布、不等补充源」，与墙钟无关；
	// 1 秒在整包 -race 并行下会被调度噪声吃掉，变成随机红。改成不设墙钟的 cancel ctx，
	// 外加一个只把「挂死」翻译成可读失败的看门狗：取消之后 loadProPlayers 只能返回
	// ctx 错误，下面的断言必然失败，所以看门狗不可能把挂死伪装成通过。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watchdog := time.AfterFunc(60*time.Second, cancel)
	defer watchdog.Stop()
	// 同步点（P1-1 修正版）：release 在测试体内不关闭，补充源 handler 只能阻塞在
	// select 上；它自带的 12s×2 超时只会让 handler「返回失败」，不会让测试体里的
	// 任何断言失效。所以「loadProPlayers 已经返回、而 supplementDone 仍是 0」就是
	// 一个不依赖墙钟的证明：发布没有等过任何一次补充源请求的完成。
	teams, _, err := a.loadProPlayers(ctx, false)
	if err != nil || len(teams) != 42 {
		t.Fatalf("directory waited for supplement: %v (teams=%d)", err, len(teams))
	}
	// 只要发布路径等过补充源（哪怕等到它自己超时），这里必然非 0。
	if done := supplementDone.Load(); done != 0 {
		t.Fatalf("directory returned only after %d supplement requests had finished（发布等过补充源）", done)
	}
	a.proPlayers.mu.Lock()
	updating := a.proPlayers.updating
	a.proPlayers.mu.Unlock()
	if !updating {
		t.Fatal("no progressive update marker")
	}
	for i := 0; i < 3; i++ {
		if _, _, err := a.loadProPlayers(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	if got := directoryCalls.Load(); got != 1 {
		t.Fatalf("poll refetched directory: %d 次目录请求（补充源 %d 次、其它后台请求 %d 次）", got, supplementCalls.Load(), otherCalls.Load())
	}
	t.Logf("P2-2 精确计数：目录 %d 次、补充源 %d 次、后台补种子/阶梯 %d 次", directoryCalls.Load(), supplementCalls.Load(), otherCalls.Load())
}

func TestProDirectoryFailureRetainsRosterAndAuth(t *testing.T) {
	a := newProMockApp(t, func(*http.Request) (*http.Response, error) { return proHTTPBody([]byte("not a directory")), nil })
	w := httptest.NewRecorder()
	a.handleProPlayers(w, httptest.NewRequest("GET", "/api/pro-players", nil))
	var result proPlayersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Unavailable || result.PlayerCount != 33 || result.AccountCount != 53 || len(result.Teams) != 6 || result.Teams[0].Players[0].Status != "available" {
		t.Fatal("failed source erased roster")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private response browser-cached")
	}
	for _, query := range []string{"?region=na", "?refresh=2", "?refresh=1&refresh=1", "?url=https://evil.invalid"} {
		w = httptest.NewRecorder()
		a.handleProPlayers(w, httptest.NewRequest("GET", "/api/pro-players"+query, nil))
		if w.Code != 400 {
			t.Fatalf("query %s accepted", query)
		}
	}
	w = httptest.NewRecorder()
	a.authorized(a.handleProPlayers)(w, httptest.NewRequest("GET", "/api/pro-players", nil))
	if w.Code != 401 {
		t.Fatal("unauthenticated directory accepted")
	}
}

// Opt-in local verification uses the same parser + normalization as production;
// never check real stable player IDs into the repository or test output.
func TestProCapturedDirectory(t *testing.T) {
	file := os.Getenv("PRO_PLAYERS_CAPTURE")
	if file == "" {
		t.Skip("set PRO_PLAYERS_CAPTURE to a locally captured OP.GG KR HTML file")
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	source, err := parseOPGGProPlayers(body)
	if err != nil {
		t.Fatal(err)
	}
	result := (&app{token: "capture-session"}).buildProPlayers(source, proRoster)
	for _, team := range result.Teams {
		for _, player := range team.Players {
			t.Logf("%s %s: %d accounts (%s)", team.Code, player.Name, len(player.Accounts), player.Status)
		}
	}
	t.Logf("%d teams, %d roster players, %d accounts, %d without accounts", len(result.Teams), result.PlayerCount, result.AccountCount, result.MissingCount)
	if len(result.Teams) != 6 || result.AccountCount < 40 {
		t.Fatal("unexpectedly incomplete live source")
	}
	if out := os.Getenv("PRO_PLAYERS_PUBLIC_OUTPUT"); out != "" && os.Getenv("PRO_PLAYERS_SUPPLEMENT_CAPTURE") == "" {
		writeProPublicCapture(t, result, out)
	}
}

func TestProCapturedLadder(t *testing.T) {
	file := os.Getenv("PRO_LADDER_CAPTURE")
	gameName := os.Getenv("PRO_LADDER_GAME_NAME")
	tagLine := os.Getenv("PRO_LADDER_TAG_LINE")
	if file == "" || gameName == "" || tagLine == "" {
		t.Skip("set PRO_LADDER_CAPTURE, PRO_LADDER_GAME_NAME and PRO_LADDER_TAG_LINE to a local OP.GG leaderboard capture")
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	rank, err := parseOPGGLadderRank(body, gameName, tagLine)
	if err != nil || rank <= 0 {
		t.Fatalf("live ladder parser failed: rank=%d err=%v", rank, err)
	}
	t.Log("live ladder rank parsed")
}

func writeProPublicCapture(t *testing.T, result proPlayersResponse, out string) {
	t.Helper()
	result.RosterVerifiedAt = proRosterVerifiedAt
	result.FetchedAt = time.Now()
	data, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(out, data, 0600); err != nil {
		t.Fatal(err)
	}
}

// Real Flight shape: account.region may be null, absent or empty while the
// containing team's directory region remains explicitly KR.
func proR97RegionHTML(t *testing.T, region any, present bool, overrides ...opggProTeam) []byte {
	t.Helper()
	teams := map[int]opggProTeam{}
	for _, team := range proRoster {
		teams[team.OPGGID] = opggProTeam{ID: team.OPGGID, Name: team.Name, ShortName: team.Code, Members: []opggProMember{}}
	}
	for _, team := range overrides {
		teams[team.ID] = team
	}
	var flight strings.Builder
	for id, team := range teams {
		raw, err := json.Marshal(team)
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatal(err)
		}
		for _, member := range object["members"].([]any) {
			for _, account := range member.(map[string]any)["summoners"].([]any) {
				fields := account.(map[string]any)
				delete(fields, "region")
				if present {
					fields["region"] = region
				}
			}
		}
		record, err := json.Marshal([]any{"$", "$L1", fmt.Sprint(id), map[string]any{"region": "kr", "team": object}})
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&flight, "%x:%s\n", id, record)
	}
	// Split the Flight stream as the actual Next.js transport does.
	stream := flight.String()
	var html strings.Builder
	for _, part := range []string{stream[:len(stream)/2], stream[len(stream)/2:]} {
		quoted, _ := json.Marshal(part)
		fmt.Fprintf(&html, "<script>self.__next_f.push([1,%s])</script>", quoted)
	}
	return []byte(html.String())
}

func TestR97ProAccountOptionalRegionSurvivesKRDirectory(t *testing.T) {
	for _, tc := range []struct {
		name              string
		region            any
		present, accepted bool
	}{
		{"null", nil, true, true}, {"missing", nil, false, true}, {"empty", "", true, true},
		{"whitespace", "  ", true, true}, {"KR", " KR ", true, true}, {"non-KR", "na", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			team := opggProTeam{ID: 385, Name: "T1", ShortName: "T1", Members: []opggProMember{}}
			for _, p := range proRoster[2].Players {
				team.Members = append(team.Members, proFixtureMember(team.ID, p.Name, p.Names[0], proFixtureAccount("R97"+p.Name, "private-"+p.Name, "CHALLENGER", 1, 1000)))
			}
			teams, err := parseOPGGProPlayers(proR97RegionHTML(t, tc.region, tc.present, team))
			if err != nil {
				t.Fatal(err)
			}
			a := &app{}
			result := a.buildProPlayers(teams, proRoster)
			a.proPlayers.teams = teams
			index := a.proIdentitySnapshot()
			want := 0
			if tc.accepted {
				want = 5
			}
			if result.AccountCount != want {
				t.Fatalf("T1 accounts cleared: got %d want %d", result.AccountCount, want)
			}
			for _, player := range result.Teams[2].Players {
				badge := a.matchProIdentity(index, "test", gameplayReference{Region: "kr", GameName: "R97" + player.Name, TagLine: "KR1"})
				if tc.accepted {
					if player.Status != "available" || len(player.Accounts) != 1 || badge == nil || badge.TeamCode != "T1" || badge.PlayerName != player.Name {
						t.Fatalf("optional region regressed: player=%+v badge=%+v", player, badge)
					}
				} else if len(player.Accounts) != 0 || badge != nil {
					t.Fatal("explicit foreign region accepted")
				}
			}
		})
	}
}

func TestR97ManagementAndBadgesShareReviewedSixTeams(t *testing.T) {
	var source []opggProTeam
	for _, team := range proRoster {
		p := team.Players[0]
		source = append(source, opggProTeam{ID: team.OPGGID, Name: team.Name, ShortName: team.Code, Members: []opggProMember{proFixtureMember(team.OPGGID, p.Name, p.Names[0], proFixtureAccount("R97"+p.Name, "private-"+p.Name, "CHALLENGER", 1, 1000))}})
	}
	for i, name := range []string{"Winners", "Young Miracles", "Machi Esports", "Suning Gaming-S", "Anyone's Legend.Young", "Suning", "VSG", "T1 Academy"} {
		id := 9900 + i
		source = append(source, opggProTeam{ID: id, Name: name, ShortName: name, Members: []opggProMember{proFixtureMember(id, "Extra", "Extra", proFixtureAccount(name, "private-extra-"+name, "CHALLENGER", 1, 1))}})
	}
	// An unreviewed member even on an approved team must not widen the roster.
	source[2].Members = append(source[2].Members, proFixtureMember(385, "Painter", "Unreviewed", proFixtureAccount("OutsideRoster", "private-outside", "", 0, 0)))
	// Reviewed historical team exceptions retain the current IG affiliation.
	source = append(source, opggProTeam{ID: 858, Name: "NIP", ShortName: "NIP", Members: []opggProMember{proFixtureMember(858, "Rookie", "Song Eui-jin", proFixtureAccount("R97Rookie", "private-rookie", "CHALLENGER", 1, 1000))}})
	teams, err := parseOPGGProPlayers(proR97RegionHTML(t, nil, true, source...))
	if err != nil {
		t.Fatal(err)
	}
	a := &app{}
	a.proPlayers.teams, a.proPlayers.fetchedAt, a.proPlayers.attemptedAt = teams, time.Now(), time.Now()
	w := httptest.NewRecorder()
	a.handleProPlayers(w, httptest.NewRequest(http.MethodGet, "/api/pro-players", nil))
	if w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	var data proPlayersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	codes := []string{"BLG", "IG", "T1", "HLE", "GEN", "DK"}
	if len(data.Teams) != 6 || data.PlayerCount != 33 || data.AccountCount != 53 {
		t.Fatalf("scope widened: %+v", data)
	}
	index := a.proIdentitySnapshot()
	for i, team := range data.Teams {
		if team.Code != codes[i] || team.Secondary {
			t.Fatal("unexpected team", team)
		}
		// R105 fixes page membership to the 53 reviewed accounts. The badge
		// index still validates the seven independent upstream fixture rows.
		for _, p := range a.buildProPlayers(teams, proRoster).Teams[i].Players {
			for _, account := range p.Accounts {
				badge := a.matchProIdentity(index, "management", gameplayReference{Region: "kr", GameName: account.GameName, TagLine: account.TagLine})
				if badge == nil || badge.TeamCode != team.Code || badge.PlayerName != p.Name || badge.Secondary {
					t.Fatal("management/badge scope drift", p, badge)
				}
			}
		}
	}
	if index.candidates != 7 {
		t.Fatal("badge inputs exceed whitelist", index.candidates)
	}
	for _, team := range source[6 : len(source)-1] {
		account := team.Members[0].Summoners[0]
		for _, surface := range []string{"match", "live", "overview", "current-game"} {
			if a.matchProIdentity(index, surface, gameplayReference{Region: "kr", GameName: account.GameName, TagLine: account.TagLine, PlayerRef: account.PUUID}) != nil {
				t.Fatal("expanded team badge", team.Name, surface)
			}
		}
	}
	if a.matchProIdentity(index, "overview", gameplayReference{Region: "kr", GameName: "OutsideRoster", TagLine: "KR1"}) != nil {
		t.Fatal("unreviewed player accepted")
	}
	if len(proDirectoryRoster(teams, proRoster)) <= 6 {
		t.Fatal("generic expansion infrastructure was removed")
	}
	for _, account := range proRankedLadderAccounts(teams) {
		if !strings.HasPrefix(account.GameName, "R97") {
			t.Fatal("ladder scope widened", account)
		}
	}
}
