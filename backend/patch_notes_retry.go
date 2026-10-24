package main

import (
	"context"
	"time"
)

// Notes retry independently of the once-per-release catalog invalidation.
type patchNotesRetryPlan struct {
	Next        time.Time
	Target      string
	Started     bool
	Complete    bool
	Failures    int
	LastAttempt time.Time
}

func patchNotesTarget(now time.Time, rows []patchRelease) (string, time.Time) {
	target := ""
	var next time.Time
	for _, row := range rows {
		at := row.updateAt()
		if !at.After(now) {
			if target == "" || comparePatchVersions(row.Version, target) > 0 {
				target = row.Version
			}
		} else if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	if next.IsZero() {
		next = now.Add(24 * time.Hour)
	}
	return target, next
}
func (p *patchNotesRetryPlan) step(now time.Time, rows []patchRelease, latest string, successfulAt time.Time, refresh func(string) (string, bool)) time.Time {
	target, boundary := patchNotesTarget(now, rows)
	trigger := "retry"
	if !p.Started {
		trigger = "startup"
		if p.Next.IsZero() {
			p.Next = now.Add(15 * time.Second)
		}
	}
	if target != p.Target {
		p.Target = target
		p.Complete = false
		p.Failures = 0
		if p.Started {
			p.Next = maxTime(now, p.LastAttempt.Add(time.Minute))
			trigger = "schedule"
		}
	}
	if p.Started && !successfulAt.Before(p.LastAttempt) && !successfulAt.IsZero() && (target == "" || comparePatchVersions(latest, target) >= 0) {
		p.Complete = true
	}
	if p.Complete {
		return boundary
	}
	if now.Before(p.Next) {
		return p.Next
	}
	if p.Started && p.Failures == 0 {
		trigger = "schedule"
	}
	p.Started = true
	p.LastAttempt = now
	latest, ok := refresh(trigger)
	if ok && (target == "" || comparePatchVersions(latest, target) >= 0) {
		p.Complete = true
		return boundary
	}
	delays := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	i := min(p.Failures, len(delays)-1)
	p.Failures++
	p.Next = now.Add(delays[i])
	return minTime(p.Next, boundary)
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func (a *app) runPatchNotesRetry(ctx context.Context) {
	plan := patchNotesRetryPlan{Next: time.Now().Add(15 * time.Second)}
	for ctx.Err() == nil {
		cache := a.patchNotesState()
		cache.mu.Lock()
		latest := ""
		if len(cache.documents) > 0 {
			latest = cache.documents[0].Version
		}
		checked := cache.successfulAt
		cache.mu.Unlock()
		next := plan.step(time.Now(), a.loadPatchReleaseState().Releases, latest, checked, func(trigger string) (string, bool) {
			if a.beginPatchNotesRefresh(false) {
				a.finishPatchNotesRefresh(ctx, trigger)
			}
			cache.mu.Lock()
			defer cache.mu.Unlock()
			version := ""
			if len(cache.documents) > 0 {
				version = cache.documents[0].Version
			}
			return version, !cache.failed && !cache.successfulAt.IsZero()
		})
		timer := time.NewTimer(max(time.Until(next), time.Second))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
