package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR106ProProfileCapturedCurrentRanks(t *testing.T) {
	for _, tc := range []struct {
		file, name, tag, tier string
		lp                    int
	}{{"profile", "Kimman", "zxfkk", "CHALLENGER", 2394}, {"diamond", "dyjkbysb", "KR1", "DIAMOND", 75}} {
		data, err := os.ReadFile("testdata/r106/opgg-" + tc.file + ".html")
		if err != nil {
			t.Fatal(err)
		}
		row, err := parseProProfile(data, gameplayReference{GameName: tc.name, TagLine: tc.tag})
		if err != nil {
			t.Fatal(err)
		}
		a, _ := normalizeProAccount(row)
		if a.Tier != tc.tier || a.LP != tc.lp || a.UpdatedAt == "" {
			t.Fatalf("wrong current rank: %+v", a)
		}
		if _, err := parseProProfile(data, gameplayReference{GameName: "someone else", TagLine: tc.tag}); err == nil {
			t.Fatal("accepted wrong identity")
		}
		if _, err := parseProProfile([]byte(strings.ReplaceAll(string(data), `\"region\":\"kr\"`, `\"region\":\"na\"`)), gameplayReference{GameName: tc.name, TagLine: tc.tag}); err == nil {
			t.Fatal("accepted wrong region")
		}
	}
}

func r106ProfilePage(name, tag, rankHTML string) []byte {
	identity, _ := json.Marshal(map[string]any{"region": "kr", "data": map[string]any{"gameName": name, "tagline": tag, "puuid": "fixture-public-player-reference"}})
	stamp := `{"puuid":"fixture-public-player-reference","initUpdatedAt":"2026-09-17T10:00:00Z"}`
	flight, _ := json.Marshal("1:" + string(identity) + "\n2:" + stamp + "\n")
	return []byte(`<section><header><span>单排/双排</span></header>` + rankHTML + `<table><tr><td><strong class="text-xl">王者</strong><span>2,000 LP</span></td></tr></table></section><script>self.__next_f.push([1,` + string(flight) + `])</script>`)
}

func TestR106ProfileUnrankedIsExplicitAndHistoryIsExcluded(t *testing.T) {
	captured, err := os.ReadFile("testdata/r106/opgg-unranked.html")
	if err != nil {
		t.Fatal(err)
	}
	liveRow, err := parseProProfile(captured, gameplayReference{GameName: "스몰더 아빠", TagLine: "tsts"})
	if err != nil {
		t.Fatal(err)
	}
	liveAccount, _ := normalizeProAccount(liveRow)
	if liveAccount.RankStatus != "unranked" || liveAccount.UpdatedAt == "" {
		t.Fatalf("captured unranked: %+v", liveAccount)
	}
	ref := gameplayReference{GameName: "Fixture", TagLine: "KR1"}
	row, err := parseProProfile(r106ProfilePage(ref.GameName, ref.TagLine, `<span>未定级</span>`), ref)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := normalizeProAccount(row)
	if a.RankStatus != "unranked" || proProfileTTL(row) != 72*time.Hour {
		t.Fatalf("%+v", a)
	}
	if _, err := parseProProfile(r106ProfilePage(ref.GameName, ref.TagLine, ``), ref); err == nil {
		t.Fatal("historical rank became current rank")
	}
}

