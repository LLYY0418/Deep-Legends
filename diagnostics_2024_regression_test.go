package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func Test2024CurrentRarityDatasetDateAndDistribution(t *testing.T) {
	page, err := os.ReadFile("testdata/diagnostics-2024/rarity.html")
	if err != nil {
		t.Fatal(err)
	}
	rows, citation, err := parseHexdataRarity(page)
	if err != nil || !hexdataCitationComplete(citation) || citation.ReportDate != "2026-08-29" || citation.Patch != "16.17" || len(rows) != 4 {
		t.Fatalf("current page rejected: %#v %#v %v", rows, citation, err)
	}
	if rows[0].Gold != 58.7 || rows[0].Games != 111451534 || rows[3].Prismatic != 26.5 {
		t.Fatalf("wrong values: %#v", rows)
	}
	wrongPage := bytes.ReplaceAll(page, []byte(`"url":"https://hexdata.com.cn/augment-rarity"`), []byte(`"url":"https://hexdata.com.cn/other"`))
	_, wrongCitation, _ := parseHexdataRarity(wrongPage)
	if wrongCitation.ReportDate != "" {
		t.Fatal("borrowed another page's date")
	}
	for _, replacement := range [][2]string{{"第 1 阶段", "第 14 阶段"}, {"第 4 阶段", "第 3 阶段"}, {"黄金 58.7%", "黄金 98.7%"}, {"111,451,534", "0"}} {
		bad := bytes.ReplaceAll(page, []byte(replacement[0]), []byte(replacement[1]))
		if _, _, err := parseHexdataRarity(bad); err == nil {
			t.Fatalf("invalid distribution accepted: %v", replacement)
		}
	}
}

func Test2024RarityProviderAcceptsCurrentPage(t *testing.T) {
	page, err := os.ReadFile("testdata/diagnostics-2024/rarity.html")
	if err != nil {
		t.Fatal(err)
	}
	p := newChampionProvider()
	p.client = &http.Client{Transport: championRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := []byte(`{}`)
		if req.URL.Path == "/augment-rarity" {
			body = page
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req}, nil
	})}
	response, err := p.loadHexdataRarity(context.Background())
	if err != nil || len(response.Stages) != 4 || response.Citation == nil || response.Citation.ReportDate != "2026-08-29" {
		t.Fatalf("provider still rejects live page: %#v %v", response, err)
	}
}

func Test2024AugmentUnsuffixedArtworkBeforeSmall(t *testing.T) {
	for _, family := range []string{"kiwi", "cherry"} {
		t.Run(family, func(t *testing.T) {
			art, err := os.ReadFile("testdata/diagnostics-2024/drop-bear.png")
			if err != nil {
				t.Fatal(err)
			}
			provider := newChampionProvider()
			var paths []string
			var pathsMu sync.Mutex
			provider.client = &http.Client{Transport: championRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				pathsMu.Lock()
				paths = append(paths, req.URL.Path)
				pathsMu.Unlock()
				status, body := 404, []byte("missing")
				if strings.HasSuffix(req.URL.Path, "/new_augment.png") {
					status, body = 200, art
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req}, nil
			})}
			recorder := httptest.NewRecorder()
			(&app{champions: provider}).handleChampionAsset(recorder, httptest.NewRequest("GET", "/api/champion-asset?source=communitydragon&path=/latest/game/assets/ux/"+family+"/augments/icons/new_augment_large.png", nil))
			pathsMu.Lock()
			defer pathsMu.Unlock()
			if recorder.Code != 200 || !bytes.Equal(recorder.Body.Bytes(), art) || len(paths) == 0 || len(paths) > 4 || strings.Contains(strings.Join(paths, " "), "_small") {
				t.Fatalf("lost colored artwork: %v status=%d", paths, recorder.Code)
			}
		})
	}
}
