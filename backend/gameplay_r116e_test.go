package main

// R116-E 的验收测试。
//
// 覆盖三块：
//  1. 用户裁决新增的「UI 入口」——海克斯大乱斗页签的数据通路（总览页队列切换器
//     的第三个 tab），以及契约附五列出的三处 420/440 硬门禁的处置；
//  2. 单双排页签与韩服路径的逐字节回归；
//  3. P2-6 静态查询的 HTTP 出口（样本不足时 available=false，前端整块不渲染）。
//
// 这些测试刻意不注入任何桩函数：rankedQueueLabel 以前就是因为测试自己塞了桩，
// 才让「生产代码里根本没定义」这个缺陷活到了真机（见 docs/r116e-execution-ledger.md）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// legacyRankedQueueTabs 复现 R116-E 之前 buildGameplayRankedQueues(solo, flex, ...)
// 的两页签调用形状，让既有断言不必逐条改写。韩服路径（riot_api.go）用的就是
// 这个形状：韩服没有海克斯大乱斗队列。
func legacyRankedQueueTabs(soloMatches, flexMatches []gameplayMatch) []gameplayRankedQueueTab {
	return []gameplayRankedQueueTab{
		{Key: "420", Label: rankedQueueLabel(seasonQueueSoloDuo), QueueIDs: []int64{seasonQueueSoloDuo}, Matches: soloMatches},
		{Key: "440", Label: rankedQueueLabel(seasonQueueFlex), QueueIDs: []int64{seasonQueueFlex}, Matches: flexMatches},
	}
}

// r116eMatch 造一场带位置的详情战绩。海斗场次刻意不给 Position——
// ARAM 系没有分路，这是「海斗不出位置统计与能力雷达」的根因。
func r116eMatch(id, queueID int64, result string, position string) gameplayMatch {
	match := abilityTestMatch(id)
	match.QueueID = queueID
	match.Result = result
	match.CreatedAt = id
	for index := range match.Participants {
		match.Participants[index].ChampionID = 157
	}
	if position == "" {
		for index := range match.Participants {
			match.Participants[index].Position = ""
		}
	}
	return match
}

// r116eSeasonCache 造一份「同一账号既有 420 场次又有海斗场次」的赛季快照。
func r116eSeasonCache() []seasonRankedMatch {
	return []seasonRankedMatch{
		{GameID: 101, CreatedAt: 101, QueueID: 420, ChampionID: 157, Win: true, Kills: 5, Deaths: 1, Assists: 5, Position: "middle"},
		{GameID: 102, CreatedAt: 102, QueueID: 440, ChampionID: 157, Win: false, Kills: 1, Deaths: 9, Assists: 2, Position: "top"},
		{GameID: 201, CreatedAt: 201, QueueID: 2400, ChampionID: 157, Win: true, Kills: 12, Deaths: 4, Assists: 9},
		{GameID: 202, CreatedAt: 202, QueueID: 2300, ChampionID: 157, Win: false, Kills: 3, Deaths: 8, Assists: 4},
		{GameID: 203, CreatedAt: 203, QueueID: 3270, ChampionID: 157, Win: true, Kills: 7, Deaths: 5, Assists: 6},
		// 3220 是「极地大乱斗」，不属于海斗页签，也不属于任何排位页签。
		{GameID: 301, CreatedAt: 301, QueueID: 3220, ChampionID: 157, Win: true, Kills: 20, Deaths: 0, Assists: 0},
	}
}

