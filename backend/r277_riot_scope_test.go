package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const r277ScopeDirectPUUID = "r277-direct-key-puuid"
const r277ScopeRelayPUUID = "r277-distinct-relay-key-puuid"

type r277ScopeFixture struct {
	p        *riotProvider
	now      time.Time
	mu       sync.Mutex
	requests []string
	wrong    int
}

func r277NewScopeFixture(t *testing.T, source string, disk bool) *r277ScopeFixture {
	t.Helper()
	store := r206RelayFixture(t)
	t.Setenv("DEEP_LEGENDS_RELAY_ENTRIES", "")
	if source == "user" {
		store.key = "RGAPI-r277-scope-direct-user"
	} else {
		riotEmbeddedKey = func() string { return "RGAPI-r277-scope-direct-embedded" }
	}
	f := &r277ScopeFixture{now: time.Now()}
	champions := newChampionProvider()
	champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		relay := r.URL.Host == "relay.example"
		expected := r277ScopeDirectPUUID
		if relay {
			expected = r277ScopeRelayPUUID
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.URL.Host+r.URL.Path)
		if relay && r.Header.Get("X-Riot-Token") != "" {
			t.Error("direct credential forwarded to distinct relay")
		}
		if !relay && r.Header.Get("X-Riot-Token") == "" {
			t.Error("direct credential missing")
		}
		if strings.Contains(r.URL.Path, "/by-puuid/") && !strings.Contains(r.URL.Path, "/by-puuid/"+expected) {
			f.wrong++
			return r206RelayResponse(400, []byte(`{"message":"Exception decrypting wrong credential PUUID"}`)), nil
		}
		switch {
		case strings.Contains(r.URL.Path, "/riot/account/v1/"):
			return r206RelayResponse(200, []byte(fmt.Sprintf(`{"puuid":%q,"gameName":"ScopeFixture","tagLine":"KR1"}`, expected))), nil
		case strings.Contains(r.URL.Path, "/lol/summoner/v4/"):
			return r206RelayResponse(200, []byte(fmt.Sprintf(`{"puuid":%q,"summonerLevel":277}`, expected))), nil
		case strings.HasSuffix(r.URL.Path, "/ids"):
			return r206RelayResponse(200, []byte(`["KR_277"]`)), nil
		default:
			return nil, fmt.Errorf("unexpected scope fixture endpoint: %s", r.URL.Path)
		}
	})}
	f.p = newRiotProvider(champions)
	f.p.limitNow = func() time.Time { return f.now }
	f.p.limitSleep = func(context.Context, time.Duration) error {
		t.Error("foreground identity waited for exhausted local window")
		return context.DeadlineExceeded
	}
	if disk {
		f.p.identityDisk = newPublicBinaryCache(&localStore{root: t.TempDir()}, "r277-scope-identities", 128, 4<<20)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := f.p.identityDisk.flushDisk(ctx); err != nil {
				t.Error("identity disk cleanup did not drain", err)
			}
		})
	}
	scope := riotRequestRateScope(f.p.accountHost(), "/riot/account/v1/accounts/by-riot-id/ScopeFixture/KR1")
	f.p.observeRiotRate(scope, http.Header{"X-App-Rate-Limit": {"100:120"}, "X-App-Rate-Limit-Count": {"100:120"}}, 200)
	return f
}

