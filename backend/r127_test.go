package main

// R127 P0：把诊断盲区补上。
//
// 之前 local_request_client 只接受 reason=complete，endpoint 只认
// status/gameplay/champions/collection/other，而图片队列、好友、职业选手、
// section-loader 上报失败用的都是 reason=failed + endpoint=image/friends/…，
// 于是每一次图片失败、每一次排队卡住都被 client_diagnostic_rejected 拒收，
// 日志里只剩「后端很快」，真正的瓶颈从来没被测到。
//
// 下面每条都是「上报 → 落日志」的端到端断言。
//
// 对抗变异：把 "failed" 从 clientDiagnosticEvents["local_request_client"] 里删掉，
// TestR127ImageFailureDiagnosticReachesLog 与
// TestR127ClientDiagnosticAcceptsNewFailureEndpoints 必须 FAIL（状态码变 400，
// 日志里只剩 client_diagnostic_rejected）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newR127DiagnosticApp(t *testing.T) *app {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	instance := &app{storage: trackTestStore(t, &localStore{root: root})}
	t.Cleanup(func() { _ = instance.storage.Close() })
	return instance
}

func r127PostDiagnostic(t *testing.T, instance *app, body string) int {
	t.Helper()
	recorder := httptest.NewRecorder()
	instance.handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(body)))
	return recorder.Code
}

func r127DiagnosticEvents(t *testing.T, instance *app) []map[string]any {
	t.Helper()
	data, err := instance.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("diagnostic line is not JSON: %s", line)
		}
		events = append(events, event)
	}
	return events
}

func r127AssertNoRejection(t *testing.T, events []map[string]any) {
	t.Helper()
	for _, event := range events {
		if event["event"] == "client_diagnostic_rejected" {
			t.Fatalf("白名单仍然拒收了这条上报: %#v", event)
		}
	}
}

func TestR127ImageFailureDiagnosticReachesLog(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	body := `{"event":"local_request_client","reason":"failed","endpoint":"image","httpStatus":0,` +
		`"errorKind":"timeout","queueWaitMs":4210,"loadMs":8001,"activeSlowCount":2,"imageSource":"communitydragon"}`
	if code := r127PostDiagnostic(t, instance, body); code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204：图片失败诊断仍被拒收", code)
	}
	events := r127DiagnosticEvents(t, instance)
	r127AssertNoRejection(t, events)
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	event := events[0]
	if event["event"] != "local_request_client" || event["reason"] != "failed" || event["endpoint"] != "image" {
		t.Fatalf("event = %#v", event)
	}
	for key, want := range map[string]float64{"queue_wait_ms": 4210, "load_ms": 8001, "active_slow_count": 2} {
		if event[key] != want {
			t.Fatalf("%s = %#v, want %v（event=%#v）", key, event[key], want, event)
		}
	}
	if event["image_source"] != "communitydragon" {
		t.Fatalf("image_source = %#v", event["image_source"])
	}
	if event["error_kind"] != "timeout" {
		t.Fatalf("error_kind = %#v", event["error_kind"])
	}
}

func TestR127ClientDiagnosticAcceptsNewFailureEndpoints(t *testing.T) {
	for _, endpoint := range []string{"image", "friends", "pro-players", "section-loader"} {
		instance := newR127DiagnosticApp(t)
		body := `{"event":"local_request_client","reason":"failed","endpoint":"` + endpoint + `","httpStatus":0,"errorKind":"network"}`
		if code := r127PostDiagnostic(t, instance, body); code != http.StatusNoContent {
			t.Fatalf("endpoint=%s status = %d, want 204", endpoint, code)
		}
		events := r127DiagnosticEvents(t, instance)
		r127AssertNoRejection(t, events)
		if len(events) != 1 {
			t.Fatalf("endpoint=%s events = %#v", endpoint, events)
		}
		if events[0]["endpoint"] != endpoint || events[0]["reason"] != "failed" {
			t.Fatalf("endpoint=%s event = %#v", endpoint, events[0])
		}
		if endpoint != "image" {
			// 排队计时只属于图片队列，其它端点不该凭空多出这些字段。
			for _, key := range []string{"queue_wait_ms", "load_ms", "active_slow_count", "image_source"} {
				if _, ok := events[0][key]; ok {
					t.Fatalf("endpoint=%s 多了图片字段 %s: %#v", endpoint, key, events[0])
				}
			}
		}
	}
}

