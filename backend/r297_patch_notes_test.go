package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR297PatchNotesBundleStrictDecodeAndLimits(t *testing.T) {
	docs, err := decodePatchNotes(bundledPatchNotes)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 || docs[0].Version != "26.20" || len(docs[0].Modes.Arena.Sections) != 0 {
		t.Fatal(docs)
	}
	clone := func() patchNoteFeed { var v patchNoteFeed; json.Unmarshal(bundledPatchNotes, &v); return v }
	encode := func(v patchNoteFeed) []byte { raw, _ := json.Marshal(v); return raw }
	cases := []struct {
		name string
		edit func(*patchNoteFeed)
	}{
		{"host", func(v *patchNoteFeed) {
			v.Versions[0].SourceURL = "https://lol.qq.com.evil.example/gicp/news/410/1.html"
		}},
		{"http", func(v *patchNoteFeed) { v.Versions[0].SourceURL = "http://lol.qq.com/gicp/news/410/1.html" }},
		{"userinfo", func(v *patchNoteFeed) { v.Versions[0].SourceURL = "https://user@lol.qq.com/gicp/news/410/1.html" }},
		{"missing mode", func(v *patchNoteFeed) { v.Versions[0].Modes.Arena.Sections = nil }},
		{"long text", func(v *patchNoteFeed) { v.Versions[0].Modes.Ranked.Sections[0].Intro = strings.Repeat("字", 4097) }},
		{"date", func(v *patchNoteFeed) { v.Versions[0].PublishedAt = "2026-02-31" }},
		{"duplicate", func(v *patchNoteFeed) { v.Versions[1] = v.Versions[0] }},
		{"count", func(v *patchNoteFeed) { v.Versions[0].Modes.Ranked.Sections[0].Entries = make([]patchNoteEntry, 257) }},
		{"mixed line", func(v *patchNoteFeed) {
			v.Versions[0].Modes.Ranked.Sections[0].Entries[0].Groups[0].Lines[0].K = "unsafe"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := clone()
			tc.edit(&v)
			if _, err := decodePatchNotes(encode(v)); err == nil {
				t.Fatal("invalid feed accepted")
			}
		})
	}
	for _, raw := range [][]byte{append(append([]byte(nil), bundledPatchNotes...), []byte(`{}`)...), bytes.Repeat([]byte{' '}, patchNotesLimit+1), bytes.Replace(bundledPatchNotes, []byte(`"schema": 1`), []byte(`"unexpected":1,"schema":1`), 1)} {
		if _, err := decodePatchNotes(raw); err == nil {
			t.Fatal("invalid raw feed accepted")
		}
	}
}
func TestR297PatchNotesCacheMergeLatestAndAPI(t *testing.T) {
	docs, err := decodePatchNotes(bundledPatchNotes)
	if err != nil {
		t.Fatal(err)
	}
	alias := docs[0]
	alias.Version = "26.20.0"
	if len(mergePatchNotes([]patchNoteDocument{docs[0]}, []patchNoteDocument{alias})) != 1 {
		t.Fatal("equivalent version duplicated")
	}
	if comparePatchVersions("26.9", "26.10") >= 0 {
		t.Fatal("lexical compare")
	}
	newer := docs[0]
	newer.Version = "26.21"
	newer.Modes.Ranked.Sections = []patchNoteSection{}
	raw, _ := json.Marshal(patchNoteFeed{Schema: 1, Versions: []patchNoteDocument{newer}})
	store := &localStore{root: t.TempDir()}
	if err := writeLocalStoreFile(store, "patch-notes.json", raw); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store, token: "patch-test"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/patch-notes", nil)
	r.Header.Set("X-Local-Token", a.token)
	a.authorized(a.handlePatchNotes)(w, r)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"version":"26.21"`)) {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(a.patchNotesState().documents) != 3 {
		t.Fatal("merge missing bundle")
	}
	// Re-read with a new app; invalid remote cache leaves the embedded latest usable.
	if err := writeLocalStoreFile(store, "patch-notes.json", []byte("bad")); err != nil {
		t.Fatal(err)
	}
	a = &app{storage: store}
	if a.patchNotesState().documents[0].Version != "26.20" {
		t.Fatal("offline bundle unavailable")
	}
	_ = os.Remove(store.root + "/patch-notes.json")
}
func TestR297PatchNotesFetchUsesAnnouncementMirrorsAndBoundedContext(t *testing.T) {
	var reads atomic.Int32
	client := &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		reads.Add(1)
		if !strings.Contains(r.URL.String(), announcementBranchURL+"patch-notes.json") {
			t.Errorf("different channel %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(bundledPatchNotes))}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, docs, err := fetchPatchNotesDocument(ctx, client)
	if err != nil || len(docs) != 3 || len(raw) > patchNotesLimit || reads.Load() != 1 {
		t.Fatal(reads.Load(), err)
	}
	client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", patchNotesLimit+1)))}, nil
	})
	if _, _, err := fetchPatchNotesDocument(ctx, client); err == nil {
		t.Fatal("oversized remote accepted")
	}
	expired, stop := context.WithCancel(context.Background())
	stop()
	client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	if _, _, err := fetchPatchNotesDocument(expired, client); err == nil {
		t.Fatal("canceled fetch succeeded")
	}
}
