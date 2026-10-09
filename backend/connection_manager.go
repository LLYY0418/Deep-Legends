package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	connectedFallbackInterval   = time.Hour
	summonerFallbackInterval    = 60 * time.Second
	eventDebounceInterval       = 900 * time.Millisecond
	champSelectEventDebounce    = 300 * time.Millisecond
	facadeEventThrottleInterval = 5 * time.Second
	eventRetryInitialInterval   = 5 * time.Second
	eventRetryMaxInterval       = time.Minute
	minimumDiscoveryBackoff     = time.Second
	maximumDiscoveryBackoff     = time.Second
	snapshotRetryInterval       = 5 * time.Second
	snapshotRetryMaxInterval    = 60 * time.Second
	snapshotRetryFailureBudget  = 10
	snapshotRetryBudgetDuration = 2 * time.Minute
	// 好友状态事件非常频繁（每位好友的每次状态变化都是一条事件），
	// 合并后只提醒前端“该重新拉取好友列表了”，不触发库存刷新。
	friendsEventDebounce   = 1500 * time.Millisecond
	collectionProbeTimeout = 2 * time.Second
	collectionProbeBudget  = 60 * time.Second
)

func isFriendsLCUEvent(event LCUEvent) bool {
	uri := strings.ToLower(event.URI)
	return strings.HasPrefix(uri, "/lol-chat/v1/friends") || strings.HasPrefix(uri, "/lol-chat/v1/friend-groups")
}

func (a *app) requestRefresh() {
	a.mu.Lock()
	a.manualDisconnected = false
	a.mu.Unlock()
	if a.refreshRequests == nil {
		return
	}
	select {
	case a.refreshRequests <- struct{}{}:
	default:
	}
}

func (a *app) refreshCollectionWithClient(client *LCUClient) bool {
	// The HTTP request returns 202 before the probe/scan finishes. Coalesce for
	// that entire lifetime, not just while a request occupies the queue.
	a.mu.Lock()
	a.collectionRefreshPending = true
	if a.collectionRefreshPendingSource == "" {
		a.collectionRefreshPendingSource = "snapshot_retry"
	}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.collectionRefreshPending = false
		a.mu.Unlock()
	}()
	a.mu.RLock()
	summonerID := a.summoner.SummonerID
	probe := a.collectionProbe
	refresh := a.collectionRefresh
	a.mu.RUnlock()
	if summonerID > 0 {
		if probe == nil {
			probe = waitForCollectionEndpoint
		}
		ready, attempts := probe(context.Background(), client, summonerID)
		if !ready {
			a.recordDiagnostic(map[string]any{"event": "collection_probe_exhausted", "attempts": attempts, "has_summoner_id": summonerID > 0})
		}
	}
	if refresh != nil {
		return refresh(client)
	}
	return a.refreshWithClient(client)
}

func waitForCollectionEndpoint(ctx context.Context, client *LCUClient, summonerID int64) (bool, int) {
	if client == nil || summonerID <= 0 {
		return false, 0
	}
	path := fmt.Sprintf("/lol-champions/v1/inventories/%d/skins-minimal", summonerID)
	delays := []time.Duration{300 * time.Millisecond, 600 * time.Millisecond, 1200 * time.Millisecond, 2400 * time.Millisecond, 5 * time.Second}
	started := time.Now()
	attempts := 0
	for {
		attempts++
		probeCtx, cancel := context.WithTimeout(ctx, collectionProbeTimeout)
		err := client.RequestJSON(probeCtx, http.MethodGet, path, nil, nil)
		cancel()
		if err == nil {
			return true, attempts
		}
		if time.Since(started) >= collectionProbeBudget {
			return false, attempts
		}
		index := attempts - 1
		if index >= len(delays) {
			index = len(delays) - 1
		}
		timer := time.NewTimer(delays[index])
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, attempts
		case <-timer.C:
		}
	}
}

type connectionLoopOps struct {
	discover func() (*LCUClient, LCUDiscoveryStatus, error)
	identity func(*LCUClient) bool
	session  func(context.Context, *LCUClient) error
	wait     func(context.Context, time.Duration) bool
}

func (a *app) runConnectionManager(ctx context.Context) {
	a.runConnectionManagerWith(ctx, connectionLoopOps{a.licensedDiscovery, a.refreshIdentityWithClient, a.runConnectedSession, a.waitForDiscovery})
}

