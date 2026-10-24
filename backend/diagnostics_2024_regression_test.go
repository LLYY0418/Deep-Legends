package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func Test2024CurrentRarityDatasetDateAndDistribution(t *testing.T) {
	p, _ := newARAMKitFixtureProvider(t, "")
	rows, e := p.loadARAMKitRarity(t.Context())
	if e != nil || len(rows.Stages) != 4 {
		t.Fatal(rows, e)
	}
	silver, gold, prismatic := 0.121079, 0.596057, 0.240263
	if rows.Stages[0].Games != 9152967 || rows.Stages[0].Silver != silver*100 || rows.Stages[0].Gold != gold*100 || rows.Stages[3].Prismatic != prismatic*100 {
		t.Fatal("real distribution changed", rows)
	}

	for _, replacement := range [][2]string{{`"1":{`, `"14":{`}, {`"4":{`, `"3":{`}, {`"probability":0.596057`, `"probability":0.996057`}, {`"sampleCount":9152967`, `"sampleCount":0`}} {
		p, _ := newARAMKitFixtureProvider(t, "")
		base := p.client.Transport
		p.client.Transport = championRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.HasSuffix(req.URL.Path, "/augment-rarity.json") {
				return publicDataResponse(req, bytes.ReplaceAll(r307Fixture(t, "augment-rarity", nil), []byte(replacement[0]), []byte(replacement[1]))), nil
			}
			return base.RoundTrip(req)
		})
		if _, e := p.loadARAMKitRarity(t.Context()); e == nil {
			t.Fatalf("invalid distribution accepted: %v", replacement)
		}
	}
}
func Test2024RarityProviderAcceptsCurrentPage(t *testing.T) {
	p, _ := newARAMKitFixtureProvider(t, "")
	response, e := p.loadARAMKitRarity(t.Context())
	if e != nil || len(response.Stages) != 4 || response.Source != "ARAMKit" {
		t.Fatal(response, e)
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
