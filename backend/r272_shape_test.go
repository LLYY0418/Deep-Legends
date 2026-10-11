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

// Entire fixture is synthetic, generated from the public Riot Match-V5 DTO
// fields (developer.riotgames.com/api-details/match-v5). Never captured data.
func TestR272SyntheticOverviewWireBudget(t *testing.T) {
	fixture, err := os.ReadFile("testdata/r272-synthetic-match.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture) < 77*1024 || len(fixture) > 79*1024 {
		t.Fatalf("synthetic raw match size=%d, need77–79KiB", len(fixture))
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
			players := info["participants"].([]any)
			players[0].(map[string]any)["puuid"] = "r98-subject"
			raw, _ := json.Marshal(body)
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
		}
		return original.RoundTrip(r)
	})
	results := map[string]any{"fixture": "synthetic_public_match_v5", "raw_match_bytes": len(fixture), "not_real_kr": true}
	for _, test := range []struct {
		name          string
		start, budget int
		stream        bool
	}{{"first", 0, 1500000, true}, {"append", 20, 600000, false}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/gameplay/overview", strings.NewReader(fmt.Sprintf(`{"gameName":"Fixture","tagLine":"KR1","region":"kr","begIndex":%d,"count":20}`, test.start)))
		if test.stream {
			r.Header.Set("Accept", "application/x-ndjson")
			r.Header.Set("X-Overview-Cards", "1")
		}
		f.a.handleGameplayOverview(w, r)
		if w.Code != 200 {
			t.Fatalf("%s status=%d body=%s", test.name, w.Code, w.Body.String())
		}
		if w.Body.Len() > test.budget {
			t.Errorf("%s wire=%d > budget=%d", test.name, w.Body.Len(), test.budget)
		}
		if test.name == "first" && os.Getenv("R272_OVERVIEW_FIXTURE") != "" {
			for _, line := range bytes.Split(w.Body.Bytes(), []byte{'\n'}) {
				var frame map[string]json.RawMessage
				_ = json.Unmarshal(line, &frame)
				if string(frame["type"]) == `"complete"` {
					if err := os.WriteFile(os.Getenv("R272_OVERVIEW_FIXTURE"), frame["overview"], 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		shape := newOverviewResponseShape()
		shape.observe(w.Body.Bytes())
		results[test.name] = shape.event()
		if shape.Frames == 0 || shape.Matches < 20 || shape.Participants < 200 {
			t.Errorf("shape missing actual serializer fields=%+v", shape)
		}
		if shape.HasRaw || shape.HasTimeline {
			t.Error("untrimmed raw/timeline on overview wire")
		}
	}
	raw, _ := json.MarshalIndent(results, "", "  ")
	t.Log(string(raw))
	if path := os.Getenv("R272_SHAPE_OUTPUT"); path != "" {
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestR272ShapePrivacyAndNestedCoverage(t *testing.T) {
	s := newOverviewResponseShape()
	s.observe([]byte(`{"matches":[{"gameId":123456789,"participants":[{"puuid":"SECRET-PUUID","gameName":"SECRET-NAME"}],"rawResponse":{"info":{"participants":[{"puuid":"SECRET"}]},"timeline":{"frames":[{"events":[{"name":"SECRET"}]}]}}}],"SECRETPLAYER": {"SECRETNAME":"SECRET"}}`))
	raw, _ := json.Marshal(s.event())
	for _, secret := range []string{"SECRET", "123456789"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("shape diagnostic leaked identity/body")
		}
	}
	if !s.HasRaw || !s.HasTimeline || s.Nested["participants"] == 0 || s.Nested["info"] == 0 {
		t.Fatal("diagnostic omitted suspected payload families")
	}
}
