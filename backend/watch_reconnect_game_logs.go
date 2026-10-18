package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const reconnectGameLogReadLimit = 64 << 10
const reconnectGameDirectoryTime = "2006-01-02T15-04-05"

func reconnectGameLogRoots(query processQueryResult) []string {
	roots := []string{}
	seen := map[string]bool{}
	add := func(root string) {
		root = filepath.Clean(root)
		key := strings.ToLower(root)
		if !seen[key] {
			seen[key] = true
			roots = append(roots, root)
		}
	}
	// Only known client locations and the existing fixed lockfile candidates.
	// Tencent places LeagueClient beside Game; root-level Riot installs place
	// the executable/lockfile in the installation directory.
	for _, lock := range targetedLockfileCandidates(query) {
		dir := filepath.Dir(lock)
		if strings.EqualFold(filepath.Base(dir), "LeagueClient") {
			add(filepath.Join(dir, "..", "Game", "Logs", "GameLogs"))
		} else {
			add(filepath.Join(dir, "Game", "Logs", "GameLogs"))
			add(filepath.Join(dir, "Logs", "GameLogs"))
		}
	}
	if len(roots) == 0 {
		for _, lock := range lockfileCandidates(nil) {
			if info, err := os.Lstat(lock); err == nil && info.Mode().IsRegular() {
				dir := filepath.Dir(lock)
				if strings.EqualFold(filepath.Base(dir), "LeagueClient") {
					add(filepath.Join(dir, "..", "Game", "Logs", "GameLogs"))
				} else {
					add(filepath.Join(dir, "Game", "Logs", "GameLogs"))
					add(filepath.Join(dir, "Logs", "GameLogs"))
				}
			}
		}
	}
	return roots
}

// The total read budget is 64 KiB: at most half from each file's tail.
func readReconnectGameLogTail(path string) (string, bool, bool, int) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false, false, 0
	}
	file, err := os.Open(path)
	if err != nil {
		return "", true, false, 0
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return "", true, false, 0
	}
	start := max(int64(0), info.Size()-reconnectGameLogReadLimit/2)
	raw := make([]byte, info.Size()-start)
	n, err := file.ReadAt(raw, start)
	if err != nil && err != io.EOF {
		return "", true, false, 0
	}
	// Do not match the suffix of a line cut at the beginning of the tail.
	text := string(raw[:n])
	if start > 0 {
		if first := strings.IndexByte(text, '\n'); first >= 0 {
			text = text[first+1:]
		} else {
			text = ""
		}
	}
	return text, true, true, n
}

// Markers are literal messages in the six user-supplied R285 game logs.
// Quit Game Confirmed deliberately is not a marker: the kill sample has it too.
func reconnectGameLogMarker(r3d, net string) string {
	early, completed, clean := false, false, false
	for _, line := range strings.Split(r3d, "\n") {
		if strings.Contains(line, "LCU Game End Status:") {
			clean = true
			early = early || strings.Contains(line, "GAMEFLOW_EVENT.GAME_EXIT_EARLY")
			completed = completed || strings.Contains(line, "GAMEFLOW_EVENT.GAME_COMPLETED")
		}
		if strings.Contains(line, "Exiting Application") {
			clean = true
		}
	}
	if early {
		return "game-exit-early"
	}
	if completed {
		return "game-completed"
	}
	if clean || strings.Contains(net, "LogOut_File: : logfile stopped") {
		return "game-clean-shutdown"
	}
	return "none"
}

func readReconnectGameLogs(roots []string, entered, now time.Time) watchReconnectSignal {
	signal := watchReconnectSignal{}
	if entered.IsZero() {
		return signal
	}
	newest, path := "", ""
	for _, root := range roots {
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := entry.Name()
			at, err := time.ParseInLocation(reconnectGameDirectoryTime, name, time.Local)
			if err != nil || at.Format(reconnectGameDirectoryTime) != name || at.Before(entered.Add(-60*time.Second)) || at.After(now) {
				continue
			}
			if name > newest {
				newest, path = name, filepath.Join(root, name)
			}
		}
	}
	if path == "" {
		return signal
	}
	r3d, found, ok, r3dBytes := readReconnectGameLogTail(filepath.Join(path, newest+"_r3dlog.txt"))
	net, _, netOK, netBytes := readReconnectGameLogTail(filepath.Join(path, newest+"_netlog.txt"))
	signal.GameLogFound = found
	signal.GameLogBytes = r3dBytes + netBytes
	signal.MatchedMarker = reconnectGameLogMarker(r3d, net)
	// A readable closing record is sufficient to stop. An unreadable file is
	// not evidence that closing records are absent.
	if signal.MatchedMarker == "none" && found && ok && netOK {
		signal.MatchedMarker = "game-truncated"
	}
	return signal
}
