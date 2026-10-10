package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

type riotLookupIdentityKey struct{}
type riotLookupIdentity struct{ name, tag, puuid, scope string }
type riotPUUIDRecovery struct {
	done  chan struct{}
	puuid string
	scope string
	err   error
}
type riotPUUIDState struct {
	mu         sync.Mutex
	recoveries map[string]*riotPUUIDRecovery
}

func riotCredentialScope(ctx context.Context) string {
	key, source := riotRequestCredential(ctx)
	if source == "user" || source == "env" {
		sum := sha256.Sum256([]byte(key))
		return fmt.Sprintf("user:%x", sum[:4])
	}
	return source
}
func (p *riotProvider) identityContextKey(ctx context.Context, identity string) string {
	return riotIdentityKey(identity + "|platform:" + p.region() + "|credential:" + riotCredentialScope(ctx))
}
func riotAccountMemoryKey(ctx context.Context, name, tag string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "\x1f" + strings.ToLower(strings.TrimSpace(tag)) + "|credential:" + riotCredentialScope(ctx)
}

// Account lookups are pinned to their credential route. Never cache a relay
// PUUID as a direct identity when a metadata request falls back mid-flight.
func riotPinnedIdentityContext(ctx context.Context) context.Context {
	if _, pinned := ctx.Value(riotPinnedCredentialKey{}).(riotPinnedCredential); !pinned {
		key, source := riotRequestCredential(ctx)
		ctx = context.WithValue(ctx, riotPinnedCredentialKey{}, riotPinnedCredential{key, source})
	}
	if route, _ := ctx.Value(riotRouteKey{}).(string); route != "" {
		return ctx
	}
	_, source := riotRequestCredential(ctx)
	route := "direct"
	if source == "relay" {
		route = "relay"
	}
	if source == "embedded" {
		riotUserKeys.mu.Lock()
		cooling := !riotUserKeys.routeUntil.IsZero() && riotUserKeys.routeUntil.After(time.Now())
		riotUserKeys.mu.Unlock()
		if isRiotBackground(ctx) || cooling {
			route = "relay"
		}
	}
	return context.WithValue(ctx, riotRouteKey{}, route)
}

func (p *riotProvider) recoverPUUID(ctx context.Context, id riotLookupIdentity) (string, string, error) {
	state := p.puuidState
	state.mu.Lock()
	key := riotIdentityKey(p.region() + "|" + strings.ToLower(id.name+"#"+id.tag) + "|" + riotCredentialScope(ctx))
	if prior := state.recoveries[key]; prior != nil {
		state.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-prior.done:
			return prior.puuid, prior.scope, prior.err
		}
	}
	if len(state.recoveries) >= 8192 {
		state.mu.Unlock()
		return "", "", errors.New("identity recovery limit reached")
	}
	flight := &riotPUUIDRecovery{done: make(chan struct{})}
	state.recoveries[key] = flight
	state.mu.Unlock()
	p.accountMu.Lock()
	delete(p.accountCache, riotAccountMemoryKey(ctx, id.name, id.tag))
	p.accountMu.Unlock()
	if p.identityDisk != nil {
		p.identityDisk.invalidate(p.identityContextKey(ctx, "account:"+strings.ToLower(id.name+"#"+id.tag)))
		p.identityDisk.invalidate(riotIdentityKey("account:" + strings.ToLower(id.name+"#"+id.tag) + "|platform:" + p.region()))
	}
	account, err := p.accountByRiotID(ctx, id.name, id.tag)
	state.mu.Lock()
	flight.puuid, flight.scope, flight.err = account.PUUID, account.scope, err
	close(flight.done)
	state.mu.Unlock()
	p.routeRecord(map[string]any{"event": "riot_identity_recovery", "scope": strings.Split(riotCredentialScope(ctx), ":")[0], "ok": err == nil})
	return account.PUUID, account.scope, err
}

// Both direct and relay enter here, so an automatic route change resolves the
// Riot ID under the new key before sending any by-puuid request.
func (p *riotProvider) getScopedPUUID(ctx context.Context, host, path string, query url.Values, out any, max int64) error {
	id, _ := ctx.Value(riotLookupIdentityKey{}).(riotLookupIdentity)
	marker := "/by-puuid/"
	index := strings.Index(path, marker)
	if index < 0 || id.name == "" || id.tag == "" {
		return p.getLimitedRoute(ctx, host, path, query, out, max)
	}
	ctx = riotPinnedIdentityContext(ctx)
	token := id.puuid
	if riotCredentialScope(ctx) != id.scope {
		account, err := p.accountByRiotID(ctx, id.name, id.tag)
		if err != nil {
			return err
		}
		token = account.PUUID
		if account.scope == "relay" {
			ctx = context.WithValue(ctx, riotRouteKey{}, "relay")
		}
	}
	prefix := path[:index+len(marker)]
	tail := path[index+len(marker):]
	suffix := ""
	if slash := strings.Index(tail, "/"); slash >= 0 {
		suffix = tail[slash:]
	}
	request := func(value string) error {
		return p.getLimitedRoute(ctx, host, prefix+url.PathEscape(value)+suffix, query, out, max)
	}
	err := request(token)
	var status *riotStatusError
	if !errors.As(err, &status) || !status.puuidMismatch {
		return err
	}
	fresh, scope, lookupErr := p.recoverPUUID(ctx, id)
	if lookupErr != nil {
		return lookupErr
	}
	if scope == "relay" {
		ctx = context.WithValue(ctx, riotRouteKey{}, "relay")
	}
	// A run reuses the one completed recovery and never performs another lookup.
	return request(fresh)
}

func riotMatchSubjectPUUID(match *riotMatch, puuid, name, tag string) string {
	for _, row := range match.Info.Participants {
		if row.PUUID == puuid {
			return puuid
		}
	}
	for _, row := range match.Info.Participants {
		if strings.EqualFold(row.RiotIDGameName, name) && strings.EqualFold(row.RiotIDTagline, tag) {
			return row.PUUID
		}
	}
	return puuid
}
