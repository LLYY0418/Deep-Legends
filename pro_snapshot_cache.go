package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const proSnapshotKey = "public-pro-snapshot-v1|kr"

// Caller holds proPlayers.mu. The disk contains public directory data only,
// never LCU credentials. The original publication time is not refreshed on read.
func (a *app) restoreProSnapshotLocked() bool {
	c := &a.proPlayers
	if c.diskChecked {
		return false
	}
	c.diskChecked = true
	c.disk = newPublicBinaryCache(a.storage, "pro-directory", 2, 16<<20)
	if len(c.teams) > 0 || a.storage == nil {
		return false
	}
	entry, err := c.disk.readDisk(proSnapshotKey)
	if err != nil || time.Now().After(entry.ExpiresAt) || time.Since(entry.FetchedAt) > proPlayersMaxStale {
		return false
	}
	var snapshot proDiskSnapshot
	if json.Unmarshal(entry.Data, &snapshot) != nil || len(snapshot.Teams) == 0 {
		return false
	}
	teams := snapshot.restore()
	c.teams, c.fetchedAt, c.attemptedAt = teams, entry.FetchedAt, time.Now()
	return true
}

func (a *app) persistProSnapshotLocked() {
	c := &a.proPlayers
	if c.disk == nil || len(c.teams) == 0 {
		return
	}
	data, err := json.Marshal(newProDiskSnapshot(c.teams))
	if err != nil {
		return
	}
	hash := sha256.Sum256(data)
	_ = c.disk.writeDisk(championCacheEnvelope{Schema: championCacheSchema, Key: proSnapshotKey, FetchedAt: c.fetchedAt, ExpiresAt: c.fetchedAt.Add(proPlayersMaxStale), StaleUntil: c.fetchedAt.Add(proPlayersMaxStale), Hash: hex.EncodeToString(hash[:]), Data: data})
}

// Upstream structs intentionally omit enrichment fields from JSON. Keep them
// explicitly in the private cache DTO, not in the public HTTP representation.
type proDiskSnapshot struct {
	Teams    []opggProTeam
	Members  map[string]proDiskMember
	Accounts map[string]proDiskAccount
}
type proDiskMember struct {
	Incomplete, Supplement bool
	FetchedAt              time.Time
}
type proDiskAccount struct {
	SeedKey, LastMatchAt string
	LastMatchAtKnown     bool
	Source               string
	Inactive, Stale      bool
	LadderRank           int
	LadderRankKnown      bool
}

func newProDiskSnapshot(teams []opggProTeam) proDiskSnapshot {
	s := proDiskSnapshot{Teams: teams, Members: map[string]proDiskMember{}, Accounts: map[string]proDiskAccount{}}
	for i, t := range teams {
		for j, m := range t.Members {
			s.Members[fmt.Sprintf("%d/%d", i, j)] = proDiskMember{m.Incomplete, m.Supplement, m.FetchedAt}
			for k, a := range m.Summoners {
				s.Accounts[fmt.Sprintf("%d/%d/%d", i, j, k)] = proDiskAccount{a.SeedKey, a.LastMatchAt, a.LastMatchAtKnown, a.Source, a.Inactive, a.Stale, a.LadderRank, a.LadderRankKnown}
			}
		}
	}
	return s
}
func (s proDiskSnapshot) restore() []opggProTeam {
	for i := range s.Teams {
		for j := range s.Teams[i].Members {
			m := &s.Teams[i].Members[j]
			e := s.Members[fmt.Sprintf("%d/%d", i, j)]
			m.Incomplete, m.Supplement, m.FetchedAt = e.Incomplete, e.Supplement, e.FetchedAt
			for k := range m.Summoners {
				a := &m.Summoners[k]
				e := s.Accounts[fmt.Sprintf("%d/%d/%d", i, j, k)]
				a.SeedKey, a.LastMatchAt, a.LastMatchAtKnown = e.SeedKey, e.LastMatchAt, e.LastMatchAtKnown
				a.Source, a.Inactive, a.Stale, a.LadderRank, a.LadderRankKnown = e.Source, e.Inactive, e.Stale, e.LadderRank, e.LadderRankKnown
			}
		}
	}
	return s.Teams
}