func TestR127MatchTierBatchReportsDuration(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	body := `{"event":"match_tiers_overview_batch","reason":"complete","totalRefs":36,"uniqueRefs":24,"cacheHits":19,"durationMs":2431}`
	if code := r127PostDiagnostic(t, instance, body); code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", code)
	}
	events := r127DiagnosticEvents(t, instance)
	r127AssertNoRejection(t, events)
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	event := events[0]
	for key, want := range map[string]float64{"total_refs": 36, "unique_refs": 24, "cache_hits": 19, "duration_ms": 2431} {
		if event[key] != want {
			t.Fatalf("%s = %#v, want %v（event=%#v）", key, event[key], want, event)
		}
	}
}

// 放宽 reason 不等于放宽一切：未知 reason 仍要被拒并留痕，未列入白名单的
// endpoint 要被丢弃而不是原样落盘（资源路径不得泄漏）。
func TestR127DiagnosticAllowlistStaysClosed(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	if code := r127PostDiagnostic(t, instance, `{"event":"local_request_client","reason":"bogus","endpoint":"image"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown reason status = %d, want 400", code)
	}
	events := r127DiagnosticEvents(t, instance)
	if len(events) != 1 || events[0]["event"] != "client_diagnostic_rejected" || events[0]["reason"] != "unknown-reason" {
		t.Fatalf("events = %#v", events)
	}

	instance = newR127DiagnosticApp(t)
	body := `{"event":"local_request_client","reason":"failed",` +
		`"endpoint":"/api/image?path=/lol-game-data/assets/v1/profile-icons/4379.jpg","imageSource":"lcu"}`
	if code := r127PostDiagnostic(t, instance, body); code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", code)
	}
	events = r127DiagnosticEvents(t, instance)
	r127AssertNoRejection(t, events)
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if _, ok := events[0]["endpoint"]; ok {
		t.Fatalf("未白名单的 endpoint 被原样记录: %#v", events[0])
	}
	encoded, err := json.Marshal(events[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"profile-icons", "4379", "/api/image", "path"} {
		if strings.Contains(string(encoded), leaked) {
			t.Fatalf("诊断泄漏了 %q: %s", leaked, encoded)
		}
	}
}

/* ---------- R127 P1-c：韩服平均段位一场都没对上 ---------- */

// r127RealOPGGGames 读取仓库里的真实 OP.GG 抓包（20 行，7 行带平均段位）。
// 这是唯一一份真实来源数据；之前的测试两边都用同一个手写基准，所以「23 场
// 0 命中」这种系统性错误在全绿的测试里根本看不出来。
func r127RealOPGGGames(t *testing.T) []opggGameTier {
	t.Helper()
	data, err := os.ReadFile("testdata/r112-opgg-average-tiers.txt")
	if err != nil {
		t.Fatal(err)
	}
	games, _, err := parseOPGGGamesPage(data)
	if err != nil {
		t.Fatal(err)
	}
	return games
}

// TestR127OPGGCreatedAtIsGameEndNotStart 用真实抓包钉死 created_at 的时间基准：
// 同一名玩家的相邻两局不可能在时间上重叠。把 created_at 当开局时间会产生 3 处
// 重叠（-21s、-105s、-574s），当结束时间则 0 处重叠。
func TestR127OPGGCreatedAtIsGameEndNotStart(t *testing.T) {
	games := r127RealOPGGGames(t)
	if len(games) != 20 {
		t.Fatalf("rows = %d, want 20", len(games))
	}
	overlaps := func(asStart bool) (count int, worst int64) {
		// 抓包按时间倒序：games[i] 比 games[i+1] 更晚。
		for index := 0; index+1 < len(games); index++ {
			newer, older := games[index], games[index+1]
			var olderEnd, newerStart int64
			if asStart {
				olderEnd = older.createdAt + older.duration*1000
				newerStart = newer.createdAt
			} else {
				olderEnd = older.createdAt
				newerStart = newer.createdAt - newer.duration*1000
			}
			if gap := newerStart - olderEnd; gap < 0 {
				count++
				if gap < worst {
					worst = gap
				}
			}
		}
		return count, worst
	}
	startCount, startWorst := overlaps(true)
	endCount, endWorst := overlaps(false)
	if startCount == 0 {
		t.Fatal("把 created_at 当开局时间本该出现不可能的重叠，抓包是不是被换掉了？")
	}
	if endCount != 0 {
		t.Fatalf("把 created_at 当结束时间仍有 %d 处重叠（最差 %dms），时间基准结论不成立", endCount, endWorst)
	}
	t.Logf("created_at 当开局时间：%d 处重叠（最差 %dms）；当结束时间：0 处重叠", startCount, startWorst)
}

// TestR127MatchOPGGAverageTierUsesRealRiotTimeBases 是 P1-c.2 的夹具测试：OP.GG
// 一侧用真实抓包，Riot 一侧按「结束时间 = created_at、开局 = 结束 - 时长、
// 房间创建 = 开局 - BP/读条」构造。
//
// 对抗变异：把匹配改回只用 gameCreation（即清掉 StartAt/EndAt），同一批数据必须
// 一场都对不上——这正是真机日志里 23 场 0 命中的形状。
func TestR127MatchOPGGAverageTierUsesRealRiotTimeBases(t *testing.T) {
	games := r127RealOPGGGames(t)
	const bpAndLoadingMS = 180_000 // 排位 BP + 读条，工单实测 2–4 分钟
	matches := make([]matchTierMatchRequest, 0, len(games))
	for _, game := range games {
		if game.tier.Tier == "" {
			continue // 只挑 OP.GG 确实给了平均段位的行
		}
		endAt := game.createdAt
		startAt := endAt - game.duration*1000
		matches = append(matches, matchTierMatchRequest{
			GameID: 1, CreatedAt: startAt - bpAndLoadingMS, Duration: game.duration,
			StartAt: startAt, EndAt: endAt,
		})
	}
	if len(matches) != 7 {
		t.Fatalf("带段位的行数 = %d, want 7", len(matches))
	}
	bases := map[string]int{}
	matched := 0
	for _, match := range matches {
		result := matchOPGGAverageTierDetailed(match, games)
		if result.tier == nil {
			continue
		}
		matched++
		bases[result.bestBase]++
		if result.bestGap > opggGamesTimeSlack {
			t.Fatalf("命中差值 %dms 超出容差", result.bestGap)
		}
	}
	if matched != len(matches) {
		t.Fatalf("matched = %d/%d：真实抓包 + 三个时间基准本该全部对上", matched, len(matches))
	}
	if bases[opggTierBaseEnd] != len(matches) {
		t.Fatalf("命中基准 = %#v, want 全部 %q", bases, opggTierBaseEnd)
	}

	creationOnly := 0
	for _, match := range matches {
		legacy := match
		legacy.StartAt, legacy.EndAt = 0, 0
		if matchOPGGAverageTierDetailed(legacy, games).tier != nil {
			creationOnly++
		}
	}
	if creationOnly != 0 {
		t.Fatalf("只用 gameCreation 竟然对上了 %d 场，说明这个夹具测不到时间基准", creationOnly)
	}
}

// TestR127OPGGNearestRowGapExposesTheMissingEvidence 锁住 P1-c.1 的诊断口径：
// 未匹配时必须能报出「与最近一行差了多少秒」，只记秒数、不记任何 ID。
// 旧的 gameCreation 基准在这个夹具上应当报出整整一场时长的差值——这正是之前
// 日志里完全缺失、导致只能靠猜的那条证据。
func TestR127OPGGNearestRowGapExposesTheMissingEvidence(t *testing.T) {
	games := []opggGameTier{{createdAt: 1_800_000_000_000, duration: 1500}}
	aligned := matchTierMatchRequest{CreatedAt: 1_800_000_000_000 - 1500_000 - 180_000, Duration: 1500, StartAt: 1_800_000_000_000 - 1500_000, EndAt: 1_800_000_000_000}
	if timeGap, spanGap := opggNearestRowGap(aligned, games); timeGap != 0 || spanGap != 0 {
		t.Fatalf("aligned gaps = %d/%d, want 0/0", timeGap, spanGap)
	}
	legacy := matchTierMatchRequest{CreatedAt: aligned.CreatedAt, Duration: 1500}
	timeGap, spanGap := opggNearestRowGap(legacy, games)
	if timeGap != 1680 || spanGap != 0 {
		t.Fatalf("legacy gaps = %d/%d, want 1680/0（时长 + BP/读条）", timeGap, spanGap)
	}
	if got := opggTierTimeBases(matchTierMatchRequest{}); len(got) != 0 {
		t.Fatalf("没有任何时间基准时不该凭空造基准: %#v", got)
	}
}

/* ---------- R127 P1-a：慢图不能堵住快图（后端侧） ---------- */

func TestR127RemoteHostBackoffStopsRepeatedTimeouts(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	const host = "raw.communitydragon.org"
	calls := 0
	timeoutLoader := func(context.Context) ([]byte, error) { calls++; return nil, context.DeadlineExceeded }
	for index := 0; index < assetHostTimeoutThreshold; index++ {
		key := fmt.Sprintf("cdragon:/fixture-%d.png", index)
		if _, err := instance.loadAssetFromHost(context.Background(), host, key, 0, 0, timeoutLoader); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("attempt %d err = %v, want DeadlineExceeded", index, err)
		}
	}
	if calls != assetHostTimeoutThreshold {
		t.Fatalf("calls = %d, want %d", calls, assetHostTimeoutThreshold)
	}
	// 退避窗口内：同一主机的其它资源必须快速失败，一次上游请求都不发。
	started := time.Now()
	for index := 0; index < 30; index++ {
		key := fmt.Sprintf("cdragon:/other-%d.png", index)
		if _, err := instance.loadAssetFromHost(context.Background(), host, key, 0, 0, timeoutLoader); !errors.Is(err, errCachedAssetFailure) {
			t.Fatalf("backoff attempt %d err = %v, want errCachedAssetFailure", index, err)
		}
	}
	if calls != assetHostTimeoutThreshold {
		t.Fatalf("退避期间仍然发上游请求：calls = %d", calls)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("30 次快速失败耗时 %s，不该等满远程超时", elapsed)
	}
	// 其它主机不受牵连。
	otherCalls := 0
	if _, err := instance.loadAssetFromHost(context.Background(), dataDragonHost, "champion-asset:ddragon:/x.png", 0, 0,
		func(context.Context) ([]byte, error) { otherCalls++; return nil, context.DeadlineExceeded }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("other host err = %v", err)
	}
	if otherCalls != 1 {
		t.Fatalf("其它主机被误伤：calls = %d", otherCalls)
	}
	// 浏览器自己取消不算主机故障，不触发退避。
	instance.observeAssetHostResult("ddragon.leagueoflegends.com", context.Canceled)
	instance.observeAssetHostResult("ddragon.leagueoflegends.com", context.Canceled)
	instance.observeAssetHostResult("ddragon.leagueoflegends.com", context.Canceled)
	if instance.assetHostBlocked(dataDragonHost) {
		t.Fatal("Canceled 被当成主机超时，退避误伤前端取消")
	}
	// 主机恢复（任何非超时结果）必须解除退避。
	instance.observeAssetHostResult(host, nil)
	if instance.assetHostBlocked(host) {
		t.Fatal("主机恢复后仍在退避")
	}
	found := false
	for _, event := range r127DiagnosticEvents(t, instance) {
		if event["event"] != "asset_host_backoff" {
			continue
		}
		found = true
		if event["host"] != host || event["consecutive_timeouts"] != float64(assetHostTimeoutThreshold) || event["backoff_seconds"] != float64(60) {
			t.Fatalf("asset_host_backoff = %#v", event)
		}
	}
	if !found {
		t.Fatal("进入退避必须留下 asset_host_backoff 日志，否则下次还是查不出为什么图标全挂")
	}
}

func TestR127RemoteImageBudgetSeparatesIconsFromArtwork(t *testing.T) {
	if publicRemoteImageTimeout >= publicImageTimeout {
		t.Fatalf("远程小图标预算 %s 必须小于整体预算 %s", publicRemoteImageTimeout, publicImageTimeout)
	}
	for _, requestPath := range []string{
		"/latest/game/assets/ux/kiwi/augments/icons/tf_icon.png",
		"/latest/plugins/rcp-be-lol-game-data/global/default/v1/champion-icons/103.png",
		"/cdn/16.16.1/img/item/3071.png",
		"/cdn/16.16.1/img/spell/SummonerFlash.png",
	} {
		if got := remoteImageBudget(requestPath); got != publicRemoteImageTimeout {
			t.Fatalf("%s budget = %s, want %s", requestPath, got, publicRemoteImageTimeout)
		}
	}
	for _, requestPath := range []string{
		"/cdn/img/champion/splash/Camille_0.jpg",
		"/cdn/img/champion/centered/Camille_0.jpg",
		"/images/lol/act/img/skin/big164001.jpg",
		"/latest/game/assets/characters/camille/skins/base/loadscreen.jpg",
	} {
		if got := remoteImageBudget(requestPath); got != publicImageTimeout {
			t.Fatalf("%s budget = %s, want %s（原画不能被小图标的预算拖垮）", requestPath, got, publicImageTimeout)
		}
	}
}

// r127EventCollector 收集 provider.diag 事件，必须带锁：observeAssetFetch 会在
// 10 秒后用 time.AfterFunc 触发 flushAssetFetch，那次回调可能落在本用例结束之后
// 的其它测试期间，无锁追加会被 -race 判成数据竞争。
type r127EventCollector struct {
	mu     sync.Mutex
	events []map[string]any
}

func (c *r127EventCollector) record(event map[string]any) {
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}

// only 按事件名过滤：延迟触发的 asset_fetch 聚合事件随时可能落进来，断言只关心
// augment_icon_fetch，否则会变成偶发失败。
func (c *r127EventCollector) only(name string) []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	filtered := make([]map[string]any, 0, len(c.events))
	for _, event := range c.events {
		if event["event"] == name {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func (c *r127EventCollector) reset() {
	c.mu.Lock()
	c.events = nil
	c.mu.Unlock()
}

func r127AugmentProvider(collector *r127EventCollector) *championProvider {
	provider := newChampionProvider()
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound, Status: http.StatusText(http.StatusNotFound), Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader("missing")), ContentLength: 7, Request: request,
		}, nil
	})}
	provider.diag = collector.record
	return provider
}

func r127BadGatewayClient(t *testing.T, status int) (*LCUClient, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, http.StatusText(status), status)
	}))
	return &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}, server.Close
}

// R127 P1-a.4：LCU 到底回了什么状态，之前完全没有痕迹，所以查不清
// Kiwi/Augments/Icons 为什么本机取不到、出网是不是必要。
func TestR127AugmentIconFetchReportsLCUStatus(t *testing.T) {
	remotePath := "/latest/game/assets/ux/kiwi/augments/icons/missing_large.png"
	assetURL := "/api/champion-asset?source=communitydragon&path=" + url.QueryEscape(remotePath)
	collector := &r127EventCollector{}

	recorder := httptest.NewRecorder()
	(&app{champions: r127AugmentProvider(collector)}).handleChampionAsset(recorder, httptest.NewRequest(http.MethodGet, assetURL, nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	events := collector.only("augment_icon_fetch")
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if events[0]["lcu_status"] != 0 {
		t.Fatalf("未连接客户端时 lcu_status = %#v, want 0（根本没问）", events[0]["lcu_status"])
	}

	client, closeClient := r127BadGatewayClient(t, http.StatusBadRequest)
	defer closeClient()
	collector.reset()
	recorder = httptest.NewRecorder()
	instance := &app{champions: r127AugmentProvider(collector), lcu: client, connected: true}
	instance.handleChampionAsset(recorder, httptest.NewRequest(http.MethodGet, assetURL, nil))
	events = collector.only("augment_icon_fetch")
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	// 这里的事件直接来自 provider.diag（没过 JSON），所以是 int 而不是 float64。
	if events[0]["lcu_status"] != http.StatusBadRequest {
		t.Fatalf("LCU 返回 400 时 lcu_status = %#v, want 400", events[0]["lcu_status"])
	}

	// 生产日志里的形状：增强符文图标走 /api/image，LCU 400 之后回退 CommunityDragon。
	collector.reset()
	recorder = httptest.NewRecorder()
	clientImagePath := "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/ZQ7XK_large.png"
	instance = &app{champions: r127AugmentProvider(collector), lcu: client, connected: true}
	instance.handleImage(recorder, httptest.NewRequest(http.MethodGet, "/api/image?path="+url.QueryEscape(clientImagePath), nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("image status = %d, want 404", recorder.Code)
	}
	events = collector.only("augment_icon_fetch")
	if len(events) != 1 {
		t.Fatalf("image events = %#v", events)
	}
	if events[0]["path_template"] != "client-game/ux/kiwi/augments/icons/{icon}_large.png" {
		t.Fatalf("path_template = %#v", events[0]["path_template"])
	}
	if events[0]["lcu_status"] != http.StatusBadRequest {
		t.Fatalf("image lcu_status = %#v, want 400", events[0]["lcu_status"])
	}
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(encoded)), "zq7xk") || strings.Contains(string(encoded), "missing") {
			t.Fatalf("augment 诊断泄漏了具体图标 ID: %s", encoded)
		}
	}
}

/* ---------- R127 P1-b：平均段位用快的数据源 ---------- */

func TestR127TierOnlyRankModePrefersSGPAndSkipsWinLossCompletion(t *testing.T) {
	playerRef := strings.Repeat("t", 48)
	newFixture := func(t *testing.T, lcuBody string) (*app, *LCUClient, *int, *int, func()) {
		t.Helper()
		lcuCalls, sgpCalls := 0, 0
		lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			lcuCalls++
			_, _ = io.WriteString(w, lcuBody)
		}))
		sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			sgpCalls++
			_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","rank":"I","leaguePoints":7,"wins":3,"losses":2}]}`)
		}))
		client := &LCUClient{baseURL: lcuServer.URL, token: "test", http: lcuServer.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
		provider := newSGPProvider()
		provider.http = sgpServer.Client()
		provider.serverBases["HN1"] = sgpServer.URL
		provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
		return &app{sgp: provider}, client, &lcuCalls, &sgpCalls, func() { lcuServer.Close(); sgpServer.Close() }
	}
	// 客户端只给出「胜场有、负场缺」的未验证胜负场。
	const unverified = `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"EMERALD","division":"III","leaguePoints":68,"wins":225,"losses":0}]}`

	// 对照组：需要胜负场的路径仍然先等 LCU，再为了胜负场补一次 SGP。
	instance, client, lcuCalls, sgpCalls, closeFixture := newFixture(t, unverified)
	ranks, _, capability := instance.loadRanksWithFallback(context.Background(), client, playerRef, false, "HN1", "PUBLIC")
	if *lcuCalls != 1 || *sgpCalls != 1 {
		t.Fatalf("对照组 calls = lcu %d / sgp %d, want 1/1", *lcuCalls, *sgpCalls)
	}
	if capability.FallbackReason != "lcu-unverified-win-loss" || len(ranks) != 1 {
		t.Fatalf("对照组 capability = %#v ranks = %#v", capability, ranks)
	}
	closeFixture()

	// 平均段位：SGP 优先，一次就够，不再为了胜负场等 LCU。
	instance, client, lcuCalls, sgpCalls, closeFixture = newFixture(t, unverified)
	ranks, _, capability = instance.loadRanksWithFallback(context.Background(), client, playerRef, false, "HN1", "PUBLIC", true)
	if *sgpCalls != 1 || *lcuCalls != 0 {
		t.Fatalf("只要段位 calls = lcu %d / sgp %d, want 0/1", *lcuCalls, *sgpCalls)
	}
	if capabilitySource(capability) != dataSourceSGP || len(ranks) != 1 || ranks[0].Tier != "GOLD" {
		t.Fatalf("只要段位 capability = %#v ranks = %#v", capability, ranks)
	}
	closeFixture()

	if got := rankSourcesTierOnly([]string{dataSourceLCU, dataSourceSGP}); len(got) != 2 || got[0] != dataSourceSGP || got[1] != dataSourceLCU {
		t.Fatalf("rankSourcesTierOnly = %#v", got)
	}
	if rankScoreCacheKey(dataSourceSGP, "HN1", playerRef) == rankScoreCacheKeyScoped(dataSourceSGP, "HN1", playerRef, rankScoreTierOnlyScope) {
		t.Fatal("只要段位与需要胜负场的缓存键必须分开，否则个人资料页会读到没有胜负场的结果")
	}
	if matchTiersRankConcurrency != 8 {
		t.Fatalf("matchTiersRankConcurrency = %d, want 8", matchTiersRankConcurrency)
	}
}

