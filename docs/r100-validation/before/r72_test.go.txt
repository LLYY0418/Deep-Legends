package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR72RecommendedTraceSeparatesWriteAndReadback(t *testing.T) {
	for _, title := range []string{"DL · 中路", "invalid\xff"} {
		t.Run(title, func(t *testing.T) {
			location := r71GameLocation(t)
			trace := &itemSetWriteTrace{TraceID: "apply-r72-trace"}
			err := writeRecommendedItemSetTraced(location, r71Recommendation(title), nil, trace)
			if !trace.FileWritten || trace.Bytes <= 0 || len(trace.SHA256) != 64 {
				t.Fatalf("missing write evidence: %+v %v", trace, err)
			}
			if trace.TraceID != "apply-r72-trace" || len(trace.ExpectedBlocks) == 0 || trace.ExpectedOrderDigest == "" {
				t.Fatalf("missing expected trace context: %+v", trace)
			}
			if strings.Contains(title, "invalid") {
				if err == nil || trace.ReadbackVerified || itemSetFailureStage(err, "") != "file-readback" || len(trace.ReadbackBlocks) == 0 || trace.ReadbackOrderDigest == "" {
					t.Fatalf("readback mismatch not identified: %+v %v", trace, err)
				}
				return
			}
			if err != nil || !trace.ReadbackVerified || trace.Stage != "verified" || len(trace.ReadbackBlocks) == 0 || trace.ExpectedOrderDigest != trace.ReadbackOrderDigest || !trace.ReadbackOrderMatch {
				t.Fatalf("successful trace: %+v %v", trace, err)
			}
			snapshot := readRecommendationDiagnostic(location)
			if snapshot["sha256"] != trace.SHA256 || snapshot["global_any_schema"] != true {
				t.Fatalf("export cannot correlate with write: %v %+v", snapshot, trace)
			}
			if err := writeRecommendedItemSetTraced(location, r71Recommendation("DL · 上路"), nil, trace); err != nil || !trace.ReplacedPrevious {
				t.Fatalf("replacement trace: %+v %v", trace, err)
			}
			if readRecommendationDiagnostic(location)["sha256"] == snapshot["sha256"] {
				t.Fatal("changed file must have changed fingerprint")
			}
			err = writeRecommendedItemSetTraced(location, r71Recommendation("third"), func() error { return context.Canceled }, trace)
			if err == nil || trace.TraceID != "apply-r72-trace" || trace.FileWritten || trace.ReadbackVerified || trace.SHA256 != "" || trace.Bytes != 0 || trace.ReplacedPrevious || trace.FailureCode != "canceled" {
				t.Fatalf("reused trace retained earlier success: %+v %v", trace, err)
			}
		})
	}
}

func TestR72RecommendedFailureStages(t *testing.T) {
	for _, stage := range []string{"initial-session-guard", "recommended-directory", "previous-file-ownership", "backup-ownership", "replace-session-guard", "backup-write", "file-write"} {
		t.Run(stage, func(t *testing.T) {
			location := r71GameLocation(t)
			file := filepath.Join(location.configRoot, "Global", "Recommended", recommendedItemSetUID+".json")
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			if stage != "recommended-directory" {
				_, err := recommendedDirectory(location)
				must(err)
			}
			switch stage {
			case "recommended-directory":
				location.allowedRoot = "relative"
			case "previous-file-ownership":
				must(os.WriteFile(file, []byte(`{"uid":"foreign"}`), 0600))
			case "backup-ownership":
				must(os.WriteFile(file+".bak", []byte(`{"uid":"foreign"}`), 0600))
			case "backup-write":
				must(writeRecommendedItemSet(location, r71Recommendation("first"), nil))
			}
			calls := 0
			guard := func() error {
				calls++
				if stage == "initial-session-guard" || (stage == "replace-session-guard" && calls == 2) {
					return context.Canceled
				}
				if calls == 2 {
					if stage == "file-write" {
						must(os.Mkdir(file, 0700))
					}
					if stage == "backup-write" {
						must(os.Mkdir(file+".bak", 0700))
					}
				}
				return nil
			}
			trace := &itemSetWriteTrace{}
			err := writeRecommendedItemSetTraced(location, r71Recommendation("next"), guard, trace)
			if err == nil || itemSetFailureStage(err, "unknown") != stage || trace.FileWritten || trace.ReadbackVerified {
				t.Fatalf("stage=%s trace=%+v error=%v", stage, trace, err)
			}
			if strings.Contains(stage, "session-guard") && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation cause")
			}
		})
	}
}

