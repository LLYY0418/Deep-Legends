package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func itemDepthTestProvider(payload string) *championProvider {
	p := newChampionProvider()
	p.cache = nil
	p.client = &http.Client{Transport: championRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload))}, nil
	})}
	return p
}

func itemDepthPresencePayload(mask, brokenMask int) string {
	var payload strings.Builder
	for depth := 4; depth <= 6; depth++ {
		if mask&(1<<(depth-4)) != 0 {
			for index := 0; index < 5; index++ {
				fmt.Fprintf(&payload, `%x:["$","tr","depth_%d_item_%d",{"metaType":"item","metaId":%d,"children":[0,"%%","1 场"]}]`+"\n", depth*16+index, depth, index, 3000+depth+index*10)
			}
		} else if brokenMask&(1<<(depth-4)) != 0 {
			fmt.Fprintf(&payload, `%x:["$","tr","depth_%d_item_0",{"metaType":"item","metaId":%d,"children":"invalid stats"}]`+"\n", depth, depth, 3000+depth)
		}
	}
	return payload.String()
}

// Missing fourth, fifth or sixth items must be independent, including when
// Flight is embedded in HTML instead of returned as a raw RSC response.
func TestRankedItemDepthPresenceMatrix(t *testing.T) {
	for _, format := range []string{"rsc", "html"} {
		for mask := 1; mask < 8; mask++ {
			t.Run(fmt.Sprintf("%s/stages_%03b", format, mask), func(t *testing.T) {
				payload := itemDepthPresencePayload(mask, 0)
				if format == "html" {
					fragment, _ := json.Marshal(payload)
					payload = `<html><script>self.__next_f.push([1,` + string(fragment) + `])</script></html>`
				}
				p := itemDepthTestProvider(payload)
				response := championDetailResponse{}
				p.resolveRankedItemDepths(t.Context(), "amumu", "jungle", "emerald_plus", "16.18", nil, "", errors.New("no fallback"), true, false, &response)
				build := response.Build
				if build.ItemSource != "OP.GG" || build.ItemChainStatus != "ready" || len(build.ItemAttempts) != 1 {
					t.Fatalf("independent stages rejected: %+v", build)
				}
				for index, rows := range [][]championMetricRow{build.FourthItems, build.FifthItems, build.SixthItems} {
					want := ((mask >> index) & 1) * 5
					if len(rows) != want {
						t.Fatalf("depth %d: got %d rows, want %d", index+4, len(rows), want)
					}
					for rowIndex, row := range rows {
						if row.Assets[0].ID != 3004+index+rowIndex*10 || row.WinRate != 0 || row.Games != 1 || row.Assets[0].Path == "" {
							t.Fatalf("real low-sample row changed: %+v", row)
						}
					}
				}
			})
		}
	}
}

