package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type arenaRetryContextKey struct{}

func arenaRetryable(err error) bool {
	if err == nil || championUpstreamHTTPStatus(err) > 0 || errors.Is(err, context.Canceled) {
		return false
	}
	var network *url.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) || strings.Contains(strings.ToLower(err.Error()), "connection") || strings.Contains(strings.ToLower(err.Error()), "timeout")
}

// The first network attempt has three seconds; its one immediate retry has a
// five-second budget. Cache admission wraps both attempts, so a first failure
// cannot create a negative cache that prevents the retry.
func (p *championProvider) fetchArenaWithMetadata(ctx context.Context, host, path string, query url.Values, key string) ([]byte, time.Time, error) {
	ctx = context.WithValue(ctx, arenaRetryContextKey{}, true)
	attempted := false
	data, at, err := p.fetchWithMetadataCacheKeyLoader(ctx, host, path, query, championJSONMax, "application/json", key, func(parent context.Context) ([]byte, error) {
		var data []byte
		var err error
		for attempt := 1; attempt <= 2; attempt++ {
			attempted = true
			budget := 3 * time.Second
			if attempt == 2 {
				budget = 5 * time.Second
			}
			attemptCtx, cancel := context.WithTimeout(parent, budget)
			started := time.Now()
			data, err = p.fetchDirect(attemptCtx, host, path, query, championJSONMax, "application/json")
			cancel()
			if p.diag != nil {
				p.diag(map[string]any{"event": "champion_upstream", "host": host, "status": championUpstreamHTTPStatus(err), "duration_ms": time.Since(started).Milliseconds(), "bytes": len(data), "cache": "miss", "attempt": attempt})
			}
			if !arenaRetryable(err) || parent.Err() != nil {
				break
			}
		}
		return data, err
	})
	if !attempted {
		p.reportChampionUpstream(host, "application/json", data, err, "cache", time.Now())
	}
	return data, at, err
}

func (p *championProvider) prewarmArenaConnections() {
	for _, host := range []string{opggChampionHost, yourGGArenaHost} {
		go func(host string) {
			defer recoverPanic("arena.prewarmArenaConnections")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodHead, "https://"+host+"/", nil)
			response, err := p.httpClient().Do(req)
			if err == nil {
				response.Body.Close()
			}
			if p.diag != nil {
				p.diag(map[string]any{"event": "arena_connection_prewarm", "source": host, "ok": err == nil})
			}
		}(host)
	}
}

func (p *championProvider) loadArenaDetailBlock(ctx context.Context, champion, block string) (championDetailResponse, error) {
	if block != "items" && block != "augments" && block != "synergies" {
		return championDetailResponse{}, errors.New("invalid arena block")
	}
	id, _, err := p.resolveChampionID(ctx, champion)
	if err != nil {
		return championDetailResponse{}, err
	}
	response := championDetailResponse{Mode: "arena", Region: "global", Source: "YOUR.GG aggregate"}
	if block == "synergies" {
		spec, path, query, err := opggDetailRequest("arena", id, "", "")
		if err != nil {
			return response, err
		}
		query, _, err = p.applyOPGGRequestVersion(ctx, spec, query)
		if err != nil {
			return response, err
		}
		data, at, err := p.fetchArenaWithMetadata(ctx, opggChampionHost, path, query, "")
		if err != nil {
			return response, err
		}
		var payload opggStructuredDetail
		if json.Unmarshal(data, &payload) != nil || payload.Data.Summary.ID != id {
			return response, errors.New("OP.GG champion detail response changed")
		}
		response.Source, response.FetchedAt = "OP.GG JSON", at
		response.TeamCompositions = p.structuredSynergies(id, payload.Data.Synergies)
		return response, nil
	}
	aggregate, items, at, err := p.loadArenaChampionAggregate(ctx, id)
	if err != nil {
		return response, err
	}
	response.FetchedAt = at
	if block == "items" {
		response.Build.CoreItems = mapYourGGArenaAggregateItems(aggregate.Response.CoreItems, items)
		response.Build.PrismItems = mapYourGGArenaAggregateItems(aggregate.Response.PrismaticItems, items)
		if len(response.Build.CoreItems)+len(response.Build.PrismItems) == 0 {
			return response, errors.New("YOUR.GG item sections are empty")
		}
	} else {
		catalog := p.arenaAugmentCatalogFast()
		groups, _, err := mapYourGGArenaAggregateAugmentsWithStats(aggregate.Response.Augments, catalog)
		if err != nil {
			return response, err
		}
		response.ArenaAugmentGroups = groups
		response.ArenaAugments = flattenArenaAugmentGroups(groups, 0)
		if len(response.ArenaAugments) == 0 {
			return response, errors.New("YOUR.GG augment sections are empty")
		}
	}
	return response, nil
}
