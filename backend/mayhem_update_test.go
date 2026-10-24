package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMayhemOfficialPatchDiscoveryAndFullNewPool(t *testing.T) {
	raw, err := os.ReadFile("../scripts/patch-notes/fixtures/26.21.html")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parseOfficialPatchNotes(raw, "https://lol.qq.com/gicp/news/410/37099805.html")
	if err != nil {
		t.Fatal(err)
	}
	newCount, removedCount := 0, 0
	for _, section := range doc.Modes.Mayhem.Sections {
		if section.Title == "新增的强化符文" {
			newCount = len(section.Entries)
		}
		if section.Title == "已移除的强化符文" {
			removedCount = len(section.Items)
		}
	}
	if doc.Version != "26.21" || newCount != 35 || removedCount != 13 {
		t.Fatal(doc.Version, newCount, removedCount)
	}
	index := []byte(`callback({"data":{"result":[{"sTitle":"26.21版本更新公告","sRedirectURL":"https://lol.qq.com/gicp/news/410/37099805.html"},{"sTitle":"99.99版本更新公告","sRedirectURL":"https://evil.example/"}]}})`)
	version, url, err := readOfficialPatchURL(index)
	if err != nil || version != "26.21" || !validPatchSource(url) {
		t.Fatal(version, url, err)
	}
}

func TestMayhemPatchRefreshRecoversAfterFailureAndPreservesLatest(t *testing.T) {
	a := &app{storage: &localStore{root: t.TempDir()}}
	client := &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})}
	if a.refreshPatchNotesWithClient(context.Background(), client) == nil {
		t.Fatal("failure not reported")
	}
	client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte(`callback({"data":{"result":[{"sTitle":"26.21版本更新公告","sRedirectURL":"https://lol.qq.com/gicp/news/410/37099805.html"}]}})`)))}, nil
	})
	if err := a.refreshPatchNotesWithClient(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if a.patchNotesState().documents[0].Version != "26.21" {
		t.Fatal("newer bundled patch lost")
	}
}

func TestMayhemOfficialFailureCannotMasqueradeAsChecked(t *testing.T) {
	a := &app{storage: &localStore{root: t.TempDir()}}
	client := &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		status, body := http.StatusOK, bundledPatchNotes
		if r.URL.Host == "apps.game.qq.com" {
			status, body = http.StatusServiceUnavailable, []byte("unavailable")
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}
	if err := a.refreshPatchNotesWithClient(context.Background(), client); err == nil {
		t.Fatal("fallback feed was incorrectly reported as an official check")
	}
	if a.patchNotesState().documents[0].Version != "26.21" {
		t.Fatal("fallback erased newer bundle")
	}
}

func TestMayhemOfficialParserHandlesContainersAndRejectsEmptyArticles(t *testing.T) {
	url := "https://lol.qq.com/gicp/news/410/37099805.html"
	raw := []byte(`<h1 class="art-tit">26.22版本更新公告</h1><span class="art-time" data="2026-11-04"></span><div id="article"><div><h3>海克斯大乱斗</h3><p>国服更新</p><h4>新增的强化符文</h4><p>新海克斯</p><ul><li>效果说明</li></ul></div></div>`)
	doc, err := parseOfficialPatchNotes(raw, url)
	if err != nil || len(doc.Modes.Mayhem.Sections) != 2 {
		t.Fatal(doc.Version, err)
	}
	if _, err := parseOfficialPatchNotes(bytes.Replace(raw, []byte("<div><h3>"), []byte("<script><h3>"), 1), url); err == nil {
		t.Fatal("empty unsupported article accepted")
	}
}

func TestMayhemLiveCatalogIgnoresRemovedRowsAndPreservesNewDescription(t *testing.T) {
	raw := []augmentRaw{}
	for _, row := range readCurrentMayhem().Augments {
		raw = append(raw, row)
	}
	data, _ := json.Marshal(map[string]any{"data": raw, "icon": "/meta/images/lol/16.21.1/champion/Annie.png"})
	fragment, _ := json.Marshal(string(data))
	page := []byte("<script>self.__next_f.push([1," + string(fragment) + "])</script>")
	result, err := parseCurrentMayhemCatalog(page, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Rows {
		if !mayhemAugmentAvailable(row.ID, "16.21.1") {
			t.Fatal(row.ID)
		}
	}
}

func TestMayhemCurrentCatalogReplacesOldPoolAndKeepsHistoryMetadata(t *testing.T) {
	catalog := bundledCurrentMayhemCatalog()
	if catalog.Citation == nil || catalog.Citation.Patch != "16.21.1" {
		t.Fatal("missing versioned snapshot")
	}
	found := false
	for _, row := range catalog.Rows {
		if row.ID == 2095 || row.ID == 2031 {
			t.Fatal("removed augment remains selectable", row.ID)
		}
		if row.ID == 3048 {
			found = row.Description != "" && row.ImagePath != ""
		}
	}
	if !found {
		t.Fatal("new augment is missing effect/artwork")
	}
	raw, _ := json.Marshal(catalog)
	if strings.Contains(string(raw), "pendingAugments") {
		t.Fatal("obsolete pending payload")
	}
	if !mayhemAugmentAvailable(2031, "16.22.1") {
		t.Fatal("a later reintroduction must not be suppressed")
	}
}

func TestMayhemCurrentDescriptionSupersedesPrePatchCache(t *testing.T) {
	p := newChampionProvider()
	p.cache = newChampionDataCache(&localStore{root: t.TempDir()})
	p.saveAugmentDescription(gameplayAugmentDescription{ID: 3048, Description: "旧版本", Status: "ok", Source: "hexdata"})
	got := p.resolveAugmentDescription(context.Background(), 3048, nil)
	if got.Status != "ok" || got.Description == "旧版本" || got.Description == "" {
		t.Fatal(got)
	}
}

func TestMayhemReadingCachedCitationDoesNotPostponeVersionCheck(t *testing.T) {
	p := newChampionProvider()
	h := newHexdataClient(p, nil)
	now := time.Now()
	h.now = func() time.Time { return now }
	old := now.Add(-13 * time.Hour)
	h.state.BuildID = "hexdata-old"
	h.state.BuildChecked = old
	h.adoptCitation(championSourceCitation{Source: "Hexdata", Patch: "16.20", ReportDate: "2026-10-12", BuildID: "hexdata-old", CanonicalURL: "https://hexdata.com.cn/augments"})
	if !h.snapshot().BuildChecked.Equal(old) {
		t.Fatal("cached citation reset build freshness")
	}
}
