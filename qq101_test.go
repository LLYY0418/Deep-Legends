package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func qq101FieldFixture(t *testing.T, fieldID, payload string) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"code":   0,
		"data":   map[string]any{"_fieldValues": map[string]any{"R" + fieldID: payload}},
		"result": payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func qq101HTTPResponse(request *http.Request, status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(string(body))), ContentLength: int64(len(body)), Request: request,
	}
}

func opggItemDepthFixture() []byte {
	return []byte(strings.Join([]string{
		`1:["$","tr",null,{"children":["depth_4_item_0",{"metaType":"item","metaId":3157},{"children":["61.19","%"]},{"children":"572 场"}]}]`,
		`2:["$","tr",null,{"children":["depth_5_item_0",{"metaType":"item","metaId":3089},{"children":["63.35","%"]},{"children":"211 场"}]}]`,
		`3:["$","tr",null,{"children":["depth_6_item_0",{"metaType":"item","metaId":3135},{"children":["66.67","%"]},{"children":"42 场"}]}]`,
	}, "\n"))
}

func TestQQ101LatestPatchUsesVersionNameAndRejectsNumericID(t *testing.T) {
	patch, err := parseQQ101LatestPatch([]byte(`{"code":0,"data":[{"id":"226","name":"16.17"}]}`))
	if err != nil || patch != "16.17" {
		t.Fatalf("patch = %q, err = %v", patch, err)
	}
	if _, err := parseQQ101LatestPatch([]byte(`{"code":0,"data":[{"id":"226","name":"226"}]}`)); err == nil {
		t.Fatal("numeric QQ101 version id was accepted as version_id")
	}
}

func TestQQ101TierMappingRejectsIncompatibleExactTiers(t *testing.T) {
	wants := map[string]int{"all": 255, "gold_plus": 24, "platinum_plus": 25, "emerald_plus": 26, "diamond_plus": 27, "master": 8, "master_plus": 28, "grandmaster": 9, "challenger": 10}
	for tier, want := range wants {
		if got, ok := qq101TierID(tier); !ok || got != want {
			t.Fatalf("QQ101 tier %q = %d/%v, want %d/true", tier, got, ok, want)
		}
	}
	for _, tier := range []string{"emerald", "platinum", "gold", "silver", "bronze", "iron", "future_tier"} {
		if got, ok := qq101TierID(tier); ok || got != 0 {
			t.Fatalf("incompatible QQ101 tier %q = %d/%v", tier, got, ok)
		}
	}
}

func TestQQ101BuildRowsParseRatesAndItemCombinations(t *testing.T) {
	rows, err := parseQQ101BuildRows(qq101FieldFixture(t, "18087", `{"forth_details":"3157_1_25.89_61.58#6657,3003_2_21.93_63.35","fifth_details":"3089_1_26.7_65.45","sixth_details":"3157_1_31.43_63.64"}`))
	if err != nil || len(rows[4]) != 2 || len(rows[5]) != 1 || len(rows[6]) != 1 {
		t.Fatalf("QQ101 build rows = %#v, err = %v", rows, err)
	}
	if len(rows[4][1].Assets) != 2 || rows[4][1].Assets[0].ID != 6657 || rows[4][1].PickRate != 21.93 || rows[4][1].WinRate != 63.35 || !rows[4][1].GamesUnavailable {
		t.Fatalf("QQ101 combined build row = %#v", rows[4][1])
	}
	if _, err := parseQQ101BuildRows([]byte(`{"code":0,"result":""}`)); err == nil {
		t.Fatal("empty QQ101 build response was accepted")
	}
}

func TestQQ101BuildRowsPreserveExtremeRatesWithoutInventingGames(t *testing.T) {
	rows, err := parseQQ101BuildRows(qq101FieldFixture(t, "18087", `{"forth_details":"3102_1_100.0_100.0#3089_2_100.0_0.0"}`))
	if err != nil || len(rows[4]) != 2 {
		t.Fatalf("QQ101 extreme build rows = %#v, err = %v", rows, err)
	}
	if !rows[4][0].GamesUnavailable || rows[4][0].Games != 0 || rows[4][0].PickRate != 100 || rows[4][0].WinRate != 100 {
		t.Fatalf("QQ101 full-win row = %#v", rows[4][0])
	}
	if !rows[4][1].GamesUnavailable || rows[4][1].Games != 0 || rows[4][1].WinRate != 0 {
		t.Fatalf("QQ101 zero-win row = %#v", rows[4][1])
	}
}

