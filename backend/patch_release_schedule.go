package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

const officialPatchScheduleURL = "https://support.riotgames.com/en-us/league-of-legends/gameplay/patch-schedule-league-of-legends/"
const patchReleaseStateFile = "patch-release-schedule.json"

//go:embed patchnotes/release-schedule-2026.json
var bundledPatchReleaseSchedule []byte

// This is an application preference, not a conversion of Riot's PT release
// instant: run at the END of the listed calendar day in Beijing (next 00:00).
// Fixed UTC+8 also works in Windows distributions without a zoneinfo database.
var patchReleaseLocation = time.FixedZone("Asia/Shanghai", 8*60*60)
var releaseVersionPattern = regexp.MustCompile(`^\d{2}\.\d{2}$`)
var releaseDatePattern = regexp.MustCompile(`^[A-Z][a-z]+ \d{1,2}, \d{4}`)

type patchRelease struct {
	Version string `json:"version"`
	Date    string `json:"date"`
}

func (r patchRelease) updateAt() time.Time {
	day, _ := time.ParseInLocation("2006-01-02", r.Date, patchReleaseLocation)
	return day.AddDate(0, 0, 1)
}

type patchReleaseState struct {
	Schema             int            `json:"schema"`
	Releases           []patchRelease `json:"releases"`
	LastAttemptVersion string         `json:"lastAttemptVersion,omitempty"`
	AttemptedAt        time.Time      `json:"attemptedAt,omitempty"`
	CalendarChecked    string         `json:"calendarChecked,omitempty"`
}

func validPatchReleases(rows []patchRelease) bool {
	if len(rows) == 0 || len(rows) > 96 {
		return false
	}
	seen := map[string]bool{}
	for _, row := range rows {
		day, err := time.Parse("2006-01-02", row.Date)
		if err != nil || !releaseVersionPattern.MatchString(row.Version) || seen[row.Version] || day.Year() != 2000+int(row.Version[0]-'0')*10+int(row.Version[1]-'0') || row.Version[3:] == "00" {
			return false
		}
		seen[row.Version] = true
	}
	return true
}

func mergePatchReleases(old, current []patchRelease) []patchRelease {
	byVersion := map[string]patchRelease{}
	for _, rows := range [][]patchRelease{old, current} {
		for _, row := range rows {
			byVersion[row.Version] = row
		}
	}
	rows := make([]patchRelease, 0, len(byVersion))
	for _, row := range byVersion {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return comparePatchVersions(rows[i].Version, rows[j].Version) < 0 })
	if len(rows) > 72 {
		rows = rows[len(rows)-72:]
	}
	return rows
}

