package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type clientHistorySnapshot struct {
	Matches      []gameplayMatch      `json:"matches"`
	Capabilities []EndpointCapability `json:"capabilities"`
	Pagination   gameplayPagination   `json:"pagination"`
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

func (a *app) clientHistorySnapshotKey(region, puuid string) string {
	if a.storage == nil || puuid == "" {
		return ""
	}
	return filepath.Join("client-history", strings.ToLower(region), a.storage.accountHash(Summoner{PUUID: puuid})+".json")
}
func (a *app) loadCurrentHistoryFast(ctx context.Context, client *LCUClient, ref gameplayReference, puuid string, count int, names, labels map[int64]string) ([]gameplayMatch, []EndpointCapability, gameplayPagination, uint64) {
	count = clampMatchCount(count)
	key := a.clientHistorySnapshotKey(clientRiotPlatform(client), puuid)
	var snapshot clientHistorySnapshot
	hasSnapshot := false
	if key != "" {
		if raw, err := readLocalStoreFile(a.storage, key); err == nil && json.Unmarshal(raw, &snapshot) == nil {
			hasSnapshot = true
			for i := range snapshot.Matches {
				for j := range snapshot.Matches[i].Participants {
					p := &snapshot.Matches[i].Participants[j]
					p.reference = normalizeGameplayReference(gameplayReference{PlayerRef: p.PlayerRef, GameName: p.GameName, TagLine: p.TagLine, DisplayName: p.DisplayName, Region: clientRiotPlatform(client), ClientIdentity: true})
				}
			}
			if len(snapshot.Matches) > count {
				snapshot.Matches = snapshot.Matches[:count]
				snapshot.Pagination.Count = count
				snapshot.Pagination.HasMore = true
			}
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
			background, cancel := context.WithTimeout(a.licenseBusinessContext(), 90*time.Second)
			defer cancel()
			started := time.Now()
			matches, caps, pagination := a.loadDetailedMatches(background, client, ref, puuid, true, 0, count, "all", names, labels)
			result := clientHistorySnapshot{matches, caps, pagination}
			success := false
			for _, cap := range caps {
				if cap.Name == "match-history" && cap.State == capabilityAvailable {
					success = true
				}
			}
			if success {
				clientHistoryEpochMu.Lock()
				latest := clientHistoryLatest[accountKey] == flight.generation
				if key != "" && latest {
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
			a.recordDiagnostic(map[string]any{"event": "client_history_refresh", "duration_ms": time.Since(started).Milliseconds(), "matches": len(matches), "success": success})
			flight.result = result
		})
	}
	if hasSnapshot {
		a.recordDiagnostic(map[string]any{"event": "client_history_snapshot", "result": "hit", "matches": len(snapshot.Matches)})
		return snapshot.Matches, snapshot.Capabilities, snapshot.Pagination, flight.generation
	}
	timer := time.NewTimer(3 * time.Second)
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
