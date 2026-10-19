package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR290GithubStarsCacheAnonymousRoutingAndExpiry(t *testing.T) {
	var requests atomic.Int32
	now := time.Now()
	client := &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.String() != githubStarsURL || r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Local-Token") != "" {
			t.Errorf("unsafe outbound request: %s", r.URL)
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Error("missing five second timeout")
		}
		return updateResponse(200, []byte(`{"stargazers_count":1234,"private":"not-persisted"}`)), nil
	})}
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	a := &app{storage: store, champions: &championProvider{client: client}}
	a.githubStars.now = func() time.Time { return now }
	if requests.Load() != 0 {
		t.Fatal("startup requested stars")
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			count, _ := a.githubStarCount(context.Background())
			if count == nil || *count != 1234 {
				t.Error("count missing")
			}
		}()
	}
	group.Wait()
	if requests.Load() != 1 {
		t.Fatal("memory cache/single flight", requests.Load())
	}
	raw, err := readLocalStoreFile(store, "github-stars.json")
	if err != nil || strings.Contains(string(raw), "not-persisted") {
		t.Fatal(string(raw), err)
	}
	var disk map[string]any
	if json.Unmarshal(raw, &disk) != nil || len(disk) != 2 {
		t.Fatal(string(raw))
	}
	restarted := &app{storage: store, champions: &championProvider{client: client}}
	restarted.githubStars.now = func() time.Time { return now }
	count, cached := restarted.githubStarCount(context.Background())
	if !cached || count == nil || *count != 1234 || requests.Load() != 1 {
		t.Fatal("disk cache miss")
	}
	now = now.Add(6 * time.Hour)
	_, cached = restarted.githubStarCount(context.Background())
	if cached || requests.Load() != 2 {
		t.Fatal("success cache failed to expire")
	}
}

func TestR290GithubStarsFailureCooldownAndZero(t *testing.T) {
	for _, payload := range []string{`{}`, `{"stargazers_count":-1}`, `{"stargazers_count":"1"}`, `broken`, `{"stargazers_count":0}`} {
		t.Run(payload, func(t *testing.T) {
			now := time.Now()
			requests := 0
			a := &app{champions: &championProvider{client: &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) {
				requests++
				return updateResponse(200, []byte(payload)), nil
			})}}}
			a.githubStars.now = func() time.Time { return now }
			w := httptest.NewRecorder()
			a.handleGithubStars(w, httptest.NewRequest("GET", "/api/github/stars", nil))
			var out struct {
				Count *int `json:"count"`
			}
			if json.Unmarshal(w.Body.Bytes(), &out) != nil || w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			if payload == `{"stargazers_count":0}` {
				if out.Count == nil || *out.Count != 0 {
					t.Fatal("zero lost")
				}
				return
			}
			if out.Count != nil {
				t.Fatal("failure displayed a number")
			}
			_, cached := a.githubStarCount(context.Background())
			if !cached || requests != 1 {
				t.Fatal("failure retried")
			}
			now = now.Add(30 * time.Minute)
			a.githubStarCount(context.Background())
			if requests != 2 {
				t.Fatal("failure cooldown did not expire")
			}
		})
	}
	for _, status := range []int{404, 403, 429, 500} {
		a := &app{champions: &championProvider{client: &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) {
			return updateResponse(status, []byte(`{"stargazers_count":99}`)), nil
		})}}}
		if count, _ := a.githubStarCount(context.Background()); count != nil {
			t.Fatal("HTTP failure returned stars", status)
		}
	}
}