func r277ScopeAssertIdentity(t *testing.T, p *riotProvider, ctx context.Context, expected string) {
	t.Helper()
	key := riotAccountMemoryKey(ctx, "ScopeFixture", "KR1")
	p.accountMu.Lock()
	entry, exists := p.accountCache[key]
	p.accountMu.Unlock()
	if expected == "" {
		if exists {
			t.Errorf("wrong-route memory cache populated: key=%s puuid=%s", key, entry.account.PUUID)
		}
	} else if !exists || entry.account.PUUID != expected {
		t.Errorf("identity memory key/result mismatch: key=%s exists=%v puuid=%s expected=%s", key, exists, entry.account.PUUID, expected)
	}
	if p.identityDisk == nil {
		return
	}
	diskKey := p.identityContextKey(ctx, "account:scopefixture#kr1")
	p.identityDisk.mu.Lock()
	envelope, exists := p.identityDisk.entries[diskKey]
	p.identityDisk.mu.Unlock()
	if expected == "" {
		if exists {
			t.Errorf("wrong-route identity cache populated: key=%s data=%s", diskKey, envelope.Data)
		}
		return
	}
	var account riotAccount
	if !exists || json.Unmarshal(envelope.Data, &account) != nil || account.PUUID != expected {
		t.Errorf("identity cache key/result mismatch: key=%s exists=%v data=%s expected=%s", diskKey, exists, envelope.Data, expected)
	}
	// Use the cache's owned queue barrier, not sleeps/polling, before inspecting
	// actual persisted envelope keys and payloads.
	if err := p.identityDisk.flushDisk(t.Context()); err != nil {
		t.Fatal(err)
	}
	persisted, err := p.identityDisk.readDisk(diskKey)
	if err != nil || json.Unmarshal(persisted.Data, &account) != nil || account.PUUID != expected || persisted.Key != diskKey {
		t.Errorf("persisted identity scope mismatch: key=%s data=%s err=%v", diskKey, persisted.Data, err)
	}
	t.Logf("cache proof: memory_key=%s disk_key=%s scope=%s puuid=%s", key, diskKey, riotCredentialScope(ctx), expected)
}

func TestR277ScopeAccountQuotaOverflow(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		for _, disk := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/disk=%v", source, disk), func(t *testing.T) {
				f := r277NewScopeFixture(t, source, disk)
				direct := riotPinnedIdentityContext(context.Background())
				relay := context.WithValue(direct, riotRouteKey{}, "relay")
				account, err := f.p.accountByRiotID(direct, "ScopeFixture", "KR1")
				if err != nil {
					t.Fatal(err)
				}
				if account.scope != "relay" || account.PUUID != r277ScopeRelayPUUID {
					t.Errorf("overflow account mislabeled: scope=%s puuid=%s err=%v", account.scope, account.PUUID, err)
				}
				r277ScopeAssertIdentity(t, f.p, direct, "")
				r277ScopeAssertIdentity(t, f.p, relay, r277ScopeRelayPUUID)
				// Recovery must use a fresh direct account, never the relay PUUID cached
				// under the direct credential by a transparent lower-level fallback.
				f.now = f.now.Add(121 * time.Second)
				recovered, err := f.p.accountByRiotID(direct, "ScopeFixture", "KR1")
				if err != nil || recovered.PUUID != r277ScopeDirectPUUID || recovered.scope != riotCredentialScope(direct) {
					t.Errorf("direct recovery identity contaminated: account=%+v scope=%s err=%v", recovered, recovered.scope, err)
				}
				r277ScopeAssertIdentity(t, f.p, direct, r277ScopeDirectPUUID)
				lookup := context.WithValue(direct, riotLookupIdentityKey{}, riotLookupIdentity{"ScopeFixture", "KR1", account.PUUID, account.scope})
				summoner, err := f.p.summonerByPUUID(lookup, account.PUUID)
				if err != nil || summoner.PUUID != r277ScopeDirectPUUID {
					t.Errorf("recovered summoner scope/endpoint mismatch: %+v err=%v", summoner, err)
				}
				ids, err := f.p.matchIDs(lookup, account.PUUID, 0, 10)
				if err != nil || len(ids) != 1 {
					t.Errorf("recovered ids mismatch: %v err=%v", ids, err)
				}
				seed, err := f.p.fetchAccountByPUUID(direct, r277ScopeDirectPUUID)
				if err != nil || seed.PUUID != r277ScopeDirectPUUID {
					t.Errorf("direct reviewed seed scope mismatch: %+v err=%v", seed, err)
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				if f.wrong != 0 {
					t.Errorf("wrong credential PUUID sent to actual by-puuid endpoints: %d", f.wrong)
				}
				if len(f.requests) < 2 || !strings.Contains(f.requests[0], "relay.example/r/asia/riot/account/v1/accounts/by-riot-id/") || !strings.Contains(f.requests[1], f.p.accountHost()+"/riot/account/v1/accounts/by-riot-id/") {
					t.Errorf("relay/direct account endpoint proof failed: %v", f.requests)
				}
				t.Logf("source=%s distinct_direct=%s distinct_relay=%s actual_endpoints=%v wrongscope_requests=%d", source, r277ScopeDirectPUUID, r277ScopeRelayPUUID, f.requests, f.wrong)
			})
		}
	}
}

