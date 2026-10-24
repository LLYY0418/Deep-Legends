package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR303AutoConfigSkipDeduplicatesPerSession(t *testing.T) {
	a := r175App(t)
	a.autoConfig.phase = "ChampSelect"
	for _, reason := range []string{"disabled", "category_off", "no_mode", "no_config", "not_locked", "yielded", "guard"} {
		for i := 0; i < 3; i++ {
			a.recordAutoConfigSkip(reason, "normal")
		}
	}
	rows := readDiagnosticEvents(t, a.storage, "auto_config_skip")
	if len(rows) != 7 {
		t.Fatal(rows)
	}
	for _, row := range rows {
		if row["modeKey"] != "normal" || row["phase"] != "ChampSelect" {
			t.Fatal(row)
		}
	}
	a.resetAutoConfig()
	a.recordAutoConfigSkip("disabled", "normal")
	if len(readDiagnosticEvents(t, a.storage, "auto_config_skip")) != 8 {
		t.Fatal("new session must emit again")
	}
}
func TestR303ProgressionTargetHelpAndCanonicalGETFallback(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(map[bool]string{false: "fallback", true: "target"}[full], func(t *testing.T) {
			a := r175App(t)
			c := &LCUClient{baseURL: "https://fixture", token: "test"}
			var callsMu sync.Mutex
			calls := map[string]int{}
			c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				callsMu.Lock()
				defer callsMu.Unlock()
				if r.Method != "GET" {
					t.Fatal(r.Method)
				}
				if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 2100*time.Millisecond {
					t.Fatal("unbounded request")
				}
				calls[r.URL.Path]++
				if r.URL.Path == "/help" {
					name := r.URL.Query().Get("target")
					if name == "" || r.URL.Query().Get("format") != "Full" {
						t.Fatal(r.URL)
					}
					if full {
						paths := map[string]string{"GetLolProgressionV1GroupsConfiguration": "/lol-progression/v7/groups/configuration", "GetLolProgressionV1GroupsByGroupIdInstanceData": "/lol-progression/v7/groups/{groupId}/instance-data", "GetLolInventoryV1CherryInventory": "/lol-inventory/v7/cherry-inventory"}
						raw, _ := json.Marshal(map[string]string{"name": name, "method": "GET", "path": paths[name]})
						return r206RelayResponse(200, raw), nil
					}
					return r206RelayResponse(404, nil), nil
				}
				if strings.HasSuffix(r.URL.Path, "configuration") {
					return r206RelayResponse(200, []byte(`{"groups":[{"groupId":"safe-group","name":"Arena"}]}`)), nil
				}
				return r206RelayResponse(200, []byte(`{"fame":42,"puuid":"PRIVATE","playerId":123}`)), nil
			})}
			snap := a.readArenaProgressionSnapshot(context.Background(), c, arenaDiscoveryHelpAll(json.RawMessage(`{"GetLolProgressionV1GroupsConfiguration":"","GetLolProgressionV1GroupsByGroupIdInstanceData":"","GetLolInventoryV1CherryInventory":""}`)), Summoner{})
			if calls["/help"] != 3 || snap.HTTPStatus["configuration"] != 200 || snap.HTTPStatus["instance-data"] != 200 || snap.HTTPStatus["cherry-inventory"] != 200 {
				t.Fatal(calls, snap)
			}
			version := "v1"
			if full {
				version = "v7"
			}
			if calls["/lol-progression/"+version+"/groups/safe-group/instance-data"] != 1 {
				t.Fatal(calls)
			}
			raw, _ := json.Marshal(snap.ResolvedPaths)
			if strings.Contains(string(raw), "safe-group") {
				t.Fatal("group leaked")
			}
			for key := range snap.Values {
				if strings.Contains(key, "puuid") || strings.Contains(key, "playerId") {
					t.Fatal(key)
				}
			}
		})
	}
}
func TestR303ProgressionEventNumericDiffAndPrivacy(t *testing.T) {
	c := &LCUClient{}
	s := c.arenaDiscoveryState()
	emit := func(value string) {
		c.observeArenaDiscoveryEvent(LCUEvent{URI: "/lol-progression/v1/private-account/private-player/private-key", Data: json.RawMessage(value)})
		c.observeArenaDiscoveryEvent(LCUEvent{URI: "/private-account/v1/private-player/private-key/progression/v1/notifications/123456/654321", Data: json.RawMessage(value)})
	}
	emit(`{"fame":10,"puuid":"PRIVATE","playerId":123,"longAccountKey12345678901234567890123456789":{"xp":999}}`)
	before := arenaProgressionEventSnapshot(s)
	emit(`{"fame":25,"puuid":"PRIVATE","playerId":123}`)
	after := arenaProgressionEventSnapshot(s)
	diff := arenaProgressionDiff(arenaProgressionSnapshot{Values: before}, arenaProgressionSnapshot{Values: after})
	if len(diff) != 2 {
		t.Fatal(diff)
	}
	raw, _ := json.Marshal(diff)
	for _, secret := range []string{"private-account", "private-player", "123456", "654321", "puuid", "playerId", "PRIVATE", "longAccount"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal(string(raw))
		}
	}
	for _, delta := range diff {
		if delta.Before != 10 || delta.After != 25 {
			t.Fatal(delta)
		}
	}
}
func TestR303ParticipantSelfUsesLiveIdentityPredicate(t *testing.T) {
	a := r175App(t)
	a.summoner.PUUID = r161Ref(0)
	match := gameplayMatch{Participants: []gameplayParticipant{{PlayerRef: r161Ref(0), GameName: "本人"}, {PlayerRef: r161Ref(1), GameName: "别人"}}}
	a.publicizeMatchReferences(&match)
	if !match.Participants[0].IsCurrent || match.Participants[1].IsCurrent {
		t.Fatal(match.Participants)
	}
}
