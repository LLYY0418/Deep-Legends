package main

import (
	"fmt"
	"sync"
	"time"
)

const gameplaySummonerTTL = 5 * time.Minute
const gameplaySummonerLimit = 256

type gameplaySummonerEntry struct {
	at         time.Time
	summoner   Summoner
	capability EndpointCapability
}
type gameplaySummonerFlight struct {
	done  chan struct{}
	entry gameplaySummonerEntry
}

// Owned by one authenticated LCU client; reconnects cannot reuse another
// client's identities. Only successful identities survive a refresh cycle.
type gameplaySummonerCache struct {
	mu      sync.Mutex
	entries map[string]gameplaySummonerEntry
	flights map[string]*gameplaySummonerFlight
}

func loadGameplaySummoner(client *LCUClient, reference gameplayReference) (Summoner, EndpointCapability) {
	reference = normalizeGameplayReference(reference)
	key := fmt.Sprintf("%s|%s|%s|%d|%d", reference.ServerID, reference.PlayerRef, reference.AlternatePlayerRef, reference.SummonerID, reference.AlternateSummonerID)
	c := &client.gameplaySummoners
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]gameplaySummonerEntry)
		c.flights = make(map[string]*gameplaySummonerFlight)
	}
	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.at) >= gameplaySummonerTTL {
			delete(c.entries, key)
		}
	}
	if entry, ok := c.entries[key]; ok {
		c.mu.Unlock()
		return entry.summoner, entry.capability
	}
	if flight := c.flights[key]; flight != nil {
		c.mu.Unlock()
		<-flight.done
		return flight.entry.summoner, flight.entry.capability
	}
	flight := &gameplaySummonerFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()
	summoner, capability := loadGameplaySummonerUncached(client, reference)
	c.mu.Lock()
	flight.entry = gameplaySummonerEntry{time.Now(), summoner, capability}
	if capability.State == capabilityAvailable {
		if len(c.entries) >= gameplaySummonerLimit {
			oldestKey := ""
			oldest := time.Now()
			for key, entry := range c.entries {
				if entry.at.Before(oldest) {
					oldestKey, oldest = key, entry.at
				}
			}
			delete(c.entries, oldestKey)
		}
		c.entries[key] = flight.entry
	}
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()
	return summoner, capability
}
