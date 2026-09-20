package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestLootIconPNGsUseRGBAColorType(t *testing.T) {
	paths, err := filepath.Glob("web/loot-icons/*.png")
	if err != nil || len(paths) == 0 {
		t.Fatalf("loot icon glob: paths=%v err=%v", paths, err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if len(data) < 26 || !bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")) || string(data[12:16]) != "IHDR" {
			t.Fatalf("%s is not a valid PNG with an IHDR header", path)
		}
		if colorType := data[25]; colorType != 6 {
			t.Fatalf("%s PNG color type = %d, want 6 (RGBA)", path, colorType)
		}
		if filepath.Base(path) != "promotion-chest.png" {
			continue
		}
		if width, height := binary.BigEndian.Uint32(data[16:20]), binary.BigEndian.Uint32(data[20:24]); width != 512 || height != 512 {
			t.Fatalf("promotion chest dimensions = %dx%d, want 512x512", width, height)
		}
		decoded, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("decode promotion chest: %v", err)
		}
		_, _, _, cornerAlpha := decoded.At(0, 0).RGBA()
		_, _, _, centerAlpha := decoded.At(decoded.Bounds().Dx()/2, decoded.Bounds().Dy()/2).RGBA()
		if cornerAlpha != 0 || centerAlpha == 0 {
			t.Fatalf("promotion chest alpha: corner=%d center=%d", cornerAlpha, centerAlpha)
		}
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

func TestStatusIncludesBuildFingerprint(t *testing.T) {
	a := &app{}
	recorder := httptest.NewRecorder()
	a.handleStatus(recorder, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var response statusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(response.BuildFingerprint) == "" {
		t.Fatalf("status omitted build fingerprint: %s", recorder.Body.String())
	}
}

func TestEmbeddedFilesystemExcludesDevOnlyFiles(t *testing.T) {
	for _, present := range []string{
		"web/champions.js",
		"web/build-item-row.css",
		"web/index.html",
		"web/app-icon.png",
		"web/image-unavailable.svg",
		"web/arena-team-icons/gromp.svg",
		"web/position-icons/all.svg",
		"web/tier-icons/1.svg",
		"web/rune-styles/precision.svg",
		"web/loot-icons/sanctum-spark.svg",
		"web/loot-icons/hextech-key.png",
		"web/rank-crests/diamond.png",
		// 侧边栏字标字体和它的版权/授权 notice 文本都必须随二进制一起分发：
		// 字体丢了字标会静默跌回系统衬线体（这种回归在开发机上看不出来，
		// 因为开发机上装着别的衬线体），notice 丢了版权来源就没法追溯。
		"web/beaufort-for-lol-bold.woff2",
		"web/beaufort-for-lol-notice.txt",
	} {
		if _, err := embedded.ReadFile(present); err != nil {
			t.Fatalf("expected %s to be embedded, got %v", present, err)
		}
	}
	for _, absent := range []string{
		"web/champions.test.cjs",
		"web/remaining-sort.test.cjs",
		"web/loot-icons/README.md",
	} {
		if _, err := embedded.ReadFile(absent); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("expected %s to be excluded from the embedded binary, got err=%v", absent, err)
		}
	}
}

// Go 内置 MIME 表里没有 .woff2，缺省会回落成 application/octet-stream；
// 我们对所有响应都加了 X-Content-Type-Options: nosniff，类型不对时浏览器
// 有权拒绝加载字体，字标就静默跌回回退字形。这种问题只在打包后的真机上暴露，
// 所以这里直接钉住注册结果，而不是相信各平台的系统 MIME 数据库。
func TestWoff2ContentTypeIsRegistered(t *testing.T) {
	if err := mime.AddExtensionType(".woff2", "font/woff2"); err != nil {
		t.Fatalf("registering woff2 mime type failed: %v", err)
	}
	if got := mime.TypeByExtension(".woff2"); !strings.HasPrefix(got, "font/woff2") {
		t.Fatalf("expected .woff2 to resolve to font/woff2, got %q", got)
	}
}

// 上面两条测试各自证明了"文件被内嵌"和"MIME 类型注册成功"，但都没有验证这两件事
// 拼在一起、走真实的静态文件服务代码路径之后，一次实际的 HTTP 请求到底会不会
// 拿到能被浏览器接受的响应——这正是 R21 内嵌 Cinzel 时踩过的坑："机制测过了，但
// 没测用户屏幕上的结果"（同一类问题见 [[deep-legends-verification-blindspot]]）。
// 这里用 embedded 这个真的 embed.FS（不是手搭的假文件系统）、真的
// http.FileServer(http.FS(...))，跑一次真请求，钉住 Content-Type、WOFF2 魔数、
// 以及字节数和源文件一致（防止以后有人不小心把占位符/空文件提交进去）。
func TestFontAssetsServeWithCorrectContentType(t *testing.T) {
	if err := mime.AddExtensionType(".woff2", "font/woff2"); err != nil {
		t.Fatalf("registering woff2 mime type failed: %v", err)
	}
	webFS, err := fs.Sub(embedded, "web")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	server := httptest.NewServer(http.FileServer(http.FS(webFS)))
	defer server.Close()

	fontResp, err := http.Get(server.URL + "/beaufort-for-lol-bold.woff2")
	if err != nil {
		t.Fatalf("GET woff2: %v", err)
	}
	defer fontResp.Body.Close()
	fontBody, err := io.ReadAll(fontResp.Body)
	if err != nil {
		t.Fatalf("read woff2 body: %v", err)
	}
	if ct := fontResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "font/woff2") {
		t.Fatalf("expected font/woff2 content type, got %q", ct)
	}
	if !bytes.HasPrefix(fontBody, []byte("wOF2")) {
		t.Fatalf("expected WOFF2 magic bytes, got %x", fontBody[:min(4, len(fontBody))])
	}
	wantBody, err := embedded.ReadFile("web/beaufort-for-lol-bold.woff2")
	if err != nil {
		t.Fatalf("read embedded font: %v", err)
	}
	if !bytes.Equal(fontBody, wantBody) {
		t.Fatalf("served font body (%d bytes) does not match embedded file (%d bytes)", len(fontBody), len(wantBody))
	}

	noticeResp, err := http.Get(server.URL + "/beaufort-for-lol-notice.txt")
	if err != nil {
		t.Fatalf("GET notice: %v", err)
	}
	defer noticeResp.Body.Close()
	if noticeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for notice txt, got %d", noticeResp.StatusCode)
	}
	noticeBody, err := io.ReadAll(noticeResp.Body)
	if err != nil {
		t.Fatalf("read notice body: %v", err)
	}
	// notice 必须如实转录字体自带的版权声明和 EULA 链接，不能只是一句空话，
	// 否则版权来源没法追溯（这是这份 notice 存在的唯一理由）。
	for _, want := range []string{"Nick Shinn", "Riot Games", "ShinnType_EULA.pdf"} {
		if !bytes.Contains(noticeBody, []byte(want)) {
			t.Fatalf("expected notice to mention %q, got: %s", want, noticeBody)
		}
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
	store := trackTestStore(t, &localStore{root: t.TempDir()})
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
	large := communityDragonImagePaths("/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Deft_large.png")
	wantLarge := []string{
		"/latest/game/assets/ux/cherry/augments/icons/deft_large.png",
		"/latest/plugins/rcp-be-lol-game-data/global/default/assets/ux/cherry/augments/icons/deft_large.png",
		"/latest/game/assets/ux/cherry/augments/icons/deft.png",
		"/latest/plugins/rcp-be-lol-game-data/global/default/assets/ux/cherry/augments/icons/deft.png",
		"/latest/game/assets/ux/cherry/augments/icons/deft_small.png",
		"/latest/plugins/rcp-be-lol-game-data/global/default/assets/ux/cherry/augments/icons/deft_small.png",
	}
	if len(large) != len(wantLarge) {
		t.Fatalf("large asset fallbacks missing: %#v", large)
	}
	for index := range wantLarge {
		if large[index] != wantLarge[index] {
			t.Fatalf("large asset priority %d = %q, want %q; all=%#v", index, large[index], wantLarge[index], large)
		}
	}
	for _, path := range large {
		if strings.Contains(path, "assets/assets") {
			t.Fatalf("candidate contains duplicated assets segment: %q", path)
		}
	}
	if !containsTestString(large, "/latest/game/assets/ux/cherry/augments/icons/deft_large.png") {
		t.Fatalf("game asset candidate missing: %#v", large)
	}
}