func TestQQ101BuildRequestUsesPatchNameInVersionQuery(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	requestedVersion := ""
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case qq101VersionPath:
			return qq101HTTPResponse(request, http.StatusOK, []byte(`{"code":0,"data":[{"id":"226","name":"16.17"}]}`)), nil
		case qq101RiftPath + "_build":
			requestedVersion = request.URL.Query().Get("version_id")
			return qq101HTTPResponse(request, http.StatusOK, qq101FieldFixture(t, "18087", `{"forth_details":"3157_1_25_61","fifth_details":"3089_1_20_62"}`)), nil
		default:
			return nil, errors.New("unexpected QQ101 build request")
		}
	})}
	if _, patch, err := provider.loadQQ101BuildRows(t.Context(), 13, "emerald_plus", "mid"); err != nil || patch != "16.17" || requestedVersion != "16.17" {
		t.Fatalf("QQ101 build version = patch:%q query:%q err:%v", patch, requestedVersion, err)
	}
}

func TestOPGGVersionFailureUsesPatchScopedFallbackCacheKey(t *testing.T) {
	provider := newChampionProvider()
	provider.patch = "16.17.1"
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return qq101HTTPResponse(request, http.StatusServiceUnavailable, nil), nil
	})}
	spec, _, query, err := opggDetailRequest("ranked", 13, "mid", "emerald_plus")
	if err != nil {
		t.Fatal(err)
	}
	resolved, dataVersion, err := provider.applyOPGGRequestVersion(t.Context(), spec, query)
	if err != nil || dataVersion != "fallback-16.17.1" || resolved.Get("version") != "" {
		t.Fatalf("OP.GG fallback version = query:%v version:%q err:%v", resolved, dataVersion, err)
	}
	oldKey := opggDetailCacheKey("ranked", spec.Region, 13, "MID", "emerald_plus", dataVersion)
	provider.patch = "16.18.1"
	_, nextVersion, err := provider.applyOPGGRequestVersion(t.Context(), spec, query)
	newKey := opggDetailCacheKey("ranked", spec.Region, 13, "MID", "emerald_plus", nextVersion)
	if err != nil || oldKey == newKey {
		t.Fatalf("OP.GG fallback cache keys were not patch scoped: %q / %q / %v", oldKey, newKey, err)
	}
}

func TestQQ101PositionsParseSingleAndDualLaneShares(t *testing.T) {
	dual, err := parseQQ101Positions(qq101FieldFixture(t, "18122", `{"lane_details":"MIDDLE：1.92_46.12_0.31_T4_52_78.54#TOP:0.53_51.5_0.3_T4_21_21.46"}`))
	if err != nil || len(dual) != 2 {
		t.Fatalf("dual positions = %#v, err = %v", dual, err)
	}
	if math.Abs(dual[0].RoleRate+dual[1].RoleRate-100) > 0.01 || dual[0].Position != "mid" || dual[1].Position != "top" {
		t.Fatalf("dual lane shares = %#v", dual)
	}
	single, err := parseQQ101Positions(qq101FieldFixture(t, "18122", `{"lane_details":"TOP：3.12_51.2_1.1_T2_8_100"}`))
	if err != nil || len(single) != 1 || single[0].Position != "top" || single[0].RoleRate != 100 {
		t.Fatalf("single lane share = %#v, err = %v", single, err)
	}
	if position, source, err := resolveGameplayRecommendationPosition("", dual); err != nil || position != "mid" || source != "opgg-primary" {
		t.Fatalf("dual lane recommendation = %q/%q/%v", position, source, err)
	}
	if position, source, err := resolveGameplayRecommendationPosition("", single); err != nil || position != "top" || source != "opgg-primary" {
		t.Fatalf("single lane recommendation = %q/%q/%v", position, source, err)
	}
}

