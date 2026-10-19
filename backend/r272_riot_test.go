package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR272DetailDiskCrossCredential(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "anonymous-key-a")
	store := &localStore{root: t.TempDir()}
	cp := newChampionProvider()
	cp.cache = newChampionDataCache(store)
	var calls atomic.Int32
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return proHTTPBody([]byte(`{"metadata":{"matchId":"KR_272"},"info":{"gameId":272,"participants":[{"participantId":1,"championId":1}]}}`)), nil
	})}
	p := newRiotProvider(cp)
	for i := 0; i < 2; i++ {
		if _, _, err := p.matchByIDWithCache(context.Background(), "KR_272"); err != nil {
			t.Fatal(err)
		}
	}
	r274Flush(t, p.matchDisk)
	t.Setenv("RIOT_API_KEY", "anonymous-key-b")
	p = newRiotProvider(cp)
	_, status, err := p.matchByIDWithCache(context.Background(), "KR_272")
	if err != nil || status != "disk" || calls.Load() != 1 {
		t.Fatal("immutable details depend on credentials", status, calls.Load(), err)
	}
}
func TestR272FiveColdKROverviewFirstTen(t *testing.T) {
	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {

			r287TimingScenario(t, func(r287Context *r287TimingContext) {
				f := r98OverviewFixture(t, true)
				ref := gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr"}

				started := time.Now()
				firstCtx, firstProbe := r273TimingContext(f.a.riot)
				first, err := f.a.loadRiotOverview(firstCtx, ref, 0, 10)
				firstElapsed := time.Since(started)
				r273LogTimings(t, f, firstProbe, "cold-first-ten", started)
				if err != nil || len(first.Matches) != 10 || r287TooSlow(r287Context, t, "backend/r272_riot_test.go:48 duration", firstElapsed, 3*time.Second, false) {
					t.Fatal("cold first10 budget", len(first.Matches), firstElapsed, err)
				}
				firstMS := firstElapsed.Milliseconds()
				nextCtx, nextProbe := r273TimingContext(f.a.riot)
				nextStarted := time.Now()
				next, err := f.a.loadRiotOverview(withRiotBackground(nextCtx), ref, 10, 10)
				r273LogTimings(t, f, nextProbe, "background-next-ten", nextStarted)
				if err != nil || len(next.Matches) != 10 {
					t.Fatal("background next10", err)
				}
				f.mu.Lock()
				before := f.calls["detail"]
				f.mu.Unlock()
				for _, m := range append(first.Matches, next.Matches...) {
					if _, _, err := f.a.riot.matchByIDWithCache(context.Background(), fmt.Sprintf("KR_%d", m.GameID)); err != nil {
						t.Fatal(err)
					}
				}
				again, err := f.a.loadRiotOverview(context.Background(), ref, 0, 10)
				if err != nil || len(again.Matches) != 10 {
					t.Fatal(err)
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				if f.calls["detail"] != before || before != 20 {
					t.Fatal("repeated overview/details made extra detail requests", before, f.calls["detail"])
				}
				for _, event := range f.events {
					if event["event"] == "riot_request" && event["foreground"] == true && strings.Contains(strings.ToLower(riotTestString(event["route"])), "relay") {
						t.Error("foreground relay")
					}
				}
				t.Logf("synthetic player%d first10=%dms total20=%dms requests20 no repeated detail; observation ends before120s", i, firstMS, time.Since(started).Milliseconds())
			})
		})
	}
}
func riotTestString(value any) string { s, _ := value.(string); return s }

func TestR272Embedded80GuardAndUserIsolation(t *testing.T) {
	s, p, _ := r266Embedded(t)
	h := http.Header{}
	h.Set("X-App-Rate-Limit", "100:120")
	h.Set("X-App-Rate-Limit-Count", "80:120")
	p.observeEmbeddedQuota(h, 200)
	if !s.routeUntil.After(time.Now()) {
		t.Fatal("embedded 80-request protection did not switch to relay")
	}
	s.mu.Lock()
	s.key = "anonymous-own-key"
	s.routeUntil = time.Time{}
	s.mu.Unlock()
	p.observeEmbeddedQuota(h, 200)
	if !s.routeUntil.IsZero() {
		t.Fatal("own key shares embedded quota")
	}
	a := context.WithValue(context.Background(), riotPinnedCredentialKey{}, riotPinnedCredential{"anonymous-key-a", "user"})
	b := context.WithValue(context.Background(), riotPinnedCredentialKey{}, riotPinnedCredential{"anonymous-key-b", "user"})
	if p.identityContextKey(a, "account:anonymous#TEST") == p.identityContextKey(b, "account:anonymous#TEST") || riotAccountMemoryKey(a, "anonymous", "TEST") == riotAccountMemoryKey(b, "anonymous", "TEST") {
		t.Fatal("PUUID identity caches share credentials")
	}
}

func TestR272SameCredentialDiagnosticPrivate(t *testing.T) {
	for _, source := range []string{"env", "user", "embedded", "relay"} {
		for _, same := range []bool{false, true} {
			key := "anonymous-other-key"
			if same {
				key = "anonymous-embedded-key"
			}
			if source == "relay" {
				key = ""
			}
			event := riotSameCredentialDiagnostic(key, "anonymous-embedded-key", source)
			if len(event) != 3 || event["same_credential"] != (same && source != "relay") || event["source"] != source {
				t.Fatal("credential diagnostic contract")
			}
			raw, _ := json.Marshal(event)
			if strings.Contains(string(raw), "key") || strings.Contains(string(raw), "hash") || strings.Contains(string(raw), "salt") {
				t.Fatal("credential diagnostic leaked material")
			}
		}
	}
}
