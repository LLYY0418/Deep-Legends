package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestR212RelayNotFoundAndHTTPStatusSummary(t *testing.T) {
	for _, status := range []int{404, 400} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			p, clock, _ := r208Relay(t, r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/health" || strings.Contains(r.URL.Path, "/status/") {
					return r206RelayResponse(200, []byte(`{}`)), nil
				}
				return r206RelayResponse(status, []byte(`{"status":{"message":"fixture"}}`)), nil
			}))
			var summary map[string]any
			p.champions.diag = func(row map[string]any) {
				if row["event"] == "riot_relay_request_summary" {
					summary = row
				}
			}
			err := p.get(context.Background(), riotPlatformHost, "/lol/spectator/v5/active-games/by-summoner/PRIVATE-IDENTITY", nil, &map[string]any{})
			if status == 404 && !errors.Is(err, errRiotNotFound) {
				t.Fatal(err)
			}
			if status == 400 {
				var failure *riotStatusError
				if !errors.As(err, &failure) || failure.status != 400 {
					t.Fatal(err)
				}
			}
			clock.advance(10 * time.Minute)
			riotRelays.flushSummary(p.champions.diag)
			failures := summary["failures"].(map[string]any)
			if status == 404 {
				if len(failures) != 0 || summary["not_found"] != 1 {
					t.Fatal(summary)
				}
			} else {
				if failures["http"] != 1 || summary["http_statuses"].(map[string]int)["400"] != 1 || summary["not_found"] != 0 {
					t.Fatal(summary)
				}
			}
			if summary["categories"].(map[string]int)["spectator"] != 1 || summary["requests"] != 1 {
				t.Fatal(summary)
			}
			raw, _ := json.Marshal(summary)
			for _, secret := range []string{"PRIVATE-IDENTITY", "relay.example", "by-summoner"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("identity in summary", string(raw))
				}
			}
			// Neither 404 nor 400 should change the active relay or create a cooldown.
			if err := riotRelays.requestErrorFor(riotPlatformHost, "/lol/spectator/v5/active-games/by-summoner/OTHER"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestR212RelayCategoriesAndSummaryReset(t *testing.T) {
	routes := map[string]string{"/riot/account/v1/accounts/by-riot-id/PRIVATE/NAME": "account", "/lol/summoner/v4/summoners/by-puuid/PRIVATE": "summoner", "/lol/match/v5/matches/KR_PRIVATE": "match", "/lol/league/v4/entries/by-puuid/PRIVATE": "league", "/lol/spectator/v5/active-games/by-summoner/PRIVATE": "spectator", "/lol/champion-mastery/v4/champion-masteries/by-puuid/PRIVATE": "mastery", "/unknown/PRIVATE": "other"}
	clock := &r208Clock{stamp: time.Now()}
	state := &riotRelayState{now: clock.now}
	t.Cleanup(func() {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.summaryTimer != nil {
			state.summaryTimer.Stop()
			state.summaryTimer = nil
		}
	})
	var summary map[string]any
	record := func(row map[string]any) { summary = row }
	for route, want := range routes {
		got := riotRelayRequestCategory(route)
		if got != want {
			t.Fatal(route, got, want)
		}
		state.recordRequest("not_found", 404, got, record)
	}
	clock.advance(10 * time.Minute)
	state.flushSummary(record)
	if summary["not_found"] != 7 || len(summary["failures"].(map[string]any)) != 0 || len(summary["categories"].(map[string]int)) != 7 {
		t.Fatal(summary)
	}
	state.recordRequest("", 200, "arbitrary identity", record)
	clock.advance(10 * time.Minute)
	state.flushSummary(record)
	if summary["not_found"] != 0 || summary["requests"] != 1 || summary["http_statuses"].(map[string]int)["404"] != 0 || summary["categories"].(map[string]int)["other"] != 1 {
		t.Fatal(summary)
	}
}

func TestR212RelayQuotaPageKeepsR208Priority(t *testing.T) {
	p, clock, _ := r208Relay(t, r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/health" || strings.Contains(r.URL.Path, "/status/") {
			return r206RelayResponse(200, []byte(`{}`)), nil
		}
		response := r206RelayResponse(404, []byte(`<html>Error 1027</html>`))
		response.Header.Set("Content-Type", "text/html")
		return response, nil
	}))
	var summary map[string]any
	p.champions.diag = func(row map[string]any) {
		if row["event"] == "riot_relay_request_summary" {
			summary = row
		}
	}
	err := p.get(t.Context(), riotPlatformHost, "/lol/spectator/v5/active-games/by-summoner/PRIVATE", nil, &map[string]any{})
	if !errors.Is(err, errRiotRelayQuotaExhausted) {
		t.Fatal("quota page became a normal not-found result", err)
	}
	clock.advance(10 * time.Minute)
	riotRelays.flushSummary(p.champions.diag)
	if summary["not_found"] != 0 || summary["failures"].(map[string]any)["quota_exhausted"] != 1 || summary["http_statuses"].(map[string]int)["404"] != 1 {
		t.Fatal(summary)
	}
}

func TestR212InstallTimingDetailsAndLegacy(t *testing.T) {
	for _, kind := range []string{"complete", "missing", "legacy", "skew"} {
		t.Run(kind, func(t *testing.T) {
			stages := map[string]int64{}
			for i, key := range updateInstallTimingStages {
				stages[key] = 1000 + int64(i)*100
			}
			if kind == "missing" {
				delete(stages, "oninit")
			}
			if kind == "legacy" {
				for k := range stages {
					if detailedInstallTimingStage(k) {
						delete(stages, k)
					}
				}
			}
			if kind == "skew" {
				stages["oninit"] = 1001
			}
			root := t.TempDir()
			raw, _ := json.Marshal(stages)
			os.WriteFile(filepath.Join(root, "update-install-timing.json"), raw, 0600)
			var event map[string]any
			consumeUpdateInstallTiming(root, func(row map[string]any) { event = row })
			durations := event["stages_ms"].(map[string]any)
			details := []string{"parent_exited_to_payload_released", "payload_released_to_nsis_start", "nsis_start_to_oninit", "oninit_to_check_done"}
			if kind == "complete" {
				if event["result"] != "ok" {
					t.Fatal(event)
				}
				for _, name := range details {
					if durations[name] != int64(100) {
						t.Fatal(name, durations)
					}
				}
			}
			if kind == "missing" {
				if event["result"] != "partial" || durations["nsis_start_to_oninit"] != nil || durations["oninit_to_check_done"] != nil {
					t.Fatal(event)
				}
			}
			if kind == "legacy" {
				if event["result"] != "ok" {
					t.Fatal(event)
				}
				for _, name := range details {
					if durations[name] != nil {
						t.Fatal("legacy inferred detail", name, durations)
					}
				}
			}
			if kind == "skew" && event["reason"] != "clock_skew" {
				t.Fatal(event)
			}
			if durations["parent_exited_to_uninstall_old_start"] != int64(500) {
				t.Fatal("aggregate gap disappeared", durations)
			}
		})
	}
}
