package main

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

// R318 P3：只恢复「本人」斗魂名望的事件读取与持久化。
// 客户端没有读取接口（0.13.5 已确认 /lol-progression 组读取一律 404），
// 只有每局斗魂结束时由 LCU 事件推送 groupId=14cc5647-… 的计数器与里程碑。
// 这里不包含任何对他人的扫描、探测或名望弹窗逻辑。
const arenaSelfFameGroup = "14cc5647-26d6-4139-9745-1772f44a2476"

// 新文件名不能落进 retired_arena_cache.go 的清理名单。
const arenaSelfFameFile = "arena-self-fame.json"

type gameplayArenaSelfFame struct {
	Level int `json:"level"`
	Fame  int `json:"fame"`
}

type arenaSelfFameRecord struct {
	Account    string    `json:"account"`
	Platform   string    `json:"platform"`
	Fame       int       `json:"fame"`
	Level      int       `json:"level"`
	Thresholds []int     `json:"thresholds,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
}

// arenaSelfFameEventValue 只接受本人进度组的事件：计数器是名望值，
// 里程碑 triggerValue 是各级门槛（升序、去重、上限 128 条）。
func arenaSelfFameEventValue(event LCUEvent) (string, int, []int, bool) {
	var value struct {
		GroupID  string `json:"groupId"`
		ID       string `json:"id"`
		Counters []struct {
			Value *float64 `json:"counterValue"`
		} `json:"counters"`
		Milestones []struct {
			Value *float64 `json:"triggerValue"`
		} `json:"milestones"`
	}
	if json.Unmarshal(event.Data, &value) != nil || len(value.Counters) == 0 || value.Counters[0].Value == nil {
		return "", 0, nil, false
	}
	fame := *value.Counters[0].Value
	if math.IsNaN(fame) || math.IsInf(fame, 0) || fame < 0 || fame > 2147483647 || fame != math.Trunc(fame) {
		return "", 0, nil, false
	}
	group := value.GroupID
	if group == "" {
		group = value.ID
	}
	parts := strings.Split(event.URI, "/")
	for i, part := range parts {
		if part == "groups" && i+1 < len(parts) {
			group = parts[i+1]
			break
		}
	}
	thresholds := make([]int, 0, len(value.Milestones))
	for _, milestone := range value.Milestones {
		if milestone.Value == nil || *milestone.Value <= 0 || *milestone.Value > 2147483647 || *milestone.Value != math.Trunc(*milestone.Value) {
			return "", 0, nil, false
		}
		thresholds = append(thresholds, int(*milestone.Value))
	}
	if len(thresholds) == 0 || len(thresholds) > 128 {
		return "", 0, nil, false
	}
	sort.Ints(thresholds)
	for i := 1; i < len(thresholds); i++ {
		if thresholds[i] == thresholds[i-1] {
			return "", 0, nil, false
		}
	}
	return group, int(fame), thresholds, true
}

func arenaSelfFameLevel(fame int, thresholds []int) int {
	level := 0
	for _, threshold := range thresholds {
		if fame >= threshold {
			level++
		}
	}
	return level
}

func (a *app) arenaSelfFameIdentity(c *LCUClient) (string, string) {
	if a.storage == nil || c == nil {
		return "", ""
	}
	a.mu.RLock()
	self := a.summoner
	a.mu.RUnlock()
	if self.PUUID == "" {
		return "", ""
	}
	return a.storage.accountHash(self), clientRiotPlatform(c) + "/" + clientTencentServerID(c)
}

func readArenaSelfFameRecords(store *localStore) map[string]arenaSelfFameRecord {
	records := map[string]arenaSelfFameRecord{}
	raw, err := readLocalStoreFile(store, arenaSelfFameFile)
	if err == nil && len(raw) <= 256*1024 {
		_ = json.Unmarshal(raw, &records)
	}
	if records == nil {
		records = map[string]arenaSelfFameRecord{}
	}
	return records
}

// observeArenaSelfFameEvent 处理一局斗魂结束后的本人名望事件。
// 只在 groupId 匹配时处理；不匹配的一律忽略（没有他人扫描/探测）。
func (a *app) observeArenaSelfFameEvent(c *LCUClient, event LCUEvent) {
	if a.storage == nil || c == nil || !strings.Contains(strings.ToLower(event.URI), "progression") {
		return
	}
	group, fame, thresholds, ok := arenaSelfFameEventValue(event)
	if !ok || group != arenaSelfFameGroup {
		return
	}
	account, platform := a.arenaSelfFameIdentity(c)
	if account == "" {
		return
	}
	// 事件若带玩家标识，必须就是本人；不匹配的一律忽略（不做任何他人扫描）。
	var owner struct {
		PUUID      string `json:"puuid"`
		SummonerID int64  `json:"summonerId"`
	}
	if json.Unmarshal(event.Data, &owner) == nil && owner.PUUID != "" {
		a.mu.RLock()
		self := a.summoner
		a.mu.RUnlock()
		if self.PUUID != "" && owner.PUUID != self.PUUID {
			return
		}
	}
	key := account + "|" + platform
	a.storage.mu.Lock()
	records := readArenaSelfFameRecords(a.storage)
	previous, hasPrevious := records[key]
	if hasPrevious && previous.Fame == fame && previous.Level == arenaSelfFameLevel(fame, thresholds) {
		a.storage.mu.Unlock()
		return
	}
	record := arenaSelfFameRecord{Account: account, Platform: platform, Fame: fame, Level: arenaSelfFameLevel(fame, thresholds), Thresholds: thresholds, ObservedAt: time.Now().UTC()}
	if len(records) >= 64 && !hasPrevious {
		for old := range records {
			delete(records, old)
			break
		}
	}
	records[key] = record
	raw, err := json.Marshal(records)
	if err == nil {
		err = writeLocalStoreFile(a.storage, arenaSelfFameFile, raw)
	}
	a.storage.mu.Unlock()
	if err != nil {
		return
	}
	delta := 0
	if hasPrevious {
		delta = fame - previous.Fame
	}
	a.recordDiagnostic(map[string]any{"event": "arena_self_fame_observed", "fame": fame, "level": record.Level, "delta": delta})
}

func (a *app) ownArenaSelfFame(c *LCUClient) *gameplayArenaSelfFame {
	account, platform := a.arenaSelfFameIdentity(c)
	if account == "" || a.storage == nil {
		return nil
	}
	a.storage.mu.Lock()
	record, ok := readArenaSelfFameRecords(a.storage)[account+"|"+platform]
	a.storage.mu.Unlock()
	if !ok || record.Account != account || record.Platform != platform || record.Fame < 0 || record.Level < 0 || record.ObservedAt.IsZero() {
		return nil
	}
	return &gameplayArenaSelfFame{Level: record.Level, Fame: record.Fame}
}

// attachArenaSelfFame 只把本人名望放到本人那一行；其他玩家字段保持为空。
func (a *app) attachArenaSelfFame(c *LCUClient, response *gameplayLiveResponse) {
	if response == nil || len(response.Players) == 0 {
		return
	}
	fame := a.ownArenaSelfFame(c)
	if fame == nil {
		return
	}
	for i := range response.Players {
		if response.Players[i].IsCurrent {
			response.Players[i].ArenaFame = fame
		}
	}
}
