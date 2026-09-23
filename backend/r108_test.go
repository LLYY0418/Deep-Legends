package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type r108WaitingContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *r108WaitingContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestR108RefreshDoesNotInheritCanceledFlight(t *testing.T) {
	cache := newOverviewQueryCache()
	ref := gameplayReference{PlayerRef: "fixture", Region: "kr"}
	key := riotOverviewQuerySnapshotKey(ref, 0, 20, "solo")
	flight := &overviewQueryFlight{done: make(chan struct{})}
	cache.flights[key] = flight
	ctx := &r108WaitingContext{Context: context.Background(), waiting: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := (&app{overviewQueries: cache}).loadRiotOverviewDeduplicated(ctx, ref, 0, 20, true, "solo")
		result <- err
	}()
	<-ctx.waiting
	cache.complete(key, flight, gameplayOverview{}, context.Canceled)
	// No provider is installed: reaching its initialization check proves the
	// still-active replacement performed a fresh load instead of dying silently.
	err := <-result
	if errors.Is(err, context.Canceled) || err == nil || !strings.Contains(err.Error(), "未初始化") {
		t.Fatal(err)
	}
}

func TestR108ModeHistoryQueriesOnlyMatchingQueuesAndCachesEmpty(t *testing.T) {
	for _, filter := range []string{"solo", "flex", "hextech-aram", "arena", "ranked"} {
		t.Run(filter, func(t *testing.T) {
			// P1-1 之后同一次查询里的多队列 ID 请求是并发的，transport 回调跑在
			// 非测试 goroutine 上：计数必须加锁，失败只能用 Errorf 并返回空结果
			// （t.Fatal 会在错误的 goroutine 上 Goexit，既停不住测试也可能挂住）。
			var mu sync.Mutex
			calls := 0
			p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
				mu.Lock()
				calls++
				mu.Unlock()
				q := r.URL.Query()
				if !strings.HasSuffix(r.URL.Path, "/ids") || (q.Get("queue") == "" && q.Get("type") != "ranked") {
					t.Errorf("unfiltered or detail request: %s", r.URL)
					return r99Response(`[]`), nil
				}
				if filter == "solo" && q.Get("queue") != "420" {
					t.Errorf("solo filter queried %s", q.Get("queue"))
					return r99Response(`[]`), nil
				}
				if filter == "flex" && q.Get("queue") != "440" {
					t.Errorf("flex filter queried %s", q.Get("queue"))
					return r99Response(`[]`), nil
				}
				return r99Response(`[]`), nil
			})
			for i := 0; i < 2; i++ {
				ids, err := p.matchIDsForOverview(context.Background(), "fixture", 0, 20, filter)
				if err != nil || len(ids) != 0 {
					t.Fatal(ids, err)
				}
				pagination := riotOverviewPagination(0, len(ids), 20, filter)
				if pagination.HasMore || !pagination.ServerFiltered || pagination.FilterFallback {
					t.Fatal(pagination)
				}
			}
			want := 1
			if filter == "ranked" {
				want = 2
			} else if filter == "arena" || filter == "hextech-aram" {
				want = 0
				for _, definition := range supportedQueueDefinitions {
					if definition.Filter == filter {
						want++
					}
				}
			}
			mu.Lock()
			gotCalls := calls
			mu.Unlock()
			if gotCalls != want {
				t.Fatalf("calls=%d want=%d; empty mode must not crawl all history", gotCalls, want)
			}
		})
	}
}

func TestR108MultiQueuePagesMergeAndReusePrefixes(t *testing.T) {
	// 同上：多队列 ID 请求并发，计数器必须加锁，否则丢自增会让 calls 变成 2。
	var mu sync.Mutex
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if r.URL.Query().Get("start") != "0" || r.URL.Query().Get("count") != "100" {
			t.Errorf("unexpected pagination: %s", r.URL)
			return r99Response(`[]`), nil
		}
		switch r.URL.Query().Get("queue") {
		case "2300":
			return r99Response(`["KR_104","KR_101"]`), nil
		case "2400":
			return r99Response(`["KR_103","KR_100"]`), nil
		case "3270":
			return r99Response(`["KR_104","KR_102"]`), nil
		default:
			t.Errorf("unexpected queue %s", r.URL)
			return r99Response(`[]`), nil
		}
	})
	for i, want := range [][]string{{"KR_104", "KR_103"}, {"KR_102", "KR_101"}, {"KR_100"}} {
		ids, err := p.matchIDsForOverview(context.Background(), "fixture", i*2, 2, "hextech-aram")
		if err != nil || !reflect.DeepEqual(ids, want) {
			t.Fatal(ids, want, err)
		}
	}
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 3 {
		t.Fatalf("repeated queue-prefix queries: %d", gotCalls)
	}
}

func TestR108OverviewCacheKeepsModesSeparateAndReusesRecentKR(t *testing.T) {
	ref := gameplayReference{PlayerRef: "fixture", Region: "kr"}
	all := riotOverviewQuerySnapshotKey(ref, 0, 20, "all")
	solo := riotOverviewQuerySnapshotKey(ref, 0, 20, "solo")
	if all == solo {
		t.Fatal("mode collision")
	}
	cache := newOverviewQueryCache()
	now := time.Now()
	cache.putLocked(all, overviewQueryCacheEntry{at: now.Add(-45 * time.Second), response: gameplayOverview{Player: gameplayPlayer{Region: "kr"}}})
	if _, ok := cache.getLocked(all, now); !ok {
		t.Fatal("recent KR snapshot expired")
	}
	if _, ok := cache.getLocked(solo, now); ok {
		t.Fatal("another mode's snapshot reused")
	}
	if _, ok := cache.getLocked(all, now.Add(time.Minute)); ok {
		t.Fatal("stale snapshot retained")
	}
}

