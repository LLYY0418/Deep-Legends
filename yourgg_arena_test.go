package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConvertYourGGArenaMatchDeduplicatesSubjectAndStripsStableIDs(t *testing.T) {
	var raw yourGGArenaMatch
	fixture := `{
		"matchId":8347514443,"matchDate":1787161553761,"gameTime":1511,"matchCategory":"Arena","result":"WIN",
		"me":{"championKey":"Ambessa","riotIdGameName":"Subject","riotIdTagline":"KR1","summonerId":"stable-me-a","subteamId":3,"subteamPlacement":1,"level":18,"kills":22,"deaths":7,"assists":14,"kda":5.14,"gold":16260,"damage":98209,"damageTaken":42560,"items":[{"id":447116},{"id":0}],"augments":[{"id":73},{"id":0}]},
		"participants":[
			{"championKey":"Ambessa","riotIdGameName":"subject","riotIdTagline":"kr1","summonerId":"stable-me-b","subteamId":3,"subteamPlacement":1},
			{"championKey":"Alistar","riotIdGameName":"Mate","riotIdTagline":"KR1","summonerId":"stable-mate","subteamId":3,"subteamPlacement":1},
			{"championKey":"Pyke","riotIdGameName":"Opponent","riotIdTagline":"KR1","summonerId":"stable-opponent","subteamId":6,"subteamPlacement":2}
		]
	}`
	if err := json.Unmarshal([]byte(fixture), &raw); err != nil {
		t.Fatal(err)
	}
	lookup := map[string]championMetadata{
		"ambessa": {ID: 799, Key: "Ambessa", NameZH: "安蓓萨"},
		"alistar": {ID: 12, Key: "Alistar", NameZH: "阿利斯塔"},
		"pyke":    {ID: 555, Key: "Pyke", NameZH: "派克"},
	}
	match, ok := convertYourGGArenaMatch(raw, lookup)
	if !ok {
		t.Fatal("expected valid arena match")
	}
	if len(match.Participants) != 3 {
		t.Fatalf("expected subject plus two unique players, got %d", len(match.Participants))
	}
	if match.SubjectParticipantID != 1 || match.Participants[0].ChampionID != 799 || match.Participants[0].Placement != 1 {
		t.Fatalf("unexpected subject mapping: %#v", match.Participants[0])
	}
	if match.Participants[0].Damage != 98209 || match.Participants[0].DamageTaken != 42560 || match.Participants[0].Gold != 16260 {
		t.Fatalf("subject combat stats were not preserved: %#v", match.Participants[0])
	}
	if len(match.Participants[0].ItemIDs) != 1 || len(match.Participants[0].AugmentIDs) != 1 {
		t.Fatalf("zero item/augment slots were not removed: %#v", match.Participants[0])
	}
	encoded, err := json.Marshal(arenaFirstPlacesResponse{Source: "YOUR.GG", Region: "KR", Matches: []gameplayMatch{match}})
	if err != nil {
		t.Fatal(err)
	}
	publicJSON := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"summonerid", "stable-me", "stable-mate", "stable-opponent", "puuid"} {
		if strings.Contains(publicJSON, forbidden) {
			t.Fatalf("public response leaked %q: %s", forbidden, publicJSON)
		}
	}
}

func TestYourGGArenaCachePolicy(t *testing.T) {
	for _, requestPath := range []string{"/kr/api/arena/champions/799", "/kr/api/arena/champions/799/top-builds"} {
		ttl, staleFor, persistDisk := championCachePolicy(yourGGArenaHost, requestPath, "application/json")
		if ttl != 20*time.Minute || staleFor != 0 || persistDisk {
			t.Fatalf("unexpected YOUR.GG cache policy for %s: %s, %s, persist=%v", requestPath, ttl, staleFor, persistDisk)
		}
	}
}

