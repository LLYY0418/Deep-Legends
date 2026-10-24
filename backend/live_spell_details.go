package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Detail payloads are chunked JSON, not truncated text. Consumers concatenate
// payload in part_index order for one capture/epoch/observation, then decode it.
// Player identities and unrelated free-form string values never enter payloads.
const (
	liveSpellDetailChunkBytes = 1024
	liveSpellDetailMaxFields  = 1024
	liveSpellDetailMaxDepth   = 16
	liveSpellDetailMaxArray   = 64
	liveSpellDetailMaxText    = 64 << 10
)

type liveSpellDetailState struct {
	fields      map[string]map[string]any
	limits      []string
	sample      int
	observation int
}

func liveSpellSensitiveKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
	for _, part := range []string{"puuid", "accountid", "accounthash", "summonerid", "riotid", "summonername", "playername", "castername", "killername", "victimname", "authorization", "password", "token", "secret"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return key == "gamename" || key == "tagline" || key == "namevisibilitytype"
}

func (p *liveSpellProbe) rememberPrivateFields(object map[string]any) {
	for key, value := range object {
		if !liveSpellSensitiveKey(key) && !strings.EqualFold(key, "displayName") {
			continue
		}
		if text, ok := value.(string); ok && text != "" && len(text) <= 1024 {
			p.privateTerms[text] = true
			if strings.Contains(strings.ToLower(key), "riotid") {
				if i := strings.LastIndexByte(text, '#'); i > 0 {
					p.privateTerms[text[:i]] = true
				}
			}
		}
	}
}

func (p *liveSpellProbe) detailText(text string) (string, bool) {
	terms := make([]string, 0, len(p.privateTerms))
	for term := range p.privateTerms {
		terms = append(terms, term)
	}
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	cleaned := text
	for _, term := range terms {
		cleaned = strings.ReplaceAll(cleaned, term, "[redacted]")
	}
	return cleaned, cleaned != text
}

func (p *liveSpellProbe) detailKey(key string) (string, bool) {
	safe := len(key) > 0 && len(key) <= 80
	if safe && !(key[0] >= 'a' && key[0] <= 'z' || key[0] >= 'A' && key[0] <= 'Z' || key[0] == '_') {
		safe = false
	}
	compact := strings.ReplaceAll(key, "-", "")
	if len(compact) == 32 {
		if _, err := hex.DecodeString(compact); err == nil {
			safe = false
		}
	}
	for _, r := range key {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			safe = false
			break
		}
	}
	_, redacted := p.detailText(key)
	if safe && !redacted {
		return key, false
	}
	digest := sha256.Sum256([]byte(p.capture + "\x00" + key))
	return "[key-" + hex.EncodeToString(digest[:8]) + "]", true
}

func liveSpellStatePath(path string) bool {
	path = strings.ToLower(path)
	for _, part := range []string{"spell", "ability", "cooldown", "recharge", "remaining", "ready", "available", "cast", "used", "charge", "ammo", "haste", "timer", "timeleft", "duration", "expire", "visible", "visibility", "fog", "isdead"} {
		if strings.Contains(path, part) {
			return true
		}
	}
	return false
}

func (p *liveSpellProbe) detailFields(source string, object map[string]any) (map[string]map[string]any, []string) {
	fields := map[string]map[string]any{}
	limits := []string{}
	var walk func(any, string, int, bool)
	walk = func(value any, path string, depth int, private bool) {
		if len(fields) >= liveSpellDetailMaxFields {
			if len(limits) == 0 || limits[len(limits)-1] != "field-count" {
				limits = append(limits, "field-count")
			}
			return
		}
		field := map[string]any{"type": liveSpellValueType(value)}
		fields[path] = field
		if private {
			field["redacted"] = true
			return
		}
		if depth > liveSpellDetailMaxDepth {
			field["coverage_limited"] = true
			limits = append(limits, "depth:"+path)
			return
		}
		switch v := value.(type) {
		case map[string]any:
			field["field_count"] = len(v)
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Slice(keys, func(i, j int) bool {
				priority := func(key string) bool {
					return key == "summonerSpells" || key == "rawDisplayName" || key == "displayName" || key == "rawDescription"
				}
				if priority(keys[i]) != priority(keys[j]) {
					return priority(keys[i])
				}
				return keys[i] < keys[j]
			})
			for _, key := range keys {
				if len(fields) >= liveSpellDetailMaxFields {
					limits = append(limits, "field-count")
					break
				}
				safe, hidden := p.detailKey(key)
				walk(v[key], path+"/"+safe, depth+1, liveSpellSensitiveKey(key))
				if hidden {
					fields[path+"/"+safe]["key_redacted"] = true
				}
			}
		case []any:
			field["length"] = len(v)
			size := len(v)
			if size > liveSpellDetailMaxArray {
				size = liveSpellDetailMaxArray
				field["coverage_limited"] = true
				limits = append(limits, "array:"+path)
			}
			for i := 0; i < size; i++ {
				if len(fields) >= liveSpellDetailMaxFields {
					limits = append(limits, "field-count")
					break
				}
				walk(v[i], path+"/"+strconv.Itoa(i), depth+1, false)
			}
		case string:
			// Names/descriptions and unknown string fields inside skill objects are
			// necessary for discovering aliases. Other free-form player strings remain
			// schema-only; state candidates also retain their original string type/value.
			raw := strings.HasPrefix(path, "/summonerSpells/") || source == "eventdata" && path == "/EventName"
			candidate := liveSpellStatePath(path)
			if !raw && !candidate {
				return
			}
			cleaned, redacted := p.detailText(v)
			if redacted {
				field["redacted"] = true
			}
			if raw || candidate {
				if len(cleaned) > liveSpellDetailMaxText {
					digest := sha256.Sum256([]byte(p.capture + "\x00" + cleaned))
					field["value_digest"] = hex.EncodeToString(digest[:])
					field["original_bytes"] = len(cleaned)
					field["coverage_limited"] = true
					limits = append(limits, "text:"+path)
					cleaned = liveSpellUTF8Prefix(cleaned, liveSpellDetailMaxText)
				}
				field["value"] = cleaned
			}
		case float64, bool:
			if source == "eventdata" || liveSpellStatePath(path) {
				field["value"] = v
			}
		}
	}
	walk(object, "", 0, false)
	return fields, limits
}