func TestR106RanksStartBeforeMatchIDsReturn(t *testing.T) {
	f := r98OverviewFixture(t, false)
	p := f.a.riot.champions
	base := p.client.Transport
	idsEntered, release, rankStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	p.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/ids") {
			close(idsEntered)
			<-release
		}
		if strings.Contains(r.URL.Path, "/league/") {
			close(rankStarted)
		}
		return base.RoundTrip(r)
	})
	done := make(chan error, 1)
	go func() {
		_, err := f.a.loadRiotOverview(context.Background(), gameplayReference{PlayerRef: "r98-subject", Region: "kr", Privacy: "PRIVATE"}, 0, 5)
		done <- err
	}()
	<-idsEntered
	select {
	case <-rankStarted:
		close(release)
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("rank read waited behind match IDs")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestR106AllReviewedAccountsBatchAndPersistentCache(t *testing.T) {
	var requests, active, maximum atomic.Int64
	client := &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		if r.URL.Host != "op.gg" || r.Header.Get("Authorization") != "" {
			t.Error("unsafe profile request")
		}
		slug := strings.TrimPrefix(r.URL.Path, "/zh-cn/lol/summoners/kr/")
		cut := strings.LastIndex(slug, "-")
		if cut < 0 {
			t.Fatal(slug)
		}
		body := r106ProfilePage(slug[:cut], slug[cut+1:], `<div><strong class="text-xl">钻石 2</strong><span>75 LP</span></div>`)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	store := &localStore{root: t.TempDir()}
	app1 := &app{storage: store, champions: &championProvider{client: client}}
	seeds := new(app).loadProSeeds(context.Background(), nil)
	result := app1.enrichProProfiles(context.Background(), seeds)
	page := app1.buildReviewedProPlayers(result)
	if requests.Load() != 53 || maximum.Load() > 6 || maximum.Load() < 2 {
		t.Fatalf("requests=%d parallel=%d", requests.Load(), maximum.Load())
	}
	for _, team := range page.Teams {
		for _, player := range team.Players {
			for _, account := range player.Accounts {
				if account.RankStatus != "ranked" || account.CheckedAt == "" || account.CheckFailed {
					t.Fatalf("missing: %+v", account)
				}
			}
		}
	}
	app2 := &app{storage: store, champions: &championProvider{client: client}}
	app2.enrichProProfiles(context.Background(), seeds)
	if requests.Load() != 53 {
		t.Fatalf("restart ignored public cache: %d", requests.Load())
	}
}

func TestR106CancelledFacadeReaderDoesNotCancelSharedRead(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lol-chat/v1/me" {
			if calls.Add(1) == 1 {
				close(entered)
				<-release
			}
		}
		fmt.Fprint(w, `{}`)
	})
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}, allSkins: []Skin{{ID: 103001, ChampionID: 103, Name: "Fixture"}}, facadeIdentityShapeDiagnosticClient: client}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := a.cachedFacadeState(ctx, false, "poll"); done <- err }()
	<-entered
	cancel()
	if <-done == nil {
		t.Fatal("reader cancellation ignored")
	}
	close(release)
	value, err := a.cachedFacadeState(context.Background(), false, "poll")
	if err != nil || len(value.Skins) != 1 || calls.Load() != 1 {
		t.Fatalf("state=%+v err=%v calls=%d", value, err, calls.Load())
	}
	a.mu.Lock()
	a.summoner.SummonerID = 2
	a.mu.Unlock()
	if _, err := a.cachedFacadeState(context.Background(), false, "poll"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("new account reused previous view")
	}
}

func TestR106ProfileIconPersistsOnlySuccessfulImage(t *testing.T) {
	var calls atomic.Int64
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 60)))
	})
	store := &localStore{root: t.TempDir()}
	for i := 0; i < 2; i++ {
		a := &app{storage: store}
		if _, err := a.loadClientIcon(context.Background(), client, "/lol-game-data/assets/v1/profile-icons/123.jpg"); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("restart fetched public image again")
	}
}

func TestR106MutationDetachesAnOlderFacadeRead(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	client := r105LCU(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lol-chat/v1/me" {
			if calls.Add(1) == 1 {
				close(entered)
				<-release
				fmt.Fprint(w, `{"statusMessage":"old"}`)
				return
			}
			fmt.Fprint(w, `{"statusMessage":"new"}`)
			return
		}
		fmt.Fprint(w, `{}`)
	})
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}, allSkins: []Skin{{ID: 103001, ChampionID: 103}}, facadeIdentityShapeDiagnosticClient: client}
	done := make(chan facadeState, 1)
	go func() { value, _ := a.cachedFacadeState(context.Background(), false, "poll"); done <- value }()
	<-entered
	a.invalidateFacadeView()
	next, err := a.cachedFacadeState(context.Background(), false, "poll")
	close(release)
	old := <-done
	if err != nil || next.Chat.StatusMessage != "new" || old.Chat.StatusMessage != "new" {
		t.Fatalf("late old view: next=%+v old=%+v error=%v", next.Chat, old.Chat, err)
	}
	if calls.Load() != 2 {
		t.Fatal("stale reader replaced the shared flight")
	}
}

func TestR106LiveProfileBatch(t *testing.T) {
	output := os.Getenv("R106_LIVE_PROFILE_OUTPUT")
	if output == "" {
		t.Skip("opt-in public profile read")
	}
	a := &app{champions: newChampionProvider()}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	started := time.Now()
	seeds := new(app).loadProSeeds(ctx, nil)
	rows := a.enrichProProfiles(ctx, seeds)
	page := a.buildReviewedProPlayers(rows)
	data, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	ranked, unranked, failed, stamps := 0, 0, 0, 0
	for _, team := range page.Teams {
		for _, player := range team.Players {
			for _, row := range player.Accounts {
				if row.CheckFailed {
					failed++
					t.Logf("failed public page: %s#%s", row.GameName, row.TagLine)
				}
				if row.RankStatus == "ranked" {
					ranked++
				}
				if row.RankStatus == "unranked" {
					unranked++
				}
				if row.UpdatedAt != "" {
					stamps++
				}
			}
		}
	}
	t.Logf("accounts=%d ranked=%d unranked=%d failed=%d source timestamps=%d duration=%s", page.AccountCount, ranked, unranked, failed, stamps, time.Since(started))
	if page.AccountCount != 53 {
		t.Fatal("incomplete reviewed roster")
	}
}