func TestR277ScopePinnedAccountCannotTransparentlyOverflow(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		t.Run(source, func(t *testing.T) {
			f := r277NewScopeFixture(t, source, true)
			ctx := riotPinnedIdentityContext(context.Background())
			_, err := f.p.accountByRiotIDPinned(ctx, "ScopeFixture", "KR1")
			if !errors.Is(err, errRiotQuotaOverflow) {
				t.Errorf("pinned account transaction switched credentials below cache boundary: %v", err)
			}
			r277ScopeAssertIdentity(t, f.p, ctx, "")
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.requests) != 0 {
				t.Errorf("pinned direct identity transaction performed relay I/O: %v", f.requests)
			}
		})
	}
}

// Audit the related identity loaders with explicitly owned route contexts:
// stable account-by-puuid and summoner must retain separate keys and tokens.
func TestR277ScopeIdentityEndpointAudit(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		t.Run(source, func(t *testing.T) {
			f := r277NewScopeFixture(t, source, true)
			f.now = f.now.Add(121 * time.Second)
			direct := riotPinnedIdentityContext(context.Background())
			relay := context.WithValue(direct, riotRouteKey{}, "relay")
			for _, route := range []struct {
				ctx   context.Context
				puuid string
			}{
				{direct, r277ScopeDirectPUUID}, {relay, r277ScopeRelayPUUID},
			} {
				account, err := f.p.fetchAccountByPUUID(route.ctx, route.puuid)
				if err != nil || account.PUUID != route.puuid {
					t.Fatalf("account-by-puuid wrong scope: %+v %v", account, err)
				}
				summoner, err := f.p.summonerByPUUID(route.ctx, route.puuid)
				if err != nil || summoner.PUUID != route.puuid {
					t.Fatalf("summoner wrong scope: %+v %v", summoner, err)
				}
				if err := f.p.identityDisk.flushDisk(t.Context()); err != nil {
					t.Fatal(err)
				}
				for _, identity := range []string{"account-by-puuid:" + route.puuid, "summoner:" + route.puuid} {
					key := f.p.identityContextKey(route.ctx, identity)
					entry, err := f.p.identityDisk.readDisk(key)
					if err != nil || entry.Key != key || !strings.Contains(string(entry.Data), route.puuid) {
						t.Fatalf("identity audit persisted wrong scope: key=%s data=%s err=%v", key, entry.Data, err)
					}
					t.Logf("identity audit: scope=%s identity=%s key=%s", riotCredentialScope(route.ctx), identity, key)
				}
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.wrong != 0 || len(f.requests) != 4 {
				t.Fatalf("identity audit endpoints=%v wrongscope=%d", f.requests, f.wrong)
			}
			t.Logf("identity audit actual_endpoints=%v", f.requests)
		})
	}
}

func TestR277ScopeIDsOverflowResolvesDistinctRelayToken(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		t.Run(source, func(t *testing.T) {
			f := r277NewScopeFixture(t, source, true)
			direct := riotPinnedIdentityContext(context.Background())
			ctx := context.WithValue(direct, riotLookupIdentityKey{}, riotLookupIdentity{"ScopeFixture", "KR1", r277ScopeDirectPUUID, riotCredentialScope(direct)})
			ids, err := f.p.matchIDs(ctx, r277ScopeDirectPUUID, 0, 10)
			if err != nil || len(ids) != 1 {
				t.Fatalf("ids foreground overflow failed: %v %v", ids, err)
			}
			r277ScopeAssertIdentity(t, f.p, direct, "")
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.wrong != 0 || len(f.requests) != 2 || !strings.Contains(f.requests[1], "/by-puuid/"+r277ScopeRelayPUUID+"/ids") {
				t.Fatalf("ids overflow sent wrong token: %v wrong=%d", f.requests, f.wrong)
			}
			t.Logf("ids overflow actual_endpoints=%v", f.requests)
		})
	}
}
