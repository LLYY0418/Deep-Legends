package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type LCUEvent struct {
	Data      json.RawMessage `json:"data"`
	EventType string          `json:"eventType"`
	URI       string          `json:"uri"`
}

type lcuEventURIStat struct {
	URI       string `json:"uri"`
	LastBytes int    `json:"last_bytes"`
	Count     int    `json:"count"`
}

type lcuEventStreamError struct {
	Err     error
	TopURIs []lcuEventURIStat
}

func (e *lcuEventStreamError) Error() string { return e.Err.Error() }
func (e *lcuEventStreamError) Unwrap() error { return e.Err }

func topLCUEventURIStats(stats map[string]lcuEventURIStat, limit int) []lcuEventURIStat {
	result := make([]lcuEventURIStat, 0, len(stats))
	for _, stat := range stats {
		result = append(result, stat)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].LastBytes != result[j].LastBytes {
			return result[i].LastBytes > result[j].LastBytes
		}
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].URI < result[j].URI
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func (c *LCUClient) ListenEvents(ctx context.Context, onReady func(), onEvent func(LCUEvent)) error {
	token, ok := c.credentials()
	if !ok {
		return errors.New("LCU client credentials are no longer available")
	}
	header := http.Header{}
	header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("riot:"+token)))
	dialer := websocket.Dialer{
		HandshakeTimeout: 8 * time.Second,
		Proxy:            nil,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // LCU is a self-signed TLS service on loopback.
			MinVersion:         tls.VersionTLS12,
		},
	}
	conn, response, err := dialer.DialContext(ctx, fmt.Sprintf("wss://127.0.0.1:%d/", c.port), header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("LCU event stream unavailable: %w", err)
	}
	defer conn.Close()
	conn.SetReadLimit(8 * 1024 * 1024)
	if err := conn.WriteJSON([]any{5, "OnJsonApiEvent"}); err != nil {
		return fmt.Errorf("LCU event subscription failed: %w", err)
	}
	if onReady != nil {
		onReady()
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	uriStats := make(map[string]lcuEventURIStat)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return &lcuEventStreamError{Err: fmt.Errorf("LCU event stream closed: %w", err), TopURIs: topLCUEventURIStats(uriStats, 3)}
		}
		var envelope []json.RawMessage
		if err := json.Unmarshal(data, &envelope); err != nil || len(envelope) < 3 {
			continue
		}
		var opcode int
		var topic string
		if json.Unmarshal(envelope[0], &opcode) != nil || opcode != 8 || json.Unmarshal(envelope[1], &topic) != nil || topic != "OnJsonApiEvent" {
			continue
		}
		var event LCUEvent
		if json.Unmarshal(envelope[2], &event) == nil && event.URI != "" {
			stat := uriStats[event.URI]
			stat.URI = event.URI
			stat.LastBytes = len(data)
			stat.Count++
			uriStats[event.URI] = stat
			if onEvent != nil {
				onEvent(event)
			}
		}
	}
}

func shouldRefreshForLCUEvent(event LCUEvent) bool {
	return lcuEventRefreshScope(event) != ""
}

// URI services and prefixes are ASCII. Fold only the compared bytes; the usual
// lowercase path needs neither a copy nor a scan of an unrelated suffix. Keep
// the old Unicode ToLower semantics on the rare non-ASCII comparison path.
func lcuPrefixFold(uri, prefix string) bool {
	if strings.HasPrefix(uri, prefix) {
		return true
	}
	for i := 0; i < len(prefix); i++ {
		if i >= len(uri) {
			return false
		}
		c := uri[i]
		if c >= 0x80 {
			return strings.HasPrefix(strings.ToLower(uri), prefix)
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[i] {
			return false
		}
	}
	return true
}

func lcuEventRefreshScope(event LCUEvent) string {
	uri := event.URI
	if len(uri) == 0 || uri[0] != '/' {
		return ""
	}
	end := strings.IndexByte(uri[1:], '/')
	if end < 0 {
		return ""
	}
	service := uri[:end+2]
	// Fold a bounded first segment on the stack, instead of lowercasing the URI.
	var folded [64]byte
	if len(service) > len(folded) {
		return ""
	}
	ascii := true
	for i := range len(service) {
		c := service[i]
		if c >= 0x80 {
			ascii = false
			break
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		folded[i] = c
	}
	if ascii {
		service = string(folded[:len(service)])
	} else {
		service = strings.ToLower(service)
	}
	switch service {
	case "/lol-champ-select/":
		if lcuPrefixFold(uri, "/lol-champ-select/v1/session") {
			return "champselect"
		}
	case "/lol-champions/":
		if lcuPrefixFold(uri, "/lol-champions/v1/inventories/") {
			return "collection"
		}
	case "/lol-champion-mastery/", "/lol-inventory/":
		return "collection"
	case "/lol-summoner/":
		if lcuPrefixFold(uri, "/lol-summoner/v1/current-summoner/summoner-profile") {
			return "summoner-profile"
		}
		if lcuPrefixFold(uri, "/lol-summoner/v1/current-summoner") {
			return "summoner"
		}
	case "/lol-loot/":
		if lcuPrefixFold(uri, "/lol-loot/v1/player-loot-map") {
			return "account"
		}
	case "/lol-rewards/":
		if lcuPrefixFold(uri, "/lol-rewards/v1/grants") {
			return "account"
		}
	}
	return ""
}
