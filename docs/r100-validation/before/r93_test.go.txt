package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestR93TwentyMatchDetailsFromDiskAfterRestart(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "r93-mock-key")
	store := &localStore{root: t.TempDir()}
	const matchCount = 20
	fixtures := make(map[string][]byte, matchCount)
	ids := make([]string, matchCount)
	for i := range ids {
		ids[i] = fmt.Sprintf("KR_%d", 930001+i)
		fixtures[ids[i]] = []byte(fmt.Sprintf(`{"metadata":{"matchId":%q},"info":{"gameId":%d,"participants":[{"participantId":1,"championId":64,"item0":3006}]}}`, ids[i], 930001+i))
	}
	newProvider := func(calls map[string]int) *riotProvider {
		champions := newChampionProvider()
		champions.cache = newChampionDataCache(store)
		champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			// Every request is intercepted, including an unexpected disk fallback.
			calls[r.URL.Path]++
			id := strings.TrimPrefix(r.URL.Path, "/lol/match/v5/matches/")
			body, ok := fixtures[id]
			if !ok || r.Method != http.MethodGet || r.URL.Host != riotClusterHost {
				return nil, fmt.Errorf("unexpected mock request: %s %s", r.Method, r.URL)
			}
			return proHTTPBody(body), nil
		})}
		provider := newRiotProvider(champions)
		// Advance the local limiter clock instead of spending wall time waiting.
		// This test checks persistence and request counts, not latency.
		now := time.Unix(1000, 0)
		provider.limitNow = func() time.Time {
			now = now.Add(time.Second)
			return now
		}
		return provider
	}
	readBatch := func(provider *riotProvider, wantStatus string) int {
		t.Helper()
		hits := 0
		for _, id := range ids {
			match, status, err := provider.matchByIDWithCache(context.Background(), id)
			if err != nil || match == nil || match.Metadata.MatchID != id {
				t.Errorf("match %s: invalid result, status=%s err=%v", id, status, err)
				continue
			}
			if status == wantStatus {
				hits++
			}
		}
		return hits
	}
	coldCalls := make(map[string]int)
	if misses := readBatch(newProvider(coldCalls), "miss"); misses != matchCount {
		t.Fatalf("cold network results=%d, want %d", misses, matchCount)
	}
	if len(coldCalls) != matchCount {
		t.Fatalf("cold distinct detail requests=%d, want %d", len(coldCalls), matchCount)
	}
	for path, calls := range coldCalls {
		if calls != 1 {
			t.Fatalf("cold requests for %s=%d, want 1", path, calls)
		}
	}
	// Rebuild both providers and their in-memory caches; only the directory is shared.
	restartCalls := make(map[string]int)
	diskHits := readBatch(newProvider(restartCalls), "disk")
	networkCalls := 0
	for _, calls := range restartCalls {
		networkCalls += calls
	}
	if diskHits != matchCount || networkCalls != 0 {
		t.Fatalf("restart disk_hits=%d network_calls=%d, want disk_hits=20 network_calls=0", diskHits, networkCalls)
	}
	t.Logf("cold_network_calls=%d restart_disk_hits=%d restart_network_calls=%d", len(coldCalls), diskHits, networkCalls)
}
