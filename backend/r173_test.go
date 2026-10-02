package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR173StarterNetOpeningInventory(t *testing.T) {
	frames := []timelineFrame{{Events: []timelineEvent{
		{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 5000, ItemID: 1055},
		{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 5000, ItemID: 2003},
		{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 5000, ItemID: 2003},
		{Type: "ITEM_UNDO", ParticipantID: 1, Timestamp: 20000, BeforeID: 1055},
		{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 20000, ItemID: 1055},
		{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 40000, ItemID: 3340},
		{Type: "ITEM_SOLD", ParticipantID: 1, Timestamp: 70000, ItemID: 2003},
		{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 90000, ItemID: 1086},
		{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 90001, ItemID: 1001},
		{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 180000, ItemID: 3006},
		{Type: "ITEM_UNDO", ParticipantID: 1, Timestamp: 180000, BeforeID: 1086},
		{Type: "ITEM_PURCHASED", ParticipantID: 2, Timestamp: 5000, ItemID: 1056},
	}}}
	if got := participantStarterItems(frames, 1); !reflect.DeepEqual(got, []int64{2003, 1055, 1086}) {
		t.Fatalf("opening net purchases: %v", got)
	}
	duplicates := []timelineFrame{{Events: []timelineEvent{{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 5000, ItemID: 2003}, {Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 5000, ItemID: 2003}}}}
	if got := participantStarterItems(duplicates, 1); !reflect.DeepEqual(got, []int64{2003, 2003}) {
		t.Fatal("duplicates removed", got)
	}
	if got := participantStarterItems(nil, 1); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestR173SpecialistListNeverWaitsForStarterTimeline(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "fixture-key")
	started, release := make(chan struct{}), make(chan struct{})
	var startOnce, releaseOnce sync.Once
	releaseTimeline := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseTimeline()
	var timelineCalls atomic.Int32
	player := championTopPlayer{Name: "Expert", Tagline: "KR1", Tier: "MASTER", Games: "100"}
	p := specialistTestProvider(gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/timeline"):
			timelineCalls.Add(1)
			startOnce.Do(func() { close(started) })
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return specialistTestResponse(r, 200, `{"info":{"frames":[{"events":[{"type":"ITEM_PURCHASED","participantId":1,"timestamp":5000,"itemId":1055}]}]}}`)
		case strings.Contains(r.URL.Host, "op.gg"):
			return specialistTestResponse(r, 200, specialistLeaderboardBody(player))
		case strings.Contains(r.URL.Path, "/accounts/"):
			return specialistTestResponse(r, 200, `{"puuid":"expert-puuid"}`)
		case strings.HasSuffix(r.URL.Path, "/ids"):
			return specialistTestResponse(r, 200, `["KR_1"]`)
		default:
			body := strings.ReplaceAll(specialistMatchBody("KR_1", "expert-puuid", 64, "complete", true), `"puuid":"expert-puuid"`, `"participantId":1,"puuid":"expert-puuid"`)
			return specialistTestResponse(r, 200, body)
		}
	}))
	p.champions.championMeta = map[int]championMetadata{64: {ID: 64, Key: "LeeSin", Slug: "lee-sin", NameZH: "盲僧"}}
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	p.champions.cache = newChampionDataCache(store)
	var budget atomic.Int64
	p.champions.diag = func(event map[string]any) {
		if event["event"] == "specialist_runes_done" {
			if used, ok := event["budget_used"].(int); ok {
				budget.Store(int64(used))
			}
		}
	}
	a := &app{riot: p, champions: p.champions}
	list := func() []gameplayRecommendationRune {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rec := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			a.handleGameplaySpecialistRunes(rec, httptest.NewRequest("GET", "/api/gameplay/specialist-runes?championId=64&position=mid", nil).WithContext(ctx))
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("rune list waited for opening timeline")
		}
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		var rows []gameplayRecommendationRune
		if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatal("missing rune fixture", rec.Body.String())
		}
		return rows
	}
	rows := list()
	// R179 may start one detached prefetch while this response completes.
	if timelineCalls.Load() > 1 {
		t.Fatal("duplicate opening timeline prefetch")
	}
	originalBudget := budget.Load()
	if originalBudget != 3 || specialistRuneRequestBudget != 36 {
		t.Fatal("unexpected first-stage budget", originalBudget)
	}
	body, _ := json.Marshal(runeStarterRequest{Source: "specialist", ChampionID: 64, Position: "mid", Rows: []runeStarterRowRequest{{rows[0].Key, rows[0].PlayedAt}}})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		a.handleGameplayRuneStarters(rec, httptest.NewRequest("POST", "/api/gameplay/rune-starters", strings.NewReader(string(body))))
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("second-stage timeline not requested")
	}
	list()
	if budget.Load() != originalBudget {
		t.Fatal("opening consumed specialist list budget", budget.Load())
	}
	releaseTimeline()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("starter response did not finish")
	}
	var result struct{ Starters []runeStarterRow }
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || len(result.Starters) != 1 || !reflect.DeepEqual(result.Starters[0].StarterItemIDs, []int64{1055}) {
		t.Fatal("starter response", rec.Body.String(), err)
	}
	// A joined cache flight can return before its prefetch owner writes disk.
	// Worker completion confirms persistence before testing a fresh provider.
	deadline := time.Now().Add(2 * time.Second)
	for {
		p.specialistMu.Lock()
		prefetchDone := p.starterPrefetches["KR_1"].done
		p.specialistMu.Unlock()
		if prefetchDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("opening prefetch did not finish persisting cache")
		}
		time.Sleep(time.Millisecond)
	}
	reloaded := newRiotProvider(p.champions)
	if items, err := reloaded.specialistMatchStarters(context.Background(), "KR_1", 1); err != nil || !reflect.DeepEqual(items, []int64{1055}) {
		t.Fatal("immutable disk cache lost", items, err)
	}
	if timelineCalls.Load() != 1 {
		t.Fatal("cached timeline fetched again")
	}
}

