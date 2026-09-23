package main

import (
	"context"
	"encoding/json"
	"fmt"
	xhtml "golang.org/x/net/html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR107PublicMatchStartAndIdentity(t *testing.T) {
	data, err := os.ReadFile("testdata/r107/opgg-profile.html")
	if err != nil {
		t.Fatal(err)
	}
	ref := gameplayReference{GameName: "Kimman", TagLine: "zxfkk"}
	row, err := parseProProfile(data, ref)
	if err != nil || !row.LastMatchAtKnown || row.LastMatchAt != "2026-09-16T18:19:58Z" {
		t.Fatalf("activity=%s known=%t err=%v", row.LastMatchAt, row.LastMatchAtKnown, err)
	}
	if row.LastMatchAt == row.UpdatedAt {
		t.Fatal("profile refresh substituted for match start")
	}
	wrong := strings.ReplaceAll(string(data), "Kimman-zxfkk#summoner", "Another-KR1#summoner")
	row, err = parseProProfile([]byte(wrong), ref)
	if err != nil || row.LastMatchAtKnown {
		t.Fatalf("accepted another agent's history: %+v %v", row, err)
	}
}

func TestR107ProfileCacheRestoresActivityAfterRestart(t *testing.T) {
	data, _ := os.ReadFile("testdata/r107/opgg-profile.html")
	calls := 0
	client := &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return proHTTPBody(data), nil })}
	store := &localStore{root: t.TempDir()}
	old := opggProAccount{GameName: "Kimman", TagLine: "zxfkk"}
	first := (&app{storage: store, champions: &championProvider{client: client}}).readProProfile(context.Background(), old)
	second := (&app{storage: store, champions: &championProvider{client: client}}).readProProfile(context.Background(), old)
	if !first.LastMatchAtKnown || second.LastMatchAt != first.LastMatchAt || !second.LastMatchAtKnown || calls != 1 {
		t.Fatalf("activity lost across restart: %s %s calls=%d", first.LastMatchAt, second.LastMatchAt, calls)
	}
}

func TestR107EmptyLocalBackgroundReadsSameAccountProfile(t *testing.T) {
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lol-summoner/v1/summoner-profile" {
			if r.URL.Query().Get("puuid") != "current-account" {
				t.Error("wrong account queried")
			}
			fmt.Fprint(w, `{"backgroundSkinId":64000,"backgroundSkinName":"盲僧"}`)
		} else {
			fmt.Fprint(w, `{"backgroundSkinId":0}`)
		}
	})
	profile, cap := new(app).loadFacadeProfile(context.Background(), client, Summoner{PUUID: "current-account"})
	if cap.State != capabilityAvailable || profile.BackgroundSkinID != 64000 {
		t.Fatalf("%+v %+v", profile, cap)
	}
}

func TestR107DoubleEscapedPublicMatchIdentity(t *testing.T) {
	for _, name := range []string{"Hide on bush", "모든일은같이"} {
		for _, foreign := range []bool{false, true} {
			slug := name + "-KR1"
			if foreign {
				slug += "extra"
			}
			link := "https://op.gg/zh-cn/lol/summoners/kr/" + url.PathEscape(url.PathEscape(slug)) + "#summoner"
			body, _ := json.Marshal(map[string]any{"@type": "PlayGameAction", "agent": map[string]any{"@id": link}, "startTime": "2026-09-16T18:19:58Z"})
			doc, _ := xhtml.Parse(strings.NewReader(`<script type="application/ld+json">` + string(body) + `</script>`))
			_, known := proProfileLastMatch(doc, gameplayReference{GameName: name, TagLine: "KR1"})
			if known == foreign {
				t.Fatalf("name=%s foreign=%t known=%t", name, foreign, known)
			}
		}
	}
}

func TestR107MissingPublicActivityUsesOwnRiotIdentityAndCaches(t *testing.T) {
	for _, public := range []bool{false, true} {
		t.Run(fmt.Sprint(public), func(t *testing.T) {
			data, _ := os.ReadFile("testdata/r107/opgg-profile.html")
			if !public {
				data = []byte(strings.ReplaceAll(string(data), "PlayGameAction", "NoHistory"))
			}
			calls := 0
			p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
				calls++
				switch {
				case strings.Contains(r.URL.Path, "/accounts/by-riot-id/"):
					return r99Response(`{"puuid":"own-key-identity","gameName":"Kimman","tagLine":"zxfkk"}`), nil
				case strings.Contains(r.URL.Path, "/by-puuid/own-key-identity/ids"):
					if r.URL.Query().Get("count") != "1" {
						t.Error("must fetch only latest match")
					}
					return r99Response(`["KR_107"]`), nil
				case strings.HasSuffix(r.URL.Path, "/matches/KR_107"):
					return r102Match("KR_107", 1789582800000), nil
				default:
					t.Fatalf("unexpected Riot lookup: %s", r.URL.Path)
					return nil, nil
				}
			})
			a := &app{riot: p, champions: &championProvider{client: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { return proHTTPBody(data), nil })}}}
			row := opggProAccount{GameName: "Kimman", TagLine: "zxfkk"}
			first := a.readProProfile(context.Background(), row)
			second := a.readProProfile(context.Background(), row)
			if !first.LastMatchAtKnown || first.ActivityFailed || second.LastMatchAt != first.LastMatchAt {
				t.Fatalf("activity: %+v", first)
			}
			want := 3
			if public {
				want = 0
			}
			if calls != want {
				t.Fatalf("requests=%d want=%d", calls, want)
			}
		})
	}
}

