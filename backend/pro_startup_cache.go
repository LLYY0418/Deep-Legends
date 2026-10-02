package main

import (
	"context"
	"encoding/json"
	"time"
)

func decodeProProfileSnapshot(data []byte) (opggProAccount, bool) {
	var stored proProfileSnapshot
	if json.Unmarshal(data, &stored) != nil {
		return opggProAccount{}, false
	}
	cached := stored.Account
	cached.LastMatchAt, cached.LastMatchAtKnown = stored.LastMatchAt, stored.LastMatchAtKnown
	cached.DirectoryRevisionAt = stored.DirectoryRevisionAt
	if cached.DirectoryRevisionAt == "" {
		cached.DirectoryRevisionAt = stored.RevisionAt
	}
	if cached.DirectoryRevisionAt == "" {
		cached.DirectoryRevisionAt = proDirectoryRevisionAt(cached)
	}
	// Older snapshots used directory revision_at as LastMatchAt.
	if stored.DirectoryRevisionAt == "" && cached.LastMatchAtKnown && cached.LastMatchAt == cached.DirectoryRevisionAt {
		setProLastMatch(&cached, time.Time{}, false)
	}
	return cached, true
}

// Startup reads only public disk snapshots. It never calls profile enrichment
// or the background Riot seed refresher. Manual identities remain available
// to live pro_identity_match without downloading 53 public profile pages.
func (a *app) warmProPlayersCaches() {
	a.proPlayers.mu.Lock()
	a.restoreProSnapshotLocked()
	teams := cloneProTeams(a.proPlayers.teams)
	if len(teams) == 0 {
		teams = new(app).loadProSeeds(context.Background(), nil)
	}
	a.proProfiles.mu.Lock()
	if a.proProfiles.entries == nil {
		a.proProfiles.entries = map[string]opggProAccount{}
		a.proProfiles.disk = newPublicBinaryCache(a.storage, "pro-profiles", 64, 4<<20)
	}
	profiles := 0
	for ti := range teams {
		for mi := range teams[ti].Members {
			for ai := range teams[ti].Members[mi].Summoners {
				old := &teams[ti].Members[mi].Summoners[ai]
				key := "pro-profile-v2|" + proLadderAccountKey(old.GameName, old.TagLine)
				entry, err := a.proProfiles.disk.readDisk(key)
				if err != nil || time.Now().After(entry.StaleUntil) {
					continue
				}
				cached, ok := decodeProProfileSnapshot(entry.Data)
				if !ok || proLadderAccountKey(cached.GameName, cached.TagLine) != proLadderAccountKey(old.GameName, old.TagLine) {
					continue
				}
				cached.SeedKey, cached.Source, cached.PUUID = old.SeedKey, old.Source, old.PUUID
				if revision := proDirectoryRevisionAt(*old); revision != "" {
					cached.DirectoryRevisionAt, cached.RevisionAt = revision, old.RevisionAt
				}
				at, _ := time.Parse(time.RFC3339Nano, cached.CheckedAt)
				cached.Stale = at.IsZero() || time.Since(at) >= proProfileTTL(cached)
				a.proProfiles.entries[key] = cached
				*old = cached
				profiles++
			}
		}
	}
	a.proProfiles.mu.Unlock()
	a.proPlayers.teams = teams
	// Disk prewarming is not a network attempt; the first page request must
	// still evaluate its unchanged directory and per-account TTLs immediately.
	a.proPlayers.attemptedAt = time.Time{}
	a.proPlayers.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "pro_startup_cache", "profiles_restored": profiles, "network_requests": 0})
}
