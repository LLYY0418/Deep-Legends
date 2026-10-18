package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The client owns this state; reconnecting creates a new observation scope.
type arenaFameDiscoveryState struct {
	mu             sync.Mutex
	started        bool
	done           chan struct{}
	queues         map[string][]string
	uris           []string
	event          map[string]any
	flushRequested bool
	emitted        bool
	lastPhase      string
	postScheduled  bool
	schedule       func(time.Duration, func()) func()
	stops          []func()
	operations     []lcuHelpOperation
	baseline       map[string]map[string]float64
	progression    *arenaProgressionProbe
}

var arenaDiscoveryName = regexp.MustCompile(`(?i)cherry|arena|fame|gladiator|progression|eog|end-of-game|pre-end-of-game`)
var arenaDiscoveryNumber = regexp.MustCompile(`(?i)fame|level|tier|points|rating|xp`)
var arenaDiscoveryIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)
var arenaDiscoverySegment = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var arenaDiscoveryEventLabel = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_./-]*$`)
var arenaDiscoveryVersion = regexp.MustCompile(`^v[0-9]+$`)
var arenaDiscoveryParameter = regexp.MustCompile(`\{[^}]+\}`)
var arenaDiscoveryPrivate = regexp.MustCompile(`(?i)puuid|uuid|token|game.?name|display.?name|tag.?line|^(summoner|account|player)?id$|Id$`)

func (c *LCUClient) arenaDiscoveryState() *arenaFameDiscoveryState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.arenaDiscovery == nil {
		c.arenaDiscovery = &arenaFameDiscoveryState{}
	}
	return c.arenaDiscovery
}
func arenaDiscoveryQueueTypes(raw json.RawMessage) []string {
	result := []string{}
	for _, entry := range rankedQueueEntries(raw) {
		var q struct {
			QueueType string `json:"queueType"`
		}
		if json.Unmarshal(entry, &q) == nil && arenaDiscoveryIdentifier.MatchString(q.QueueType) && !slices.Contains(result, q.QueueType) {
			result = append(result, q.QueueType)
		}
	}
	sort.Strings(result)
	return result
}
func (c *LCUClient) rememberArenaQueueTypes(source string, types []string) {
	if c == nil {
		return
	}
	s := c.arenaDiscoveryState()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queues == nil {
		s.queues = map[string][]string{}
	}
	s.queues[source] = types
}
func arenaDiscoveryURI(uri string) string {
	parsed, err := url.ParseRequestURI(uri)
	if err != nil {
		return ""
	}
	parts := strings.Split(parsed.Path, "/")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if _, err := strconv.ParseInt(p, 10, 64); err == nil || len(p) > 36 || !arenaDiscoverySegment.MatchString(p) {
			parts[i] = "{id}"
		} else if !strings.HasPrefix(p, "lol-") && !arenaDiscoveryVersion.MatchString(p) && !arenaDiscoveryName.MatchString(p) && !slices.Contains([]string{"current", "data", "stats", "status", "progress", "levels", "level", "tiers", "tier", "points", "xp", "rating", "ratings", "player", "players", "notifications", "update"}, p) {
			parts[i] = "{id}"
		}

	}
	return strings.Join(parts, "/")
}
func (c *LCUClient) observeArenaDiscoveryEvent(event LCUEvent) {
	if c == nil || !arenaDiscoveryObservedURI(event.URI) {
		return
	}
	s := c.arenaDiscoveryState()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.emitted || len(s.uris) >= 64 {
		return
	}
	uri := arenaDiscoveryURI(event.URI)
	if uri != "" && !slices.Contains(s.uris, uri) {
		s.uris = append(s.uris, uri)
	}
}

// Keep field paths, never payload strings, array contents containing identities,
// or dynamic object keys. Only numeric fame-related leaves are retained.
func arenaDiscoveryShape(raw json.RawMessage) ([]string, map[string]float64) {
	var root any
	keys := []string{}
	numbers := map[string]float64{}
	if json.Unmarshal(raw, &root) != nil {
		return keys, numbers
	}
	var walk func(any, string, int)
	walk = func(v any, path string, depth int) {
		if depth > 6 || len(numbers) >= 48 {
			return
		}
		switch node := v.(type) {
		case map[string]any:
			names := make([]string, 0, len(node))
			for k := range node {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				if !arenaDiscoveryIdentifier.MatchString(k) || arenaDiscoveryPrivate.MatchString(k) {
					continue
				}
				p := k
				if path != "" {
					p = path + "." + k
				}
				if depth == 0 {
					keys = append(keys, k)
				}
				if n, ok := node[k].(float64); ok && arenaDiscoveryNumber.MatchString(k) {
					numbers[p] = n
				} else {
					walk(node[k], p, depth+1)
				}
			}
		case []any:
			for i, item := range node {
				if i >= 16 {
					break
				}
				walk(item, path+"["+strconv.Itoa(i)+"]", depth+1)
			}
		}
	}
	walk(root, "", 0)
	return keys, numbers
}
func arenaDiscoveryHelp(raw json.RawMessage) []lcuHelpOperation {
	ops := arenaDiscoveryHelpAll(raw)
	if len(ops) > 60 {
		ops = ops[:60]
	}
	return ops
}
func arenaDiscoveryHelpAll(raw json.RawMessage) []lcuHelpOperation {
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	ops := []lcuHelpOperation{}
	var walk func(any, string)
	walk = func(v any, key string) {
		switch node := v.(type) {
		case string:
			if arenaDiscoveryName.MatchString(key) && len(key) <= 120 && arenaDiscoveryEventLabel.MatchString(key) {
				op := lcuHelpOperation{Name: key}
				if !slices.Contains(ops, op) {
					ops = append(ops, op)
				}
			}
		case []any:
			for _, child := range node {
				walk(child, "")
			}
		case map[string]any:
			name := firstString(node, "name")
			if name == "" {
				name = key
			}
			path := firstString(node, "path", "url", "uri", "endpoint")
			method := strings.ToUpper(firstString(node, "method", "httpMethod", "http_method", "verb"))
			for _, label := range []string{name, key} {
				parts := strings.Fields(label)
				if len(parts) == 2 && strings.HasPrefix(parts[1], "/") {
					method, path = strings.ToUpper(parts[0]), parts[1]
				}
			}
			if path == "" && strings.HasPrefix(name, "/") {
				path = name
			}
			if path != "" && strings.HasPrefix(path, "/lol-") && arenaDiscoveryName.MatchString(path) && !strings.ContainsAny(path, "?#") {
				// A static path can still require query/body arguments. Do not probe it.
				for _, label := range []string{"arguments", "parameters"} {
					if arenaHelpRequiredOutsidePath(node[label], path) {
						method = "GET_REQUIRES_PARAMETERS"
					}
				}
				op := lcuHelpOperation{Method: method, Path: path, Name: name}
				if !slices.Contains(ops, op) {
					ops = append(ops, op)
				}
			}
			// /help event maps often only expose their event name.
			if path == "" && arenaDiscoveryName.MatchString(name) && len(name) <= 120 && arenaDiscoveryEventLabel.MatchString(name) {
				op := lcuHelpOperation{Name: name}
				if !slices.Contains(ops, op) {
					ops = append(ops, op)
				}
			}
			for k, child := range node {
				walk(child, k)
			}
		}
	}
	walk(root, "")
	slices.SortFunc(ops, func(a, b lcuHelpOperation) int { return strings.Compare(a.Path+a.Name, b.Path+b.Name) })

	return ops
}
func arenaDiscoveryGETPath(template string, self Summoner) string {
	path := template
	for _, part := range strings.Split(path, "/") {
		if !strings.ContainsAny(part, "{}") {
			continue
		}
		var value string
		switch strings.ToLower(part) {
		case "{summonerid}":
			if self.SummonerID > 0 {
				value = strconv.FormatInt(self.SummonerID, 10)
			}
		case "{puuid}":
			value = self.PUUID
		default:
			return ""
		}
		if value == "" {
			return ""
		}
		path = strings.ReplaceAll(path, part, url.PathEscape(value))
	}
	if strings.ContainsAny(path, "{}?#") || validateLCURequestPath(path) != nil {
		return ""
	}
	return path
}
func (a *app) discoverArenaFame(client *LCUClient) {
	if a == nil || client == nil {
		return
	}
	s := client.arenaDiscoveryState()
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.done = make(chan struct{})
	s.mu.Unlock()
	client.mu.RLock()
	session := client.sessionContext
	client.mu.RUnlock()
	if session == nil {
		session = context.Background()
	}
	a.goSafe("arena.fame-discovery", func() {
		defer close(s.done)
		ctx, cancel := context.WithTimeout(session, 30*time.Second)
		defer cancel()
		a.mu.RLock()
		self := a.summoner
		a.mu.RUnlock()
		event := map[string]any{"event": "arena_fame_discovery"}
		var help json.RawMessage
		var helpStatus atomic.Int64
		err := client.RequestJSON(context.WithValue(ctx, lcuResponseStatusKey{}, &helpStatus), http.MethodGet, "/help?format=Full", nil, &help)
		operations := arenaDiscoveryHelp(help)
		if err != nil || len(operations) == 0 {
			help = nil
			helpStatus.Store(0)
			_ = client.RequestJSON(context.WithValue(ctx, lcuResponseStatusKey{}, &helpStatus), http.MethodGet, "/help", nil, &help)
			operations = arenaDiscoveryHelp(help)
		}
		event["help_status"] = int(helpStatus.Load())
		baseline := map[string]map[string]float64{}
		results := []map[string]any{}
		for _, op := range operations {
			row := map[string]any{}
			if op.Path != "" {
				row["path"] = arenaDiscoveryHelpPath(op.Path)
			} else {
				row["name"] = op.Name
			}
			row["method"] = op.Method
			if op.Method == http.MethodGet && arenaProgressionKind(op) == "" {
				if path := arenaDiscoveryGETPath(op.Path, self); path != "" {
					var raw json.RawMessage
					probe, cancel := context.WithTimeout(ctx, 2*time.Second)
					var status atomic.Int64
					_ = client.RequestJSON(context.WithValue(probe, lcuResponseStatusKey{}, &status), http.MethodGet, path, nil, &raw)
					cancel()
					row["status"] = int(status.Load())
					keys, numbers := arenaDiscoveryShape(raw)
					row["keys"], row["numbers"] = keys, numbers
					baseline[op.Path] = numbers
				}
			}

			results = append(results, row)
		}
		event["interfaces"] = results
		s.mu.Lock()
		event["queue_types"] = map[string][]string{"lcu": s.queues[dataSourceLCU], "sgp": s.queues[dataSourceSGP]}
		event["event_uris"] = append([]string(nil), s.uris...)
		s.mu.Unlock()

		s.mu.Lock()
		s.event = event
		s.operations = operations
		s.baseline = baseline
		flush := s.flushRequested
		s.mu.Unlock()
		if flush {
			a.finishArenaFameDiscovery(s)
		}
	})
}

// Emit the complete observation when Arena ends (or the connection closes),
// including WebSocket names seen after the help probes, in bounded records.
func (a *app) finishArenaFameDiscovery(s *arenaFameDiscoveryState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.flushRequested = true
	if s.event == nil || s.emitted {
		s.mu.Unlock()
		return
	}
	s.emitted = true
	event := make(map[string]any, len(s.event))
	for key, value := range s.event {
		event[key] = value
	}
	for _, stop := range s.stops {
		stop()
	}
	s.stops = nil
	event["queue_types"] = map[string][]string{"lcu": s.queues[dataSourceLCU], "sgp": s.queues[dataSourceSGP]}
	event["event_uris"] = append([]string(nil), s.uris...)
	s.mu.Unlock()
	for _, part := range arenaFameDiscoveryParts(event) {
		a.recordDiagnostic(part)
	}
}

// Keep the compact form for small observations. Larger observations are split
// by top-level field; concatenating data in part_index order yields that field's
// exact JSON. Split on runes so even oversized individual shapes remain intact.
func arenaFameDiscoveryParts(event map[string]any) []map[string]any {
	if raw, err := json.Marshal(event); err == nil && len(raw) <= 3000 {
		return []map[string]any{event}
	}
	keys := make([]string, 0, len(event))
	for key := range event {
		if key != "event" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := []map[string]any{}
	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	for _, key := range keys {
		raw, err := json.Marshal(event[key])
		if err != nil {
			continue
		} // Observations contain only JSON values.
		chunk := ""
		emit := func() {
			parts = append(parts, map[string]any{"event": event["event"], "discovery_id": id, "part": key, "encoding": "json-fragments", "data": chunk})
			chunk = ""
		}
		for _, r := range string(raw) {
			next := chunk + string(r)
			encoded, _ := json.Marshal(next)
			// 2 KiB of encoded data leaves room for the standard log envelope,
			// the part labels and indices, safely below the 4095-byte limit.
			if len(encoded) > 2000 && chunk != "" {
				emit()
			}
			chunk += string(r)
		}
		if chunk != "" {
			emit()
		}
	}
	for i, part := range parts {
		part["part_index"], part["part_count"] = i+1, len(parts)
	}
	return parts
}

func arenaDiscoveryHelpPath(path string) string {
	path = arenaDiscoveryParameter.ReplaceAllString(path, "{id}")
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if _, err := strconv.ParseUint(p, 10, 64); err == nil || len(p) >= 36 {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func arenaDiscoveryObservedURI(uri string) bool {
	if arenaDiscoveryName.MatchString(uri) {
		return true
	}
	for _, prefix := range []string{"/lol-end-of-game/", "/lol-pre-end-of-game/", "/lol-ranked/", "/lol-progression/", "/lol-challenges/"} {
		if strings.HasPrefix(uri, prefix) {
			return true
		}
	}
	return false
}

// PreEndOfGame is part of the observation, not a flush boundary.
func (a *app) observeArenaDiscoveryPhase(client *LCUClient, phase string) {
	if client == nil {
		return
	}
	a.observeArenaProgressionPhase(client, phase)
	s := client.arenaDiscoveryState()
	s.mu.Lock()
	previous := s.lastPhase
	s.lastPhase = phase
	next := phase == "ChampSelect" && s.started && previous != "" && previous != "ChampSelect"
	reset := phase == "ChampSelect" && s.emitted
	if next || reset {
		s.mu.Unlock()
		a.finishArenaFameDiscovery(s)
		client.mu.Lock()
		if client.arenaDiscovery == s {
			client.arenaDiscovery = &arenaFameDiscoveryState{lastPhase: phase}
		}
		client.mu.Unlock()
		return
	}
	if !s.started || s.emitted || s.postScheduled || (phase != "Lobby" && phase != "None") {
		s.mu.Unlock()
		return
	}
	s.postScheduled = true
	schedule := s.schedule
	if schedule == nil {
		schedule = func(delay time.Duration, fn func()) func() {
			timer := time.AfterFunc(delay, fn)
			return func() { timer.Stop() }
		}
	}
	s.stops = append(s.stops, schedule(20*time.Second, func() { a.probeArenaFamePostGame(client, s) }), schedule(120*time.Second, func() { a.finishArenaFameDiscovery(s) }))
	s.mu.Unlock()
}
func (a *app) probeArenaFamePostGame(client *LCUClient, s *arenaFameDiscoveryState) {
	s.mu.Lock()
	if s.emitted {
		s.mu.Unlock()
		return
	}
	done := s.done
	s.mu.Unlock()
	client.mu.RLock()
	session := client.sessionContext
	client.mu.RUnlock()
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
	s.mu.Lock()
	ops := append([]lcuHelpOperation(nil), s.operations...)
	baseline := s.baseline
	s.mu.Unlock()
	a.mu.RLock()
	self := a.summoner
	a.mu.RUnlock()
	for _, path := range []string{"/lol-end-of-game/v1/eog-stats-block", "/lol-pre-end-of-game/v1/currentSequenceEvent"} {
		op := lcuHelpOperation{Method: http.MethodGet, Path: path}
		if !slices.Contains(ops, op) {
			ops = append(ops, op)
		}
	}
	rows := []map[string]any{}
	for _, op := range ops {
		if op.Method != http.MethodGet || arenaProgressionKind(op) != "" {
			continue
		}
		path := arenaDiscoveryGETPath(op.Path, self)
		if path == "" {
			continue
		}
		probe, stop := context.WithTimeout(ctx, 2*time.Second)
		var raw json.RawMessage
		var status atomic.Int64
		_ = client.RequestJSON(context.WithValue(probe, lcuResponseStatusKey{}, &status), http.MethodGet, path, nil, &raw)
		stop()
		keys, numbers := arenaDiscoveryShape(raw)
		difference := map[string]float64{}
		for key, value := range numbers {
			if before, ok := baseline[op.Path][key]; ok {
				difference[key] = value - before
			}
		}
		row := map[string]any{"path": arenaDiscoveryHelpPath(op.Path), "method": op.Method, "status": int(status.Load()), "keys": keys, "numbers": numbers, "difference": difference}
		rows = append(rows, row)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emitted || s.event == nil {
		return
	}
	s.event["post_game"] = rows
}

// Full help uses both arrays and name-keyed maps for arguments. Unknown nonempty
// parameter shapes are unresolved; no query/body argument is guessed.
func arenaHelpRequiredOutsidePath(value any, path string) bool {
	switch n := value.(type) {
	case nil:
		return false
	case []any:
		for _, v := range n {
			if arenaHelpRequiredOutsidePath(v, path) {
				return true
			}
		}
		return false
	case map[string]any:
		if len(n) == 0 {
			return false
		}
		if name := firstString(n, "name"); name != "" {
			optional, _ := n["optional"].(bool)
			required, _ := n["required"].(bool)
			return (required || !optional) && !strings.Contains(path, "{"+name+"}")
		}
		for key, v := range n {
			if arg, ok := v.(map[string]any); ok {
				copy := map[string]any{"name": key}
				for k, x := range arg {
					copy[k] = x
				}
				if arenaHelpRequiredOutsidePath(copy, path) {
					return true
				}
			} else {
				return true
			}
		}
		return false
	default:
		return true
	}
}
