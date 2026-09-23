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
	facadeEventThrottleInterval = 2 * time.Second
	eventRetryInitialInterval   = 5 * time.Second
	eventRetryMaxInterval       = time.Minute
	minimumDiscoveryBackoff     = 3 * time.Second
	maximumDiscoveryBackoff     = 8 * time.Second
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
	a.runConnectionManagerWith(ctx, connectionLoopOps{discoverLCUDetailed, a.refreshIdentityWithClient, a.runConnectedSession, a.waitForDiscovery})
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
		if err != nil {
			a.markDisconnected(friendlyError(err))
			if !ops.wait(ctx, backoff) {
				return
			}
			backoff *= 2
			if backoff > maximumDiscoveryBackoff {
				backoff = maximumDiscoveryBackoff
			}
			continue
		}
		if !ops.identity(client) {
			client.Close()
			a.setConnectionPhase("error", false)
			if !ops.wait(ctx, backoff) {
				return
			}
			backoff *= 2
			if backoff > maximumDiscoveryBackoff {
				backoff = maximumDiscoveryBackoff
			}
			continue
		}
		backoff = minimumDiscoveryBackoff
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
	defer a.stopMayhemSamplerForClient("connection-ended", client)
	champSelectPoll := time.NewTicker(time.Second)
	defer champSelectPoll.Stop()
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
	go a.collectObjectiveDiagnostics(sessionCtx, client, "connected")
	startEvents := func() {
		go func() {
			err := client.ListenEvents(sessionCtx, func() {
				a.setEventStream(client, true)
				select {
				case eventReady <- struct{}{}:
				default:
				}
				go a.primeGameplayState(sessionCtx, client)
				go a.primeWatchState(client)
			}, func(event LCUEvent) {
				client.rememberAcceptFocusEvent(event)
				recordObjectiveEvent(event)
				a.broadcastGameplayIdentity(event)
				// 对局阶段变化直接推送给界面（“对局”页签的新对局提示灯），
				// 并触发便捷设置的自动动作（自动接受、再来一局、断线重连）。
				if strings.EqualFold(event.URI, "/lol-gameflow/v1/gameflow-phase") {
					var phase string
					if json.Unmarshal(event.Data, &phase) == nil && phase != "" {
						if phase == "Matchmaking" {
							go a.collectAcceptFocusInspectionAt(sessionCtx, client, "matchmaking-not-at-accept")
						}
						a.observeGameplayPhase(sessionCtx, client, phase)
						a.broadcastEvent("gameflow:" + phase)
						if watch := a.activeWatch(); watch != nil {
							watch.handlePhase(client, phase)
						}
						// 排位结算时记录这一场的胜点变化（段位优先走 SGP，
						// 本机客户端的 ranked-stats 已不返回负场）。
						if playerRef := a.currentPlayerRef(); playerRef != "" {
							a.lpTracker.handlePhase(client, phase, playerRef, func() ([]gameplayRank, EndpointCapability) {
								ranks, _, capability := a.loadRanksWithFallback(context.Background(), client, playerRef, true, clientTencentServerID(client), "")
								return ranks, capability
							})
						}
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
				select {
				case eventTriggers <- scope:
				default:
				}
			})
			select {
			case eventErrors <- err:
			case <-sessionCtx.Done():
			}
		}()
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
		case <-champSelectPoll.C:
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
			a.setEventStream(client, false)
			droppedEvent, oversizeEvent := eventStreamDropDiagnostics(err)
			if oversizeEvent != nil {
				a.recordDiagnostic(oversizeEvent)
			}
			a.recordDiagnostic(droppedEvent)
			if err := client.probe(); err != nil {
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
	a.queueFacadeChangedEvent(time.Now())
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
	a.facadeEventMu.Unlock()
	a.broadcastEvent("facade:changed")
}

func (a *app) clearFacadeEventThrottle() {
	a.facadeEventMu.Lock()
	a.facadeEventGeneration++
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

func (a *app) markCollectionDirty() {
	a.mu.Lock()
	a.collectionDirty = true
	a.collectionDirtyAt = time.Now()
	a.mu.Unlock()
	a.broadcastEvent("collection-dirty")
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
	if a.identityReady || a.snapshotReady {
		a.connectionState = "connected"
	} else {
		a.connectionState = "connecting"
	}
	a.mu.Unlock()
	a.broadcastEvent("connection-state")
}

func (a *app) setConnectionPhase(phase string, eventStream bool) {
	a.mu.Lock()
	a.connectionState = phase
	a.eventStream = eventStream
	a.mu.Unlock()
	a.broadcastEvent("connection-state")
}

func (a *app) setEventStream(client *LCUClient, connected bool) {
	a.mu.Lock()
	if a.lcu == client {
		a.eventStream = connected
	}
	a.mu.Unlock()
	a.broadcastEvent("connection-state")
}

func (a *app) markDisconnected(message string) {
	a.mu.Lock()
	oldClient := a.lcu
	a.clearSnapshotLocked(message)
	a.connectionState = "disconnected"
	a.mu.Unlock()
	a.clearOverviewQuerySnapshots()
	a.clearFacadeEventThrottle()
	a.clearFacadeChallengeCatalog(oldClient)
	if oldClient != nil {
		oldClient.Close()
	}
	a.clearAssetCache()
	a.broadcastEvent("connection-state")
}

func (a *app) disconnectClient(client *LCUClient, message string) {
	a.mu.Lock()
	disconnectedCurrent := a.lcu == client
	if a.lcu == client {
		a.clearSnapshotLocked(message)
		a.connectionState = "disconnected"
	}
	a.mu.Unlock()
	if disconnectedCurrent {
		a.clearOverviewQuerySnapshots()
		a.clearFacadeEventThrottle()
	}
	a.clearFacadeChallengeCatalog(client)
	client.Close()
	a.clearAssetCache()
	a.broadcastEvent("connection-state")
}
