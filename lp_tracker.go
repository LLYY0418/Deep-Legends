package main

// lp_tracker.go 记录当前登录玩家每场排位的胜点（LP）变化。
// 官方接口（LCU / Riot / SGP）都不提供“单场 LP 增减”，OP.GG 等网站
// 是靠服务器持续轮询实现的；本地助手改为监听客户端 gameflow 事件：
//
//  1. 平时读取排位数据时记录基线快照（段位 + 胜点 + 场次）；
//  2. 对局进入 EndOfGame 后轮询排位数据，等到胜负场次 +1 时
//     用绝对分数差得出这一场的 LP 变化（跨小段晋降级也成立）；
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
	lpHistorySchemaVersion = 1
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
	Games map[string]lpGameRecord `json:"games"`
}

type lpTracker struct {
	mu      sync.Mutex
	store   *localStore
	history lpHistoryData
	pending map[string]bool
}

func newLPTracker(store *localStore) *lpTracker {
	tracker := &lpTracker{store: store, pending: make(map[string]bool)}
	tracker.history = lpHistoryData{SchemaVersion: lpHistorySchemaVersion, Baselines: make(map[string]map[string]lpSnapshot), Games: make(map[string]lpGameRecord)}
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
	return tracker
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
	if history.SchemaVersion != lpHistorySchemaVersion || len(history.Games) > lpHistoryLimit {
		return false
	}
	for accountHash := range history.Baselines {
		if !validLPAccountHash(accountHash) {
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
func (t *lpTracker) observe(playerRef string, ranks []gameplayRank) {
	accountHash := t.accountHash(playerRef)
	if accountHash == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	changed := false
	for _, rank := range ranks {
		if rank.Tier == "" {
			continue
		}
		if _, tracked := lpRankedQueues[queueIDForRankedType(rank.QueueType)]; !tracked {
			continue
		}
		if t.pending[accountHash+"|"+rank.QueueType] {
			continue
		}
		byQueue := t.history.Baselines[accountHash]
		if byQueue == nil {
			byQueue = make(map[string]lpSnapshot)
			t.history.Baselines[accountHash] = byQueue
		}
		snapshot := lpSnapshotFromRank(rank)
		if byQueue[rank.QueueType] != snapshot {
			byQueue[rank.QueueType] = snapshot
			changed = true
		}
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

// handlePhase 由 LCU gameflow 事件触发：对局结算时启动一次捕获。
// loadRanks 由调用方注入（优先 SGP 段位数据：新版客户端 ranked-stats
// 不返回负场，输掉的排位靠它才能察觉场次 +1）。
func (t *lpTracker) handlePhase(client *LCUClient, phase, playerRef string, loadRanks func() ([]gameplayRank, EndpointCapability)) {
	if t == nil || client == nil || playerRef == "" || loadRanks == nil {
		return
	}
	if phase != "EndOfGame" && phase != "PreEndOfGame" && phase != "WaitingForStats" {
		return
	}
	go t.capture(client, playerRef, loadRanks)
}

type lpGameflowSession struct {
	GameData struct {
		GameID int64 `json:"gameId"`
		Queue  struct {
			ID int64 `json:"id"`
		} `json:"queue"`
	} `json:"gameData"`
}

func (t *lpTracker) capture(client *LCUClient, playerRef string, loadRanks func() ([]gameplayRank, EndpointCapability)) {
	accountHash := t.accountHash(playerRef)
	if accountHash == "" {
		return
	}
	var session lpGameflowSession
	if err := client.GetJSON("/lol-gameflow/v1/session", &session); err != nil {
		return
	}
	gameID := session.GameData.GameID
	queueType, ranked := lpRankedQueues[session.GameData.Queue.ID]
	if !ranked || gameID <= 0 {
		return
	}
	gameKey := strconv.FormatInt(gameID, 10)
	pendingKey := accountHash + "|" + queueType
	t.mu.Lock()
	_, recorded := t.history.Games[gameKey]
	if recorded || t.pending[pendingKey] {
		t.mu.Unlock()
		return
	}
	t.pending[pendingKey] = true
	baseline, hasBaseline := t.history.Baselines[accountHash][queueType]
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.pending, pendingKey)
		t.mu.Unlock()
	}()
	time.Sleep(lpCaptureFirstWait)
	for attempt := 0; attempt < lpCaptureAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(lpCaptureInterval)
		}
		ranks, capability := loadRanks()
		if capability.State != capabilityAvailable {
			continue
		}
		var current *gameplayRank
		for index := range ranks {
			if ranks[index].QueueType == queueType {
				current = &ranks[index]
				break
			}
		}
		if current == nil || current.Tier == "" {
			continue
		}
		snapshot := lpSnapshotFromRank(*current)
		// 结算数据尚未同步时场次不变，继续等待。
		if hasBaseline && snapshot.games() <= baseline.games() {
			continue
		}
		t.mu.Lock()
		byQueue := t.history.Baselines[accountHash]
		if byQueue == nil {
			byQueue = make(map[string]lpSnapshot)
			t.history.Baselines[accountHash] = byQueue
		}
		byQueue[queueType] = snapshot
		// 恰好多出一场才可靠归因；缺基线或跨越多场时只刷新基线。
		if hasBaseline && snapshot.games() == baseline.games()+1 {
			after, afterOK := rankAbsoluteScore(snapshot.Tier, snapshot.Division, snapshot.LeaguePoints)
			before, beforeOK := rankAbsoluteScore(baseline.Tier, baseline.Division, baseline.LeaguePoints)
			if afterOK && beforeOK {
				t.history.Games[gameKey] = lpGameRecord{AccountHash: accountHash, QueueType: queueType, Delta: after - before, RecordedAt: time.Now().UnixMilli()}
			}
		}
		t.persistLocked()
		t.mu.Unlock()
		return
	}
}