// Opt-in read-only verification against the configured public sources.
func TestR107LiveProfileActivityBatch(t *testing.T) {
	output := os.Getenv("R107_LIVE_PROFILE_OUTPUT")
	if output == "" {
		t.Skip("opt-in network verification")
	}
	key, err := os.ReadFile("../riot_key.local.txt")
	if err != nil {
		t.Fatal("local key unavailable")
	}
	t.Setenv("RIOT_API_KEY", strings.TrimSpace(string(key)))
	cp := newChampionProvider()
	a := &app{champions: cp, riot: newRiotProvider(cp)}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	page := a.buildReviewedProPlayers(a.enrichProProfiles(ctx, new(app).loadProSeeds(ctx, nil)))
	body, _ := json.MarshalIndent(page, "", "  ")
	if err := os.WriteFile(output, body, 0600); err != nil {
		t.Fatal(err)
	}
	total, ranked, known := 0, 0, 0
	for _, team := range page.Teams {
		for _, player := range team.Players {
			for _, row := range player.Accounts {
				total++
				if row.RankStatus == "ranked" {
					ranked++
					if !row.LastMatchAtKnown {
						t.Logf("ranked activity unavailable: %s#%s", row.GameName, row.TagLine)
					}
				}
				if row.LastMatchAtKnown {
					known++
				}
			}
		}
	}
	t.Logf("accounts=%d ranked=%d known match starts=%d", total, ranked, known)
	if total != 53 {
		t.Fatal("incomplete roster")
	}
}

func TestR107ImageBudgetReachesUnderlyingTransport(t *testing.T) {
	for _, endpoint := range []string{"client", "public"} {
		t.Run(endpoint, func(t *testing.T) {
			calls := 0
			p := newChampionProvider()
			p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				deadline, ok := r.Context().Deadline()
				remaining := time.Until(deadline)
				// R127 P1-a.3：远程小图标预算从 8 秒收紧到 3 秒（原画仍是 8 秒）。
				// 这条护栏盯的仍然是「外层预算必须真的传到底层 Transport」，只是
				// 基准换成按路径计算的 remoteImageBudget。
				budget := remoteImageBudget(r.URL.Path)
				if !ok || remaining < budget-time.Second || remaining > budget+100*time.Millisecond {
					t.Errorf("inner image timeout truncated budget: %s (want ~%s)", remaining, budget)
				}
				return r99Response("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 60)), nil
			})}
			a := &app{champions: p}
			w := httptest.NewRecorder()
			if endpoint == "client" {
				a.handleImage(w, httptest.NewRequest("GET", "/api/image?path=/lol-game-data/assets/v1/champion-icons/103.png", nil))
			} else {
				a.handleChampionAsset(w, httptest.NewRequest("GET", "/api/champion-asset?source=communitydragon&path=/latest/plugins/rcp-be-lol-game-data/global/default/v1/champion-icons/103.png", nil))
			}
			if w.Code != 200 || calls != 1 {
				t.Fatalf("status=%d calls=%d", w.Code, calls)
			}
		})
	}
}

