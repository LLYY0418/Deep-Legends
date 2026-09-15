package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in public upstream diagnosis. Prints only shapes and public asset paths,
// never account rows, upstream bodies, credentials or local session tokens.
func TestR92LivePublicDiagnosis(t *testing.T) {
	if os.Getenv("DEEP_LEGENDS_R92_LIVE") != "1" {
		t.Skip("real public network only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	p := newChampionProvider()
	p.diag = func(e map[string]any) {
		if e["event"] == "load_top_players_shape" || e["event"] == "champion_upstream" {
			b, _ := json.Marshal(e)
			t.Log(string(b))
		}
	}
	for _, resource := range []string{"summoner-spells", "queues", "champions/76"} {
		data, err := p.fetchDirect(ctx, communityDragonHost, "/latest/plugins/rcp-be-lol-game-data/global/default/v1/"+resource+".json", nil, championJSONMax, "application/json")
		if err != nil {
			t.Errorf("resource %s failed: %v", resource, err)
			continue
		}
		if resource == "champions/76" {
			var v struct {
				ID    int    `json:"id"`
				Name  string `json:"name"`
				Skins []struct {
					ID     int    `json:"id"`
					IsBase bool   `json:"isBase"`
					Splash string `json:"splashPath"`
				} `json:"skins"`
			}
			json.Unmarshal(data, &v)
			for _, s := range v.Skins {
				if s.ID == 76000 {
					t.Logf("background champion=%d name=%s skin=%d isBase=%v splash=%s", v.ID, v.Name, s.ID, s.IsBase, s.Splash)
				}
			}
		} else {
			var rows []map[string]any
			json.Unmarshal(data, &rows)
			selected := []map[string]any{}
			for _, r := range rows {
				id, _ := r["id"].(float64)
				keep := false
				if resource == "summoner-spells" {
					for _, n := range []int{1, 3, 4, 6, 7, 11, 12, 14, 21} {
						keep = keep || id == float64(n)
					}
				} else {
					keep = id == 1750 || id == 2400 || id == 3140
				}
				if keep {
					selected = append(selected, r)
				}
			}
			if resource == "summoner-spells" {
				b, _ := json.MarshalIndent(selected, "", "  ")
				os.WriteFile("/tmp/deep-legends-r92/spell-paths.json", b, 0600)
			}
			for _, r := range selected {
				t.Logf("%s id=%v name=%v gameMode=%v icon=%v", resource, r["id"], r["name"], r["gameMode"], r["iconPath"])
			}
		}
	}
	// The existing reader has no mode parameter: these three calls intentionally
	// demonstrate the actual identical URL/shape, not fictional mode endpoints.
	for _, q := range []int{1750, 2400, 3140} {
		p.cache = newChampionDataCache(nil)
		rows := p.loadTopPlayersForPosition(ctx, "leesin", "")
		t.Logf("queue=%d actual_url=https://op.gg/zh-cn/lol/leaderboards/champions/leesin?region=kr parsed=%d", q, len(rows))
	}
}

func TestR92LiveBackground(t *testing.T) {
	if os.Getenv("DEEP_LEGENDS_R92_LIVE") != "1" {
		t.Skip("real public network only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	p := newChampionProvider()
	p.cache = newChampionDataCache(nil)
	player := gameplayPlayer{}
	applyMasteryBackgroundFallback(&player, []gameplayMastery{{ChampionID: 76, ChampionPoints: 100}})
	a := &app{champions: p}
	w := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/champion-asset?"+url.Values{"source": {player.BackgroundSource}, "path": {player.BackgroundPath}}.Encode(), nil)
	a.handleChampionAsset(w, request.WithContext(ctx))
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/") {
		t.Fatalf("real background failed status=%d", w.Code)
	}
	t.Logf("real mastery background skin=%d source=%s HTTP=%d bytes=%d content_type=%s", player.BackgroundSkinID, player.BackgroundSource, w.Code, w.Body.Len(), w.Header().Get("Content-Type"))
}

func TestR92LiveProDirectory(t *testing.T) {
	if os.Getenv("DEEP_LEGENDS_R92_LIVE") != "1" {
		t.Skip("real public network only")
	}
	p := newChampionProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	data, err := fetchProDirectory(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	teams, err := parseOPGGProPlayers(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("real directory bytes=%d teams=%d ranked_accounts=%d", len(data), len(teams), len(proRankedLadderAccounts(teams)))
}
