package main

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type imageProxyCosts struct {
	mu                                          sync.Mutex
	timer                                       *time.Timer
	localHits, local404, remoteOK, remoteFailed int
	remoteMS                                    []int64
	categories                                  map[string]int
}

func imageResourceKind(path string) string {
	path = strings.ToLower(path)
	switch {
	case strings.Contains(path, "profile-icons/"):
		return "profile-icon"
	case strings.Contains(path, "banner"):
		return "banner"
	case strings.Contains(path, "loot"):
		return "loot"
	default:
		return "other"
	}
}

func (a *app) recordImageProxyCost(source, path string, ok bool, elapsed time.Duration) {
	s := &a.imageCosts
	s.mu.Lock()
	defer s.mu.Unlock()
	switch source {
	case "local-hit":
		s.localHits++
	case "local-404":
		s.local404++
	case "communitydragon":
		if ok {
			s.remoteOK++
		} else {
			s.remoteFailed++
		}
		s.remoteMS = append(s.remoteMS, elapsed.Milliseconds())
	}
	if source != "local-hit" {
		if s.categories == nil {
			s.categories = make(map[string]int)
		}
		s.categories[imageResourceKind(path)]++
	}
	if s.timer == nil {
		s.timer = time.AfterFunc(30*time.Second, a.flushImageProxyCosts)
	}
}

func (a *app) flushImageProxyCosts() {
	s := &a.imageCosts
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	sort.Slice(s.remoteMS, func(i, j int) bool { return s.remoteMS[i] < s.remoteMS[j] })
	percentile := func(percent int) int64 {
		if len(s.remoteMS) == 0 {
			return 0
		}
		return s.remoteMS[(len(s.remoteMS)-1)*percent/100]
	}
	row := map[string]any{"event": "image_proxy_cost", "local_hits": s.localHits, "local_404": s.local404, "communitydragon_ok": s.remoteOK, "communitydragon_failed": s.remoteFailed, "communitydragon_p50_ms": percentile(50), "communitydragon_p90_ms": percentile(90), "resource_categories": s.categories}
	s.localHits, s.local404, s.remoteOK, s.remoteFailed = 0, 0, 0, 0
	s.remoteMS, s.categories = nil, nil
	s.mu.Unlock()
	a.recordDiagnostic(row)
}