// Dependencies are explicit so tests drive the same loop without sleeping
// through its production backoff or querying processes on the developer machine.
func (a *app) runConnectionManagerWith(ctx context.Context, ops connectionLoopOps) {
	backoff := minimumDiscoveryBackoff
	for ctx.Err() == nil {
		a.mu.RLock()
		paused := a.manualDisconnected
		a.mu.RUnlock()
		if paused {
			a.setConnectionPhase("disconnected", false)
			select {
			case <-ctx.Done():
				return
			case <-a.refreshRequests:
				continue
			}
		}
		a.setConnectionPhase("connecting", false)
		client, report, err := ops.discover()
		a.updateDiscovery(report)
		if err != nil && client != nil && report.PortOpen {
			if readyErr := a.waitForDiscoverySummoner(ctx, client); readyErr == nil {
				report.Result = "connected"
				report.SummonerReadyAt = time.Now()
				a.updateDiscovery(report)
				err = nil
			} else {
				report.ProbeErrorKind = discoveryProbeErrorKind(readyErr)
				client.Close()
			}
		}
		if err != nil {
			a.markDisconnected(friendlyError(err))
			if !ops.wait(ctx, a.clientDiscoveryInterval(backoff, report, time.Now())) {
				return
			}
			backoff *= 2
			if backoff > maximumDiscoveryBackoff {
				backoff = maximumDiscoveryBackoff
			}
			continue
		}
		a.observeColdLaunchMilestone("connected_ms", time.Now())
		if !ops.identity(client) {
			client.Close()
			a.setConnectionPhase("error", false)
			if !ops.wait(ctx, a.clientDiscoveryInterval(backoff, report, time.Now())) {
				return
			}
			backoff *= 2
			if backoff > maximumDiscoveryBackoff {
				backoff = maximumDiscoveryBackoff
			}
			continue
		}
		backoff = minimumDiscoveryBackoff
		a.observeColdLaunchMilestone("identity_ms", time.Now())
		a.setSnapshotPhase(client)
		if err := ops.session(ctx, client); err != nil && !errors.Is(err, context.Canceled) {
			a.disconnectClient(client, friendlyError(err))
		}
	}
}

func (a *app) waitForDiscovery(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-a.refreshRequests:
		return true
	case <-timer.C:
		return true
	}
}

