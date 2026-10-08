package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type facadeEventCount struct {
	Received  int `json:"received"`
	Ignored   int `json:"ignored"`
	Broadcast int `json:"broadcast"`
}

func facadeEventSource(uri string) string {
	uri = strings.ToLower(uri)
	switch {
	case strings.HasPrefix(uri, "/lol-chat/"):
		return "chat"
	case strings.HasPrefix(uri, "/lol-challenges/"):
		return "challenges"
	case strings.HasPrefix(uri, "/lol-regalia/"):
		return "regalia"
	default:
		return "backdrop"
	}
}

// Fingerprints stay in memory; raw chat, identities and URI account components
// never enter diagnostics. Partial/scalar events are conservative invalidations.
func facadeEventFingerprint(event LCUEvent) (string, bool) {
	var raw map[string]any
	if json.Unmarshal(event.Data, &raw) != nil || raw == nil {
		return "", false
	}
	var projection any = raw
	switch facadeEventSource(event.URI) {
	case "chat":
		if strings.ToLower(event.URI) != "/lol-chat/v1/me" {
			return "", false
		}
		for _, key := range []string{"icon", "availability", "statusMessage", "lol"} {
			if _, ok := raw[key]; !ok {
				return "", false
			}
		}
		if _, ok := raw["lol"].(map[string]any); !ok {
			return "", false
		}
		projection = projectFacadeChat(raw)
	case "challenges":
		uri := strings.ToLower(event.URI)
		if uri != "/lol-challenges/v1/summary-player-data" && uri != "/lol-challenges/v1/summary-player-data/local-player" {
			return "", false
		}
		ids, valid := facadeSelectedChallengeIDs(raw)
		if !valid || len(ids) == 0 {
			ids = nil
			for _, v := range facadeTopChallenges(raw) {
				ids = append(ids, v.ID)
			}
		}
		projection = struct {
			Title any
			IDs   []string
		}{projectFacadeTitle(raw["title"]), ids}
	}
	bytes, err := json.Marshal(projection)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), true
}

func (a *app) facadeEventChanged(event LCUEvent) bool {
	source := facadeEventSource(event.URI)
	key := strings.ToLower(event.URI)
	fingerprint, valid := facadeEventFingerprint(event)
	a.facadeEventMu.Lock()
	defer a.facadeEventMu.Unlock()
	if a.facadeEventSources == nil {
		a.facadeEventSources = map[string]*facadeEventCount{}
	}
	count := a.facadeEventSources[source]
	if count == nil {
		count = &facadeEventCount{}
		a.facadeEventSources[source] = count
	}
	count.Received++
	if a.facadeEventSummaryTimer == nil {
		a.scheduleFacadeSummaryLocked()
	}
	if a.facadeEventFingerprints == nil {
		a.facadeEventFingerprints = map[string]string{}
	}
	if valid && a.facadeEventFingerprints[key] == fingerprint {
		count.Ignored++
		return false
	}
	if valid {
		if _, exists := a.facadeEventFingerprints[key]; !exists && len(a.facadeEventFingerprints) >= 256 {
			a.facadeEventFingerprints = map[string]string{}
		}
		a.facadeEventFingerprints[key] = fingerprint
	} else {
		delete(a.facadeEventFingerprints, key)
	}
	if a.facadeEventPendingSources == nil {
		a.facadeEventPendingSources = map[string]bool{}
	}
	a.facadeEventPendingSources[source] = true
	return true
}

func (a *app) scheduleFacadeSummaryLocked() {
	if a.facadeEventSummaryTimer != nil {
		return
	}
	a.facadeEventSummaryGeneration++
	generation := a.facadeEventSummaryGeneration
	a.facadeEventSummaryTimer = time.AfterFunc(time.Minute, func() { a.flushFacadeEventSourcesGeneration(generation) })
}
func (a *app) countFacadeBroadcastLocked() {
	a.facadeEventBroadcasts++
	if a.facadeEventSources == nil {
		a.facadeEventSources = map[string]*facadeEventCount{}
	}
	for source := range a.facadeEventPendingSources {
		count := a.facadeEventSources[source]
		if count == nil {
			count = &facadeEventCount{}
			a.facadeEventSources[source] = count
		}
		count.Broadcast++
	}
	if len(a.facadeEventSources) > 0 {
		a.scheduleFacadeSummaryLocked()
	}
	a.facadeEventPendingSources = nil
}
func (a *app) flushFacadeEventSourcesGeneration(generation uint64) {
	a.facadeEventMu.Lock()
	if generation != a.facadeEventSummaryGeneration {
		a.facadeEventMu.Unlock()
		return
	}
	if a.facadeEventSummaryTimer != nil {
		a.facadeEventSummaryTimer.Stop()
	}
	sources := a.facadeEventSources
	broadcast := a.facadeEventBroadcasts
	a.facadeEventSources = nil
	a.facadeEventBroadcasts = 0
	a.facadeEventSummaryTimer = nil
	a.facadeEventSummaryGeneration++
	a.facadeEventMu.Unlock()
	if len(sources) == 0 {
		return
	}
	received, ignored := 0, 0
	for _, v := range sources {
		received += v.Received
		ignored += v.Ignored
	}
	a.recordDiagnostic(map[string]any{"event": "facade_event_source", "window_ms": 60000, "sources": sources, "received": received, "ignored": ignored, "broadcast": broadcast})
}

