package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Reduced from the public Anivia mid/emerald_plus/16.18 Flight response on
// 2026-09-10: five fourth-item rows, no fifth/sixth row keys at all.
const anivia1555Depths = `1:["$","tr","depth_4_item_0",{"metaType":"item","metaId":3157,"children":["66.67%","3 场"]}]
2:["$","tr","depth_4_item_1",{"metaType":"item","metaId":3041,"children":["66.67%","3 场"]}]
3:["$","tr","depth_4_item_2",{"metaType":"item","metaId":6653,"children":["33.33%","3 场"]}]
4:["$","tr","depth_4_item_3",{"metaType":"item","metaId":3165,"children":["50.00%","2 场"]}]
5:["$","tr","depth_4_item_4",{"metaType":"item","metaId":3135,"children":["0.00%","1 场"]}]`

func Test1555AniviaKeepsFourthItemsWithoutFifthSamples(t *testing.T) {
	p := newChampionProvider()
	p.cache = nil
	p.client = &http.Client{Transport: championRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(anivia1555Depths))}, nil
	})}
	response := championDetailResponse{}
	p.resolveRankedItemDepths(context.Background(), "anivia", "mid", "emerald_plus", "16.18", nil, "16.18", fmt.Errorf("QQ101 empty response"), true, false, &response)
	build := response.Build
	if len(build.FourthItems) != 5 || len(build.FifthItems) != 0 || build.ItemChainStatus != "ready" || build.ItemSource != "OP.GG" {
		t.Fatalf("fourth items dropped: %+v", build)
	}
	if build.FourthItems[4].WinRate != 0 || build.FourthItems[4].Games != 1 || build.FourthItems[0].Assets[0].ID != 3157 {
		t.Fatal("source rows changed")
	}
}

func Test1555MalformedFifthKeepsValidFourthButReportsFailure(t *testing.T) {
	p := newChampionProvider()
	p.cache = nil
	p.client = &http.Client{Transport: championRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(anivia1555Depths + `6:["$","tr","depth_5_item_0",{"metaType":"item","metaId":3089,"children":"bad stats"}]`))}, nil
	})}
	response := championDetailResponse{}
	p.resolveRankedItemDepths(context.Background(), "anivia", "mid", "emerald_plus", "16.18", nil, "16.18", fmt.Errorf("QQ101 empty response"), true, false, &response)
	if len(response.Build.FourthItems) != 5 || response.Build.ItemChainStatus != "unavailable" {
		t.Fatalf("partial parse treated as empty samples: %+v", response.Build)
	}
}
