package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// Public lookup results only: never credentials, request headers or LCU data.
// Short freshness windows avoid serial identity requests after a restart while
// still resolving renamed accounts again. No stale-on-error reuse is allowed.
func newRiotIdentityCache(p *championProvider) *championDataCache {
	if p == nil || p.cache == nil || p.cache.dir == "" {
		return nil
	}
	return newPublicBinaryCache(&localStore{root: filepath.Dir(p.cache.dir)}, "riot-identities", 1024, 4<<20)
}

func (p *riotProvider) cachedPublicIdentity(ctx context.Context, identity string, ttl time.Duration, out any, loader func(context.Context) error) error {
	if p.identityDisk == nil {
		return loader(ctx)
	}
	return p.cachedPublicIdentityTTL(ctx, identity, ttl, nil, out, loader)
}

func riotIdentityKey(identity string) string {
	hash := sha256.Sum256([]byte(identity))
	key := "riot-identity-v1|" + hex.EncodeToString(hash[:])
	// Preserve a recognizable non-identifying namespace through both hash layers.
	// Reviewed seed anchors remain protected; ordinary identity entries remain LRU.
	if strings.HasPrefix(identity, "proseed:v1:") {
		key = "riot-identity-v1|proseed:" + hex.EncodeToString(hash[:])
	}
	return key
}

func (p *riotProvider) cachedPublicIdentityTTL(ctx context.Context, identity string, ttl time.Duration, resultTTL func([]byte) time.Duration, out any, loader func(context.Context) error) error {
	if p.identityDisk == nil {
		return loader(ctx)
	}
	key := riotIdentityKey(identity)
	result, err := p.identityDisk.loadWithResultTTL(ctx, key, ttl, 0, true, resultTTL, func(ctx context.Context) ([]byte, error) {
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