func TestR127MatchTierLongTermCacheRoundTrip(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	if _, ok := instance.readMatchTierCache(7001); ok {
		t.Fatal("冷缓存不该命中")
	}
	instance.writeMatchTierCache(7001, &matchTiersResponse{Tier: "GOLD", Division: "II", Score: 1050, Samples: 9})
	got, ok := instance.readMatchTierCache(7001)
	if !ok || got.Tier != "GOLD" || got.Division != "II" || got.Score != 1050 || got.Samples != 9 {
		t.Fatalf("roundtrip = %#v ok=%t", got, ok)
	}
	// 没匹配到段位的结果绝不落盘：OP.GG 现在没收录不等于永远没有。
	instance.writeMatchTierCache(7002, &matchTiersResponse{})
	instance.writeMatchTierCache(7003, nil)
	if _, ok := instance.readMatchTierCache(7002); ok {
		t.Fatal("空结果被落盘，会把「暂时查不到」变成永久结论")
	}
	if _, ok := instance.readMatchTierCache(7003); ok {
		t.Fatal("nil 被落盘")
	}
	response, missing, hits := instance.splitCachedMatchTiers([]matchTierMatchRequest{
		{GameID: 7001, Duration: 1500}, {GameID: 7004, Duration: 1500},
	})
	if hits != 1 || len(missing) != 1 || missing[0].GameID != 7004 {
		t.Fatalf("hits = %d missing = %#v", hits, missing)
	}
	if response["7001"] == nil || response["7001"].Tier != "GOLD" {
		t.Fatalf("response = %#v", response)
	}
	if _, exists := response["7004"]; exists {
		t.Fatal("未命中的对局不该出现在缓存响应里")
	}
	// 跨重启（新的 app 实例、同一个存储目录）仍然命中。
	restarted := &app{storage: instance.storage}
	if _, ok := restarted.readMatchTierCache(7001); !ok {
		t.Fatal("重启后长期缓存丢失")
	}
	// 过期条目必须失效。
	cache := restarted.matchTierDiskCache()
	entry, err := cache.readDisk(matchTierCacheKey(7001))
	if err != nil {
		t.Fatal(err)
	}
	entry.ExpiresAt = time.Now().Add(-time.Hour)
	if err := cache.writeDisk(entry); err != nil {
		t.Fatal(err)
	}
	if _, ok := (&app{storage: instance.storage}).readMatchTierCache(7001); ok {
		t.Fatal("过期缓存仍然命中")
	}
}

