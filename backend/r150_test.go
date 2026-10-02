package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestR150NormalizeChampionGrade(t *testing.T) {
	tests := []struct {
		source string
		tier   int
		raw    string
		want   string
	}{
		{"yourgg", 0, "OP", "OP"}, {"yourgg", 0, "S", "S"}, {"yourgg", 0, "A", "A"},
		{"yourgg", 0, "B", "B"}, {"yourgg", 0, "C", "C"}, {"yourgg", 0, "D", "D"}, {"yourgg", 0, "F", "F"},
		{"yourgg", 0, "", ""}, {"yourgg", 0, "E", ""},
		{"opgg", 0, "", "OP"}, {"opgg", 1, "", "S"}, {"opgg", 2, "", "A"},
		{"opgg", 3, "", "B"}, {"opgg", 4, "", "C"}, {"opgg", 5, "", "D"},
		{"opgg", -1, "", ""}, {"opgg", 6, "", ""},
		{"hexdata-hero", 1, "", "S"}, {"hexdata-hero", 2, "", "A"}, {"hexdata-hero", 3, "", "B"},
		{"hexdata-hero", 4, "", "C"}, {"hexdata-hero", 5, "", "F"},
		{"hexdata-hero", 0, "", ""}, {"hexdata-hero", 6, "", ""},
		{"hexdata-augment", 0, "hang", "S"}, {"hexdata-augment", 0, "top", "A"},
		{"hexdata-augment", 0, "elite", "B"}, {"hexdata-augment", 0, "npc", "C"},
		{"hexdata-augment", 0, "trap", "F"}, {"hexdata-augment", 0, "insufficient", ""},
		{"hexdata-augment", 0, "", ""}, {"hexdata-augment", 0, "unknown", ""},
		{"local", 0, "S", "S"}, {"local", 0, "A", "A"}, {"local", 0, "B", "B"}, {"local", 0, "OP", ""},
		{"opgg-augment-catalog", 0, "", "S"}, {"opgg-augment-catalog", 1, "", "A"},
		{"opgg-augment-catalog", 2, "", "B"}, {"opgg-augment-catalog", 3, "", "C"},
		{"opgg-augment-catalog", 4, "", "D"}, {"opgg-augment-catalog", 5, "", "F"},
		{"opgg-augment-catalog", -1, "", ""}, {"opgg-augment-catalog", 6, "", ""},
	}
	for _, tt := range tests {
		if got := normalizeChampionGrade(tt.source, tt.tier, tt.raw); got != tt.want {
			t.Errorf("%s/%d/%q = %q, want %q", tt.source, tt.tier, tt.raw, got, tt.want)
		}
	}
}

func TestR150YourGGOPAndSKeepDistinctGradesAndOrder(t *testing.T) {
	data := []byte(`{"response":{"champions":[{"championId":3,"matches":3288,"tier":"OP","score":98.5,"winRate":0.5596,"banRate":0.1045,"averagePlacement":3.26,"firstPlacementRate":0.22},{"championId":4,"matches":3000,"tier":"S","score":95,"winRate":0.54,"banRate":0.09,"averagePlacement":3.3,"firstPlacementRate":0.2}],"totalMatches":10000,"version":"16.19"},"statusCode":200,"success":true}`)
	got, err := parseYourGGArenaRankings(data, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 2 || got.Rows[0].Rank != 1 || got.Rows[0].Grade != "OP" || got.Rows[1].Rank != 2 || got.Rows[1].Grade != "S" || got.Rows[0].Tier != got.Rows[1].Tier {
		t.Fatalf("OP/S list mapping = %+v", got.Rows)
	}
}

func TestR150LocalGradeNeverBecomesDisplayedScore(t *testing.T) {
	rows := []championMetricRow{{Games: 1000, WinRate: 60}, {Games: 1000, WinRate: 50}, {HexTier: "top", HexLabel: "顶级", Score: 88}}
	applyLocalAugmentGrades(rows)
	if rows[0].Score != 0 || rows[1].Score != 0 || rows[0].Grade == "" || rows[1].Grade == "" || rows[2].Score != 88 || rows[2].Grade != "A" {
		t.Fatalf("local/official grade and score = %+v", rows)
	}
	groups := []arenaAugmentGroup{{Rows: []championMetricRow{{Games: 1000, WinRate: 60}, {Games: 1000, WinRate: 50}}}}
	applyLocalArenaAugmentGrades(groups)
	for _, row := range groups[0].Rows {
		if row.Score != 0 || row.Grade == "" {
			t.Fatalf("Arena fallback leaked local score: %+v", row)
		}
	}
}

func TestR150ArenaHeaderDiagnosticOnlyRecordsListMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	request := `{"event":"arena_header_source","reason":"rendered","championId":3,"rank":1,"grade":"OP","listGames":3288,"hasListRow":true}`
	recorder := httptest.NewRecorder()
	a.handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(request)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("diagnostic status = %d: %s", recorder.Code, recorder.Body.String())
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"event": "arena_header_source", "championId": float64(3), "rank": float64(1), "grade": "OP", "listGames": float64(3288), "hasListRow": true} {
		if event[key] != want {
			t.Fatalf("%s = %v, want %v", key, event[key], want)
		}
	}
	for _, forbidden := range []string{"cookie", "token", "requestBody", "pickRate", "winRate"} {
		if _, ok := event[forbidden]; ok {
			t.Fatalf("unexpected diagnostic field %q: %+v", forbidden, event)
		}
	}
}

func TestR150ArenaDetailResponseOmitsRemovedMixedSourceStats(t *testing.T) {
	data, err := json.Marshal(championDetailResponse{Mode: "arena"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"arenaStats"`) {
		t.Fatalf("Arena detail still exposes OP.GG hero stats: %s", data)
	}
}