func containsTestString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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

func TestMergeClientLaunchCandidatePreservesOrderAndDeduplicates(t *testing.T) {
	byID := make(map[string]clientInstallation)
	installation := clientInstallation{ID: "tcls", Name: "TCLS", Kind: "tcls"}
	candidates := []clientLaunchCandidate{
		{Source: "shortcut", shortcut: `C:\Users\private\Desktop\英雄联盟.lnk`},
		{Source: "launcher", executable: `D:\League\Launcher\Client.exe`},
		{Source: "tcls", executable: `D:\League\TCLS\Client.exe`},
	}
	for _, candidate := range candidates {
		mergeClientLaunchCandidate(byID, installation, candidate)
	}
	mergeClientLaunchCandidate(byID, installation, candidates[1])

	merged := byID["tcls"]
	if !merged.Available || len(merged.launchCandidates) != 3 {
		t.Fatalf("merged installation = %#v", merged)
	}
	for index, source := range []string{"shortcut", "launcher", "tcls"} {
		if merged.launchCandidates[index].Source != source {
			t.Fatalf("candidate %d source = %q, want %q", index, merged.launchCandidates[index].Source, source)
		}
	}
	if merged.shortcut != candidates[0].shortcut || merged.executable != "" {
		t.Fatalf("primary candidate changed: %#v", merged)
	}
}