func TestRankedItemDepthFallbackPreservesAvailableStages(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		primary, broken, fallback int
		wantSource, wantStatus    string
	}{
		{"fourth plus fallback fifth", 1, 2, 7, "OP.GG + QQ101", "ready"},
		{"fifth plus fallback fourth", 2, 1, 5, "OP.GG + QQ101", "ready"},
		{"sixth survives incomplete fallback", 4, 1, 2, "OP.GG + QQ101", "unavailable"},
		{"overlapping fallback cannot replace primary", 1, 2, 1, "OP.GG", "unavailable"},
		{"empty fallback keeps primary", 1, 2, 0, "OP.GG", "unavailable"},
		{"all primary rows failed", 0, 7, 7, "QQ101", "ready"},
		{"broken sixth remains an error", 3, 4, 3, "OP.GG", "unavailable"},
		{"fallback repairs sixth", 3, 4, 4, "OP.GG + QQ101", "ready"},
		{"absent fifth does not replace valid source", 1, 0, 7, "OP.GG", "ready"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := itemDepthTestProvider(itemDepthPresencePayload(tc.primary, tc.broken))
			fallback := make(map[int][]championMetricRow)
			for index := 0; index < 3; index++ {
				if tc.fallback&(1<<index) != 0 {
					fallback[4+index] = []championMetricRow{{Assets: []championAsset{{ID: 4004 + index}}, WinRate: 75, Games: 20}}
				}
			}
			response := championDetailResponse{}
			p.resolveRankedItemDepths(t.Context(), "nami", "support", "emerald_plus", "16.18", fallback, "16.18", nil, true, false, &response)
			build := response.Build
			if build.ItemSource != tc.wantSource || build.ItemChainStatus != tc.wantStatus {
				t.Fatalf("source/status = %s/%s, want %s/%s", build.ItemSource, build.ItemChainStatus, tc.wantSource, tc.wantStatus)
			}
			for index, rows := range [][]championMetricRow{build.FourthItems, build.FifthItems, build.SixthItems} {
				wantID := 0
				wantRows := 1
				if tc.primary&(1<<index) != 0 {
					wantID = 3004 + index
					wantRows = 5
				} else if tc.broken != 0 && tc.fallback&(1<<index) != 0 {
					wantID = 4004 + index
				}
				if wantID == 0 {
					if len(rows) != 0 {
						t.Fatalf("invented depth %d: %+v", index+4, rows)
					}
				} else if len(rows) != wantRows || rows[0].Assets[0].ID != wantID || wantRows == 5 && rows[4].Assets[0].ID != wantID+40 {
					t.Fatalf("lost/replaced depth %d, want item %d: %+v", index+4, wantID, rows)
				}
			}
		})
	}
}

func TestRankedItemDepthDelayedStatisticsAndEmptySlots(t *testing.T) {
	// Reduced from Lulu/Yuumi support on 2026-09-10: a zero-win or one-win
	// sample is followed by four empty table slots. Slots are not items.
	for _, rate := range []string{"0.00%", "100.00%"} {
		payload := `1:["$","tr","depth_5_item_0",{"children":[{"metaType":"item","metaId":3109},"$L7f","$L80"]}]
7f:["$","strong",null,{"children":"` + rate + `"}]
80:["$","span",null,{"children":"1 场"}]` + "\n"
		for index := 1; index < 5; index++ {
			payload += fmt.Sprintf(`%x:["$","tr","depth_5_item_%d",{"children":[["$","td",null,{"children":null}],["$","td",null,{"children":[null,["$","span",null,{"children":""}]]}]]}]`+"\n", 128+index, index)
		}
		fragment, _ := json.Marshal(payload)
		p := itemDepthTestProvider(`<script>self.__next_f.push([1,` + string(fragment) + `])</script>`)
		depths, _, err := p.loadOPGGDepthRows(t.Context(), "yuumi", "support", "emerald_plus", "16.18", time.Time{})
		if err != nil || len(depths[5]) != 1 || depths[5][0].Games != 1 || depths[5][0].Assets[0].ID != 3109 {
			t.Fatalf("delayed real row lost or placeholder treated as an item: %v %+v", err, depths)
		}
		if rate == "0.00%" && depths[5][0].WinRate != 0 || rate == "100.00%" && depths[5][0].WinRate != 100 {
			t.Fatalf("delayed win rate changed: %+v", depths[5])
		}
	}
}

func TestRankedItemDepthEmptyAndErrorPagesStayUnavailable(t *testing.T) {
	for _, payload := range []string{"", `<html>Bad Gateway</html>`, `1:["$","div",null,{"children":["404","This page could not be found."]}]`} {
		p := itemDepthTestProvider(payload)
		response := championDetailResponse{}
		p.resolveRankedItemDepths(t.Context(), "lulu", "support", "challenger", "16.18", nil, "", errors.New("no fallback"), true, false, &response)
		if response.Build.ItemChainStatus != "unavailable" || len(response.Build.FourthItems)+len(response.Build.FifthItems)+len(response.Build.SixthItems) != 0 {
			t.Fatalf("invalid page labeled as verified empty samples: %+v", response.Build)
		}
	}
}