func TestR107LiveImagesWithoutClient(t *testing.T) {
	if os.Getenv("R107_LIVE_IMAGES") != "1" {
		t.Skip("opt-in public image read")
	}
	t.Setenv("DEEP_LEGENDS_ITEM_PREWARM", "0")
	store := &localStore{root: t.TempDir()}
	paths := []string{}
	for _, id := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15} {
		paths = append(paths, fmt.Sprintf("/lol-game-data/assets/v1/champion-icons/%d.png", id), fmt.Sprintf("/lol-game-data/assets/v1/profile-icons/%d.jpg", id))
	}
	for pass := 0; pass < 2; pass++ {
		p := newChampionProvider()
		p.communityImageCache = newCommunityImageCache(store)
		p.imageCache = newPublicBinaryCache(store, "champion-images", 2048, 64<<20)
		transport := p.client.Transport
		var requests atomic.Int32
		p.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { requests.Add(1); return transport.RoundTrip(r) })
		a := &app{storage: store, champions: p}
		started := time.Now()
		jobs := make(chan int)
		var workers sync.WaitGroup
		var success atomic.Int32
		for worker := 0; worker < 6; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for i := range jobs {
					w := httptest.NewRecorder()
					if i%2 == 0 {
						a.handleImage(w, httptest.NewRequest("GET", "/api/image?path="+url.QueryEscape(paths[i]), nil))
					} else {
						remote := "/latest/plugins/rcp-be-lol-game-data/global/default/" + strings.TrimPrefix(paths[i], "/lol-game-data/assets/")
						a.handleChampionAsset(w, httptest.NewRequest("GET", "/api/champion-asset?source=communitydragon&path="+url.QueryEscape(remote), nil))
					}
					if w.Code == 200 && strings.HasPrefix(w.Header().Get("Content-Type"), "image/") {
						success.Add(1)
					} else {
						t.Errorf("pass=%d resource=%s status=%d", pass, paths[i], w.Code)
					}
				}
			}()
		}
		for i := range paths {
			jobs <- i
		}
		close(jobs)
		workers.Wait()
		t.Logf("pass=%d successful=%d/%d remote_requests=%d elapsed=%s", pass, success.Load(), len(paths), requests.Load(), time.Since(started))
		if pass == 1 && requests.Load() != 0 {
			t.Error("restart did not reuse image disk cache")
		}
		p.client.CloseIdleConnections()
	}
}

func TestR117ImageETagRevalidation(t *testing.T) {
	p := newChampionProvider()
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return r99Response("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 60)), nil
	})}
	a := &app{champions: p}
	first := httptest.NewRecorder()
	a.handleImage(first, httptest.NewRequest(http.MethodGet, "/api/image?path=/lol-game-data/assets/v1/champion-icons/103.png", nil))
	if first.Code != http.StatusOK || first.Header().Get("ETag") == "" {
		t.Fatalf("initial image response: status=%d headers=%v", first.Code, first.Header())
	}
	request := httptest.NewRequest(http.MethodGet, "/api/image?path=/lol-game-data/assets/v1/champion-icons/103.png", nil)
	request.Header.Set("If-None-Match", first.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	a.handleImage(cached, request)
	if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
		t.Fatalf("image revalidation: status=%d body=%d", cached.Code, cached.Body.Len())
	}
}

