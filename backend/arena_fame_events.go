package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const arenaFameEventGroup = "14cc5647-26d6-4139-9745-1772f44a2476"

type arenaFameEventRecord struct {
	Account    string    `json:"account"`
	Platform   string    `json:"platform"`
	Fame       int       `json:"fame"`
	Level      int       `json:"level"`
	Thresholds []int     `json:"thresholds"`
	ObservedAt time.Time `json:"observedAt"`
	Delta      *int      `json:"delta,omitempty"`
	GameID     int64     `json:"gameId,omitempty"`
}

func arenaFameEventValue(event LCUEvent) (string, int, []int, bool) {
	var v struct {
		GroupID  string `json:"groupId"`
		ID       string `json:"id"`
		Counters []struct {
			Value *float64 `json:"counterValue"`
		} `json:"counters"`
		Milestones []struct {
			Value *float64 `json:"triggerValue"`
		} `json:"milestones"`
	}
	if json.Unmarshal(event.Data, &v) != nil || len(v.Counters) == 0 || v.Counters[0].Value == nil {
		return "", 0, nil, false
	}
	value := *v.Counters[0].Value
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 2147483647 || value != math.Trunc(value) {
		return "", 0, nil, false
	}
	group := v.GroupID
	if group == "" {
		group = v.ID
	}
	parts := strings.Split(event.URI, "/")
	for i, p := range parts {
		if p == "groups" && i+1 < len(parts) {
			group = parts[i+1]
			break
		}
	}
	thresholds := []int{}
	for _, m := range v.Milestones {
		if m.Value == nil || *m.Value <= 0 || *m.Value > 2147483647 || *m.Value != math.Trunc(*m.Value) {
			return "", 0, nil, false
		}
		thresholds = append(thresholds, int(*m.Value))
	}
	if len(thresholds) == 0 || len(thresholds) > 128 {
		return "", 0, nil, false
	}
	if group != arenaFameEventGroup {
		for i := 1; i < len(thresholds); i++ {
			if thresholds[i] <= thresholds[i-1] {
				return "", 0, nil, false
			}
		}
	}
	sort.Ints(thresholds)
	for i := 1; i < len(thresholds); i++ {
		if thresholds[i] == thresholds[i-1] {
			return "", 0, nil, false
		}
	}
	return group, int(value), thresholds, true
}
func arenaLevelFromMilestones(fame int, thresholds []int) int {
	level := 0
	for _, threshold := range thresholds {
		if fame >= threshold {
			level++
		}
	}
	return level
}
func (a *app) bindArenaFameEvents(c *LCUClient) {
	c.diagnosticMu.Lock()
	c.arenaFameObserve = func(event LCUEvent) { a.observeOwnArenaFame(c, event) }
	c.diagnosticMu.Unlock()
}
func (a *app) arenaFameEventIdentity(c *LCUClient) (string, string) {
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
func readArenaFameEvents(store *localStore) map[string]arenaFameEventRecord {
	records := map[string]arenaFameEventRecord{}
	raw, err := readLocalStoreFile(store, "arena-fame-events.json")
	if err == nil && len(raw) <= 256*1024 {
		_ = json.Unmarshal(raw, &records)
	}
	if records == nil {
		records = map[string]arenaFameEventRecord{}
	}
	return records
}
func (a *app) observeOwnArenaFame(c *LCUClient, event LCUEvent) {
	group, fame, thresholds, ok := arenaFameEventValue(event)
	if !ok {
		return
	}
	account, platform := a.arenaFameEventIdentity(c)
	if account == "" {
		return
	}
	key := account + "|" + platform
	s := c.arenaDiscoveryState()
	s.mu.Lock()
	gameID := int64(0)
	if s.progression != nil {
		gameID = s.progression.gameID
	}
	s.mu.Unlock()
	a.storage.mu.Lock()
	records := readArenaFameEvents(a.storage)
	previous, hasPrevious := records[key]
	if group != arenaFameEventGroup {
		a.storage.mu.Unlock()
		if len(thresholds) >= 8 && (!hasPrevious || fame >= previous.Fame) {
			s.mu.Lock()
			changed := s.fameGroupChanged
			s.fameGroupChanged = true
			s.mu.Unlock()
			if !changed {
				a.recordDiagnostic(map[string]any{"event": "arena_fame_group_changed"})
			}
		}
		return
	}
	sameThresholds := len(previous.Thresholds) == len(thresholds)
	if sameThresholds {
		for i, v := range thresholds {
			if previous.Thresholds[i] != v {
				sameThresholds = false
				break
			}
		}
	}
	if hasPrevious && previous.Fame == fame && sameThresholds && previous.GameID == gameID {
		a.storage.mu.Unlock()
		return
	}
	record := arenaFameEventRecord{Account: account, Platform: platform, Fame: fame, Level: arenaLevelFromMilestones(fame, thresholds), Thresholds: thresholds, ObservedAt: time.Now().UTC(), GameID: gameID}
	if hasPrevious {
		delta := fame - previous.Fame
		record.Delta = &delta
	}
	if len(records) >= 64 && !hasPrevious {
		for old := range records {
			delete(records, old)
			break
		}
	}
	records[key] = record
	raw, err := json.Marshal(records)
	if err == nil {
		err = writeLocalStoreFile(a.storage, "arena-fame-events.json", raw)
	}
	a.storage.mu.Unlock()
	if err != nil {
		return
	}
	mismatch := len(thresholds) != len(arenaFameLevelThresholds)
	if !mismatch {
		for i, v := range thresholds {
			if v != arenaFameLevelThresholds[i] {
				mismatch = true
				break
			}
		}
	}
	if mismatch {
		s.mu.Lock()
		logged := s.fameThresholdMismatch
		s.fameThresholdMismatch = true
		s.mu.Unlock()
		if !logged {
			client12, wiki12 := 0, 0
			if len(thresholds) >= 12 {
				client12 = thresholds[11]
			}
			if len(arenaFameLevelThresholds) >= 12 {
				wiki12 = arenaFameLevelThresholds[11]
			}
			a.recordDiagnostic(map[string]any{"event": "arena_fame_threshold_mismatch", "client_count": len(thresholds), "wiki_count": len(arenaFameLevelThresholds), "client_threshold_12": client12, "wiki_threshold_12": wiki12})
		}
	}
	if hasPrevious {
		a.recordDiagnostic(map[string]any{"event": "arena_fame_delta", "before": previous.Fame, "after": fame, "delta": fame - previous.Fame, "baseline_source": "previous_event", "game_id": gameID})
	}
}
func (a *app) ownArenaFame(c *LCUClient) *gameplayArenaFame {
	account, platform := a.arenaFameEventIdentity(c)
	if account == "" {
		return nil
	}
	a.storage.mu.Lock()
	record, ok := readArenaFameEvents(a.storage)[account+"|"+platform]
	a.storage.mu.Unlock()
	if !ok || record.Account != account || record.Platform != platform || len(record.Thresholds) == 0 || record.Fame < 0 || record.ObservedAt.IsZero() {
		return nil
	}
	level := arenaLevelFromMilestones(record.Fame, record.Thresholds)
	return &gameplayArenaFame{Fame: record.Fame, Level: level, Tier: arenaTierForLevel(level), Source: "progression_event", ObservedAt: &record.ObservedAt, LastDelta: record.Delta}
}

// Only schema key paths survive. Arrays never include indices or player IDs.
func arenaFameOtherPaths(raw json.RawMessage) []string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	paths := map[string]bool{}
	var walk func(any, string, int)
	walk = func(value any, path string, depth int) {
		if depth > 12 || len(paths) >= 128 {
			return
		}
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if arenaDiscoveryPrivate.MatchString(key) {
					continue
				}
				// Object maps may be keyed by opaque player/challenge IDs rather
				// than field names. Keep looking inside, without logging that key.
				field := key
				if len(key) >= 36 || !arenaDiscoveryIdentifier.MatchString(key) || strings.ContainsAny(key, "0123456789-") {
					field = "{key}"
				}
				next := field
				if path != "" {
					next = path + "." + field
				}
				lower := strings.ToLower(field)
				for _, word := range []string{"cherry", "arena", "fame", "prestige", "progression"} {
					if strings.Contains(lower, word) {
						paths[next] = true
						break
					}
				}
				walk(child, next, depth+1)
			}
		case []any:
			for _, child := range v {
				walk(child, path+"[]", depth+1)
			}
		}
	}
	walk(value, "", 0)
	out := []string{}
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}
func (a *app) scanOtherArenaFame(c *LCUClient, ref, source string, raw json.RawMessage) {
	if c == nil || ref == "" {
		return
	}
	s := c.arenaDiscoveryState()
	s.mu.Lock()
	if s.fameOtherRef == "" {
		s.fameOtherRef = ref
	}
	if s.fameOtherRef != ref {
		s.mu.Unlock()
		return
	}
	if s.fameOtherSources == nil {
		s.fameOtherSources = map[string]bool{}
	}
	if s.fameOtherSources[source] {
		s.mu.Unlock()
		return
	}
	s.fameOtherSources[source] = true
	s.mu.Unlock()
	paths := arenaFameOtherPaths(raw)
	a.recordDiagnostic(map[string]any{"event": "arena_fame_other_scan", "source": source, "paths": paths, "count": len(paths)})
}
func (a *app) probeOtherArenaChallenges(ctx context.Context, c *LCUClient, ref string) {
	if c == nil {
		return
	}
	s := c.arenaDiscoveryState()
	s.mu.Lock()
	if s.fameOtherRef != ref || s.fameOtherChallenges {
		s.mu.Unlock()
		return
	}
	s.fameOtherChallenges = true
	s.mu.Unlock()
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var raw json.RawMessage
	if c.RequestJSON(probe, http.MethodGet, "/lol-challenges/v1/summary-player-data/player/"+url.PathEscape(ref), nil, &raw) == nil {
		a.scanOtherArenaFame(c, ref, "challenges", raw)
	}
}
