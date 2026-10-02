package main

// lp_tracker.go 记录当前登录玩家每场排位的胜点（LP）变化。
// 官方接口（LCU / Riot / SGP）都不提供“单场 LP 增减”，OP.GG 等网站
// 是靠服务器持续轮询实现的；本地助手改为监听客户端 gameflow 事件：
//
//  1. 平时读取排位数据时记录基线快照（段位 + 胜点 + 场次）；
//  2. GameStart/InProgress 单独保存本局快照，结算时等待同源绝对分稳定；
//     没有开局快照时沿用胜负场次恰好 +1 的保守判断；
//  3. 结果按 gameId 与加盐脱敏账号标识存入本地 lp-history.json，
//     战绩列表读取时标注；稳定玩家标识不会写入文件。
//
// 只统计单双排（420）与灵活组排（440）；助手关闭期间进行的对局
// 无法回溯，只保留基线以保证下一场的差值正确。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	lpHistorySchemaVersion = 2
	lpHistoryFile          = "lp-history.json"
	lpHistoryLimit         = 400
	lpCaptureAttempts      = 18
	lpCaptureInterval      = 5 * time.Second
	lpCaptureFirstWait     = 2 * time.Second
)

var lpRankedQueues = map[int64]string{420: "RANKED_SOLO_5x5", 440: "RANKED_FLEX_SR"}

