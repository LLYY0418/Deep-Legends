package main

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Per-client, bounded memory of successful writes. Eventhub can lag the grant
// ledger by several seconds. Never make a stale claim-all actionable during that
// gap. Match exact reward groups, not item titles, amounts or the whole event.
type claimSettlements struct {
	mu      sync.Mutex
	entries map[string]claimSettlement
}
type claimSettlement struct {
	until     time.Time
	groups    []string
	signature string
}

func claimGroupSignature(entry claimEntry) string {
	groups := append([]string(nil), entry.RewardGroupIDs...)
	if entry.RewardGroupID != "" {
		groups = append(groups, entry.RewardGroupID)
	}
	sort.Strings(groups)
	return strings.Join(groups, "\x00")
}

func (s *claimSettlements) remember(entry claimEntry, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = map[string]claimSettlement{}
	}
	for key, item := range s.entries {
		if !now.Before(item.until) {
			delete(s.entries, key)
		}
	}
	if len(s.entries) >= 512 { // Bounded even when a client returns unbounded rewards.
		var oldest string
		var deadline time.Time
		for key, item := range s.entries {
			if oldest == "" || item.until.Before(deadline) {
				oldest, deadline = key, item.until
			}
		}
		delete(s.entries, oldest)
	}
	groups := append([]string(nil), entry.RewardGroupIDs...)
	if entry.RewardGroupID != "" {
		groups = append(groups, entry.RewardGroupID)
	}
	s.entries[entry.Key] = claimSettlement{until: now.Add(30 * time.Second), groups: groups, signature: claimGroupSignature(entry)}
}
func (s *claimSettlements) filter(scan *claimScanResponse, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	groups := map[string]bool{}
	for key, item := range s.entries {
		if !now.Before(item.until) {
			delete(s.entries, key)
			continue
		}
		for _, group := range item.groups {
			groups[group] = true
		}
	}
	kept := make([]claimEntry, 0, len(scan.Items))
	for _, entry := range scan.Items {
		if settled, done := s.entries[entry.Key]; done && settled.signature == claimGroupSignature(entry) {
			continue
		}
		stale := false
		if entry.Source == "event" {
			for _, group := range entry.RewardGroupIDs {
				if groups[group] {
					stale = true
					break
				}
			}
		}
		if stale {
			continue
		}
		kept = append(kept, entry)
	}
	scan.Items = kept
}
