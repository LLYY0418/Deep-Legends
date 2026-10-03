package main

import "time"

// Call under the owning cache mutex, on reads and writes. Expired keys that are
// never looked up again are reclaimed too; active flights are never evicted.
func pruneTTLCache[K comparable, V any](entries map[K]V, now time.Time, limit int, expires func(V) time.Time) {
	for key, value := range entries {
		if !now.Before(expires(value)) {
			delete(entries, key)
		}
	}
	for len(entries) > limit {
		var oldest K
		var until time.Time
		first := true
		for key, value := range entries {
			at := expires(value)
			if first || at.Before(until) {
				oldest = key
				until = at
				first = false
			}
		}
		delete(entries, oldest)
	}
}
