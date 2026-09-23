package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestR126CareerCollectionCatalogKeepsBaseSkinForAutomaticBackdrop(t *testing.T) {
	const image = "/lol-game-data/assets/ASSETS/Characters/LeeSin/Skins/Base/Images/leesin_splash_centered_0.jpg"
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-collections/v1/inventories/7/backdrop" {
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"summonerId":    7,
			"championId":    64,
			"backdropImage": image,
			"backdropType":  "highest-mastery",
		})
	})
	base := Skin{ID: 64000, ChampionID: 64, ChampionName: "盲僧", Name: "盲僧", SplashPath: image, Owned: true}
	a := &app{
		allSkins:         []Skin{{ID: 64001, ChampionID: 64, ChampionName: "盲僧", Name: "传统僧侣 李青"}},
		allSkinsWithBase: []Skin{base, {ID: 64001, ChampionID: 64, ChampionName: "盲僧", Name: "传统僧侣 李青"}},
	}

	skins, source, err := a.loadFacadeSkins(context.Background(), client)
	if err != nil || source != "collection" {
		t.Fatalf("loadFacadeSkins source=%q err=%v", source, err)
	}
	state := facadeState{Skins: make([]facadeSkin, 0, len(skins))}
	for _, skin := range skins {
		state.Skins = append(state.Skins, projectFacadeSkin(skin))
	}
	a.applyFacadeBackdrop(context.Background(), client, Summoner{SummonerID: 7}, &state)

	if state.Profile.BackgroundSkinID != 64000 {
		t.Fatalf("background skin=%d, want 64000", state.Profile.BackgroundSkinID)
	}
	if state.Profile.BackgroundChampionID != 64 {
		t.Fatalf("background champion=%d, want 64", state.Profile.BackgroundChampionID)
	}
	if state.Profile.BackgroundSkinName != "盲僧" {
		t.Fatalf("background name=%q, want base skin name", state.Profile.BackgroundSkinName)
	}
	if len(state.Skins) == 0 || state.Skins[0].ID != 64000 || !state.Skins[0].Owned {
		t.Fatalf("base skin missing or lost ownership: %+v", state.Skins)
	}
	backgroundInCatalog := false
	for _, skin := range state.Skins {
		backgroundInCatalog = backgroundInCatalog || skin.ID == state.Profile.BackgroundSkinID
	}
	if !backgroundInCatalog {
		t.Fatal("resolved automatic backdrop is not present in the career catalog")
	}
}

func TestR126AutomaticBackdropKeepsChampionWhenCatalogCannotMatch(t *testing.T) {
	const image = "/lol-game-data/assets/ASSETS/Characters/LeeSin/Skins/Base/Images/leesin_splash_centered_0.jpg"
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"summonerId":    7,
			"championId":    64,
			"backdropImage": image,
			"backdropType":  "highest-mastery",
		})
	})
	state := facadeState{}
	new(app).applyFacadeBackdrop(context.Background(), client, Summoner{SummonerID: 7}, &state)
	if state.Profile.BackgroundSkinID != 0 || state.Profile.BackgroundChampionID != 64 {
		t.Fatalf("unmatched backdrop identity = %+v", state.Profile)
	}
	if state.Profile.BackgroundSkinName != "客户端当前背景" {
		t.Fatalf("unmatched backdrop name=%q", state.Profile.BackgroundSkinName)
	}
}

func TestR126AutomaticBackdropKeepsChampionWithoutUsableImage(t *testing.T) {
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"summonerId": 7, "championId": 64, "backdropImage": "", "backdropType": "highest-mastery",
		})
	})
	state := facadeState{}
	new(app).applyFacadeBackdrop(context.Background(), client, Summoner{SummonerID: 7}, &state)
	if state.Profile.BackgroundChampionID != 64 {
		t.Fatalf("empty image discarded explicit champion identity: %+v", state.Profile)
	}
}

func TestR126CollectionRefreshInstallsFullCatalogForCareer(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "a.allSkinsWithBase = result.AllWithBase") {
		t.Fatal("collection refresh no longer installs the full skin catalog for career")
	}
}