func TestMapYourGGArenaAggregateItemsConvertsRatesAndUsesUpstreamGrades(t *testing.T) {
	fixture := `{
		"response": {
			"coreItems": [
				{"itemId":223071,"tier":"D","score":12.11,"winRate":0.4959,"averagePlacement":3.502,"firstPlacementRate":0.1414,"pickRate":0.2290,"matches":2835},
				{"itemId":446632,"tier":"S","score":88.50,"winRate":0.601,"averagePlacement":2.8,"firstPlacementRate":0.22,"pickRate":0.12,"matches":800}
			],
			"prismaticItems": [{"itemId":223078,"tier":"OP","score":99.9,"winRate":0.7,"averagePlacement":2.1,"firstPlacementRate":0.31,"pickRate":0.05,"matches":200}]
		}
	}`
	var payload yourGGArenaAggregateResponse
	if err := json.Unmarshal([]byte(fixture), &payload); err != nil {
		t.Fatal(err)
	}
	catalog := []gameplayItem{
		{ID: 223071, Name: "黑色切割者", IconPath: "/latest/game/assets/items/3071.png"},
		{ID: 446632, Name: "神圣分离者", IconPath: "/latest/game/assets/items/6632.png"},
		{ID: 223078, Name: "三相之力", IconPath: "/latest/game/assets/items/3078.png"},
	}
	rows := mapYourGGArenaAggregateItems(payload.Response.CoreItems, catalog)
	if len(rows) != 2 || rows[0].Score != 88.5 {
		t.Fatalf("aggregate rows were not score-sorted: %#v", rows)
	}
	row := rows[1]
	if row.Tier != "D" || row.Grade != "D" || row.Games != 2835 || row.Score != 12.11 {
		t.Fatalf("upstream grade/score/sample mapping = %#v", row)
	}
	if math.Abs(row.WinRate-49.59) > 1e-9 || math.Abs(row.FirstPlaceRate-14.14) > 1e-9 || math.Abs(row.PickRate-22.9) > 1e-9 || math.Abs(row.AveragePlacement-3.502) > 1e-9 {
		t.Fatalf("aggregate rate conversion = %#v", row)
	}
	asset := row.Assets[0]
	if asset.ID != 223071 || asset.Name != "黑色切割者" || asset.Source != "communitydragon" || asset.Path != "/latest/game/assets/items/3071.png" {
		t.Fatalf("item catalog mapping = %#v", asset)
	}
	if prism := mapYourGGArenaAggregateItems(payload.Response.PrismaticItems, catalog); len(prism) != 1 || prism[0].Grade != "OP" || prism[0].Assets[0].Name != "三相之力" {
		t.Fatalf("prismatic mapping = %#v", prism)
	}
}

