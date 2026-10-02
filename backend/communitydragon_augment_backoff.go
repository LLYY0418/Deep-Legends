package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	communityDragonAugmentRequestBudget = 4 * time.Second
	communityDragonAugmentRetryDelay    = 45 * time.Second
)

var errCommunityDragonAugmentBackoff = errors.New("community dragon augment catalog temporarily unavailable")

type communityDragonAugmentBackoffState struct {
	started time.Time
	until   time.Time
	skipped int
	reason  string
}

type communityDragonAugmentBackoff struct {
	mu     sync.Mutex
	now    func() time.Time
	stages map[string]communityDragonAugmentBackoffState
}

func (b *communityDragonAugmentBackoff) clockNow() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

func (p *championProvider) augmentCatalogBackoffCheck(stage string) error {
	b := &p.augmentBackoff
	now := b.clockNow()
	var event map[string]any
	b.mu.Lock()
	state := b.stages[stage]
	switch {
	case state.until.IsZero():
	case now.Before(state.until):
		state.skipped++
		b.stages[stage] = state
		event = map[string]any{"event": "community_dragon_augments_backoff", "stage": stage, "result": "skipped", "reason": state.reason, "started_at": state.started, "until": state.until, "skipped_requests": state.skipped}
	default:
		delete(b.stages, stage)
		event = map[string]any{"event": "community_dragon_augments_backoff", "stage": stage, "result": "ended", "reason": "retry-window-ended", "started_at": state.started, "ended_at": now, "skipped_requests": state.skipped}
	}
	b.mu.Unlock()
	if event != nil && p.diag != nil {
		p.diag(event)
	}
	if event != nil && event["result"] == "skipped" {
		return errCommunityDragonAugmentBackoff
	}
	return nil
}

func (p *championProvider) augmentCatalogBackoffObserve(parentCtx context.Context, stage string, err error) {
	if parentCtx.Err() != nil {
		return
	}
	var transportErr *url.Error
	// Only a request without an HTTP response can trip the backoff. A caller's
	// canceled or expired context is not evidence that the host is unhealthy.
	failedWithoutResponse := err != nil && errors.As(err, &transportErr) && transportErr.Err != nil && !strings.Contains(transportErr.Err.Error(), "redirect")
	hostResponded := err == nil || championUpstreamHTTPStatus(err) > 0
	b := &p.augmentBackoff
	now := b.clockNow()
	var event map[string]any
	b.mu.Lock()
	state := b.stages[stage]
	if failedWithoutResponse {
		if state.until.IsZero() || !now.Before(state.until) {
			reason := championProviderErrorKind(err)
			if reason != "timeout" {
				reason = "no-response"
			}
			state = communityDragonAugmentBackoffState{started: now, until: now.Add(communityDragonAugmentRetryDelay), reason: reason}
			if b.stages == nil {
				b.stages = make(map[string]communityDragonAugmentBackoffState)
			}
			b.stages[stage] = state
			event = map[string]any{"event": "community_dragon_augments_backoff", "stage": stage, "result": "started", "reason": reason, "started_at": state.started, "until": state.until, "skipped_requests": 0}
		}
	} else if hostResponded && !state.until.IsZero() {
		delete(b.stages, stage)
		event = map[string]any{"event": "community_dragon_augments_backoff", "stage": stage, "result": "ended", "reason": "host-responded", "started_at": state.started, "ended_at": now, "skipped_requests": state.skipped}
	}
	b.mu.Unlock()
	if event != nil && p.diag != nil {
		p.diag(event)
	}
}

func (p *championProvider) fetchCommunityDragonAugmentPart(ctx, parentCtx context.Context, stage, requestPath string) ([]byte, error) {
	data, _, err := p.fetchWithMetadataCacheKeyLoader(ctx, communityDragonHost, requestPath, nil, 1<<20, "application/json", "", func(loadCtx context.Context) ([]byte, error) {
		if err := loadCtx.Err(); err != nil {
			return nil, err
		}
		if err := p.augmentCatalogBackoffCheck(stage); err != nil {
			return nil, err
		}
		data, err := p.fetchDirect(loadCtx, communityDragonHost, requestPath, nil, 1<<20, "application/json")
		p.augmentCatalogBackoffObserve(parentCtx, stage, err)
		return data, err
	})
	return data, err
}