func TestWindowsClientDetectionUsesOrderedProductionBuilder(t *testing.T) {
	data, err := os.ReadFile("client_installations_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	start := strings.Index(source, "func detectClientInstallationsWithScan()")
	end := strings.Index(source, "func launchClientInstallation(")
	if start < 0 || end <= start {
		t.Fatal("detectClientInstallationsWithScan production body was not found")
	}
	body := source[start:end]
	if !strings.Contains(body, "buildDetectedClientInstallations(uniquePaths(gameRoots), riotClientCandidates(), shortcuts, regularFile)") {
		t.Fatal("Windows production detector does not use the ordered candidate builder")
	}
}

func TestBuildDetectedClientInstallationsPutsShortcutsLast(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "Launcher", "Client.exe"),
		filepath.Join(root, "TCLS", "Client.exe"),
		filepath.Join(root, "LeagueClient", "LeagueClient.exe"),
		filepath.Join(root, "LeagueClient.exe"),
		filepath.Join(root, "RiotClientServices.exe"),
		filepath.Join(root, "英雄联盟.lnk"),
		filepath.Join(root, "Riot Client.lnk"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	items := buildDetectedClientInstallations(
		[]string{root},
		[]string{filepath.Join(root, "RiotClientServices.exe")},
		[]clientInstallation{
			{ID: "tcls", Name: "英雄联盟", Kind: "tcls", shortcut: filepath.Join(root, "英雄联盟.lnk")},
			{ID: "riot", Name: "Riot 客户端", Kind: "riot", shortcut: filepath.Join(root, "Riot Client.lnk")},
		},
		func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && info.Mode().IsRegular()
		},
	)
	if len(items) != 2 || items[0].ID != "tcls" || items[1].ID != "riot" {
		t.Fatalf("detected installations = %#v", items)
	}
	for index, source := range []string{"launcher", "tcls", "league-client", "league-client", "shortcut"} {
		if got := items[0].launchCandidates[index].Source; got != source {
			t.Fatalf("tcls candidate %d source = %q, want %q", index, got, source)
		}
	}
	for index, source := range []string{"riot", "shortcut"} {
		if got := items[1].launchCandidates[index].Source; got != source {
			t.Fatalf("riot candidate %d source = %q, want %q", index, got, source)
		}
	}
}

