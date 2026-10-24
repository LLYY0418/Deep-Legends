package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestR304AutoConfigDisabledSessionEventsDoNotRequestLCU(t *testing.T) {
	a := r175App(t)
	a.autoConfig.loaded, a.autoConfig.settings = true, defaultAutoConfig()
	a.autoConfig.phase = "ChampSelect"
	var calls atomic.Int64
	client := &LCUClient{baseURL: "https://fixture", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return r206RelayResponse(404, nil), nil
	})}}
	cell := int64(0)
	session := lcuChampSelectSession{GameID: 304, QueueID: 430, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 1}}}
	emit := func() {
		raw, err := json.Marshal(session)
		if err != nil {
			t.Fatal(err)
		}
		a.observeAutoConfigEvent(context.Background(), client, LCUEvent{URI: "/lol-champ-select/v1/session", EventType: "Update", Data: raw})
		r300WaitAuto(t, a)
	}
	for i := 0; i < 5; i++ {
		emit()
	}
	a.triggerAutoConfig(context.Background(), client)
	r300WaitAuto(t, a)
	if calls.Load() != 0 {
		t.Fatalf("disabled auto-config requested LCU %d times", calls.Load())
	}
	rows := readDiagnosticEvents(t, a.storage, "auto_config_skip")
	if len(rows) != 1 || rows[0]["reason"] != "disabled" || rows[0]["phase"] != "ChampSelect" {
		t.Fatal(rows)
	}
	session.GameID++
	for i := 0; i < 3; i++ {
		emit()
	}
	rows = readDiagnosticEvents(t, a.storage, "auto_config_skip")
	if len(rows) != 2 || rows[1]["reason"] != "disabled" || calls.Load() != 0 {
		t.Fatal(rows, calls.Load())
	}
}

func TestR304AutoConfigLegacyStarterTrinketsApplyWithoutChangingSavedEntry(t *testing.T) {
	for _, tc := range []struct {
		name       string
		starter    []autoConfigStarterItem
		want       int64
		normalized bool
	}{
		{"unbuyable then legal", []autoConfigStarterItem{{3363, 1}, {3340, 1}}, 3340, true},
		{"duplicate wards", []autoConfigStarterItem{{3340, 1}, {3340, 1}}, 3340, true},
		{"lens quantity", []autoConfigStarterItem{{3364, 2}}, 3364, true},
		{"already legal", []autoConfigStarterItem{{3364, 1}}, 3364, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := r175App(t)
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "Game"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "Game", "League of Legends.exe"), []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			cell := int64(0)
			session := lcuChampSelectSession{GameID: 304, QueueID: 430, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 1}}, Actions: [][]lcuChampSelectAction{{{ActorCellID: cell, ChampionID: 1, Type: "pick", Completed: true}}}}
			flow := lcuGameflowSession{}
			flow.GameData.Queue.ID, flow.Map.ID = 430, 11
			client := &LCUClient{baseURL: "https://fixture", token: "fixture", region: "NA"}
			client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				var data any
				switch r.Method + " " + r.URL.Path {
				case "GET /lol-gameflow/v1/gameflow-phase":
					data = "ChampSelect"
				case "GET /lol-champ-select/v1/session":
					data = session
				case "GET /lol-gameflow/v1/session":
					data = flow
				case "GET /data-store/v1/install-dir":
					data = root
				case "GET /riotclient/command-line-args":
					data = []string{"--region=NA", "--rso_platform_id=NA1"}
				case "GET /lol-platform-config/v1/namespaces/LoginDataPacket/platformId":
					data = "NA1"
				case "GET /lol-item-sets/v1/item-sets/123/sets":
					data = map[string]any{"accountId": 456, "itemSets": []any{}}
				default:
					t.Errorf("unexpected LCU request: %s %s", r.Method, r.URL.Path)
					return r206RelayResponse(404, nil), nil
				}
				raw, err := json.Marshal(data)
				if err != nil {
					return nil, err
				}
				return r206RelayResponse(200, raw), nil
			})}
			a.lcu, a.connected, a.summoner = client, true, Summoner{SummonerID: 123, AccountID: 456}
			a.autoConfig.loaded, a.autoConfig.settings = true, defaultAutoConfig()
			a.autoConfig.settings.Enabled, a.autoConfig.settings.Runes, a.autoConfig.settings.Spells = true, false, false
			entry := autoConfigEntry{Items: &autoConfigItems{Starter: append([]autoConfigStarterItem{{2003, 2}}, tc.starter...), Core: []int64{3071, 3089}, Situational: []int64{3157}}}
			a.autoConfig.settings.Champions["1"] = map[string]autoConfigEntry{"normal": entry}
			before, err := json.Marshal(a.autoConfig.settings)
			if err != nil {
				t.Fatal(err)
			}
			catalog := autoConfigCatalog{}
			for _, id := range []int64{2003, 3340, 3363, 3364, 3071, 3089, 3157} {
				i := autoConfigItem{ID: id, InStore: true, DisplayInItemSets: true, Maps: map[string]bool{"11": true}, From: []int64{1001}}
				if id == 2003 {
					i.Price, i.Categories = 50, []string{"Consumable"}
				} else if id == 3340 || id == 3363 || id == 3364 {
					i.Categories = []string{"Trinket"}
				}
				catalog.Items = append(catalog.Items, i)
			}
			a.autoConfig.catalog, a.autoConfig.catalogClient = &catalog, client
			a.autoConfig.catalogKey, _ = clientCatalogKey(client)
			a.autoConfig.catalogAt = time.Now()
			a.autoConfig.runtime = autoConfigRuntime{Session: autoConfigSessionKey(session), client: client, generation: 1, Results: map[string]string{}}
			a.applyAutoConfig(context.Background(), client, session, 1, 1, "normal", "normal", entry, true)
			if a.autoConfig.runtime.Results["items"] != "applied" {
				t.Fatal(a.autoConfig.runtime.Results)
			}
			raw, err := os.ReadFile(filepath.Join(root, "Config", "Global", "Recommended", recommendedItemSetUID+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var written recommendedItemSet
			if err := json.Unmarshal(raw, &written); err != nil {
				t.Fatal(err)
			}
			want := []recommendedItemSetBlock{{Type: "出门装", Items: []lcuItemSetItem{{ID: "2003", Count: 2}, {ID: fmt.Sprint(tc.want), Count: 1}}}, {Type: "核心出装", Items: []lcuItemSetItem{{ID: "3071", Count: 1}, {ID: "3089", Count: 1}}}, {Type: "备选", Items: []lcuItemSetItem{{ID: "3157", Count: 1}}}}
			if !reflect.DeepEqual(written.Blocks, want) {
				t.Fatalf("written blocks = %#v", written.Blocks)
			}
			after, err := json.Marshal(a.autoConfig.settings)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("automatic normalization rewrote saved settings")
			}
			rows := readDiagnosticEvents(t, a.storage, "auto_config_apply")
			if len(rows) != 1 {
				t.Fatal(rows)
			}
			items := rows[0]["results"].(map[string]any)["items"]
			if tc.normalized {
				result, ok := items.(map[string]any)
				if !ok || result["status"] != "applied" || result["normalized"] != true {
					t.Fatal(items)
				}
			} else if items != "applied" {
				t.Fatal(items)
			}
		})
	}
}