func TestMergeQQ101PositionSharesPreservesOPGGOnlyLane(t *testing.T) {
	existing := []championPositionOption{{Position: "mid", RoleRate: 0, Play: 80}, {Position: "top", RoleRate: 0, Play: 20}}
	official := []championPositionOption{{Position: "mid", RoleRate: 100}}
	merged := mergeQQ101PositionShares(existing, official)
	if len(merged) != 2 || merged[0].Position != "mid" || merged[0].RoleRate != 100 || merged[1].Position != "top" || merged[1].Play != 20 {
		t.Fatalf("partial QQ101 position merge = %#v", merged)
	}
}

func TestQQ101ProbeKeepsPartialResults(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	var runeCalls int
	var active, maxActive int32
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != qq101VersionPath {
			current := atomic.AddInt32(&active, 1)
			for {
				observed := atomic.LoadInt32(&maxActive)
				if current <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, current) {
					break
				}
			}
			defer atomic.AddInt32(&active, -1)
			time.Sleep(15 * time.Millisecond)
		}
		switch request.URL.Path {
		case qq101VersionPath:
			return qq101HTTPResponse(request, http.StatusOK, []byte(`{"code":0,"data":[{"id":"226","name":"16.17"}]}`)), nil
		case qq101RiftPath + "_build":
			return qq101HTTPResponse(request, http.StatusOK, qq101FieldFixture(t, "18087", `{"forth_details":"3157_1_25.89_61.58#3089_2_21.93_63.35","fifth_details":"3089_1_26.7_65.45","sixth_details":"3157_1_31.43_63.64"}`)), nil
		case qq101RiftPath + "_runeinfo":
			runeCalls++
			return qq101HTTPResponse(request, http.StatusInternalServerError, []byte(`{"code":1}`)), nil
		case qq101RiftPath + "_skill":
			return qq101HTTPResponse(request, http.StatusOK, qq101FieldFixture(t, "18029", `{"data_details":"12_4_45.13_48.75#4_14_40_50"}`)), nil
		default:
			return nil, errors.New("unexpected QQ101 request: " + request.URL.String())
		}
	})}
	event := provider.qq101Probe(t.Context(), "ryze", 13, "emerald_plus", "mid")
	if event == nil || event["ok"] != false || event["forth_n"] != 2 || event["fifth_n"] != 1 || event["sixth_n"] != 1 || event["spells_n"] != 2 {
		t.Fatalf("probe event = %#v", event)
	}
	if runeCalls != 2 {
		t.Fatalf("rune retry calls = %d, want 2", runeCalls)
	}
	if maxActive < 3 {
		t.Fatalf("QQ101 probe requests were not concurrent: max active = %d", maxActive)
	}
	parts, ok := event["attempts"].([]qq101ProbePart)
	if !ok || len(parts) != 3 || parts[0].Outcome != "success" || parts[1].Outcome != "failed" || parts[2].Outcome != "success" {
		t.Fatalf("probe attempts = %#v", event["attempts"])
	}
}

