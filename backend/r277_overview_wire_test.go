package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestR277OverviewWireSingleFullRecordAndLazyScores(t *testing.T) {
	fixture, err := os.ReadFile("testdata/r272-synthetic-match.json")
	if err != nil {
		t.Fatal(err)
	}
	f := r98OverviewFixture(t, false)
	cp := f.a.riot.champions
	original := cp.client.Transport
	cp.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/lol/match/v5/matches/KR_") && !strings.HasSuffix(r.URL.Path, "/timeline") {
			var body map[string]any
			if err := json.Unmarshal(fixture, &body); err != nil {
				return nil, err
			}
			id, _ := strconv.Atoi(r.URL.Path[strings.LastIndex(r.URL.Path, "_")+1:])
			body["metadata"].(map[string]any)["matchId"] = fmt.Sprintf("KR_%d", id)
			info := body["info"].(map[string]any)
			info["gameId"] = id
			info["participants"].([]any)[0].(map[string]any)["puuid"] = "r98-subject"
			raw, _ := json.Marshal(body)
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
		}
		return original.RoundTrip(r)
	})
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/gameplay/overview", strings.NewReader(`{"gameName":"Fixture","tagLine":"KR1","region":"kr","count":20}`))
			if stream {
				r.Header.Set("Accept", "application/x-ndjson")
				r.Header.Set("X-Overview-Cards", "1")
			}
			f.a.handleGameplayOverview(w, r)
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			full := map[int64]int{}
			rankFrames := 0
			parts := 0
			complete := false
			for _, line := range bytes.Split(w.Body.Bytes(), []byte{'\n'}) {
				var frame map[string]json.RawMessage
				if json.Unmarshal(line, &frame) != nil {
					continue
				}
				var kind string
				json.Unmarshal(frame["type"], &kind)
				payload := frame
				if overview := frame["overview"]; overview != nil {
					json.Unmarshal(overview, &payload)
				}
				if ranked := payload["rankedQueues"]; len(ranked) > 0 && string(ranked) != "null" {
					rankFrames++
				}
				var matches []map[string]json.RawMessage
				json.Unmarshal(payload["matches"], &matches)
				for _, m := range matches {
					var id int64
					json.Unmarshal(m["gameId"], &id)
					var ps []map[string]json.RawMessage
					json.Unmarshal(m["participants"], &ps)
					if len(ps) > 0 {
						full[id]++
					}
					for _, p := range ps {
						var score map[string]json.RawMessage
						json.Unmarshal(p["score"], &score)
						if _, ok := score["parts"]; ok {
							parts++
						}
					}
					if kind == "complete" && len(ps) == 0 && len(m) != 2 {
						t.Errorf("complete reference contains extra fields: %v", m)
					}
				}
				if kind == "complete" {
					complete = true
				}
			}
			if stream && !complete {
				t.Error("missing complete frame")
			}
			if len(full) != 20 {
				t.Errorf("full records %d !=20", len(full))
			}
			for id, count := range full {
				if count != 1 {
					t.Errorf("game %d sent full %d times", id, count)
				}
			}
			if parts != 0 {
				t.Errorf("list contains %d score.parts fields", parts)
			}
			if rankFrames != 1 {
				t.Errorf("rankedQueues sent %d times", rankFrames)
			}
		})
	}
}
