package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestR74RiotTransientIdentityFailureIsNotCached(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	calls := 0
	p := newChampionProvider()
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, context.DeadlineExceeded
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"puuid":"fixture","gameName":"test","tagLine":"KR1"}`)), Request: r}, nil
	})}
	riot := newRiotProvider(p)
	if _, err := riot.accountByRiotID(context.Background(), "test", "KR1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v", err)
	}
	account, err := riot.accountByRiotID(context.Background(), "test", "KR1")
	if err != nil || account.PUUID != "fixture" || calls != 2 {
		t.Fatalf("poisoned identity cache: calls=%d account=%+v err=%v", calls, account, err)
	}
}

func TestR74FacadeCatalogLoadsWithoutOpeningCollection(t *testing.T) {
	rows := []map[string]any{}
	for champion := 1; champion <= 100; champion++ {
		for skin := 0; skin < 10; skin++ {
			rows = append(rows, map[string]any{"id": champion*1000 + skin, "championId": champion, "name": "Fixture", "championName": "Fixture"})
		}
	}
	body, _ := json.Marshal(rows)
	reads := 0
	client := &LCUClient{baseURL: "https://fixture", token: "fixture-token", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		reads++
		if r.URL.Path != "/lol-game-data/assets/v1/skins.json" {
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
	})}}
	a := &app{}
	for i := 0; i < 2; i++ {
		skins, source, err := a.loadFacadeSkins(context.Background(), client)
		if err != nil || len(skins) != 1000 || source != "lcu-catalog" {
			t.Fatalf("catalog=%d source=%s err=%v", len(skins), source, err)
		}
		for _, skin := range skins {
			if skin.Owned {
				t.Fatal("public metadata must not invent ownership")
			}
		}
	}
	if reads != 1 || len(a.allSkins) != 0 {
		t.Fatal("career should cache metadata without triggering collection")
	}
	a.allSkinsWithBase = []Skin{{ID: 1000, Owned: true}}
	skins, source, err := a.loadFacadeSkins(context.Background(), client)
	if err != nil || source != "collection" || !skins[0].Owned {
		t.Fatal("collection ownership must supersede metadata")
	}
}

func TestR74FacadeConnectionChangeInvalidatesStaleCatalog(t *testing.T) {
	client := &LCUClient{baseURL: "https://fixture", token: "fixture-token", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}}
	a := &app{}
	_, _, err := a.loadFacadeSkins(context.Background(), client)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	// A connection change must not inherit a stale successful or failed cache.
	a.facadeSkinCatalog = []Skin{{ID: 1000}}
	a.facadeSkinCatalogAt = time.Now()
	other := &LCUClient{baseURL: client.baseURL, token: "fixture-token", http: client.http}
	skins, _, err := a.loadFacadeSkins(context.Background(), other)
	if len(skins) != 0 || err == nil {
		t.Fatal("connection switch retained stale catalog")
	}
}

func TestR74RiotBudgetExhaustionReturnsRetryAfterWithoutWaitingForTimeout(t *testing.T) {
	p := newRiotProvider(newChampionProvider())
	for i := 0; i < 90; i++ {
		p.longWindow = append(p.longWindow, time.Now())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	started := time.Now()
	err := p.wait(ctx)
	if riotErrorStatus(err) != 429 || time.Since(started) > time.Second {
		t.Fatalf("budget failure must be immediate, err=%v", err)
	}
	recorder := httptest.NewRecorder()
	writeRiotHTTPError(recorder, err)
	if recorder.Code != 429 || recorder.Header().Get("Retry-After") == "" || !strings.Contains(recorder.Body.String(), "额度正在恢复") {
		t.Fatalf("missing recovery hint: %+v", recorder)
	}
	if len(p.longWindow) != 90 {
		t.Fatal("failed wait must not reserve another request")
	}
}

func TestR74RecommendationInventoryFindsOtherNamespacesWithoutModifyingFiles(t *testing.T) {
	root := t.TempDir()
	location := settingsLocation{allowedRoot: root, configRoot: filepath.Join(root, "Config")}
	files := map[string]string{
		"Global/Recommended/current.json":      `{"uid":"deep-legends-v2-recommended"}`,
		"Champions/Ryze/Recommended/old.json":  `{"uid":"deep-legends-v1-13-middle","title":"private-title"}`,
		"Champions/Ryze/Recommended/user.json": `{"uid":"user","title":"private-user"}`,
		"Global/Recommended/imported.json":     `{"uid":"imported","startedFrom":"DL"}`,
	}
	for name, body := range files {
		file := filepath.Join(location.configRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	out := readRecommendationInventory(context.Background(), location)
	for _, key := range []string{"managed_v2", "legacy_uid", "dl_marker_only", "other"} {
		if out[key] != 1 {
			t.Fatalf("inventory=%v", out)
		}
	}
	if out["files"] != 4 {
		t.Fatal(out)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "Ryze") {
		t.Fatal("inventory exposed user contents or paths")
	}
	for name, body := range files {
		data, err := os.ReadFile(filepath.Join(location.configRoot, filepath.FromSlash(name)))
		if err != nil || string(data) != body {
			t.Fatal("diagnostics changed a recommendation")
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if readRecommendationInventory(canceled, location)["truncated"] != true {
		t.Fatal("inventory ignored cancellation")
	}
}
