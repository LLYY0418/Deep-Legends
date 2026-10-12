package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The R206 fixture deliberately has no local translation catalog. Return that
// exact 404 inline, leaving all collection HTTP and the public timeout real.
func r276MissingLocalTranslationTransport(origin *url.URL, upstream http.RoundTripper) http.RoundTripper {
	return r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Scheme == origin.Scheme && r.URL.Host == origin.Host && r.URL.Path == "/fe/lol-loot/trans.json" && r.URL.RawQuery == "" {
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Type", "application/json")
			http.NotFound(recorder, r)
			response := recorder.Result()
			response.Request = r
			return response, nil
		}
		return upstream.RoundTrip(r)
	})
}

func TestR276MissingTranslationFixturePreservesHTTPContract(t *testing.T) {
	origin, _ := url.Parse("http://fixture.invalid")
	var calls atomic.Int64
	transport := r276MissingLocalTranslationTransport(origin, r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return updateResponse(http.StatusAccepted, []byte("delegated")), nil
	}))
	request, _ := http.NewRequest(http.MethodGet, origin.String()+"/fe/lol-loot/trans.json", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	expected := httptest.NewRecorder()
	expected.Header().Set("Content-Type", "application/json")
	http.NotFound(expected, request)
	if err != nil || response.StatusCode != 404 || string(body) != expected.Body.String() || response.Header.Get("Content-Type") != expected.Header().Get("Content-Type") || response.Request != request || calls.Load() != 0 {
		t.Fatal("missing catalog response changed", response, string(body), err, calls.Load())
	}
	for _, item := range []struct{ method, address string }{
		{"POST", origin.String() + "/fe/lol-loot/trans.json"},
		{"GET", "http://other.invalid/fe/lol-loot/trans.json"},
		{"GET", "https://fixture.invalid/fe/lol-loot/trans.json"},
		{"GET", origin.String() + "/fe/lol-loot/trans.json?other=1"},
		{"GET", origin.String() + "/lol-game-data/assets/v1/skins.json"},
	} {
		r, _ := http.NewRequest(item.method, item.address, nil)
		got, err := transport.RoundTrip(r)
		if err != nil || got.StatusCode != http.StatusAccepted {
			t.Fatal("unrelated request was replaced", item, err)
		}
		got.Body.Close()
	}
	if calls.Load() != 5 {
		t.Fatal("unrelated HTTP lost", calls.Load())
	}
}

func TestR276TranslationFixtureReservesPublicTimeout(t *testing.T) {
	origin, _ := url.Parse("http://fixture.invalid")
	for _, inline := range []bool{false, true} {
		name := "old-local-hop-consumes-deadline"
		if inline {
			name = "inline-missing-catalog-keeps-public-wait"
		}
		t.Run(name, func(t *testing.T) {
			localAttempt := make(chan bool, 1)
			upstream := r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/fe/lol-loot/trans.json" {
					user, password, ok := r.BasicAuth()
					localAttempt <- ok && user == "riot" && password == "fixture" && r.Header.Get("Accept") == "application/json, image/*;q=0.9, */*;q=0.1"
					<-r.Context().Done() // Controlled missing hop: no sleep or disk dependency.
					return nil, r.Context().Err()
				}
				return updateResponse(404, []byte(`{}`)), nil
			})
			transport := http.RoundTripper(upstream)
			if inline {
				transport = r276MissingLocalTranslationTransport(origin, transport)
			}
			client := &LCUClient{baseURL: origin.String(), token: "fixture", http: &http.Client{Transport: transport}}
			provider := newChampionProvider()
			provider.lootTranslationTimeout = 300 * time.Millisecond
			provider.lootTranslations = map[string]lootMetadata{"CHEST_FIXTURE": {Name: "cached"}}
			observed := make(chan time.Duration, 1)
			provider.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "trans.json") {
					deadline, ok := r.Context().Deadline()
					if !ok {
						return nil, context.Canceled
					}
					observed <- time.Until(deadline)
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				return updateResponse(404, []byte(`{}`)), nil
			})}
			metadata := loadLootMetadata(context.Background(), client, provider, nil)
			remaining := <-observed
			if inline {
				if remaining <= 0 {
					t.Fatalf("public timeout already consumed: remaining=%s", remaining)
				}
				select {
				case <-localAttempt:
					t.Fatal("inline missing catalog still used delayed hop")
				default:
				}
			} else {
				if remaining > 0 || !<-localAttempt {
					t.Fatal("old delayed authenticated local hop was not reproduced", remaining)
				}
			}
			if metadata["CHEST_FIXTURE"].Name != "cached" {
				t.Fatal("cached translation lost")
			}
		})
	}
}
