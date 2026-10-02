package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestCommunityDragonImagePathsCoverFrontendLootIcons(t *testing.T) {
	cases := map[string]string{
		"/fe/lol-loot/assets/loot_item_icons/chest_128.png":                 "/latest/plugins/rcp-fe-lol-loot/global/default/assets/loot_item_icons/chest_128.png",
		"/fe/lol-loot/assets/loot_item_icons/material_clashtickets.png":     "/latest/plugins/rcp-fe-lol-loot/global/default/assets/loot_item_icons/material_clashtickets.png",
		"/fe/lol-loot/assets/loot_item_icons/MATERIAL_ClashTickets.png":     "/latest/plugins/rcp-fe-lol-loot/global/default/assets/loot_item_icons/material_clashtickets.png",
		"/fe/lol-static-assets/images/currency/icons/currency_champion.png": "/latest/plugins/rcp-fe-lol-static-assets/global/default/images/currency/icons/currency_champion.png",
	}
	for input, want := range cases {
		got := communityDragonImagePaths(input)
		if len(got) != 1 || got[0] != want {
			t.Fatalf("communityDragonImagePaths(%q) = %#v, want [%q]", input, got, want)
		}
	}
	for _, rejected := range []string{
		"/fe/lol-loot/assets/loot_item_icons/.png",
		"/fe/lol-loot/assets/loot_item_icons/",
		"/fe/lol-loot/assets/loot_item_icons/sub/chest.png",
		"/fe/lol-loot/assets/loot_item_icons/..%2Fsecret.png",
		"/fe/lol-loot/assets/loot_item_icons/a.png?x=1",
		"/fe/lol-other/assets/chest_128.png",
	} {
		if got := communityDragonImagePaths(rejected); got != nil {
			t.Fatalf("communityDragonImagePaths(%q) = %#v, want no upstream candidates", rejected, got)
		}
	}
}

// The connected client answers 404 for these two legacy icons (0924 log), so the
// image endpoint must fall back to the public mirror instead of returning 404.
func TestHandleImageFallsBackToCommunityDragonForMissingLegacyLootIcons(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 60))
	newApp := func(upstream *[]string) *app {
		var mu sync.Mutex
		p := newChampionProvider()
		p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			mu.Lock()
			*upstream = append(*upstream, r.URL.Path)
			mu.Unlock()
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader(string(png)))}, nil
		})}
		lcu := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("missing"))}, nil
		})}}
		return &app{lcu: lcu, connected: true, champions: p}
	}
	get := func(a *app, assetPath string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		a.handleImage(w, httptest.NewRequest(http.MethodGet, "/api/image?path="+url.QueryEscape(assetPath), nil))
		return w
	}
	for _, name := range []string{"chest_128.png", "material_clashtickets.png"} {
		var upstream []string
		w := get(newApp(&upstream), "/fe/lol-loot/assets/loot_item_icons/"+name)
		if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/") || w.Body.Len() != len(png) {
			t.Fatalf("%s: status=%d type=%q bytes=%d", name, w.Code, w.Header().Get("Content-Type"), w.Body.Len())
		}
		if len(upstream) != 1 || upstream[0] != "/latest/plugins/rcp-fe-lol-loot/global/default/assets/loot_item_icons/"+name {
			t.Fatalf("%s: upstream requests = %#v", name, upstream)
		}
	}
	// A record without a name yields ".png": nothing is asked of the public mirror.
	var upstream []string
	if w := get(newApp(&upstream), "/fe/lol-loot/assets/loot_item_icons/.png"); w.Code != http.StatusNotFound || len(upstream) != 0 {
		t.Fatalf("nameless icon: status=%d upstream=%#v", w.Code, upstream)
	}
}
