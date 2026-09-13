package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func Test1042ClaimSettlementSuppressesLagNotNewEntitlements(t *testing.T) {
	now := time.Now()
	s := claimSettlements{}
	s.remember(claimEntry{Key: "grant:old", RewardGroupID: "group-old"}, now)
	rows := []claimEntry{{Key: "grant:old", Source: "grant", RewardGroupID: "group-old"}, {Key: "event:old", Source: "event", RewardGroupIDs: []string{"group-old"}}, {Key: "grant:new", Source: "grant", EventID: "same-event", RewardGroupID: "group-new"}}
	scan := claimScanResponse{Items: rows}
	s.filter(&scan, now.Add(9*time.Second))
	if len(scan.Items) != 1 || scan.Items[0].Key != "grant:new" {
		t.Fatalf("stale claim escaped: %+v", scan.Items)
	}
	scan.Items = rows
	s.filter(&scan, now.Add(31*time.Second))
	if len(scan.Items) != 3 {
		t.Fatal("suppression did not expire")
	}
	other := claimSettlements{}
	scan.Items = rows
	other.filter(&scan, now)
	if len(scan.Items) != 3 {
		t.Fatal("settlement crossed client sessions")
	}
	s.remember(claimEntry{Key: "mission:chain", RewardGroupIDs: []string{"first"}}, now)
	scan.Items = []claimEntry{{Key: "mission:chain", Source: "mission", RewardGroupIDs: []string{"next"}}}
	s.filter(&scan, now.Add(time.Second))
	if len(scan.Items) != 1 {
		t.Fatal("next mission reward group was hidden")
	}
}

func Test1042ObjectiveClearOnlyUnreadIDsAndReadback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rejected"}[fail], func(t *testing.T) {
			writes := 0
			marked := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					writes++
					if r.URL.Path != "/lol-missions/v1/player" {
						t.Errorf("wrong endpoint: %s", r.URL.Path)
					}
					var body map[string][]string
					json.NewDecoder(r.Body).Decode(&body)
					if len(body) != 2 || strings.Join(body["missionIds"], ",") != "private-mission" || strings.Join(body["seriesIds"], ",") != "private-series" {
						t.Errorf("unsafe body: %+v", body)
					}
					if fail {
						w.WriteHeader(404)
						return
					}
					marked = true
					w.WriteHeader(204)
					return
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected method %s", r.Method)
				}
				if marked {
					w.Write([]byte(`[]`))
					return
				}
				if r.URL.Path == "/lol-missions/v1/series" {
					w.Write([]byte(`[{"id":"private-series","viewed":false}]`))
					return
				}
				w.Write([]byte(`[{"id":"private-mission","viewed":true,"isNew":true},{"id":"already-read","viewed":true},{"id":"unknown-state"}]`))
			}))
			defer server.Close()
			store := &localStore{root: t.TempDir()}
			os.MkdirAll(filepath.Join(store.root, "logs"), 0755)
			a := &app{storage: store}
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			err := a.clearObjectiveBadge(context.Background(), client)
			if (err != nil) != fail {
				t.Fatalf("error=%v", err)
			}
			if writes != 1 {
				t.Fatalf("writes=%d", writes)
			}
			raw, _ := store.readDiagnosticLog()
			for _, secret := range []string{"private-mission", "private-series", "already-read"} {
				if strings.Contains(string(raw), secret) {
					t.Fatalf("log leaked %s", secret)
				}
			}
			if !strings.Contains(string(raw), "objective_badge_clear") {
				t.Fatal("missing dedicated diagnostics")
			}
		})
	}
}

func Test1042ObjectiveUnreadRejectsMalformedAndUnknown(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `[{"id":"../bad","viewed":false}]`} {
		if _, err := objectiveUnreadIDs([]byte(raw)); err == nil {
			t.Fatal("accepted invalid input", raw)
		}
	}
	ids, err := objectiveUnreadIDs([]byte(`[{"id":"unknown"},{"id":"read","viewed":true}]`))
	if err != nil || len(ids) != 0 {
		t.Fatalf("unknown marked: %v %v", ids, err)
	}
	ids, err = objectiveUnreadIDs([]byte(`[{"id":"progress","viewed":true,"objectives":[{"progress":{"currentProgress":3,"lastViewedProgress":1}}]}]`))
	if err != nil || len(ids) != 1 || ids[0] != "progress" {
		t.Fatalf("unseen progress skipped: %v %v", ids, err)
	}
}