func (a *app) runConnectedSession(ctx context.Context, client *LCUClient) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.beginConnectionPriority(sessionCtx, client)
	a.goSafe("connection_manager.platform", func() { a.recordDiagnostic(clientPlatformDiagnostic(client)) })
	a.scheduleConnectionWork(sessionCtx, client, "connection_manager.settings-watch", false, func() { a.beginGameSettingsWatch(sessionCtx, client) })
	a.scheduleConnectionWork(sessionCtx, client, "connection_manager.catalog-version", false, func() { a.warmClientCatalogVersion(sessionCtx, client) })
	requestDiagnosticTicker := time.NewTicker(30 * time.Second)
	defer requestDiagnosticTicker.Stop()
	a.goSafe("connection_manager.runConnectedSession.1", func() { client.runRequestDiagnosticFlush(sessionCtx, requestDiagnosticTicker.C) })
	defer a.stopMayhemSamplerForClient("connection-ended", client)
	phaseChanges, unsubscribePhase := a.subscribeGameplayPhase(client)
	defer unsubscribePhase()
	champSelectPoll := &champSelectPolling{}
	defer champSelectPoll.stop()
	var startupIdleTimer *time.Timer
	var startupIdleTick <-chan time.Time
	defer func() {
		if startupIdleTimer != nil {
			startupIdleTimer.Stop()
		}
	}()
	syncPhaseTimers := func() {
		champSelectPoll.sync(a, client)
		if startupIdleTimer != nil {
			startupIdleTimer.Stop()
			startupIdleTimer = nil
			startupIdleTick = nil
		}
		if a.clientShutdownPending(client) {
			return
		}
		a.maybeStartupPrefetch(client, time.Now())
		a.mu.RLock()
		at := a.startupIdleAt
		done := a.startupPrefetchDone && (a.startupConnectionsWarmed || a.champions == nil)
		a.mu.RUnlock()
		if !at.IsZero() && !done {
			delay := time.Until(at.Add(10 * time.Second))
			if delay > 0 {
				startupIdleTimer = time.NewTimer(delay)
				startupIdleTick = startupIdleTimer.C
			}
		}
	}
	syncPhaseTimers()
	eventTriggers := make(chan string, 4)
	champSelectTriggers := make(chan struct{}, 8)
	friendTriggers := make(chan struct{}, 1)
	eventErrors := make(chan error, 1)
	eventReady := make(chan struct{}, 1)
	recordObjectiveEvent := a.objectiveEventRecorder(sessionCtx, client)
	var champEvents, champCoalesced atomic.Uint64
	defer func() {
		a.recordDiagnostic(map[string]any{"event": "champselect_delivery", "reason": "connection-ended", "events": champEvents.Load(), "ui_coalesced": champCoalesced.Load()})
	}()
	a.scheduleConnectionWork(sessionCtx, client, "connection_manager.objective", true, func() { a.collectObjectiveDiagnostics(sessionCtx, client, "connected") })
	startEvents := func() {
		a.goSafe("connection_manager.runConnectedSession.3", func() {
			err := client.ListenEvents(sessionCtx, func() {
				a.setEventStream(client, true)
				select {
				case eventReady <- struct{}{}:
				default:
				}
				a.goSafe("connection_manager.runConnectedSession.4", func() { a.primeGameplayState(sessionCtx, client) })
				a.scheduleConnectionWork(sessionCtx, client, "connection_manager.watch", false, func() { a.primeWatchState(client) })
				a.warmSGPTokens(sessionCtx, client)
			}, func(event LCUEvent) {
				a.mu.RLock()
				currentTokenClient := a.lcu == client && a.clientSessionConnectedLocked() && a.shutdownClient != client
				a.mu.RUnlock()
				if currentTokenClient {
					a.sgp.observeTokenEvent(client, event)
					a.observeClientCatalogVersion(client, event)
				}
				a.observeClientShutdownEvent(client, event, time.Now())
				client.rememberAcceptFocusEvent(event)
				recordObjectiveEvent(event)
				a.broadcastGameplayIdentity(event)
				// 对局阶段变化直接推送给界面（“对局”页签的新对局提示灯），
				// 并触发便捷设置的自动动作（自动接受、再来一局、断线重连）。
				if strings.EqualFold(event.URI, "/lol-gameflow/v1/gameflow-phase") {
					var phase string
					if json.Unmarshal(event.Data, &phase) == nil && phase != "" {
						if phase == "Matchmaking" {
							a.goSafe("connection_manager.runConnectedSession.6", func() { a.collectAcceptFocusInspectionAt(sessionCtx, client, "matchmaking-not-at-accept") })
						}
						a.observeGameplayPhase(sessionCtx, client, phase)
						a.broadcastEvent("gameflow:" + phase)
						if watch := a.activeWatch(); watch != nil {
							watch.handlePhase(client, phase)
						}
						a.handleLPGameflowPhase(client, phase)
					}
					return
				}
				if a.handleFacadeLCUEvent(event) {
					return
				}
				a.mu.RLock()
				current := a.summoner
				a.mu.RUnlock()
				if watch := a.activeWatch(); watch != nil {
					watch.handleEvent(client, event, current)
				}
				uri := strings.ToLower(event.URI)
				if strings.HasPrefix(uri, "/lol-rewards/v1/grants") || strings.HasPrefix(uri, "/lol-missions/v1/missions") {
					a.broadcastEvent("claim:changed")
				}
				if strings.HasPrefix(uri, "/lol-champ-select/v1/ongoing-champion-swap") {
					if watch := a.activeWatch(); watch != nil {
						watch.handleChampSelectTrade(client)
					}
					select {
					case champSelectTriggers <- struct{}{}:
					default:
					}
					return
				}
				if isChampSelectAutomationEvent(uri) {
					count := champEvents.Add(1)
					if count == 1 || count%100 == 0 {
						a.recordDiagnostic(map[string]any{"event": "champselect_delivery", "reason": "events-received", "events": count, "ui_coalesced": champCoalesced.Load()})
					}
					if watch := a.activeWatch(); watch != nil {
						watch.handleChampSelect(client, current)
					}
					select {
					case champSelectTriggers <- struct{}{}:
					default:
						champCoalesced.Add(1)
					}
					return
				}
				if isFriendsLCUEvent(event) {
					select {
					case friendTriggers <- struct{}{}:
					default:
					}
					return
				}
				scope := lcuEventRefreshScope(event)
				if scope == "" {
					return
				}
				switch scope {
				case "summoner":
					a.handleSummonerIdentityEvent(client, event.Data)
					return
				case "summoner-profile":
					a.handleSummonerProfileEvent(client, event.Data)
					return
				}
				if scope == "collection" || scope == "account+collection" {
					a.markCollectionDirty(collectionURIKind(event.URI))
				}
				if scope == "collection" {
					return
				}
				if scope == "account+collection" {
					scope = "account"
				}
				select {
				case eventTriggers <- scope:
				default:
				}
			})
			select {
			case eventErrors <- err:
			case <-sessionCtx.Done():
			}
		})
	}
	startEvents()
	a.scheduleFacadeLoginReset(sessionCtx, client)
	fallback := time.NewTicker(connectedFallbackInterval)
	defer fallback.Stop()
	identityFallback := time.NewTicker(summonerFallbackInterval)
	defer identityFallback.Stop()
	snapshotRetryTimer := time.NewTimer(time.Hour)
	if !snapshotRetryTimer.Stop() {
		<-snapshotRetryTimer.C
	}
	defer snapshotRetryTimer.Stop()
	var snapshotRetry <-chan time.Time
	retryDelay := snapshotRetryInterval
	retryFailures := 0
	var debounceTimer *time.Timer
	var debounce <-chan time.Time
	var champSelectTimer *time.Timer
	var champSelectDebounce <-chan time.Time
	var friendsTimer *time.Timer
	var friendsDebounce <-chan time.Time
	var retryTimer *time.Timer
	var retry <-chan time.Time
	eventRetryDelay := eventRetryInitialInterval
	eventStreamReady := false
	pendingScope := ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-phaseChanges:
			syncPhaseTimers()
		case <-startupIdleTick:
			startupIdleTick = nil
			if !a.clientShutdownPending(client) {
				a.maybeStartupPrefetch(client, time.Now())
			}
		case <-champSelectPoll.ticks:
			if !champSelectPoll.sync(a, client) {
				continue
			}
			if watch := a.activeWatch(); watch != nil {
				watch.handleChampSelectAutomation(client)
			}
		case <-a.refreshRequests:
			a.mu.RLock()
			collectionRequested := a.collectionRequested
			a.mu.RUnlock()
			if collectionRequested {
				if !a.refreshCollectionWithClient(client) {
					return errors.New("LCU refresh failed")
				}
				a.setSnapshotPhase(client)
				retryDelay = snapshotRetryInterval
				snapshotRetryTimer.Reset(retryDelay)
				snapshotRetry = snapshotRetryTimer.C
			} else if _, err := a.refreshSummonerIdentity(client); err != nil {
				a.recordDiagnostic(map[string]any{"event": "summoner_identity_refresh_failed", "reason": safeDiagnosticReason(err)})
			}
		case scope := <-eventTriggers:
			if scope == "champselect" {
				continue
			}
			pendingScope = mergeDebouncedRefreshScope(pendingScope, scope)
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(eventDebounceInterval)
			} else {
				if !debounceTimer.Stop() {
					select {
					case <-debounceTimer.C:
					default:
					}
				}
				debounceTimer.Reset(eventDebounceInterval)
			}
			debounce = debounceTimer.C
		case <-champSelectTriggers:
			if champSelectTimer == nil {
				champSelectTimer = time.NewTimer(champSelectEventDebounce)
			} else {
				if !champSelectTimer.Stop() {
					select {
					case <-champSelectTimer.C:
					default:
					}
				}
				champSelectTimer.Reset(champSelectEventDebounce)
			}
			champSelectDebounce = champSelectTimer.C
		case <-champSelectDebounce:
			champSelectDebounce = nil
			a.broadcastEvent("champselect:changed")
		case <-friendTriggers:
			if friendsTimer == nil {
				friendsTimer = time.NewTimer(friendsEventDebounce)
			} else {
				if !friendsTimer.Stop() {
					select {
					case <-friendsTimer.C:
					default:
					}
				}
				friendsTimer.Reset(friendsEventDebounce)
			}
			friendsDebounce = friendsTimer.C
		case <-friendsDebounce:
			friendsDebounce = nil
			a.broadcastEvent("friends-updated")
		case <-debounce:
			debounce = nil
			scope := pendingScope
			pendingScope = ""
			if !a.handleDebouncedRefreshScope(client, scope, a.refreshAccountWithClient, a.refreshWithClient) {
				return errors.New("LCU inventory refresh failed")
			}
		case <-fallback.C:
			if a.clientShutdownPending(client) {
				continue
			}
			a.mu.RLock()
			collectionRequested := a.collectionRequested
			a.mu.RUnlock()
			if collectionRequested {
				if !a.refreshCollectionWithClient(client) {
					return errors.New("LCU fallback refresh failed")
				}
				a.setSnapshotPhase(client)
			} else if _, err := a.refreshSummonerIdentity(client); err != nil {
				a.recordDiagnostic(map[string]any{"event": "summoner_identity_fallback_failed", "reason": safeDiagnosticReason(err)})
			}
		case <-identityFallback.C:
			if a.clientShutdownPending(client) {
				continue
			}
			if _, err := a.refreshSummonerIdentity(client); err != nil {
				a.recordDiagnostic(map[string]any{"event": "summoner_identity_fallback_failed", "reason": safeDiagnosticReason(err)})
			}
		case <-snapshotRetry:
			a.mu.RLock()
			collectionRequested := a.collectionRequested
			a.mu.RUnlock()
			if !collectionRequested {
				snapshotRetry = nil
				continue
			}
			if a.snapshotReadyForClient(client) {
				retryDelay = snapshotRetryInterval
				retryFailures = 0
				snapshotRetryTimer.Reset(retryDelay)
				continue
			}
			if !a.refreshCollectionWithClient(client) {
				return errors.New("LCU snapshot retry failed")
			}
			a.setSnapshotPhase(client)
			if a.snapshotReadyForClient(client) {
				retryDelay = snapshotRetryInterval
				retryFailures = 0
			} else {
				a.mu.RLock()
				retryExhausted := a.snapshotRetryExhausted
				a.mu.RUnlock()
				retryFailures++
				retryDelay = nextSnapshotRetryDelay(retryFailures, retryExhausted)
			}
			snapshotRetryTimer.Reset(retryDelay)
		case <-eventReady:
			result := "connected"
			if eventStreamReady {
				result = "restored"
			}
			eventStreamReady = true
			eventRetryDelay = eventRetryInitialInterval
			a.recordDiagnostic(map[string]any{"event": "lcu_event_stream", "result": result})
		case err := <-eventErrors:
			a.recordClientShutdownTrace(client, time.Now())
			a.setEventStream(client, false)
			droppedEvent, oversizeEvent := eventStreamDropDiagnostics(err)
			if oversizeEvent != nil {
				a.recordDiagnostic(oversizeEvent)
			}
			a.recordDiagnostic(droppedEvent)
			if err := a.probeAfterEventStreamClose(sessionCtx, client); err != nil {
				return err
			}
			a.recordDiagnostic(map[string]any{"event": "lcu_event_stream", "result": "retrying"})
			if retryTimer == nil {
				retryTimer = time.NewTimer(eventRetryDelay)
			} else {
				if !retryTimer.Stop() {
					select {
					case <-retryTimer.C:
					default:
					}
				}
				retryTimer.Reset(eventRetryDelay)
			}
			retry = retryTimer.C
			eventRetryDelay = nextEventRetryDelay(eventRetryDelay)
		case <-retry:
			retry = nil
			startEvents()
		}
	}
}

