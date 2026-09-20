package main

import (
	"context"
	"encoding/json"
	"fmt"
	xhtml "golang.org/x/net/html"
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

func TestR107RejectedOfficialIconUsesVerifiedChatScope(t *testing.T) {
	for _, retain := range []bool{true, false} {
		t.Run(fmt.Sprint(retain), func(t *testing.T) {
			chatIcon := int64(1)
			var writes []string
			client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/lol-game-data/assets/v1/summoner-icons.json":
					fmt.Fprint(w, `[{"id":99,"title":"Fixture","imagePath":"/lol-game-data/assets/v1/profile-icons/99.jpg"}]`)
				case "/lol-game-data/assets/v1/summoner-icon-sets.json":
					fmt.Fprint(w, `[]`)
				case "/lol-inventory/v2/inventory/SUMMONER_ICON":
					fmt.Fprint(w, `[{"itemId":1,"owned":true}]`)
				case "/lol-summoner/v1/current-summoner/icon":
					writes = append(writes, "profile")
					w.WriteHeader(401)
				case "/lol-chat/v1/me":
					if r.Method == http.MethodPut {
						writes = append(writes, "chat")
						var b map[string]int64
						json.NewDecoder(r.Body).Decode(&b)
						if len(b) != 1 || b["icon"] != 99 {
							t.Error("unexpected chat mutation")
						}
						if retain {
							chatIcon = b["icon"]
						}
					} else {
						fmt.Fprintf(w, `{"icon":%d}`, chatIcon)
					}
				default:
					fmt.Fprint(w, `{}`)
				}
			})
			a := &app{lcu: client, summoner: Summoner{SummonerID: 1, ProfileIconID: 1}}
			scope, err := a.writeFacadeIconResult(context.Background(), client, 99)
			if retain && (err != nil || scope != "chat") {
				t.Fatalf("scope=%s err=%v", scope, err)
			}
			if !retain && err == nil {
				t.Fatal("unconfirmed chat write reported success")
			}
			if a.summoner.ProfileIconID != 1 {
				t.Fatal("chat-only icon overwritten official identity")
			}
			if strings.Join(writes, ",") != "profile,chat" {
				t.Fatalf("writes=%v", writes)
			}
		})
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

func TestR108BannerWaitsForCareerSummaryReadback(t *testing.T) {
	reads, writes := 0, 0
	equipped := "old-accent"
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-game-data/assets/v1/regalia.json":
			fmt.Fprint(w, `[{"id":"4","idSecondary":"","regaliaType":"kBanner","isSelectable":true}]`)
		case facadeChallengeSummaryPath:
			reads++
			if writes > 0 && reads >= 6 {
				equipped = "4"
			}
			fmt.Fprintf(w, `{"bannerId":%q,"selectedChallengesString":"","title":null}`, equipped)
		case facadeChallengePreferencesPath:
			writes++
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["bannerAccent"] != "4" {
				t.Error("wrong banner identity", body)
			}
		default:
			t.Fatalf("unexpected endpoint %s", r.URL.Path)
		}
	})
	if err := writeFacadeBanner(context.Background(), client, "4"); err != nil || writes != 1 || reads != 6 {
		t.Fatalf("reads=%d writes=%d err=%v", reads, writes, err)
	}
}

// Opt-in read-only verification against the configured public sources.
func TestR107LiveProfileActivityBatch(t *testing.T) {
	output := os.Getenv("R107_LIVE_PROFILE_OUTPUT")
	if output == "" {
		t.Skip("opt-in network verification")
	}
	key, err := os.ReadFile("riot_key.local.txt")
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
				if !ok || remaining < 7*time.Second || remaining > 8100*time.Millisecond {
					t.Errorf("inner image timeout truncated budget: %s", remaining)
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
