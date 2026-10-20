package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestR297PlayerTagsPersistenceAPIAndImport(t *testing.T) {
	s := &localStore{root: t.TempDir()}
	a := &app{storage: s, token: "tags-fixture"}
	rawID := "11111111-1111-4111-8111-111111111111"
	ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: rawID, GameName: "测试玩家", TagLine: "123", ProfileIconID: 7})
	request := func(method string, value any) (int, []byte) {
		t.Helper()
		body, _ := json.Marshal(value)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, "http://127.0.0.1/api/player-tags", bytes.NewReader(body))
		r.Header.Set("X-Local-Token", a.token)
		a.authorized(a.handlePlayerTags)(w, r)
		return w.Code, w.Body.Bytes()
	}
	update := map[string]any{"op": "update", "playerRef": ref, "tags": []string{"preset-2", "preset-8"}, "note": "这次会沟通"}
	if code, body := request("POST", update); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	raw, err := os.ReadFile(filepath.Join(s.root, "player-tags.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v playerTagsFile
	if json.Unmarshal(raw, &v) != nil || v.Players[rawID].Note != "这次会沟通" {
		t.Fatal(string(raw))
	}
	info, err := os.Stat(filepath.Join(s.root, "player-tags.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Windows FileMode does not express POSIX owner-only permission bits.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	// A new app/token exposes only new opaque references, retaining the same machine records.
	a = &app{storage: &localStore{root: s.root}, token: "new-token"}
	code, body := request("GET", nil)
	if code != 200 || bytes.Contains(body, []byte(rawID)) {
		t.Fatalf("public list exposed identity: %d %s", code, body)
	}
	var list struct {
		Players []struct {
			PlayerRef string `json:"playerRef"`
		}
	}
	json.Unmarshal(body, &list)
	if len(list.Players) != 1 {
		t.Fatal(string(body))
	}
	ref = list.Players[0].PlayerRef
	incoming := v
	incoming.Players = map[string]playerTagRecord{rawID: {Tags: []string{"preset-9"}, Note: "较新备注", CreatedAt: 1, UpdatedAt: v.Players[rawID].UpdatedAt + 1}}
	code, body = request("POST", map[string]any{"op": "import", "import": incoming, "commit": false})
	if code != 200 || !bytes.Contains(body, []byte(`"updated":1`)) {
		t.Fatal(code, string(body))
	}
	after, _ := os.ReadFile(filepath.Join(s.root, "player-tags.json"))
	if !bytes.Equal(after, raw) {
		t.Fatal("preview wrote disk")
	}
	code, body = request("POST", map[string]any{"op": "import", "import": incoming, "commit": true})
	if code != 200 {
		t.Fatal(code, string(body))
	}
	v, _ = s.readPlayerTagsLocked()
	if len(v.Players[rawID].Tags) != 3 || v.Players[rawID].Note != "较新备注" {
		t.Fatal(v.Players[rawID])
	}
	hidden := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: rawID, TagBlocked: true})
	code, body = request("POST", map[string]any{"op": "query", "playerRefs": []string{hidden, rawID}})
	if code != 200 || bytes.Contains(body, []byte("较新备注")) {
		t.Fatal("hidden query leak", string(body))
	}
	code, _ = request("POST", map[string]any{"op": "update", "playerRef": hidden, "tags": []string{"preset-1"}})
	if code != 404 {
		t.Fatal("hidden edit allowed", code)
	}
	// The settings list re-registers historical names but must never clear a live visibility block.
	if status, _ := request("GET", nil); status != 200 {
		t.Fatal(status)
	}
	a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: rawID, GameName: "historical-name"})
	code, body = request("POST", map[string]any{"op": "query", "playerRefs": []string{hidden}})
	if bytes.Contains(body, []byte("较新备注")) {
		t.Fatal("historical registration cleared hidden state")
	}
	ref = a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: rawID, GameName: "测试玩家", TagVisibilityKnown: true})
	code, body = request("POST", map[string]any{"op": "update", "playerRef": ref, "tags": []string{}, "note": ""})
	if code != 200 {
		t.Fatal(code, string(body))
	}
	v, _ = s.readPlayerTagsLocked()
	if len(v.Players) != 0 {
		t.Fatal("empty record retained")
	}
	files, _ := filepath.Glob(filepath.Join(s.root, ".tmp-*"))
	if len(files) != 0 {
		t.Fatal(files)
	}
}
func TestR297PlayerTagLimitsAndLastMet(t *testing.T) {
	v := playerTagsFile{Schema: 1, Catalog: defaultPlayerTags(), Players: map[string]playerTagRecord{}}
	for i := 0; i < 5000; i++ {
		v.Players[fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)] = playerTagRecord{Tags: []string{"preset-1"}}
	}
	if err := validatePlayerTags(v); err != nil {
		t.Fatal(err)
	}
	v.Players["99999999-1111-4111-8111-111111111111"] = playerTagRecord{}
	if validatePlayerTags(v) == nil {
		t.Fatal("5001 players accepted")
	}
	v.Players = map[string]playerTagRecord{"11111111-1111-4111-8111-111111111111": {Note: strings.Repeat("字", 41)}}
	if validatePlayerTags(v) == nil {
		t.Fatal("41 chars accepted")
	}
	if validCustomTagName("七个汉字太长了") || !validCustomTagName("六个汉字刚好") || !validCustomTagName("123456789012") || validCustomTagName("1234567890123") {
		t.Fatal("name limit")
	}
	bad := v
	bad.Catalog = append([]playerTag(nil), v.Catalog...)
	bad.Catalog[0].Preset = false
	bad.Catalog[0].Name = "改名"
	bad.Players = map[string]playerTagRecord{}
	if validatePlayerTags(bad) == nil {
		t.Fatal("preset renamed by clearing flag")
	}
	s := &localStore{root: t.TempDir()}
	a := &app{storage: s, token: "fixture"}
	id := "11111111-1111-4111-8111-111111111111"
	ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: id, GameName: "visible"})
	v.Players = map[string]playerTagRecord{id: {Tags: []string{"preset-1"}}}
	if err := s.writePlayerTagsLocked(&v); err != nil {
		t.Fatal(err)
	}
	live := gameplayLiveResponse{GameID: 77, QueueID: 420, Players: []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: ref}, ChampionID: 13, ChampionName: "瑞兹"}}}
	a.rememberTaggedLivePlayers(live)
	first, _ := os.ReadFile(filepath.Join(s.root, "player-tags.json"))
	a.rememberTaggedLivePlayers(live)
	second, _ := os.ReadFile(filepath.Join(s.root, "player-tags.json"))
	if !bytes.Equal(first, second) {
		t.Fatal("lastMet wrote twice for same game")
	}
	var got playerTagsFile
	json.Unmarshal(first, &got)
	if got.Players[id].LastMet.GameID != 77 {
		t.Fatal(string(first))
	}
}

