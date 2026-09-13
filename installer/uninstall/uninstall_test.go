package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	uiassets "deeplegends/installer/ui"
)

func TestUninstallProgressDeletionPrecedesProcessExit(t *testing.T) {
	m := uninstallProgressModel{baselineBytes: 1000, baselineFiles: 10}
	first := m.update(directorySnapshot{Bytes: 1000, Files: 10}, 0)
	stopping := m.update(directorySnapshot{Bytes: 1000, Files: 10}, 3*time.Second)
	if first.Stage != stageStopping || stopping.Stage != stageStopping || stopping.Percent <= first.Percent {
		t.Fatalf("stopping stage needs a bounded time floor: %+v, %+v", first, stopping)
	}
	deleting := m.update(directorySnapshot{Bytes: 500, Files: 5}, 4*time.Second)
	if deleting.Stage != stageDeleting || deleting.Percent <= stopping.Percent {
		t.Fatalf("missing measured deletion progress: %+v", deleting)
	}
	// Directory disappears at 5s, while NSIS is still cleaning registry until 7s.
	for _, elapsed := range []time.Duration{5 * time.Second, 6 * time.Second, 7 * time.Second} {
		update := m.update(directorySnapshot{Missing: true}, elapsed)
		if update.Stage != stageCleaning || update.Percent != 97 {
			t.Fatalf("must show deletion complete before Core exits: %+v", update)
		}
	}
	if update := m.update(directorySnapshot{Bytes: 9000, Files: 100}, time.Hour); update.Percent < 97 || update.Percent >= 100 {
		t.Fatalf("progress cannot regress or claim success before exit: %+v", update)
	}
}

func TestUninstallProgressEmptyFilesAndZeroBaseline(t *testing.T) {
	m := uninstallProgressModel{baselineFiles: 10}
	if update := m.update(directorySnapshot{Files: 2}, time.Second); update.Stage != stageDeleting || update.Percent < 70 {
		t.Fatalf("zero-byte files must count: %+v", update)
	}
	zero := uninstallProgressModel{}
	if update := zero.update(directorySnapshot{}, time.Hour); update.Percent > 10 || update.Stage != stageStopping {
		t.Fatalf("empty but existing directory is not proof of success: %+v", update)
	}
}

func TestSnapshotsMeasureActualRemovalAndSkipLinks(t *testing.T) {
	root := t.TempDir()
	installation := filepath.Join(root, "Deep Legends")
	if err := os.Mkdir(installation, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installation, "file"), []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installation, "empty"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(installation, "outside")); err != nil {
		t.Fatal(err)
	}
	if got := snapshotDirectory(installation); got.Bytes != 5 || got.Files != 2 || got.Missing {
		t.Fatal(got)
	}
	if err := os.RemoveAll(installation); err != nil {
		t.Fatal(err)
	}
	if got := snapshotDirectory(installation); !got.Missing {
		t.Fatal(got)
	}
}

func TestUninstallDataDeletionAndCompletion(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		code                     int
		deleteData, cleanupError bool
	}{
		{"retain", 0, false, false}, {"delete", 0, true, false},
		{"cache locked", 0, true, true}, {"core failed retain", 7, false, false}, {"core failed delete", 7, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cache := filepath.Join(root, "LOLLootAssistant")
			os.Mkdir(cache, 0700)
			os.WriteFile(filepath.Join(cache, "saved-data"), []byte("preserve unless opted in"), 0600)
			var calls []string
			var events []string
			ok := finishUninstall(tc.code, tc.deleteData, root, func(directory string) error {
				calls = append(calls, directory)
				if tc.cleanupError {
					return errors.New("locked")
				}
				return os.RemoveAll(directory)
			}, func(method string, value any) {
				events = append(events, method)
				if method == "progress" && value.(progressMessage).Stage != stageCacheWarning {
					t.Fatal(value)
				}
			})
			wantDelete := tc.code == 0 && tc.deleteData
			if wantDelete {
				if !reflect.DeepEqual(calls, []string{cache}) {
					t.Fatalf("cache cleanup calls: %v", calls)
				}
			} else if len(calls) != 0 {
				t.Fatalf("unauthorized or failed uninstall deleted cache: %v", calls)
			}
			_, err := os.Stat(cache)
			if os.IsNotExist(err) != (wantDelete && !tc.cleanupError) {
				t.Fatalf("wrong on-disk cache outcome: %v", err)
			}
			if ok != (tc.code == 0) {
				t.Fatal("wrong success result")
			}
			want := []string{"done"}
			if tc.code != 0 {
				want = []string{"failed"}
			} else if tc.cleanupError {
				want = []string{"progress", "done"}
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("events %v want %v", events, want)
			}
		})
	}
}

