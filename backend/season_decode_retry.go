package main

import "strconv"

// Persist only the stream/position, retry count and scan stamp. No payload or
// player identity is retained. A page re-read within one scan cannot age a hole.
type seasonEmptyHistory struct {
	Scans    int   `json:"scans"`
	LastScan int64 `json:"lastScan"`
}

func seasonSkippedUndecodable(entries map[string]seasonEmptyHistory) int {
	n := 0
	for _, e := range entries {
		if e.Scans >= 2 {
			n++
		}
	}
	return n
}
func (scan *seasonScanState) seasonDecodeIncomplete(stream string, parsed, consumed int, empty []sgpEmptyHistoryEntry) bool {
	if scan.cache.EmptyHistory == nil {
		scan.cache.EmptyHistory = map[string]seasonEmptyHistory{}
	}
	skipped := 0
	for _, hole := range empty {
		key := stream + ":" + strconv.Itoa(hole.Offset)
		prior := scan.cache.EmptyHistory[key]
		if prior.LastScan != scan.scanID {
			prior.Scans = min(2, prior.Scans+1)
			prior.LastScan = scan.scanID
			scan.cache.EmptyHistory[key] = prior
		}
		if prior.Scans >= 2 {
			skipped++
		}
	}
	scan.cache.SkippedUndecodable = seasonSkippedUndecodable(scan.cache.EmptyHistory)
	return parsed+skipped < consumed
}