func (v facadeState) MarshalJSON() ([]byte, error) {
	type plain facadeState
	if !v.OmitSkins {
		return json.Marshal(plain(v))
	}
	return json.Marshal(struct {
		plain
		Skins *[]facadeSkin `json:"skins,omitempty"`
	}{plain: plain(v)})
}

func (a *app) trackLocalHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.localHTTPInFlight.Add(1)
		defer a.localHTTPInFlight.Add(-1)
		next.ServeHTTP(w, r)
	})
}

// Inject ticks and MemStats in tests; one sampler follows the app's runtime
// context, including disconnected periods. SSE requests also count as HTTP.
func (a *app) runBackendRuntimeMetrics(ctx context.Context, ticks <-chan time.Time, read func(*runtime.MemStats)) {
	if read == nil {
		read = runtime.ReadMemStats
	}
	var timer *time.Ticker
	if ticks == nil {
		timer = time.NewTicker(time.Minute)
		defer timer.Stop()
		ticks = timer.C
	}
	var previous runtime.MemStats
	read(&previous)
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok || ctx.Err() != nil {
				return
			}
			var m runtime.MemStats
			read(&m)
			pause := uint64(0)
			if m.PauseTotalNs >= previous.PauseTotalNs {
				pause = m.PauseTotalNs - previous.PauseTotalNs
			}
			previous = m
			a.eventMu.Lock()
			sse := len(a.eventSubscribers)
			a.eventMu.Unlock()
			snapshotP50, snapshotP95 := a.discoverySnapshotPercentiles()
			a.recordDiagnostic(map[string]any{"event": "backend_runtime_metrics", "process_snapshot_p50_ms": snapshotP50, "process_snapshot_p95_ms": snapshotP95, "window_ms": 60000, "goroutines": runtime.NumGoroutine(), "heap_alloc_mb": float64(m.HeapAlloc) / (1 << 20), "heap_inuse_mb": float64(m.HeapInuse) / (1 << 20), "num_gc": m.NumGC, "gc_pause_delta_ms": float64(pause) / 1e6, "sse_connections": sse, "local_http_inflight": a.localHTTPInFlight.Load()})
		}
	}
}

func (a *app) recordRendererPerformance(r clientDiagnosticRequest) {
	clamp := func(v float64) float64 { return min(1e9, max(0, v)) }
	groups := []map[string]any{}
	for _, g := range r.Groups {
		if len(groups) >= 32 {
			break
		}
		if !allowedPerfPage(g.Section, g.Tab) {
			continue
		}
		groups = append(groups, map[string]any{"section": g.Section, "tab": g.Tab, "longtask_count": min(1000000, max(0, g.Count)), "longtask_total_ms": clamp(g.TotalMS), "longtask_max_ms": clamp(g.MaxMS)})
	}
	event := map[string]any{"event": "renderer_perf", "window_ms": min(100000000, max(0, r.WindowMs)), "longtask_count": min(1000000, max(0, r.LongtaskCount)), "longtask_total_ms": clamp(r.LongtaskTotalMS), "longtask_max_ms": clamp(r.LongtaskMaxMS), "timer_lag_count": min(1000000, max(0, r.TimerLagCount)), "timer_lag_max_ms": clamp(r.TimerLagMaxMS), "dom_nodes": min(10000000, max(0, r.DOMNodes)), "img_count": min(1000000, max(0, r.ImgCount)), "groups": groups}
	if r.HeapUsedMB != nil {
		event["heap_used_mb"] = clamp(*r.HeapUsedMB)
	}
	if r.HeapLimitMB != nil {
		event["heap_limit_mb"] = clamp(*r.HeapLimitMB)
	}
	a.recordDiagnostic(event)
}
func allowedPerfPage(section, tab string) bool {
	switch section {
	case "overview", "live", "champions", "favorites", "suite", "settings", "pro-players":
	default:
		return false
	}
	switch tab {
	case "main", "watch", "rig", "facade", "sweep", "champselect", "collection", "account", "facade-collection", "items", "pools", "icons", "banners", "runes", "build", "specialist", "pro", "opgg":
		return true
	}
	return false
}
