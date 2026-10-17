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
}

var arenaDiscoveryName = regexp.MustCompile(`(?i)cherry|arena|fame|gladiator|progression`)
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
	if c == nil || !arenaDiscoveryName.MatchString(event.URI) {
		return
	}
	s := c.arenaDiscoveryState()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || len(s.uris) >= 32 {
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
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	ops := []lcuHelpOperation{}
	var walk func(any, string)
	walk = func(v any, key string) {
		switch node := v.(type) {
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
					if params, ok := node[label].([]any); ok {
						for _, param := range params {
							if arg, ok := param.(map[string]any); ok {
								optional, _ := arg["optional"].(bool)
								required, _ := arg["required"].(bool)
								name := firstString(arg, "name")
								if (required || !optional) && !strings.Contains(path, "{"+name+"}") {
									method = "GET_REQUIRES_PARAMETERS"
								}
							}
						}
					}
				}
				op := lcuHelpOperation{Method: method, Path: path}
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
		_ = client.RequestJSON(context.WithValue(ctx, lcuResponseStatusKey{}, &helpStatus), http.MethodGet, "/help", nil, &help)
		event["help_status"] = int(helpStatus.Load())
		results := []map[string]any{}
		for _, op := range arenaDiscoveryHelp(help) {
			row := map[string]any{}
			if op.Path != "" {
				row["path"] = arenaDiscoveryHelpPath(op.Path)
			} else {
				row["name"] = op.Name
			}
			row["method"] = op.Method
			if op.Method == http.MethodGet {
				if path := arenaDiscoveryGETPath(op.Path, self); path != "" {
					var raw json.RawMessage
					probe, cancel := context.WithTimeout(ctx, 2*time.Second)
					var status atomic.Int64
					_ = client.RequestJSON(context.WithValue(probe, lcuResponseStatusKey{}, &status), http.MethodGet, path, nil, &raw)
					cancel()
					row["status"] = int(status.Load())
					keys, numbers := arenaDiscoveryShape(raw)
					row["keys"], row["numbers"] = keys, numbers
				}
			}
			event["interfaces"] = append(results, row)
			encoded, _ := json.Marshal(event)
			if len(encoded) > 6500 {
				event["truncated"] = true
				break
			}
			results = append(results, row)
		}
		event["interfaces"] = results
		s.mu.Lock()
		event["queue_types"] = map[string][]string{"lcu": s.queues[dataSourceLCU], "sgp": s.queues[dataSourceSGP]}
		event["event_uris"] = append([]string(nil), s.uris...)
		s.mu.Unlock()
		// Leave headroom for recordDiagnostic's common envelope.
		if encoded, _ := json.Marshal(event); len(encoded) > 7800 {
			event["event_uris"] = []string{}
			event["truncated"] = true
		}
		s.mu.Lock()
		s.event = event
		flush := s.flushRequested
		s.mu.Unlock()
		if flush {
			a.finishArenaFameDiscovery(s)
		}
	})
}

// Emit one aggregate when Arena ends (or the connection closes), so WebSocket
// names observed after the help probes are included in the same bounded event.
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
	event := s.event
	event["event_uris"] = append([]string(nil), s.uris...)
	s.mu.Unlock()
	for {
		raw, _ := json.Marshal(event)
		if len(raw) <= 7200 {
			break
		}
		rows, _ := event["interfaces"].([]map[string]any)
		if len(rows) > 0 {
			event["interfaces"] = rows[:len(rows)-1]
		} else {
			event["queue_types"] = map[string][]string{}
			event["event_uris"] = []string{}
		}
		event["truncated"] = true
	}
	a.recordDiagnostic(event)
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