// R127 P1-c.3 验收：重开同一个韩服玩家时平均段位立即显示，命中长期缓存，
// 一个 OP.GG 请求都不发。
func TestR127MatchTierLongTermCacheSkipsOPGGEntirely(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	t.Cleanup(func() { _ = store.Close() })
	var calls atomic.Int32
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return testHTTPResponse(request, http.StatusOK, "0:{\"a\":\"$@1\"}\n1:{\"data\":[]}\n"), nil
	})}
	champions.clientMu.Unlock()
	newInstance := func() *app {
		return &app{
			storage: store, token: "session-secret", champions: champions, opgg: newOPGGInsights(),
			gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
		}
	}
	instance := newInstance()
	publicRef := instance.registerGameplayReferenceDetails(gameplayReference{
		PlayerRef: strings.Repeat("m", 48), GameName: "Cached", TagLine: "KR1", Region: riotRegionKR,
	})
	// 上一轮会话已经匹配到并落盘的结果。
	instance.writeMatchTierCache(9001, &matchTiersResponse{Tier: "PLATINUM", Division: "I", LP: 42, Score: 1700, Samples: 10})
	query := func(gameID int64) (*httptest.ResponseRecorder, map[string]*matchTiersResponse) {
		body, err := json.Marshal(matchTiersRequest{
			Region: riotRegionKR, PlayerRef: publicRef,
			Matches: []matchTierMatchRequest{{GameID: gameID, CreatedAt: time.Now().UnixMilli(), Duration: 1800}},
		})
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		instance.handleGameplayMatchTiers(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/match-tiers", strings.NewReader(string(body))))
		var response map[string]*matchTiersResponse
		_ = json.Unmarshal(recorder.Body.Bytes(), &response)
		return recorder, response
	}
	recorder, response := query(9001)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := response["9001"]; got == nil || got.Tier != "PLATINUM" || got.Division != "I" || got.LP != 42 {
		t.Fatalf("cached tier = %#v", got)
	}
	if calls.Load() != 0 {
		t.Fatalf("命中长期缓存却发了 %d 次 OP.GG 请求", calls.Load())
	}
	found := false
	for _, event := range r127DiagnosticEvents(t, instance) {
		if event["event"] != "opgg_match_tiers_result" {
			continue
		}
		found = true
		if event["opgg_requests"] != float64(0) || event["cache_hits"] != float64(1) || event["matched"] != float64(1) {
			t.Fatalf("opgg_match_tiers_result = %#v", event)
		}
	}
	if !found {
		t.Fatal("全命中也必须留下 opgg_match_tiers_result，否则看不出有没有出网")
	}
	// 没缓存过的场次仍然要出网查。
	query(9002)
	if calls.Load() == 0 {
		t.Fatal("未命中长期缓存时没有去查 OP.GG")
	}
}

