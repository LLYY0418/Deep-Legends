package main

import (
	"context"
	"sync"
	"time"
)

type matchScoreBatchKey struct{}
type matchScoreBatch struct {
	sync.Mutex
	started time.Time
	games   map[int64]bool
	failed  int
	badges  map[string]int
}

func newMatchScoreBatch(ctx context.Context) (context.Context, *matchScoreBatch) {
	b := &matchScoreBatch{started: time.Now(), games: map[int64]bool{}, badges: map[string]int{}}
	return context.WithValue(ctx, matchScoreBatchKey{}, b), b
}
func (a *app) finishMatchScoreBatch(b *matchScoreBatch) {
	if b == nil {
		return
	}
	b.Lock()
	defer b.Unlock()
	if len(b.games) == 0 {
		return
	}
	a.recordDiagnostic(map[string]any{"event": "match_score_computed", "matches": len(b.games), "duration_ms": time.Since(b.started).Milliseconds(), "version": matchScoreParamVersion, "failed": b.failed, "badges": b.badges})
}
