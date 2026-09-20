package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestObjectiveDiagnosticsReadonlyContractAndStateTransition(t *testing.T) {
	var writes atomic.Int32
	before := `[{"id":"secret-mission-id","title":"private-title","puuid":"private-puuid","viewed":false,"isNew":true,"objectives":[{"progress":{"currentProgress":3,"lastViewedProgress":0,"totalCount":5}}]}]`
	after := strings.ReplaceAll(strings.ReplaceAll(before, `"viewed":false`, `"viewed":true`), `"lastViewedProgress":0`, `"lastViewedProgress":3`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes.Add(1)
			t.Errorf("unexpected write %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/lol-missions/v1/missions":
			w.Write([]byte(before))
		case "/lol-missions/v1/series", "/lol-missions/v1/data":
			w.Write([]byte(`{"unreadCount":7,"private":"secret-value"}`))
		case "/swagger/v3/openapi.json":
			http.NotFound(w, r)
		case "/swagger/v2/swagger.json":
			w.Write([]byte(`{"paths":{"/lol-missions/v1/player":{"put":{"parameters":[{"name":"body","required":true,"schema":{"$ref":"#/definitions/Viewed"}}]}}},"definitions":{"Viewed":{"type":"object","properties":{"missionIds":{"type":"array","items":{"type":"string"}},"seriesIds":{"type":"array","items":{"type":"string"}}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0755); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store}
	client := &LCUClient{baseURL: server.URL, token: "private-token", http: server.Client()}
	if !a.collectObjectiveDiagnostics(context.Background(), client, "manual") {
		t.Fatal("not collected")
	}
	a.recordObjectiveState(client, "/lol-missions/v1/missions", "websocket", "Update", []byte(after))
	count := client.objectiveDiagnostics.events
	a.recordObjectiveState(client, "/lol-missions/v1/missions", "websocket", "Update", []byte(after))
	if client.objectiveDiagnostics.events != count {
		t.Fatal("unchanged event was not deduplicated")
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-mission-id", "private-title", "private-puuid", "private-token", "secret-value"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	for _, evidence := range []string{"objective_badge_contracts", `"method":"PUT"`, `"missionIds"`, `"seriesIds"`, `.lastViewedProgress`, `"changed":true`, `"previous_digest"`, `"status":404`} {
		if !strings.Contains(string(data), evidence) {
			t.Fatalf("missing %s in %s", evidence, data)
		}
	}
	if writes.Load() != 0 {
		t.Fatal("writes")
	}
	client.objectiveDiagnostics.until = time.Now().Add(-time.Second)
	a.recordObjectiveState(client, "/lol-missions/v1/missions", "websocket", "Update", []byte(before))
	if client.objectiveDiagnostics.events != count {
		t.Fatal("expired capture emitted")
	}
}

func TestObjectiveDiagnosticsShapesBoundedAndOpaquePaths(t *testing.T) {
	if got := objectiveDiagnosticPath("/lol-missions/v1/player/ab?secret=123"); got != "/lol-missions/v1/player/{id}" {
		t.Fatal(got)
	}
	if objectiveDiagnosticPath("/lol-chat/v1/me") != "" {
		t.Fatal("unrelated namespace")
	}
	data := []byte(`{"viewed":false,"isNew":true,"description":"SECRET","objectives":[{"progress":{"currentProgress":3,"lastViewedProgress":2}}]}`)
	shape := objectiveStateShape(data, "test-salt")
	encoded, _ := json.Marshal(shape)
	if strings.Contains(string(encoded), "SECRET") || !strings.Contains(string(encoded), "currentProgress") {
		t.Fatal(string(encoded))
	}
	large := objectiveStateShape(make([]byte, (2<<20)+1), "test")
	if large["result"] != "too-large" {
		t.Fatal(large)
	}
	contracts := objectiveContracts([]byte(`{"paths":{"/lol-missions/v1/player":{"patch":{"requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"viewed":{"type":"boolean"}}}}}}}}}}`))
	if len(contracts) != 1 || contracts[0]["method"] != "PATCH" || contracts[0]["body"] == nil {
		t.Fatal(contracts)
	}
}

func TestOverviewCatalogPreservesBothCompositions(t *testing.T) {
	centered := "/lol-game-data/assets/ahri_splash_centered_86.jpg"
	uncentered := "/lol-game-data/assets/ahri_splash_uncentered_86.jpg"
	entries := []any{map[string]any{"id": 103086, "name": "阿狸", "splashPath": centered, "uncenteredSplashPath": uncentered}}
	for champion := 1; champion <= 103; champion++ {
		for skin := 0; skin < 10; skin++ {
			entries = append(entries, map[string]any{"id": champion*1000 + skin, "name": "fixture"})
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	skins, err := loadSkinCatalog(client)
	if err != nil || len(skins) != 1031 {
		t.Fatal(skins, err)
	}
	for _, skin := range skins {
		if skin.ID == 103086 && (skin.SplashPath != uncentered || skin.CenteredSplashPath != centered) {
			t.Fatal(skin)
		}
	}
	poster, _ := overviewSkinMedia(skins, 103086)
	if poster != centered {
		t.Fatal(poster)
	}
	variants := questSkinVariants(map[string]any{"questSkinInfo": map[string]any{"tiers": []any{map[string]any{"id": float64(103086), "name": "child", "splashPath": centered}}}}, Skin{ID: 103085, CenteredSplashPath: "parent"})
	if len(variants) != 1 || variants[0].CenteredSplashPath != centered {
		t.Fatal(variants)
	}
}

func TestAcceptWindowDiagnosticsDoNotSerializeHandles(t *testing.T) {
	events := []map[string]any{}
	calls := 0
	finish := observeAcceptRequest(func(event map[string]any) { events = append(events, event) }, func() acceptWindowSnapshot {
		calls++
		if calls == 1 {
			return acceptWindowSnapshot{123456789, "other"}
		}
		return acceptWindowSnapshot{987654321, "league-client"}
	})
	finish()
	raw, _ := json.Marshal(events)
	if len(events) != 2 || events[1]["foreground_changed"] != true || events[1]["window_action"] != "none" {
		t.Fatal(events)
	}
	if strings.Contains(string(raw), "123456789") || strings.Contains(string(raw), "987654321") {
		t.Fatal("handles serialized")
	}
}

func TestObjectiveEventRecorderDoesNotBlockSocketOnDiagnosticLock(t *testing.T) {
	a := &app{}
	client := &LCUClient{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	record := a.objectiveEventRecorder(ctx, client)
	client.objectiveDiagnostics.mu.Lock()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 40; i++ {
			record(LCUEvent{URI: "/lol-missions/v1/missions", Data: json.RawMessage(`[]`)})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		client.objectiveDiagnostics.mu.Unlock()
		t.Fatal("socket blocked")
	}
	client.objectiveDiagnostics.mu.Unlock()
}

func TestOverviewDoesNotMixCenteredPosterWithCollectionAnimation(t *testing.T) {
	skins := []Skin{{ID: 103086, CenteredSplashPath: "/lol-game-data/assets/ahri_centered_86.jpg", CollectionVideoPath: "/lol-game-data/assets/ahri_uncentered.webm"}}
	_, video := overviewSkinMedia(skins, 103086)
	if video != "" {
		t.Fatal("mixed composition", video)
	}
	skins[0].SplashVideoPath = "/lol-game-data/assets/ahri_centered.webm"
	_, video = overviewSkinMedia(skins, 103086)
	if video != skins[0].SplashVideoPath {
		t.Fatal("matching animation missing")
	}
}
