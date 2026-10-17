package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR282ArenaDiscoveryReadOnlyOnceAndPrivate(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	os.MkdirAll(filepath.Join(store.root, "logs"), 0755)
	a := &app{storage: store, summoner: Summoner{SummonerID: 12345, PUUID: "PRIVATE-PUUID"}}
	completed := make(chan struct{}, 2)
	var calls atomic.Int64
	client := &LCUClient{baseURL: "https://fixture", token: "test", sessionContext: context.Background()}
	client.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Errorf("write %s", r.Method)
		}
		if r.URL.Path == "/help" {
			calls.Add(1)
			return r206RelayResponse(200, []byte(`{"functions":[{"method":"GET","path":"/lol-cherry/v1/fame/{summonerId}"},{"method":"POST","path":"/lol-cherry/v1/reset"},{"method":"GET","path":"/lol-cherry/v1/other/{otherId}"},{"method":"GET","path":"/lol-cherry/v1/progression"}],"events":{"OnJsonApiEvent_lol_cherry_v1_progression":{}}}`)), nil
		}
		if r.URL.Path == "/lol-cherry/v1/progression" {
			return r206RelayResponse(204, nil), nil
		}
		if r.URL.Path != "/lol-cherry/v1/fame/12345" {
			t.Errorf("unsafe probe %s", r.URL.Path)
		}
		completed <- struct{}{}
		return r206RelayResponse(200, []byte(`{"puuid":"PRIVATE-PUUID","summonerId":12345,"gameName":"PRIVATE-NAME","playerProgression":{"fame":47600,"level":12,"tier":5,"xp":27},"unrelated":99}`)), nil
	})}
	client.rememberArenaQueueTypes("lcu", arenaDiscoveryQueueTypes(json.RawMessage(`{"queues":[{"queueType":"RANKED_SOLO_5x5"}]}`)))
	client.rememberArenaQueueTypes("sgp", []string{"RANKED_FLEX_SR"})
	a.discoverArenaFame(client)
	<-completed
	// Wait for the asynchronous aggregation by sharing its mutex, without sleeps.
	client.observeArenaDiscoveryEvent(LCUEvent{URI: "/lol-cherry/v1/progression/12345"})
	a.finishArenaFameDiscovery(client.arenaDiscoveryState())
	<-client.arenaDiscoveryState().done
	a.finishArenaFameDiscovery(client.arenaDiscoveryState())
	a.discoverArenaFame(client)
	<-client.arenaDiscoveryState().done
	events := readDiagnosticEvents(t, store, "arena_fame_discovery")
	if len(events) != 1 || calls.Load() != 1 {
		t.Fatalf("events=%d calls=%d", len(events), calls.Load())
	}
	raw, _ := json.Marshal(events[0])
	if len(raw) > 8192 {
		t.Fatal("oversized discovery")
	}
	for _, secret := range []string{"PRIVATE-PUUID", "PRIVATE-NAME", "summonerId", "puuid", "12345"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("identity leaked: %s", raw)
		}
	}
	for _, wanted := range []string{"47600", "RANKED_SOLO_5x5", "RANKED_FLEX_SR", "{id}", "playerProgression.fame", `"status":204`} {
		if !strings.Contains(string(raw), wanted) {
			t.Fatalf("missing %s: %s", wanted, raw)
		}
	}
}

func TestR282BraveryActionAndIntentPreservedForOwnTeam(t *testing.T) {
	cell := int64(7)
	for _, mode := range []string{"action", "champion", "intent"} {
		s := lcuChampSelectSession{MyTeam: []lcuChampSelectPlayer{{CellID: &cell, PUUID: "r282-private-reference"}}}
		switch mode {
		case "action":
			s.Actions = [][]lcuChampSelectAction{{{ActorCellID: cell, Type: "pick", ChampionID: -3}}}
		case "champion":
			s.MyTeam[0].ChampionID = -3
		case "intent":
			s.MyTeam[0].ChampionPickIntent = -3
		}
		rows := mergeChampSelectPlayers(nil, s, 0)
		if len(rows) != 1 || !rows[0].player.Bravery {
			t.Fatalf("%s lost bravery: %+v", mode, rows)
		}
	}
}

func TestR282UnverifiedRankedFameRemainsHidden(t *testing.T) {
	a := &app{}
	c := &LCUClient{baseURL: "https://fixture", token: "test", http: &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		return r206RelayResponse(200, []byte(`{"queues":[{"queueType":"CHERRY","ratedTier":"GLADIATOR","ratedRating":87510}]}`)), nil
	})}}
	if a.loadLiveArenaFame(context.Background(), c, gameplayReference{}, "r282-private-reference", false, "") != nil {
		t.Fatal("unverified ranked fields exposed as Fame")
	}
}
