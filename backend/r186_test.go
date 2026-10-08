package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR186InstallDetectionFiveReasons(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-user", "app")
	for _, tc := range []struct {
		portable, uninstall bool
		entries             []updateRegistryInstallation
		result              string
		found, matches      bool
	}{
		{false, true, []updateRegistryInstallation{{"Deep Legends", root}}, "installed", true, true},
		{true, true, []updateRegistryInstallation{{"Deep Legends", root}}, "portable_env", true, true},
		{false, false, nil, "no_uninstaller", false, false},
		{false, true, []updateRegistryInstallation{{"Other", root}}, "registry_missing", false, false},
		{false, true, []updateRegistryInstallation{{"Deep Legends", root + "-old"}}, "registry_location_mismatch", true, false},
	} {
		d := classifyUpdateInstallation(root, tc.portable, tc.uninstall, tc.entries)
		if d.Result != tc.result || d.RegistryDisplayFound != tc.found || d.LocationMatches != tc.matches {
			t.Fatal(d)
		}
		data, _ := json.Marshal(d)
		if strings.Contains(string(data), root) || strings.Contains(string(data), "private-user") {
			t.Fatal("path leaked")
		}
		u := updateTestManager(t, []byte("setup"))
		u.installDetection = d
		seen := make(chan map[string]any, 8)
		u.diagnostic = func(e map[string]any) {
			if e["event"] == "update_install_detection" {
				seen <- e
			}
		}
		u.client.Transport = updateRoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(503, nil), nil })
		u.Start()
		event := <-seen
		if event["result"] != tc.result {
			t.Fatal(event)
		}
		u.Close()
	}
}
func TestR186PortableDownloadApplyAndFailures(t *testing.T) {
	data := []byte("setup")
	u := updateTestManager(t, data)
	u.status.Portable = true
	u.status.ManualOnly = true
	u.client.Transport = updateRoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(200, data), nil })
	u.mirrors = []string{""}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	u.mu.Lock()
	done := u.downloadDone
	u.mu.Unlock()
	<-done
	if u.Status().State != "ready" {
		t.Fatal(u.Status())
	}
	var launches int
	dest, _ := u.portableDirectory()
	u.launch = func(setup, path string) error {
		launches++
		if path != dest || setup != filepath.Join(u.directory, u.manifest.Asset.Name) {
			t.Fatal("wrong destination")
		}
		return nil
	}
	if err := u.Apply(); err != nil || launches != 1 {
		t.Fatal(err, launches)
	}
	command := portableUpdateCommandLine(`C:\Temp Space\setup.exe`, `C:\Users\test\AppData\Local\Programs\Deep Legends`, 42)
	if !strings.Contains(command, `--fresh-install`) || !strings.Contains(command, `--parent-pid 42`) || !strings.Contains(command, `--dest "C:\Users\test\AppData\Local\Programs\Deep Legends"`) {
		t.Fatal(command)
	}
	u.status.State = "ready"
	os.WriteFile(filepath.Join(u.directory, u.manifest.Asset.Name), []byte("wrong"), 0600)
	if u.Apply() == nil || launches != 1 || u.Status().State != "failed" {
		t.Fatal("tamper launched")
	}
	os.WriteFile(filepath.Join(u.directory, u.manifest.Asset.Name), data, 0600)
	u.status.State = "ready"
	u.launch = func(string, string) error { return errors.New("canceled") }
	var quit atomic.Int32
	if err := u.ApplyAsync(func() { quit.Add(1) }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for u.Status().State != "failed" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if u.Status().State != "failed" || quit.Load() != 0 {
		t.Fatal(u.Status(), quit.Load())
	}
	env := portableUpdateEnvironment([]string{"KEEP=1", "PORTABLE_EXECUTABLE_FILE=old.exe", "portable_executable_dir=old", "LOL_LOOT_DATA_DIR=old"})
	if !reflect.DeepEqual(env, []string{"KEEP=1"}) {
		t.Fatal(env)
	}
}
func TestR186DataMigrationNeverOverwritesAndKeepsSourceOnFailure(t *testing.T) {
	source := filepath.Join(t.TempDir(), "old")
	os.MkdirAll(filepath.Join(source, "lp"), 0700)
	os.WriteFile(filepath.Join(source, "settings.json"), []byte("settings"), 0600)
	os.WriteFile(filepath.Join(source, "lp", "record.json"), []byte("LP"), 0600)
	target := filepath.Join(t.TempDir(), "new")
	u := updateTestManager(t, nil)
	u.store = &localStore{root: source}
	u.migrationDirectory = func() (string, error) { return target, nil }
	var events []map[string]any
	u.diagnostic = func(e map[string]any) { events = append(events, e) }
	if err := u.preparePortableData(); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"settings.json", "lp/record.json"} {
		old, _ := os.ReadFile(filepath.Join(source, file))
		next, _ := os.ReadFile(filepath.Join(target, file))
		if !reflect.DeepEqual(old, next) {
			t.Fatal(file)
		}
	}
	os.WriteFile(filepath.Join(target, "settings.json"), []byte("existing"), 0600)
	if err := u.preparePortableData(); err != nil {
		t.Fatal(err)
	}
	kept, _ := os.ReadFile(filepath.Join(target, "settings.json"))
	if string(kept) != "existing" || events[len(events)-1]["result"] != "skipped_existing" {
		t.Fatal(events, string(kept))
	}
	target = filepath.Join(t.TempDir(), "broken")
	os.Symlink("missing", filepath.Join(source, "bad-link"))
	if u.preparePortableData() == nil || events[len(events)-1]["result"] != "failed" {
		t.Fatal(events)
	}
	old, _ := os.ReadFile(filepath.Join(source, "settings.json"))
	if string(old) != "settings" {
		t.Fatal("source removed")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("partial copy published")
	}
}