func r173ProOpeningBody(g proRuneIndexGame) []byte {
	return []byte(fmt.Sprintf(`{"frames":[{"rfc460Timestamp":%q,"participants":[{"participantId":3,"items":[]}]},{"rfc460Timestamp":%q,"participants":[{"participantId":3,"items":[]}]},{"rfc460Timestamp":%q,"participants":[{"participantId":3,"items":[1055,2003,3340],"summonerSpells":[999987,999988],"SECRET_PLAYER_FIELD":"private-value"}]}]}`, g.Start.Add(60*time.Second).Format(time.RFC3339), g.Start.Add(61*time.Second).Format(time.RFC3339), g.Start.Add(62*time.Second).Format(time.RFC3339)))
}

func TestR173ProOpeningCacheAndRawShapePrivacy(t *testing.T) {
	cp := proTestProvider()
	g := proTestGame()
	g.Start = g.Start.Truncate(10 * time.Second)
	raw := r173ProOpeningBody(g)
	var startCalls, endCalls atomic.Int32
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("startingTime") == proStartingTime(g.Start.Add(proStarterOffset)) {
			startCalls.Add(1)
			return specialistTestResponse(r, 200, string(raw))
		}
		endCalls.Add(1)
		return specialistTestResponse(r, 200, fmt.Sprintf(`{"frames":[{"rfc460Timestamp":%q,"participants":[{"participantId":3,"items":[3006],"perkMetadata":{}}]}]}`, g.Start.Add(30*time.Minute).Format(time.RFC3339)))
	})}
	events := []map[string]any{}
	cp.diag = func(event map[string]any) { events = append(events, event) }
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	p := newProRuneProvider(cp, store, nil)
	p.ends[g.ID] = proTestEnd(g)
	p.writeDisk("end-"+g.ID, p.ends[g.ID])
	end, err := p.supplement(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	opening, err := p.openingDetails(context.Background(), g, []int{3})
	if err != nil {
		t.Fatal(err)
	}
	items, offset := proStarterItems(opening, 3, g.Start)
	if !reflect.DeepEqual(items, []int64{1055, 2003}) || offset != 62 {
		t.Fatal("first nonempty opening frame", items, offset)
	}
	if !reflect.DeepEqual(end.Frames[0].Participants[0].Items, []int64{3006}) {
		t.Fatal("terminal cache polluted")
	}
	_, _ = p.openingDetails(context.Background(), g, []int{3})
	p.recordDetailsShape(raw, proDetailsObservation{g, "start", []int{3}})
	if startCalls.Load() != 1 || endCalls.Load() != 1 || len(events) != 2 {
		t.Fatal("shape not deduplicated or cache not reused", startCalls.Load(), endCalls.Load(), events)
	}
	log, _ := json.Marshal(events)
	for _, secret := range []string{"1055", "2003", "3340", "3006", "999987", "SECRET_PLAYER_FIELD", "private-value", g.ID, g.Metadata.Blue.TeamID, `"participantId":3`} {
		if strings.Contains(string(log), secret) {
			t.Fatalf("raw value leaked: %s in %s", secret, log)
		}
	}
	if !strings.Contains(string(log), "unknown-key") || !strings.Contains(string(log), "summonerSpells") {
		t.Fatal("raw participant shape lost", string(log))
	}
	if events[1]["first_offset_seconds"] != float64(60) || events[1]["last_offset_seconds"] != float64(62) {
		t.Fatal("window origin offsets", events[1])
	}
	selected := events[1]["selected_frames"].([]map[string]any)
	if selected[0]["offset_seconds"] != float64(62) || selected[0]["item_count"] != 2 {
		t.Fatal(selected)
	}
	q := newProRuneProvider(cp, store, nil)
	if _, err = q.openingDetails(context.Background(), g, []int{3}); err != nil {
		t.Fatal(err)
	}
	if _, err = q.supplement(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if startCalls.Load() != 1 || endCalls.Load() != 1 {
		t.Fatal("start and end disk caches not independent")
	}
	opening.Frames[0].Participants = append(opening.Frames[0].Participants, opening.Frames[0].Participants[0])
	if validProStartDetails(opening, g) {
		t.Fatal("duplicate participant accepted")
	}
	opening.Frames[0].Participants = opening.Frames[0].Participants[:1]
	opening.Frames[0].Participants[0].ID = 77
	if validProStartDetails(opening, g) {
		t.Fatal("foreign participant accepted")
	}
}

func TestR173ProOpeningWorkersReserveForegroundSlot(t *testing.T) {
	cp := proTestProvider()
	started, release := make(chan struct{}, 8), make(chan struct{})
	defer close(release)
	var active, peak atomic.Int32
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "foreground") {
			return specialistTestResponse(r, 200, "{}")
		}
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return nil, errors.New("optional starter unavailable")
	})}
	p := newProRuneProvider(cp, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 7; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g := proTestGame()
			g.ID = fmt.Sprint(i + 100)
			_, _ = p.openingDetails(ctx, g, []int{3})
		}(i)
	}
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("background slots failed to start")
		}
	}
	foreground, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	var result map[string]any
	if err := p.fetch(foreground, true, "foreground", nil, &result); err != nil {
		t.Fatal("background requests occupied foreground slot", err)
	}
	cancel()
	wg.Wait()
	if peak.Load() > 3 {
		t.Fatal("background opening concurrency exceeded", peak.Load())
	}
}