// 附五判据 1：海斗页签的「近期战绩」只含海斗场次，不含任何 420/440 场次。
func TestR116EMayhemTabOnlyContainsMayhemGames(t *testing.T) {
	cached := r116eSeasonCache()
	tabs := gameplayRankedQueueTabs(map[int64][]gameplayMatch{420: nil, 440: nil}, cached)
	if len(tabs) != 3 {
		t.Fatalf("tabs = %d, want 3 (solo / flex / mayhem): %#v", len(tabs), tabs)
	}
	mayhem := tabs[2]
	if mayhem.Key != "2300" || mayhem.Label != "海克斯大乱斗" {
		t.Fatalf("unexpected mayhem tab: %#v", mayhem)
	}
	queues := buildGameplayRankedQueues(tabs, "subject", nil, "")
	entry, ok := queues["2300"]
	if !ok || entry.RecentRanked == nil {
		t.Fatalf("mayhem queue entry missing: %#v", queues)
	}
	// 3 场海斗（2400/2300/3270 各一场）全都要算进来，3220 与 420/440 都不算。
	if entry.RecentRanked.Games != 3 || entry.RecentRanked.Wins != 2 || entry.RecentRanked.Losses != 1 {
		t.Fatalf("mayhem recent summary = %#v, want 3 games / 2 wins / 1 loss", entry.RecentRanked)
	}
	if entry.RecentRanked.QueueLabel != "海克斯大乱斗" {
		t.Fatalf("mayhem queue label = %q, want 海克斯大乱斗", entry.RecentRanked.QueueLabel)
	}
	if entry.PositionQueueLabel != "海克斯大乱斗" {
		t.Fatalf("mayhem position label = %q, want 海克斯大乱斗", entry.PositionQueueLabel)
	}
	// 海斗没有分路：位置胜率列表必须为空，不能用别的队列的数据顶上。
	if len(entry.RecentRanked.Positions) != 0 {
		t.Fatalf("mayhem summary must not carry position win rates: %#v", entry.RecentRanked.Positions)
	}
}

// 附五判据 2：海斗页签不渲染位置统计与能力雷达。
// 对抗变异：把 positionStatsForQueue 的早退改回 `return positionStats(matches, playerRef)`，
// 或把 gameplayRankedTabHasPositions 改成恒真，本测试必须 FAIL。
func TestR116EMayhemTabRendersNoPositionsAndNoAbilityRadar(t *testing.T) {
	matches := []gameplayMatch{
		r116eMatch(1, 420, "win", "top"),
		r116eMatch(2, 420, "win", "top"),
		r116eMatch(3, 420, "loss", "top"),
		r116eMatch(4, 2400, "win", ""),
		r116eMatch(5, 2300, "loss", ""),
	}
	tabs := gameplayRankedQueueTabs(map[int64][]gameplayMatch{420: matches, 440: nil}, r116eSeasonCache())
	queues := buildGameplayRankedQueues(tabs, "subject", nil, "")
	mayhem := queues["2300"]
	if mayhem.Positions != nil {
		t.Fatalf("mayhem tab must not carry position stats, got %#v", mayhem.Positions)
	}
	if mayhem.Ability != nil {
		t.Fatalf("mayhem tab must not carry an ability radar, got %#v", mayhem.Ability)
	}
	if mayhem.AbilitySampleGames != 0 {
		t.Fatalf("mayhem ability sample games = %d, want 0", mayhem.AbilitySampleGames)
	}
	// 页签自己也知道它没有分路口径（后端据此不下发 positions/ability）。
	if tabs[2].gameplayRankedTabHasPositions() {
		t.Fatalf("mayhem tab must not claim to have lane positions: %#v", tabs[2])
	}
	if !tabs[0].gameplayRankedTabHasPositions() || !tabs[1].gameplayRankedTabHasPositions() {
		t.Fatal("classic ranked tabs must keep their lane positions")
	}
	// 单双排页签照样有位置与能力样本（回归：不能被海斗的处置带坏）。
	solo := queues["420"]
	if positionStatsGames(solo.Positions) == 0 {
		t.Fatalf("solo tab lost its position stats: %#v", solo.Positions)
	}
	if solo.AbilitySampleGames == 0 {
		t.Fatalf("solo tab lost its ability sample count: %#v", solo)
	}
	// positionStatsForQueue 本体也必须拒绝回退到全量统计：
	// 这 5 场里 3 场是 420，如果回退，海斗队列会数出 3 场 top。
	if got := positionStatsForQueue(matches, "subject", 2400); got != nil {
		t.Fatalf("positionStatsForQueue(2400) = %#v, want nil (ARAM has no lanes)", got)
	}
	if got := positionStatsGames(positionStatsForQueue(matches, "subject", 420)); got != 3 {
		t.Fatalf("positionStatsForQueue(420) games = %d, want 3", got)
	}
	if got := liveRecentPositions(matches, "subject", 2400); got != nil {
		t.Fatalf("liveRecentPositions(2400) = %#v, want nil", got)
	}
}

