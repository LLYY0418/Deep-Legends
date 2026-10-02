package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func r152Augment(id, matches int, tier string, score *float64) yourGGArenaAggregateAugment {
	return yourGGArenaAggregateAugment{AugmentID: id, Matches: matches, Tier: tier, Score: score}
}

func r152Score(value float64) *float64 { return &value }

func r152Catalog() []gameplayAugment {
	return []gameplayAugment{
		{ID: 901, Name: "银一", Rarity: "kSilver"},
		{ID: 902, Name: "金一", Rarity: "kGold"},
		{ID: 903, Name: "棱一", Rarity: "kPrismatic"},
		{ID: 904, Name: "银二", Rarity: "kSilver"},
		{ID: 905, Name: "金二", Rarity: "kGold"},
	}
}

func r152Rows() []yourGGArenaAggregateAugment {
	return []yourGGArenaAggregateAugment{
		r152Augment(901, 100, "S", r152Score(91)),
		r152Augment(902, 90, "A", r152Score(81)),
		r152Augment(903, 80, "B", r152Score(71)),
		r152Augment(904, 70, "C", r152Score(61)),
	}
}

func TestR152AugmentRowToleranceAndCounters(t *testing.T) {
	base := r152Rows()
	for _, tc := range []struct {
		name  string
		row   yourGGArenaAggregateAugment
		check func(yourGGArenaAugmentMapStats) bool
	}{
		{"matches", r152Augment(905, 0, "S", r152Score(50)), func(s yourGGArenaAugmentMapStats) bool { return s.SkippedMatches == 1 }},
		{"score", r152Augment(905, 50, "S", nil), func(s yourGGArenaAugmentMapStats) bool { return s.SkippedScore == 1 }},
		{"grade", r152Augment(905, 50, "E", r152Score(50)), func(s yourGGArenaAugmentMapStats) bool { return s.SkippedGrade == 1 }},
		{"id", r152Augment(0, 50, "S", r152Score(50)), func(s yourGGArenaAugmentMapStats) bool { return s.SkippedID == 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups, stats, err := mapYourGGArenaAggregateAugmentsWithStats(append(append([]yourGGArenaAggregateAugment{}, base...), tc.row), r152Catalog())
			if err != nil || len(flattenArenaAugmentGroups(groups, 0)) != 4 || stats.RowsIn != 5 || stats.RowsKept != 4 || !tc.check(stats) {
				t.Fatalf("single invalid row must preserve four official rows: groups=%+v stats=%+v err=%v", groups, stats, err)
			}
		})
	}
}

func TestR152DuplicateAugmentKeepsMostMatchesAndFirstTie(t *testing.T) {
	for _, tc := range []struct {
		name      string
		matches   int
		wantScore float64
	}{
		{"more matches", 200, 99},
		{"equal matches", 100, 91},
		{"fewer matches", 50, 91},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := append(r152Rows(), r152Augment(901, tc.matches, "OP", r152Score(99)))
			groups, stats, err := mapYourGGArenaAggregateAugmentsWithStats(rows, r152Catalog())
			if err != nil || stats.SkippedDuplicate != 1 || stats.RowsKept != 4 || groups[0].Rows[0].Score != tc.wantScore {
				t.Fatalf("duplicate resolution: groups=%+v stats=%+v err=%v", groups, stats, err)
			}
		})
	}
}

