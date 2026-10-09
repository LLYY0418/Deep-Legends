package main

import "time"

// Status is a cache read. Platform and gateway I/O never occupy its response path.
func (a *app) refreshSelfReadinessAsync(client *LCUClient) {
	if client == nil || client.http == nil {
		return
	}
	a.mu.Lock()
	account := a.summoner.PUUID
	epoch := a.clientSessionEpoch
	if a.lcu != client || !a.clientSessionConnectedLocked() || a.selfReadinessPending || a.selfReadinessClient == client && a.selfReadinessAccount == account {
		a.mu.Unlock()
		return
	}
	a.selfReadinessPending = true
	a.mu.Unlock()
	a.goSafe("client-view-readiness", func() {
		defer func() {
			a.mu.Lock()
			if epoch != a.clientSessionEpoch {
				a.mu.Unlock()
				return
			}
			a.selfReadinessPending = false
			next := a.lcu
			changed := a.clientSessionConnectedLocked() && (next != client || a.summoner.PUUID != account)
			a.mu.Unlock()
			if changed {
				a.refreshSelfReadinessAsync(next)
			}
		}()
		deadline := time.Now().Add(30 * time.Second)
		for {
			client.resolvePlatform(true)
			region, _ := client.platformInfo()
			_, _, ready := a.sgp.available(client)
			a.mu.Lock()
			current := epoch == a.clientSessionEpoch && a.lcu == client && a.clientSessionConnectedLocked() && a.summoner.PUUID == account
			if current && (ready || region != "" && region != "TENCENT") {
				a.selfReadinessClient, a.selfReadinessAccount, a.selfSGPReady = client, account, ready
			}
			a.mu.Unlock()
			if !current {
				return
			}
			a.publishClientView()
			if ready || region != "" && region != "TENCENT" || time.Now().After(deadline) {
				return
			}
			time.Sleep(2 * time.Second)
		}
	})
}
