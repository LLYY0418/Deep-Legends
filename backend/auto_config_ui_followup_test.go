package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestR298UIFallbackShardDescriptions(t *testing.T) {
	for _, slot := range defaultGameplayStatModSlots() {
		for _, perk := range slot.Perks {
			if strings.TrimSpace(perk.LongDescription) == "" {
				t.Errorf("shard %d lacks its numeric description", perk.ID)
			}
		}
	}
}

func TestR298UICatalogAvailability(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		connected, failItems bool
		want                 string
	}{{"disconnected", false, false, "disconnected"}, {"connected", true, false, "ready"}, {"items failed", true, true, "failed"}} {
		t.Run(tc.name, func(t *testing.T) {
			catalog, _ := r298ReviewFixture(t)
			a := &app{}
			var client *LCUClient
			if tc.connected {
				client = &LCUClient{baseURL: "https://127.0.0.1", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					body := "[]"
					status := 200
					if strings.HasSuffix(req.URL.Path, "items.json") {
						if tc.failItems {
							status = 503
						} else {
							data, _ := json.Marshal(catalog.Items)
							body = string(data)
						}
					}
					if strings.HasSuffix(req.URL.Path, "summoner-spells.json") {
						data, _ := json.Marshal(catalog.Spells)
						body = string(data)
					}
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
				})}}
				a.connected = true
				a.lcu = client
			}
			key, _ := clientCatalogKey(client)
			if client == nil {
				key = "ddragon"
			}
			a.perkCatalog = map[string]gameplayPerkCatalogCacheEntry{key: {payload: gameplayPerkCatalogResponse{Styles: catalog.Styles, StatModSlots: catalog.StatModSlots, Perks: catalog.Perks}, loadedAt: time.Now()}}
			got, err := a.loadAutoConfigCatalog(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got.StatModSlots[0].Perks[0].LongDescription != runeShardDescription(int(got.StatModSlots[0].Perks[0].ID)) {
				t.Fatal("old cached shard was not enriched with its numeric description")
			}
			if a.perkCatalog[key].payload.StatModSlots[0].Perks[0].LongDescription != "" {
				t.Fatal("enriching the response mutated the shared rune catalog")
			}
			raw, _ := json.Marshal(got)
			var wire map[string]any
			json.Unmarshal(raw, &wire)
			if wire["clientConnected"] != tc.connected {
				t.Errorf("clientConnected = %v", wire["clientConnected"])
			}
			if wire["itemsStatus"] != tc.want {
				t.Errorf("itemsStatus = %v, want %s", wire["itemsStatus"], tc.want)
			}
			spellsWant := tc.want
			if tc.connected {
				spellsWant = "ready"
			}
			if wire["spellsStatus"] != spellsWant {
				t.Errorf("spellsStatus = %v, want %s", wire["spellsStatus"], spellsWant)
			}
		})
	}
}
