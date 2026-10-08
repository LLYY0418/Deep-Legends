package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"
	"time"
)

// The idle path only enumerates process names. Command lines, image paths and
// lockfiles are read after a League process exists (or the snapshot failed).
type coldLCUDiscovery struct {
	mu         sync.Mutex
	swept      bool
	sweepPaths []string
	samples    []float64
	snapshot   func() (processQueryResult, error)
	commands   func() (processQueryResult, error)
	candidates func([]string) []string
}

func (a *app) discoverColdLCU() (*LCUClient, LCUDiscoveryStatus, error) {
	s := &a.coldDiscovery
	s.mu.Lock()
	defer s.mu.Unlock()
	started := time.Now()
	snapshot := s.snapshot
	if snapshot == nil {
		snapshot = nativeLeagueProcessSnapshot
	}
	light, lightErr := snapshot()
	sample := float64(time.Since(started)) / float64(time.Millisecond)
	s.samples = append(s.samples, sample)
	if len(s.samples) > 512 {
		s.samples = s.samples[len(s.samples)-512:]
	}
	if lightErr == nil && light.ProcessCount == 0 {
		s.swept, s.sweepPaths = false, nil
		return nil, LCUDiscoveryStatus{AttemptAt: started, Method: "native-snapshot", Result: "process-not-found", Detail: "未检测到 LeagueClient 进程", DurationMS: time.Since(started).Milliseconds()}, errLCUNotFound
	}
	commands := s.commands
	if commands == nil {
		commands = leagueProcessCommands
	}
	query, commandErr := commands()
	candidates := s.candidates
	if candidates == nil {
		candidates = lockfileCandidates
	}
	sweptThisAttempt := false
	once := func(lines []string) []string {
		if !s.swept {
			s.sweepPaths = candidates(lines)
			s.swept = true
			sweptThisAttempt = true
		}
		return s.sweepPaths
	}
	client, report, err := discoverLCUFromProcessesWith(query, commandErr, once, started, a.bindColdRequestCounters)
	report.Sweep = sweptThisAttempt
	if sweptThisAttempt {
		foundPaths := []string{}
		for _, path := range s.sweepPaths {
			if info, statErr := os.Lstat(path); statErr == nil && info.Mode().IsRegular() {
				foundPaths = append(foundPaths, path)
			}
		}
		s.sweepPaths = foundPaths
	}
	if light.ProcessCount > report.ProcessCount {
		report.ProcessCount = light.ProcessCount
	}
	return client, report, err
}
func (a *app) discoverySnapshotPercentiles() (float64, float64) {
	s := &a.coldDiscovery
	s.mu.Lock()
	values := append([]float64(nil), s.samples...)
	s.samples = nil
	s.mu.Unlock()
	if len(values) == 0 {
		return 0, 0
	}
	sort.Float64s(values)
	percentile := func(p int) float64 { return values[min(len(values)-1, (len(values)*p+99)/100-1)] }
	return percentile(50), percentile(95)
}

// Open the event stream as soon as HTTP responds, including a startup 404.
// A current-summoner event wins immediately; a one-second probe remains the
// fallback for clients which lose the first event or do not support WebSocket.
func (a *app) waitForDiscoverySummoner(ctx context.Context, client *LCUClient) error {
	waiting, cancel := context.WithCancel(ctx)
	defer cancel()
	ready := make(chan Summoner, 1)
	streamDone := make(chan error, 1)
	start := func() {
		a.goSafe("discovery.summoner-events", func() {
			streamDone <- client.ListenEvents(waiting, nil, func(event LCUEvent) {
				if event.URI != "/lol-summoner/v1/current-summoner" {
					return
				}
				var summoner Summoner
				if json.Unmarshal(event.Data, &summoner) == nil && summoner.SummonerID > 0 {
					select {
					case ready <- summoner:
					default:
					}
				}
			})
		})
	}
	start()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	// Probe off the select loop so an in-flight 1.5s HTTP request cannot delay
	// delivery of a ready summoner event.
	type probeResult struct {
		summoner Summoner
		portOpen bool
		err      error
	}
	probes := make(chan probeResult, 1)
	probing, streamRunning := false, true
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case summoner := <-ready:
			client.mu.Lock()
			client.discoverySummoner = &summoner
			client.mu.Unlock()
			return nil
		case <-streamDone:
			streamRunning = false
		case <-ticker.C:
			if !streamRunning {
				streamRunning = true
				start()
			}
			if !probing {
				probing = true
				a.goSafe("discovery.summoner-probe", func() {
					value, open, err := client.discoveryProbe(waiting)
					select {
					case probes <- probeResult{value, open, err}:
					case <-waiting.Done():
					}
				})
			}
		case result := <-probes:
			probing = false
			if result.err == nil && result.summoner.SummonerID > 0 {
				return nil
			}
			if !result.portOpen && (discoveryProbeErrorKind(result.err) == "refused" || errors.Is(result.err, errLicenseLocked)) {
				return result.err
			}
		}
	}
}