// 附五判据 3：单双排页签的行为不变——有没有海斗样本，420/440 两个 key 的内容
// 都必须完全一致。
func TestR116EClassicTabsAreUnaffectedByMayhemSamples(t *testing.T) {
	matches := map[int64][]gameplayMatch{
		420: {r116eMatch(1, 420, "win", "top"), r116eMatch(2, 420, "loss", "top")},
		440: {r116eMatch(3, 440, "win", "jungle")},
	}
	without := buildGameplayRankedQueues(gameplayRankedQueueTabs(matches, nil), "subject", nil, "")
	with := buildGameplayRankedQueues(gameplayRankedQueueTabs(matches, r116eSeasonCache()), "subject", nil, "")
	if len(without) != 2 {
		t.Fatalf("without mayhem samples there must be exactly two tabs, got %#v", without)
	}
	if len(with) != 3 {
		t.Fatalf("with mayhem samples there must be three tabs, got %#v", with)
	}
	for _, key := range []string{"420", "440"} {
		before, _ := json.Marshal(without[key])
		after, _ := json.Marshal(with[key])
		if string(before) != string(after) {
			t.Fatalf("queue %s changed when mayhem samples appeared:\n before=%s\n after =%s", key, before, after)
		}
	}
	// 420 页签绝不能混进海斗场次。
	if got := with["420"].RecentRanked.Games; got != 2 {
		t.Fatalf("solo recent games = %d, want 2", got)
	}
	if got := with["440"].RecentRanked.Games; got != 1 {
		t.Fatalf("flex recent games = %d, want 1", got)
	}
}

// 附五判据 4：韩服路径（riot_api.go）行为不变——只产出两个页签，
// 没有海斗 key，也没有任何海斗文案。
func TestR116EKoreanPathStillProducesExactlyTwoQueues(t *testing.T) {
	matches := []gameplayMatch{r116eMatch(1, 420, "win", "top"), r116eMatch(2, 440, "loss", "jungle")}
	queues := buildGameplayRankedQueues([]gameplayRankedQueueTab{
		{Key: "420", Label: rankedQueueLabel(seasonQueueSoloDuo), QueueIDs: []int64{seasonQueueSoloDuo}, Matches: recentRankedMatchesForQueue(matches, 420, defaultMatchCount)},
		{Key: "440", Label: rankedQueueLabel(seasonQueueFlex), QueueIDs: []int64{seasonQueueFlex}, Matches: recentRankedMatchesForQueue(matches, 440, defaultMatchCount)},
	}, "subject", nil, riotRegionKR)
	if len(queues) != 2 {
		t.Fatalf("KR queues = %d, want 2: %#v", len(queues), queues)
	}
	for key, expected := range map[string]string{"420": "单双排", "440": "灵活组排"} {
		entry, ok := queues[key]
		if !ok || entry.RecentRanked == nil {
			t.Fatalf("KR queue %s missing: %#v", key, queues)
		}
		if entry.PositionQueueLabel != expected || entry.RecentRanked.QueueLabel != expected {
			t.Fatalf("KR queue %s labels = %q / %q, want %q", key, entry.PositionQueueLabel, entry.RecentRanked.QueueLabel, expected)
		}
	}
	if _, exists := queues["2300"]; exists {
		t.Fatal("KR path must never produce a mayhem tab")
	}
}