func TestR173StartupUsesOnlyDiskProfilesThenPageRefreshes(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	seeds := new(app).loadProSeeds(context.Background(), nil)
	account := seeds[0].Members[0].Summoners[0]
	account.CheckedAt = time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
	account.Rank = json.RawMessage(`{"tier":"MASTER"}`)
	data, _ := json.Marshal(proProfileSnapshot{Account: account})
	disk := newPublicBinaryCache(store, "pro-profiles", 64, 4<<20)
	key := "pro-profile-v2|" + proLadderAccountKey(account.GameName, account.TagLine)
	if _, err := disk.load(context.Background(), key, 15*time.Minute, 7*24*time.Hour, true, func(context.Context) ([]byte, error) { return data, nil }); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	profiles := make(chan struct{}, 64)
	cp := proTestProvider()
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if strings.Contains(r.URL.Path, "/summoners/") {
			profiles <- struct{}{}
		}
		return nil, errors.New("fixture offline")
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &app{storage: store, champions: cp, proRefreshContext: ctx}
	a.warmProPlayersCaches()
	if requests.Load() != 0 {
		t.Fatal("startup downloaded profiles", requests.Load())
	}
	cached := a.proProfiles.entries[key]
	if cached.CheckedAt != account.CheckedAt || !strings.Contains(string(cached.Rank), "MASTER") || !cached.Stale {
		t.Fatal("startup failed to preserve seven-day cached profile", cached)
	}
	if !a.proPlayers.fetchedAt.IsZero() || !a.proPlayers.attemptedAt.IsZero() {
		t.Fatal("startup fabricated network freshness")
	}
	if a.proIdentitySnapshot().candidates == 0 {
		t.Fatal("offline manual pro identity missing")
	}
	rec := httptest.NewRecorder()
	a.handleProPlayers(rec, httptest.NewRequest("GET", "/api/pro-players", nil))
	select {
	case <-profiles:
	case <-time.After(2 * time.Second):
		t.Fatal("opening pro page did not refresh expired profiles", requests.Load(), rec.Body.String())
	}
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSource), "a.warmProPlayersCaches()") || strings.Contains(string(mainSource), "a.loadProPlayers(runtimeContext, true)") {
		t.Fatal("main startup restored forced network refresh")
	}
}

