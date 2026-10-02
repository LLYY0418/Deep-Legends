package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// This watcher never writes game files or LCU settings. State and fingerprints
// are connection-local; snapshots are serialized to preserve comparison order.
type gameSettingsFileSnapshot struct {
	PathKind string                       `json:"path_kind"`
	Exists   bool                         `json:"exists"`
	Located  bool                         `json:"is_located_target"`
	ReadOnly bool                         `json:"read_only"`
	Size     int64                        `json:"size"`
	Mtime    string                       `json:"mtime_utc"`
	Hash     string                       `json:"content_hash8"`
	Values   map[string]string            `json:"camera_values"`
	Changed  bool                         `json:"changed_since_prev"`
	Changes  map[string]map[string]string `json:"camera_changed_keys"`
	Result   string                       `json:"result"`
	stage    string
}
type gameSettingsWatchJob struct {
	stage              string
	generation         uint64
	location           *settingsLocation
	delayed            bool
	lockReadOnlyBefore *bool
}
type gameSettingsWatchState struct {
	mu              sync.Mutex
	client          *LCUClient
	standalone      bool
	ctx             context.Context
	cancel          context.CancelFunc
	queue           chan struct{}
	jobs            []gameSettingsWatchJob
	generation      uint64
	seen            map[string]bool
	prev            map[string]gameSettingsFileSnapshot
	end             map[string]gameSettingsFileSnapshot
	endChanged      map[string]bool
	phase           string
	started         bool
	ended           bool
	inGameScheduled bool
	// Tests replace the cancellable clock, never the production read pipeline.
	wait func(context.Context, time.Duration) bool
}