func TestSilentDispatchNeverCreatesWindow(t *testing.T) {
	for _, args := range []string{`/S`, `/currentuser /S`, `/allusers /S --updated`, `/S /KEEP_APP_DATA --keep-shortcuts _?=D:\游戏 文件\Deep Legends`, `"/S"`, `/s`} {
		options, err := parseOptions(args)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		code := dispatchMode(options, func() int { calls++; return 7 }, func() int { t.Fatalf("window created for %q", args); return 0 })
		if calls != 1 || code != 7 {
			t.Fatalf("wrong silent dispatch for %q", args)
		}
	}
	for _, args := range []string{"", "/currentuser", `/something /SILENT`, `--log="hello /S world"`, `_?=D:\Games /S folder`} {
		options, err := parseOptions(args)
		if err != nil {
			t.Fatal(err)
		}
		if options.silent() {
			t.Fatalf("non-flag mistaken for /S: %q", args)
		}
	}
}

func TestCoreCommandPreservesUpgradeFlagsAndNSISTail(t *testing.T) {
	options, err := parseOptions(`/S /KEEP_APP_DATA /currentuser --keep-shortcuts --updated --custom="a b" _?=D:\游戏 Folder\Deep Legends`)
	if err != nil {
		t.Fatal(err)
	}
	if options.InstallDir != `D:\游戏 Folder\Deep Legends` {
		t.Fatal(options)
	}
	core := `C:\Temp\Uninstall Deep Legends Core.exe`
	got := options.coreCommandLine(core, options.InstallDir, true, false)
	want := `"C:\Temp\Uninstall Deep Legends Core.exe" /KEEP_APP_DATA /currentuser --keep-shortcuts --updated --custom="a b" /S _?=D:\游戏 Folder\Deep Legends`
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	options.Arguments = `/allusers --delete-app-data /S`
	if got := options.coreCommandLine(core, `D:\Deep Legends\`, true, false); strings.Contains(got, "--delete-app-data") || strings.HasSuffix(got, `\`) {
		t.Fatal(got)
	}
	if got := options.coreCommandLine(core, options.InstallDir, true, true); strings.Count(got, "--delete-app-data") != 1 {
		t.Fatal(got)
	}
	if got := options.coreCommandLine(core, options.InstallDir, false, false); strings.Contains(got, "/S") {
		t.Fatal("fallback must show original NSIS")
	}
	for _, args := range []string{`/S _?=`, `/S _?="D:\Deep Legends"`} {
		if _, err := parseOptions(args); err == nil {
			t.Fatal("malformed tail accepted")
		}
	}
}

func TestUninstallDesignAndState(t *testing.T) {
	html, err := uiassets.RenderUninstaller()
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"__LOGO__", "__INIT__"} {
		if strings.Contains(html, token) {
			t.Fatal(token)
		}
	}
	for _, stage := range []string{stageStopping, stageDeleting, stageCleaning} {
		if !strings.Contains(html, `data-match="`+stage+`"`) {
			t.Fatal(stage)
		}
	}
	var state uninstallState
	if state.busy() || !state.begin() || !state.busy() || state.begin() {
		t.Fatal("duplicate operation or close allowed")
	}
	state.running = false
	if !state.begin() {
		t.Fatal("retry blocked")
	}
}