// A shutdown signal needs no HTTP round trip. A WS-only loss gets a bounded
// health probe, then retains the existing five-second event reconnect interval.
func (a *app) probeAfterEventStreamClose(ctx context.Context, client *LCUClient) error {
	if a.clientShutdownPending(client) {
		a.disconnectClient(client, "客户端正在退出")
		return errors.New("LCU shutting down")
	}
	if err := client.probeContext(ctx, discoveryProbeTimeout); err != nil {
		a.mu.Lock()
		a.beginClientCloseLocked(client, time.Now())
		a.mu.Unlock()
		a.disconnectClient(client, "客户端事件流断开")
		return err
	}
	return nil
}

func eventStreamDropDiagnostics(err error) (map[string]any, map[string]any) {
	reason := eventStreamDropReason(err)
	dropped := map[string]any{"event": "lcu_event_stream", "result": "dropped", "reason": reason}
	var streamErr *lcuEventStreamError
	if reason != "read-limit" || !errors.As(err, &streamErr) || len(streamErr.TopURIs) == 0 {
		return dropped, nil
	}
	dropped["top_uris"] = streamErr.TopURIs
	return dropped, map[string]any{"event": "lcu_event_stream_oversize", "top_uris": streamErr.TopURIs}
}

func isFacadeLCUEvent(event LCUEvent) bool {
	uri := strings.ToLower(event.URI)
	return (strings.HasPrefix(uri, "/lol-collections/v1/inventories/") && strings.HasSuffix(uri, "/backdrop")) ||
		hasLCUEventPrefix(uri, "/lol-challenges/v1/summary-player-data") ||
		hasLCUEventPrefix(uri, "/lol-chat/v1/me") ||
		hasLCUEventPrefix(uri, "/lol-regalia/v2/current-summoner/regalia")
}