func TestR297PlayerTagsBrowser(t *testing.T) {
	if os.Getenv("R297_TAGS_BROWSER") != "1" {
		t.Skip("optional Chromium proof")
	}
	a := &app{storage: &localStore{root: t.TempDir()}, token: "browser"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/player-tags", a.handlePlayerTags)
	mux.HandleFunc("/fixture/register-tags", func(w http.ResponseWriter, r *http.Request) {
		var items []gameplayPlayer
		if json.NewDecoder(r.Body).Decode(&items) != nil {
			http.Error(w, "fixture", 400)
			return
		}
		result := []string{}
		for i, p := range items {
			result = append(result, a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: fmt.Sprintf("r297-browser-player-%04d", i), GameName: p.GameName, DisplayName: p.DisplayName, TagLine: p.TagLine, ProfileIconID: p.ProfileIconID, TagBlocked: p.Hidden}))
		}
		respondJSON(w, result)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	cmd := exec.Command("node", "../scripts/r297-player-tags-browser.cjs", server.URL)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestR297FeatureAssetsEmbedded(t *testing.T) {
	for _, name := range []string{"web/player-tags.js", "web/player-tags.css", "web/ui-preferences.js"} {
		raw, err := embedded.ReadFile(name)
		if err != nil || len(raw) == 0 {
			t.Fatalf("missing installed asset %s: %v", name, err)
		}
	}
}
