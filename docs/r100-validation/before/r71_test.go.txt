package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func r71GameLocation(t *testing.T) settingsLocation {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "League of Legends.exe"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	return settingsLocation{allowedRoot: root, installRoot: root, configRoot: filepath.Join(root, "Config")}
}
func r71Recommendation(title string) recommendedItemSet {
	return asRecommendedItemSet(lcuItemSet{Title: title, Blocks: []lcuItemSetBlock{{Type: "核心装", Items: []lcuItemSetItem{{ID: "1001", Count: 1}}}}})
}

func TestR71RecommendedFilesAreBoundedBackedUpAndGameCompatible(t *testing.T) {
	location := r71GameLocation(t)
	for _, title := range []string{"DL · 中路", "DL · 下路", "DL · 打野"} {
		if err := writeRecommendedItemSet(location, r71Recommendation(title), nil); err != nil {
			t.Fatal(err)
		}
	}
	directory := filepath.Join(location.configRoot, "Global", "Recommended")
	files, err := os.ReadDir(directory)
	if err != nil || len(files) != 2 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	data, _ := os.ReadFile(filepath.Join(directory, recommendedItemSetUID+".json"))
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["title"] != "DL · 打野" || doc["type"] != "global" || doc["sortrank"] != float64(0) {
		t.Fatal(doc)
	}
	for _, field := range []string{"associatedChampions", "associatedMaps", "preferredItemSlots"} {
		if len(doc[field].([]any)) != 0 {
			t.Fatalf("unexpected %s", field)
		}
	}
	block := doc["blocks"].([]any)[0].(map[string]any)
	if len(block) != 2 || block["hideIfSummonerSpell"] != nil {
		t.Fatal(block)
	}
	backup, _ := os.ReadFile(filepath.Join(directory, recommendedItemSetUID+".json.bak"))
	if !strings.Contains(string(backup), "DL · 下路") {
		t.Fatalf("backup %s", backup)
	}
	if _, err := os.Stat(filepath.Join(location.configRoot, "game.cfg")); !os.IsNotExist(err) {
		t.Fatal("must not write HUD settings")
	}
}