func hasLCUEventPrefix(uri, prefix string) bool {
	return uri == prefix || strings.HasPrefix(uri, prefix+"/")
}

func (a *app) handleFacadeLCUEvent(event LCUEvent) bool {
	if !isFacadeLCUEvent(event) {
		return false
	}
	if a.facadeEventChanged(event) {
		a.queueFacadeChangedEvent(time.Now())
	}
	return true
}

func (a *app) queueFacadeChangedEvent(now time.Time) {
	a.facadeEventMu.Lock()
	if a.facadeEventLastBroadcast.IsZero() || now.Sub(a.facadeEventLastBroadcast) >= facadeEventThrottleInterval {
		if a.facadeEventTimer != nil {
			a.facadeEventTimer.Stop()
			a.facadeEventTimer = nil
			a.facadeEventGeneration++
		}
		a.facadeEventLastBroadcast = now
		a.facadeEventPending = false
		a.countFacadeBroadcastLocked()
		a.facadeEventMu.Unlock()
		a.broadcastEvent("facade:changed")
		return
	}
	a.facadeEventPending = true
	if a.facadeEventTimer == nil {
		a.facadeEventGeneration++
		generation := a.facadeEventGeneration
		delay := facadeEventThrottleInterval - now.Sub(a.facadeEventLastBroadcast)
		a.facadeEventTimer = time.AfterFunc(delay, func() { a.flushFacadeChangedEvent(generation) })
	}
	a.facadeEventMu.Unlock()
}

