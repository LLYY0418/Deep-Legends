package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

const r198JSON = "{\r\n  \"files\": [{\"name\":\"Game.cfg\", \"sections\":[{\"name\":\"General\",\"settings\":[{\"name\":\"CameraMode\", \"value\":\"2\"},{\"name\":\"CameraModeWASD\",\"value\":\"1\"},{\"name\":\"Other\",\"value\":\"2\"}]}]}]\r\n}\r\n"
const r198INI = "; untouched\r\n[General]\r\n  CameraMode = 2  ; camera\r\nCameraModeWASD=1\r\nOther=2\r\n[HUD]\r\nCameraMode=2\r\n"

type r198Fixture struct {
	f       *r186SyncFixture
	phase   atomic.Value
	running atomic.Bool
}

func newR198Fixture(t *testing.T) *r198Fixture {
	f := newR186SyncFixture(t)
	v := &r198Fixture{f: f}
	v.phase.Store("ChampSelect")
	f.lcu["General"]["CameraMode"] = json.Number("2")
	f.lcu["General"]["CameraModeWASD"] = json.Number("1")
	f.save = true
	os.WriteFile(f.location.file, []byte(r198JSON), 0600)
	os.WriteFile(filepath.Join(f.location.configRoot, "game.cfg"), []byte(r198INI), 0600)
	original := f.client.http.Transport
	f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/lol-gameflow/v1/gameflow-phase" {
			body, _ := json.Marshal(v.phase.Load())
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		}
		return original.RoundTrip(r)
	})
	f.a.lcu = f.client
	f.a.cameraProcessRunning = func() (bool, error) { return v.running.Load(), nil }
	mode := "free"
	if err := f.a.saveGameSettingsPreference(mode); err != nil {
		t.Fatal(err)
	}
	return v
}
func (v *r198Fixture) apply(stage string) {
	v.f.a.applyGameCameraMode(context.Background(), v.f.client, stage, nil)
}
func cameraEvent(t *testing.T, v *r198Fixture) map[string]any {
	rows := r175Events(t, v.f.a, "game_camera_mode_apply")
	if len(rows) == 0 {
		t.Fatal("missing event")
	}
	return rows[len(rows)-1]
}
func r198Read(t *testing.T, p string) string {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestR198CameraSingleFieldPatchAndSave(t *testing.T) {
	v := newR198Fixture(t)
	v.apply("champselect")
	if len(v.f.patches) != 1 {
		t.Fatalf("patches=%v", v.f.patches)
	}
	body, _ := json.Marshal(v.f.patches[0])
	if string(body) != `{"General":{"CameraMode":0}}` {
		t.Fatalf("whole settings patch: %s", body)
	}
	e := cameraEvent(t, v)
	if e["lcu_result"] != "ok" || e["save_called"] != true || e["lcu_before"] != "2" || e["lcu_after"] != "0" {
		t.Fatalf("event=%v", e)
	}
}
func TestR198CameraAlreadyTargetDoesNotPatch(t *testing.T) {
	v := newR198Fixture(t)
	v.f.lcu["General"]["CameraMode"] = json.Number("0")
	v.apply("champselect")
	if len(v.f.patches) != 0 || cameraEvent(t, v)["lcu_result"] != "unchanged" {
		t.Fatal("unchanged camera patched")
	}
}
func TestR198CameraFileBytesPreserved(t *testing.T) {
	v := newR198Fixture(t)
	v.apply("champselect")
	expected := strings.Replace(r198JSON, `"value":"2"`, `"value":"0"`, 1)
	if got := r198Read(t, v.f.location.file); got != expected {
		t.Fatalf("JSON bytes changed outside camera:\n%q", got)
	}
	if got := r198Read(t, filepath.Join(v.f.location.configRoot, "game.cfg")); got != strings.Replace(r198INI, "CameraMode = 2", "CameraMode = 0", 1) {
		t.Fatalf("INI bytes changed: %q", got)
	}
	if cameraEvent(t, v)["file_result"] != "ok" {
		t.Fatal(cameraEvent(t, v))
	}
}
func TestR198MalformedJSONIsRejected(t *testing.T) {
	for _, data := range []string{"", " ", "{", "[", `{"files":[`, `{"General":{"CameraMode":2}`} {
		if _, _, err := replaceCameraModeBytes([]byte(data), true, 0); err == nil {
			t.Fatalf("accepted malformed JSON: %q", data)
		}
	}
}
func TestR198ReadOnlyStillPatchesLCU(t *testing.T) {
	v := newR198Fixture(t)
	os.Chmod(v.f.location.file, 0444)
	v.apply("champselect")
	if r198Read(t, v.f.location.file) != strings.Replace(r198JSON, `"value":"2"`, `"value":"0"`, 1) || len(v.f.patches) != 1 || cameraEvent(t, v)["file_result"] != "ok_relocked" || cameraEvent(t, v)["relocked"] != true {
		t.Fatalf("read-only result=%v", cameraEvent(t, v))
	}
}
func TestR198NoWritesDuringGame(t *testing.T) {
	for _, phase := range []string{"InProgress", "Reconnect"} {
		t.Run(phase, func(t *testing.T) {
			v := newR198Fixture(t)
			v.phase.Store(phase)
			for _, stage := range []string{"champselect", "game_start", "settings_changed"} {
				v.apply(stage)
			}
			if len(v.f.patches) != 0 || r198Read(t, v.f.location.file) != r198JSON || r198Read(t, filepath.Join(v.f.location.configRoot, "game.cfg")) != r198INI {
				t.Fatal("in-game write")
			}
			if len(v.f.calls) != 0 {
				t.Fatalf("in-game settings read/write=%v", v.f.calls)
			}
		})
	}
}
func TestR198NoneDoesNotReadOrWrite(t *testing.T) {
	v := newR198Fixture(t)
	mode := "none"
	v.f.a.saveGameSettingsPreference(mode)
	v.apply("champselect")
	if len(v.f.calls) != 0 || r198Read(t, v.f.location.file) != r198JSON {
		t.Fatal("none touched settings")
	}
}
func TestR198WASDNeverChanges(t *testing.T) {
	for mode := range gameCameraModeValues {
		t.Run(mode, func(t *testing.T) {
			v := newR198Fixture(t)
			v.f.a.saveGameSettingsPreference(mode)
			v.apply("champselect")
			if !strings.Contains(r198Read(t, v.f.location.file), `"name":"CameraModeWASD","value":"1"`) || !strings.Contains(r198Read(t, filepath.Join(v.f.location.configRoot, "game.cfg")), "CameraModeWASD=1") {
				t.Fatal("WASD camera modified")
			}
			if !settingMatches("1", v.f.lcu["General"]["CameraModeWASD"]) {
				t.Fatal("LCU WASD camera modified")
			}
		})
	}
}
func TestR198UnsafeFilePathsRejected(t *testing.T) {
	v := newR198Fixture(t)
	outside := filepath.Join(t.TempDir(), "PersistedSettings.json")
	os.WriteFile(outside, []byte(r198JSON), 0600)
	for _, mode := range []string{"outside", "symlink", "directory-link"} {
		t.Run(mode, func(t *testing.T) {
			location := v.f.location
			switch mode {
			case "outside":
				location.file = outside
			case "symlink":
				location.file = filepath.Join(location.configRoot, "linked.json")
				if err := os.Symlink(outside, location.file); err != nil {
					t.Fatal(err)
				}
			case "directory-link":
				dir := filepath.Join(location.installRoot, "linked")
				if err := os.Symlink(filepath.Dir(outside), dir); err != nil {
					t.Fatal(err)
				}
				location.file = filepath.Join(dir, "PersistedSettings.json")
			}
			_, _, result := applyCameraModeFileWithPermissions(location, 0, func() bool { return true }, cameraFilePermissions)
			if result != "write_failed" || r198Read(t, outside) != r198JSON {
				t.Fatalf("unsafe result=%s", result)
			}
		})
	}
}
func TestR198LobbyOnlyLCUAndPreferencesRetained(t *testing.T) {
	v := newR198Fixture(t)
	v.phase.Store("Lobby")
	v.apply("settings_changed")
	if len(v.f.patches) != 1 || r198Read(t, v.f.location.file) != r198JSON {
		t.Fatal("lobby file write or missing LCU patch")
	}
	writeLocalStoreFile(v.f.a.storage, "game-settings-sync.json", []byte(`{"enabled":true,"cameraMode":"free"}`))
	if v.f.a.cameraModePreference() != "free" {
		t.Fatal("preference overwritten")
	}
	fresh := &app{storage: v.f.a.storage}
	if fresh.cameraModePreference() != "free" {
		t.Fatal("preference not restored")
	}
	w := httptest.NewRecorder()
	fresh.handleCameraModePreference(w, httptest.NewRequest("POST", "/api/rig/camera-mode", strings.NewReader(`{"mode":"dynamic"}`)))
	if w.Code != 200 || fresh.cameraModePreference() != "dynamic" {
		t.Fatal("save failed")
	}
	data, _ := readLocalStoreFile(fresh.storage, "game-settings-sync.json")
	if strings.Contains(string(data), "enabled") {
		t.Fatal("legacy sync preference retained", string(data))
	}
}
func TestR198GameStartRunningProcessSkipsFilesAndVerifyFailure(t *testing.T) {
	v := newR198Fixture(t)
	v.phase.Store("GameStart")
	v.running.Store(true)
	v.apply("game_start")
	if len(v.f.patches) != 1 || r198Read(t, v.f.location.file) != r198JSON || cameraEvent(t, v)["file_skipped"] != "game_running" {
		t.Fatal(cameraEvent(t, v))
	}
	v = newR198Fixture(t)
	v.f.verifyFailed = true
	v.apply("champselect")
	if len(v.f.patches) != 1 || cameraEvent(t, v)["lcu_result"] != "verify_failed" {
		t.Fatal("verification failure not recorded")
	}
	for _, call := range v.f.calls {
		if call == "POST /lol-game-settings/v1/save" {
			t.Fatal("saved unverified settings")
		}
	}
}
func TestR198JSONShapesAndAmbiguousFields(t *testing.T) {
	for _, raw := range []string{`{"Game.cfg":{"General":{"CameraMode":2,"CameraModeWASD":1}}}`, `{"General":{"CameraMode":"2","CameraModeWASD":1}}`} {
		next, _, err := replaceCameraModeBytes([]byte(raw), true, 0)
		if err != nil || !strings.Contains(string(next), `"CameraModeWASD":1`) {
			t.Fatalf("shape %s err=%v", raw, err)
		}
	}
	for _, raw := range []string{`{"General":{"CameraMode":2,"CameraMode":1}}`, `{"files":[{"name":"Input.ini","sections":[{"name":"General","settings":[{"name":"CameraMode","value":"2"}]}]}]}`} {
		if _, _, err := replaceCameraModeBytes([]byte(raw), true, 0); err == nil {
			t.Fatalf("unsafe shape=%s", raw)
		}
	}
}
func TestR198InGameSnapshotReportsActualFiles(t *testing.T) {
	v := newR198Fixture(t)
	v.apply("champselect")
	s := &v.f.a.gameSettingsWatch
	s.client = v.f.client
	s.ctx = context.Background()
	s.generation = 1
	s.prev = map[string]gameSettingsFileSnapshot{}
	v.phase.Store("InProgress")
	v.f.a.recordGameSettingsWatch(context.Background(), v.f.client, gameSettingsWatchJob{stage: "in_game_60s", generation: 1})
	events := r175Events(t, v.f.a, "game_settings_watch")
	if events[len(events)-1]["camera_mode_matches_target"] != true {
		t.Fatal(events)
	}
	os.WriteFile(v.f.location.file, []byte(r198JSON), 0600)
	v.f.a.recordGameSettingsWatch(context.Background(), v.f.client, gameSettingsWatchJob{stage: "in_game_60s", generation: 1})
	events = r175Events(t, v.f.a, "game_settings_watch")
	if events[len(events)-1]["camera_mode_matches_target"] != false {
		t.Fatal("LCU target masked PersistedSettings reset")
	}
	if !reflect.DeepEqual(v.f.patches[0], map[string]map[string]any{"General": {"CameraMode": float64(0)}}) {
		t.Fatal(v.f.patches)
	}
}

func TestR198FinalRecheckAfterStageProbe(t *testing.T) {
	for _, change := range []string{"read-only", "changed", "symlink"} {
		t.Run(change, func(t *testing.T) {
			v := newR198Fixture(t)
			outside := filepath.Join(t.TempDir(), "outside.json")
			os.WriteFile(outside, []byte(r198JSON), 0600)
			_, _, result := applyCameraModeFileWithPermissions(v.f.location, 0, func() bool {
				switch change {
				case "read-only":
					os.Chmod(v.f.location.file, 0444)
				case "changed":
					os.WriteFile(v.f.location.file, []byte(strings.Replace(r198JSON, "Other", "Different", 1)), 0600)
				case "symlink":
					os.Remove(v.f.location.file)
					if err := os.Symlink(outside, v.f.location.file); err != nil {
						t.Fatal(err)
					}
				}
				return true
			}, cameraFilePermissions)
			if result != "write_failed" {
				t.Fatal("final unsafe replacement", result)
			}
			if change == "changed" && !strings.Contains(r198Read(t, v.f.location.file), "Different") {
				t.Fatal("concurrent change overwritten")
			}
			if r198Read(t, outside) != r198JSON {
				t.Fatal("outside target changed")
			}
		})
	}
}
