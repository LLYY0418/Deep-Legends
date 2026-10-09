package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type clientHistorySnapshot struct {
	Matches      []gameplayMatch      `json:"matches"`
	Capabilities []EndpointCapability `json:"capabilities"`
	Pagination   gameplayPagination   `json:"pagination"`
	HeadGameID   int64                `json:"headGameId,omitempty"`
}

type overviewHistoryHeadContextKey struct{}

func recordOverviewHistoryHead(ctx context.Context, id int64) {
	if head, ok := ctx.Value(overviewHistoryHeadContextKey{}).(*atomic.Int64); ok {
		head.CompareAndSwap(0, id)
	}
}

type clientHistoryFlight struct {
	done       chan struct{}
	result     clientHistorySnapshot
	generation uint64
}

var clientHistoryFlights sync.Map
var clientHistoryEpochMu sync.Mutex
var clientHistoryEpoch uint64
var clientHistoryLatest = map[clientHistoryAccount]uint64{}

type clientHistoryAccount struct {
	a      *app
	client *LCUClient
	puuid  string
}

// Public references are written in place; keep participant backing arrays private.
func cloneClientHistoryMatches(matches []gameplayMatch) []gameplayMatch {
	result := append([]gameplayMatch(nil), matches...)
	for i := range result {
		result[i].Participants = append([]gameplayParticipant(nil), matches[i].Participants...)
	}
	return result
}

