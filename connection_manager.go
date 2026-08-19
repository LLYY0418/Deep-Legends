package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	connectedFallbackInterval = time.Hour
	eventDebounceInterval     = 900 * time.Millisecond
	eventRetryInterval        = 5 * time.Minute
	minimumDiscoveryBackoff   = 3 * time.Second
	maximumDiscoveryBackoff   = 8 * time.Second
	snapshotRetryInterval     = 5 * time.Second
	// 好友状态事件非常频繁（每位好友的每次状态变化都是一条事件），
	// 合并后只提醒前端“该重新拉取好友列表了”，不触发库存刷新。
	friendsEventDebounce = 1500 * time.Millisecond
)

func isFriendsLCUEvent(event LCUEvent) bool {
	uri := strings.ToLower(event.URI)
	return strings.HasPrefix(uri, "/lol-chat/v1/friends") || strings.HasPrefix(uri, "/lol-chat/v1/friend-groups")
}

func (a *app) requestRefresh() {
	if a.refreshRequests == nil {
		return
	}
	select {
	case a.refreshRequests <- struct{}{}:
	default:
	}
}

func (a *app) runConnectionManager(ctx context.Context) {
	backoff := minimumDiscoveryBackoff
	for ctx.Err() == nil {
		a.setConnectionPhase("connecting", false)
		client, report, err := discoverLCUDetailed()
		a.updateDiscovery(report)
		if err != nil {
			a.markDisconnected(friendlyError(err))
			if !a.waitForDiscovery(ctx, backoff) {
				return
			}
			backoff *= 2
			if backoff > maximumDiscoveryBackoff {
				backoff = maximumDiscoveryBackoff
			}
			continue
		}
		if !a.refreshWithClient(client) {
			client.Close()
			a.setConnectionPhase("error", false)
			if !a.waitForDiscovery(ctx, backoff) {
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
		if err := a.runConnectedSession(ctx, client); err != nil && !errors.Is(err, context.Canceled) {
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
	eventTriggers := make(chan string, 4)
	friendTriggers := make(chan struct{}, 1)
	eventErrors := make(chan error, 1)
	startEvents := func() {
		go func() {
			err := client.ListenEvents(sessionCtx, func() { a.setEventStream(client, true) }, func(event LCUEvent) {
				// 对局阶段变化直接推送给界面（“对局”页签的新对局提示灯），
				// 并触发便捷设置的自动动作（自动接受、再来一局、断线重连）。
				if strings.EqualFold(event.URI, "/lol-gameflow/v1/gameflow-phase") {
					var phase string
					if json.Unmarshal(event.Data, &phase) == nil && phase != "" {
						a.broadcastEvent("gameflow:" + phase)
						a.convenience.handlePhase(client, phase)
						// 排位结算时记录这一场的胜点变化（段位优先走 SGP，
						// 本机客户端的 ranked-stats 已不返回负场）。
						if playerRef := a.currentPlayerRef(); playerRef != "" {
							a.lpTracker.handlePhase(client, phase, playerRef, func() ([]gameplayRank, EndpointCapability) {
								return a.loadRanksWithFallback(context.Background(), client, playerRef, true, clientTencentServerID(client))
							})
						}
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
	fallback := time.NewTicker(connectedFallbackInterval)
	defer fallback.Stop()
	snapshotRetry := time.NewTicker(snapshotRetryInterval)
	defer snapshotRetry.Stop()
	var debounceTimer *time.Timer
	var debounce <-chan time.Time
	var friendsTimer *time.Timer
	var friendsDebounce <-chan time.Time
	var retryTimer *time.Timer
	var retry <-chan time.Time
	pendingScope := ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.refreshRequests:
			if !a.refreshWithClient(client) {
				return errors.New("LCU refresh failed")
			}
			a.setSnapshotPhase(client)
		case scope := <-eventTriggers:
			if scope == "full" || pendingScope == "" {
				pendingScope = scope
			}
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
			if scope == "account" {
				a.refreshAccountWithClient(client)
			} else if !a.refreshWithClient(client) {
				return errors.New("LCU inventory refresh failed")
			} else {
				a.setSnapshotPhase(client)
			}
		case <-fallback.C:
			if !a.refreshWithClient(client) {
				return errors.New("LCU fallback refresh failed")
			}
			a.setSnapshotPhase(client)
		case <-snapshotRetry.C:
			if a.snapshotReadyForClient(client) {
				continue
			}
			if !a.refreshWithClient(client) {
				return errors.New("LCU snapshot retry failed")
			}
			a.setSnapshotPhase(client)
		case <-eventErrors:
			a.setEventStream(client, false)
			if err := client.probe(); err != nil {
				return err
			}
			if retryTimer == nil {
				retryTimer = time.NewTimer(eventRetryInterval)
			} else {
				retryTimer.Reset(eventRetryInterval)
			}
			retry = retryTimer.C
		case <-retry:
			retry = nil
			startEvents()
		}
	}
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
	if a.snapshotReady {
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
	if oldClient != nil {
		oldClient.Close()
	}
	a.clearAssetCache()
	a.broadcastEvent("connection-state")
}

func (a *app) disconnectClient(client *LCUClient, message string) {
	a.mu.Lock()
	if a.lcu == client {
		a.clearSnapshotLocked(message)
		a.connectionState = "disconnected"
	}
	a.mu.Unlock()
	client.Close()
	a.clearAssetCache()
	a.broadcastEvent("connection-state")
}