func (s *gameSettingsWatchState) enqueueLocked(job gameSettingsWatchJob) {
	s.jobs = append(s.jobs, job)
	select {
	case s.queue <- struct{}{}:
	default:
	}
}
func gameSettingsWait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func (a *app) beginGameSettingsWatch(parent context.Context, client *LCUClient) {
	a.startGameSettingsWatch(parent, client, false)
}
func (a *app) startGameSettingsWatch(parent context.Context, client *LCUClient, standalone bool) {
	if a.storage == nil || client == nil {
		return
	}
	s := &a.gameSettingsWatch
	s.mu.Lock()
	if s.client == client && s.ctx != nil && s.ctx.Err() == nil && (!s.standalone || standalone) {
		s.mu.Unlock()
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx, s.cancel = context.WithCancel(parent)
	s.client = client
	s.standalone = standalone
	s.generation++
	s.queue = make(chan struct{}, 1)
	s.jobs = nil
	s.seen = map[string]bool{"app_start": true}
	s.prev = make(map[string]gameSettingsFileSnapshot)
	s.end = nil
	s.endChanged = nil
	s.phase = ""
	s.started = false
	s.ended = false
	s.inGameScheduled = false
	ctx, queue := s.ctx, s.queue
	s.enqueueLocked(gameSettingsWatchJob{stage: "app_start", generation: s.generation})
	s.mu.Unlock()
	a.goSafe("game-settings-watch", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-queue:
				for {
					s.mu.Lock()
					if len(s.jobs) == 0 || s.ctx != ctx {
						s.mu.Unlock()
						break
					}
					job := s.jobs[0]
					s.jobs = s.jobs[1:]
					s.mu.Unlock()
					a.recordGameSettingsWatch(ctx, client, job)
				}
			}
		}
	})
}
func (a *app) observeGameSettingsPhase(client *LCUClient, phase string) {
	s := &a.gameSettingsWatch
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != client || s.ctx == nil || s.ctx.Err() != nil {
		return
	}
	// Keep previous file snapshots across games so pre-launch rewrites remain visible.
	if phase == "ChampSelect" && s.phase != "ChampSelect" && (s.started || s.ended) || (phase == "GameStart" || phase == "InProgress") && s.ended {
		s.generation++
		s.seen = make(map[string]bool)
		s.end = nil
		s.endChanged = nil
		s.started = false
		s.ended = false
		s.inGameScheduled = false
	}
	s.phase = phase
	enqueue := func(stage string) {
		if s.seen[stage] {
			return
		}
		s.seen[stage] = true
		s.enqueueLocked(gameSettingsWatchJob{stage: stage, generation: s.generation})
	}
	later := func(stage string, delay time.Duration) {
		ctx, generation, wait := s.ctx, s.generation, s.wait
		if wait == nil {
			wait = gameSettingsWait
		}
		a.goSafe("game-settings-watch-delay", func() {
			if !wait(ctx, delay) {
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if ctx.Err() != nil || s.generation != generation || s.client != client || s.seen[stage] || stage == "in_game_60s" && s.ended {
				return
			}
			s.seen[stage] = true
			s.enqueueLocked(gameSettingsWatchJob{stage: stage, generation: generation, delayed: true})
		})
	}
	switch phase {
	case "ChampSelect":
		enqueue("champselect")
	case "GameStart", "InProgress":
		s.started = true
		enqueue("game_start")
		if phase == "InProgress" && !s.inGameScheduled {
			s.inGameScheduled = true
			later("in_game_60s", 60*time.Second)
		}
	case "WaitingForStats", "PreEndOfGame", "EndOfGame":
		if !s.ended {
			s.ended = true
			enqueue("game_end")
			later("lobby_after_10s", 10*time.Second)
			later("lobby_after_60s", 60*time.Second)
		}
	}
}
func (a *app) queueSettingsLockSnapshot(client *LCUClient, location settingsLocation, before bool) {
	if a.storage == nil {
		return
	}
	s := &a.gameSettingsWatch
	s.mu.Lock()
	if s.client == client && s.ctx != nil && s.ctx.Err() == nil {
		// Every explicit action is a distinct observation, unlike repeated phase events.
		s.enqueueLocked(gameSettingsWatchJob{stage: "lock_action", generation: s.generation, location: &location, lockReadOnlyBefore: &before})
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	// A lock action may precede the event stream. It still produces a read-only
	// snapshot, detached from the HTTP request's short lifetime.
	a.startGameSettingsWatch(context.Background(), client, true)
	s.mu.Lock()
	s.enqueueLocked(gameSettingsWatchJob{stage: "lock_action", generation: s.generation, location: &location, lockReadOnlyBefore: &before})
	s.mu.Unlock()
}
func settingsWatchPathKind(location settingsLocation, file string) string {
	rel, err := filepath.Rel(location.installRoot, file)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	for _, dir := range []string{"Config/", "../Game/Config/", "Game/Config/"} {
		for _, name := range []string{"PersistedSettings.json", "game.cfg", "input.ini"} {
			if rel == dir+name {
				return rel
			}
		}
	}
	return ""
}
func settingsWatchNoSymlinks(location settingsLocation, file string) bool {
	if !filepath.IsAbs(location.allowedRoot) || !pathWithin(location.allowedRoot, file) {
		return false
	}
	for path := file; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return false
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if path == location.allowedRoot {
			return true
		}
		if filepath.Dir(path) == path {
			return false
		}
	}
}
func gameSettingsCandidates(location settingsLocation) []string {
	files := []string{location.file}
	for _, dir := range []string{filepath.Join(location.installRoot, "Config"), filepath.Join(location.installRoot, "..", "Game", "Config"), filepath.Join(location.installRoot, "Game", "Config")} {
		for _, name := range []string{"PersistedSettings.json", "game.cfg", "input.ini"} {
			files = append(files, filepath.Join(dir, name))
		}
	}
	// Include neighbours of the located target even if layout rules evolve.
	files = append(files, filepath.Join(location.configRoot, "game.cfg"), filepath.Join(location.configRoot, "input.ini"))
	seen := map[string]bool{}
	out := []string{}
	for _, file := range files {
		file = filepath.Clean(file)
		if !seen[file] {
			seen[file] = true
			out = append(out, file)
		}
	}
	return out
}
func cameraSettingName(name string, input bool) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "camera") || !input && strings.Contains(name, "lock")
}
func cameraDiagnosticValue(value any) (string, bool) {
	// Only primitive setting values. Container contents, paths and credentials
	// must not leak through a coincidentally named Camera/Lock parent.
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case json.Number:
		text = v.String()
	case bool:
		if v {
			text = "true"
		} else {
			text = "false"
		}
	default:
		return "", false
	}
	if strings.ContainsAny(text, "/\\\r\n") || strings.Contains(text, "://") {
		return "[redacted]", true
	}
	runes := []rune(text)
	if len(runes) > 32 {
		text = string(runes[:32])
	}
	return text, utf8.ValidString(text)
}
func cameraSafeKey(key string) bool {
	lower := strings.ToLower(key)
	for _, word := range []string{"account", "puuid", "summoner", "username", "password", "token"} {
		if strings.Contains(lower, word) {
			return false
		}
	}
	if len(key) > 160 || strings.ContainsAny(key, "/\\\r\n:") {
		return false
	}
	return true
}
func cameraSettingsFromJSON(data []byte) map[string]string {
	values := map[string]string{}
	var root any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if decoder.Decode(&root) != nil {
		return values
	}
	var walk func(any, string, bool, int)
	walk = func(node any, prefix string, input bool, depth int) {
		if depth > 16 {
			return
		}
		switch v := node.(type) {
		case map[string]any:
			// PersistedSettings files/sections/settings records and LCU named records.
			if name, ok := v["name"].(string); ok {
				if strings.EqualFold(name, "Input.ini") {
					input = true
				}
				if value, ok := v["value"]; ok {
					key := strings.Trim(prefix+"."+name, ".")
					if cameraSettingName(name, input) && cameraSafeKey(key) {
						if input || strings.HasPrefix(strings.ToLower(name), "evt") {
							values[key] = "present"
						} else if text, ok := cameraDiagnosticValue(value); ok {
							values[key] = text
						}
					}
					return
				}
				if strings.EqualFold(name, "Game.cfg") || strings.EqualFold(name, "Input.ini") {
					prefix = name
				} else {
					prefix = strings.Trim(prefix+"."+name, ".")
				}
			}
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if key == "name" || key == "description" {
					continue
				}
				child := v[key]
				path := strings.Trim(prefix+"."+key, ".")
				if key == "files" || key == "sections" || key == "settings" {
					path = prefix
				}
				nextInput := input || strings.EqualFold(key, "input.ini") || strings.EqualFold(key, "input") || strings.EqualFold(key, "hotkeys") || strings.EqualFold(key, "keybindings")
				switch child.(type) {
				case map[string]any, []any:
					walk(child, path, nextInput, depth+1)
				default:
					if cameraSettingName(key, nextInput) && cameraSafeKey(path) {
						if nextInput || strings.HasPrefix(strings.ToLower(key), "evt") {
							values[path] = "present"
						} else if text, ok := cameraDiagnosticValue(child); ok {
							values[path] = text
						}
					}
				}
			}
		case []any:
			for _, child := range v {
				walk(child, prefix, input, depth+1)
			}
		}
	}
	walk(root, "", false, 0)
	return values
}
func cameraSettingsFromINI(data []byte, input bool) map[string]string {
	values := map[string]string{}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 256<<10+1)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		key := strings.Trim(section+"."+name, ".")
		if !ok || !cameraSettingName(name, input) || !cameraSafeKey(key) {
			continue
		}
		if input {
			values[key] = "present"
		} else if text, ok := cameraDiagnosticValue(strings.TrimSpace(value)); ok {
			values[key] = text
		}
	}
	return values
}
func readGameSettingsWatchFiles(location settingsLocation, stage string) []gameSettingsFileSnapshot {
	snapshots := []gameSettingsFileSnapshot{}
	for _, file := range gameSettingsCandidates(location) {
		kind := settingsWatchPathKind(location, file)
		if kind == "" {
			continue
		}
		snapshot := gameSettingsFileSnapshot{PathKind: kind, Located: file == location.file, Values: map[string]string{}, Changes: map[string]map[string]string{}, stage: stage}
		if !settingsWatchNoSymlinks(location, file) {
			snapshot.Result = "unsafe-or-unreadable"
			snapshots = append(snapshots, snapshot)
			continue
		}
		candidate := location
		candidate.file = file
		_, info, err := safeSettingsFile(candidate)
		if err != nil {
			snapshot.Result = "unsafe-or-unreadable"
			if os.IsNotExist(err) {
				snapshot.Result = "missing"
			}
			snapshots = append(snapshots, snapshot)
			continue
		}
		snapshot.Exists = true
		snapshot.ReadOnly = gameSettingsReadOnly(file, info)
		snapshot.Size = info.Size()
		snapshot.Mtime = info.ModTime().UTC().Format(time.RFC3339Nano)
		data, result := readBoundedItemSetDiagnosticFile(candidate, file, 256<<10)
		snapshot.Result = result
		if result == "ok" {
			_, after, statErr := safeSettingsFile(candidate)
			if statErr != nil || !settingsWatchNoSymlinks(location, file) || !os.SameFile(info, after) || info.Size() != int64(len(data)) || !info.ModTime().Equal(after.ModTime()) || snapshot.ReadOnly != gameSettingsReadOnly(file, after) {
				snapshot.Result = "changed-during-read"
				snapshots = append(snapshots, snapshot)
				continue
			}
		}
		if result == "ok" {
			snapshot.Hash = itemSetDigest(data)[:8]
			if strings.EqualFold(filepath.Base(file), "PersistedSettings.json") {
				snapshot.Values = cameraSettingsFromJSON(data)
			} else {
				snapshot.Values = cameraSettingsFromINI(data, strings.EqualFold(filepath.Base(file), "input.ini"))
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

// A camera-shaped string must not become a route for known local usernames or
// the current player's identity. Keep ordinary camera enums and numbers intact.
func sanitizeGameSettingsCameraValues(values map[string]string, location settingsLocation, current Summoner) {
	secrets := []string{current.PUUID, current.GameName, current.DisplayName, current.TagLine}
	if current.AccountID > 0 {
		secrets = append(secrets, strconv.FormatInt(current.AccountID, 10))
	}
	if current.SummonerID > 0 {
		secrets = append(secrets, strconv.FormatInt(current.SummonerID, 10))
	}
	common := map[string]bool{"users": true, "home": true, "game": true, "config": true, "league": true, "leagueclient": true, "league of legends": true, "riot games": true, "private": true, "folders": true}
	for _, part := range strings.FieldsFunc(location.installRoot, func(r rune) bool { return r == '/' || r == '\\' }) {
		if !common[strings.ToLower(part)] {
			secrets = append(secrets, part)
		}
	}
	sanitized := []string{}
	exact := map[string]bool{}
	for _, secret := range secrets {
		if len([]rune(secret)) >= 2 {
			if _, err := strconv.ParseInt(secret, 10, 64); err != nil || len(secret) >= 4 {
				exact[strings.ToLower(secret)] = true
			}
		}
		if len([]rune(secret)) >= 4 {
			sanitized = append(sanitized, strings.ToLower(secret))
			runes := []rune(secret)
			if len(runes) > 32 {
				sanitized = append(sanitized, strings.ToLower(string(runes[:32])))
			}
		}
	}
	for key, value := range values {
		if exact[strings.ToLower(value)] {
			values[key] = "[redacted]"
			continue
		}
		for _, secret := range sanitized {
			if strings.Contains(strings.ToLower(key), secret) {
				delete(values, key)
				break
			}
			if strings.Contains(strings.ToLower(value), secret) {
				values[key] = "[redacted]"
				break
			}
		}
	}
}

func gameSettingsCameraChanges(before, after map[string]string) map[string]map[string]string {
	changes := map[string]map[string]string{}
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	for key := range keys {
		old, oldOK := before[key]
		next, nextOK := after[key]
		if old != next || oldOK != nextOK {
			changes[key] = map[string]string{"before": old, "after": next}
		}
	}
	return changes
}
func gameSettingsWriterGuess(stage, previous string, hashChanged bool) string {
	if stage == "lock_action" && !hashChanged {
		return "app_lock_action"
	}
	if stage == "champselect" || stage == "game_start" && previous == "champselect" {
		return "client_before_launch"
	}
	return "game"
}
func (a *app) recordGameSettingsWatch(parent context.Context, client *LCUClient, job gameSettingsWatchJob) {
	s := &a.gameSettingsWatch
	s.mu.Lock()
	valid := s.client == client && (!job.delayed || s.generation == job.generation)
	s.mu.Unlock()
	if !valid || parent.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var location settingsLocation
	var err error
	if job.location != nil {
		location = *job.location
	} else {
		location, err = locateGameSettings(ctx, client)
	}
	event := map[string]any{"event": "game_settings_watch", "stage": job.stage, "files": []gameSettingsFileSnapshot{}}
	snapshots := []gameSettingsFileSnapshot{}
	if err == nil {
		snapshots = readGameSettingsWatchFiles(location, job.stage)
	} else {
		event["result"] = "locate-unavailable"
	}
	var payload json.RawMessage
	settingsErr := client.RequestJSON(ctx, http.MethodGet, "/lol-game-settings/v1/game-settings", nil, &payload)
	if settingsErr != nil {
		event["lcu_settings"] = "unavailable"
		status := 0
		var httpErr *LCUHTTPError
		if errors.As(settingsErr, &httpErr) {
			status = httpErr.StatusCode
		}
		event["lcu_http_status"] = status
	} else {
		event["lcu_settings"] = cameraSettingsFromJSON(payload)
	}
	a.mu.RLock()
	current := a.summoner
	a.mu.RUnlock()
	for i := range snapshots {
		sanitizeGameSettingsCameraValues(snapshots[i].Values, location, current)
	}
	if values, ok := event["lcu_settings"].(map[string]string); ok {
		sanitizeGameSettingsCameraValues(values, location, current)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != client || job.delayed && s.generation != job.generation || parent.Err() != nil {
		return
	}
	for i := range snapshots {
		next := &snapshots[i]
		previous, known := s.prev[next.PathKind]
		if !known {
			previous.stage = "lock_action_before"
		}
		next.Changes = gameSettingsCameraChanges(previous.Values, next.Values)
		if !known {
			next.Changes = map[string]map[string]string{}
		}
		hashChanged := known && previous.Hash != "" && next.Hash != "" && previous.Hash != next.Hash
		permissionChanged := known && previous.Exists && next.Exists && previous.ReadOnly != next.ReadOnly
		readOnlyBefore := previous.ReadOnly
		if job.stage == "lock_action" && next.Located && job.lockReadOnlyBefore != nil {
			readOnlyBefore = *job.lockReadOnlyBefore
			permissionChanged = next.Exists && readOnlyBefore != next.ReadOnly
		}
		next.Changed = hashChanged
		if hashChanged || job.stage == "lock_action" && permissionChanged {
			a.recordDiagnostic(map[string]any{"event": "game_settings_changed", "path_kind": next.PathKind, "between": previous.stage + " → " + job.stage, "camera_changed_keys": next.Changes, "read_only_before": readOnlyBefore, "read_only_after": next.ReadOnly, "writer_guess": gameSettingsWriterGuess(job.stage, previous.stage, hashChanged)})
		}
		if hashChanged && s.end != nil && s.generation == job.generation {
			s.endChanged[next.PathKind] = true
		}
		if job.stage == "lobby_after_60s" {
			if ended, ok := s.end[next.PathKind]; ok && ended.Hash != "" && next.Hash == ended.Hash && next.ReadOnly && !s.endChanged[next.PathKind] {
				a.recordDiagnostic(map[string]any{"event": "game_settings_write_blocked_suspected", "path_kind": next.PathKind, "between": "game_end → lobby_after_60s"})
			}
		}
		s.prev[next.PathKind] = *next
	}
	if job.stage == "game_end" && s.generation == job.generation {
		s.end = make(map[string]gameSettingsFileSnapshot)
		s.endChanged = make(map[string]bool)
		for _, file := range snapshots {
			s.end[file.PathKind] = file
		}
	}
	event["files"] = snapshots
	a.recordDiagnostic(event)
}
