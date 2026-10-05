package main

import (
	"context"
	"strconv"
	"strings"
	"sync"
)

type overviewFreshHistoryKey struct{}
type overviewAllHistoryKey struct{}

// Request-local evidence is collected before participant/detail filtering.
// A match ID is sufficient even when that match's detail is not yet available.
type overviewAllHistory struct {
	mu   sync.Mutex
	ids  []string
	read bool
}

func overviewFreshHistory(ctx context.Context) bool {
	fresh, _ := ctx.Value(overviewFreshHistoryKey{}).(bool)
	return fresh
}

func normalizeExpectedGameID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if platform, id, found := strings.Cut(value, "_"); found && isRiotRegion(platform) && platform == strings.ToUpper(riotPlatform(platform)) {
		value = id
	}
	id, err := strconv.ParseInt(value, 10, 64)
	return strconv.FormatInt(id, 10), err == nil && id >= 0
}

func recordOverviewAllHistory(ctx context.Context, ids []string) {
	if evidence, _ := ctx.Value(overviewAllHistoryKey{}).(*overviewAllHistory); evidence != nil {
		evidence.mu.Lock()
		evidence.ids, evidence.read = append([]string(nil), ids...), true
		evidence.mu.Unlock()
	}
}

func applyExpectedOverviewGame(response *gameplayOverview, expected string, ids []string) {
	present := false
	for _, id := range ids {
		normalized, valid := normalizeExpectedGameID(id)
		if !valid {
			continue
		}
		if response.LatestAllGameID == "" {
			response.LatestAllGameID = normalized
		}
		if expected != "0" && normalized == expected {
			present = true
		}
	}
	response.ExpectedGamePresent = &present
}

func (a *app) verifyExpectedOverviewGame(ctx context.Context, client *LCUClient, reference gameplayReference, request gameplayOverviewRequest, response *gameplayOverview) {
	if request.ExpectGameID == "" {
		return
	}
	var ids []string
	read := false
	if evidence, _ := ctx.Value(overviewAllHistoryKey{}).(*overviewAllHistory); evidence != nil {
		evidence.mu.Lock()
		ids, read = append([]string(nil), evidence.ids...), evidence.read
		evidence.mu.Unlock()
	}
	if !read {
		// Resolve the public response token, rather than exposing the underlying
		// account identifier or trusting an unavailable detail's participant list.
		resolved, ok := a.resolveGameplayReferenceDetails(response.Player.PlayerRef)
		if ok {
			reference = resolved
		}
		if isRiotRegion(reference.Region) && a.riot != nil {
			if result, err := a.riot.forPlatform(reference.Region).matchIDs(ctx, reference.PlayerRef, 0, request.Count); err == nil {
				ids = result
			}
		} else if client != nil {
			playerRef := reference.PlayerRef
			_, _, _ = a.loadDetailedMatches(ctx, client, reference, playerRef, response.Player.IsCurrent, 0, request.Count, "all", nil, nil)
			if evidence, _ := ctx.Value(overviewAllHistoryKey{}).(*overviewAllHistory); evidence != nil {
				evidence.mu.Lock()
				ids = append([]string(nil), evidence.ids...)
				evidence.mu.Unlock()
			}
		}
	}
	applyExpectedOverviewGame(response, request.ExpectGameID, ids)
}
