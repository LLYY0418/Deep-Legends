package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBootstrapSetsHttpOnlyCookieAndRedirects(t *testing.T) {
	a := &app{token: "test-secret"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/?bootstrap=test-secret", nil)
	recorder := httptest.NewRecorder()
	a.handleBootstrap(next).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusSeeOther)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "lol_loot_token" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected cookie: %#v", cookies)
	}
}

func TestBootstrapReissuesCookieForTrustedNavigation(t *testing.T) {
	a := &app{token: "test-secret"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	request.Header.Set("Sec-Fetch-Dest", "document")
	request.Header.Set("Sec-Fetch-Site", "none")
	recorder := httptest.NewRecorder()
	a.handleBootstrap(next).ServeHTTP(recorder, request)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != "test-secret" || !cookies[0].HttpOnly || cookies[0].MaxAge <= 0 {
		t.Fatalf("trusted navigation should reissue session cookie, got %#v", cookies)
	}
}

func TestBootstrapIgnoresCrossSiteAndNonNavigationRequests(t *testing.T) {
	a := &app{token: "test-secret"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	cases := map[string]map[string]string{
		"cross-site navigation": {"Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document", "Sec-Fetch-Site": "cross-site"},
		"same-origin fetch":     {"Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty", "Sec-Fetch-Site": "same-origin"},
		"no sec-fetch headers":  {},
	}
	for name, headers := range cases {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		recorder := httptest.NewRecorder()
		a.handleBootstrap(next).ServeHTTP(recorder, request)
		if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
			t.Fatalf("%s should not receive a session cookie, got %#v", name, cookies)
		}
	}
}

func TestSessionTokenPersistsAcrossRestarts(t *testing.T) {
	store := &localStore{root: t.TempDir()}
	first, err := loadOrCreateSessionToken(store)
	if err != nil {
		t.Fatal(err)
	}
	if !isSessionToken(first) {
		t.Fatalf("generated token has unexpected shape: %q", first)
	}
	second, err := loadOrCreateSessionToken(store)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("token changed across restarts: %q != %q", second, first)
	}
}