// R127 复审第 1、2 条的回归锁：主机退避既不能被外层聚合错误抹掉，也不能误伤
// 已经缓存在本地、根本不需要联网的图标。
func TestR127HostBackoffSurvivesAggregateErrorsAndSparesCachedAssets(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	const host = "raw.communitydragon.org"

	// 生产形状：/api/image 的外层是「该 assetPath 全部候选均失败」的聚合键，
	// 内层每个候选各自向同一台主机取图。
	aggregate := func(key string) error {
		_, err := instance.loadAssetFromHost(context.Background(), host, "cdragon-resolved:"+key, 0, communityImageResolveNegativeTTL,
			func(context.Context) ([]byte, error) {
				for index := 0; index < assetHostTimeoutThreshold; index++ {
					_, _ = instance.loadAssetFromHost(context.Background(), host, fmt.Sprintf("cdragon:%s-%d", key, index), 0, 0,
						func(context.Context) ([]byte, error) { return nil, context.DeadlineExceeded })
				}
				return nil, errCommunityImageCandidatesExhausted
			})
		return err
	}
	if err := aggregate("first"); !errors.Is(err, errCommunityImageCandidatesExhausted) {
		t.Fatalf("aggregate err = %v", err)
	}
	if !instance.assetHostBlocked(host) {
		t.Fatal("内层连续超时后必须进入主机退避")
	}
	// 退避生效期间，外层聚合键也必须直接快速失败：不再把 6 个候选重放一遍，
	// 更不能用聚合错误把内层刚建立的退避抹掉。
	if err := aggregate("second"); !errors.Is(err, errCachedAssetFailure) {
		t.Fatalf("second aggregate err = %v, want errCachedAssetFailure（快速失败）", err)
	}
	if !instance.assetHostBlocked(host) {
		t.Fatal("外层聚合错误把主机退避抹掉了：这条生产路径上退避等于没有")
	}

	// 退避只挡「真的要出网」的请求。
	cachedCalls := 0
	seed := func() []byte {
		data, err := instance.loadAssetFromHost(context.Background(), dataDragonHost, "champion-asset:cached", 1<<20, 0,
			func(context.Context) ([]byte, error) { cachedCalls++; return []byte("cached-image"), nil })
		if err != nil {
			t.Fatalf("cached load err = %v", err)
		}
		return data
	}
	if got := seed(); string(got) != "cached-image" || cachedCalls != 1 {
		t.Fatalf("seed = %s calls = %d", got, cachedCalls)
	}
	for index := 0; index < assetHostTimeoutThreshold; index++ {
		if _, err := instance.loadAssetFromHost(context.Background(), dataDragonHost, fmt.Sprintf("champion-asset:timeout-%d", index), 0, 0,
			func(context.Context) ([]byte, error) { return nil, context.DeadlineExceeded }); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout %d err = %v", index, err)
		}
	}
	if !instance.assetHostBlocked(dataDragonHost) {
		t.Fatal("ddragon 连续超时后必须进入退避")
	}
	if got := seed(); string(got) != "cached-image" || cachedCalls != 1 {
		t.Fatalf("退避期间已缓存的图必须照常返回：got %s loader calls = %d", got, cachedCalls)
	}
	// 退避窗口内未缓存的资源仍然快速失败，不再等满远程超时。
	started := time.Now()
	if _, err := instance.loadAssetFromHost(context.Background(), dataDragonHost, "champion-asset:brand-new", 0, 0,
		func(context.Context) ([]byte, error) { t.Fatal("退避期间不该出网"); return nil, nil }); !errors.Is(err, errCachedAssetFailure) {
		t.Fatalf("brand new err = %v, want errCachedAssetFailure", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("快速失败耗时 %s", elapsed)
	}
}