func TestLaunchClientCandidatesFallsBackInOrderAndStopsOnSuccess(t *testing.T) {
	installation := clientInstallation{launchCandidates: []clientLaunchCandidate{
		{Source: "shortcut", shortcut: `C:\private\英雄联盟.lnk`},
		{Source: "launcher", executable: `D:\private\Launcher\Client.exe`},
		{Source: "tcls", executable: `D:\private\TCLS\Client.exe`},
		{Source: "league-client", executable: `D:\private\LeagueClient\LeagueClient.exe`},
	}}
	attempted := make([]string, 0, 4)
	result, err := launchClientCandidates(installation, func(candidate clientLaunchCandidate) (clientLaunchFailure, error) {
		attempted = append(attempted, candidate.Source)
		if candidate.Source == "tcls" {
			return clientLaunchFailure{}, nil
		}
		return clientLaunchFailure{Stage: "shell-execute", ErrorCode: 740}, errors.New("private launch failure")
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(attempted, ",") != "shortcut,launcher,tcls" {
		t.Fatalf("attempted = %v", attempted)
	}
	if result.Source != "tcls" || len(result.Failures) != 2 || result.Failures[0].Source != "shortcut" || result.Failures[1].Source != "launcher" {
		t.Fatalf("result = %#v", result)
	}
}

func TestLaunchClientCandidatesReturnsSanitizedFailures(t *testing.T) {
	installation := clientInstallation{launchCandidates: []clientLaunchCandidate{
		{Source: "shortcut", shortcut: `C:\private\英雄联盟.lnk`},
		{Source: `D:\private\Client.exe`, executable: `D:\private\Client.exe`},
	}}
	result, err := launchClientCandidates(installation, func(candidate clientLaunchCandidate) (clientLaunchFailure, error) {
		return clientLaunchFailure{Stage: `D:\private\stage`, ErrorCode: 5}, errors.New("private launch failure")
	})
	if err == nil {
		t.Fatal("all failed candidates returned nil error")
	}
	if len(result.Failures) != 2 || result.Failures[0].Source != "shortcut" || result.Failures[1].Source != "unknown" {
		t.Fatalf("failures = %#v", result.Failures)
	}
	for _, failure := range result.Failures {
		if failure.Stage != "unknown" || failure.ErrorCode != 5 {
			t.Fatalf("unsafe failure = %#v", failure)
		}
	}
}

func TestOfficialLoginLaunchUsesOnlyDetectedTCLSAndRecordsSafeDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	launched := clientInstallation{}
	refreshRequests := make(chan struct{}, 1)
	a := &app{
		storage:         store,
		refreshRequests: refreshRequests,
		clientInstallations: func() []clientInstallation {
			return []clientInstallation{{
				ID: "tcls", Name: "TCLS 客户端", Kind: "tcls", Available: true,
				executable: `C:\private-account\Launcher\Client.exe`,
			}}
		},
		clientLauncher: func(installation clientInstallation) (clientLaunchResult, error) {
			launched = installation
			return clientLaunchResult{Source: "tcls", Failures: []clientLaunchFailure{
				{Source: "shortcut", Stage: "shell-execute", ErrorCode: 2},
				{Source: "launcher", Stage: "shell-execute", ErrorCode: 740},
			}}, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(`{"id":"tcls"}`))
	recorder := httptest.NewRecorder()
	a.handleClientLaunch(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	if launched.ID != "tcls" || !strings.HasSuffix(launched.executable, `Launcher\Client.exe`) {
		t.Fatalf("unexpected launched installation: %#v", launched)
	}
	select {
	case <-refreshRequests:
	default:
		t.Fatal("official login launch did not wake client discovery")
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	logText := string(data)
	if strings.Contains(logText, "private-account") || strings.Contains(logText, "Client.exe") {
		t.Fatalf("official login diagnostic leaked installation details: %s", logText)
	}
	if !strings.Contains(logText, `"event":"official_login_launch"`) || !strings.Contains(logText, `"result":"requested"`) || !strings.Contains(logText, `"result":"attempt_failed"`) || !strings.Contains(logText, `"result":"started"`) || !strings.Contains(logText, `"source":"tcls"`) || !strings.Contains(logText, `"error_code":740`) {
		t.Fatalf("official login diagnostic timeline is incomplete: %s", logText)
	}
}

func TestOfficialLoginLaunchRejectsCredentialFieldsBeforeLaunching(t *testing.T) {
	launched := false
	a := &app{
		clientInstallations: func() []clientInstallation {
			return []clientInstallation{{ID: "tcls", Available: true, executable: `C:\League\Launcher\Client.exe`}}
		},
		clientLauncher: func(clientInstallation) (clientLaunchResult, error) {
			launched = true
			return clientLaunchResult{Source: "launcher"}, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(`{"id":"tcls","password":"must-not-be-read"}`))
	recorder := httptest.NewRecorder()
	a.handleClientLaunch(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if launched {
		t.Fatal("credential-bearing request reached the client launcher")
	}
	if strings.Contains(recorder.Body.String(), "must-not-be-read") {
		t.Fatalf("credential leaked in response: %s", recorder.Body.String())
	}
}

func TestOfficialLoginLaunchRejectsTrailingJSONBeforeLaunching(t *testing.T) {
	launched := false
	a := &app{
		clientInstallations: func() []clientInstallation {
			return []clientInstallation{{ID: "tcls", Available: true, executable: `C:\League\Launcher\Client.exe`}}
		},
		clientLauncher: func(clientInstallation) (clientLaunchResult, error) {
			launched = true
			return clientLaunchResult{Source: "launcher"}, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(`{"id":"tcls"}{"password":"must-not-be-read"}`))
	recorder := httptest.NewRecorder()
	a.handleClientLaunch(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if launched {
		t.Fatal("request with trailing JSON reached the client launcher")
	}
	if strings.Contains(recorder.Body.String(), "must-not-be-read") {
		t.Fatalf("trailing content leaked in response: %s", recorder.Body.String())
	}
}

func TestOfficialLoginLaunchRejectsOversizedBodyBeforeLaunching(t *testing.T) {
	launched := false
	a := &app{
		clientInstallations: func() []clientInstallation {
			return []clientInstallation{{ID: "tcls", Available: true, executable: `C:\League\Launcher\Client.exe`}}
		},
		clientLauncher: func(clientInstallation) (clientLaunchResult, error) {
			launched = true
			return clientLaunchResult{Source: "launcher"}, nil
		},
	}
	body := `{"id":"tcls"}` + strings.Repeat(" ", 5000)
	request := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	a.handleClientLaunch(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if launched {
		t.Fatal("oversized request reached the client launcher")
	}
}

func TestClientLaunchSerializesRequestsAndAppliesSuccessCooldown(t *testing.T) {
	var launches atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	a := &app{
		clientInstallations: func() []clientInstallation {
			return []clientInstallation{{ID: "tcls", Available: true, executable: `C:\League\Launcher\Client.exe`}}
		},
		clientLauncher: func(clientInstallation) (clientLaunchResult, error) {
			if launches.Add(1) == 1 {
				close(started)
				<-release
			}
			return clientLaunchResult{Source: "launcher"}, nil
		},
	}
	firstResult := make(chan int, 1)
	go func() {
		request := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(`{"id":"tcls"}`))
		recorder := httptest.NewRecorder()
		a.handleClientLaunch(recorder, request)
		firstResult <- recorder.Code
	}()
	<-started

	secondRequest := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(`{"id":"tcls"}`))
	secondRecorder := httptest.NewRecorder()
	a.handleClientLaunch(secondRecorder, secondRequest)
	if secondRecorder.Code != http.StatusTooManyRequests || launches.Load() != 1 {
		t.Fatalf("concurrent status/launches = %d/%d, want %d/1", secondRecorder.Code, launches.Load(), http.StatusTooManyRequests)
	}

	close(release)
	if code := <-firstResult; code != http.StatusAccepted {
		t.Fatalf("first status = %d, want %d", code, http.StatusAccepted)
	}
	thirdRequest := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(`{"id":"tcls"}`))
	thirdRecorder := httptest.NewRecorder()
	a.handleClientLaunch(thirdRecorder, thirdRequest)
	if thirdRecorder.Code != http.StatusTooManyRequests || launches.Load() != 1 {
		t.Fatalf("cooldown status/launches = %d/%d, want %d/1", thirdRecorder.Code, launches.Load(), http.StatusTooManyRequests)
	}
}

func TestClientLaunchFailureReleasesServerLock(t *testing.T) {
	var launches atomic.Int32
	a := &app{
		clientInstallations: func() []clientInstallation {
			return []clientInstallation{{ID: "tcls", Available: true, executable: `C:\League\Launcher\Client.exe`}}
		},
		clientLauncher: func(clientInstallation) (clientLaunchResult, error) {
			if launches.Add(1) == 1 {
				return clientLaunchResult{}, errors.New("first launch failed")
			}
			return clientLaunchResult{Source: "launcher"}, nil
		},
	}
	for attempt, want := range []int{http.StatusInternalServerError, http.StatusAccepted} {
		request := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(`{"id":"tcls"}`))
		recorder := httptest.NewRecorder()
		a.handleClientLaunch(recorder, request)
		if recorder.Code != want {
			t.Fatalf("attempt %d status = %d, want %d", attempt+1, recorder.Code, want)
		}
	}
	if launches.Load() != 2 {
		t.Fatalf("launches = %d, want 2", launches.Load())
	}
}

func TestOfficialLoginLaunchFailureDoesNotLogUnderlyingError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{
		storage: store,
		clientInstallations: func() []clientInstallation {
			return []clientInstallation{{ID: "tcls", Available: true, executable: `C:\League\Launcher\Client.exe`}}
		},
		clientLauncher: func(clientInstallation) (clientLaunchResult, error) {
			return clientLaunchResult{Failures: []clientLaunchFailure{{Source: "launcher", Stage: "shell-execute", ErrorCode: 740}}}, errors.New("private path and launcher-token must stay private")
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/client-launch", strings.NewReader(`{"id":"tcls"}`))
	recorder := httptest.NewRecorder()
	a.handleClientLaunch(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	combined := recorder.Body.String() + string(data)
	if strings.Contains(combined, "private path") || strings.Contains(combined, "launcher-token") {
		t.Fatalf("official login failure leaked underlying error: %s", combined)
	}
	if !strings.Contains(string(data), `"result":"failed"`) || !strings.Contains(string(data), `"result":"attempt_failed"`) || !strings.Contains(string(data), `"source":"launcher"`) || !strings.Contains(string(data), `"stage":"shell-execute"`) || !strings.Contains(string(data), `"error_code":740`) {
		t.Fatalf("official login failure was not diagnosed: %s", data)
	}
}

func TestClientInstallationsDoNotExposeExecutableField(t *testing.T) {
	a := &app{
		token: "test-secret",
		clientInstallations: func() []clientInstallation {
			return []clientInstallation{{ID: "tcls", Name: "TCLS", Location: `C:\private\英雄联盟`, Available: true, executable: `C:\private\Client.exe`, shortcut: `C:\private\英雄联盟.lnk`, arguments: []string{"--private"}, launchCandidates: []clientLaunchCandidate{{Source: "launcher", executable: `C:\private\Launcher\Client.exe`, arguments: []string{"--candidate-private"}}}}}
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/api/client-installations", nil)
	recorder := httptest.NewRecorder()
	a.handleClientInstallations(recorder, request)
	response := recorder.Body.String()
	if strings.Contains(response, "location") || strings.Contains(response, "executable") || strings.Contains(response, "shortcut") || strings.Contains(response, "arguments") || strings.Contains(response, "launchCandidates") || strings.Contains(response, `C:\private`) || strings.Contains(response, "candidate-private") || strings.Contains(response, "英雄联盟") {
		t.Fatalf("private launch fields leaked: %s", recorder.Body.String())
	}
}

func TestClientInstallationScanDiagnosticDoesNotExposePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{
		storage: store,
		clientInstallations: func() []clientInstallation {
			return []clientInstallation{{ID: "tcls", Available: true, Location: `D:\private-account\英雄联盟`, executable: `D:\private-account\英雄联盟\Launcher\Client.exe`}}
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/api/client-installations", nil)
	recorder := httptest.NewRecorder()
	a.handleClientInstallations(recorder, request)
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	logText := string(data)
	if strings.Contains(logText, "private-account") || strings.Contains(logText, "Client.exe") || strings.Contains(logText, "英雄联盟") {
		t.Fatalf("installation scan diagnostic leaked paths: %s", logText)
	}
	if !strings.Contains(logText, `"event":"client_installations_scan"`) || !strings.Contains(logText, `"detected_ids":["tcls"]`) || !strings.Contains(logText, `"result":"detected"`) {
		t.Fatalf("installation scan diagnostic is incomplete: %s", logText)
	}
}

func TestClassifyClientShortcut(t *testing.T) {
	tests := map[string]string{
		"英雄联盟Wegame版.lnk":                   "",
		"WeGame.lnk":                        "",
		"wegame英雄联盟.lnk":                    "",
		"英雄联盟.lnk":                          "tcls",
		"TCLS.lnk":                          "tcls",
		"League of Legends.lnk":             "tcls",
		"卸载英雄联盟.lnk":                        "",
		"英雄联盟卸载.lnk":                        "",
		"英雄联盟 - 卸载.lnk":                     "",
		"League of Legends Uninstaller.lnk": "",
		"League of Legends uninstall.lnk":   "",
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
