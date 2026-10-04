package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const r204FixtureKey = "RGAPI-fixture-only-credential"

func r204KeyFixture(t *testing.T) *riotKeyStore {
	t.Helper()
	old, embedded := riotUserKeys, riotEmbeddedKey
	t.Setenv("RIOT_API_KEY", "")
	riotEmbeddedKey = func() string { return "" }
	riotUserKeys = loadRiotKeyStore(newFlowDiagnosticStore(t))
	t.Cleanup(func() { riotUserKeys, riotEmbeddedKey = old, embedded })
	return riotUserKeys
}
func r204ValidationClient(status int) *http.Client {
	return &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if status == 0 {
			return nil, errors.New("fixture network failure")
		}
		return updateResponse(status, []byte(`{}`)), nil
	})}
}
func TestR204KeyPriority(t *testing.T) {
	s := r204KeyFixture(t)
	if riotKeySource() != "none" || riotKey() != "" {
		t.Fatal("expected no key")
	}
	riotEmbeddedKey = func() string { return "fixture-embedded" }
	if riotKey() != "" || riotKeySource() != "none" {
		t.Fatal("legacy embedded key must not be a runtime fallback after R206")
	}
	s.mu.Lock()
	err := s.writeLocked(r204FixtureKey)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if riotKey() != r204FixtureKey || riotKeySource() != "user" {
		t.Fatal("user must override embedded")
	}
	t.Setenv("RIOT_API_KEY", "fixture-env")
	if riotKey() != "fixture-env" || riotKeySource() != "env" {
		t.Fatal("env must override both")
	}
}
func TestR204KeySaveAndClear(t *testing.T) {
	s := r204KeyFixture(t)
	for _, status := range []int{200, 401, 403, 0} {
		key := r204FixtureKey + string(rune('a'+status%26))
		before := s.key
		result, err := s.save(context.Background(), r204ValidationClient(status), key)
		if err != nil {
			t.Fatal(err)
		}
		if status == 401 || status == 403 {
			if result != "invalid" || s.key != before || riotKeyState() != "invalid" {
				t.Fatal("rejected credential was saved or incorrectly marked")
			}
		} else {
			want := "ok"
			if status == 0 {
				want = "network_error"
			}
			if result != want || riotKey() != key || riotKeyState() != "configured" {
				t.Fatal("accepted credential unavailable")
			}
			reloaded := loadRiotKeyStore(s.store)
			if reloaded.key != key {
				t.Fatal("saved credential cannot reload")
			}
			info, err := os.Stat(filepath.Join(s.store.root, riotUserKeyFile))
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
				t.Fatal("key file permissions")
			}
			data, _ := os.ReadFile(filepath.Join(s.store.root, riotUserKeyFile))
			if runtime.GOOS == "windows" && bytes.Contains(data, []byte(key)) {
				t.Fatal("DPAPI did not encrypt")
			}
		}
	}
	riotEmbeddedKey = func() string { return "fixture-embedded" }
	s.mu.Lock()
	err := s.writeLocked("")
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if riotKey() != "" || riotKeySource() != "none" {
		t.Fatal("clear unexpectedly reactivated legacy embedded key")
	}
	riotEmbeddedKey = func() string { return "" }
	if riotKeyState() != "unconfigured" || riotKeyConfigured() {
		t.Fatal("clear did not disable")
	}
}
func TestR204KeyMigration(t *testing.T) {
	for _, kind := range []string{"embedded", "user", "public", "cleared"} {
		t.Run(kind, func(t *testing.T) {
			s := r204KeyFixture(t)
			want := "ok"
			riotEmbeddedKey = func() string { return "fixture-embedded" }
			if kind == "user" || kind == "cleared" {
				s.mu.Lock()
				key := r204FixtureKey
				if kind == "cleared" {
					key = ""
				}
				err := s.writeLocked(key)
				s.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
				want = "skipped_exists"
			}
			if kind == "public" {
				riotEmbeddedKey = func() string { return "" }
				want = "skipped_no_embedded"
			}
			var event map[string]any
			if err := s.migrateEmbedded(func(row map[string]any) { event = row }); err != nil {
				t.Fatal(err)
			}
			if event["result"] != want {
				t.Fatal(event)
			}
			if kind == "embedded" && loadRiotKeyStore(s.store).key != "fixture-embedded" {
				t.Fatal("migration unavailable after restart")
			}
			if kind == "user" && s.key != r204FixtureKey {
				t.Fatal("migration overwrote user")
			}
			if kind == "cleared" && s.key != "" {
				t.Fatal("migration undid clear")
			}
		})
	}
}
func TestR204KeyRuntime401AndPrivacy(t *testing.T) {
	s := r204KeyFixture(t)
	a := &app{storage: s.store, champions: newChampionProvider()}
	a.championDataProvider().client = r204ValidationClient(200)
	req := httptest.NewRequest(http.MethodPost, "/api/riot-key", strings.NewReader(`{"key":"`+r204FixtureKey+`"}`))
	w := httptest.NewRecorder()
	a.handleRiotKeySettings(w, req)
	if w.Code != 200 || riotKey() != r204FixtureKey {
		t.Fatal("settings save failed", w.Code)
	}
	// Exercise the actual authenticated Riot request branch, not observe alone.
	p := newRiotProvider(a.championDataProvider())
	p.champions.client = r204ValidationClient(401)
	var out map[string]any
	if err := p.get(context.Background(), "kr.api.riotgames.com", "/lol/status/v4/platform-data", nil, &out); err == nil || riotKeyState() != "invalid" {
		t.Fatal("runtime 401 not reflected")
	}
	s.migrateEmbedded(a.recordDiagnostic)
	a.recordDiagnostic(map[string]any{"event": "app_start", "riot_key_source": riotKeySource()})
	data, err := s.store.readDiagnosticLogForExport()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{r204FixtureKey, "RGAPI-"} {
		if bytes.Contains(data, []byte(secret)) || strings.Contains(w.Body.String(), secret) {
			t.Fatal("credential leaked")
		}
	}
	if runtime.GOOS == "windows" {
		cipher, _ := os.ReadFile(filepath.Join(s.store.root, riotUserKeyFile))
		if len(cipher) > 0 && bytes.Contains(data, cipher) {
			t.Fatal("cipher leaked")
		}
	}
	var settings map[string]any
	if json.Unmarshal(w.Body.Bytes(), &settings) != nil || settings["source"] != "user" {
		t.Fatal("credential status response")
	}
}

func TestR204CredentialChangeDetachesOldSpecialistFlight(t *testing.T) {
	r204KeyFixture(t)
	p := newRiotProvider(newChampionProvider())
	a := &app{riot: p}
	old := &specialistRuneFlight{done: make(chan struct{}), position: "mid"}
	key := specialistRuneKey(64, "mid")
	p.specialistFlights[key] = old
	a.clearRiotCredentialFailures()
	if p.specialistFlights[key] != nil {
		t.Fatal("new key joins old credential flight")
	}
	fresh := &specialistRuneFlight{done: make(chan struct{}), position: "mid"}
	p.specialistFlights[key] = fresh
	p.finishSpecialistRuneFlight(key, old, nil, specialistOutcomeTimeout, time.Now())
	if p.specialistFlights[key] != fresh || len(p.specialistCache) != 0 {
		t.Fatal("old credential response replaced fresh flight or cache")
	}
	p.finishSpecialistRuneFlight(key, fresh, nil, specialistOutcomeNoPositionSample, time.Now())
}
