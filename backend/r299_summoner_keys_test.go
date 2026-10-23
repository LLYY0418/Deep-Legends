package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR299SummonerBindingPriority(t *testing.T) {
	for _, tc := range []struct {
		name     string
		bindings [2][4]string // Cast, SmartCast, SmartPlusSelfCast, NormalCast.
		want     [2]string
	}{
		{"default cast keys", [2][4]string{{"[d]", "[Shift][d]"}, {"[f]", "[Shift][f]"}}, [2]string{"D", "F"}},
		{"smart plus self bare keys", [2][4]string{{"[Shift][d]", "", "[d]"}, {"[Shift][f]", "", "[f]"}}, [2]string{"D", "F"}},
		{"only smart plus self", [2][4]string{{"", "", "[d]"}, {}}, [2]string{"D", ""}},
		{"smart cast bare keys", [2][4]string{{"[Shift][d]", "[n],[1]"}, {"[Shift][f]", "[m]"}}, [2]string{"N", "M"}},
		{"normal cast bare keys", [2][4]string{{"[Shift][d]", "[Ctrl][d]", "[Alt][d]", "[n]"}, {"", "", "", "[m]"}}, [2]string{"N", "M"}},
		{"first bare candidate wins", [2][4]string{{"[d]", "[n]", "[q]", "[w]"}, {"", "[f]", "[m]", "[e]"}}, [2]string{"D", "F"}},
		{"first modified candidate wins", [2][4]string{{"[Shift][d]", "[Ctrl][n]", "[Alt][q]", "[Shift][w]"}, {"", "[Ctrl][f]", "[Alt][m]", "[Shift][e]"}}, [2]string{"Shift+D", "Ctrl+F"}},
		{"slots are independent", [2][4]string{{"[Shift][d]"}, {"", "", "[m]", "[f]"}}, [2]string{"Shift+D", "M"}},
		{"all unbound ignores self cast", [2][4]string{}, [2]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := map[string]string{}
			var ini strings.Builder
			ini.WriteString("[GameEvents]\n")
			for slot, bindings := range tc.bindings {
				for i, event := range []string{"evtCastAvatarSpell", "evtSmartCastAvatarSpell", "evtSmartPlusSelfCastAvatarSpell", "evtNormalCastAvatarSpell"} {
					value := bindings[i]
					if value == "" {
						value = "[<Unbound>]"
					}
					key := fmt.Sprintf("%s%d", event, slot+1)
					events[key] = value
					fmt.Fprintf(&ini, "%s=%s\n", key, value)
				}
				key := fmt.Sprintf("evtSelfCastAvatarSpell%d", slot+1)
				events[key] = "[x]"
				fmt.Fprintf(&ini, "%s=[x]\n", key)
			}
			other := map[string]string{"evtCastAvatarSpell1": "[q]", "evtCastAvatarSpell2": "[w]"}
			data, err := json.Marshal(map[string]any{"GameEvents": events, "Other": other})
			if err != nil {
				t.Fatal(err)
			}
			if got := summonerKeysFromJSON(data); got != tc.want {
				t.Errorf("LCU keys = %q, want %q", got, tc.want)
			}
			ini.WriteString("[Other]\nevtCastAvatarSpell1=[q]\nevtCastAvatarSpell2=[w]\n")
			if got := summonerKeysFromValues(allSettingsFromINI([]byte(ini.String()), false)); got != tc.want {
				t.Errorf("INI keys = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestR299SummonerRefreshOverwritesCachedModifiedKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "summoner-keys.json"), []byte(`{"keys":["Shift+D","Shift+F"],"flashSlot":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: &localStore{root: dir}}
	a.loadSummonerKeysLocked()
	if a.autoConfig.keys.Keys != [2]string{"Shift+D", "Shift+F"} {
		t.Fatal("old persisted keys not loaded", a.autoConfig.keys.Keys)
	}
	client := &LCUClient{baseURL: "https://127.0.0.1", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/lol-game-settings/v1/input-settings" {
			t.Errorf("unexpected %s %s", req.Method, req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"GameEvents":{"evtCastAvatarSpell1":"[d]","evtSmartCastAvatarSpell1":"[Shift][d]","evtCastAvatarSpell2":"[f]","evtSmartCastAvatarSpell2":"[Shift][f]"}}`))}, nil
	})}}
	a.refreshSummonerKeys(context.Background(), client)
	if a.autoConfig.keys.Keys != [2]string{"D", "F"} {
		t.Fatal("refresh did not replace cached keys", a.autoConfig.keys.Keys)
	}
	fresh := &app{storage: a.storage}
	fresh.loadSummonerKeysLocked()
	if fresh.autoConfig.keys.Keys != [2]string{"D", "F"} || fresh.autoConfig.keys.FlashSlot == nil || *fresh.autoConfig.keys.FlashSlot != 1 {
		t.Fatalf("corrected keys or flash slot not persisted: %+v", fresh.autoConfig.keys)
	}
}
