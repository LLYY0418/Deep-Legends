package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type r285GameLogFixture struct {
	Directory string
	R3D       string `json:"r3d"`
	Net       string
}

func r285GameLogFixtures(t *testing.T) []r285GameLogFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/r285-game-log-signatures.json")
	if err != nil {
		t.Fatal(err)
	}
	var result []r285GameLogFixture
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func r285WriteGameLogs(t *testing.T, root string, f r285GameLogFixture) {
	t.Helper()
	dir := filepath.Join(root, f.Directory)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for suffix, raw := range map[string]string{"_r3dlog.txt": f.R3D, "_netlog.txt": f.Net} {
		if err := os.WriteFile(filepath.Join(dir, f.Directory+suffix), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func r285FixtureSignal(t *testing.T, f r285GameLogFixture) watchReconnectSignal {
	t.Helper()
	root := t.TempDir()
	r285WriteGameLogs(t, root, f)
	at, err := time.ParseInLocation(reconnectGameDirectoryTime, f.Directory, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	s := readReconnectGameLogs([]string{root}, at.Add(-4*time.Second), at.Add(time.Hour))
	s.ProcessKnown, s.ProcessGone = true, true
	return s
}
func TestR285UnknownReconnectIsNotEvidence(t *testing.T) {
	if got := reconnectSignalOutcome(watchReconnectSignal{}); got != "canceled-unknown" {
		t.Fatalf("missing evidence allowed reconnect: %q", got)
	}
}
func TestR285RealGameLogReconnectDecisions(t *testing.T) {
	for _, sample := range r285GameLogFixtures(t) {
		t.Run(sample.Directory, func(t *testing.T) {
			s := r285FixtureSignal(t, sample)
			outcome, marker := "canceled-voluntary", "game-exit-early"
			if strings.Contains(sample.Directory, "13-49-36") {
				outcome, marker = "fired", "game-truncated"
			}
			if strings.Contains(sample.Directory, "14-14-35") {
				outcome, marker = "canceled-clean-exit", "game-completed"
			}
			f := r284Reconnect(t, s)
			f.start(t)
			f.now.Store(15000)
			f.release <- struct{}{}
			d := f.decision(t, outcome)
			if d["matched_marker"] != marker || d["game_log_found"] != true || d["process_gone"] != true || d["game_log_bytes"].(int) <= 0 {
				t.Fatal(d)
			}
			want := int32(0)
			if outcome == "fired" {
				want = 1
			}
			if f.posts.Load() != want {
				t.Fatalf("posts=%d", f.posts.Load())
			}
			for _, removed := range []string{"client_log_parsed", "exit_code", "exit_signal", "game_end_seen"} {
				if _, ok := d[removed]; ok {
					t.Fatalf("obsolete diagnostic %s", removed)
				}
			}
		})
	}
}
func TestR285UnknownAndRunningReconnect(t *testing.T) {
	f := r284Reconnect(t, watchReconnectSignal{ProcessKnown: true, ProcessGone: true})
	f.start(t)
	f.release <- struct{}{}
	f.decision(t, "canceled-unknown")
	if f.posts.Load() != 0 {
		t.Fatal("unknown fired")
	}
	running := r285CrashSignal()
	running.ProcessGone = false
	f = r284Reconnect(t, running)
	f.start(t)
	f.release <- struct{}{}
	select {
	case <-f.waiting:
	case <-time.After(time.Second):
		t.Fatal("running game was not awaited")
	}
	select {
	case d := <-f.decisions:
		t.Fatalf("classified a running game: %v", d)
	default:
	}
	f.signal.Store(r285CrashSignal())
	f.release <- struct{}{}
	f.decision(t, "fired")
	f.phase.Store("InProgress")
	f.runner.handlePhase(f.client, "InProgress")
	f.start(t)
	f.decision(t, "canceled-once-per-game")
	if f.posts.Load() != 1 {
		t.Fatal("repeated game attempt")
	}
}
func TestR285GameLogPathsAndTimeBounds(t *testing.T) {
	root := t.TempDir()
	cn := filepath.Join(root, "WeGameApps", "英雄联盟", "LeagueClient")
	riot := filepath.Join(root, "Riot Games", "League of Legends")
	for _, tc := range []struct{ dir, log string }{{cn, filepath.Join(root, "WeGameApps", "英雄联盟", "Game", "Logs", "GameLogs")}, {riot, filepath.Join(riot, "Game", "Logs", "GameLogs")}} {
		roots := reconnectGameLogRoots(processQueryResult{ImagePaths: []string{filepath.Join(tc.dir, "LeagueClient.exe")}, CommandLines: []string{`--install-directory="` + tc.dir + `"`}})
		found := false
		for _, r := range roots {
			found = found || r == tc.log
		}
		if !found {
			t.Fatal(roots, tc.log)
		}
	}
	samples := r285GameLogFixtures(t)
	r285WriteGameLogs(t, root, samples[0])
	r285WriteGameLogs(t, root, samples[2])
	at, _ := time.ParseInLocation(reconnectGameDirectoryTime, samples[2].Directory, time.Local)
	s := readReconnectGameLogs([]string{root}, at.Add(-4*time.Second), at.Add(time.Hour))
	if s.MatchedMarker != "game-truncated" {
		t.Fatal(s)
	}
	for _, anchor := range []time.Time{{}, at.Add(2 * time.Hour)} {
		if s = readReconnectGameLogs([]string{root}, anchor, at.Add(3*time.Hour)); s.GameLogFound {
			t.Fatal("old or unbound game reused", s)
		}
	}
	if s = readReconnectGameLogs([]string{root}, at.Add(-4*time.Second), at.Add(-time.Second)); s.GameLogFound {
		t.Fatal("future directory reused", s)
	}
}
func TestR285GameLogTailReadBudgetAndPartialFiles(t *testing.T) {
	sample := r285GameLogFixtures(t)[0]
	root := t.TempDir()
	sample.R3D = strings.Repeat(sample.R3D, 80)
	sample.Net = strings.Repeat(sample.Net, 500)
	r285WriteGameLogs(t, root, sample)
	at, _ := time.ParseInLocation(reconnectGameDirectoryTime, sample.Directory, time.Local)
	s := readReconnectGameLogs([]string{root}, at, at.Add(time.Hour))
	if s.GameLogBytes > 64<<10 || s.MatchedMarker != "game-exit-early" {
		t.Fatal(s)
	}
	// Readable net closing alone is enough; unreadable/missing r3d alone cannot
	// prove truncation. Reuse only the original real lines in these fixtures.
	os.Remove(filepath.Join(root, sample.Directory, sample.Directory+"_r3dlog.txt"))
	s = readReconnectGameLogs([]string{root}, at, at.Add(time.Hour))
	if s.MatchedMarker != "game-clean-shutdown" || s.GameLogFound {
		t.Fatal(s)
	}
	killed := r285GameLogFixtures(t)[2]
	root = t.TempDir()
	r285WriteGameLogs(t, root, killed)
	at, _ = time.ParseInLocation(reconnectGameDirectoryTime, killed.Directory, time.Local)
	os.Remove(filepath.Join(root, killed.Directory, killed.Directory+"_netlog.txt"))
	s = readReconnectGameLogs([]string{root}, at, at.Add(time.Hour))
	if s.MatchedMarker != "none" {
		t.Fatal("unreadable net was taken as absence of closing records", s)
	}
}
func TestR285GameProcessSnapshotIsNamesOnly(t *testing.T) {
	raw, err := os.ReadFile("process_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	start := strings.Index(src, "func nativeLeagueGameProcessRunning()")
	if start < 0 {
		t.Fatal("missing name snapshot")
	}
	body := src[start:]
	body = body[:strings.Index(body, "\n}")]
	if !strings.Contains(body, `nativeProcessSnapshot(false, "League of Legends.exe")`) || strings.Contains(body, "OpenProcess") || strings.Contains(body, "nativeProcessCommands(") {
		t.Fatal(body)
	}
}

func TestR285InProgressAnchorOnlyChangesOnEntry(t *testing.T) {
	f := r284Reconnect(t, watchReconnectSignal{})
	m := &watchReconnectMonitor{client: f.client, cancel: func() {}}
	f.runner.reconnectMonitor = m
	settings := f.runner.currentWatch()
	f.now.Store(1000)
	f.runner.observeReconnectPhase(f.client, "InProgress", settings)
	entered := m.inProgressAt
	if !entered.Equal(f.runner.clockNow()) {
		t.Fatal("entry not captured")
	}
	f.now.Store(2000)
	f.runner.observeReconnectPhase(f.client, "InProgress", settings)
	if !m.inProgressAt.Equal(entered) {
		t.Fatal("heartbeat moved entry")
	}
	f.runner.observeReconnectPhase(f.client, "Reconnect", settings)
	if !m.inProgressAt.Equal(entered) {
		t.Fatal("reconnect moved entry")
	}
	f.now.Store(3000)
	f.runner.observeReconnectPhase(f.client, "InProgress", settings)
	if !m.inProgressAt.Equal(f.runner.clockNow()) {
		t.Fatal("reentry not captured")
	}
	f.runner.observeReconnectPhase(f.client, "GameStart", settings)
	if !m.inProgressAt.IsZero() {
		t.Fatal("new process kept old entry")
	}
}