// P3-5：一个 assetPath 最多扇出 6 个 CommunityDragon 候选（_large.png 缺图时逐个 404）。
// 负缓存必须按分钟级、并且「全部候选均失败」要按 assetPath 单独记一条，否则同一个
// 资源每 5 秒就把整组扇出重放一遍（657 个海克斯里有 60 个只有 _small）。
func TestR117CommunityImageFanoutNegativeCacheIsPerAssetPathAndMinuteLevel(t *testing.T) {
	if communityImageCandidateNegativeTTL < time.Minute {
		t.Fatalf("candidate negative TTL = %v, want >= 1m", communityImageCandidateNegativeTTL)
	}
	if communityImageResolveNegativeTTL < time.Minute {
		t.Fatalf("resolve negative TTL = %v, want >= 1m", communityImageResolveNegativeTTL)
	}
	const assetPath = "/lol-game-data/assets/v1/champion-tiles/103/103000_large.png"
	if got := len(communityDragonImagePaths(assetPath)); got != 6 {
		t.Fatalf("candidate fan-out = %d, want 6", got)
	}
	missing := func() *http.Response {
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("missing"))}
	}
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 60))
	request := func(a *app) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		a.serveCommunityDragonImage(w, httptest.NewRequest(http.MethodGet, "/api/image?path="+url.QueryEscape(assetPath), nil), assetPath)
		return w
	}
	rewindCandidateFailures := func(a *app) {
		a.assetCacheMu.Lock()
		for key, until := range a.assetFailureUntil {
			if strings.HasPrefix(key, "cdragon:") && !strings.HasPrefix(key, "cdragon-resolved:") {
				a.assetFailureUntil[key] = until.Add(-2 * communityImageCandidateNegativeTTL)
			}
		}
		a.assetCacheMu.Unlock()
	}

	t.Run("every candidate failing is remembered per assetPath", func(t *testing.T) {
		var calls atomic.Int32
		p := newChampionProvider()
		p.client = &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return missing(), nil
		})}
		a := &app{champions: p}
		if first := request(a); first.Code != http.StatusNotFound {
			t.Fatalf("initial status = %d", first.Code)
		}
		if got := calls.Load(); got != 6 {
			t.Fatalf("initial upstream calls = %d, want the full 6-candidate fan-out", got)
		}
		a.assetCacheMu.Lock()
		_, resolved := a.assetFailureUntil["cdragon-resolved:"+assetPath]
		a.assetCacheMu.Unlock()
		if !resolved {
			t.Fatal("all-candidate failure was not negative-cached under the assetPath key")
		}
		// 把候选级负缓存全部作废，只留 assetPath 级那一条：第二次请求仍不得打上游。
		// 摘掉 serveCommunityDragonImage 外层的 assetPath 负缓存，这里就会重新扇出 6 次。
		rewindCandidateFailures(a)
		if second := request(a); second.Code != http.StatusNotFound {
			t.Fatalf("repeat status = %d", second.Code)
		}
		if got := calls.Load(); got != 6 {
			t.Fatalf("repeat upstream calls = %d, want 6 (assetPath negative cache must short-circuit the fan-out)", got)
		}
	})

	t.Run("partial failure does not replay the missing candidates", func(t *testing.T) {
		var largeMisses, smallHits atomic.Int32
		p := newChampionProvider()
		p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "_small.png") {
				smallHits.Add(1)
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader(string(png)))}, nil
			}
			largeMisses.Add(1)
			return missing(), nil
		})}
		a := &app{champions: p}
		first := request(a)
		if first.Code != http.StatusOK || first.Header().Get("ETag") == "" {
			t.Fatalf("initial status = %d etag = %q", first.Code, first.Header().Get("ETag"))
		}
		if got := largeMisses.Load(); got != 4 {
			t.Fatalf("initial 404 candidates = %d, want 4", got)
		}
		if second := request(a); second.Code != http.StatusOK {
			t.Fatalf("repeat status = %d", second.Code)
		}
		if got := largeMisses.Load(); got != 4 {
			t.Fatalf("repeat 404 candidates = %d, want 4 (minute-level candidate negative cache must hold)", got)
		}
		if got := smallHits.Load(); got > 2 {
			t.Fatalf("small candidate upstream hits = %d, want at most one per request", got)
		}
	})
}

func TestR117ProfileUsesJSONLDStartOverDirectoryRevision(t *testing.T) {
	data, err := os.ReadFile("testdata/r107/opgg-profile.html")
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return proHTTPBody(data), nil
	})}
	// old.LastMatchAt 必须同时满足两个条件，否则这条测试测不到合并逻辑：
	//  1. 不等于 directory revision —— readProProfile 第一步就调 proRealLastMatchAt，
	//     两者是同一时刻时 LastMatchAtKnown 会被直接重置为 false，「取大值」旧 bug
	//     和「按来源优先」新逻辑就都通过了（这是独立验收揪出来的假护栏）。
	//  2. 晚于 JSON-LD startTime —— 这样旧 bug 的 max() 会保留 old 的值，
	//     新逻辑必须无条件用 JSON-LD 覆盖，两者结果不同才测得出来。
	old := opggProAccount{
		GameName: "Kimman", TagLine: "zxfkk", Region: "kr",
		RevisionAt: "2026-09-17T03:56:23+09:00", DirectoryRevisionAt: "2026-09-17T03:56:23+09:00",
		LastMatchAt: "2026-09-16T23:30:00Z", LastMatchAtKnown: true,
	}
	got := (&app{champions: &championProvider{client: client}}).readProProfile(context.Background(), old)
	if !got.LastMatchAtKnown || got.LastMatchAt != "2026-09-16T18:19:58Z" {
		t.Fatalf("JSON-LD startTime must win over a later directory/old timestamp: %+v", got)
	}
}

func TestR117ProfileThrottleIsPendingWithoutShortTTL(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-fixture")
	data, err := os.ReadFile("testdata/r107/opgg-profile.html")
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), "PlayGameAction", "NoHistory"))
	cp := newChampionProvider()
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return proHTTPBody(data), nil
	})}
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		t.Fatalf("throttled activity must not reach Riot transport: %s", r.URL)
		return nil, nil
	})
	p.limitMu.Lock()
	p.limitQueue = []*riotLimitWaiter{{turn: make(chan struct{})}}
	p.limitMu.Unlock()
	row := (&app{champions: cp, riot: p}).readProProfile(context.Background(), opggProAccount{
		GameName: "Kimman", TagLine: "zxfkk", Region: "kr",
	})
	if !row.ActivityPending || row.ActivityFailed || proProfileTTL(row) != 15*time.Minute {
		t.Fatalf("throttled activity collapsed profile TTL: %+v ttl=%s", row, proProfileTTL(row))
	}
}
