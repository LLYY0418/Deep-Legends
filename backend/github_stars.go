package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

const githubStarsURL = "https://api.github.com/repos/LLYY0418/Deep-Legends"

type githubStarsValue struct {
	Count *int      `json:"count"`
	At    time.Time `json:"at"`
}
type githubStarsCache struct {
	mu      sync.Mutex
	loaded  bool
	value   githubStarsValue
	retryAt time.Time
	now     func() time.Time
}

// One fixed public repository, no cookies, authorization, redirects or user data.
// The lock coalesces simultaneous settings visits into one bounded request.
func (a *app) githubStarCount(ctx context.Context) (*int, bool) {
	c := &a.githubStars
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	if !c.loaded {
		c.loaded = true
		if a.storage != nil {
			if raw, err := readLocalStoreFile(a.storage, "github-stars.json"); err == nil && len(raw) <= 1024 {
				var value githubStarsValue
				if json.Unmarshal(raw, &value) == nil && value.Count != nil && *value.Count >= 0 {
					c.value = value
				}
			}
		}
	}
	if c.value.Count != nil && now.Sub(c.value.At) >= 0 && now.Sub(c.value.At) < 6*time.Hour {
		count := *c.value.Count
		return &count, true
	}
	if now.Before(c.retryAt) {
		return nil, true
	}
	// Cache only public count and time. Failure never exposes a previous stale number.
	c.value = githubStarsValue{}
	c.retryAt = now.Add(30 * time.Minute)
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, githubStarsURL, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Deep-Legends")
	client := a.externalHTTPClient(&http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
	response, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 256*1024+1))
	if err != nil || len(raw) > 256*1024 {
		return nil, false
	}
	var payload struct {
		Count *int `json:"stargazers_count"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Count == nil || *payload.Count < 0 {
		return nil, false
	}
	c.value = githubStarsValue{Count: payload.Count, At: now}
	c.retryAt = time.Time{}
	if a.storage != nil {
		data, _ := json.Marshal(c.value)
		_ = writeLocalStoreFile(a.storage, "github-stars.json", data)
	}
	count := *payload.Count
	return &count, false
}

func (a *app) handleGithubStars(w http.ResponseWriter, r *http.Request) {
	count, cached := a.githubStarCount(r.Context())
	result := "failed"
	if count != nil {
		result = "ok"
	}
	a.recordDiagnostic(map[string]any{"event": "github_stars", "result": result, "cached": cached})
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, struct {
		Count *int `json:"count"`
	}{count})
}
