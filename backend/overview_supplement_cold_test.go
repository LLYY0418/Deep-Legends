package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOPGGSupplementsRecoverColdChampionCatalog(t *testing.T) {
	for _, kind := range []string{"season", "current", "none"} {
		t.Run(kind, func(t *testing.T) {
			a, current, ref, _ := currentGameFixture(t)
			data := map[string]any{}
			for _, m := range a.champions.championMeta {
				data[m.Key] = map[string]any{"id": m.Key, "key": fmt.Sprint(m.ID), "name": m.Key}
			}
			data["Jayce"] = map[string]any{"id": "Jayce", "key": "126", "name": "杰斯"}
			for i := 1000; i < 1100; i++ {
				key := fmt.Sprintf("Fixture%d", i)
				data[key] = map[string]any{"id": key, "key": fmt.Sprint(i), "name": key}
			}
			catalog, _ := json.Marshal(map[string]any{"data": data})
			a.champions.championMeta = map[int]championMetadata{}
			a.opgg = newOPGGInsights()
			current = []byte(strings.ReplaceAll(string(current), "2026-09-09T00:56:11+09:00", time.Now().Add(-4*time.Minute).UTC().Format(time.RFC3339)))
			paths := map[string]int{}
			a.champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				paths[r.URL.Host+r.URL.Path]++
				var body []byte
				if r.URL.Host == "ddragon.leagueoflegends.com" {
					switch r.URL.Path {
					case "/api/versions.json":
						body = []byte(`["16.18.1"]`)
					case "/cdn/16.18.1/data/zh_CN/champion.json", "/cdn/16.18.1/data/en_US/champion.json":
						body = catalog
					default:
						t.Fatalf("unexpected catalog request %s", r.URL)
					}
				} else if r.URL.Host == "op.gg" {
					if r.Method == http.MethodGet {
						body = summaryFixtureHTML(ref.PlayerRef, 33)
					} else if kind == "season" {
						body = summaryFixtureResponse()
					} else if kind == "none" {
						body = []byte("0:{\"a\":\"$@7\"}\n7:null")
					} else {
						body = current
					}
				} else {
					t.Fatalf("unexpected history request: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
			})}
			if kind == "season" {
				got, err := a.fetchOPGGSeasonSummary(context.Background(), ref)
				if err != nil {
					t.Fatal(err)
				}
				if got.Overall.Games != 573 || got.Champions[0].ChampionID != 126 {
					t.Fatalf("bad season %+v", got)
				}
			} else {
				got, err := a.fetchOPGGCurrentGame(context.Background(), ref)
				if err != nil {
					t.Fatal(err)
				}
				want := "active"
				if kind == "none" {
					want = "none"
				}
				if got.Status != want {
					t.Fatalf("status=%s", got.Status)
				}
			}
			for _, path := range []string{"/api/versions.json", "/cdn/16.18.1/data/zh_CN/champion.json", "/cdn/16.18.1/data/en_US/champion.json"} {
				want := 1
				if kind == "none" {
					want = 0
				}
				if paths["ddragon.leagueoflegends.com"+path] != want {
					t.Fatalf("catalog fetch %s: %v", path, paths)
				}
			}
		})
	}
}
