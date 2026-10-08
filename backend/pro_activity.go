package main

import (
	"context"
	"time"
)

// The ID lookup is unfiltered: normal games and ARAM also indicate activity.
// Errors are not cached; no records is a successful, explicitly unknown result.
func (p *riotProvider) lastMatchStart(ctx context.Context, puuid string) (time.Time, bool, error) {
	return p.lastMatchStartCached(ctx, puuid, "proseed-lastmatch:v1:", 7*24*time.Hour)
}

func (p *riotProvider) lastMatchStartCached(ctx context.Context, puuid, prefix string, ttl time.Duration) (time.Time, bool, error) {
	var value struct {
		At    time.Time `json:"at"`
		Known bool      `json:"known"`
	}
	err := p.cachedPublicIdentity(ctx, prefix+puuid, ttl, &value, func(ctx context.Context) error {
		ids, err := p.matchIDsFiltered(ctx, puuid, 0, 1, 0, "")
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		match, _, err := p.matchByIDWithCache(ctx, ids[0])
		if err != nil {
			return err
		}
		// Older cached payloads can lack this field. Never substitute creation time.
		if match.Info.GameStartTimestamp > 0 {
			value.At, value.Known = time.UnixMilli(match.Info.GameStartTimestamp).UTC(), true
		}
		return nil
	})
	return value.At, value.Known, err
}
func setProLastMatch(row *opggProAccount, at time.Time, known bool) {
	row.LastMatchAt, row.LastMatchAtKnown = "", known
	if known {
		row.LastMatchAt = at.UTC().Format(time.RFC3339Nano)
	}
}