func liveSpellUTF8Prefix(text string, size int) string {
	if len(text) <= size {
		return text
	}
	for size > 0 && !utf8.RuneStart(text[size]) {
		size--
	}
	return text[:size]
}

func (p *liveSpellProbe) detailRows(source, subject string, object map[string]any, once bool) []map[string]any {
	fields, limits := p.detailFields(source, object)
	key := source + ":" + subject
	before, exists := p.detailPrevious[key]
	// Compare all captured paths, including strings, missing values and limits.
	encoded, _ := json.Marshal([]any{fields, limits})
	old, _ := json.Marshal([]any{before.fields, before.limits})
	if exists && string(encoded) == string(old) {
		return nil
	}
	p.observation++
	payload := map[string]any{"source": source, "subject": subject, "fields": fields, "limits": limits}
	kind := "snapshot"
	if exists {
		kind = "change"
		changes := []map[string]any{}
		paths := map[string]bool{}
		for path := range fields {
			paths[path] = true
		}
		for path := range before.fields {
			paths[path] = true
		}
		sorted := make([]string, 0, len(paths))
		for path := range paths {
			sorted = append(sorted, path)
		}
		sort.Strings(sorted)
		for _, path := range sorted {
			next, nextOK := fields[path]
			prior, priorOK := before.fields[path]
			a, _ := json.Marshal(next)
			b, _ := json.Marshal(prior)
			if nextOK == priorOK && string(a) == string(b) {
				continue
			}
			changes = append(changes, map[string]any{"path": path, "before_present": priorOK, "after_present": nextOK, "before": prior, "after": next})
		}
		payload = map[string]any{"source": source, "subject": subject, "previous_observation": before.observation, "previous_sample": before.sample, "changes": changes, "limits": limits}
	}
	data, _ := json.Marshal(payload)
	payloadBytes := len(data)
	payloadDigest := sha256.Sum256(data)
	chunks := []string{}
	for len(data) > 0 {
		part := liveSpellUTF8Prefix(string(data), liveSpellDetailChunkBytes)
		chunks = append(chunks, part)
		data = data[len(part):]
	}
	rows := make([]map[string]any, 0, len(chunks))
	for i, chunk := range chunks {
		row := p.row(kind)
		row["source"], row["subject"], row["observation"] = source, subject, p.observation
		row["previous_observation"] = before.observation
		row["part_index"], row["part_count"], row["payload_encoding"], row["payload"] = i+1, len(chunks), "json", chunk
		row["coverage_limited"] = len(limits) > 0
		row["payload_bytes"], row["payload_sha256"] = payloadBytes, hex.EncodeToString(payloadDigest[:])
		if strings.HasPrefix(subject, "player:") {
			row["player_slot"], _ = strconv.Atoi(strings.TrimPrefix(subject, "player:"))
		}
		rows = append(rows, row)
	}
	if !once {
		p.detailPrevious[key] = liveSpellDetailState{fields: fields, limits: limits, sample: p.sample, observation: p.observation}
	}
	return rows
}

func (p *liveSpellProbe) rejectedEventRows(index int, event map[string]any) []map[string]any {
	data, _ := json.Marshal(event)
	digest := sha256.Sum256(append([]byte(p.capture+"\x00"), data...))
	if p.rejectedEvents[digest] {
		return nil
	}
	if len(p.rejectedEvents) >= 128 {
		if p.rejectedLimitReported {
			return nil
		}
		p.rejectedLimitReported = true
		row := p.row("limit")
		row["reason"], row["limit"] = "rejected-event-details", 128
		return []map[string]any{row}
	}
	p.rejectedEvents[digest] = true
	row := p.row("event_rejected")
	row["reason"], row["event_index"] = "invalid-event-id", index
	rows := []map[string]any{row}
	return append(rows, p.detailRows("eventdata", "unindexed:"+hex.EncodeToString(digest[:8]), event, true)...)
}
