package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
)

// Names and encoded lengths only. Values, identifiers and bodies never leave
// this per-request counter. Count every frame to expose repeated stream data.
type overviewResponseShape struct {
	account string

	Bytes, Frames, Matches, Participants int
	Top, Match, Participant, Nested      map[string]int
	FrameKinds                           map[string]int
	HasTimeline, HasRaw                  bool
	EmptyRevision                        bool
}

func newOverviewResponseShape() *overviewResponseShape {
	return &overviewResponseShape{Top: map[string]int{}, Match: map[string]int{}, Participant: map[string]int{}, Nested: map[string]int{}, FrameKinds: map[string]int{}}
}

var overviewShapeFields = func() map[string]bool {
	fields := map[string]bool{"type": true, "overview": true, "timeline": true, "frames": true, "events": true, "raw": true, "rawMatch": true, "rawResponse": true, "info": true, "metadata": true, "proMismatch": true}
	var visit func(reflect.Type)
	visited := map[reflect.Type]bool{}
	visit = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || visited[t] {
			return
		}
		visited[t] = true
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = true
			}
			visit(f.Type)
		}
	}
	visit(reflect.TypeOf(gameplayOverview{}))
	visit(reflect.TypeOf(riotMatch{}))
	return fields
}()

func shapeFieldName(name string) string {
	if overviewShapeFields[name] {
		return name
	}
	return "other"
}
func (s *overviewResponseShape) observe(data []byte) {
	s.Bytes += len(data)
	for _, raw := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		var top map[string]json.RawMessage
		if json.Unmarshal(raw, &top) != nil {
			continue
		}
		kind := "complete"
		if value := top["type"]; value != nil {
			var input string
			_ = json.Unmarshal(value, &input)
			switch input {
			case "cards", "progress", "complete", "error":
				kind = input
			default:
				kind = "other"
			}
		}
		if overview := top["overview"]; overview != nil {
			var body map[string]json.RawMessage
			_ = json.Unmarshal(overview, &body)
			top = body
		}
		var player struct {
			Ref    string `json:"playerRef"`
			Region string `json:"region"`
			Server string `json:"serverId"`
		}
		if json.Unmarshal(top["player"], &player) == nil && player.Ref != "" {
			s.account = player.Region + ":" + player.Server + ":" + player.Ref
		}
		s.Frames++
		s.FrameKinds[kind]++
		for key, value := range top {
			s.Top[shapeFieldName(key)] += len(value)
		}
		var matches []map[string]json.RawMessage
		_ = json.Unmarshal(top["matches"], &matches)
		s.Matches += len(matches)
		for _, match := range matches {
			if len(match) > 2 && (len(match["revision"]) == 0 || string(match["revision"]) == `""`) {
				s.EmptyRevision = true
			}
			for key, value := range match {
				s.Match[shapeFieldName(key)] += len(value)
			}
			var players []map[string]json.RawMessage
			_ = json.Unmarshal(match["participants"], &players)
			s.Participants += len(players)
			for _, player := range players {
				for key, value := range player {
					s.Participant[shapeFieldName(key)] += len(value)
				}
			}
		}
		s.observeNested(top, 0)
	}
}
func (s *overviewResponseShape) observeNested(object map[string]json.RawMessage, depth int) {
	if depth > 6 {
		return
	}
	for key, value := range object {
		switch key {
		case "timeline", "frames", "events":
			s.HasTimeline = true
			s.Nested[key] += len(value)
		case "raw", "rawMatch", "rawResponse", "info", "metadata":
			s.HasRaw = true
			s.Nested[key] += len(value)
		case "participants", "score", "parts", "perks", "perkStats", "teams", "masteries", "rankedQueues":
			s.Nested[key] += len(value)
		}
		var child map[string]json.RawMessage
		if json.Unmarshal(value, &child) == nil && child != nil {
			s.observeNested(child, depth+1)
			continue
		}
		var rows []map[string]json.RawMessage
		if json.Unmarshal(value, &rows) == nil {
			for _, row := range rows {
				s.observeNested(row, depth+1)
			}
		}
	}
}
func (s *overviewResponseShape) event() map[string]any {
	return map[string]any{"event": "overview_response_shape", "response_bytes": s.Bytes, "frames": s.Frames, "frames_by_type": s.FrameKinds, "match_occurrences": s.Matches, "participant_occurrences": s.Participants, "top_level_bytes": s.Top, "match_field_bytes": s.Match, "participant_field_bytes": s.Participant, "nested_bytes": s.Nested, "has_timeline": s.HasTimeline, "has_raw_response": s.HasRaw}
}

type overviewShapeWriter struct {
	http.ResponseWriter
	shape *overviewResponseShape
}

func (w *overviewShapeWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.shape.observe(data[:n])
	}
	return n, err
}
func (w *overviewShapeWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (w *overviewShapeWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// The shared flight retains complete snapshots; only wire frames are deltas.
// This avoids retransmitting N full participants after every single detail.
func compactOverviewProgress(partial gameplayOverview, seen map[int64]string) gameplayOverview {
	if seen == nil {
		return partial
	}
	delta := make([]gameplayMatch, 0)
	for _, match := range partial.Matches {
		if revision, ok := seen[match.GameID]; !ok || revision != match.Revision {
			delta = append(delta, match)
			seen[match.GameID] = match.Revision
		}
	}
	partial.Matches = delta
	partial.MatchesDelta = true
	partial.Pagination.Partial = true
	return partial
}