func TestR152AugmentFallbackThresholdAndQualities(t *testing.T) {
	tooMany := append(r152Rows(), r152Augment(905, 0, "S", r152Score(50)), r152Augment(906, 0, "S", r152Score(50)))
	_, stats, err := mapYourGGArenaAggregateAugmentsWithStats(tooMany, r152Catalog())
	if !errors.Is(err, errYourGGArenaAugmentsTooManyInvalid) || stats.RowsKept != 4 || stats.RowsIn != 6 {
		t.Fatalf("below 80%% should fall back: stats=%+v err=%v", stats, err)
	}
	quality := []yourGGArenaAggregateAugment{
		r152Augment(901, 100, "S", r152Score(90)), r152Augment(904, 90, "S", r152Score(80)),
		r152Augment(902, 80, "A", r152Score(70)), r152Augment(905, 70, "A", r152Score(60)),
		r152Augment(903, 0, "A", r152Score(50)),
	}
	_, stats, err = mapYourGGArenaAggregateAugmentsWithStats(quality, r152Catalog())
	if !errors.Is(err, errYourGGArenaAugmentsQualityMissing) || stats.RowsKept != 4 || stats.SkippedMatches != 1 {
		t.Fatalf("missing prismatic quality should fall back at 80%%: stats=%+v err=%v", stats, err)
	}
}