func TestR173StarterRequestBudgetAndOptionalFailure(t *testing.T) {
	a := &app{}
	rows := make([]runeStarterRowRequest, 10)
	for i := range rows {
		rows[i] = runeStarterRowRequest{fmt.Sprint(i), 1700000000000}
	}
	body, _ := json.Marshal(runeStarterRequest{Source: "specialist", ChampionID: 64, Position: "mid", Rows: rows})
	rec := httptest.NewRecorder()
	a.handleGameplayRuneStarters(rec, httptest.NewRequest("POST", "/", strings.NewReader(string(body))))
	if rec.Code != 400 || runeStarterRowLimit != 9 {
		t.Fatal("independent nine-game budget not enforced", rec.Code)
	}
	body, _ = json.Marshal(runeStarterRequest{Source: "specialist", ChampionID: 64, Position: "mid", Rows: rows[:9]})
	rec = httptest.NewRecorder()
	a.handleGameplayRuneStarters(rec, httptest.NewRequest("POST", "/", strings.NewReader(string(body))))
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"starters":[]}` {
		t.Fatal("optional source failure changed rune list", rec.Code, rec.Body.String())
	}
}

func TestR173OpeningInventoryCapsAndUndoSale(t *testing.T) {
	events := []timelineEvent{
		{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 5000, ItemID: 1055},
		{Type: "ITEM_SOLD", ParticipantID: 1, Timestamp: 6000, ItemID: 1055},
		{Type: "ITEM_UNDO", ParticipantID: 1, Timestamp: 7000, AfterID: 1055},
	}
	for i := 0; i < 7; i++ {
		events = append(events, timelineEvent{Type: "ITEM_PURCHASED", ParticipantID: 1, Timestamp: 8000, ItemID: 2003})
	}
	got := participantStarterItems([]timelineFrame{{Events: events}}, 1)
	if !reflect.DeepEqual(got, []int64{1055, 2003, 2003, 2003, 2003, 2003}) {
		t.Fatal("undo sale or separate six-slot cap", got)
	}
}

func TestR173BackgroundTerminalAndOpeningShareThreeSlots(t *testing.T) {
	cp := proTestProvider()
	started := make(chan struct{}, 8)
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "foreground") {
			return specialistTestResponse(r, 200, "{}")
		}
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	p := newProRuneProvider(cp, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var result map[string]any
			_ = p.fetch(withProBackground(ctx), true, fmt.Sprintf("window/%d", 900+i), nil, &result)
		}(i)
	}
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("existing background did not start")
		}
	}
	wg.Add(1)
	go func() { defer wg.Done(); _, _ = p.openingDetails(ctx, proTestGame(), []int{3}) }()
	select {
	case <-started:
		t.Fatal("opening plus terminal work occupied fourth slot")
	case <-time.After(30 * time.Millisecond):
	}
	foreground, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	var result map[string]any
	if err := p.fetch(foreground, true, "foreground", nil, &result); err != nil {
		t.Fatal(err)
	}
	cancel()
	wg.Wait()
}
