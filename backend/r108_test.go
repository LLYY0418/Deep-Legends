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
			calls := 0
			p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
				calls++
				q := r.URL.Query()
				if !strings.HasSuffix(r.URL.Path, "/ids") || (q.Get("queue") == "" && q.Get("type") != "ranked") {
					t.Fatalf("unfiltered or detail request: %s", r.URL)
				}
				if filter == "solo" && q.Get("queue") != "420" {
					t.Fatal(q)
				}
				if filter == "flex" && q.Get("queue") != "440" {
					t.Fatal(q)
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
			if calls != want {
				t.Fatalf("calls=%d want=%d; empty mode must not crawl all history", calls, want)
			}
		})
	}
}

func TestR108MultiQueuePagesMergeAndReusePrefixes(t *testing.T) {
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("start") != "0" || r.URL.Query().Get("count") != "100" {
			t.Fatal(r.URL)
		}
		switch r.URL.Query().Get("queue") {
		case "2300":
			return r99Response(`["KR_104","KR_101"]`), nil
		case "2400":
			return r99Response(`["KR_103","KR_100"]`), nil
		case "3270":
			return r99Response(`["KR_104","KR_102"]`), nil
		default:
			t.Fatalf("unexpected queue %s", r.URL)
			return nil, nil
		}
	})
	for i, want := range [][]string{{"KR_104", "KR_103"}, {"KR_102", "KR_101"}, {"KR_100"}} {
		ids, err := p.matchIDsForOverview(context.Background(), "fixture", i*2, 2, "hextech-aram")
		if err != nil || !reflect.DeepEqual(ids, want) {
			t.Fatal(ids, want, err)
		}
	}
	if calls != 3 {
		t.Fatalf("repeated queue-prefix queries: %d", calls)
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

func TestR108BannerMissingSnapshotDoesNotWrite(t *testing.T) {
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("write without restorable snapshot")
		}
		if strings.HasSuffix(r.URL.Path, "regalia.json") {
			fmt.Fprint(w, `[{"id":"4","idSecondary":"accent","regaliaType":"kBanner","isSelectable":true}]`)
		} else {
			fmt.Fprint(w, `{}`)
		}
	})
	if err := writeFacadeBanner(context.Background(), client, "4"); err == nil {
		t.Fatal("missing state accepted")
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
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 7}, allSkins: []Skin{{ID: 64000, ChampionID: 64, Name: "盲僧", SplashPath: image}}}
	a.facadeIdentityShapeDiagnosticClient = client
	state := a.loadFacadeState(context.Background())
	data, err := json.Marshal(state)
	if err != nil || state.Profile.BackgroundSkinID != 64000 || state.BannerAccent != "actual-accent" || !strings.Contains(string(data), `"backgroundPath":"`+image+`"`) {
		t.Fatal(string(data), err)
	}
}