func TestLoadArenaChampionAggregateUsesAggregateEndpointAndCatalog(t *testing.T) {
	provider := newChampionProvider()
	provider.clientMu.Lock()
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body string
		switch request.URL.Host {
		case yourGGArenaHost:
			if request.URL.Path != "/kr/api/arena/champions/799" {
				return nil, errors.New("aggregate endpoint path changed: " + request.URL.Path)
			}
			body = `{"response":{"coreItems":[],"prismaticItems":[]}}`
		case communityDragonHost:
			if request.URL.Path != "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json" {
				return nil, errors.New("item catalog endpoint path changed: " + request.URL.Path)
			}
			body = `[{"id":223071,"name":"黑色切割者","iconPath":"/lol-game-data/assets/Items/3071.png","priceTotal":3000}]`
		default:
			return nil, errors.New("unexpected aggregate host: " + request.URL.Host)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	provider.clientMu.Unlock()

	payload, catalog, _, err := provider.loadArenaChampionAggregate(context.Background(), 799)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Response.CoreItems == nil || catalog == nil || len(catalog) != 1 || catalog[0].Name != "黑色切割者" || catalog[0].Price != 3000 {
		t.Fatalf("aggregate/catalog response = %#v %#v", payload.Response, catalog)
	}
}

func TestLoadArenaFirstPlacesPreservesMemoryCacheFetchedAt(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	requestPath := "/kr/api/arena/champions/799/top-builds"
	key := championCacheKey(yourGGArenaHost, requestPath, "limit=10", "application/json")
	fetchedAt := time.Now().Add(-10 * time.Minute).Round(0)
	provider.cache.mu.Lock()
	provider.cache.storeMemoryLocked(key, championCacheEnvelope{
		Key: key, FetchedAt: fetchedAt, ExpiresAt: time.Now().Add(10 * time.Minute),
		Data: []byte(`{"success":true,"statusCode":200,"response":{"version":"16.16","builds":[]}}`),
	})
	provider.cache.mu.Unlock()

	response, err := provider.loadArenaFirstPlaces(context.Background(), 799, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !response.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("arena first places fetchedAt = %v, want memory cache time %v", response.FetchedAt, fetchedAt)
	}
}

func TestResolveChampionIDAcceptsNumericWithoutCatalog(t *testing.T) {
	provider := newChampionProvider()
	id, metadata, err := provider.resolveChampionID(context.Background(), "799")
	if err != nil || id != 799 || metadata.ID != 0 {
		t.Fatalf("numeric fallback = id:%d metadata:%#v err:%v", id, metadata, err)
	}
}

func TestFilterYourGGArenaBuildsReportsDropReasons(t *testing.T) {
	build := func(matchID int64, champion string, placement int) yourGGArenaBuild {
		return yourGGArenaBuild{Match: yourGGArenaMatch{
			MatchID: matchID, MatchDate: 1_787_161_553_761, GameTime: 900,
			Me: yourGGArenaSubject{yourGGArenaParticipant: yourGGArenaParticipant{
				ChampionKey: champion, SubteamID: 1, SubteamPlacement: placement,
			}},
		}}
	}
	builds := []yourGGArenaBuild{
		build(1, "Ambessa", 1),
		build(0, "Ambessa", 1),
		build(2, "FutureChampion", 1),
		build(3, "Alistar", 1),
		build(4, "Ambessa", 2),
	}
	lookup := map[string]championMetadata{
		"ambessa": {ID: 799, Key: "Ambessa"},
		"alistar": {ID: 12, Key: "Alistar"},
	}
	matches, stats := filterYourGGArenaBuilds(builds, lookup, 799, 10)
	if len(matches) != 1 || stats.Returned != 5 || stats.Accepted != 1 || stats.Invalid != 1 || stats.UnknownChampion != 1 || stats.ChampionMismatch != 1 || stats.NotFirst != 1 {
		t.Fatalf("unexpected filter funnel: matches=%d stats=%+v", len(matches), stats)
	}
	for reason, candidate := range map[string]arenaFirstPlaceFilterStats{
		"empty":                      {},
		"champion-metadata-mismatch": {Returned: 1, UnknownChampion: 1},
		"champion-mismatch":          {Returned: 1, ChampionMismatch: 1},
		"no-first-place":             {Returned: 1, NotFirst: 1},
		"invalid-upstream-data":      {Returned: 1, Invalid: 1},
	} {
		if got := arenaFirstUnavailableReason(candidate, 0); got != reason {
			t.Fatalf("reason for %+v = %q, want %q", candidate, got, reason)
		}
	}
}

func TestArenaFirstPlacesDiagnosticsUseAggregateFieldsOnly(t *testing.T) {
	provider := newChampionProvider()
	events := make([]map[string]any, 0, 2)
	provider.diag = func(event map[string]any) { events = append(events, event) }
	provider.reportArenaFirstPlaces(arenaFirstPlaceFilterStats{Returned: 6, Accepted: 1, Invalid: 1, UnknownChampion: 1, ChampionMismatch: 1, NotFirst: 2}, 1)
	provider.reportArenaFirstPlacesFailure("forbidden")
	if len(events) != 2 {
		t.Fatalf("diagnostic event count = %d", len(events))
	}
	wantSuccess := map[string]bool{
		"event": true, "source": true, "returned": true, "accepted": true, "rowsOut": true,
		"invalid": true, "unknownChampion": true, "championMismatch": true, "notFirst": true,
	}
	for key := range events[0] {
		if !wantSuccess[key] {
			t.Fatalf("unexpected first-place success field %q", key)
		}
	}
	if len(events[0]) != len(wantSuccess) || events[0]["event"] != "arena_first_places" || events[0]["rowsOut"] != 1 {
		t.Fatalf("unexpected first-place success event: %#v", events[0])
	}
	wantFailure := map[string]bool{"event": true, "source": true, "outcome": true, "errorKind": true}
	for key := range events[1] {
		if !wantFailure[key] {
			t.Fatalf("unexpected first-place failure field %q", key)
		}
	}
	if len(events[1]) != len(wantFailure) || events[1]["errorKind"] != "forbidden" {
		t.Fatalf("unexpected first-place failure event: %#v", events[1])
	}
}

func TestInferYourGGSubjectChampionOnlyForSingleUnknownKey(t *testing.T) {
	build := func(key string) yourGGArenaBuild {
		return yourGGArenaBuild{Match: yourGGArenaMatch{Me: yourGGArenaSubject{yourGGArenaParticipant: yourGGArenaParticipant{ChampionKey: key}}}}
	}
	lookup := map[string]championMetadata{}
	inferYourGGSubjectChampion(lookup, []yourGGArenaBuild{build("NewChampion"), build("newchampion")}, 999)
	if lookup["newchampion"].ID != 999 {
		t.Fatalf("single unknown key was not inferred: %#v", lookup)
	}
	lookup = map[string]championMetadata{}
	inferYourGGSubjectChampion(lookup, []yourGGArenaBuild{build("One"), build("Two")}, 999)
	if len(lookup) != 0 {
		t.Fatalf("ambiguous keys must not be inferred: %#v", lookup)
	}
}

func TestWriteArenaFirstPlacesErrorClassifiesSafeFailures(t *testing.T) {
	cases := []struct {
		err  error
		code int
		text string
	}{
		{errors.New("champion provider returned HTTP 403"), http.StatusBadGateway, "HTTP 403"},
		{errors.New("champion provider returned HTTP 404"), http.StatusBadGateway, "HTTP 404"},
		{context.DeadlineExceeded, http.StatusGatewayTimeout, "请求超时"},
		{errors.New("champion provider returned an empty response"), http.StatusBadGateway, "空响应"},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeArenaFirstPlacesError(recorder, testCase.err)
		if recorder.Code != testCase.code || !strings.Contains(recorder.Body.String(), testCase.text) {
			t.Fatalf("error %v => %d %q", testCase.err, recorder.Code, recorder.Body.String())
		}
	}
}
