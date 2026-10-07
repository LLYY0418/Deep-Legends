package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

var errClientRiotIdentity = errors.New("Riot ID unavailable for client identity")

type clientRiotIdentityEntry struct {
	puuid string
	until time.Time
}
type clientRiotIdentityFlight struct {
	done  chan struct{}
	puuid string
	err   error
}
type clientRiotIdentityState struct {
	mu         sync.Mutex
	generation uint64
	entries    map[string]clientRiotIdentityEntry
	flights    map[string]*clientRiotIdentityFlight
}

// LCU and public API PUUIDs belong to different encryption scopes. Never send
// an unconverted client identity to a public by-puuid endpoint.
func (a *app) resolveClientRiotPUUID(ctx context.Context, client *LCUClient, region, puuid string, ref gameplayReference) (string, error) {
	if a.riot == nil {
		return "", errRiotKeyMissing
	}
	key := riotPlatform(region) + "\x00" + puuid
	c := &a.clientRiotIdentities
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && time.Now().Before(entry.until) {
		c.mu.Unlock()
		return entry.puuid, nil
	}
	if f := c.flights[key]; f != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-f.done:
			return f.puuid, f.err
		}
	}
	if c.entries == nil {
		c.entries = map[string]clientRiotIdentityEntry{}
		c.flights = map[string]*clientRiotIdentityFlight{}
	}
	f := &clientRiotIdentityFlight{done: make(chan struct{})}
	generation := c.generation
	c.flights[key] = f
	c.mu.Unlock()
	if ref.GameName == "" || ref.TagLine == "" {
		a.mu.RLock()
		current := a.summoner
		a.mu.RUnlock()
		if current.PUUID == puuid {
			ref.GameName, ref.TagLine = current.GameName, current.TagLine
		}
	}
	if (ref.GameName == "" || ref.TagLine == "") && client != nil {
		var summoner Summoner
		if err := client.GetJSONContext(ctx, "/lol-summoner/v2/summoners/puuid/"+url.PathEscape(puuid), &summoner); err == nil {
			ref.GameName, ref.TagLine = summoner.GameName, summoner.TagLine
		}
	}
	var public string
	err := errClientRiotIdentity
	if strings.TrimSpace(ref.GameName) != "" && strings.TrimSpace(ref.TagLine) != "" {
		account, loadErr := a.riot.forPlatform(region).accountByRiotID(ctx, ref.GameName, ref.TagLine)
		public, err = account.PUUID, loadErr
	}
	c.mu.Lock()
	if generation != c.generation {
		public, err = "", context.Canceled
	} else if err == nil {
		c.entries[key] = clientRiotIdentityEntry{public, time.Now().Add(6 * time.Hour)}
		pruneTTLCache(c.entries, time.Now(), 512, func(v clientRiotIdentityEntry) time.Time { return v.until })
	}
	if c.flights[key] == f {
		delete(c.flights, key)
	}
	f.puuid, f.err = public, err
	close(f.done)
	c.mu.Unlock()
	return public, err
}
func (a *app) clearClientRiotIdentities() {
	c := &a.clientRiotIdentities
	c.mu.Lock()
	c.generation++
	c.entries = nil
	c.flights = nil
	c.mu.Unlock()
}

// Registered Riot API references are already public; client-derived references
// and unregistered live roster identities must pass through the same resolver.
func (a *app) riotReferenceForPlayer(player string) (gameplayReference, bool) {
	a.gameplayRefsMu.Lock()
	defer a.gameplayRefsMu.Unlock()
	var fallback gameplayReference
	for _, ref := range a.gameplayRefDetails {
		if ref.PlayerRef == player {
			if isRiotRegion(ref.Region) && !ref.ClientIdentity && !ref.OPGGIdentity {
				return ref, true
			}
			fallback = ref
		}
	}
	return fallback, false
}
func riotHistoryFailureCategory(err error) string {
	var status *riotStatusError
	if errors.As(err, &status) && status.status == 400 {
		if status.puuidMismatch {
			return "riot-puuid-mismatch"
		}
		return "riot-http-400"
	}
	return "riot-failed"
}
func (p *riotProvider) clientMasteries(ctx context.Context, puuid string) ([]riotMasteryEntry, error) {
	var rows []riotMasteryEntry
	err := p.cachedPublicIdentity(ctx, "mastery:all:"+puuid, 30*time.Minute, &rows, func(ctx context.Context) error {
		return p.get(ctx, p.platformHost(), "/lol/champion-mastery/v4/champion-masteries/by-puuid/"+url.PathEscape(puuid), nil, &rows)
	})
	return rows, err
}