// 海斗页签只在真的有样本时出现：没有海斗场次的玩家不该看到一个空页签。
func TestR116EMayhemTabOnlyAppearsWhenSamplesExist(t *testing.T) {
	if tabs := gameplayRankedQueueTabs(nil, nil); len(tabs) != 2 {
		t.Fatalf("tabs = %d, want 2 when there is no mayhem sample", len(tabs))
	}
	onlyClassic := []seasonRankedMatch{{GameID: 1, CreatedAt: 1, QueueID: 420}, {GameID: 2, CreatedAt: 2, QueueID: 3220}}
	if tabs := gameplayRankedQueueTabs(nil, onlyClassic); len(tabs) != 2 {
		t.Fatalf("tabs = %d, want 2: 3220 is plain ARAM, not hextech mayhem", len(tabs))
	}
	withMayhem := append(onlyClassic, seasonRankedMatch{GameID: 3, CreatedAt: 3, QueueID: 3270})
	tabs := gameplayRankedQueueTabs(nil, withMayhem)
	if len(tabs) != 3 {
		t.Fatalf("tabs = %d, want 3 once a mayhem sample exists", len(tabs))
	}
	if len(tabs[2].Cached) != 1 || tabs[2].Cached[0].GameID != 3 {
		t.Fatalf("mayhem tab must only carry mayhem samples: %#v", tabs[2].Cached)
	}
}

// recentRankedSummaryForQueue 的早退以前会把「不分队列的混合汇总」当成某个具体
// 队列的结果返回。放开海斗之后那就是伪造口径，必须按队列过滤。
func TestR116ERecentRankedSummaryForQueueNeverMixesQueues(t *testing.T) {
	cached := r116eSeasonCache()
	mayhem := recentRankedSummaryForQueue(nil, "subject", cached, 2400)
	if mayhem.Games != 1 || mayhem.Wins != 1 {
		t.Fatalf("queue 2400 summary = %#v, want exactly its own single game", mayhem)
	}
	if mayhem.QueueLabel != "海克斯大乱斗" {
		t.Fatalf("queue 2400 label = %q, want 海克斯大乱斗", mayhem.QueueLabel)
	}
	// 多队列版本：三个海斗 ID 合成一个页签。
	grouped := recentRankedSummaryForQueues(nil, "subject", cached, seasonMayhemQueueIDs, "海克斯大乱斗")
	if grouped.Games != 3 || grouped.Wins != 2 {
		t.Fatalf("grouped mayhem summary = %#v, want 3 games / 2 wins", grouped)
	}
	if grouped.QueueLabel != "海克斯大乱斗" {
		t.Fatalf("grouped label = %q", grouped.QueueLabel)
	}
	// 经典排位口径不变：recentRankedSummary 仍然只认 420/440。
	classic := recentRankedSummary(nil, "subject", cached)
	// recentRankedSummary 会先选定队列（有 420 就选 420），所以这里只剩那场 420。
	if classic.Games != 1 || classic.Wins != 1 {
		t.Fatalf("classic summary = %#v, want only the 420 game", classic)
	}
	if classic.QueueID != 420 || classic.QueueLabel != "单双排" {
		t.Fatalf("classic summary must still prefer solo/duo: %#v", classic)
	}
	// 对抗变异：把 recentRankedSummary 的队列门禁放开成「所有队列都收」，
	// 下面这条会立刻 FAIL。只有海斗（和极地大乱斗）样本时，「近 20 场排位」
	// 必须一场都不算——否则那张写着「排位」的卡片会显示海斗的数字。
	mayhemOnly := []seasonRankedMatch{
		{GameID: 1, CreatedAt: 1, QueueID: 2400, Win: true, Kills: 12, Deaths: 4, Assists: 9},
		{GameID: 2, CreatedAt: 2, QueueID: 3220, Win: true, Kills: 20, Deaths: 0, Assists: 0},
	}
	if got := recentRankedSummary(nil, "subject", mayhemOnly); got.Games != 0 || got.QueueID != 0 {
		t.Fatalf("classic summary must ignore mayhem-only samples: %#v", got)
	}
}

