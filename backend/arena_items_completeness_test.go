package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"testing"
)

// Public aggregate capture only: no matches or account identities are stored.
func TestArenaCapturedItemsSurviveStructuredDetail(t *testing.T) {
	aggregate, err := os.ReadFile("testdata/arena-items/yourgg-122-20260909.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := os.ReadFile("testdata/arena-items/catalog-20260909.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw yourGGArenaAggregateResponse
	if err := json.Unmarshal(aggregate, &raw); err != nil {
		t.Fatal(err)
	}
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.17.1"
	provider.static["item/3153.png"] = championAssetDescription{Name: "破败王者之刃"}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body []byte
		switch {
		case request.URL.Host == opggChampionHost && request.URL.Path == "/api/global/champions/arena/versions":
			body = []byte(`{"data":["16.17"]}`)
		case request.URL.Host == opggChampionHost && request.URL.Path == "/api/global/champions/arena/122":
			body = []byte(`{"data":{"summary":{"id":122,"average_stats":{"play":1000,"win":550}},"core_items":[{"ids":[3153],"play":100,"win":60}],"augment_group":[]},"meta":{"version":"16.17"}}`)
		case request.URL.Host == communityDragonHost && request.URL.Path == "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json":
			body = catalog
		case request.URL.Host == communityDragonHost:
			body = []byte(`[]`)
		case request.URL.Host == yourGGArenaHost && request.URL.Path == "/kr/api/arena/champions/122":
			body = aggregate
		default:
			return nil, fmt.Errorf("unexpected fixture request: %s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: request}, nil
	})}
	detail, err := provider.loadStructuredDetail(context.Background(), "arena", "122", "mid", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		raw   []yourGGArenaAggregateMetric
		rows  []championMetricRow
		count int
	}{
		{"core", raw.Response.CoreItems, detail.Build.CoreItems, 56},
		{"prismatic", raw.Response.PrismaticItems, detail.Build.PrismItems, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.raw) != tc.count || len(tc.rows) != tc.count {
				t.Fatalf("raw=%d detail=%d want=%d", len(tc.raw), len(tc.rows), tc.count)
			}
			expected, actual := make([]int, 0, len(tc.raw)), make([]int, 0, len(tc.rows))
			metrics := make(map[int]yourGGArenaAggregateMetric)
			for _, row := range tc.raw {
				expected = append(expected, row.ItemID)
				metrics[row.ItemID] = row
			}
			for _, row := range tc.rows {
				if len(row.Assets) != 1 {
					t.Fatalf("aggregate item turned into a route: %#v", row)
				}
				asset := row.Assets[0]
				actual = append(actual, asset.ID)
				want := metrics[asset.ID]
				if row.Games != want.Matches || row.Score != want.Score || row.Tier != want.Tier || row.WinRate != want.WinRate*100 {
					t.Fatalf("statistics changed for %d", asset.ID)
				}
				if asset.Name == "" || asset.Path == "" {
					t.Fatalf("missing metadata for %d", asset.ID)
				}
			}
			sort.Ints(expected)
			sort.Ints(actual)
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("item identities changed: got=%v want=%v", actual, expected)
			}
		})
	}
}