type lpSnapshot struct {
	Tier         string `json:"tier"`
	Division     string `json:"division"`
	LeaguePoints int    `json:"leaguePoints"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
}

func (s lpSnapshot) games() int { return s.Wins + s.Losses }

// trustworthy reports whether a snapshot is complete enough for LP differences.
// Tencent ranked responses with wins but no losses are known to be truncated.
func (s lpSnapshot) trustworthy() bool {
	return !(s.Wins > 0 && s.Losses == 0)
}

type lpGameRecord struct {
	AccountHash string `json:"accountHash"`
	QueueType   string `json:"queueType"`
	Delta       int    `json:"delta"`
	RecordedAt  int64  `json:"recordedAt"`
}

type lpHistoryData struct {
	SchemaVersion int `json:"schemaVersion"`
	// Baselines: 加盐脱敏账号标识 -> 排位队列 -> 最近一次已知快照。
	Baselines map[string]map[string]lpSnapshot `json:"baselines"`
	// Games: gameId -> 该场的胜点变化。
	Games        map[string]lpGameRecord                   `json:"games"`
	BaselineInfo map[string]map[string]lpBaselineInfo      `json:"baselineInfo,omitempty"`
	GameStarts   map[string]map[string]lpGameStartBaseline `json:"gameStarts,omitempty"`
}

type lpTracker struct {
	mu                  sync.Mutex
	store               *localStore
	history             lpHistoryData
	pending             map[string]bool
	captureIndex        uint64
	observeEvent        func(map[string]any)
	sleep               func(time.Duration)
	now                 func() time.Time
	startSeen           map[string]bool
	startFlights        map[string]chan struct{}
	baselineDiagnostics map[string]lpBaselineDiagnostic
	staleRejections     map[string]time.Time
	invalidateRanks     func(string)
}

func newLPTracker(store *localStore) *lpTracker {
	tracker := &lpTracker{store: store, pending: make(map[string]bool), sleep: time.Sleep, now: time.Now, startSeen: make(map[string]bool), startFlights: make(map[string]chan struct{}), baselineDiagnostics: make(map[string]lpBaselineDiagnostic)}
	tracker.history = lpHistoryData{SchemaVersion: lpHistorySchemaVersion, Baselines: make(map[string]map[string]lpSnapshot), Games: make(map[string]lpGameRecord), BaselineInfo: make(map[string]map[string]lpBaselineInfo), GameStarts: make(map[string]map[string]lpGameStartBaseline)}
	if store == nil {
		return tracker
	}
	store.mu.Lock()
	historyPath := filepath.Join(store.root, lpHistoryFile)
	data, err := os.ReadFile(historyPath)
	store.mu.Unlock()
	if err != nil {
		return tracker
	}
	var loaded lpHistoryData
	if len(data) > 1<<20 || json.Unmarshal(data, &loaded) != nil || !validLPHistory(loaded) {
		// 旧格式直接以 PUUID 为键。为兑现“不落盘稳定账号标识”的承诺，
		// 不迁移也不保留旧内容，而是立即覆盖为空的新格式。
		if err := tracker.persistLocked(); err != nil {
			store.mu.Lock()
			_ = os.Remove(historyPath)
			store.mu.Unlock()
		}
		return tracker
	}
	if loaded.Baselines != nil {
		tracker.history.Baselines = loaded.Baselines
	}
	if loaded.Games != nil {
		tracker.history.Games = loaded.Games
	}
	if loaded.BaselineInfo != nil {
		tracker.history.BaselineInfo = loaded.BaselineInfo
	}
	if loaded.GameStarts != nil {
		tracker.history.GameStarts = loaded.GameStarts
	}
	return tracker
}

func (t *lpTracker) recordObservation(event map[string]any) {
	if t != nil && t.observeEvent != nil {
		t.observeEvent(event)
	}
}

func (t *lpTracker) wait(duration time.Duration) {
	if t != nil && t.sleep != nil {
		t.sleep(duration)
		return
	}
	time.Sleep(duration)
}

func validLPAccountHash(value string) bool {
	if len(value) != 16 {
		return false
	}
	for _, character := range []byte(value) {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validLPHistory(history lpHistoryData) bool {
	if (history.SchemaVersion != 1 && history.SchemaVersion != lpHistorySchemaVersion) || len(history.Games) > lpHistoryLimit {
		return false
	}
	for accountHash := range history.Baselines {
		if !validLPAccountHash(accountHash) {
			return false
		}
	}
	for hash, byQueue := range history.GameStarts {
		if !validLPAccountHash(hash) {
			return false
		}
		for queue, baseline := range byQueue {
			if queueIDForRankedType(queue) == 0 || baseline.QueueType != queue || baseline.GameID <= 0 || baseline.TakenAt <= 0 || (baseline.Source != "lcu" && baseline.Source != "sgp") {
				return false
			}
		}
	}
	for hash := range history.BaselineInfo {
		if !validLPAccountHash(hash) {
			return false
		}
	}
	for _, record := range history.Games {
		if !validLPAccountHash(record.AccountHash) {
			return false
		}
	}
	return true
}

func (t *lpTracker) persistLocked() error {
	if t.store == nil {
		return nil
	}
	// 超出上限时按记录时间保留最新的一批。
	if len(t.history.Games) > lpHistoryLimit {
		type keyed struct {
			key    string
			record lpGameRecord
		}
		items := make([]keyed, 0, len(t.history.Games))
		for key, record := range t.history.Games {
			items = append(items, keyed{key, record})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].record.RecordedAt > items[j].record.RecordedAt })
		trimmed := make(map[string]lpGameRecord, lpHistoryLimit)
		for _, item := range items[:lpHistoryLimit] {
			trimmed[item.key] = item.record
		}
		t.history.Games = trimmed
	}
	t.history.SchemaVersion = lpHistorySchemaVersion
	data, err := json.Marshal(t.history)
	if err != nil {
		return err
	}
	t.store.mu.Lock()
	err = atomicWriteFile(filepath.Join(t.store.root, lpHistoryFile), data, 0o600)
	t.store.mu.Unlock()
	return err
}

func (t *lpTracker) accountHash(playerRef string) string {
	if t == nil || t.store == nil {
		return ""
	}
	playerRef = strings.TrimSpace(playerRef)
	if playerRef == "" {
		return ""
	}
	return t.store.accountHash(Summoner{PUUID: playerRef})
}

func lpSnapshotFromRank(rank gameplayRank) lpSnapshot {
	return lpSnapshot{Tier: rank.Tier, Division: rank.Division, LeaguePoints: rank.LeaguePoints, Wins: rank.Wins, Losses: rank.Losses}
}

// observe 在读取到当前登录玩家的排位数据时刷新基线。
// 正在等待结算捕获的队列跳过，避免赛后快照顶掉赛前基线。
func (t *lpTracker) observe(playerRef string, ranks []gameplayRank, capabilities ...EndpointCapability) {
	accountHash := t.accountHash(playerRef)
	if accountHash == "" {
		return
	}
	source := ""
	if len(capabilities) > 0 {
		source = lpRankSource(capabilities[0])
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	changed := false
	for _, rank := range ranks {
		if rank.Tier == "" || queueIDForRankedType(rank.QueueType) == 0 {
			continue
		}
		snapshot := lpSnapshotFromRank(rank)
		if rank.seasonFallback {
			t.baselineObservationLocked(accountHash, rank.QueueType, snapshot, "observe", true, "season_fallback")
			continue
		}
		if !snapshot.trustworthy() {
			t.recordObservation(map[string]any{"event": "lp_snapshot_rejected", "stage": "observe", "reason": "missing_losses"})
			continue
		}
		if t.pending[accountHash+"|"+rank.QueueType] {
			continue
		}
		if baseline, ok := t.history.Baselines[accountHash][rank.QueueType]; ok && snapshot.games() < baseline.games() {
			key := accountHash + "|" + rank.QueueType
			now := t.now()
			if t.staleRejections == nil {
				t.staleRejections = make(map[string]time.Time)
			}
			if last, seen := t.staleRejections[key]; !seen || now.Sub(last) >= time.Minute {
				t.staleRejections[key] = now
				t.recordObservation(map[string]any{"event": "lp_snapshot_rejected", "stage": "observe", "reason": "stale_regression"})
			}
			continue
		}
		// Queue observations may advance; the independent game-start snapshot is immutable.
		t.writeQueueBaselineLocked(accountHash, rank.QueueType, snapshot, "observe", source)
		changed = true
	}
	if changed {
		t.persistLocked()
	}
}

func queueIDForRankedType(queueType string) int64 {
	for id, name := range lpRankedQueues {
		if name == queueType {
			return id
		}
	}
	return 0
}

// annotate 把已记录的胜点变化写进属于该玩家的战绩。
func (t *lpTracker) annotate(matches []gameplayMatch, playerRef string) {
	accountHash := t.accountHash(playerRef)
	if accountHash == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for index := range matches {
		record, ok := t.history.Games[strconv.FormatInt(matches[index].GameID, 10)]
		if !ok || record.AccountHash != accountHash {
			continue
		}
		delta := record.Delta
		matches[index].LpDelta = &delta
	}
}

// handlePhase 由 LCU gameflow 事件触发开局快照与结算捕获。
// 两阶段共用调用方注入的 loadRanks；没有开局快照时保留场次 +1 判断。
func (t *lpTracker) handlePhase(client *LCUClient, phase, playerRef string, loadRanks func() ([]gameplayRank, EndpointCapability)) {
	if t == nil || client == nil || playerRef == "" || loadRanks == nil {
		return
	}
	if phase == "GameStart" || phase == "InProgress" {
		goSafe("lp_tracker.gameStart", func() { t.takeGameStart(client, playerRef, loadRanks) })
		return
	}
	if phase != "EndOfGame" && phase != "PreEndOfGame" && phase != "WaitingForStats" {
		return
	}
	goSafe("lp_tracker.handlePhase.1", func() { t.capture(client, playerRef, loadRanks) })
}

type lpGameflowSession struct {
	GameData struct {
		GameID int64 `json:"gameId"`
		Queue  struct {
			ID int64 `json:"id"`
		} `json:"queue"`
	} `json:"gameData"`
}

type lpBaselineInfo struct {
	Writer  string `json:"writer"`
	Source  string `json:"source,omitempty"`
	TakenAt int64  `json:"takenAt"`
}
type lpGameStartBaseline struct {
	GameID    int64      `json:"gameId"`
	QueueType string     `json:"queueType"`
	Snapshot  lpSnapshot `json:"snapshot"`
	Source    string     `json:"source"`
	TakenAt   int64      `json:"takenAt"`
}
type lpBaselineDiagnostic struct {
	Snapshot       lpSnapshot
	SeasonFallback bool
	Reason         string
	At             time.Time
}

func lpRankSource(capability EndpointCapability) string {
	source := capabilitySource(capability)
	if source == dataSourceLCU || source == dataSourceSGP {
		return source
	}
	return ""
}
func lpScore(snapshot lpSnapshot) (int, bool) {
	return rankAbsoluteScore(snapshot.Tier, snapshot.Division, snapshot.LeaguePoints)
}
func lpDeltaFields(snapshot, baseline lpSnapshot, known bool) map[string]any {
	fields := map[string]any{"wins_delta": nil, "losses_delta": nil, "score_delta": nil}
	if known {
		fields["wins_delta"] = snapshot.Wins - baseline.Wins
		fields["losses_delta"] = snapshot.Losses - baseline.Losses
		after, afterOK := lpScore(snapshot)
		before, beforeOK := lpScore(baseline)

		if afterOK && beforeOK {
			fields["score_delta"] = after - before
		}
	}
	return fields
}
func (t *lpTracker) baselineObservationLocked(hash, queue string, snapshot lpSnapshot, writer string, fallback bool, reason string) {
	key := hash + "|" + queue
	now := t.now()
	last, exists := t.baselineDiagnostics[key]
	if exists && last.Snapshot == snapshot && last.SeasonFallback == fallback && last.Reason == reason && now.Sub(last.At) < time.Minute {
		return
	}
	t.baselineDiagnostics[key] = lpBaselineDiagnostic{snapshot, fallback, reason, now}
	baseline, known := t.history.Baselines[hash][queue]
	event := lpDeltaFields(snapshot, baseline, known)
	event["event"] = "lp_baseline_written"
	event["writer"] = writer
	event["season_fallback"] = fallback
	event["has_baseline"] = known
	if reason != "" {
		event["reason"] = reason
	}
	t.recordObservation(event)
}
func (t *lpTracker) writeQueueBaselineLocked(hash, queue string, snapshot lpSnapshot, writer, source string) {
	t.baselineObservationLocked(hash, queue, snapshot, writer, false, "")
	if t.history.Baselines[hash] == nil {
		t.history.Baselines[hash] = make(map[string]lpSnapshot)
	}
	if t.history.BaselineInfo[hash] == nil {
		t.history.BaselineInfo[hash] = make(map[string]lpBaselineInfo)
	}
	t.history.Baselines[hash][queue] = snapshot
	t.history.BaselineInfo[hash][queue] = lpBaselineInfo{Writer: writer, Source: source, TakenAt: t.now().UnixMilli()}
}
func lpCurrentSnapshot(ranks []gameplayRank, capability EndpointCapability, queue string) (lpSnapshot, bool) {
	if capability.State == capabilityAvailable {
		for _, rank := range ranks {
			if rank.QueueType == queue && rank.Tier != "" && !rank.seasonFallback {
				return lpSnapshotFromRank(rank), true
			}
		}
	}
	return lpSnapshot{}, false
}
func (t *lpTracker) takeGameStart(client *LCUClient, playerRef string, loadRanks func() ([]gameplayRank, EndpointCapability)) {
	hash := t.accountHash(playerRef)
	if hash == "" {
		return
	}
	var session lpGameflowSession
	if client.GetJSON("/lol-gameflow/v1/session", &session) != nil {
		t.recordObservation(map[string]any{"event": "lp_baseline_written", "writer": "game_start", "season_fallback": false, "reason": "session_unavailable"})
		return
	}
	gameID := session.GameData.GameID
	queue, ranked := lpRankedQueues[session.GameData.Queue.ID]
	if !ranked || gameID <= 0 {
		return
	}
	flightKey := hash + "|" + strconv.FormatInt(gameID, 10)
	t.mu.Lock()
	if t.startSeen[flightKey] || t.history.GameStarts[hash][queue].GameID == gameID {
		t.mu.Unlock()
		return
	}
	t.startSeen[flightKey] = true
	if len(t.startSeen) > lpHistoryLimit {
		t.startSeen = map[string]bool{flightKey: true}
	}
	done := make(chan struct{})
	t.startFlights[flightKey] = done
	t.mu.Unlock()
	defer func() { t.mu.Lock(); delete(t.startFlights, flightKey); close(done); t.mu.Unlock() }()
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			t.wait(lpCaptureInterval)
		}
		ranks, capability := loadRanks()
		snapshot, ok := lpCurrentSnapshot(ranks, capability, queue)
		source := lpRankSource(capability)
		t.recordObservation(map[string]any{"event": "lp_game_start_poll", "attempt": attempt, "capability_state": capability.State})
		if !ok || source == "" {
			continue
		}
		if !snapshot.trustworthy() {
			t.recordObservation(map[string]any{"event": "lp_snapshot_rejected", "stage": "game_start", "reason": "missing_losses"})
			continue
		}
		if _, ok = lpScore(snapshot); !ok {
			continue
		}
		t.mu.Lock()
		t.baselineObservationLocked(hash, queue, snapshot, "game_start", false, "")
		if t.history.GameStarts[hash] == nil {
			t.history.GameStarts[hash] = make(map[string]lpGameStartBaseline)
		}
		t.history.GameStarts[hash][queue] = lpGameStartBaseline{gameID, queue, snapshot, source, t.now().UnixMilli()}
		t.persistLocked()
		t.mu.Unlock()
		return
	}
	t.recordObservation(map[string]any{"event": "lp_baseline_written", "writer": "game_start", "season_fallback": false, "reason": "read_failed", "attempts": 3})
}

func (t *lpTracker) capture(client *LCUClient, playerRef string, loadRanks func() ([]gameplayRank, EndpointCapability)) {
	if t != nil && t.invalidateRanks != nil {
		defer t.invalidateRanks(playerRef)
	}
	hash := t.accountHash(playerRef)
	if hash == "" {
		t.recordObservation(map[string]any{"event": "lp_capture_ignored", "reason": "invalid_player_reference"})
		return
	}
	var session lpGameflowSession
	if client.GetJSON("/lol-gameflow/v1/session", &session) != nil {
		t.recordObservation(map[string]any{"event": "lp_capture_ignored", "reason": "session_unavailable"})
		return
	}
	gameID := session.GameData.GameID
	queue, ranked := lpRankedQueues[session.GameData.Queue.ID]
	if !ranked {
		t.recordObservation(map[string]any{"event": "lp_capture_ignored", "reason": "unranked_queue"})
		return
	}
	if gameID <= 0 {
		t.recordObservation(map[string]any{"event": "lp_capture_ignored", "reason": "invalid_game"})
		return
	}
	gameKey := strconv.FormatInt(gameID, 10)
	pendingKey := hash + "|" + queue
	t.mu.Lock()
	flight := t.startFlights[hash+"|"+gameKey]
	t.mu.Unlock()
	if flight != nil {
		<-flight
	}
	t.mu.Lock()
	_, recorded := t.history.Games[gameKey]
	if recorded || t.pending[pendingKey] {
		pending := t.pending[pendingKey]
		t.mu.Unlock()
		reason := "already_recorded"
		if pending {
			reason = "capture_pending"
		}
		t.recordObservation(map[string]any{"event": "lp_capture_ignored", "reason": reason})
		return
	}
	t.captureIndex++
	index := t.captureIndex
	t.pending[pendingKey] = true
	baseline, hasBaseline := t.history.Baselines[hash][queue]
	info := t.history.BaselineInfo[hash][queue]
	gameStart, hasStart := t.history.GameStarts[hash][queue]
	hasStart = hasStart && gameStart.GameID == gameID
	if hasStart {
		baseline = gameStart.Snapshot
		hasBaseline = true
		info = lpBaselineInfo{Writer: "game_start", Source: gameStart.Source, TakenAt: gameStart.TakenAt}
	}
	if info.Writer == "" {
		info.Writer = "observe"
	}
	t.mu.Unlock()
	t.recordObservation(map[string]any{"event": "lp_capture_started", "capture_index": index, "has_baseline": hasBaseline})
	var last lpSnapshot
	lastOK := false
	lastSource := ""
	diagnostic := func(name, reason string, snapshot lpSnapshot, source string) map[string]any {
		event := lpDeltaFields(snapshot, baseline, hasBaseline)
		if hasStart && source != gameStart.Source {
			event["score_delta"] = nil
		}
		event["event"] = name
		event["capture_index"] = index
		event["has_baseline"] = hasBaseline
		event["baseline_source"] = info.Writer
		age := int64(0)
		if info.TakenAt > 0 {
			age = max(int64(0), (t.now().UnixMilli()-info.TakenAt)/1000)
		}
		event["baseline_age_s"] = age
		if source != "" {
			event["snapshot_source"] = source
		}
		if reason != "" {
			event["reason"] = reason
		}
		if hasBaseline {
			event["games_gap"] = snapshot.games() - baseline.games()
		}
		return event
	}
	defer func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		// Keep the final upstream observation for future fallback captures, even on timeout.
		if lastOK {
			t.writeQueueBaselineLocked(hash, queue, last, "capture", lastSource)
		}
		if start := t.history.GameStarts[hash][queue]; start.GameID == gameID {
			delete(t.history.GameStarts[hash], queue)
		}
		delete(t.pending, pendingKey)
		t.persistLocked()
	}()
	t.wait(lpCaptureFirstWait)
	firstChanged := false
	settleSamples := 0
	previousScore := 0
	previousValid := false
	seenMismatch := false
	seenSameSource := false
	debugRemaining := 0
	debugStarted := false
	maxPolls := lpCaptureAttempts
	for attempt := 1; attempt <= maxPolls; attempt++ {
		if attempt > 1 {
			t.wait(lpCaptureInterval)
		}
		ranks, capability := loadRanks()
		snapshot, ok := lpCurrentSnapshot(ranks, capability, queue)
		source := lpRankSource(capability)
		t.recordObservation(map[string]any{"event": "lp_capture_poll", "capture_index": index, "attempt": attempt, "capability_state": capability.State})
		if firstChanged || debugRemaining > 0 {
			if debugRemaining > 0 {
				debugRemaining--
			}
			event := map[string]any{"event": "lp_capture_settle", "capture_index": index, "attempt": attempt, "capability_state": capability.State}
			if ok {
				event = diagnostic("lp_capture_settle", "", snapshot, source)
				event["attempt"] = attempt
				event["capability_state"] = capability.State
			}
			t.recordObservation(event)
		}
		if !ok {
			previousValid = false
			continue
		}
		if !snapshot.trustworthy() {
			t.recordObservation(map[string]any{"event": "lp_snapshot_rejected", "stage": "capture", "capture_index": index, "reason": "missing_losses"})
			previousValid = false
			continue
		}
		last, lastOK, lastSource = snapshot, true, source
		if hasStart && source != gameStart.Source {
			seenMismatch = true
			previousValid = false
			continue
		}
		seenSameSource = true
		after, afterOK := lpScore(snapshot)
		before, beforeOK := lpScore(baseline)

		if !hasStart {
			if hasBaseline && !debugStarted && (snapshot.Wins != baseline.Wins || snapshot.Losses != baseline.Losses || afterOK && beforeOK && after != before) {
				debugStarted = true
				debugRemaining = 2
			}
			if hasBaseline && snapshot.games() <= baseline.games() {
				continue
			}
			// Preserve the old decision from the first eligible snapshot. Follow-up
			// samples diagnose upstream changes; they must not rewrite that decision.
			reason := ""
			switch {
			case !hasBaseline:
				reason = "no_baseline"
			case snapshot.games() > baseline.games()+1:
				reason = "games_jumped"
			case !afterOK || !beforeOK:
				reason = "score_unresolved"
			}
			for settle := 1; settle <= 2; settle++ {
				t.wait(lpCaptureInterval)
				nextRanks, nextCapability := loadRanks()
				next, nextOK := lpCurrentSnapshot(nextRanks, nextCapability, queue)
				nextSource := lpRankSource(nextCapability)
				event := map[string]any{"event": "lp_capture_settle", "capture_index": index, "attempt": attempt + settle, "capability_state": nextCapability.State}
				if nextOK {
					event = diagnostic("lp_capture_settle", "", next, nextSource)
					event["attempt"] = attempt + settle
					event["capability_state"] = nextCapability.State
					if next.trustworthy() {
						last, lastOK, lastSource = next, true, nextSource
					}
				}
				t.recordObservation(event)
			}
			if reason != "" {
				t.recordObservation(diagnostic("lp_capture_skipped", reason, snapshot, source))
				return
			}
			t.mu.Lock()
			t.history.Games[gameKey] = lpGameRecord{hash, queue, after - before, t.now().UnixMilli()}
			t.mu.Unlock()
			t.recordObservation(diagnostic("lp_capture_recorded", "", snapshot, source))
			return
		}
		if !firstChanged {
			if hasBaseline {
				if hasStart {
					if snapshot.Wins == baseline.Wins && snapshot.Losses == baseline.Losses && afterOK && beforeOK && after == before {
						continue
					}
				}
			}
			firstChanged = true
			previousScore = after
			previousValid = afterOK
			// P1 requests two follow-up observations. P2 permits at most three
			// comparable follow-ups to find consecutive equal scores. Allow these at
			// the waiting budget's boundary too, without restarting the 18-poll wait.
			maxPolls = max(maxPolls, attempt+3)
			continue
		}
		settleSamples++
		stable := previousValid && afterOK && after == previousScore
		previousScore, previousValid = after, afterOK
		if settleSamples < 2 {
			continue
		}
		if hasStart && !stable && settleSamples < 3 {
			continue
		}
		reason := ""
		delta := 0
		switch {
		case !hasBaseline:
			reason = "no_baseline"
		case !afterOK || !beforeOK:
			reason = "score_unresolved"
		case hasStart && !stable:
			reason = "score_unstable"
		default:
			delta = after - before
		}
		if reason != "" {
			t.recordObservation(diagnostic("lp_capture_skipped", reason, snapshot, source))
			return
		}
		t.mu.Lock()
		t.history.Games[gameKey] = lpGameRecord{hash, queue, delta, t.now().UnixMilli()}
		t.mu.Unlock()
		t.recordObservation(diagnostic("lp_capture_recorded", "", snapshot, source))
		return
	}
	if hasStart && seenMismatch && !seenSameSource {
		t.recordObservation(diagnostic("lp_capture_skipped", "source_mismatch", last, lastSource))
		return
	}
	t.recordObservation(map[string]any{"event": "lp_capture_timeout", "capture_index": index, "attempts": maxPolls})
}