const r186A = `{"files":[{"name":"Game.cfg","sections":[{"name":"General","settings":[{"name":"SomeSetting","value":"1"},{"name":"Unchanged","value":"9"}]},{"name":"HUD","settings":[{"name":"Other","value":"0"}]}]},{"name":"Input.ini","sections":[{"name":"GameEvents","settings":[{"name":"evtAction","value":"PRIVATE_Q"}]}]}]}`
const r186B = `{"files":[{"name":"Game.cfg","sections":[{"name":"General","settings":[{"name":"SomeSetting","value":"2"},{"name":"Unchanged","value":"9"}]},{"name":"HUD","settings":[{"name":"Other","value":"1"}]}]},{"name":"Input.ini","sections":[{"name":"GameEvents","settings":[{"name":"evtAction","value":"PRIVATE_R"}]}]}]}`

func TestR186AllSettingChangesAndLCUABComparison(t *testing.T) {
	a, b := allSettingsFromJSON([]byte(r186A)), allSettingsFromJSON([]byte(r186B))
	keys, truncated := gameSettingsChangedKeys(a, b, settingsLocation{}, Summoner{})
	want := []string{"Game.cfg.General.SomeSetting: 1 → 2", "Game.cfg.HUD.Other: 0 → 1", "Input.ini.GameEvents.evtAction"}
	if !reflect.DeepEqual(keys, want) || truncated != 0 {
		t.Fatal(keys, truncated)
	}
	many := map[string]string{}
	for i := 0; i < 70; i++ {
		many["Game.cfg.General.Setting"+strings.Repeat("X", i)] = "new"
	}
	rows, n := gameSettingsChangedKeys(nil, many, settingsLocation{}, Summoner{})
	if len(rows) != 60 || n != 10 {
		t.Fatal(len(rows), n)
	}
	start := map[string]gameSettingsFileSnapshot{"one": {Located: true, AllValues: a}}
	end := map[string]gameSettingsFileSnapshot{"one": {Located: true, AllValues: b}}
	for _, tc := range []struct{ body, want string }{{`{"General":{"SomeSetting":1,"Unchanged":9},"HUD":{"Other":0}}`, "A"}, {`{"General":{"SomeSetting":2,"Unchanged":9},"HUD":{"Other":1}}`, "B"}, {`{"General":{"SomeSetting":4}}`, "neither"}} {
		if got := matchLCUSettingsFile([]byte(tc.body), start, end, nil); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
}

type r186SyncFixture struct {
	a            *app
	client       *LCUClient
	location     settingsLocation
	lcu          map[string]map[string]any
	calls        []string
	patches      []map[string]map[string]any
	verifyFailed bool
	save         bool
}

func newR186SyncFixture(t *testing.T) *r186SyncFixture {
	f := &r186SyncFixture{a: r175App(t), lcu: map[string]map[string]any{"General": {"SomeSetting": json.Number("1"), "Unchanged": json.Number("9")}, "HUD": {"Other": json.Number("0")}}}
	root := t.TempDir()
	config := filepath.Join(root, "Config")
	os.MkdirAll(config, 0700)
	f.location = settingsLocation{installRoot: root, configRoot: config, allowedRoot: root, file: filepath.Join(config, "PersistedSettings.json")}
	os.WriteFile(f.location.file, []byte(r186B), 0600)
	os.WriteFile(filepath.Join(config, "game.cfg"), []byte("[General]\nSomeSetting=2\n"), 0600)
	os.WriteFile(filepath.Join(config, "input.ini"), []byte("[GameEvents]\nevtAction=PRIVATE_R\n"), 0600)
	t.Cleanup(func() { os.Chmod(f.location.file, 0600) })
	f.client = &LCUClient{baseURL: "http://lcu.local", token: "test"}
	f.client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		var value any = f.lcu
		status := 200
		switch r.URL.Path {
		case "/lol-game-settings/v1/game-settings":
			if r.Method == http.MethodPatch {
				var patch map[string]map[string]any
				json.NewDecoder(r.Body).Decode(&patch)
				f.patches = append(f.patches, patch)
				if !f.verifyFailed {
					for section, fields := range patch {
						for name, v := range fields {
							f.lcu[section][name] = v
						}
					}
				}
				value = map[string]any{}
			}
		case "/swagger/v3/openapi.json":
			value = map[string]any{"paths": map[string]any{}}
			if f.save {
				value = map[string]any{"paths": map[string]any{"/lol-game-settings/v1/save": map[string]any{"post": map[string]any{}}}}
			}
		case "/lol-game-settings/v1/save":
			value = map[string]any{}
		case "/data-store/v1/install-dir":
			value = root
		case "/lol-game-settings/v1/input-settings":
			value = map[string]any{"evtAction": "PRIVATE_R"}
		default:
			status = 404
			value = map[string]any{}
		}
		data, _ := json.Marshal(value)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}
	return f
}