func TestRankedItemDepthPartialCacheKeepsOlderTimestamp(t *testing.T) {
	for _, useFallback := range []bool{false, true} {
		p := itemDepthTestProvider("unused")
		p.cache = newChampionDataCache(nil)
		detailAt := time.Now().UTC()
		depthAt := detailAt.Add(-time.Hour)
		key := "v3|opgg-depth|amumu|jungle|emerald_plus|16.18|" + detailAt.Format(time.RFC3339Nano)
		payload := []byte(itemDepthPresencePayload(1, 2))
		p.cache.entries[key] = championCacheEnvelope{
			Schema: championCacheSchema, Key: key, Data: payload, FetchedAt: depthAt,
			ExpiresAt: detailAt.Add(time.Hour), StaleUntil: detailAt.Add(2 * time.Hour),
		}
		p.cache.order, p.cache.bytes = []string{key}, len(payload)
		var fallback map[int][]championMetricRow
		if useFallback {
			fallback = map[int][]championMetricRow{5: {{Assets: []championAsset{{ID: 3089}}, Games: 1, WinRate: 100}}}
		}
		response := championDetailResponse{FetchedAt: detailAt}
		p.resolveRankedItemDepths(t.Context(), "amumu", "jungle", "emerald_plus", "16.18", fallback, "16.18", nil, true, false, &response)
		if !response.FetchedAt.Equal(depthAt) || len(response.Build.FourthItems) != 5 {
			t.Fatalf("partial snapshot age/rows lost, fallback=%v: %+v", useFallback, response)
		}
	}
}

// Opt-in replay of captured public pages through the production loader and
// resolver. Expected counts come from an independent scan of valid rows;
// OP.GG pads some tables with empty row keys, which must not count as items.
func TestCapturedChampionItemDepthPages(t *testing.T) {
	manifests := os.Getenv("DEEP_LEGENDS_ITEM_DEPTH_MANIFESTS")
	if manifests == "" {
		t.Skip("set DEEP_LEGENDS_ITEM_DEPTH_MANIFESTS to captured-page manifests")
	}
	for _, path := range strings.Split(manifests, string(os.PathListSeparator)) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var samples []struct {
			Champion, Position, Tier, Patch, Path string
			Rows                                  map[int]int
		}
		if err := json.Unmarshal(data, &samples); err != nil {
			t.Fatal(err)
		}
		for _, sample := range samples {
			t.Run(sample.Champion+"/"+sample.Position+"/"+sample.Tier, func(t *testing.T) {
				payload, err := os.ReadFile(sample.Path)
				if err != nil {
					t.Fatal(err)
				}
				p := itemDepthTestProvider(string(payload))
				depths, _, err := p.loadOPGGDepthRows(context.Background(), sample.Champion, sample.Position, sample.Tier, sample.Patch, time.Time{})
				available := sample.Rows[4]+sample.Rows[5]+sample.Rows[6] > 0
				if (err == nil) != available {
					t.Fatalf("page state: %v, available=%v", err, available)
				}
				response := championDetailResponse{}
				p.resolveRankedItemDepths(t.Context(), sample.Champion, sample.Position, sample.Tier, sample.Patch, nil, "", errors.New("no fallback"), true, false, &response)
				wantStatus := "unavailable"
				if available {
					wantStatus = "ready"
				}
				if response.Build.ItemChainStatus != wantStatus {
					t.Fatalf("page resolved as %s, want %s", response.Build.ItemChainStatus, wantStatus)
				}
				for index, rows := range [][]championMetricRow{response.Build.FourthItems, response.Build.FifthItems, response.Build.SixthItems} {
					if len(depths[4+index]) != sample.Rows[4+index] || len(rows) != sample.Rows[4+index] {
						t.Fatalf("stage %d: parsed %d/resolved %d, valid source rows %d", index+4, len(depths[4+index]), len(rows), sample.Rows[4+index])
					}
				}
				t.Logf("fourth=%d fifth=%d sixth=%d status=%s", len(depths[4]), len(depths[5]), len(depths[6]), response.Build.ItemChainStatus)
			})
		}
	}
}
