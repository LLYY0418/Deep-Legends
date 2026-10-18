package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Attached only when a confirmed Arena live snapshot is in ChampSelect/GameStart.
// The existing discovery completion channel publishes the immutable before data.
type arenaProgressionProbe struct {
	before    arenaProgressionSnapshot
	done      chan struct{}
	scheduled bool
	sawGame   bool
	emitted   bool
}
type arenaProgressionSnapshot struct {
	Values     map[string]float64
	Status     map[string]string
	Operations []lcuHelpOperation
}
type arenaProgressionDelta struct {
	Before float64 `json:"before"`
	After  float64 `json:"after"`
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
func (a *app) startArenaProgressionDiscovery(client *LCUClient, phase string) {
	if a == nil || client == nil || (phase != "ChampSelect" && phase != "GameStart") {
		return
	}
	s := client.arenaDiscoveryState()
	s.mu.Lock()
	if s.progression != nil {
		s.mu.Unlock()
		return
	}
	p := &arenaProgressionProbe{done: make(chan struct{}), sawGame: phase == "GameStart"}
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
		_ = client.RequestJSON(ctx, http.MethodGet, "/help?format=Full", nil, &help)
		a.mu.RLock()
		self := a.summoner
		a.mu.RUnlock()
		p.before = a.readArenaProgressionSnapshot(ctx, client, arenaDiscoveryHelpAll(help), self)
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
				if !arenaDiscoveryIdentifier.MatchString(k) || arenaDiscoveryPrivate.MatchString(k) || len(k) >= 36 {
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
		if strings.EqualFold(part, "{groupId}") {
			if group == "" {
				return ""
			}
			path = strings.ReplaceAll(path, part, url.PathEscape(group))
		}
	}
	return arenaDiscoveryGETPath(path, self)
}
func (a *app) readArenaProgressionSnapshot(ctx context.Context, c *LCUClient, ops []lcuHelpOperation, self Summoner) arenaProgressionSnapshot {
	snap := arenaProgressionSnapshot{Values: map[string]float64{}, Status: map[string]string{}, Operations: append([]lcuHelpOperation(nil), ops...)}
	selected := map[string]lcuHelpOperation{}
	for _, op := range ops {
		kind := arenaProgressionKind(op)
		if kind != "" {
			if _, ok := selected[kind]; !ok || op.Method == http.MethodGet && op.Path != "" {
				selected[kind] = op
			}
		}
	}
	read := func(op lcuHelpOperation, group string) (json.RawMessage, string) {
		path := arenaProgressionPath(op, self, group)
		if path == "" {
			return nil, "unresolved"
		}
		probe, stop := context.WithTimeout(ctx, 2*time.Second)
		defer stop()
		var raw json.RawMessage
		if c.RequestJSON(probe, http.MethodGet, path, nil, &raw) != nil {
			return nil, "read_failed"
		}
		if !json.Valid(raw) {
			return nil, "read_failed"
		}
		return raw, "ok"
	}
	raw, status := read(selected["configuration"], "")
	snap.Status["configuration"] = status
	groups := arenaProgressionGroups(raw)
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
			label := g.name
			if label == "" {
				label = "group"
			}
			// Group IDs come only from configuration and keep keys stable across reorderings.
			label = label + "(" + g.id + ")"
			for k, v := range arenaProgressionNumbers(raw) {
				snap.Values[label+"/"+k] = v
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
			changes[name] = arenaProgressionDelta{Before: prior, After: value}
		}
	}
	return changes
}
func (a *app) observeArenaProgressionPhase(c *LCUClient, phase string) {
	s := c.arenaDiscoveryState()
	s.mu.Lock()
	p := s.progression
	if p != nil && (phase == "GameStart" || phase == "InProgress" || phase == "Reconnect") {
		p.sawGame = true
	}
	if phase != "Lobby" && phase != "None" {
		s.mu.Unlock()
		return
	}
	if p == nil || !p.sawGame || p.scheduled || p.emitted {
		s.mu.Unlock()
		return
	}
	p.scheduled = true
	schedule := s.schedule
	if schedule == nil {
		schedule = func(delay time.Duration, fn func()) func() {
			timer := time.AfterFunc(delay, fn)
			return func() { timer.Stop() }
		}
	}
	s.stops = append(s.stops, schedule(20*time.Second, func() { a.probeArenaProgressionAfter(c, s) }))
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
	after := a.readArenaProgressionSnapshot(ctx, c, p.before.Operations, self)
	changes := arenaProgressionDiff(p.before, after)
	s.mu.Lock()
	if p.emitted {
		s.mu.Unlock()
		return
	}
	p.emitted = true
	s.mu.Unlock()
	event := map[string]any{"event": "arena_fame_progression_diff", "changes": changes, "before_status": p.before.Status, "after_status": after.Status}
	for _, part := range arenaFameDiscoveryParts(event) {
		a.recordDiagnostic(part)
	}
}