type coldLaunchTimeline struct {
	lastAttempt                             time.Time
	mu                                      sync.Mutex
	seen, observed, initialRunning, emitted bool
	process                                 time.Time
	milestones                              map[string]int64
	attempts, sweeps                        int
	matchesSource                           string
	overviewSource                          string
	overlayShown                            int
	lcuStarts                               []time.Time
	sgpFirstScreenBytes                     int64
	sgpSamples                              []coldSGPBytes
	browserMeasured, browserWindowElapsed   bool
	browserQueued, browserResources         int
	metricsRevision                         uint64
}

var coldLaunchMilestones = []string{"ux_cmdline_ms", "credentials_ms", "port_open_ms", "summoner_ready_ms", "connected_ms", "identity_ms", "sgp_token_ms", "overview_first_card_ms", "overlay_hidden_ms", "ui_first_change_ms", "self_tab_header_ms", "matches_card_ms"}

func (a *app) observeColdLaunchDiscovery(report LCUDiscoveryStatus, now time.Time) {
	s := &a.coldLaunch
	s.mu.Lock()
	if !s.observed {
		s.initialRunning = report.ProcessCount > 0
		s.observed = true
	}
	if report.Result == "process-not-found" {
		event := s.finishLocked()
		s.seen, s.emitted, s.milestones = false, false, nil
		s.matchesSource, s.overviewSource, s.overlayShown = "", "", 0
		s.lcuStarts = nil
		s.sgpFirstScreenBytes = 0
		s.sgpSamples = nil
		s.browserMeasured = false
		s.browserWindowElapsed = false
		s.browserQueued = 0
		s.browserResources = 0
		s.metricsRevision = 0
		s.mu.Unlock()
		if event != nil {
			a.recordDiagnostic(event)
		}
		return
	}
	if report.ProcessCount == 0 && !s.seen {
		s.mu.Unlock()
		return
	}
	newWindow := !s.seen
	if !s.seen {
		s.seen = true
		s.process = report.ProcessAt
		if s.process.IsZero() {
			s.process = report.AttemptAt
		}
		if s.process.IsZero() {
			s.process = now
		}
		s.milestones = map[string]int64{}
		s.attempts, s.sweeps = 0, 0
		s.lastAttempt = time.Time{}
	}
	if s.lastAttempt.IsZero() || s.lastAttempt != report.AttemptAt {
		s.attempts++
		if report.Sweep {
			s.sweeps++
		}
		s.lastAttempt = report.AttemptAt
	}
	for name, at := range map[string]time.Time{"ux_cmdline_ms": report.CommandLineAt, "credentials_ms": report.CredentialsAt, "port_open_ms": report.PortOpenAt, "summoner_ready_ms": report.SummonerReadyAt} {
		s.markLocked(name, at)
	}
	s.mu.Unlock()
	if newWindow {
		if window := a.coldRequestWindow(); window != nil {
			func() { raw, _ := json.Marshal(window); a.broadcastEvent(string(raw)) }()
		}
	}
}
func (s *coldLaunchTimeline) markLocked(name string, now time.Time) {
	if !s.seen || now.IsZero() || now.Before(s.process) {
		return
	}
	if _, ok := s.milestones[name]; !ok {
		s.milestones[name] = max(0, now.Sub(s.process).Milliseconds())
	}
}
func (a *app) observeColdLaunchMilestone(name string, now time.Time) {
	s := &a.coldLaunch
	s.mu.Lock()
	s.markLocked(name, now)
	if name == "self_tab_header_ms" {
		s.markLocked("ui_first_change_ms", now)
	}
	var event map[string]any
	if name == "matches_card_ms" || name == "self_tab_header_ms" {
		_, header := s.milestones["self_tab_header_ms"]
		_, matches := s.milestones["matches_card_ms"]
		if header && matches {
			event = s.finishLocked()
		}
	}
	if name == "overlay_hidden_ms" || name == "overview_first_card_ms" {
		if _, hidden := s.milestones["overlay_hidden_ms"]; hidden {
			_, ready := s.milestones["overview_first_card_ms"]
			if ready || name == "overlay_hidden_ms" {
				event = s.finishLocked()
			}
		}
	}
	s.mu.Unlock()
	if event != nil {
		a.recordDiagnostic(event)
	}
}
func (a *app) observeFirstMatchesCard(source string, now time.Time) {
	a.coldLaunch.mu.Lock()
	if !a.coldLaunch.seen || now.Before(a.coldLaunch.process) {
		a.coldLaunch.mu.Unlock()
		return
	}
	if a.coldLaunch.matchesSource == "" {
		if a.coldLaunch.overviewSource == "snapshot" {
			source = "snapshot"
		}
		a.coldLaunch.sgpFirstScreenBytes = 0
		for _, sample := range a.coldLaunch.sgpSamples {
			if !sample.at.After(now) {
				a.coldLaunch.sgpFirstScreenBytes += sample.bytes
			}
		}
		a.coldLaunch.matchesSource = source
	}
	a.coldLaunch.mu.Unlock()
	a.observeColdLaunchMilestone("matches_card_ms", now)
}