func (a *app) clientHistorySnapshotKey(region, serverID, puuid string) string {
	if a.storage == nil || puuid == "" || region == "" && serverID == "" {
		return ""
	}
	return filepath.Join("client-history", strings.ToLower(region), strings.ToUpper(serverID), a.storage.accountHash(Summoner{PUUID: puuid})+".json")
}
func (a *app) loadCurrentHistoryFast(ctx context.Context, client *LCUClient, ref gameplayReference, puuid string, count int, names, labels map[int64]string) ([]gameplayMatch, []EndpointCapability, gameplayPagination, uint64) {
	count = min(20, clampMatchCount(count))
	key := a.clientHistorySnapshotKey(clientRiotPlatform(client), ref.ServerID, puuid)
	var snapshot clientHistorySnapshot
	hasSnapshot := false
	if key != "" {
		if raw, err := readLocalStoreFile(a.storage, key); err == nil && json.Unmarshal(raw, &snapshot) == nil {
			for _, capability := range snapshot.Capabilities {
				if capability.Name == "match-history" && capability.State == capabilityAvailable && snapshot.Matches != nil {
					hasSnapshot = true
				}
			}
			for i := range snapshot.Matches {
				for j := range snapshot.Matches[i].Participants {
					p := &snapshot.Matches[i].Participants[j]
					p.reference = normalizeGameplayReference(gameplayReference{PlayerRef: p.PlayerRef, GameName: p.GameName, TagLine: p.TagLine, DisplayName: p.DisplayName, Region: clientRiotPlatform(client), ServerID: ref.ServerID, ClientIdentity: true})
				}
			}
			if len(snapshot.Matches) > count {
				snapshot.Matches = snapshot.Matches[:count]
				snapshot.Pagination.Count = count
				snapshot.Pagination.HasMore = true
			}
		}
	}
	// A retained SGP page is also a self snapshot. Paint it before validating
	// its head; TTL expiry must not make an in-memory page cold again.
	if !hasSnapshot && a.sgp != nil && ref.ServerID != "" {
		page, exists := a.sgp.retainedHistoryPage(ref.ServerID, puuid, count)
		if exists && page.decodeFailed == 0 && page.consumed == len(page.games) {
			snapshot.Matches = make([]gameplayMatch, 0, len(page.games))
			for _, info := range page.games {
				if info == nil || len(info.Participants) == 0 {
					continue
				}
				match := convertRiotMatchInfo(info, puuid, names, labels, "", ref.ServerID)
				if !isCustomGameplayMatch(match) {
					snapshot.Matches = append(snapshot.Matches, match)
				}
			}
			if len(page.games) > 0 && page.games[0] != nil {
				snapshot.HeadGameID = page.games[0].GameID
			}
			detailState := capabilityAvailable
			if summarizeRiotParticipants(page.games).Incomplete > 0 {
				detailState = capabilityFailed
			}
			snapshot.Capabilities = []EndpointCapability{{Name: "match-history", State: capabilityAvailable, Count: len(snapshot.Matches)}, {Name: "match-details", State: detailState, Count: len(snapshot.Matches)}}
			snapshot.Pagination = gameplayPagination{Count: page.consumed, HasMore: page.more, Filter: "all"}
			hasSnapshot = true
		}
	}
	flightKey := struct {
		a      *app
		client *LCUClient
		puuid  string
		count  int
	}{a, client, puuid, count}
	flight := &clientHistoryFlight{done: make(chan struct{})}
	accountKey := clientHistoryAccount{a, client, puuid}
	clientHistoryEpochMu.Lock()
	actual, loaded := clientHistoryFlights.LoadOrStore(flightKey, flight)
	if !loaded {
		clientHistoryEpoch++
		flight.generation = clientHistoryEpoch
		clientHistoryLatest[accountKey] = flight.generation
	}
	clientHistoryEpochMu.Unlock()
	if loaded {
		flight = actual.(*clientHistoryFlight)
	} else {
		a.goSafe("client-history.refresh", func() {
			defer clientHistoryFlights.Delete(flightKey)
			defer close(flight.done)
			background, cancel := context.WithTimeout(a.overviewSupplementContext(client), 90*time.Second)
			defer cancel()
			started := time.Now()
			cost := &overviewLoadCost{}
			background = context.WithValue(background, overviewLoadCostContextKey{}, cost)
			head := &atomic.Int64{}
			background = context.WithValue(background, overviewHistoryHeadContextKey{}, head)
			var result clientHistorySnapshot
			if hasSnapshot && isTencentClient(client) && a.sgp != nil && ref.ServerID != "" {
				result = a.refreshCurrentHistorySnapshot(background, client, ref, puuid, count, snapshot, names, labels)
			} else {
				matches, caps, pagination := a.loadDetailedMatches(background, client, ref, puuid, true, 0, count, "all", names, labels)
				result = clientHistorySnapshot{Matches: matches, Capabilities: caps, Pagination: pagination, HeadGameID: head.Load()}
			}
			matches, caps, pagination := result.Matches, result.Capabilities, result.Pagination
			success := false
			for _, cap := range caps {
				if cap.Name == "match-history" && cap.State == capabilityAvailable {
					success = true
				}
			}
			if success {
				clientHistoryEpochMu.Lock()
				latest := clientHistoryLatest[accountKey] == flight.generation
				a.mu.RLock()
				bound := a.lcu == client && a.clientSessionConnectedLocked() && a.summoner.PUUID == puuid
				a.mu.RUnlock()
				if key != "" && latest && bound {
					if raw, err := json.Marshal(result); err == nil {
						if err = writeLocalStoreFile(a.storage, key, raw); err != nil {
							a.recordDiagnostic(map[string]any{"event": "client_history_snapshot", "result": "write-failed"})
						}
					}
				}
				clientHistoryEpochMu.Unlock()
				a.mu.RLock()
				same := a.lcu == client && a.summoner.PUUID == puuid
				a.mu.RUnlock()
				clientHistoryEpochMu.Lock()
				latest = clientHistoryLatest[accountKey] == flight.generation
				clientHistoryEpochMu.Unlock()
				if same && latest {
					public := gameplayOverview{Matches: cloneClientHistoryMatches(matches)}
					a.publicizeOverviewReferences(&public)
					event, _ := json.Marshal(map[string]any{"type": "overview-matches", "account": a.registerGameplayReferenceDetails(mergeGameplayReferences(ref, gameplayReference{PlayerRef: puuid, ClientIdentity: true})), "matches": public.Matches, "pagination": pagination, "replace": true, "historyGeneration": flight.generation})
					a.broadcastEvent(string(event))
				}
			}
			requests, bytes, _, _ := cost.snapshot()
			a.recordDiagnostic(map[string]any{"event": "client_history_refresh", "duration_ms": time.Since(started).Milliseconds(), "matches": len(matches), "success": success, "sgp_requests": requests, "sgp_bytes": bytes})
			flight.result = result
		})
	}
	if hasSnapshot {
		a.observeOverviewSource("snapshot")
		a.recordDiagnostic(map[string]any{"event": "client_history_snapshot", "result": "hit", "matches": len(snapshot.Matches)})
		return cloneClientHistoryMatches(snapshot.Matches), append([]EndpointCapability(nil), snapshot.Capabilities...), snapshot.Pagination, flight.generation
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-flight.done:
		result := flight.result
		return cloneClientHistoryMatches(result.Matches), result.Capabilities, result.Pagination, flight.generation
	case <-ctx.Done():
	case <-timer.C:
	}
	return nil, []EndpointCapability{{Name: "match-history", State: "pending"}}, gameplayPagination{HasMore: true, Partial: true, Filter: "all"}, flight.generation
}