// rankedQueueLabel 是后端唯一的队列名解析点；未知队列返回空串而不是编一个名字。
func TestR116ERankedQueueLabelCoversMayhemAndRefusesUnknownQueues(t *testing.T) {
	cases := map[int64]string{
		420: "单双排", 440: "灵活组排",
		2300: "海克斯大乱斗", 2400: "海克斯大乱斗", 3270: "海克斯大乱斗",
		3220: "", 450: "", 1700: "", 0: "", -1: "",
	}
	for queueID, expected := range cases {
		if got := rankedQueueLabel(queueID); got != expected {
			t.Fatalf("rankedQueueLabel(%d) = %q, want %q", queueID, got, expected)
		}
	}
	if !seasonClassicRankedQueue(420) || !seasonClassicRankedQueue(440) {
		t.Fatal("420/440 must stay classic ranked")
	}
	for _, queueID := range seasonMayhemQueueIDs {
		if seasonClassicRankedQueue(queueID) {
			t.Fatalf("mayhem queue %d must not be labelled ranked", queueID)
		}
	}
}

// championStats 是「英雄胜率」卡片在赛季缓存不可用时的兜底，卡片标题写着
// 「本赛季 · N 场排位」，所以它必须继续只认 420/440——放开会把海斗场次
// 混进一张写着「排位」的卡片。
func TestR116EChampionStatsStaysRankedOnly(t *testing.T) {
	matches := []gameplayMatch{
		r116eMatch(1, 420, "win", "top"),
		r116eMatch(2, 2400, "win", ""),
		r116eMatch(3, 2300, "loss", ""),
		r116eMatch(4, 3220, "win", ""),
	}
	stats := championStats(matches, "subject", map[int64]string{157: "亚索"})
	total := 0
	for _, item := range stats {
		total += item.Games
	}
	if total != 1 {
		t.Fatalf("championStats counted %d games, want only the 420 one", total)
	}
}

// ---------------------------------------------------------------------------
// P2-6 静态查询的 HTTP 出口
// ---------------------------------------------------------------------------

func r116eMayhemBuildsApp(t *testing.T, samples []seasonAugmentSample) (*app, string) {
	t.Helper()
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	subject := &app{storage: store, lcu: &LCUClient{}, summoner: Summoner{PUUID: "subject"}}
	subject.mu.Lock()
	subject.connected = true
	subject.mu.Unlock()
	t.Cleanup(func() {
		subject.mu.Lock()
		subject.connected = false
		subject.mu.Unlock()
	})
	season, _ := currentRankedSeason(seasonStartS26.Add(48 * time.Hour))
	cache := seasonStatsCache{
		SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource,
		Season: season, AccountHash: store.accountHash(subject.summoner),
		AugmentSamples: samples, Complete: true,
	}
	if err := store.saveSeasonStats(cache); err != nil {
		t.Fatalf("saving the synthetic season cache failed: %v", err)
	}
	return subject, season
}

func r116eMayhemBuildsRequest(t *testing.T, subject *app, query string) seasonMayhemBuildsResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	subject.handleGameplaySeasonMayhemBuilds(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/season-mayhem-builds"+query, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response seasonMayhemBuildsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decoding the response failed: %v (%s)", err, recorder.Body.String())
	}
	return response
}