func TestR71RecommendedFilesFailClosedOnForeignFilesAndLinks(t *testing.T) {
	for _, target := range []string{"file", "backup", "directory", "outside", "missing-exe"} {
		t.Run(target, func(t *testing.T) {
			loc := r71GameLocation(t)
			dir := filepath.Join(loc.configRoot, "Global", "Recommended")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			var foreignPath string
			switch target {
			case "file", "backup":
				foreignPath = filepath.Join(dir, recommendedItemSetUID+".json")
				if target == "backup" {
					foreignPath += ".bak"
				}
				if err := os.WriteFile(foreignPath, []byte(`{"uid":"user-private","blocks":[]}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(dir); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), dir); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			case "outside":
				loc.configRoot = filepath.Join(t.TempDir(), "Config")
			case "missing-exe":
				if err := os.Remove(filepath.Join(loc.installRoot, "League of Legends.exe")); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeRecommendedItemSet(loc, r71Recommendation("DL"), nil); err == nil {
				t.Fatal("unsafe location/file accepted")
			}
			if foreignPath != "" {
				data, _ := os.ReadFile(foreignPath)
				if string(data) != `{"uid":"user-private","blocks":[]}` {
					t.Fatal("foreign file overwritten")
				}
			}
		})
	}
}

func TestR71RecommendedGuardRunsBeforeAndImmediatelyBeforeWrite(t *testing.T) {
	for _, rejectAt := range []int{1, 2} {
		loc := r71GameLocation(t)
		calls := 0
		err := writeRecommendedItemSet(loc, r71Recommendation("DL"), func() error {
			calls++
			if calls == rejectAt {
				return errors.New("phase changed")
			}
			return nil
		})
		if err == nil || calls != rejectAt {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
		if _, err := os.Stat(filepath.Join(loc.configRoot, "Global", "Recommended", recommendedItemSetUID+".json")); !os.IsNotExist(err) {
			t.Fatal("written after guard rejection")
		}
	}
}

func TestR71RecommendedMigrationPreservesForeignItemSets(t *testing.T) {
	for _, scenario := range []string{"ok", "missing-account", "different-account", "guard", "malformed", "put-failed", "verify-failed", "preflight-account", "preflight-edit", "verify-account"} {
		t.Run(scenario, func(t *testing.T) {
			account := any(float64(12))
			if scenario == "different-account" {
				account = float64(13)
			}
			kept := map[string]any{"uid": "user-set", "title": "My build", "blocks": []any{}, "extension": map[string]any{"x": true}}
			document := map[string]any{"accountId": account, "timestamp": float64(123), "extension": "keep", "itemSets": []any{map[string]any{"uid": "deep-legends-v1-13-middle"}, kept, map[string]any{"uid": "deep-legends-v1-imported", "startedFrom": "deep-legends"}, map[string]any{"uid": "deep-legends-v1-103-middle"}}}
			if scenario == "missing-account" {
				delete(document, "accountId")
			}
			if scenario == "malformed" {
				document["itemSets"] = []any{nil}
			}
			puts, gets := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					gets++
					if gets == 2 && scenario == "preflight-account" || puts > 0 && scenario == "verify-account" {
						document["accountId"] = float64(13)
					}
					if gets == 2 && scenario == "preflight-edit" {
						document["itemSets"] = append(document["itemSets"].([]any), map[string]any{"uid": "user-added-while-applying"})
					}
				}
				if r.Method == http.MethodPut {
					puts++
					if scenario == "put-failed" {
						http.Error(w, "failed", 500)
						return
					}
					if err := json.NewDecoder(r.Body).Decode(&document); err != nil {
						t.Error(err)
					}
				}
				if scenario == "verify-failed" && puts > 0 && r.Method == http.MethodGet {
					http.Error(w, "failed", 500)
					return
				}
				_ = json.NewEncoder(w).Encode(document)
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
			removed, err := removeLegacyItemSets(context.Background(), client, Summoner{SummonerID: 7, AccountID: 12}, 13, func() error {
				if scenario == "guard" {
					return errors.New("changed")
				}
				return nil
			})
			if scenario == "ok" {
				if err != nil || removed != 1 || puts != 1 {
					t.Fatalf("removed=%d puts=%d err=%v", removed, puts, err)
				}
				if document["extension"] != "keep" || !reflect.DeepEqual(document["itemSets"].([]any)[0], kept) || len(document["itemSets"].([]any)) != 3 {
					t.Fatalf("foreign data changed: %v", document)
				}
			} else {
				if err == nil {
					t.Fatal("expected failure")
				}
				if scenario != "put-failed" && scenario != "verify-failed" && scenario != "verify-account" && puts != 0 {
					t.Fatal("write before validation")
				}
			}
		})
	}
}

func TestR71RecommendedTencentPathAndPartialCleanup(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Game")
	clientDir := filepath.Join(root, "LeagueClient")
	for _, dir := range []string{game, clientDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(game, "League of Legends.exe"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/data-store/v1/install-dir" {
			_ = json.NewEncoder(w).Encode(clientDir)
			return
		}
		http.Error(w, "cleanup unavailable", 503)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client(), region: "TENCENT"}
	uid, _, stats, err := applyRecommendedItemSet(context.Background(), client, Summoner{SummonerID: 7, AccountID: 12}, gameplayItemSetApplyRequest{ChampionID: 13, Position: "middle"}, nil, nil)
	if err != nil || uid != recommendedItemSetUID || !stats.LegacyCleanupFailed || stats.Storage != "recommended-file" {
		t.Fatalf("%s %+v %v", uid, stats, err)
	}
	if _, err := os.Stat(filepath.Join(game, "Config", "Global", "Recommended", recommendedItemSetUID+".json")); err != nil {
		t.Fatal(err)
	}
}

func TestR71RecommendedInternationalConfigAndExecutableRoots(t *testing.T) {
	for _, linkedGame := range []bool{false, true} {
		t.Run(fmt.Sprintf("linked-game-%t", linkedGame), func(t *testing.T) {
			root := t.TempDir()
			game := filepath.Join(root, "Game")
			if linkedGame {
				target := t.TempDir()
				if err := os.WriteFile(filepath.Join(target, "League of Legends.exe"), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, game); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			} else {
				if err := os.Mkdir(game, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(game, "League of Legends.exe"), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/data-store/v1/install-dir" {
					_ = json.NewEncoder(w).Encode(root)
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client(), region: "NA"}
			location, err := locateGameSettings(context.Background(), client)
			if err != nil {
				t.Fatal(err)
			}
			err = writeRecommendedItemSet(location, r71Recommendation("DL"), nil)
			if linkedGame {
				if err == nil {
					t.Fatal("external executable link accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(root, "Config", "Global", "Recommended", recommendedItemSetUID+".json")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(game, "Config")); !os.IsNotExist(err) {
				t.Fatal("international Config incorrectly written under Game")
			}
		})
	}
}

func TestR71LiveRecentPositionsUseAtMostTenActualRankedSamples(t *testing.T) {
	match := func(queue int64, pos string) gameplayMatch {
		return gameplayMatch{QueueID: queue, SubjectParticipantID: 1, Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: "self", Position: pos}}}
	}
	matches := []gameplayMatch{match(420, "middle"), match(420, "top"), match(440, "utility"), match(420, "unknown")}
	got := liveRecentPositions(matches, "self", 420)
	if len(got) != 2 || got[0].Position != "top" || got[0].Games != 1 || got[1].Position != "middle" {
		t.Fatalf("positions %+v", got)
	}
	if got := liveRecentPositions(matches, "self", 440); len(got) != 1 || got[0].Position != "utility" {
		t.Fatal(got)
	}
	for len(matches) < 10 {
		matches = append(matches, match(420, "unknown"))
	}
	matches = append(matches, match(420, "jungle"))
	if len(liveRecentPositions(matches, "self", 420)) != 2 {
		t.Fatal("read beyond ten-game window")
	}
	for _, queue := range []int64{400, 450, 1700} {
		if len(liveRecentPositions(matches, "self", queue)) != 0 {
			t.Fatal("non-ranked roles shown")
		}
	}
	empty := gameplayLivePlayer{RecentPositions: liveRecentPositions([]gameplayMatch{match(420, "unknown")}, "self", 420)}
	data, _ := json.Marshal(empty)
	if strings.Contains(string(data), "recentPositions") {
		t.Fatal("empty sample must be omitted")
	}
}

func TestR71CollectionRequestsCoalesceAcrossQueueProbeAndScan(t *testing.T) {
	a := &app{refreshRequests: make(chan struct{}, 1), summoner: Summoner{SummonerID: 1}}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); a.requestCollectionRefresh() }()
	}
	wg.Wait()
	if len(a.refreshRequests) != 1 {
		t.Fatal("duplicate queue entries")
	}
	<-a.refreshRequests
	probeEntered, releaseProbe, scanEntered, releaseScan := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	a.collectionProbe = func(context.Context, *LCUClient, int64) (bool, int) {
		close(probeEntered)
		<-releaseProbe
		return true, 1
	}
	a.collectionRefresh = func(*LCUClient) bool { close(scanEntered); <-releaseScan; return true }
	done := make(chan struct{})
	go func() { a.refreshCollectionWithClient(nil); close(done) }()
	<-probeEntered
	a.requestCollectionRefresh()
	if len(a.refreshRequests) != 0 {
		t.Fatal("probe duplicated request")
	}
	close(releaseProbe)
	<-scanEntered
	a.requestCollectionRefresh()
	if len(a.refreshRequests) != 0 {
		t.Fatal("scan duplicated request")
	}
	close(releaseScan)
	<-done
	a.syncing = true // An account-only sync must not swallow a collection request.
	a.requestCollectionRefresh()
	if len(a.refreshRequests) != 1 {
		t.Fatal("completed scan/account sync stranded retry")
	}
}

func TestR71ClaimIdentityQuantityAndDiagnostics(t *testing.T) {
	one := RewardGrant{ID: "grant-a", RewardGroupID: "group-a", Status: "PENDING", Title: "待领取奖励", Items: []RewardItem{{ID: "r1", ItemID: "currency", ItemType: "CURRENCY", Quantity: 25}}}
	two := one
	two.ID = "grant-b"
	unique := uniqueRewardGrants([]RewardGrant{one, one, two})
	if len(unique) != 2 || unique[0].ValidationError != "" {
		t.Fatal(unique)
	}
	changed := one
	changed.Title = "different"
	if got := uniqueRewardGrants([]RewardGrant{one, changed, one}); len(got) != 1 || got[0].ValidationError == "" {
		t.Fatal("conflicting ID not blocked")
	}
	values := []any{map[string]any{"id": "r", "itemId": "currency", "quantity": float64(1)}, map[string]any{"id": "r", "itemId": "currency", "quantity": float64(5)}}
	if got := collectClaimRewardItems(values); len(got) != 2 {
		t.Fatalf("different quantities collapsed: %+v", got)
	}
	first := claimEntry{Source: "grant", ID: "private-grant", Title: "private-title", Items: one.Items, Actionable: true}
	second := first
	second.Items = []RewardItem{{ID: "r1", ItemID: "currency", ItemType: "CURRENCY", Quantity: 50}}
	if claimSignature(first) == claimSignature(second) {
		t.Fatal("overlap ignored quantity")
	}
	shape := claimScanShape(claimScanResponse{Items: []claimEntry{first, second}})
	data, _ := json.Marshal(shape)
	for _, private := range []string{"private-grant", "private-title", "currency\"", "r1"} {
		if strings.Contains(string(data), private) {
			t.Fatalf("diagnostic leaked %s", private)
		}
	}
	if shape["entry_count"] != 2 || shape["reward_item_rows"] != 2 || shape["quantity_by_type"].(map[string]int)["CURRENCY"] != 75 {
		t.Fatal(shape)
	}
}

func TestR71ClaimScanKeepsConflictsNonActionableAndResolvesExactQuantities(t *testing.T) {
	for _, scenario := range []string{"duplicate-conflict", "quantity-fallback", "quantity-conflict", "foreign-element", "different-item", "different-type"} {
		t.Run(scenario, func(t *testing.T) {
			element := map[string]any{"elementId": "r1", "itemId": "orange", "itemType": "CURRENCY", "quantity": 25}
			quantity := 0
			if scenario == "quantity-conflict" {
				quantity = 50
			}
			if scenario == "foreign-element" {
				element["elementId"] = "r2"
			}
			if scenario == "different-item" {
				element["itemId"] = "blue"
			}
			if scenario == "different-type" {
				element["itemType"] = "MATERIAL"
			}
			grant := map[string]any{
				"info":        map[string]any{"id": "grant-a", "status": "PENDING_SELECTION", "grantElements": []any{element}},
				"rewardGroup": map[string]any{"id": "group-a", "rewards": []any{map[string]any{"id": "r1", "itemId": "orange", "itemType": "CURRENCY", "quantity": quantity}}},
			}
			grants := []any{grant}
			if scenario == "duplicate-conflict" {
				grants = append(grants, map[string]any{"info": map[string]any{"id": "grant-a", "status": "PENDING_SELECTION"}, "rewardGroup": map[string]any{"id": "group-a", "rewards": []any{map[string]any{"id": "r1", "itemId": "orange", "itemType": "CURRENCY", "quantity": 50}}}})
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("scan must never write")
					http.Error(w, "blocked", 405)
					return
				}
				if r.URL.Path == "/lol-rewards/v1/grants" {
					_ = json.NewEncoder(w).Encode(grants)
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
			result := scanClaims(context.Background(), client)
			if len(result.Items) != 1 || len(result.Items[0].Items) != 1 {
				t.Fatalf("unexpected scan: %+v", result)
			}
			entry := result.Items[0]
			conflicting := scenario == "duplicate-conflict" || scenario == "quantity-conflict"
			if entry.Actionable == conflicting || (conflicting && entry.Detail == "") {
				t.Fatalf("unsafe actionable state: %+v", entry)
			}
			wantQuantity := 25
			if scenario == "quantity-conflict" {
				wantQuantity = 50
			}
			if scenario == "foreign-element" || scenario == "different-item" || scenario == "different-type" {
				wantQuantity = 0
			}
			if entry.Items[0].Quantity != wantQuantity {
				t.Fatalf("quantity=%d want=%d", entry.Items[0].Quantity, wantQuantity)
			}
			if conflicting && claimScanShape(result)["actionable_entries"] != 0 {
				t.Fatal("conflict counted as actionable")
			}
		})
	}
}

func TestR71ItemSetDocumentComparisonPreservesIntegerPrecision(t *testing.T) {
	decode := func(raw string) lcuItemSetDocument {
		t.Helper()
		var doc lcuItemSetDocument
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	original := decode(`{"accountId":9007199254740992,"itemSets":[{"uid":"own","x":true}]}`)
	reordered := decode(`{ "itemSets": [ {"x":true, "uid":"own"} ], "accountId":9007199254740992 }`)
	if !sameItemSetDocument(original, reordered) {
		t.Fatal("formatting is not a concurrent edit")
	}
	for _, changed := range []string{
		`{"accountId":9007199254740993,"itemSets":[{"uid":"own","x":true}]}`,
		`{"accountId":9007199254740992,"itemSets":[{"uid":"other","x":true}]}`,
		`{"accountId":9007199254740992,"itemSets":[{"uid":"own","x":true}],"extension":"changed"}`,
	} {
		if sameItemSetDocument(original, decode(changed)) {
			t.Fatalf("missed concurrent change: %s", changed)
		}
	}
}
