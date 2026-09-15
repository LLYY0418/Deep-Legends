package main

import (
	"errors"
	"sync"
	"time"
)

var errCollectionNotSynced = errors.New("客户端数据暂未同步，可稍后重试")

// Lives for one refresh only, including failures/empty bodies. Concurrent
// ownership readers share a flight; acquisition reads only captured payloads.
// Nothing is cached across refreshes or accounts.
type collectionRead struct {
	done chan struct{}
	data []byte
	err  error
}
type collectionReads struct {
	mu      sync.Mutex
	entries map[string]*collectionRead
	client  *LCUClient
}

func newCollectionReads(client *LCUClient) *collectionReads {
	return &collectionReads{client: client, entries: make(map[string]*collectionRead)}
}

func collectionDataPending(account AccountData) bool {
	for _, item := range account.Loot {
		if item.DataPending {
			return true
		}
	}
	for _, capability := range account.Capabilities {
		if capability.Name == "player-loot" && capability.State == "pending" {
			return true
		}
	}
	return false
}

func (a *app) scheduleCollectionDataRetry(client *LCUClient, account AccountData) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.collectionDataRetryClient != client || !collectionDataPending(account) {
		if a.collectionDataRetry != nil {
			a.collectionDataRetry.Stop()
			a.collectionDataRetry = nil
		}
		a.collectionDataRetryCount = 0
		a.collectionDataRetryClient = client
	}
	delays := []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}
	if !collectionDataPending(account) || a.collectionDataRetry != nil || a.collectionDataRetryCount >= len(delays) || a.lcu != client || !a.connected {
		return
	}
	delay := delays[a.collectionDataRetryCount]
	a.collectionDataRetryCount++
	a.collectionDataRetry = time.AfterFunc(delay, func() {
		a.mu.Lock()
		valid := a.lcu == client && a.connected && !a.manualDisconnected && collectionDataPending(a.account)
		a.collectionDataRetry = nil
		a.mu.Unlock()
		if valid {
			a.requestCollectionRefresh()
		}
	})
}
func (r *collectionReads) get(path string) ([]byte, error) {
	r.mu.Lock()
	if entry := r.entries[path]; entry != nil {
		r.mu.Unlock()
		<-entry.done
		return entry.data, entry.err
	}
	entry := &collectionRead{done: make(chan struct{})}
	r.entries[path] = entry
	r.mu.Unlock()
	entry.data, entry.err = r.client.GetBytes(path)
	close(entry.done)
	return entry.data, entry.err
}
func (r *collectionReads) captured(path string) ([]byte, error) {
	r.mu.Lock()
	entry := r.entries[path]
	r.mu.Unlock()
	if entry == nil {
		return nil, errors.New("inventory source not needed for ownership")
	}
	<-entry.done
	return entry.data, entry.err
}