func TestQQ101ProbeCooldownIsScopedByPatch(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	events := make(chan map[string]any, 2)
	provider.diag = func(event map[string]any) {
		if event["event"] == "qq101_probe" {
			events <- event
		}
	}
	var buildCalls int32
	var version atomic.Value
	version.Store("16.17")
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case qq101VersionPath:
			return qq101HTTPResponse(request, http.StatusOK, []byte(`{"code":0,"data":[{"name":"`+version.Load().(string)+`"}]}`)), nil
		case qq101RiftPath + "_build":
			atomic.AddInt32(&buildCalls, 1)
			return qq101HTTPResponse(request, http.StatusOK, qq101FieldFixture(t, "18087", `{"forth_details":"3157_1_25_61","fifth_details":"3089_1_20_62"}`)), nil
		case qq101RiftPath + "_runeinfo":
			return qq101HTTPResponse(request, http.StatusOK, qq101FieldFixture(t, "18119", `{"rune_details":"1_8230_jj_8230_20_50_100"}`)), nil
		case qq101RiftPath + "_skill":
			return qq101HTTPResponse(request, http.StatusOK, qq101FieldFixture(t, "18029", `{"data_details":"12_4_45_48"}`)), nil
		default:
			return nil, errors.New("unexpected QQ101 cooldown request")
		}
	})}
	provider.startQQ101Probe(t.Context(), "ryze", 13, "emerald_plus", "mid")
	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("first QQ101 probe did not complete")
	}
	deadline := time.Now().Add(time.Second)
	for {
		provider.qq101ProbeMu.Lock()
		running := provider.qq101ProbeRunning["13|emerald_plus|MIDDLE"]
		provider.qq101ProbeMu.Unlock()
		if !running || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	firstCalls := atomic.LoadInt32(&buildCalls)
	provider.startQQ101Probe(t.Context(), "ryze", 13, "emerald_plus", "mid")
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&buildCalls); got != firstCalls {
		t.Fatalf("probe cooldown repeated build request: %d -> %d", firstCalls, got)
	}
	select {
	case event := <-events:
		t.Fatalf("probe cooldown emitted another event: %#v", event)
	default:
	}
	deadline = time.Now().Add(time.Second)
	for {
		provider.qq101ProbeMu.Lock()
		running := provider.qq101ProbeRunning["13|emerald_plus|MIDDLE"]
		provider.qq101ProbeMu.Unlock()
		if !running || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	provider.qq101ProbeMu.Lock()
	stillRunning := provider.qq101ProbeRunning["13|emerald_plus|MIDDLE"]
	provider.qq101ProbeMu.Unlock()
	if stillRunning {
		t.Fatal("same-patch cooldown check did not finish")
	}
	provider.cache = newChampionDataCache(nil)
	version.Store("16.18")
	provider.startQQ101Probe(t.Context(), "ryze", 13, "emerald_plus", "mid")
	select {
	case event := <-events:
		if event["patch"] != "16.18" {
			t.Fatalf("patch-scoped probe event = %#v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("new QQ101 patch was incorrectly suppressed by the old cooldown")
	}
	if got := atomic.LoadInt32(&buildCalls); got != firstCalls+1 {
		t.Fatalf("new QQ101 patch did not trigger one build request: %d -> %d", firstCalls, got)
	}
}

func TestQQ101FeatureGateStopsUpstreamRequests(t *testing.T) {
	provider := newChampionProvider()
	provider.featureGates.apply(map[string]bool{featureGateQQ101: false})
	calls := 0
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("request should be gated")
	})}
	if _, err := provider.loadQQ101LatestPatch(t.Context()); err == nil || !strings.Contains(err.Error(), "disabled by feature gate") {
		t.Fatalf("disabled QQ101 error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("disabled QQ101 made %d upstream requests", calls)
	}
}