// Settlement remains a read-only diagnostic even when an old preference file
// still says enabled=true. Keep the A/B comparison and redaction coverage.
func TestSettlementSettingsWatchDoesNotSynchronize(t *testing.T) {
	f := newR186SyncFixture(t)
	writeLocalStoreFile(f.a.storage, "game-settings-sync.json", []byte(`{"enabled":true}`))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &f.a.gameSettingsWatch
	s.client, s.ctx, s.generation = f.client, ctx, 1
	s.queue = make(chan struct{}, 1)
	s.prev, s.seen = map[string]gameSettingsFileSnapshot{}, map[string]bool{}
	os.WriteFile(f.location.file, []byte(r186A), 0600)
	f.a.recordGameSettingsWatch(ctx, f.client, gameSettingsWatchJob{stage: "game_start", generation: 1})
	os.WriteFile(f.location.file, []byte(r186B), 0600)
	f.a.recordGameSettingsWatch(ctx, f.client, gameSettingsWatchJob{stage: "game_end", generation: 1})
	f.a.recordGameSettingsWatch(ctx, f.client, gameSettingsWatchJob{stage: "lobby_after_10s", generation: 1})
	f.a.recordGameSettingsWatch(ctx, f.client, gameSettingsWatchJob{stage: "lobby_after_60s", generation: 1})
	if len(f.patches) != 0 || len(s.jobs) != 0 {
		t.Fatal("settlement still writes/schedules sync", f.calls, s.jobs)
	}
	for _, call := range f.calls {
		if !strings.HasPrefix(call, "GET ") {
			t.Fatal("settlement write", call)
		}
	}
	watch := r175Events(t, f.a, "game_settings_watch")
	if watch[1]["lcu_matches_file"] != "A" {
		t.Fatal(watch[1])
	}
	changes, _ := json.Marshal(r175Events(t, f.a, "game_settings_changed"))
	if strings.Contains(string(changes), "PRIVATE_") || !strings.Contains(string(changes), "Input.ini.GameEvents.evtAction") {
		t.Fatal(string(changes))
	}
}

func TestR186MigrationPreflightStopsInstallerAndRejectsNestedTarget(t *testing.T) {
	u := updateTestManager(t, []byte("setup"))
	u.status.Portable = true
	u.status.State = "ready"
	os.WriteFile(filepath.Join(u.directory, u.manifest.Asset.Name), []byte("setup"), 0600)
	source := t.TempDir()
	os.Symlink("missing", filepath.Join(source, "bad"))
	u.store = &localStore{root: source}
	u.migrationDirectory = func() (string, error) { return filepath.Join(t.TempDir(), "new"), nil }
	launches := 0
	u.launch = func(string, string) error { launches++; return nil }
	if u.Apply() == nil || launches != 0 || u.Status().State != "failed" {
		t.Fatal("failed preflight installed", launches, u.Status())
	}
	if _, err := copyUpdateData(source, filepath.Join(source, "nested"), true); err == nil {
		t.Fatal("nested migration accepted")
	}
}
