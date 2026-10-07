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

// settleBlankLoot ends the "retry later" state for identity-less loot records.
// It returns a copy of the account, so a previously shared slice is never
// changed in place, and how many records were settled. A record that is still
// DataPending here has been read back unchanged by every scheduled retry.
func settleBlankLoot(account AccountData) (AccountData, int) {
	settled := 0
	loot := append([]LootItem(nil), account.Loot...)
	for index := range loot {
		if loot[index].DataPending {
			loot[index].DataPending = false
			settled++
		}
	}
	if settled == 0 {
		return account, 0
	}
	account.Loot = loot
	return account, settled
}

func (a *app) scheduleCollectionDataRetry(client *LCUClient, account AccountData) {
	var exhausted map[string]any
	// Registered before the unlock so the diagnostic is written outside a.mu.
	defer func() {
		if exhausted != nil {
			a.recordDiagnostic(exhausted)
		}
	}()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lcu == client && a.connected {
		if settled, suppressed := a.storage.suppressCachedCollectionBlanks(account, time.Now()); suppressed > 0 {
			account = settled
			a.account, _ = a.storage.suppressCachedCollectionBlanks(a.account, time.Now())
		}
	}
	if a.collectionDataRetryClient != client || !collectionDataPending(account) {
		if a.collectionDataRetry != nil {
			a.collectionDataRetry.Stop()
			a.collectionDataRetry = nil
		}
		a.collectionDataRetryCount = 0
		a.collectionDataRetryClient = client
	}
	delays := []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}
	if collectionDataPending(account) && a.collectionDataRetry == nil && a.collectionDataRetryCount >= len(delays) && a.lcu == client && a.connected {
		// Every retry read the same record back. Stop presenting it as "not
		// synced yet": the client is returning it as-is.
		if settled, blanks := settleBlankLoot(a.account); blanks > 0 {
			kinds, samples, lastError := collectionBlankDiagnostics(a.account)
			cacheErr := a.storage.cacheExhaustedCollectionBlanks(a.account, time.Now())
			a.account = settled
			exhausted = map[string]any{"event": "collection_data_retry_exhausted", "attempts": a.collectionDataRetryCount, "blank_entries": blanks, "blank_kinds": kinds, "blank_samples": samples, "last_error_kind": lastError}
			if cacheErr != nil {
				exhausted["negative_cache_error_kind"] = "local_write"
			}
		}
		return
	}
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
			// A bounded data retry is not an LCU event; event throttling must
			// not consume its remaining retry budget.
			a.queueCollectionRefresh("pending_retry")
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