func TestStructuredDetailUsesQQ101SharesAndExplicitOPGGVersion(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.17.1"
	provider.championMeta[13] = championMetadata{ID: 13, Key: "Ryze", Slug: "ryze"}
	provider.championIDs["ryze"] = 13
	provider.static["item/6657.png"] = championAssetDescription{Name: "时光之杖"}
	provider.abilities["16.17.1/ryze"] = map[string]championAssetDescription{"Q": {Name: "超负荷"}}
	var detailVersion string
	var opggPageCalls int
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case qq101Host:
			if request.URL.Path == qq101VersionPath {
				return qq101HTTPResponse(request, http.StatusOK, []byte(`{"code":0,"data":[{"id":"226","name":"16.17"}]}`)), nil
			}
			if request.URL.Path == qq101RiftPath+"_newlane" {
				return qq101HTTPResponse(request, http.StatusOK, qq101FieldFixture(t, "18122", `{"lane_details":"MIDDLE：1.92_46.12_0.31_T4_52_78.54#TOP：0.53_51.5_0.3_T4_21_21.46"}`)), nil
			}
			if request.URL.Path == qq101RiftPath+"_build" {
				return qq101HTTPResponse(request, http.StatusOK, qq101FieldFixture(t, "18087", `{"forth_details":"3157_1_25.89_61.58","fifth_details":"3089_1_26.7_65.45","sixth_details":"3135_1_31.43_63.64"}`)), nil
			}
		case opggChampionHost:
			if strings.HasSuffix(request.URL.Path, "/versions") {
				return qq101HTTPResponse(request, http.StatusOK, []byte(`{"data":["16.17","16.16"]}`)), nil
			}
			detailVersion = request.URL.Query().Get("version")
			body := []byte(`{"data":{"summary":{"id":13,"average_stats":{"play":100,"win_rate":0.5},"positions":[{"name":"MIDDLE","stats":{"play":80,"win_rate":0.5,"role_rate":0}},{"name":"TOP","stats":{"play":20,"win_rate":0.51,"role_rate":0}}]},"core_items":[{"ids":[6657],"play":100,"win":50}]},"meta":{"version":"16.17"}}`)
			return qq101HTTPResponse(request, http.StatusOK, body), nil
		case opggPageHost:
			opggPageCalls++
			return qq101HTTPResponse(request, http.StatusNotFound, nil), nil
		}
		return nil, errors.New("unexpected detail request: " + request.URL.String())
	})}
	detail, err := provider.loadStructuredDetail(t.Context(), "ranked", "ryze", "mid", "emerald_plus")
	if err != nil {
		t.Fatal(err)
	}
	if detailVersion != "16.17" || detail.PositionsSource != "QQ101" || len(detail.Positions) != 2 || detail.Build.ItemSource != "QQ101" || detail.Build.ItemChainStatus != "ready" {
		t.Fatalf("detail version/source/positions = %q/%q/%#v", detailVersion, detail.PositionsSource, detail.Positions)
	}
	if opggPageCalls != 0 || len(detail.Build.FourthItems) != 1 || len(detail.Build.ItemAttempts) != 1 || detail.Build.ItemAttempts[0].Source != dataSourceQQ101 || detail.Build.ItemAttempts[0].Outcome != dataSourceSuccess {
		t.Fatalf("QQ101 item-depth decision = calls:%d build:%#v", opggPageCalls, detail.Build)
	}
	if math.Abs(detail.Positions[0].RoleRate+detail.Positions[1].RoleRate-100) > 0.01 {
		t.Fatalf("detail lane shares = %#v", detail.Positions)
	}
}

func TestStructuredDetailFallsBackWhenQQ101PositionsAreEmpty(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.17.1"
	provider.championMeta[13] = championMetadata{ID: 13, Key: "Ryze", Slug: "ryze"}
	provider.championIDs["ryze"] = 13
	provider.static["item/6657.png"] = championAssetDescription{Name: "时光之杖"}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case qq101Host:
			if request.URL.Path == qq101VersionPath {
				return qq101HTTPResponse(request, http.StatusOK, []byte(`{"code":0,"data":[{"id":"226","name":"16.17"}]}`)), nil
			}
			return qq101HTTPResponse(request, http.StatusOK, []byte(`{"code":0,"result":""}`)), nil
		case opggChampionHost:
			if strings.HasSuffix(request.URL.Path, "/versions") {
				return qq101HTTPResponse(request, http.StatusOK, []byte(`{"data":["16.17"]}`)), nil
			}
			body := []byte(`{"data":{"summary":{"id":13,"positions":[{"name":"MID","stats":{"play":100,"win_rate":0.5,"role_rate":1}}]},"core_items":[{"ids":[6657],"play":100,"win":50}]},"meta":{"version":"16.17"}}`)
			return qq101HTTPResponse(request, http.StatusOK, body), nil
		case opggPageHost:
			return qq101HTTPResponse(request, http.StatusOK, opggItemDepthFixture()), nil
		}
		return nil, errors.New("unexpected fallback request")
	})}
	detail, err := provider.loadStructuredDetail(context.Background(), "ranked", "ryze", "mid", "emerald_plus")
	if err != nil || len(detail.Build.CoreItems) == 0 || len(detail.Positions) != 1 || detail.Positions[0].RoleRate != 100 || detail.PositionsSource != "" || detail.Build.ItemSource != "OP.GG" || detail.Build.ItemChainStatus != "ready" {
		t.Fatalf("fallback detail = %#v, err = %v", detail, err)
	}
	if len(detail.Build.ItemAttempts) != 2 || detail.Build.ItemAttempts[0].Source != dataSourceQQ101 || detail.Build.ItemAttempts[0].Outcome != dataSourceFailed || detail.Build.ItemAttempts[1].Source != dataSourceOPGG || detail.Build.ItemAttempts[1].Outcome != dataSourceSuccess {
		t.Fatalf("empty QQ101 fallback attempts = %#v", detail.Build.ItemAttempts)
	}
}

