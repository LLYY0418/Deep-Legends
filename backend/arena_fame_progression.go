package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Attached only when a confirmed Arena live snapshot is in ChampSelect/GameStart.
// The existing discovery completion channel publishes the immutable before data.
type arenaProgressionProbe struct {
	account      string
	platform     string
	gameID       int64
	at           time.Time
	lobbyAt      time.Time
	before       arenaProgressionSnapshot
	beforeEvents map[string]float64
	done         chan struct{}
	scheduled    bool
	sawGame      bool
	emitted      bool
}
type arenaProgressionSnapshot struct {
	GroupIDs      []string
	GroupPaths    []string
	Values        map[string]float64
	Status        map[string]string
	Operations    []lcuHelpOperation
	ResolvedPaths map[string]string
	HTTPStatus    map[string]int
}
type arenaProgressionDelta struct {
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Delta  float64 `json:"delta"`
}
type arenaProgressionGroup struct{ id, name string }

func arenaProgressionKind(op lcuHelpOperation) string {
	label := strings.ToLower(op.Name + " " + op.Path)
	if strings.Contains(label, "progression") && strings.Contains(label, "group") {
		if strings.Contains(label, "configuration") {
			return "configuration"
		}
		if strings.Contains(label, "instancedata") || strings.Contains(label, "instance-data") {
			return "instance-data"
		}
	}
	if strings.Contains(label, "inventory") && strings.Contains(label, "cherry") {
		return "cherry-inventory"
	}
	return ""
}
func (a *app) startArenaProgressionDiscovery(client *LCUClient, phase string, gameIDs ...int64) {
	if a == nil || client == nil || (phase != "ChampSelect" && phase != "GameStart") {
		return
	}
	s := client.arenaDiscoveryState()
	s.mu.Lock()
	if s.progression != nil {
		p := s.progression
		if p.gameID == 0 && len(gameIDs) > 0 && gameIDs[0] > 0 {
			p.gameID = gameIDs[0]
			// The first read persists this ID if it is still running. Once
			// published, update the existing snapshot without another GET.
			select {
			case <-p.done:
				a.persistArenaProgression(p)
			default:
			}
		}
		s.mu.Unlock()
		return
	}
	p := &arenaProgressionProbe{done: make(chan struct{}), sawGame: phase == "GameStart", beforeEvents: arenaProgressionEventSnapshot(s)}
	if len(gameIDs) > 0 {
		p.gameID = gameIDs[0]
	}
	p.at = time.Now()
	a.mu.RLock()
	self := a.summoner
	a.mu.RUnlock()
	if a.storage != nil {
		p.account = a.storage.accountHash(self)
	}
	p.platform = clientRiotPlatform(client) + "/" + clientTencentServerID(client)
	s.progression = p
	s.mu.Unlock()
	// Progression owns its pregame read even if the older discovery already ran.
	a.goSafe("arena.progression-before", func() {
		defer close(p.done)
		client.mu.RLock()
		session := client.sessionContext
		client.mu.RUnlock()
		if session == nil {
			session = context.Background()
		}
		ctx, cancel := context.WithTimeout(session, 30*time.Second)
		defer cancel()
		var help json.RawMessage
		helpCtx, helpCancel := context.WithTimeout(ctx, 2*time.Second)
		_ = client.RequestJSON(helpCtx, http.MethodGet, "/help?format=Full", nil, &help)
		helpCancel()
		a.mu.RLock()
		self := a.summoner
		a.mu.RUnlock()
		p.before = a.readArenaProgressionSnapshot(ctx, client, arenaDiscoveryHelpAll(help), self)
		s.mu.Lock()
		a.persistArenaProgression(p)
		s.mu.Unlock()
		event := map[string]any{"event": "arena_fame_progression_before", "resolved_paths": p.before.ResolvedPaths, "before_status": p.before.Status, "before_http_status": p.before.HTTPStatus, "numeric_keys": len(p.before.Values), "group_ids": p.before.GroupIDs}
		for _, part := range arenaFameDiscoveryParts(event) {
			a.recordDiagnostic(part)
		}
	})
	a.discoverArenaFame(client)
}