// P3 判据 2：给定一个 augment ID，能聚合出「最常出的装备组合 + 场次 + 胜率」。
func TestR116ESeasonMayhemBuildsEndpointAggregatesPersonalSamples(t *testing.T) {
	samples := r116eAugmentSamples(157, 5001, 12, 7, []int64{6672, 3153, 3031})
	samples = append(samples, seasonAugmentSample{GameID: 900, ChampionID: 157, AugmentIDs: []int64{5002}, ItemIDs: []int64{6672}, Win: true})
	samples = append(samples, r116eAugmentSamples(222, 5001, 40, 40, []int64{1111})...)
	subject, season := r116eMayhemBuildsApp(t, samples)

	response := r116eMayhemBuildsRequest(t, subject, "?championId=157&augmentId=5001")
	if !response.Available {
		t.Fatalf("expected an available report, got %#v", response)
	}
	if response.Scope != "current-account" || response.Season != season || response.ChampionID != 157 || response.AugmentID != 5001 {
		t.Fatalf("unexpected envelope: %#v", response)
	}
	if len(response.Groups) != 1 || response.Groups[0].Games != 12 || response.Groups[0].Wins != 7 {
		t.Fatalf("unexpected groups: %#v", response.Groups)
	}
	combo := response.Groups[0].Combos[0]
	if combo.Games != 12 || combo.WinRate != 58 || combo.Share != 100 {
		t.Fatalf("unexpected combo: %#v", combo)
	}
	// 别的英雄的样本不能混进来（详情页是英雄语境）。
	if response.SampleGames != 13 {
		t.Fatalf("sample games = %d, want 13 (champion 157 only)", response.SampleGames)
	}
}

// P3 判据 2 的另一半：样本不足时 available=false，前端整块不渲染。
func TestR116ESeasonMayhemBuildsEndpointHidesInsufficientSamples(t *testing.T) {
	subject, _ := r116eMayhemBuildsApp(t, r116eAugmentSamples(157, 5001, 4, 4, []int64{6672}))
	response := r116eMayhemBuildsRequest(t, subject, "?championId=157")
	if response.Available || len(response.Groups) != 0 {
		t.Fatalf("4 games must stay hidden: %#v", response)
	}
	if response.Reason == "" {
		t.Fatal("an unavailable report must carry a reason so the UI never guesses")
	}
	if response.MinimumSample != seasonAugmentMinimumSample || response.SampleGames != 4 {
		t.Fatalf("unexpected counters: %#v", response)
	}
	// 门槛可配置：调到 3 场之后同一份缓存就能出结果。
	configured := r116eMayhemBuildsRequest(t, subject, "?championId=157&minimumSample=3")
	if !configured.Available || len(configured.Groups) != 1 {
		t.Fatalf("configured threshold was ignored: %#v", configured)
	}
	if configured.MinimumSample != 3 {
		t.Fatalf("minimumSample = %d, want 3", configured.MinimumSample)
	}
}

// 参数校验与「读不到就明说」：非法入参 400，没连客户端 / 没有缓存都返回
// available=false 加原因，绝不拿最近 20 场代替、不猜、不编。
func TestR116ESeasonMayhemBuildsEndpointRejectsBadInputAndReportsMissingData(t *testing.T) {
	subject, _ := r116eMayhemBuildsApp(t, nil)
	for _, query := range []string{"", "?championId=0", "?championId=abc", "?championId=157&augmentId=0", "?championId=157&augmentId=x", "?championId=157&minimumSample=0"} {
		recorder := httptest.NewRecorder()
		subject.handleGameplaySeasonMayhemBuilds(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/season-mayhem-builds"+query, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("query %q: status = %d, want 400", query, recorder.Code)
		}
	}
	// 空缓存：读得到文件但没有任何海斗样本。
	if response := r116eMayhemBuildsRequest(t, subject, "?championId=157"); response.Available || response.Reason == "" {
		t.Fatalf("an empty cache must degrade loudly: %#v", response)
	}
	// 没有客户端连接：拿不到「本人」是谁，不能退化成扫全目录（那会混账号）。
	disconnected := &app{storage: subject.storage}
	recorder := httptest.NewRecorder()
	disconnected.handleGameplaySeasonMayhemBuilds(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/season-mayhem-builds?championId=157", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with available=false", recorder.Code)
	}
	var response seasonMayhemBuildsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if response.Available || response.Reason == "" {
		t.Fatalf("a disconnected client must degrade loudly: %#v", response)
	}
}