func (a *app) flushFacadeChangedEvent(generation uint64) {
	a.facadeEventMu.Lock()
	if generation != a.facadeEventGeneration {
		a.facadeEventMu.Unlock()
		return
	}
	a.facadeEventTimer = nil
	if !a.facadeEventPending {
		a.facadeEventMu.Unlock()
		return
	}
	a.facadeEventPending = false
	a.facadeEventLastBroadcast = time.Now()
	a.countFacadeBroadcastLocked()
	a.facadeEventMu.Unlock()
	a.broadcastEvent("facade:changed")
}

func (a *app) clearFacadeEventThrottle() {
	a.facadeEventMu.Lock()
	a.facadeEventGeneration++
	if a.facadeEventSummaryTimer != nil {
		a.facadeEventSummaryTimer.Stop()
	}
	a.facadeEventSummaryTimer = nil
	a.facadeEventSummaryGeneration++
	a.facadeEventBroadcasts = 0
	a.facadeEventFingerprints = nil
	a.facadeEventSources = nil
	a.facadeEventPendingSources = nil
	if a.facadeEventTimer != nil {
		a.facadeEventTimer.Stop()
	}
	a.facadeEventTimer = nil
	a.facadeEventPending = false
	a.facadeEventLastBroadcast = time.Time{}
	a.facadeEventMu.Unlock()
}

func nextEventRetryDelay(current time.Duration) time.Duration {
	if current < eventRetryInitialInterval {
		return eventRetryInitialInterval
	}
	if current >= eventRetryMaxInterval/2 {
		return eventRetryMaxInterval
	}
	return current * 2
}

func eventStreamDropReason(err error) string {
	if err == nil {
		return "other"
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "read limit") || strings.Contains(lower, "message is too big") || strings.Contains(lower, "too large") {
		return "read-limit"
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out") {
		return "timeout"
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		if closeErr.Code == websocket.CloseNormalClosure || closeErr.Code == websocket.CloseGoingAway {
			return "close-normal"
		}
		return "close-abnormal"
	}
	if errors.Is(err, io.EOF) || strings.Contains(lower, "unexpected end of") || strings.Contains(lower, "eof") {
		return "eof"
	}
	if strings.Contains(lower, "close 1000") || strings.Contains(lower, "normal closure") || strings.Contains(lower, "going away") {
		return "close-normal"
	}
	if strings.Contains(lower, "websocket") && strings.Contains(lower, "close") {
		return "close-abnormal"
	}
	return "other"
}

