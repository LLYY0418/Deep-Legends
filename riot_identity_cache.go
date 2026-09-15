package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"time"
)

// Public lookup results only: never credentials, request headers or LCU data.
// Short freshness windows avoid serial identity requests after a restart while
// still resolving renamed accounts again. No stale-on-error reuse is allowed.
func newRiotIdentityCache(p *championProvider) *championDataCache {
	if p == nil || p.cache == nil || p.cache.dir == "" {
		return nil
	}
	return newPublicBinaryCache(&localStore{root: filepath.Dir(p.cache.dir)}, "riot-identities", 256, 4<<20)
}

func (p *riotProvider) cachedPublicIdentity(ctx context.Context, identity string, ttl time.Duration, out any, loader func(context.Context) error) error {
	if p.identityDisk == nil {
		return loader(ctx)
	}
	hash := sha256.Sum256([]byte(identity))
	key := "riot-identity-v1|" + hex.EncodeToString(hash[:])
	result, err := p.identityDisk.loadWithStatus(ctx, key, ttl, 0, true, func(ctx context.Context) ([]byte, error) {
		if err := loader(ctx); err != nil {
			return nil, err
		}
		return json.Marshal(out)
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(result.data, out)
}
