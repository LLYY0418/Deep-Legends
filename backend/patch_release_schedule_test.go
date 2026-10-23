package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func releaseTestTime(value string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", value, patchReleaseLocation)
	if err != nil {
		panic(err)
	}
	return t
}

func TestPatchReleaseScheduleOfficialTableAndMidnight(t *testing.T) {
	raw, err := os.ReadFile("patchnotes/release-schedule-2026-source.html")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseOfficialPatchSchedule(raw)
	if err != nil {
		t.Fatal(err)
	}
	bundle := (&app{}).loadPatchReleaseState()
	if len(rows) != 24 || !reflect.DeepEqual(rows, bundle.Releases) {
		t.Fatal("bundled dates differ from the verified official table")
	}
	state := &patchReleaseState{Releases: rows, LastAttemptVersion: "26.21"}
	for _, tc := range []struct{ at, version, next string }{
		{"2026-11-04 23:59:59", "", "2026-11-05 00:00:00"},
		{"2026-11-05 00:00:00", "26.22", "2026-11-19 00:00:00"},
		{"2026-12-01 12:00:00", "26.23", "2026-12-10 00:00:00"},
	} {
		due, next := state.pending(releaseTestTime(tc.at))
		if due.Version != tc.version || !next.Equal(releaseTestTime(tc.next)) {
			t.Fatal(tc, due, next)
		}
	}
	// Wall-clock policy is Beijing even when the process/machine uses UTC.
	due, _ := state.pending(time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC))
	if due.Version != "26.22" {
		t.Fatal(due)
	}
	for _, bad := range [][]byte{[]byte("<html>blocked</html>"), []byte(strings.Replace(string(raw), "November 4, 2026", "February 30, 2026", 1)), []byte(strings.Replace(string(raw), "26.22", "26.21", 1))} {
		if _, err := parseOfficialPatchSchedule(bad); err == nil {
			t.Fatal("invalid/incomplete calendar accepted")
		}
	}
}

func TestPatchReleaseScheduleOnceAfterMissedTimeAndRestart(t *testing.T) {
	a := &app{storage: trackTestStore(t, &localStore{root: t.TempDir()})}
	state := a.loadPatchReleaseState()
	state.LastAttemptVersion = "26.20"
	var calendarCalls, refreshes atomic.Int64
	calendar := func() ([]patchRelease, error) { calendarCalls.Add(1); return nil, errors.New("offline") }
	refresh := func(at time.Time) {
		refreshes.Add(1)
		// The claim must already be durable even when network work fails.
		stored := a.loadPatchReleaseState()
		if stored.LastAttemptVersion != "26.21" || !stored.AttemptedAt.Equal(at) {
			t.Fatal(stored)
		}
	}
	now := releaseTestTime("2026-10-23 14:00:00")
	next := state.step(now, calendar, func() error { return a.savePatchReleaseState(state) }, refresh)
	if refreshes.Load() != 1 || calendarCalls.Load() != 1 || !next.Equal(releaseTestTime("2026-11-05 00:00:00")) {
		t.Fatal(refreshes.Load(), calendarCalls.Load(), next)
	}
	state = (&app{storage: a.storage}).loadPatchReleaseState()
	for _, offset := range []time.Duration{5 * time.Minute, time.Hour, 24 * time.Hour} {
		state.step(now.Add(offset), calendar, func() error { return a.savePatchReleaseState(state) }, refresh)
	}
	if refreshes.Load() != 1 || calendarCalls.Load() != 1 {
		t.Fatal("background retry/restart repeated networking", refreshes.Load(), calendarCalls.Load())
	}
	state.LastAttemptVersion = ""
	due, _ := state.pending(releaseTestTime("2026-12-25 12:00:00"))
	if due.Version != "26.24" {
		t.Fatal("must coalesce missed releases", due)
	}
}

func TestPatchReleaseScheduleRespectsPostponementAndNextYear(t *testing.T) {
	state := (&app{}).loadPatchReleaseState()
	state.LastAttemptVersion = "26.21"
	updated := append([]patchRelease(nil), state.Releases...)
	for i := range updated {
		if updated[i].Version == "26.22" {
			updated[i].Date = "2026-11-06"
		}
	}
	var calls atomic.Int64
	var refreshes atomic.Int64
	calendar := func() ([]patchRelease, error) { calls.Add(1); return updated, nil }
	next := state.step(releaseTestTime("2026-11-05 00:00:00"), calendar, func() error { return nil }, func(time.Time) { refreshes.Add(1) })
	if refreshes.Load() != 0 || calls.Load() != 1 || !next.Equal(releaseTestTime("2026-11-07 00:00:00")) {
		t.Fatal(refreshes.Load(), calls.Load(), next)
	}
	state.step(releaseTestTime("2026-11-07 00:00:00"), calendar, func() error { return nil }, func(time.Time) { refreshes.Add(1) })
	if refreshes.Load() != 1 {
		t.Fatal(refreshes.Load())
	}
	state.LastAttemptVersion = "26.24"
	calendar = func() ([]patchRelease, error) {
		calls.Add(1)
		return []patchRelease{{Version: "27.01", Date: "2027-01-07"}}, nil
	}
	next = state.step(releaseTestTime("2027-01-01 00:00:00"), calendar, func() error { return nil }, func(time.Time) { refreshes.Add(1) })
	if !next.Equal(releaseTestTime("2027-01-08 00:00:00")) || refreshes.Load() != 1 {
		t.Fatal(next, refreshes.Load())
	}
	state.step(releaseTestTime("2027-01-08 00:00:00"), calendar, func() error { return nil }, func(time.Time) { refreshes.Add(1) })
	if refreshes.Load() != 2 || state.LastAttemptVersion != "27.01" {
		t.Fatal(state, refreshes.Load())
	}
}