func parseOfficialPatchSchedule(raw []byte) ([]patchRelease, error) {
	root, err := xhtml.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	var rows []patchRelease
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "tr" {
			var cells []string
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == xhtml.ElementNode && child.Data == "td" {
					cells = append(cells, strings.Join(strings.Fields(nodeText(child)), " "))
				}
			}
			if len(cells) == 2 && releaseVersionPattern.MatchString(cells[0]) {
				day, err := time.Parse("January 2, 2006", releaseDatePattern.FindString(cells[1]))
				if err != nil {
					rows = append(rows, patchRelease{}) // reject a partial/malformed table
				} else {
					rows = append(rows, patchRelease{Version: cells[0], Date: day.Format("2006-01-02")})
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	if len(rows) < 10 || !validPatchReleases(rows) {
		return nil, errors.New("official patch schedule table is unavailable or incomplete")
	}
	return mergePatchReleases(nil, rows), nil
}

func (a *app) loadPatchReleaseState() *patchReleaseState {
	var bundle struct {
		Releases []patchRelease `json:"releases"`
	}
	_ = json.Unmarshal(bundledPatchReleaseSchedule, &bundle)
	state := &patchReleaseState{Schema: 1, Releases: bundle.Releases}
	if a.storage != nil {
		if file, err := os.Open(filepath.Join(a.storage.root, patchReleaseStateFile)); err == nil {
			raw, readErr := readLimited(file, 128<<10)
			file.Close()
			var cached patchReleaseState
			if readErr == nil && json.Unmarshal(raw, &cached) == nil && cached.Schema == 1 && validPatchReleases(cached.Releases) && (cached.LastAttemptVersion == "" || releaseVersionPattern.MatchString(cached.LastAttemptVersion)) && !cached.AttemptedAt.After(time.Now().Add(5*time.Minute)) && len(cached.CalendarChecked) < 100 {
				cached.Releases = mergePatchReleases(bundle.Releases, cached.Releases)
				state = &cached
			}
		}
	}
	return state
}

func (a *app) savePatchReleaseState(state *patchReleaseState) error {
	if a.storage == nil {
		return nil
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeLocalStoreFile(a.storage, patchReleaseStateFile, raw)
}

// Pick only the most recent missed release, never replay a season's requests.
func (s *patchReleaseState) pending(now time.Time) (patchRelease, time.Time) {
	var due patchRelease
	var next time.Time
	for _, row := range s.Releases {
		at := row.updateAt()
		if at.After(now) {
			if next.IsZero() || at.Before(next) {
				next = at
			}
		} else if comparePatchVersions(row.Version, s.LastAttemptVersion) > 0 && (due.Version == "" || comparePatchVersions(row.Version, due.Version) > 0) {
			due = row
		}
	}
	if next.IsZero() {
		local := now.In(patchReleaseLocation)
		lastYear := 0
		for _, row := range s.Releases {
			day, _ := time.Parse("2006-01-02", row.Date)
			if year := day.Year(); year > lastYear {
				lastYear = year
			}
		}
		if local.Year() <= lastYear {
			next = time.Date(lastYear+1, 1, 1, 0, 0, 0, 0, patchReleaseLocation)
		} else {
			// A new year's table may not yet exist. Check the calendar once a day
			// until it does; this does not poll announcements or augment endpoints.
			next = time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, patchReleaseLocation)
		}
	}
	return due, next
}

func (s *patchReleaseState) step(now time.Time, calendar func() ([]patchRelease, error), save func() error, refresh func(time.Time)) time.Time {
	due, next := s.pending(now)
	calendarKey := ""
	if due.Version != "" {
		calendarKey = due.Version + "@" + due.Date
	}
	maxYear := 0
	for _, row := range s.Releases {
		day, _ := time.Parse("2006-01-02", row.Date)
		if year := day.Year(); year > maxYear {
			maxYear = year
		}
	}
	if now.In(patchReleaseLocation).Year() > maxYear {
		calendarKey = now.In(patchReleaseLocation).Format("2006-01-02")
	}
	if calendarKey != "" && calendarKey != s.CalendarChecked {
		s.CalendarChecked = calendarKey
		if save() != nil {
			return now.Add(24 * time.Hour)
		}
		if rows, err := calendar(); err == nil && validPatchReleases(rows) {
			s.Releases = mergePatchReleases(s.Releases, rows)
			if save() != nil {
				return now.Add(24 * time.Hour)
			}
		}
		due, next = s.pending(now) // respect a postponement published by Riot
	}
	if due.Version != "" {
		s.LastAttemptVersion, s.AttemptedAt = due.Version, now
		// Catalog invalidation remains once per release. Notes have a separate
		// startup/retry loop and do not use this durable attempt as a success.
		if save() != nil {
			return now.Add(24 * time.Hour)
		}
		refresh(now)
	}
	return next
}

func (a *app) runPatchReleaseSchedule(ctx context.Context) {
	state := a.loadPatchReleaseState()
	provider := a.championDataProvider()
	provider.cache.setMayhemReleaseBoundary(state.AttemptedAt)
	for ctx.Err() == nil {
		next := state.step(time.Now(), func() ([]patchRelease, error) {
			requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			raw, err := fetchOfficialPatchBytes(requestCtx, a.externalHTTPClient(&http.Client{Timeout: 8 * time.Second}), officialPatchScheduleURL, 2<<20)
			if err != nil {
				return nil, err
			}
			return parseOfficialPatchSchedule(raw)
		}, func() error {
			err := a.savePatchReleaseState(state)
			if err != nil {
				a.recordDiagnostic(map[string]any{"event": "patch_release_schedule_save", "success": false})
			}
			return err
		}, func(at time.Time) { a.refreshPatchReleaseData(ctx, at) })
		timer := time.NewTimer(max(time.Until(next), time.Second))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (a *app) refreshPatchReleaseData(ctx context.Context, at time.Time) {
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	provider := a.championDataProvider()
	provider.cache.setMayhemReleaseBoundary(at)
	catalog, catalogErr := provider.loadAugments(requestCtx)
	_, rankingsErr := provider.loadARAMRankings(requestCtx)
	a.recordDiagnostic(map[string]any{"event": "patch_release_refresh", "catalog_success": catalogErr == nil && catalog.DataNotice == "", "rankings_success": rankingsErr == nil})
	a.broadcastEvent(`{"type":"mayhem-data-changed"}`)
}

func mayhemReleaseCacheKey(key string) bool {
	return strings.HasPrefix(key, "v2|opgg-rsc|aram-mayhem|") ||
		strings.HasPrefix(key, "hexdata-") || strings.HasPrefix(key, "bootstrap|") ||
		key == championCacheKey(opggPageHost, "/zh-cn/lol/modes/aram-mayhem", "", "text/html,application/xhtml+xml") ||
		key == championCacheKey(opggChampionHost, "/api/contents/tiers", "type=aram_mayhem", "application/json")
}

func (c *championDataCache) setMayhemReleaseBoundary(at time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if at.After(c.mayhemReleaseAfter) {
		c.mayhemReleaseAfter = at
	}
	c.mu.Unlock()
}

// Called with c.mu held. Old successes remain available as stale fallbacks.
func (c *championDataCache) freshForMayhemRelease(entry championCacheEnvelope) bool {
	return !mayhemReleaseCacheKey(entry.Key) || !entry.FetchedAt.Before(c.mayhemReleaseAfter)
}
