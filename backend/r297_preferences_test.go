package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestR297UIPreferencesPersistAcrossOriginsAndMigration(t *testing.T) {
	s := &localStore{root: t.TempDir()}
	a := &app{storage: s, token: "fixture-token"}
	request := func(method, body string) uiPreferences {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, "http://127.0.0.1:47391/api/ui-preferences", bytes.NewBufferString(body))
		r.Header.Set("X-Local-Token", a.token)
		a.authorized(a.handleUIPreferences)(w, r)
		if w.Code != 200 {
			t.Fatalf("%d: %s", w.Code, w.Body)
		}
		var got uiPreferences
		json.Unmarshal(w.Body.Bytes(), &got)
		return got
	}
	request("POST", `{"mode":"migrate","values":{"lol-loot-settings-app-seen":"true","lol-loot-theme":"dark","deep-legends-announcements-dismissed-v1":"[\"notice:1\"]"}}`)
	// A new backend and a different renderer origin retain the authoritative file.
	a = &app{storage: &localStore{root: s.root}, token: "new-session"}
	got := request("POST", `{"mode":"migrate","values":{"lol-loot-theme":"light","lol-loot-mask-names":"true"}}`)
	if got.Values["lol-loot-theme"] != "dark" || got.Values["lol-loot-mask-names"] != "true" || got.Values["lol-loot-settings-app-seen"] != "true" {
		t.Fatal(got)
	}
	request("POST", `{"mode":"update","values":{"lol-loot-theme":"light","lol-loot-mask-names":null}}`)
	got = request("GET", "")
	if got.Values["lol-loot-theme"] != "light" || got.Values["lol-loot-mask-names"] != "" {
		t.Fatal(got)
	}
	info, err := os.Stat(filepath.Join(s.root, "ui-preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatal("preferences permissions")
	}
	files, _ := filepath.Glob(filepath.Join(s.root, ".tmp-*"))
	if len(files) != 0 {
		t.Fatal("atomic write residue")
	}
	for _, body := range []string{`{"mode":"update","values":{"token":"secret"}}`, `{"mode":"update","values":{"lol-loot-demo":"1"}}`, `{"mode":"update","values":{}}{}`} {
		w := httptest.NewRecorder()
		a.handleUIPreferences(w, httptest.NewRequest("POST", "/api/ui-preferences", bytes.NewBufferString(body)))
		if w.Code != 400 {
			t.Fatal(w.Code, body)
		}
	}
	w := httptest.NewRecorder()
	a.authorized(a.handleUIPreferences)(w, httptest.NewRequest("GET", "/api/ui-preferences", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatal("unauthorized preferences read")
	}
}
func TestR297FixedUIPortWaitsForOldProcess(t *testing.T) {
	old, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := old.Addr().String()
	done := make(chan struct{})
	go func() { defer close(done); time.Sleep(250 * time.Millisecond); old.Close() }()
	defer func() { old.Close(); <-done }()
	started := time.Now()
	listener, err := listenLocalUI(address, true)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if listener.Addr().String() != address {
		t.Fatal("fell back before old process released fixed port")
	}
	if time.Since(started) > time.Second {
		t.Fatal("unnecessary fixed port retry delay")
	}
}

func TestR297PreferencesBrowser(t *testing.T) {
	if os.Getenv("R297_PREFERENCES_BROWSER") != "1" {
		t.Skip("requires Chromium browser proof")
	}
	a := &app{storage: &localStore{root: t.TempDir()}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ui-preferences", a.handleUIPreferences)
	mux.HandleFunc("/api/announcements", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, map[string]any{"source": "fixture", "announcements": []map[string]any{{"id": "r297", "revision": "1", "popup": true, "title": "公告", "summary": "本地持久化夹具", "content": "本地持久化夹具"}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	cmd := exec.Command("node", "../scripts/r297-preferences-browser.cjs", server.URL)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
