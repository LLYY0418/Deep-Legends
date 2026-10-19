package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

func (a *app) readScopedProProfile(ctx context.Context, old opggProAccount) opggProAccount {
	a.initializeProProfileCache()
	c := &a.proProfiles
	key := "pro-profile-v2|" + proLadderAccountKey(old.GameName, old.TagLine)
	c.mu.Lock()
	cached, found := c.entries[key]
	disk := c.disk
	c.mu.Unlock()
	if !found && disk != nil {
		if entry, err := disk.readDisk(key); err == nil && a.proProfileNow().Before(entry.StaleUntil) {
			cached, found = decodeProProfileSnapshot(entry.Data)
			if found {
				c.mu.Lock()
				c.entries[key] = cached
				c.mu.Unlock()
			}
		}
	}
	if found {
		cached.SeedKey, cached.Source, cached.PUUID = old.SeedKey, old.Source, old.PUUID
		if proDirectoryRevisionAt(old) != "" {
			cached.DirectoryRevisionAt = proDirectoryRevisionAt(old)
		}
		old = cached
	}
	old = expireProProfileFacts(old, a.proProfileNow())
	manual, _ := ctx.Value(proManualRefreshKey{}).(bool)
	if !manual && !proProfileDue(old, a.proProfileNow()) {
		return old
	}
	if ctx.Err() != nil {
		return old
	}
	next := old
	if next.LastSuccessAt == "" && !old.CheckFailed && (len(old.Rank) > 0 || old.LastMatchAtKnown || old.LadderRankKnown) {
		next.LastSuccessAt = old.CheckedAt
	}
	failed := false
	fallbackPage := []byte(nil)
	yielded := false
	report := func(stage string, err error) {
		if err == nil {
			return
		}
		recordProProfileFailure(ctx, stage, err)
		failed = true
		if errors.Is(err, errThrottled) || riotErrorStatus(err) == 429 {
			yielded = true
		}
	}
	if a.riot != nil && riotKeyConfigured() {
		provider := a.riot.forPlatform("kr")
		ctx = withRiotBackground(ctx)
		identity, err := a.proOfficialIdentity(ctx, old)
		report("riot_account", err)
		if errors.Is(err, errRiotNotFound) {
			next.IdentityUnavailable = true
		}
		if err == nil {
			next.IdentityUnavailable = false
			ids, idsErr := provider.matchIDs(ctx, identity, 0, 1)
			report("riot_ids", idsErr)
			if idsErr == nil && len(ids) > 0 && ids[0] != old.LastMatchID {
				match, _, matchErr := provider.matchByIDWithCache(ctx, ids[0])
				report("riot_match", matchErr)
				if matchErr == nil && match.Info.GameStartTimestamp > 0 {
					setProLastMatch(&next, time.UnixMilli(match.Info.GameStartTimestamp), true)
					next.LastMatchID = ids[0]
				}
			}
			if !yielded {
				ranks, rankErr := provider.leagueEntries(ctx, identity)
				if errors.Is(rankErr, errRiotNotFound) {
					rankErr = nil
				}
				report("league", rankErr)
				if rankErr == nil {
					next.Rank = json.RawMessage(`{"tier":"UNRANKED"}`)
					for _, rank := range ranks {
						if rank.QueueType == "RANKED_SOLO_5x5" {
							division := map[string]int{"I": 1, "II": 2, "III": 3, "IV": 4}[rank.Rank]
							if division == 0 {
								division = 1
							}
							next.Rank, _ = json.Marshal(map[string]any{"tier": rank.Tier, "division": division, "lp": rank.LeaguePoints})
							break
						}
					}
					next.UpdatedAt = a.proProfileNow().UTC().Format(time.RFC3339Nano)
				}
			}
		}
	} else {
		ref := gameplayReference{Region: "kr", GameName: old.GameName, TagLine: old.TagLine}
		request, cancel := context.WithTimeout(ctx, 8*time.Second)
		data, err := a.readOPGGPlayerPage(request, ref, http.MethodGet, "", nil)
		cancel()
		report("opgg_page", err)
		if err == nil {
			fallbackPage = data
			row, parseErr := parseProProfile(data, ref)
			report("parse", parseErr)
			if parseErr == nil {
				next.Rank, next.UpdatedAt = row.Rank, row.UpdatedAt
				if row.LastMatchAtKnown {
					next.LastMatchAt, next.LastMatchAtKnown = row.LastMatchAt, true
				}
			}
		}
	}
	// OP.GG contributes ladder rank only when Riot is configured.
	if !yielded && ctx.Err() == nil {
		account, _ := normalizeProAccount(next)
		if account.RankStatus == "ranked" {
			var rank int
			var err error
			if fallbackPage != nil {
				rank, err = parseOPGGLadderRank(fallbackPage, old.GameName, old.TagLine)
			} else {
				rank, err = fetchOPGGLadderRank(ctx, a.championDataProvider(), old.GameName, old.TagLine)
				report("opgg_page", err)
			}
			if err == nil {
				next.LadderRank, next.LadderRankKnown = rank, true
			}
		}
	}
	if ctx.Err() != nil && !failed {
		return old
	}
	now := a.proProfileNow()
	next.CheckedAt = now.UTC().Format(time.RFC3339Nano)
	next.CheckFailed = failed || yielded
	next.ActivityFailed = failed && !yielded && !next.LastMatchAtKnown
	next.ActivityPending = yielded
	if failed {
		next.Failures = old.Failures + 1
		next.Stale = true
	} else {
		next.Failures = 0
		next.LastSuccessAt = next.CheckedAt
		next.Stale = false
	}
	// Failure state is persisted beside the successful facts; it never replaces
	// known rank, activity or ladder values with missing/error placeholders.
	c.mu.Lock()
	c.entries[key] = next
	pruneProProfilesLocked(c, now)
	if disk != nil {
		safe := next
		safe.PUUID = ""
		raw, _ := json.Marshal(proProfileSnapshot{Account: safe, LastMatchAt: safe.LastMatchAt, LastMatchAtKnown: safe.LastMatchAtKnown, DirectoryRevisionAt: safe.DirectoryRevisionAt, RevisionAt: safe.RevisionAt})
		hash := sha256.Sum256(raw)
		disk.enqueueDiskWrite(championCacheEnvelope{Schema: championCacheSchema, Key: key, FetchedAt: now, ExpiresAt: now.Add(proProfileTTL(next)), StaleUntil: proProfileRetainedUntil(next, now), Hash: hex.EncodeToString(hash[:]), Data: raw})
	}
	c.mu.Unlock()
	return next
}