func TestR72DisplayConfigIsNumericAllowlistedAndBounded(t *testing.T) {
	location := r71GameLocation(t)
	if err := os.MkdirAll(location.configRoot, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(location.configRoot, "game.cfg")
	original := []byte("[General]\nWidth=1920\nHeight=1080\nWindowMode=2\nAccount=private-account\n[HUD]\nGlobalScale=0.85\nItemShopResizeWidth=1300\nItemShopResizeHeight=900\nShopScale=NaN\nSecret=private-token\n")
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	out := readDisplayConfigDiagnostic(location)
	values := out["values"].(map[string]float64)
	if out["result"] != "ok" || values["General.Width"] != 1920 || values["HUD.ItemShopResizeWidth"] != 1300 || out["invalid_or_duplicate_fields"] != 1 {
		t.Fatalf("snapshot=%v", out)
	}
	encoded, _ := json.Marshal(out)
	for _, secret := range []string{"private", location.allowedRoot, "Secret", "Account", "NaN"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("leaked %q: %s", secret, encoded)
		}
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(original) {
		t.Fatal("read-only diagnostic changed game settings")
	}
	values, invalid := parseItemSetDisplayConfig([]byte("[General]\nWidth=1920\nWidth=2560\nWidth=1000\nHeight=Inf\nWindowMode=999999999\n"))
	if len(values) != 0 || invalid != 4 {
		t.Fatalf("ambiguous values=%v invalid=%d", values, invalid)
	}
	if err := os.WriteFile(file, make([]byte, (256<<10)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if readDisplayConfigDiagnostic(location)["result"] != "too-large" {
		t.Fatal("unbounded configuration read")
	}
}

func TestR72ExportSnapshotMissingUnsafeAndForeignFiles(t *testing.T) {
	location := r71GameLocation(t)
	if out := readRecommendationDiagnostic(location); out["result"] != "missing" {
		t.Fatal(out)
	}
	if _, err := os.Stat(location.configRoot); !os.IsNotExist(err) {
		t.Fatal("export created Config")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "game.cfg"), []byte("[General]\nWidth=9999"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, location.configRoot); err != nil {
		t.Fatal(err)
	}
	if readDisplayConfigDiagnostic(location)["result"] != "unsafe-or-unreadable" {
		t.Fatal("followed external config link")
	}
	location = r71GameLocation(t)
	dir, err := recommendedDirectory(location)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, recommendedItemSetUID+".json"), []byte(`{"uid":"foreign","title":"secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	out := readRecommendationDiagnostic(location)
	if out["result"] != "invalid-or-foreign" || out["sha256"] != nil {
		t.Fatalf("foreign fingerprint: %v", out)
	}
}

func TestR72ExportIncludesReadOnlyCheckpointWithoutSecrets(t *testing.T) {
	location := r71GameLocation(t)
	trace := &itemSetWriteTrace{}
	if err := writeRecommendedItemSetTraced(location, r71Recommendation("private-title"), nil, trace); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("export attempted mutation")
			w.WriteHeader(500)
			return
		}
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		case "/data-store/v1/install-dir":
			_ = json.NewEncoder(w).Encode(location.installRoot)
		case "/lol-item-sets/v1/item-sets/123/sets":
			_, _ = w.Write([]byte(`{"accountId":456,"itemSets":[{"uid":"deep-legends-v1-13-middle","title":"private-title"},{"uid":"user-secret"}]}`))
		default:
			http.Error(w, "private-upstream-body", 503)
		}
	}))
	defer server.Close()
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{connected: true, lcu: &LCUClient{baseURL: server.URL, http: server.Client(), token: "private-token"}, summoner: Summoner{SummonerID: 123, AccountID: 456, PUUID: "private-player"}, storage: store}
	recorder := httptest.NewRecorder()
	a.handleDiagnosticLog(recorder, httptest.NewRequest(http.MethodGet, "/api/diagnostics/log", nil))
	if recorder.Code != 200 {
		t.Fatalf("export %d %s", recorder.Code, recorder.Body.String())
	}
	var event map[string]any
	objectiveProbeFound := false
	for _, line := range strings.Split(strings.TrimSpace(recorder.Body.String()), "\n") {
		var current map[string]any
		if err := json.Unmarshal([]byte(line), &current); err != nil {
			t.Fatal(err, line)
		}
		if current["event"] == "item_set_export_snapshot" {
			event = current
		}
		if current["event"] == "objective_badge_probe" {
			objectiveProbeFound = true
		}
	}
	if !objectiveProbeFound {
		t.Fatal("export omitted objective checkpoint")
	}
	if event["event"] != "item_set_export_snapshot" || event["phase"] != "InProgress" || event["game_render_geometry"] != "unobservable" {
		t.Fatal(event)
	}
	if event["recommendation"].(map[string]any)["sha256"] != trace.SHA256 || event["legacy"].(map[string]any)["managed_legacy_count"] != float64(1) {
		t.Fatal(event)
	}
	for _, secret := range []string{"private-", "user-secret", location.installRoot, "accountId", "summonerId"} {
		if strings.Contains(recorder.Body.String(), secret) {
			t.Fatalf("leaked %q", secret)
		}
	}
}

func TestR72CleanupFailureStageDoesNotLeakRawError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private-token/account/path", 503) }))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, http: server.Client()}
	_, err := removeLegacyItemSets(context.Background(), client, Summoner{SummonerID: 7, AccountID: 12}, 13, nil)
	if itemSetFailureStage(err, "unknown") != "legacy-read" {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(&itemSetWriteTrace{CleanupStage: itemSetFailureStage(err, "unknown")})
	if strings.Contains(string(encoded), "private") {
		t.Fatal(string(encoded))
	}
}

func TestR72FailureCodesOnlyExposeEnums(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timeout"},
		{&LCUHTTPError{StatusCode: 503, Path: "/account/123", Message: "secret"}, "lcu-http-503"},
		{errors.New("secret-token /private/account/file"), "operation-failed"},
		{errors.New("未找到游戏程序，已停止写入推荐方案"), "game-executable-missing"},
		{errors.New("装备方案已变化，已保留现有方案"), "concurrent-document-change"},
	} {
		if got := itemSetFailureCode(tc.err); got != tc.want {
			t.Fatalf("code=%s want=%s", got, tc.want)
		}
	}
}

func TestR72PersistedShopSnapshotUsesOnlyKnownNumericFields(t *testing.T) {
	location := r71GameLocation(t)
	if err := os.MkdirAll(location.configRoot, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(location.configRoot, "PersistedSettings.json")
	fixture := `{"description":"private-description","files":[{"name":"Game.cfg","sections":[{"name":"HUD","settings":[{"name":"ItemShopPrevX","value":"-130"},{"name":"ItemShopPrevY","value":"20"},{"name":"ItemShopResizeWidth","value":1300},{"name":"ItemShopResizeHeight","value":"900"},{"name":"GlobalScale","value":"0.8"},{"name":"Account","value":"private-account"},{"name":"ShopScale","value":"private-token"}]}]},{"name":"Input.ini","sections":[{"name":"HUD","settings":[{"name":"ItemShopPrevX","value":"9999"}]}]}]}`
	if err := os.WriteFile(file, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	out := readDisplayConfigDiagnostic(location)
	if out["result"] != "missing" {
		t.Fatal(out)
	}
	persisted := out["persisted"].(map[string]any)
	values := persisted["values"].(map[string]float64)
	if persisted["result"] != "ok" || values["HUD.ItemShopPrevX"] != -130 || values["HUD.ItemShopPrevY"] != 20 || values["HUD.ItemShopResizeWidth"] != 1300 || persisted["invalid_or_duplicate_fields"] != 1 {
		t.Fatal(persisted)
	}
	encoded, err := json.Marshal(out)
	if err != nil || strings.Contains(string(encoded), "private-") || strings.Contains(string(encoded), "9999") {
		t.Fatal(string(encoded), err)
	}
	after, _ := os.ReadFile(file)
	if string(after) != fixture {
		t.Fatal("persisted settings changed by diagnostic")
	}
	for _, body := range []string{`not-json`, `{}`, `null`} {
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if readPersistedDisplayDiagnostic(location)["result"] != "invalid-schema" {
			t.Fatal("accepted unknown persisted structure")
		}
	}
}

func TestR72PersistedSnapshotDistinguishesMissingGameConfig(t *testing.T) {
	location := r71GameLocation(t)
	if err := os.MkdirAll(location.configRoot, 0700); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"files":[]}`, `{"files":[{"name":"Input.ini"}]}`} {
		if err := os.WriteFile(filepath.Join(location.configRoot, "PersistedSettings.json"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if got := readPersistedDisplayDiagnostic(location); got["result"] != "game-config-missing" {
			t.Fatal(got)
		}
	}
	values, invalid := parseItemSetDisplayConfig([]byte("[HUD]\nGlobalScale=1\nGlobalScale=2\nGlobalScale=3\n"))
	if _, exists := values["HUD.GlobalScale"]; exists || invalid != 2 {
		t.Fatalf("duplicate field must remain excluded: %v invalid=%d", values, invalid)
	}
}

func TestR72ApplyLogsVerifiedPartialAndFailedStates(t *testing.T) {
	for _, mode := range []string{"verified", "cleanup-failed", "locate-failed"} {
		t.Run(mode, func(t *testing.T) {
			location := r71GameLocation(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/lol-gameflow/v1/gameflow-phase":
					_, _ = w.Write([]byte(`"ChampSelect"`))
				case "/lol-champ-select/v1/session":
					_, _ = w.Write([]byte(`{"myTeam":[{"summonerId":123,"championId":13}]}`))
				case "/lol-game-data/assets/v1/items.json":
					_, _ = w.Write([]byte(`[{"id":1001,"priceTotal":300}]`))
				case "/data-store/v1/install-dir":
					if mode == "locate-failed" {
						http.Error(w, "private-install-path", 503)
						return
					}
					_ = json.NewEncoder(w).Encode(location.installRoot)
				case "/lol-item-sets/v1/item-sets/123/sets":
					if mode == "cleanup-failed" {
						http.Error(w, "private-account-body", 503)
						return
					}
					_, _ = w.Write([]byte(`{"accountId":456,"itemSets":[]}`))
				default:
					http.Error(w, "unexpected", 404)
				}
			}))
			defer server.Close()
			store := trackTestStore(t, &localStore{root: t.TempDir()})
			if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0700); err != nil {
				t.Fatal(err)
			}
			a := &app{connected: true, lcu: &LCUClient{baseURL: server.URL, http: server.Client(), token: "private-token"}, summoner: Summoner{SummonerID: 123, AccountID: 456}, storage: store}
			body := `{"championId":13,"position":"middle","mapId":11,"storage":"recommended","blocks":[{"type":"核心装","items":[{"id":1001,"count":1}]}]}`
			recorder := httptest.NewRecorder()
			a.handleGameplayItemSetApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/item-sets/apply", strings.NewReader(body)))
			wantStatus := http.StatusOK
			if mode == "locate-failed" {
				wantStatus = http.StatusConflict
			}
			if recorder.Code != wantStatus {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			data, err := store.readDiagnosticLog()
			if err != nil {
				t.Fatal(err)
			}
			var apply map[string]any
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var event map[string]any
				if json.Unmarshal([]byte(line), &event) == nil && event["event"] == "item_set_apply" {
					apply = event
				}
			}
			if apply == nil || apply["game_render_geometry"] != "unobservable" || apply["game_loaded_recommendation"] != "unknown" {
				t.Fatalf("missing observability boundary: %s", data)
			}
			trace := apply["write_trace"].(map[string]any)
			switch mode {
			case "verified":
				if apply["stored"] != true || apply["apply_stage"] != "completed" || trace["readback_verified"] != true || trace["cleanup_stage"] != "verified" {
					t.Fatal(apply)
				}
			case "cleanup-failed":
				if apply["stored"] != true || apply["apply_stage"] != "completed-with-legacy-warning" || trace["readback_verified"] != true || trace["cleanup_stage"] != "legacy-read" || trace["cleanup_failure_code"] != "lcu-http-503" {
					t.Fatal(apply)
				}
			case "locate-failed":
				if apply["stored"] != false || apply["apply_stage"] != "locate-installation" || trace["file_written"] != false || trace["failure_code"] != "locate-failed" {
					t.Fatal(apply)
				}
			}
			for _, secret := range []string{"private-", location.allowedRoot} {
				if strings.Contains(string(data), secret) {
					t.Fatalf("leaked %q", secret)
				}
			}
		})
	}
}