func TestStructuredDetailFallsBackWhenQQ101BuildTimesOut(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.qq101Wait = 10 * time.Millisecond
	provider.patch = "16.17.1"
	provider.championMeta[13] = championMetadata{ID: 13, Key: "Ryze", Slug: "ryze"}
	provider.championIDs["ryze"] = 13
	provider.static["item/6657.png"] = championAssetDescription{Name: "时光之杖"}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case qq101Host:
			if request.URL.Path == qq101VersionPath {
				return qq101HTTPResponse(request, http.StatusOK, []byte(`{"code":0,"data":[{"name":"16.17"}]}`)), nil
			}
			<-request.Context().Done()
			return nil, request.Context().Err()
		case opggChampionHost:
			if strings.HasSuffix(request.URL.Path, "/versions") {
				return qq101HTTPResponse(request, http.StatusOK, []byte(`{"data":["16.17"]}`)), nil
			}
			return qq101HTTPResponse(request, http.StatusOK, []byte(`{"data":{"summary":{"id":13},"core_items":[{"ids":[6657],"play":100,"win":50}]},"meta":{"version":"16.17"}}`)), nil
		case opggPageHost:
			return qq101HTTPResponse(request, http.StatusOK, opggItemDepthFixture()), nil
		}
		return nil, errors.New("unexpected timeout fallback request")
	})}
	detail, err := provider.loadStructuredDetail(t.Context(), "ranked", "ryze", "mid", "emerald_plus")
	if err != nil || detail.Build.ItemSource != "OP.GG" || detail.Build.ItemFallback != "qq101-timeout" || len(detail.Build.ItemAttempts) != 2 || detail.Build.ItemAttempts[0].Outcome != dataSourceFailed || detail.Build.ItemAttempts[1].Outcome != dataSourceSuccess {
		t.Fatalf("timeout fallback detail = %#v, err = %v", detail.Build, err)
	}
}

func TestStructuredDetailReportsBothItemDepthFailures(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.17.1"
	provider.championMeta[13] = championMetadata{ID: 13, Key: "Ryze", Slug: "ryze"}
	provider.championIDs["ryze"] = 13
	provider.static["item/6657.png"] = championAssetDescription{Name: "时光之杖"}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case qq101Host:
			if request.URL.Path == qq101VersionPath {
				return qq101HTTPResponse(request, http.StatusOK, []byte(`{"code":0,"data":[{"name":"16.17"}]}`)), nil
			}
			return qq101HTTPResponse(request, http.StatusOK, []byte(`{"code":0,"result":""}`)), nil
		case opggChampionHost:
			if strings.HasSuffix(request.URL.Path, "/versions") {
				return qq101HTTPResponse(request, http.StatusOK, []byte(`{"data":["16.17"]}`)), nil
			}
			return qq101HTTPResponse(request, http.StatusOK, []byte(`{"data":{"summary":{"id":13},"core_items":[{"ids":[6657],"play":100,"win":50}]},"meta":{"version":"16.17"}}`)), nil
		case opggPageHost:
			return qq101HTTPResponse(request, http.StatusNotFound, nil), nil
		}
		return nil, errors.New("unexpected double failure request")
	})}
	detail, err := provider.loadStructuredDetail(t.Context(), "ranked", "ryze", "mid", "emerald_plus")
	if err != nil || len(detail.Build.CoreItems) == 0 || detail.Build.ItemChainStatus != "unavailable" || len(detail.Build.ItemAttempts) != 2 {
		t.Fatalf("double failure detail = %#v, err = %v", detail, err)
	}
	if detail.Build.ItemAttempts[0].Source != dataSourceQQ101 || detail.Build.ItemAttempts[0].Outcome != dataSourceFailed || detail.Build.ItemAttempts[1].Source != dataSourceOPGG || detail.Build.ItemAttempts[1].Outcome != dataSourceFailed {
		t.Fatalf("double failure attempts = %#v", detail.Build.ItemAttempts)
	}
}
