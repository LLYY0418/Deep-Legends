package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const coldOverviewPoster = "/lol-game-data/assets/ASSETS/Characters/Ahri/Skins/Skin86/Images/ahri_splash_centered_86.jpg"
const coldOverviewVideo = "/lol-game-data/assets/ASSETS/Characters/Ahri/Skins/Skin86/Images/ahri_splash_centered_86.webm"

func TestOverviewColdBackgroundBeforeCollectionAndStatus(t *testing.T) {
	a, _, _ := newGameplayOverviewSGPFixture(t, false)
	a.summoner.SummonerID = 7
	var catalogReads atomic.Int32
	catalog := []map[string]any{}
	for champion := 1; champion <= 110; champion++ {
		for offset := 0; offset < 10; offset++ {
			catalog = append(catalog, map[string]any{"id": champion*1000 + offset, "name": "Fixture skin", "championId": champion})
		}
	}
	catalog = append(catalog, map[string]any{"id": 103086, "name": "Fixture selected skin", "championId": 103, "splashPath": coldOverviewPoster, "splashVideoPath": coldOverviewVideo})
	catalogJSON, _ := json.Marshal(catalog)
	original := a.lcu.http.Transport
	a.lcu.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Errorf("artwork wrote to client: %s", r.Method)
		}
		body := ""
		switch r.URL.Path {
		case "/lol-game-data/assets/v1/skins.json":
			catalogReads.Add(1)
			body = string(catalogJSON)
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			body = `{"backgroundSkinId":103086}`
		case "/lol-collections/v1/inventories/7/backdrop":
			body = `{"summonerId":7,"championId":103,"backdropType":"specified-skin","backdropImage":"` + coldOverviewPoster + `"}`
		default:
			if strings.Contains(r.URL.Path, "inventory") || strings.Contains(r.URL.Path, "loot") {
				t.Errorf("overview requested owned collection: %s", r.URL.Path)
			}
			return original.RoundTrip(r)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	// Exercise the real first overview assembly, with no collection or career load.
	got := a.loadGameplayOverview(context.Background(), a.lcu, a.summoner, gameplayReference{}, 0, 20, "all", false)
	if got.Player.BackgroundSkinID != 103086 || got.Player.BackgroundPosterPath != coldOverviewPoster || got.Player.BackgroundVideoPath != coldOverviewVideo {
		t.Fatalf("cold art missing: %+v", got.Player)
	}
	if len(a.allSkins) != 0 || a.snapshotReady || a.collectionRequested {
		t.Fatal("artwork bootstrapped the collection")
	}
	w := httptest.NewRecorder()
	a.handleStatus(w, httptest.NewRequest("GET", "/api/status", nil))
	var status statusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Summoner.BackgroundPosterPath != coldOverviewPoster || status.Summoner.BackgroundVideoPath != coldOverviewVideo {
		t.Fatal("status erased first artwork", status.Summoner)
	}
	_ = a.loadOverviewBackground(context.Background(), a.lcu, a.summoner, true)
	if catalogReads.Load() != 1 {
		t.Fatal("public catalog cache not reused", catalogReads.Load())
	}
	if filename := os.Getenv("MAINTENANCE_OVERVIEW_FIXTURE"); filename != "" {
		data, _ := json.MarshalIndent(got.Player, "", "  ")
		if err := os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOverviewDirectBackdropSurvivesUnavailableCatalog(t *testing.T) {
	a := r175App(t)
	current := Summoner{SummonerID: 7}
	a.lcu = r105LCU(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			w.Write([]byte(`{"backgroundSkinId":0}`))
		case "/lol-collections/v1/inventories/7/backdrop":
			w.Write([]byte(`{"summonerId":7,"championId":103,"backdropType":"highest-mastery","backdropImage":"` + coldOverviewPoster + `"}`))
		default:
			w.WriteHeader(503)
		}
	})
	player := a.loadOverviewBackground(context.Background(), a.lcu, current, true)
	overview := gameplayOverview{Player: player, Masteries: []gameplayMastery{{ChampionID: 64, ChampionPoints: 10}}}
	a.completeOverviewBackground(&overview, "")
	if overview.Player.BackgroundPosterPath != coldOverviewPoster || overview.Player.BackgroundSkinID != 0 {
		t.Fatal("unmatched backdrop replaced by guessed art", overview.Player)
	}
	// A new client must not reuse the previous client's public artwork cache.
	a.facadeSkinCatalogClient = a.lcu
	a.facadeSkinCatalog = []Skin{{ID: 103086, CenteredSplashPath: coldOverviewPoster}}
	a.lcu = &LCUClient{}
	p := gameplayPlayer{BackgroundSkinID: 103086}
	a.applyOverviewSkinMedia(&p)
	if p.BackgroundPosterPath != "" {
		t.Fatal("old client artwork reused")
	}
}

func TestOverviewUnavailableArtKeepsVerifiedFallback(t *testing.T) {
	a := &app{}
	overview := gameplayOverview{Masteries: []gameplayMastery{{ChampionID: 103, ChampionPoints: 10}}}
	a.completeOverviewBackground(&overview, "")
	if overview.Player.BackgroundSkinID != 103000 || overview.Player.BackgroundSource != "gtimg" || overview.Player.BackgroundPath == "" {
		t.Fatal("fallback lost", overview.Player)
	}
}

func TestOverviewSlowCatalogDoesNotSkipBackdropOrBlockStatus(t *testing.T) {
	a := r175App(t)
	current := Summoner{SummonerID: 7}
	entered, done := make(chan struct{}), make(chan struct{})
	a.lcu = &LCUClient{baseURL: "http://fixture", token: "fixture-token", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/lol-game-data/assets/v1/skins.json":
			close(entered)
			<-r.Context().Done()
			close(done)
			return nil, r.Context().Err()
		case "/lol-game-data/v1/skins.json":
			return nil, r.Context().Err()
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			body = `{"backgroundSkinId":0}`
		case "/lol-collections/v1/inventories/7/backdrop":
			body = `{"summonerId":7,"championId":103,"backdropType":"highest-mastery","backdropImage":"` + coldOverviewPoster + `"}`
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	output := make(chan gameplayPlayer, 1)
	go func() { output <- a.loadOverviewBackground(ctx, a.lcu, current, true) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("catalog fixture was not reached")
	}
	start := time.Now()
	_, _, _, _ = a.cachedOverviewSkinMedia(a.lcu, 103086)
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("status waited behind catalog network request")
	}
	player := <-output
	<-done
	if player.BackgroundPosterPath != coldOverviewPoster || player.BackgroundSkinID != 0 {
		t.Fatal("slow catalog skipped actual backdrop", player)
	}
}