// IDs and names are accepted only from group configuration, never player payloads.
func arenaProgressionGroups(raw json.RawMessage) []arenaProgressionGroup {
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	groups := map[string]arenaProgressionGroup{}
	var walk func(any, string, int)
	walk = func(v any, key string, depth int) {
		if depth > 8 {
			return
		}
		switch n := v.(type) {
		case []any:
			for _, child := range n {
				walk(child, "", depth+1)
			}
		case map[string]any:
			id := ""
			for _, field := range []string{"groupId", "id"} {
				switch x := n[field].(type) {
				case string:
					id = x
				case float64:
					if x >= 0 && x == float64(int64(x)) {
						id = strconv.FormatInt(int64(x), 10)
					}
				}
				if id != "" {
					break
				}
			}
			name := firstString(n, "groupName", "name")

			if id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/\\?#%{}\x00\r\n") {
				if !arenaDiscoveryIdentifier.MatchString(name) || arenaDiscoveryPrivate.MatchString(name) {
					name = ""
				}
				groups[id] = arenaProgressionGroup{id, name}
				return
			}
			keys := make([]string, 0, len(n))
			for k := range n {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(n[k], k, depth+1)
			}
		}
	}
	walk(root, "", 0)
	out := make([]arenaProgressionGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// Keep numeric leaves only. Dynamic account keys, identifiers, names, tokens and
// payload strings are discarded rather than forwarded to diagnostics.
var arenaProgressionPrivate = regexp.MustCompile(`(?i)puuid|uuid|game.?name|display.?name|tag.?line|^(?:(?:access|refresh|auth|session|entitlements)?token|(?:summoner|account|player)?id)$|Id$`)

func arenaProgressionNumbers(raw json.RawMessage) map[string]float64 {
	out := map[string]float64{}
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return out
	}
	var walk func(any, string, int)
	walk = func(v any, path string, depth int) {
		if depth > 12 {
			return
		}
		switch n := v.(type) {
		case float64:
			if path != "" {
				out[path] = n
			}
		case map[string]any:
			keys := make([]string, 0, len(n))
			for k := range n {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if !arenaDiscoveryIdentifier.MatchString(k) || arenaProgressionPrivate.MatchString(k) || len(k) >= 36 {
					continue
				}
				p := k
				if path != "" {
					p = path + "." + k
				}
				walk(n[k], p, depth+1)
			}
		case []any:
			for i, item := range n {
				walk(item, fmt.Sprintf("%s[%d]", path, i), depth+1)
			}
		}
	}
	walk(root, "", 0)
	return out
}
func arenaProgressionPath(op lcuHelpOperation, self Summoner, group string) string {
	if op.Method != http.MethodGet || op.Path == "" {
		return ""
	}
	path := op.Path
	for _, part := range strings.Split(path, "/") {
		if strings.EqualFold(part, "{groupId}") || strings.EqualFold(part, "{id}") {
			if group == "" {
				return ""
			}
			path = strings.ReplaceAll(path, part, url.PathEscape(group))
		}
	}
	return arenaDiscoveryGETPath(path, self)
}
func (a *app) readArenaProgressionSnapshot(ctx context.Context, c *LCUClient, ops []lcuHelpOperation, self Summoner, baseline ...arenaProgressionSnapshot) arenaProgressionSnapshot {
	snap := arenaProgressionSnapshot{Values: map[string]float64{}, Status: map[string]string{}, Operations: append([]lcuHelpOperation(nil), ops...), ResolvedPaths: map[string]string{}, HTTPStatus: map[string]int{}}
	selected := map[string]lcuHelpOperation{}
	for _, op := range ops {
		kind := arenaProgressionKind(op)
		if kind != "" {
			if _, ok := selected[kind]; !ok || op.Method == http.MethodGet && op.Path != "" {
				selected[kind] = op
			}
		}
	}
	for _, kind := range []string{"configuration", "instance-data", "cherry-inventory"} {
		op := selected[kind]
		snap.HTTPStatus[kind] = 0
		if op.Path == "" || op.Method == "" {
			names := map[string]string{"configuration": "GetLolProgressionV1GroupsConfiguration", "instance-data": "GetLolProgressionV1GroupsByGroupIdInstanceData", "cherry-inventory": "GetLolInventoryV1CherryInventory"}
			name := names[kind]
			probe, stop := context.WithTimeout(ctx, 2*time.Second)
			var full json.RawMessage
			err := c.RequestJSON(probe, http.MethodGet, "/help?target="+url.QueryEscape(name)+"&format=Full", nil, &full)
			stop()
			for _, candidate := range arenaDiscoveryHelpAll(full) {
				if arenaProgressionKind(candidate) == kind && candidate.Method == http.MethodGet && candidate.Path != "" {
					op = candidate
					break
				}
			}
			_ = err // Endpoint status is recorded below, without response strings.
			if op.Path == "" || op.Method == "" {
				op = lcuHelpOperation{Name: name, Method: http.MethodGet, Path: map[string]string{"configuration": "/lol-progression/v1/groups/configuration", "instance-data": "/lol-progression/v1/groups/{groupId}/instance-data", "cherry-inventory": "/lol-inventory/v1/cherry-inventory"}[kind]}
			}
			selected[kind] = op
		}
		if op.Method == http.MethodGet {
			snap.ResolvedPaths[kind] = arenaDiscoveryURI(op.Path)
		}
	}
	snap.Operations = nil
	for _, kind := range []string{"configuration", "instance-data", "cherry-inventory"} {
		snap.Operations = append(snap.Operations, selected[kind])
	}
	read := func(op lcuHelpOperation, group string) (json.RawMessage, string) {
		path := arenaProgressionPath(op, self, group)
		if path == "" {
			return nil, "unresolved"
		}
		probe, stop := context.WithTimeout(ctx, 2*time.Second)
		defer stop()
		var raw json.RawMessage
		kind := arenaProgressionKind(op)
		if kind == "" {
			kind = "group_path/" + path
		}
		if err := c.RequestJSON(probe, http.MethodGet, path, nil, &raw); err != nil {
			var httpErr *LCUHTTPError
			if errors.As(err, &httpErr) {
				snap.HTTPStatus[kind] = httpErr.StatusCode
				return nil, fmt.Sprintf("http-%d", httpErr.StatusCode)
			}
			snap.HTTPStatus[kind] = 0
			return nil, "read_failed"
		}
		snap.HTTPStatus[kind] = http.StatusOK
		if !json.Valid(raw) {
			return nil, "read_failed"
		}
		return raw, "ok"
	}
	raw, status := read(selected["configuration"], "")
	snap.Status["configuration"] = status
	groups := arenaProgressionGroups(raw)
	if len(baseline) > 0 {
		groups = nil
		for _, id := range baseline[0].GroupIDs {
			groups = append(groups, arenaProgressionGroup{id: id})
		}
	}
	state := c.arenaDiscoveryState()
	state.mu.Lock()
	if state.progressionGroupIDs == nil {
		state.progressionGroupIDs = map[string]bool{}
	}
	for _, g := range groups {
		snap.GroupIDs = append(snap.GroupIDs, g.id)
		state.progressionGroupIDs[g.id] = true
	}
	for path := range state.progressionGroupPaths {
		snap.GroupPaths = append(snap.GroupPaths, path)
	}
	state.mu.Unlock()
	if len(baseline) > 0 {
		snap.GroupPaths = append([]string(nil), baseline[0].GroupPaths...)
	} else {
		base := strings.TrimSuffix(arenaProgressionPath(selected["configuration"], self, ""), "configuration")
		for _, g := range groups {
			snap.GroupPaths = append(snap.GroupPaths, base+url.PathEscape(g.id))
		}
	}
	sort.Strings(snap.GroupPaths)
	if status == "ok" && len(groups) == 0 {
		snap.Status["configuration"] = "empty"
	}
	instance := selected["instance-data"]
	if arenaProgressionPath(instance, self, "group") == "" {
		snap.Status["instance-data"] = "unresolved"
	} else if status != "ok" {
		snap.Status["instance-data"] = "configuration_unavailable"
	} else {
		snap.Status["instance-data"] = "ok"
		for _, g := range groups {
			raw, result := read(instance, g.id)
			if result != "ok" {
				snap.Status["instance-data"] = result
				continue
			}
			label := "group/" + g.id
			for k, v := range arenaProgressionNumbers(raw) {
				if arenaProgressionMetric(k) {
					snap.Values[label+"/"+k] = v
				}
			}
		}
	}
	for _, path := range snap.GroupPaths {
		op := lcuHelpOperation{Method: http.MethodGet, Path: path, Name: "progression group"}
		raw, result := read(op, "")
		snap.Status["group_path/"+path] = result
		if result == "ok" {
			for k, v := range arenaProgressionNumbers(raw) {
				if arenaProgressionMetric(k) {
					snap.Values["path/"+path+"/"+k] = v
				}
			}
		}
	}
	raw, status = read(selected["cherry-inventory"], "")
	snap.Status["cherry-inventory"] = status
	if status == "ok" {
		for k, v := range arenaProgressionNumbers(raw) {
			snap.Values["cherry-inventory/"+k] = v
		}
	}
	return snap
}
func arenaProgressionDiff(before, after arenaProgressionSnapshot) map[string]arenaProgressionDelta {
	changes := map[string]arenaProgressionDelta{}
	for name, value := range after.Values {
		if prior, ok := before.Values[name]; ok && prior != value {
			changes[name] = arenaProgressionDelta{Before: prior, After: value, Delta: value - prior}
		}
	}
	return changes
}
func (a *app) observeArenaProgressionPhase(c *LCUClient, phase string) {
	s := c.arenaDiscoveryState()
	s.mu.Lock()
	p := s.progression
	if p == nil {
		p = a.restoreArenaProgression(c)
		s.progression = p
	}
	if p != nil && (phase == "GameStart" || phase == "InProgress" || phase == "Reconnect") {
		p.sawGame = true
	}
	if phase != "Lobby" && phase != "None" && phase != "EndOfGame" && phase != "PreEndOfGame" {
		s.mu.Unlock()
		return
	}
	if p == nil || !p.sawGame || p.scheduled || p.emitted {
		s.mu.Unlock()
		return
	}
	p.scheduled = true
	delay := 2 * time.Minute
	if p.lobbyAt.IsZero() {
		p.lobbyAt = time.Now()
		select {
		case <-p.done:
			a.persistArenaProgression(p)
		default:
		}
	} else {
		delay = time.Until(p.lobbyAt.Add(2 * time.Minute))
		if delay < 0 {
			delay = 0
		}
	}
	schedule := s.schedule
	if schedule == nil {
		schedule = func(delay time.Duration, fn func()) func() {
			timer := time.AfterFunc(delay, fn)
			return func() { timer.Stop() }
		}
	}
	s.stops = append(s.stops, schedule(delay, func() { a.probeArenaProgressionAfter(c, s) }))
	s.mu.Unlock()
}
func (a *app) probeArenaProgressionAfter(c *LCUClient, s *arenaFameDiscoveryState) {
	s.mu.Lock()
	p := s.progression
	var done <-chan struct{}
	if p != nil {
		done = p.done
	}
	if p == nil || p.emitted {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	c.mu.RLock()
	session := c.sessionContext
	c.mu.RUnlock()
	if session == nil {
		session = context.Background()
	}
	ctx, cancel := context.WithTimeout(session, 30*time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return
	}
	if c.arenaDiscoveryState() != s {
		return
	}
	a.mu.RLock()
	self := a.summoner
	a.mu.RUnlock()
	after := a.readArenaProgressionSnapshot(ctx, c, p.before.Operations, self, p.before)
	changes := arenaProgressionDiff(p.before, after)
	s.mu.Lock()
	if p.emitted {
		s.mu.Unlock()
		return
	}
	p.emitted = true
	a.clearArenaProgression(p)
	afterEvents := arenaProgressionEventSnapshot(s)
	s.mu.Unlock()
	eventChanges := arenaProgressionDiff(arenaProgressionSnapshot{Values: p.beforeEvents}, arenaProgressionSnapshot{Values: afterEvents})
	event := map[string]any{"event": "arena_fame_progression_diff_v3", "groups": arenaProgressionGroupDiff(p.before, after), "group_ids": p.before.GroupIDs, "changes": changes, "before_status": p.before.Status, "after_status": after.Status, "resolved_paths": after.ResolvedPaths, "before_http_status": p.before.HTTPStatus, "after_http_status": after.HTTPStatus, "event_before": p.beforeEvents, "event_after": afterEvents, "event_changes": eventChanges}
	for _, part := range arenaFameDiscoveryParts(event) {
		a.recordDiagnostic(part)
	}
}

// Caller holds discovery.mu. Numeric observations are bounded and never retain strings.
func arenaProgressionEventSnapshot(s *arenaFameDiscoveryState) map[string]float64 {
	out := map[string]float64{}
	for path, values := range s.progressionEvents {
		for key, value := range values {
			out[path+"/"+key] = value
		}
	}
	return out
}
func (c *LCUClient) observeArenaProgressionEvent(event LCUEvent) {
	if c == nil || !(strings.HasPrefix(event.URI, "/lol-progression/v1/") || strings.Contains(event.URI, "/progression/v1/notifications/")) {
		return
	}
	path := arenaDiscoveryURI(event.URI)
	if path == "" {
		return
	}
	values := arenaProgressionNumbers(event.Data)
	s := c.arenaDiscoveryState()
	s.mu.Lock()
	if s.progressionEvents == nil {
		s.progressionEvents = map[string]map[string]float64{}
	}
	if _, ok := s.progressionEvents[path]; !ok && len(s.progressionEvents) >= 32 {
		s.mu.Unlock()
		return
	}
	bounded := map[string]float64{}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(bounded) >= 256 {
			break
		}
		bounded[key] = values[key]
	}
	group := ""
	parts := strings.Split(event.URI, "/")
	safeParts := strings.Split(path, "/")
	for i, p := range parts {
		if s.progressionGroupIDs[p] {
			group = p
			safeParts[i] = p
		}
	}
	path = strings.Join(safeParts, "/")
	s.progressionEvents[path] = bounded
	if group != "" && strings.Contains(event.URI, "/groups/") && !strings.Contains(path, "{id}") {
		if s.progressionGroupPaths == nil {
			s.progressionGroupPaths = map[string]bool{}
		}
		if len(s.progressionGroupPaths) < 32 {
			s.progressionGroupPaths[path] = true
		}
	}
	diagnostic := map[string]any{"event": "arena_fame_progression_event_v3", "path": path, "group_id": group, "keys": arenaProgressionKeys(event.Data), "numeric": bounded}
	s.mu.Unlock()
	c.diagnosticMu.Lock()
	observe := c.diagnosticObserve
	c.diagnosticMu.Unlock()
	if observe != nil {
		observe(diagnostic)
	}
}

