package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

//go:embed patchnotes/patch-notes.json
var bundledPatchNotes []byte

const patchNotesLimit = 512 << 10

var patchVersion = regexp.MustCompile(`^\d{1,3}(?:\.\d{1,3}){1,3}$`)
var patchSectionID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type patchNoteLine struct {
	Text string `json:"text,omitempty"`
	K    string `json:"k,omitempty"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}
type patchNoteGroup struct {
	Label string          `json:"label,omitempty"`
	Lines []patchNoteLine `json:"lines"`
}
type patchNoteEntry struct {
	Name       string           `json:"name"`
	ChampionID int64            `json:"championId,omitempty"`
	ItemID     int64            `json:"itemId,omitempty"`
	AugmentID  int64            `json:"augmentId,omitempty"`
	IsNew      bool             `json:"isNew,omitempty"`
	Groups     []patchNoteGroup `json:"groups"`
}
type patchNoteSection struct {
	ID      string           `json:"id"`
	Title   string           `json:"title"`
	Intro   string           `json:"intro,omitempty"`
	Entries []patchNoteEntry `json:"entries,omitempty"`
	Items   []string         `json:"items,omitempty"`
}
type patchNoteMode struct {
	Sections []patchNoteSection `json:"sections"`
}
type patchNoteModes struct {
	Ranked patchNoteMode `json:"ranked"`
	Mayhem patchNoteMode `json:"aramMayhem"`
	Arena  patchNoteMode `json:"arena"`
}
type patchNoteDocument struct {
	Schema      int            `json:"schema"`
	Version     string         `json:"version"`
	PublishedAt string         `json:"publishedAt"`
	SourceURL   string         `json:"sourceUrl"`
	Modes       patchNoteModes `json:"modes"`
}
type patchNoteFeed struct {
	Schema   int                 `json:"schema"`
	Versions []patchNoteDocument `json:"versions"`
}
type patchNotesCache struct {
	mu        sync.Mutex
	documents []patchNoteDocument
	started   bool
}

func validPatchSource(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Host == "lol.qq.com" && len(value) <= 256 && u.User == nil && u.RawQuery == "" && u.Fragment == "" && regexp.MustCompile(`^/gicp/news/410/\d+\.html$`).MatchString(u.Path)
}
func validPatchText(value string, max int, required bool) bool {
	if required && strings.TrimSpace(value) == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > max {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func validatePatchDocument(d patchNoteDocument) error {
	if d.Schema != 1 || !patchVersion.MatchString(d.Version) || !validPatchSource(d.SourceURL) {
		return errors.New("invalid patch document")
	}
	if _, err := time.Parse("2006-01-02", d.PublishedAt); err != nil {
		return err
	}
	count := 0
	for _, mode := range []patchNoteMode{d.Modes.Ranked, d.Modes.Mayhem, d.Modes.Arena} {
		if mode.Sections == nil || len(mode.Sections) > 64 {
			return errors.New("invalid mode sections")
		}
		seen := map[string]bool{}
		for _, s := range mode.Sections {
			count++
			if !patchSectionID.MatchString(s.ID) || seen[s.ID] || !validPatchText(s.Title, 160, true) || !validPatchText(s.Intro, 4096, false) || len(s.Entries) > 256 || len(s.Items) > 256 {
				return errors.New("invalid section")
			}
			seen[s.ID] = true
			for _, item := range s.Items {
				count++
				if !validPatchText(item, 4096, true) {
					return errors.New("invalid bug item")
				}
			}
			for _, e := range s.Entries {
				count++
				if !validPatchText(e.Name, 160, true) || e.ChampionID < 0 || e.ChampionID > 100000000 || e.ItemID < 0 || e.ItemID > 100000000 || e.AugmentID < 0 || e.AugmentID > 100000000 || e.Groups == nil || len(e.Groups) > 64 {
					return errors.New("invalid entry")
				}
				for _, g := range e.Groups {
					count++
					if !validPatchText(g.Label, 160, false) || g.Lines == nil || len(g.Lines) > 256 {
						return errors.New("invalid group")
					}
					for _, l := range g.Lines {
						count++
						if l.Text != "" {
							if l.K != "" || l.From != "" || l.To != "" || !validPatchText(l.Text, 4096, true) {
								return errors.New("invalid text line")
							}
						} else if !validPatchText(l.K, 256, true) || !validPatchText(l.From, 2048, true) || !validPatchText(l.To, 2048, true) {
							return errors.New("invalid value line")
						}
					}
				}
			}
		}
	}
	if count > 6000 {
		return errors.New("patch content count limit")
	}
	return nil
}
func decodePatchNotes(raw []byte) ([]patchNoteDocument, error) {
	if len(raw) > patchNotesLimit {
		return nil, errors.New("patch notes too large")
	}
	var feed patchNoteFeed
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&feed) != nil || decoder.Decode(&extra) != io.EOF || feed.Schema != 1 || len(feed.Versions) == 0 || len(feed.Versions) > 3 {
		return nil, errors.New("invalid patch feed")
	}
	seen := map[string]bool{}
	for _, d := range feed.Versions {
		if err := validatePatchDocument(d); err != nil {
			return nil, err
		}
		if seen[canonicalPatchVersion(d.Version)] {
			return nil, errors.New("duplicate patch version")
		}
		seen[canonicalPatchVersion(d.Version)] = true
	}
	return mergePatchNotes(nil, feed.Versions), nil
}
func canonicalPatchVersion(value string) string {
	parts := strings.Split(value, ".")
	for i, p := range parts {
		n, _ := strconv.Atoi(p)
		parts[i] = strconv.Itoa(n)
	}
	for len(parts) > 1 && parts[len(parts)-1] == "0" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, ".")
}
func comparePatchVersions(a, b string) int {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(left), len(right)); i++ {
		x, y := 0, 0
		if i < len(left) {
			x, _ = strconv.Atoi(left[i])
		}
		if i < len(right) {
			y, _ = strconv.Atoi(right[i])
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
func mergePatchNotes(bundled, remote []patchNoteDocument) []patchNoteDocument {
	byVersion := map[string]patchNoteDocument{}
	for _, d := range bundled {
		byVersion[canonicalPatchVersion(d.Version)] = d
	}
	for _, d := range remote {
		byVersion[canonicalPatchVersion(d.Version)] = d
	}
	out := make([]patchNoteDocument, 0, len(byVersion))
	for _, d := range byVersion {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return comparePatchVersions(out[i].Version, out[j].Version) > 0 })
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}
func (a *app) patchNotesState() *patchNotesCache {
	a.patchNotesOnce.Do(func() {
		documents, _ := decodePatchNotes(bundledPatchNotes)
		if a.storage != nil {
			if file, err := os.Open(filepath.Join(a.storage.root, "patch-notes.json")); err == nil {
				raw, err := readLimited(file, patchNotesLimit)
				file.Close()
				if err == nil {
					if cached, err := decodePatchNotes(raw); err == nil {
						documents = mergePatchNotes(documents, cached)
					}
				}
			}
		}
		a.patchNotes = &patchNotesCache{documents: documents}
	})
	return a.patchNotes
}
func fetchPatchNotesDocument(ctx context.Context, client *http.Client) ([]byte, []patchNoteDocument, error) {
	for _, prefix := range updateMirrors {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, prefix+announcementBranchURL+"patch-notes.json", nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		raw, err := readLimited(resp.Body, patchNotesLimit)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			continue
		}
		if documents, err := decodePatchNotes(raw); err == nil {
			return raw, documents, nil
		}
	}
	return nil, nil, errors.New("patch notes fetch failed")
}
func (a *app) startPatchNotes(ctx context.Context) {
	cache := a.patchNotesState()
	cache.mu.Lock()
	if cache.started {
		cache.mu.Unlock()
		return
	}
	cache.started = true
	cache.mu.Unlock()
	a.goSafe("patch-notes-startup", func() {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		raw, remote, err := fetchPatchNotesDocument(ctx, a.externalHTTPClient(&http.Client{Timeout: 2 * time.Second}))
		if err != nil {
			return
		}
		cache.mu.Lock()
		cache.documents = mergePatchNotes(cache.documents, remote)
		cache.mu.Unlock()
		if a.storage != nil {
			if err := writeLocalStoreFile(a.storage, "patch-notes.json", raw); err != nil {
				a.recordDiagnostic(map[string]any{"event": "patch_notes_cache", "saved": false})
			}
		}
		a.broadcastEvent(`{"type":"patch-notes-changed"}`)
	})
}
func (a *app) handlePatchNotes(w http.ResponseWriter, r *http.Request) {
	c := a.patchNotesState()
	c.mu.Lock()
	defer c.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	if len(c.documents) == 0 {
		http.Error(w, "暂时无法获取版本公告", 503)
		return
	}
	respondJSON(w, c.documents[0])
}