func TestPatchReleaseScheduleSaveFailureDoesNotRunNetworkRefresh(t *testing.T) {
	state := (&app{}).loadPatchReleaseState()
	state.CalendarChecked = "26.21@2026-10-21"
	state.step(releaseTestTime("2026-10-23 00:00:00"), func() ([]patchRelease, error) { t.Fatal("unexpected calendar request"); return nil, nil }, func() error { return errors.New("disk full") }, func(time.Time) { t.Fatal("refresh without durable claim") })
}

func TestMayhemReleaseBoundaryRefreshesMemoryAndDiskPreservingFallback(t *testing.T) {
	for _, disk := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "disk"}[disk], func(t *testing.T) {
			store := trackTestStore(t, &localStore{root: t.TempDir()})
			cache := newChampionDataCache(store)
			if err := os.MkdirAll(cache.dir, 0o700); err != nil {
				t.Fatal(err)
			}
			now := releaseTestTime("2026-11-04 23:59:00")
			cache.now = func() time.Time { return now }
			key := "v2|opgg-rsc|aram-mayhem|jinx"
			other := "v2|opgg-rsc|arena|jinx"
			for _, k := range []string{key, other} {
				_, err := cache.load(context.Background(), k, 6*time.Hour, 24*time.Hour, true, func(context.Context) ([]byte, error) { return []byte("old-success"), nil })
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := cache.flushDisk(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err := cache.readDisk(key); err != nil {
				t.Fatalf("test seed not persisted: %v; migration: %v", err, cache.migrationErr)
			}
			if disk {
				cache = newChampionDataCache(store)
				cache.now = func() time.Time { return now }
			}
			now = releaseTestTime("2026-11-05 00:00:00")
			cache.setMayhemReleaseBoundary(now)
			var calls atomic.Int64
			data, err := cache.load(context.Background(), key, 6*time.Hour, 24*time.Hour, true, func(context.Context) ([]byte, error) { calls.Add(1); return nil, errors.New("offline") })
			if err != nil || string(data) != "old-success" || calls.Load() != 1 {
				t.Fatal(string(data), err, calls.Load())
			}
			_, err = cache.load(context.Background(), other, 6*time.Hour, 24*time.Hour, true, func(context.Context) ([]byte, error) { t.Fatal("unrelated Arena cache expired"); return nil, nil })
			if err != nil {
				t.Fatal(err)
			}
			data, err = cache.load(context.Background(), key, 6*time.Hour, 24*time.Hour, true, func(context.Context) ([]byte, error) { calls.Add(1); return []byte("new-success"), nil })
			if err != nil || string(data) != "new-success" || calls.Load() != 2 {
				t.Fatal(string(data), err, calls.Load())
			}
			_, err = cache.load(context.Background(), key, 6*time.Hour, 24*time.Hour, true, func(context.Context) ([]byte, error) { t.Fatal("new version fetched again"); return nil, nil })
			if err != nil {
				t.Fatal(err)
			}
			if err := cache.flushDisk(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPatchReleaseScheduleBadStateKeepsBundledCalendar(t *testing.T) {
	a := &app{storage: trackTestStore(t, &localStore{root: t.TempDir()})}
	for _, raw := range [][]byte{[]byte("broken"), []byte(`{"schema":1,"releases":[{"version":"26.22","date":"2026-02-30"}]}`)} {
		if err := writeLocalStoreFile(a.storage, patchReleaseStateFile, raw); err != nil {
			t.Fatal(err)
		}
		if got := a.loadPatchReleaseState(); len(got.Releases) != 24 || got.LastAttemptVersion != "" {
			t.Fatal(got)
		}
	}
	raw, err := json.Marshal(a.loadPatchReleaseState())
	if err != nil || !strings.Contains(string(raw), "2026-11-04") {
		t.Fatal(err)
	}
}