func TestAuthorizedAcceptsCookieAndRejectsMissingToken(t *testing.T) {
	a := &app{token: "test-secret"}
	handler := a.authorized(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	missingRequest := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	missingRecorder := httptest.NewRecorder()
	handler(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d", missingRecorder.Code)
	}

	validRequest := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	validRequest.AddCookie(&http.Cookie{Name: "lol_loot_token", Value: "test-secret"})
	validRecorder := httptest.NewRecorder()
	handler(validRecorder, validRequest)
	if validRecorder.Code != http.StatusNoContent {
		t.Fatalf("valid token status = %d", validRecorder.Code)
	}
}

func TestImagePathTraversalRejected(t *testing.T) {
	a := &app{token: "test-secret"}
	request := httptest.NewRequest(http.MethodGet, "/api/image?path=%2Flol-game-data%2Fassets%2F..%2Flol-summoner%2Fv1%2Fcurrent-summoner", nil)
	recorder := httptest.NewRecorder()
	a.handleImage(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestCommunityDragonImagePathsCoverPluginAndGameAssets(t *testing.T) {
	paths := communityDragonImagePaths("/lol-game-data/assets/UX/Cherry/TeamIcons/TeamPoros.png")
	want := []string{
		"/latest/plugins/rcp-be-lol-game-data/global/default/ux/cherry/teamicons/teamporos.png",
		"/latest/game/assets/ux/cherry/teamicons/teamporos.png",
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %#v", paths)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("path %d = %q, want %q", index, paths[index], want[index])
		}
	}
	if paths := communityDragonImagePaths("https://example.com/private.png"); paths != nil {
		t.Fatalf("external path was accepted: %#v", paths)
	}
}

func TestPrestigeArtworkURLIsFixedToTencentImageHost(t *testing.T) {
	got, ok := prestigeImageURL("11914b2b-f986-474e-b3f7-1e8cc41b72c9")
	if !ok || got != "https://game.gtimg.cn/images/lol/act/a20230715chromahub/skin/site3-11914b2b-f986-474e-b3f7-1e8cc41b72c9.jpg" {
		t.Fatalf("prestige artwork URL = %q/%v", got, ok)
	}
	if _, ok := prestigeImageURL("../../account-token"); ok {
		t.Fatal("invalid prestige artwork identifier was accepted")
	}
}

func TestPrestigeImageRejectsUnknownCatalogIDWithoutNetwork(t *testing.T) {
	a := &app{assetCache: make(map[string][]byte)}
	request := httptest.NewRequest(http.MethodGet, "/api/prestige-image?id=999999999", nil)
	recorder := httptest.NewRecorder()
	a.handlePrestigeImage(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestSkinArtworkURLIsFixedToTencentImageHost(t *testing.T) {
	got, ok := skinArtworkURL(15001)
	if !ok || got != "https://game.gtimg.cn/images/lol/act/img/skin/big15001.jpg" {
		t.Fatalf("skin artwork URL = %q/%v", got, ok)
	}
	for _, id := range []int64{0, -1, 999, 10_000_000} {
		if _, ok := skinArtworkURL(id); ok {
			t.Fatalf("invalid skin ID %d was accepted", id)
		}
	}
}

func TestSkinArtRejectsInvalidIDWithoutNetwork(t *testing.T) {
	a := &app{assetCache: make(map[string][]byte)}
	for _, raw := range []string{"abc", "", "999", "10000000"} {
		request := httptest.NewRequest(http.MethodGet, "/api/skin-art?id="+raw, nil)
		recorder := httptest.NewRecorder()
		a.handleSkinArt(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("id %q status = %d, want %d", raw, recorder.Code, http.StatusBadRequest)
		}
	}
}

func TestSkinArtServesValidatedCachedArtwork(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	a := &app{assetCache: map[string][]byte{"skin-art:15001": jpeg}}
	request := httptest.NewRequest(http.MethodGet, "/api/skin-art?id=15001", nil)
	recorder := httptest.NewRecorder()
	a.handleSkinArt(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/jpeg" || !bytes.Equal(recorder.Body.Bytes(), jpeg) {
		t.Fatalf("cached skin art response = status %d, type %q, body %x", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.Bytes())
	}
}

func TestPrestigeImageServesOnlyValidatedCachedArtwork(t *testing.T) {
	catalog, err := loadPrestigeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	metadata := catalog[10082]
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	a := &app{assetCache: map[string][]byte{"prestige:" + metadata.InstanceID: jpeg}}
	request := httptest.NewRequest(http.MethodGet, "/api/prestige-image?id=10082", nil)
	recorder := httptest.NewRecorder()
	a.handlePrestigeImage(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/jpeg" || !bytes.Equal(recorder.Body.Bytes(), jpeg) {
		t.Fatalf("cached prestige response = status %d, type %q, body %x", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.Bytes())
	}
}

func TestDesktopReadyUsesSingleMachineReadableLine(t *testing.T) {
	var output bytes.Buffer
	if err := writeDesktopReady(&output, "http://127.0.0.1:41000", "http://127.0.0.1:41000/?bootstrap=secret", strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(output.String())
	if !strings.HasPrefix(line, "LOOT_READY ") || strings.Count(line, "\n") != 0 {
		t.Fatalf("unexpected ready line: %q", line)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "LOOT_READY ")), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["baseUrl"] != "http://127.0.0.1:41000" || payload["token"] != strings.Repeat("a", 48) {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestClientLaunchRejectsUnknownInstallation(t *testing.T) {
	a := &app{token: "test-secret"}
	request := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(`{"id":"arbitrary-path"}`))
	recorder := httptest.NewRecorder()
	a.handleClientLaunch(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestClientInstallationsDoNotExposeExecutableField(t *testing.T) {
	a := &app{token: "test-secret"}
	request := httptest.NewRequest(http.MethodGet, "/api/client-installations", nil)
	recorder := httptest.NewRecorder()
	a.handleClientInstallations(recorder, request)
	if strings.Contains(recorder.Body.String(), "executable") || strings.Contains(recorder.Body.String(), "shortcut") || strings.Contains(recorder.Body.String(), "arguments") {
		t.Fatalf("private launch fields leaked: %s", recorder.Body.String())
	}
}

func TestClassifyClientShortcut(t *testing.T) {
	tests := map[string]string{
		"英雄联盟.lnk":                          "tcls",
		"League of Legends.lnk":             "tcls",
		"英雄联盟卸载.lnk":                        "",
		"英雄联盟 - 卸载.lnk":                     "",
		"League of Legends Uninstaller.lnk": "",
		"League of Legends uninstall.lnk":   "",
		"WeGame.lnk":                        "wegame",
		"Riot Client.lnk":                   "riot",
		"其他工具.lnk":                          "",
	}
	for name, expected := range tests {
		id, _, _, _ := classifyClientShortcut(name)
		if id != expected {
			t.Fatalf("shortcut %q classified as %q, want %q", name, id, expected)
		}
	}
}

func TestPoolSkinsReturnsOnlySelectedPoolCatalog(t *testing.T) {
	a := &app{
		connected:     true,
		snapshotReady: true,
		poolTotal:     2,
		poolMatched:   2,
		poolID:        "selected",
		pools:         map[string]PoolManifest{"selected": {ID: "selected", Name: "当前奖池"}},
		allSkins:      []Skin{{ID: 1001, Name: "奖池一", PoolName: "奖池一", Owned: true}, {ID: 1002, Name: "奖池二", PoolName: "奖池二"}, {ID: 1003, Name: "目录外"}},
	}
	recorder := httptest.NewRecorder()
	a.handlePoolSkins(recorder, httptest.NewRequest(http.MethodGet, "/api/pool-skins", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Items    []Skin `json:"items"`
		PoolName string `json:"poolName"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.PoolName != "当前奖池" || len(payload.Items) != 2 || payload.Items[0].ID != 1001 || payload.Items[1].ID != 1002 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestSnapshotPhaseOnlyReportsConnectedAfterSnapshotIsReady(t *testing.T) {
	client := &LCUClient{}
	a := &app{lcu: client, connected: true, connectionState: "connected"}
	a.setSnapshotPhase(client)
	if a.connectionState != "connecting" {
		t.Fatalf("phase = %q, want connecting while snapshot is unavailable", a.connectionState)
	}
	a.snapshotReady = true
	a.setSnapshotPhase(client)
	if a.connectionState != "connected" {
		t.Fatalf("phase = %q, want connected after snapshot is ready", a.connectionState)
	}
}