func TestR152StructuredArenaAugmentDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name             string
		augments         string
		wantSource       string
		wantReason       string
		wantKept         int
		aggregateFailure bool
		catalogFailure   bool
		emptyCatalog     bool
	}{
		{name: "single invalid row", augments: `[{"augmentId":901,"tier":"S","score":90,"matches":100},{"augmentId":902,"tier":"A","score":80,"matches":90},{"augmentId":903,"tier":"B","score":70,"matches":80},{"augmentId":904,"tier":"C","score":60,"matches":70},{"augmentId":905,"tier":"S","score":50,"matches":0}]`, wantSource: "your.gg", wantKept: 4},
		{name: "too many invalid", augments: `[{"augmentId":901,"tier":"S","score":90,"matches":100},{"augmentId":902,"tier":"A","score":80,"matches":90},{"augmentId":903,"tier":"B","score":70,"matches":80},{"augmentId":904,"tier":"C","score":60,"matches":70},{"augmentId":905,"tier":"S","score":50,"matches":0},{"augmentId":906,"tier":"S","score":50,"matches":0}]`, wantSource: "op.gg", wantReason: "too-many-invalid", wantKept: 4},
		{name: "quality missing", augments: `[{"augmentId":901,"tier":"S","score":90,"matches":100},{"augmentId":904,"tier":"S","score":80,"matches":90},{"augmentId":902,"tier":"A","score":70,"matches":80},{"augmentId":905,"tier":"A","score":60,"matches":70},{"augmentId":903,"tier":"B","score":50,"matches":0}]`, wantSource: "op.gg", wantReason: "quality-missing", wantKept: 4},
		{name: "aggregate failed", augments: `[]`, wantSource: "op.gg", wantReason: "aggregate-failed", aggregateFailure: true},
		{name: "augment missing", augments: `[]`, wantSource: "op.gg", wantReason: "augment-missing"},
		{name: "catalog failed", augments: `[{"augmentId":901,"tier":"S","score":90,"matches":100}]`, wantSource: "op.gg", wantReason: "quality-missing", wantKept: 1, catalogFailure: true},
		{name: "catalog empty", augments: `[{"augmentId":901,"tier":"S","score":90,"matches":100}]`, wantSource: "op.gg", wantReason: "quality-missing", wantKept: 1, emptyCatalog: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := newChampionProvider()
			provider.cache = newChampionDataCache(nil)
			provider.patch = "16.19.1"
			// R160 uses bundled metadata without requesting CommunityDragon. Add
			// this synthetic ID fixture to the already-cached catalog so the R152
			// row-quality guard remains exercised independently of network state.
			provider.rememberCommunityDragonAugments([]gameplayAugment{
				{ID: 901, Name: "银一", Rarity: "kSilver"},
				{ID: 902, Name: "金一", Rarity: "kGold"},
				{ID: 903, Name: "棱一", Rarity: "kPrismatic"},
				{ID: 904, Name: "银二", Rarity: "kSilver"},
				{ID: 905, Name: "金二", Rarity: "kGold"},
			})
			var sourceEvent map[string]any
			provider.diag = func(event map[string]any) {
				if event["event"] == "arena_augment_source" {
					sourceEvent = event
				}
			}
			provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				key := request.URL.Host + request.URL.Path
				var body string
				status := http.StatusOK
				switch key {
				case opggChampionHost + "/api/global/champions/arena/versions":
					body = `{"data":["16.19"]}`
				case opggChampionHost + "/api/global/champions/arena/67":
					body = `{"data":{"summary":{"id":67,"average_stats":{"play":1000,"win":550}},"core_items":[{"ids":[3153],"play":100,"win":60}],"augment_group":[{"rarity":1,"augments":[{"id":904,"play":100,"win":55}]}]},"meta":{"version":"16.19"}}`
				case yourGGArenaHost + "/kr/api/arena/champions/67":
					body = `{"success":true,"statusCode":200,"response":{"version":"16.19","coreItems":[],"prismaticItems":[],"augments":` + tc.augments + `}}`
					if tc.aggregateFailure {
						status = http.StatusServiceUnavailable
					}
				case communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
					body = `[{"id":901,"nameTRA":"银一","rarity":"kSilver"},{"id":902,"nameTRA":"金一","rarity":"kGold"},{"id":903,"nameTRA":"棱一","rarity":"kPrismatic"},{"id":904,"nameTRA":"银二","rarity":"kSilver"},{"id":905,"nameTRA":"金二","rarity":"kGold"}]`
					if tc.catalogFailure {
						status = http.StatusServiceUnavailable
					}
					if tc.emptyCatalog {
						body = `[]`
					}
				case communityDragonHost + "/latest/cdragon/arena/zh_cn.json":
					body = `{"augments":[]}`
				case communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json":
					body = `[{"id":3153,"name":"测试装备"}]`
				default:
					return nil, fmt.Errorf("unexpected R152 request: %s", key)
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
			})}
			response, err := provider.loadStructuredDetail(context.Background(), "arena", "67", "", "")
			if err != nil {
				t.Fatal(err)
			}
			wantIn := strings.Count(tc.augments, `"augmentId"`)
			if tc.aggregateFailure {
				wantIn = 0
			}
			wantSkippedMatches := strings.Count(tc.augments, `"matches":0`)
			if tc.aggregateFailure {
				wantSkippedMatches = 0
			}
			if sourceEvent == nil || sourceEvent["source"] != tc.wantSource || sourceEvent["rows_in"] != wantIn || sourceEvent["rows_kept"] != tc.wantKept || sourceEvent["skipped_matches"] != wantSkippedMatches {
				t.Fatalf("diagnostic=%+v", sourceEvent)
			}
			if sourceEvent["rows"] != len(response.ArenaAugments) {
				t.Fatalf("output row count diagnostic=%+v, rows=%d", sourceEvent, len(response.ArenaAugments))
			}
			if tc.wantReason == "" {
				if _, ok := sourceEvent["reason"]; ok {
					t.Fatalf("unexpected fallback reason: %+v", sourceEvent)
				}
			} else if sourceEvent["reason"] != tc.wantReason {
				t.Fatalf("reason=%+v", sourceEvent)
			}
			allowed := map[string]bool{"event": true, "source": true, "reason": true, "rows": true, "rows_in": true, "rows_kept": true, "skipped_matches": true, "skipped_score": true, "skipped_grade": true, "skipped_id": true, "skipped_duplicate": true}
			for key, value := range sourceEvent {
				if !allowed[key] {
					t.Fatalf("diagnostic leaks extra field %q: %+v", key, sourceEvent)
				}
				if key == "rows" || strings.HasPrefix(key, "rows_") || strings.HasPrefix(key, "skipped_") {
					if _, ok := value.(int); !ok {
						t.Fatalf("diagnostic counter %q is not int: %T", key, value)
					}
				}
			}
			if tc.wantSource == "your.gg" {
				if len(response.ArenaAugments) != 4 || response.ArenaAugments[0].Score != 90 {
					t.Fatalf("official rows=%+v", response.ArenaAugments)
				}
			} else if len(response.ArenaAugments) != 1 || response.ArenaAugments[0].Score != 0 {
				t.Fatalf("fallback rows=%+v", response.ArenaAugments)
			}
		})
	}
}