func (a *app) primeWatchState(client *LCUClient) {
	watch := a.activeWatch()
	if client == nil || watch == nil {
		return
	}
	settings := watch.currentWatch()
	if !settings.MasterEnabled && !settings.ChampSelect.Enabled {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	watch.refreshCustomSession(client)
	var phase string
	if err := client.RequestJSON(ctx, "GET", "/lol-gameflow/v1/gameflow-phase", nil, &phase); err != nil {
		a.recordDiagnostic(map[string]any{"event": "watch_state_prime", "stage": "phase", "result": "failed"})
	} else if strings.TrimSpace(phase) != "" {
		watch.handlePhase(client, phase)
		a.recordDiagnostic(map[string]any{"event": "watch_state_prime", "stage": "phase", "result": "ok"})
	}
	if settings.Rules.PromoteLeader.Enabled || settings.Rules.AutoMatchmaking.Enabled {
		a.mu.RLock()
		current := a.summoner
		a.mu.RUnlock()
		watch.handleLobby(client, current, settings.Rules)
		a.recordDiagnostic(map[string]any{"event": "watch_state_prime", "stage": "lobby", "result": "completed"})
	}
}

func isChampSelectAutomationEvent(uri string) bool {
	return strings.HasPrefix(uri, "/lol-champ-select/v1/session") ||
		strings.HasPrefix(uri, "/lol-champ-select-legacy/v1/session") ||
		uri == "/lol-champ-select-legacy/v1/implementation-active" ||
		uri == "/lol-champ-select-legacy/v1/pickable-champion-ids" ||
		uri == "/lol-champ-select-legacy/v1/bannable-champion-ids" ||
		uri == "/lol-champ-select/v1/pickable-champion-ids" ||
		uri == "/lol-champ-select/v1/bannable-champion-ids" ||
		uri == "/lol-champ-select/v1/all-grid-champions" ||
		strings.HasPrefix(uri, "/lol-champ-select/v1/grid-champions/")
}

func mergeDebouncedRefreshScope(current, next string) string {
	if current == "" || current == next {
		return next
	}
	if next == "" {
		return current
	}
	if current == "full" || next == "full" {
		return "full"
	}
	if current == "account+collection" || next == "account+collection" ||
		(current == "account" && next == "collection") || (current == "collection" && next == "account") {
		return "account+collection"
	}
	return next
}

func (a *app) handleDebouncedRefreshScope(client *LCUClient, scope string, refreshAccount func(*LCUClient), refreshFull func(*LCUClient) bool) bool {
	switch scope {
	case "collection":
		a.markCollectionDirty()
		return true
	case "account":
		refreshAccount(client)
		return true
	case "account+collection":
		a.markCollectionDirty()
		refreshAccount(client)
		return true
	default:
		if !refreshFull(client) {
			return false
		}
		a.setSnapshotPhase(client)
		return true
	}
}

func collectionURIKind(uri string) string {
	uri = strings.ToLower(uri)
	switch {
	case strings.Contains(uri, "inventory"):
		return "inventory"
	case strings.Contains(uri, "loot"):
		return "loot"
	case strings.Contains(uri, "champion"):
		return "champions"
	default:
		return "other"
	}
}

func (a *app) markCollectionDirty(kinds ...string) bool {
	return a.markCollectionDirtyAt(time.Now(), kinds...)
}

func (a *app) markCollectionDirtyAt(now time.Time, kinds ...string) bool {
	kind := "other"
	if len(kinds) > 0 {
		switch kinds[0] {
		case "inventory", "loot", "champions", "identity", "loot-pending":
			kind = kinds[0]
		}
	}
	a.mu.Lock()
	during := a.syncing
	suppressed := !a.collectionRefreshFinishedAt.IsZero() && now.Sub(a.collectionRefreshFinishedAt) < 5*time.Second
	if previous := a.collectionEventAt[kind]; !previous.IsZero() && now.Sub(previous) < 30*time.Second {
		suppressed = true
	}
	if !suppressed {
		if a.collectionEventAt == nil {
			a.collectionEventAt = make(map[string]time.Time)
		}
		a.collectionEventAt[kind] = now
		a.collectionDirty = true
		a.collectionDirtyAt = now
	}
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "collection_dirty_marked", "uri_kind": kind, "during_refresh": during, "suppressed": suppressed})
	if !suppressed {
		a.broadcastEvent("collection-dirty")
	}
	return !suppressed
}

// Polling runs in the existing session loop. Unknown phases fail closed until
// the gameflow prime succeeds; entering a game resets the idle interval.
func (a *app) maybeStartupPrefetch(client *LCUClient, now time.Time) bool {
	a.gameplayFlow.mu.Lock()
	phase, phaseClient := a.gameplayFlow.phase, a.gameplayFlow.client
	a.gameplayFlow.mu.Unlock()
	idle := phaseClient == client && (phase == "None" || phase == "Lobby" || phase == "Matchmaking" || phase == "ReadyCheck")
	a.mu.Lock()
	if a.lcu != client || !a.clientSessionConnectedLocked() || a.shutdownClient == client || !a.identityReady || !idle {
		a.startupIdleAt = time.Time{}
		a.mu.Unlock()
		return false
	}
	if a.startupIdleAt.IsZero() {
		a.startupIdleAt = now
	}
	warm := !a.startupConnectionsWarmed && a.champions != nil && now.Sub(a.startupIdleAt) >= 10*time.Second
	if warm {
		a.startupConnectionsWarmed = true
	}
	ready := !a.startupPrefetchDone && !a.collectionRequested && !a.snapshotReady && now.Sub(a.startupIdleAt) >= 10*time.Second
	// Existing collection work owns its retry lifecycle. The idle prefetch must
	// not wait for it or schedule a second collection request after it finishes.
	if now.Sub(a.startupIdleAt) >= 10*time.Second && (ready || a.collectionRequested || a.snapshotReady) {
		a.startupPrefetchDone = true
	}
	a.mu.Unlock()
	if warm {
		a.champions.prewarmArenaConnections()
	}
	if ready {
		a.requestCollectionRefresh("startup_prefetch")
	}
	return ready
}