// Only filtered numeric leaves and approved GET templates survive restart. The
// device-salted account hash and platform keep another account from consuming it.
type arenaProgressionDisk struct {
	Account  string                   `json:"account"`
	Platform string                   `json:"platform"`
	GameID   int64                    `json:"gameId"`
	At       time.Time                `json:"at"`
	LobbyAt  time.Time                `json:"lobbyAt,omitempty"`
	Before   arenaProgressionSnapshot `json:"before"`
}

func (a *app) persistArenaProgression(p *arenaProgressionProbe) {
	if a.storage == nil || p.emitted {
		return
	}
	raw, err := json.Marshal(arenaProgressionDisk{Account: p.account, Platform: p.platform, GameID: p.gameID, At: p.at, LobbyAt: p.lobbyAt, Before: p.before})
	if err == nil && len(raw) <= 256*1024 {
		a.storage.mu.Lock()
		defer a.storage.mu.Unlock()
		_ = writeLocalStoreFile(a.storage, "arena-progression-before.json", raw)
	}
}
func (a *app) clearArenaProgression(p *arenaProgressionProbe) {
	if a.storage == nil {
		return
	}
	a.storage.mu.Lock()
	defer a.storage.mu.Unlock()
	raw, err := readLocalStoreFile(a.storage, "arena-progression-before.json")
	var disk arenaProgressionDisk
	if err == nil && json.Unmarshal(raw, &disk) == nil && (p == nil || disk.At.Equal(p.at) && disk.GameID == p.gameID && disk.Account == p.account) {
		_ = os.Remove(filepath.Join(a.storage.root, "arena-progression-before.json"))
	}
}
func (a *app) restoreArenaProgression(c *LCUClient) *arenaProgressionProbe {
	if a.storage == nil {
		return nil
	}
	raw, err := readLocalStoreFile(a.storage, "arena-progression-before.json")
	if err != nil {
		return nil
	}
	var disk arenaProgressionDisk
	if len(raw) > 256*1024 || json.Unmarshal(raw, &disk) != nil || time.Since(disk.At) > 30*time.Minute || time.Until(disk.At) > time.Minute {
		a.clearArenaProgression(nil)
		return nil
	}
	a.mu.RLock()
	self := a.summoner
	a.mu.RUnlock()
	if disk.Account != a.storage.accountHash(self) || disk.Platform != clientRiotPlatform(c)+"/"+clientTencentServerID(c) {
		return nil
	}
	p := &arenaProgressionProbe{account: disk.Account, platform: disk.Platform, before: disk.Before, gameID: disk.GameID, at: disk.At, lobbyAt: disk.LobbyAt, sawGame: true, done: make(chan struct{})}
	close(p.done)
	return p
}

