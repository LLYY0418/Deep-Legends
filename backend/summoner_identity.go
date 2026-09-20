package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	summonerIdentityTimeout       = 3 * time.Second
	summonerIdentityEventDebounce = 200 * time.Millisecond
)

type summonerIdentityFlight struct {
	client  *LCUClient
	done    chan struct{}
	changed bool
	err     error
}

// refreshSummonerIdentity is independent from the full catalog snapshot. It
// intentionally neither reads nor changes a.syncing.
func (a *app) refreshSummonerIdentity(client *LCUClient) (changed bool, err error) {
	if client == nil {
		return false, errors.New("summoner identity client is unavailable")
	}

	a.summonerIdentityMu.Lock()
	if flight := a.summonerIdentityFlight; flight != nil && flight.client == client {
		done := flight.done
		a.summonerIdentityMu.Unlock()
		<-done
		return flight.changed, flight.err
	}
	flight := &summonerIdentityFlight{client: client, done: make(chan struct{})}
	a.summonerIdentityFlight = flight
	a.summonerIdentityMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), summonerIdentityTimeout)
	next, loadErr := NewSummonerAPI(client).withContext(ctx).Current()
	var profile SummonerProfile
	var profileCapability EndpointCapability
	if loadErr == nil {
		profile, profileCapability = NewSummonerAPI(client).withContext(ctx).Profile()
	}
	cancel()
	if loadErr == nil {
		flight.changed, loadErr = a.applySummonerIdentity(client, next, time.Now())
		if profileCapability.State != "" {
			a.applySummonerProfile(client, profile, profileCapability)
		}
	}
	flight.err = loadErr

	a.summonerIdentityMu.Lock()
	if a.summonerIdentityFlight == flight {
		a.summonerIdentityFlight = nil
	}
	close(flight.done)
	a.summonerIdentityMu.Unlock()
	return flight.changed, flight.err
}

func (a *app) applySummonerIdentity(client *LCUClient, next Summoner, observedAt time.Time) (bool, error) {
	if next.SummonerID == 0 {
		return false, errors.New("current summoner is not ready")
	}
	a.mu.Lock()
	if client != nil && a.lcu != client {
		a.mu.Unlock()
		return false, errors.New("summoner identity belongs to an inactive client session")
	}
	previous := a.summoner
	a.summonerIdentityAt = observedAt
	a.identityReady = next.SummonerID != 0 && a.connected
	if !summonerIdentityChanged(previous, next) {
		a.mu.Unlock()
		return false, nil
	}
	accountChanged := gameplaySummonerChanged(previous, next)
	a.summoner = next
	a.mu.Unlock()

	if accountChanged {
		a.clearGameplayReferences()
		a.requestCollectionRefresh()
	}
	a.broadcastEvent("summoner-updated")
	a.queueFacadeChangedEvent(time.Now())
	return true, nil
}

func (a *app) applySummonerProfile(client *LCUClient, profile SummonerProfile, capability EndpointCapability) bool {
	a.mu.Lock()
	if client != nil && a.lcu != client {
		a.mu.Unlock()
		return false
	}
	changed := capability.State == capabilityAvailable && a.account.Profile != profile
	if changed {
		a.account.Profile = profile
	}
	if capability.Name != "" {
		updated := false
		for index := range a.account.Capabilities {
			if a.account.Capabilities[index].Name == capability.Name {
				a.account.Capabilities[index] = capability
				updated = true
				break
			}
		}
		if !updated {
			a.account.Capabilities = append(a.account.Capabilities, capability)
		}
	}
	a.mu.Unlock()
	if changed {
		a.broadcastEvent("summoner-updated")
		a.queueFacadeChangedEvent(time.Now())
	}
	return changed
}

func summonerIdentityChanged(previous, next Summoner) bool {
	return previous != next
}

func (a *app) handleSummonerIdentityEvent(client *LCUClient, data json.RawMessage) bool {
	var object map[string]any
	if json.Unmarshal(data, &object) != nil {
		return false
	}
	next := summonerFromLCUObject(object)
	if next.SummonerID == 0 {
		return false
	}

	a.summonerIdentityEventMu.Lock()
	a.summonerIdentityEventGeneration++
	generation := a.summonerIdentityEventGeneration
	if a.summonerIdentityEventTimer != nil {
		a.summonerIdentityEventTimer.Stop()
	}
	a.summonerIdentityEventTimer = time.AfterFunc(summonerIdentityEventDebounce, func() {
		a.applyDebouncedSummonerIdentity(client, next, generation)
	})
	a.summonerIdentityEventMu.Unlock()
	return true
}

func (a *app) applyDebouncedSummonerIdentity(client *LCUClient, next Summoner, generation uint64) {
	a.summonerIdentityEventMu.Lock()
	if generation != a.summonerIdentityEventGeneration {
		a.summonerIdentityEventMu.Unlock()
		return
	}
	a.summonerIdentityEventTimer = nil
	a.summonerIdentityEventMu.Unlock()
	if _, err := a.applySummonerIdentity(client, next, time.Now()); err != nil {
		a.recordDiagnostic(map[string]any{"event": "summoner_identity_event_ignored", "reason": safeDiagnosticReason(err)})
	}
}

func (a *app) handleSummonerProfileEvent(client *LCUClient, data json.RawMessage) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	var profile SummonerProfile
	if json.Unmarshal(trimmed, &profile) != nil {
		return false
	}
	a.mu.Lock()
	if client != nil && a.lcu != client {
		a.mu.Unlock()
		return false
	}
	a.mu.Unlock()
	return a.applySummonerProfile(client, profile, EndpointCapability{Name: "summoner-profile", Path: "/lol-summoner/v1/current-summoner/summoner-profile", State: capabilityAvailable, Count: 1})
}

func (a *app) summonerCapabilityDetail(prefix string) string {
	a.mu.RLock()
	at := a.summonerIdentityAt
	if at.IsZero() {
		at = a.lastSync
	}
	a.mu.RUnlock()
	if at.IsZero() {
		return prefix + "；暂无上次读取时间"
	}
	return fmt.Sprintf("%s；上次读取时间 %s", prefix, at.Local().Format("2006-01-02 15:04:05"))
}