func nextSnapshotRetryDelay(attempt int, exhausted bool) time.Duration {
	if exhausted {
		return snapshotRetryMaxInterval
	}
	if attempt < 1 {
		return snapshotRetryInterval
	}
	delay := snapshotRetryInterval
	for i := 0; i < attempt; i++ {
		if delay >= snapshotRetryMaxInterval/2 {
			return snapshotRetryMaxInterval
		}
		delay *= 2
	}
	if delay > snapshotRetryMaxInterval {
		return snapshotRetryMaxInterval
	}
	return delay
}

func (a *app) snapshotReadyForClient(client *LCUClient) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lcu == client && a.snapshotReady
}

func (a *app) setSnapshotPhase(client *LCUClient) {
	a.mu.Lock()
	if a.lcu != client {
		a.mu.Unlock()
		return
	}
	if a.shutdownClient == client {
		a.mu.Unlock()
		return
	}
	if a.identityReady || a.snapshotReady {
		a.connectionState = "connected"
	} else {
		a.connectionState = "connecting"
	}
	a.mu.Unlock()
	a.publishClientView()
}

func (a *app) setConnectionPhase(phase string, eventStream bool) {
	a.mu.Lock()
	a.connectionState = phase
	a.eventStream = eventStream
	a.mu.Unlock()
	a.publishClientView()
}

func (a *app) setEventStream(client *LCUClient, connected bool) {
	a.mu.Lock()
	if a.lcu == client {
		a.eventStream = connected
	}
	a.mu.Unlock()
	a.publishClientView()
	a.signalGameplayPhase(client)
}

func (a *app) markDisconnected(message string) {
	a.mu.Lock()
	oldClient := a.lcu
	playerRef := a.summoner.PUUID
	if a.shutdownTimer != nil {
		a.shutdownTimer.Stop()
	}
	a.shutdownClient = nil
	a.shutdownEvents = nil
	a.clientLastEventAt = time.Time{}
	a.shutdownChatOfflineAt = time.Time{}
	a.shutdownFriendsEmptyAt = time.Time{}
	a.clearSnapshotLocked(message)
	a.connectionState = "disconnected"
	a.mu.Unlock()
	a.clearOverviewQuerySnapshots(playerRef)
	a.clearFacadeEventThrottle()
	a.clearFacadeChallengeCatalog(oldClient)
	if oldClient != nil {
		oldClient.Close()
	}
	a.publishClientView()
}

func (a *app) disconnectClient(client *LCUClient, message string) {
	a.mu.Lock()
	disconnectedCurrent := a.lcu == client
	playerRef := a.summoner.PUUID
	if a.lcu == client {
		if a.shutdownTimer != nil {
			a.shutdownTimer.Stop()
		}
		a.shutdownClient = nil
		a.clientLastEventAt = time.Time{}
		a.clearSnapshotLocked(message)
		a.connectionState = "disconnected"
	}
	a.mu.Unlock()
	if disconnectedCurrent {
		a.stopPostGameReveal()
		a.clearOverviewQuerySnapshots(playerRef)
		a.clearFacadeEventThrottle()
	}
	a.clearFacadeChallengeCatalog(client)
	client.Close()
	a.publishClientView()
}

// Both game-start and settlement use this uncached, identical rank loader.
func (a *app) handleLPGameflowPhase(client *LCUClient, phase string) {
	if playerRef := a.currentPlayerRef(); playerRef != "" {
		a.lpTracker.handlePhase(client, phase, playerRef, func() ([]gameplayRank, EndpointCapability) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			ranks, _, capability := a.loadRanksWithFallback(ctx, client, playerRef, true, clientTencentServerID(client), "")
			return ranks, capability
		})
	}
}
