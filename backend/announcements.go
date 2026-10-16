package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed announcements/announcements.json
var bundledAnnouncements []byte

const announcementBranchURL = "https://github.com/" + updateRepo + "/raw/refs/heads/notices/"

type announcement struct {
	ID          string   `json:"id"`
	Revision    string   `json:"revision"`
	Popup       bool     `json:"popup"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Content     string   `json:"content"`
	Images      []string `json:"images"`
	PublishedAt string   `json:"publishedAt"`
	UpdatedAt   string   `json:"updatedAt"`
	ExpiresAt   string   `json:"expiresAt,omitempty"`
}
type announcementImage struct {
	body        []byte
	contentType string
}
type announcementCache struct {
	mu            sync.Mutex
	notices       []announcement
	source        string
	at            time.Time
	fetching      bool
	fetchFailed   bool
	images        map[string]announcementImage
	loadingImages map[string]bool
}

var noticeIdentifier = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,80}$`)

func announcementImagePath(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "" && u.Host == "" && u.RawQuery == "" && u.Fragment == "" && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && path.Clean(value) == value && !strings.Contains(value, "..") && strings.HasPrefix(value, "images/") && len(value) <= 180
}
func decodeAnnouncements(data []byte) ([]announcement, error) {
	if len(data) > 256<<10 {
		return nil, errors.New("notices too large")
	}
	var objects []map[string]json.RawMessage
	if json.Unmarshal(data, &objects) != nil || objects == nil || len(objects) > 32 {
		return nil, errors.New("invalid notice list")
	}
	var out []announcement
	seen := map[string]bool{}
	for _, o := range objects {
		for _, key := range []string{"id", "revision", "popup", "title", "summary", "content", "images", "publishedAt", "updatedAt"} {
			if o[key] == nil || string(o[key]) == "null" {
				return nil, errors.New("missing notice field")
			}
		}
		if string(o["expiresAt"]) == "null" {
			return nil, errors.New("null expiry")
		}
		raw, _ := json.Marshal(o)
		var n announcement
		for key := range o {
			switch key {
			case "id", "revision", "popup", "title", "summary", "content", "images", "publishedAt", "updatedAt", "expiresAt":
			default:
				return nil, errors.New("unknown notice field")
			}
		}
		if json.Unmarshal(raw, &n) != nil || n.Images == nil || !noticeIdentifier.MatchString(n.ID) || !noticeIdentifier.MatchString(n.Revision) || seen[n.ID] || len(n.Title) == 0 || len(n.Title) > 240 || len(n.Summary) > 800 || len(n.Content) > 32<<10 || len(n.Images) > 8 {
			return nil, errors.New("invalid notice fields")
		}
		for _, stamp := range []string{n.PublishedAt, n.UpdatedAt} {
			if _, err := time.Parse(time.RFC3339, stamp); err != nil {
				return nil, err
			}
		}
		if n.ExpiresAt != "" {
			if _, err := time.Parse(time.RFC3339, n.ExpiresAt); err != nil {
				return nil, err
			}
		}
		for _, image := range n.Images {
			if !announcementImagePath(image) {
				return nil, errors.New("invalid notice image")
			}
		}
		seen[n.ID] = true
		out = append(out, n)
	}
	return out, nil
}
func (a *app) announcementState() *announcementCache {
	a.announcementsOnce.Do(func() {
		notices, _ := decodeAnnouncements(bundledAnnouncements)
		source := "bundled"
		if a.storage != nil {
			if raw, err := readLocalStoreFile(a.storage, "announcements.json"); err == nil {
				if cached, err := decodeAnnouncements(raw); err == nil {
					notices = mergeAnnouncements(notices, cached)
					source = "cache"
				}
			}
		}
		a.announcements = &announcementCache{notices: notices, source: source, images: map[string]announcementImage{}, loadingImages: map[string]bool{}}
	})
	return a.announcements
}
func mergeAnnouncements(bundled, remote []announcement) []announcement {
	out := append([]announcement(nil), remote...)
	ids := map[string]bool{}
	for _, n := range out {
		ids[n.ID] = true
	}
	for _, n := range bundled {
		if !ids[n.ID] {
			out = append(out, n)
		}
	}
	return out
}
func (a *app) fetchAnnouncements() {
	c := a.announcementState()
	c.mu.Lock()
	if c.fetching || !c.at.IsZero() && time.Since(c.at) < time.Hour {
		c.mu.Unlock()
		return
	}
	c.fetching = true
	c.at = time.Now()
	c.mu.Unlock()
	a.goSafe("announcements-fetch", func() {
		defer func() { c.mu.Lock(); c.fetching = false; c.mu.Unlock() }()
		ctx, cancel := context.WithTimeout(a.licenseBusinessContext(), 4*time.Second)
		defer cancel()
		client := a.externalHTTPClient(&http.Client{Timeout: 2 * time.Second})
		for _, prefix := range updateMirrors {
			req, _ := http.NewRequestWithContext(ctx, "GET", prefix+announcementBranchURL+"announcements.json", nil)
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			raw, err := readLimited(resp.Body, 256<<10)
			resp.Body.Close()
			if err != nil || resp.StatusCode != 200 {
				continue
			}
			notices, err := decodeAnnouncements(raw)
			if err != nil {
				continue
			}
			bundle, _ := decodeAnnouncements(bundledAnnouncements)
			c.mu.Lock()
			c.notices = mergeAnnouncements(bundle, notices)
			c.source = "remote"
			c.fetchFailed = false
			c.at = time.Now()
			c.images = map[string]announcementImage{}
			c.mu.Unlock()
			if a.storage != nil {
				_ = writeLocalStoreFile(a.storage, "announcements.json", raw)
			}
			a.broadcastEvent(`{"type":"announcements-changed"}`)
			return
		}
		c.mu.Lock()
		c.fetchFailed = true
		c.mu.Unlock()
		a.recordDiagnostic(map[string]any{"event": "announcement_state", "id": "fetch", "revision": "", "source": "bundled", "action": "fetch-failed"})
	})
}
func (a *app) handleAnnouncements(w http.ResponseWriter, r *http.Request) {
	c := a.announcementState()
	a.fetchAnnouncements()
	c.mu.Lock()
	list := append([]announcement(nil), c.notices...)
	source := c.source
	c.mu.Unlock()
	active := list[:0]
	for _, n := range list {
		if n.ExpiresAt != "" {
			expires, _ := time.Parse(time.RFC3339, n.ExpiresAt)
			if !expires.After(time.Now()) {
				continue
			}
		}
		active = append(active, n)
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, map[string]any{"announcements": active, "source": source})
}
func (a *app) handleAnnouncementImage(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("path")
	if !announcementImagePath(name) {
		http.Error(w, "invalid image", 400)
		return
	}
	c := a.announcementState()
	c.mu.Lock()
	allowed := false
	for _, n := range c.notices {
		for _, image := range n.Images {
			allowed = allowed || image == name
		}
	}
	cached, ok := c.images[name]
	c.mu.Unlock()
	if !allowed {
		http.NotFound(w, r)
		return
	}
	if !ok {
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		client := a.externalHTTPClient(&http.Client{Timeout: 2 * time.Second})
		for _, prefix := range updateMirrors {
			req, _ := http.NewRequestWithContext(ctx, "GET", prefix+announcementBranchURL+name, nil)
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			raw, err := readLimited(resp.Body, 1<<20)
			resp.Body.Close()
			if err != nil || resp.StatusCode != 200 {
				continue
			}
			kind := http.DetectContentType(raw)
			kind, _, _ = mime.ParseMediaType(kind)
			if kind != "image/png" && kind != "image/jpeg" && kind != "image/gif" && kind != "image/webp" {
				continue
			}
			cached = announcementImage{raw, kind}
			c.mu.Lock()
			if len(c.images) < 64 {
				c.images[name] = cached
			}
			c.mu.Unlock()
			ok = true
			break
		}
	}
	if !ok {
		http.Error(w, "image unavailable", 502)
		return
	}
	w.Header().Set("Content-Type", cached.contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(cached.body)
}