func TestR108CareerUsesActualClientBackdrop(t *testing.T) {
	for _, kind := range []string{"recently-played", "highest-mastery", "specified-skin", "summoner-icon", "foreign"} {
		t.Run(kind, func(t *testing.T) {
			image := "/lol-game-data/assets/ASSETS/Characters/LeeSin/Skins/Base/Images/leesin_splash_centered_0.jpg"
			if kind == "summoner-icon" {
				image = "/lol-game-data/assets/v1/profile-icons/99.jpg"
			}
			client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/lol-collections/v1/inventories/7/backdrop" {
					t.Fatal(r.URL.Path)
				}
				id := 7
				if kind == "foreign" {
					id = 8
				}
				json.NewEncoder(w).Encode(map[string]any{"summonerId": id, "championId": 64, "backdropImage": image, "backdropType": kind})
			})
			state := facadeState{Profile: facadeProfile{BackgroundSkinID: 22015}, Skins: []facadeSkin{{ID: 64000, ChampionID: 64, Name: "盲僧", SplashPath: image}}}
			if kind == "summoner-icon" {
				state.Skins = nil
			}
			new(app).applyFacadeBackdrop(context.Background(), client, Summoner{SummonerID: 7}, &state)
			if kind == "foreign" {
				if state.Profile.BackgroundPath != "" || state.Profile.BackgroundSkinID != 22015 {
					t.Fatal("foreign backdrop accepted")
				}
				return
			}
			if state.Profile.BackgroundPath != image {
				t.Fatal(state.Profile)
			}
			want := int64(64000)
			if kind == "summoner-icon" {
				want = 0
			}
			if state.Profile.BackgroundSkinID != want {
				t.Fatal(state.Profile)
			}
		})
	}
	if !isFacadeLCUEvent(LCUEvent{URI: "/lol-collections/v1/inventories/7/backdrop"}) {
		t.Fatal("client background changes not observed")
	}
}

func TestR108FacadeStateReturnsResolvedBackdropAndBanner(t *testing.T) {
	const image = "/lol-game-data/assets/ASSETS/Characters/LeeSin/Skins/Base/Images/leesin_splash_centered_0.jpg"
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-collections/v1/inventories/7/backdrop":
			fmt.Fprintf(w, `{"summonerId":7,"championId":64,"backdropImage":%q,"backdropType":"recently-played"}`, image)
		case facadeChallengeSummaryPath:
			fmt.Fprint(w, `{"bannerId":"actual-accent","topChallenges":[],"title":null}`)
		case "/lol-chat/v1/me":
			fmt.Fprint(w, `{"lol":{"bannerIdSelected":"stale-chat-value"}}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	})
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 7}, allSkinsWithBase: []Skin{{ID: 64000, ChampionID: 64, Name: "盲僧", SplashPath: image}}}
	a.facadeIdentityShapeDiagnosticClient = client
	state := a.loadFacadeState(context.Background())
	data, err := json.Marshal(state)
	if err != nil || state.Profile.BackgroundSkinID != 64000 || state.BannerAccent != "actual-accent" || !strings.Contains(string(data), `"backgroundPath":"`+image+`"`) {
		t.Fatal(string(data), err)
	}
}

func TestR117MultiQueueHistoryHasBoundedConcurrentIDRequests(t *testing.T) {
	var mu sync.Mutex
	calls, active, peak := 0, 0, 0
	page := make([]string, 100)
	for index := range page {
		page[index] = fmt.Sprintf("KR_%d", 100000+index)
	}
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.URL.Path, "/ids") {
			t.Fatalf("unexpected request: %s", r.URL)
		}
		mu.Lock()
		calls++
		active++
		if active > peak {
			peak = active
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return r99Response(string(body)), nil
	})
	ids, err := p.matchIDsForOverview(context.Background(), "bounded-fixture", 500, 20, "arena")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotCalls, gotPeak := calls, peak
	mu.Unlock()
	if gotCalls > riotOverviewMaxIDRequests {
		t.Fatalf("ID request budget exceeded: %d > %d", gotCalls, riotOverviewMaxIDRequests)
	}
	if gotPeak < 2 {
		t.Fatalf("multi-queue history did not overlap requests: peak=%d ids=%d", gotPeak, len(ids))
	}
	// 只断言 peak>=2 证明不了「有上界」：把信号量容量改成 10 万（等于去掉限流）
	// 那条断言照样通过。arena 过滤有 9 个队列、预算 12 次请求，没有限流时 peak
	// 会明显超过 4，所以这条才是真正的并发上界护栏。
	// 上界值必须在测试里硬编码，不能引用生产常量：引用同一个常量时，把常量改大
	// 等于同时移动球门，peak 会跟着涨到 9 而断言永远成立（独立验收揪出的假护栏）。
	// 两条断言分别钉住「常量本身没被改」和「真实并发确实被限住」。
	const auditedMaxConcurrent = 4
	if riotOverviewMaxConcurrentIDRequests != auditedMaxConcurrent {
		t.Fatalf("production concurrency bound drifted: riotOverviewMaxConcurrentIDRequests=%d, audited=%d", riotOverviewMaxConcurrentIDRequests, auditedMaxConcurrent)
	}
	if gotPeak > auditedMaxConcurrent {
		t.Fatalf("concurrent ID requests exceeded the audited bound: peak=%d > %d (calls=%d)", gotPeak, auditedMaxConcurrent, gotCalls)
	}
}