func (a *app) observeStartupOverlayShown() {
	a.coldLaunch.mu.Lock()
	a.coldLaunch.overlayShown++
	a.coldLaunch.mu.Unlock()
	a.mu.Lock()
	if !a.clientClose.started.IsZero() {
		a.clientClose.overlayShown++
	}
	a.mu.Unlock()
}

func (s *coldLaunchTimeline) finishLocked() map[string]any {
	if !s.seen || s.emitted {
		return nil
	}
	s.emitted = true
	return s.eventLocked()
}
func (s *coldLaunchTimeline) eventLocked() map[string]any {
	s.metricsRevision++
	event := map[string]any{"event": "client_cold_launch_timeline", "process_ms": int64(0), "attempts": s.attempts, "sweeps": s.sweeps, "client_running_at_app_start": s.initialRunning}
	event["matches_card_source"], event["overlay_shown"] = s.matchesSource, s.overlayShown
	event["overview_source"] = s.overviewSource
	event["process_at"] = s.process.UnixMilli()
	event["timeline_revision"] = s.metricsRevision
	count := 0
	for _, at := range s.lcuStarts {
		if !at.Before(s.process) && at.Before(s.process.Add(3*time.Second)) {
			count++
		}
	}
	event["lcu_requests_first_3s"] = count
	event["lcu_requests_window_elapsed"] = time.Since(s.process) >= 3*time.Second
	event["sgp_bytes_first_screen"] = s.sgpFirstScreenBytes
	event["browser_queued_requests_first_3s"] = -1
	if s.browserMeasured {
		event["browser_queued_requests_first_3s"] = s.browserQueued
	}
	event["browser_resource_count_first_3s"] = s.browserResources
	event["browser_requests_window_elapsed"] = s.browserWindowElapsed
	event["browser_queue_measurement"] = "resource-timing-excluding-dns-connect"
	for _, name := range coldLaunchMilestones {
		value, ok := s.milestones[name]
		if !ok {
			value = -1
		}
		event[name] = value
	}
	return event
}