func arenaProgressionMetric(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "counter") || strings.Contains(key, "milestone") || strings.Contains(key, "triggervalue")
}
func arenaProgressionKeys(raw json.RawMessage) []string {
	var root map[string]json.RawMessage
	keys := []string{}
	if json.Unmarshal(raw, &root) != nil {
		return keys
	}
	for key := range root {
		if arenaDiscoveryIdentifier.MatchString(key) && !arenaProgressionPrivate.MatchString(key) && len(key) < 36 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > 128 {
		keys = keys[:128]
	}
	return keys
}
func arenaProgressionGroupDiff(before, after arenaProgressionSnapshot) []map[string]any {
	groups := []map[string]any{}
	for _, id := range before.GroupIDs {
		prefix := "group/" + id + "/"
		old, next, milestones := map[string]float64{}, map[string]float64{}, map[string]float64{}
		for key, value := range before.Values {
			if strings.HasPrefix(key, prefix) {
				old[strings.TrimPrefix(key, prefix)] = value
			}
		}
		for key, value := range after.Values {
			if strings.HasPrefix(key, prefix) {
				k := strings.TrimPrefix(key, prefix)
				next[k] = value
				if strings.Contains(strings.ToLower(k), "milestone") {
					milestones[k] = value
				}
			}
		}
		changes := arenaProgressionDiff(arenaProgressionSnapshot{Values: old}, arenaProgressionSnapshot{Values: next})
		if len(changes) > 0 {
			groups = append(groups, map[string]any{"group_id": id, "before": arenaProgressionChangedValues(old, changes), "after": arenaProgressionChangedValues(next, changes), "changes": changes, "milestones": milestones})
		}
	}
	return groups
}

func arenaProgressionChangedValues(values map[string]float64, changes map[string]arenaProgressionDelta) map[string]float64 {
	out := map[string]float64{}
	for key := range changes {
		out[key] = values[key]
	}
	return out
}
