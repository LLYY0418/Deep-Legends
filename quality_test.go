package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func inventoryTestClient(t *testing.T, handler http.Handler) *LCUClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
}

func TestEmptyOwnedInventoryIsValidWhenTwoSourcesAgree(t *testing.T) {
	client := inventoryTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "skins-minimal") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":1001,"owned":false}]`))
			return
		}
		if r.URL.Path == "/lol-inventory/v2/inventory/CHAMPION_SKIN" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
			return
		}
		http.NotFound(w, r)
	}))
	ids, statuses, err := loadOwnedSkinInventory(client, 42, []Skin{{ID: 1001, Name: "测试皮肤", ChampionID: 1}})
	if err != nil || len(ids) != 0 {
		t.Fatalf("empty permanent inventory should be valid: ids=%#v statuses=%#v err=%v", ids, statuses, err)
	}
	if countSourceState(statuses, "success") != 2 {
		t.Fatalf("success sources = %#v", statuses)
	}
}

func TestEmptyAndNonEmptyOwnedSourcesDisagree(t *testing.T) {
	client := inventoryTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "skins-minimal"):
			_, _ = w.Write([]byte(`[{"id":1001,"owned":false}]`))
		case r.URL.Path == "/lol-inventory/v2/inventory/CHAMPION_SKIN":
			_, _ = w.Write([]byte(`[{"itemId":1001,"inventoryType":"CHAMPION_SKIN"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	ids, statuses, err := loadOwnedSkinInventory(client, 42, []Skin{{ID: 1001, Name: "测试皮肤", ChampionID: 1}})
	if err != nil || len(ids) != 0 || countSourceState(statuses, "warning") != 1 {
		t.Fatalf("explicit full-coverage source should win over presence-only disagreement: ids=%#v statuses=%#v err=%v", ids, statuses, err)
	}
}

func TestOneExplicitFullCoverageSourceIsSufficient(t *testing.T) {
	client := inventoryTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "skins-minimal") {
			_, _ = w.Write([]byte(`[{"id":1001,"owned":true}]`))
			return
		}
		http.NotFound(w, r)
	}))
	ids, _, err := loadOwnedSkinInventory(client, 42, []Skin{{ID: 1001, Name: "测试皮肤", ChampionID: 1}})
	if err != nil || !ids[1001] || len(ids) != 1 {
		t.Fatalf("full-coverage explicit source should be accepted: ids=%#v err=%v", ids, err)
	}
}

func TestTransientOwnedSourceFailureStopsCalculation(t *testing.T) {
	client := inventoryTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "skins-minimal"):
			_, _ = w.Write([]byte(`[{"id":1001,"owned":true}]`))
		case r.URL.Path == "/lol-inventory/v2/inventory/CHAMPION_SKIN":
			_, _ = w.Write([]byte(`[{"itemId":1001,"inventoryType":"CHAMPION_SKIN"}]`))
		case strings.Contains(r.URL.Path, "/champions"):
			http.Error(w, "temporary", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	ids, statuses, err := loadOwnedSkinInventory(client, 42, []Skin{{ID: 1001, Name: "测试皮肤", ChampionID: 1}})
	if err != nil || !ids[1001] || len(ids) != 1 || countSourceState(statuses, "failed") != 1 {
		t.Fatalf("a redundant source failure must not veto verified evidence: ids=%#v statuses=%#v err=%v", ids, statuses, err)
	}
}

func countSourceState(statuses []OwnershipSourceStatus, state string) int {
	count := 0
	for _, status := range statuses {
		if status.State == state {
			count++
		}
	}
	return count
}

func TestExplicitOwnershipDoesNotWalkUnknownNestedObjects(t *testing.T) {
	fixture := map[string]any{
		"metadata": map[string]any{"id": float64(1001), "owned": true},
		"skins":    []any{map[string]any{"id": float64(1002), "owned": true}},
	}
	ids := extractOwnedIDs(fixture, false)
	if ids[1001] || !ids[1002] || len(ids) != 1 {
		t.Fatalf("only schema-bound skins container should be traversed: %#v", ids)
	}
}

func TestFailedRefreshStateCannotServeOldSnapshot(t *testing.T) {
	a := &app{
		connected: true, summoner: Summoner{SummonerID: 1, PUUID: "secret"},
		allSkins: []Skin{{ID: 1001}}, owned: []Skin{{ID: 1001}}, remaining: []Skin{{ID: 2001}},
		poolTotal: 2, poolMatched: 2, poolIssues: []PoolIssue{{Name: "old"}}, lcu: &LCUClient{},
		ownership: []OwnershipSourceStatus{{State: "success"}}, catalog: CatalogStats{SkinCount: 1000},
	}
	a.clearSnapshotLocked("disconnected")
	if a.connected || a.summoner.SummonerID != 0 || a.lcu != nil || len(a.allSkins)+len(a.owned)+len(a.remaining) != 0 || a.poolMatched != 0 || len(a.poolIssues) != 0 {
		t.Fatalf("stale state remained after invalidation: %#v", a)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/skins?view=owned", nil)
	recorder := httptest.NewRecorder()
	a.handleSkins(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale skins endpoint status = %d", recorder.Code)
	}
}

func TestChromaEndpointKeepsCatalogAndOwnershipSeparate(t *testing.T) {
	a := &app{
		connected: true, snapshotReady: true,
		chromas: []Chroma{
			{ID: 60_103_001, Name: "臻彩测试", ChampionID: 103, IsPrestige: true, Owned: true},
			{ID: 1_030_011, Name: "普通炫彩", ChampionID: 103, ParentSkinID: 103001},
		},
		chromaState: EndpointCapability{Name: "owned-chromas", State: capabilityAvailable, Count: 1},
	}
	request := httptest.NewRequest(http.MethodGet, "/api/chromas", nil)
	recorder := httptest.NewRecorder()
	a.handleChromas(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("chroma endpoint status = %d", recorder.Code)
	}
	var payload struct {
		Count      int      `json:"count"`
		OwnedCount int      `json:"ownedCount"`
		Items      []Chroma `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 2 || payload.OwnedCount != 1 || len(payload.Items) != 2 || !payload.Items[0].IsPrestige {
		t.Fatalf("unexpected chroma response: %#v", payload)
	}
}

func TestSnapshotValidationErrorRetainsHealthyClientButNotSnapshot(t *testing.T) {
	client := &LCUClient{token: "temporary"}
	a := &app{poolTotal: 554, poolMatched: 554, allSkins: []Skin{{ID: 9999}}, owned: []Skin{{ID: 9999}}, snapshotReady: true}
	a.retainClientAfterSnapshotErrorLocked(client, Snapshot{
		Summoner:  Summoner{SummonerID: 7, GameName: "玩家"},
		Ownership: []OwnershipSourceStatus{{Path: "/skins", State: "conflict", Count: 3}},
		Catalog:   CatalogStats{SkinCount: 1700, ChampionCount: 170},
	}, "库存证据冲突")
	if !a.connected || a.snapshotReady || a.lcu != client || a.summoner.GameName != "玩家" || len(a.ownership) != 1 || a.catalog.SkinCount != 1700 {
		t.Fatalf("healthy client or diagnostics were not retained: %#v", a)
	}
	if len(a.allSkins)+len(a.owned)+len(a.remaining) != 0 || a.calculationOKLocked() {
		t.Fatalf("invalid snapshot remained visible: %#v", a)
	}
}

func TestReadLimitedRejectsOversizedBody(t *testing.T) {
	if data, err := readLimited(bytes.NewReader([]byte("1234")), 4); err != nil || string(data) != "1234" {
		t.Fatalf("exact limit failed: %q %v", data, err)
	}
	if _, err := readLimited(bytes.NewReader([]byte("12345")), 4); err == nil {
		t.Fatal("oversized body must fail explicitly")
	}
}

func TestStatusResponseDoesNotExposeStableAccountIdentifiers(t *testing.T) {
	a := &app{connected: true, poolTotal: 1, poolMatched: 1, summoner: Summoner{SummonerID: 7, AccountID: 8, PUUID: "private-puuid", GameName: "玩家", TagLine: "CN1"}}
	recorder := httptest.NewRecorder()
	a.handleStatus(recorder, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	body := recorder.Body.String()
	for _, secret := range []string{"private-puuid", "summonerId", "accountId", "puuid"} {
		if strings.Contains(body, secret) {
			t.Fatalf("status leaked %q: %s", secret, body)
		}
	}
}

func TestPoolManifestValidationAndHashAreDeterministic(t *testing.T) {
	left, err := validatePoolManifest(PoolManifest{Name: "测试", Names: []string{"皮肤 A", "皮肤 B"}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := validatePoolManifest(PoolManifest{Name: "测试", Names: []string{"皮肤 B", "皮肤 A"}})
	if err != nil {
		t.Fatal(err)
	}
	if left.Hash != right.Hash {
		t.Fatalf("order-independent pool hashes differ: %s %s", left.Hash, right.Hash)
	}
	if _, err := validatePoolManifest(PoolManifest{Name: "重复", Names: []string{"海克斯科技·安妮", "海克斯科技 安妮"}}); err == nil {
		t.Fatal("normalized duplicate pool names must be rejected")
	}
	stableLeft, err := validatePoolManifest(PoolManifest{Name: "稳定", Entries: []PoolEntry{{ID: 1001, Name: "皮肤 A"}, {ID: 2001, Name: "皮肤 B"}}})
	if err != nil {
		t.Fatal(err)
	}
	stableRight, err := validatePoolManifest(PoolManifest{Name: "稳定", Entries: []PoolEntry{{ID: 2001, Name: "皮肤 B"}, {ID: 1001, Name: "皮肤 A"}}})
	if err != nil || stableLeft.Hash != stableRight.Hash || stableLeft.SchemaVersion != 2 {
		t.Fatalf("stable manifest validation failed: left=%#v right=%#v err=%v", stableLeft, stableRight, err)
	}
	if _, err := validatePoolManifest(PoolManifest{Name: "重复 ID", Entries: []PoolEntry{{ID: 1001, Name: "A"}, {ID: 1001, Name: "B"}}}); err == nil {
		t.Fatal("duplicate stable skin IDs must be rejected")
	}
}

func TestCSVFormulaInjectionIsNeutralized(t *testing.T) {
	for _, value := range []string{"=cmd", "+1", "-2", "@SUM(A1)"} {
		if got := safeCSVCell(value); !strings.HasPrefix(got, "'") {
			t.Fatalf("unsafe CSV value %q -> %q", value, got)
		}
	}
}

func TestSnapshotDiffRejectsDifferentAccountOrPool(t *testing.T) {
	base := SnapshotRecord{ID: "a", AccountHash: "account-a", PoolHash: "pool-a"}
	if _, err := diffSnapshots(base, SnapshotRecord{ID: "b", AccountHash: "account-a", PoolHash: "pool-b"}); err == nil {
		t.Fatal("different pools must not be compared")
	}
	if _, err := diffSnapshots(base, SnapshotRecord{ID: "b", AccountHash: "account-b", PoolHash: "pool-a"}); err == nil {
		t.Fatal("different accounts must not be compared")
	}
}

func TestSnapshotPersistenceIsRedacted(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"snapshots", "pools", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store := &localStore{root: root, salt: bytes.Repeat([]byte{1}, 32)}
	pool, _ := validatePoolManifest(PoolManifest{Name: "测试", Version: "1", Names: []string{"测试皮肤"}})
	snapshot := Snapshot{Summoner: Summoner{SummonerID: 7, PUUID: "private-puuid", GameName: "玩家"}, Owned: []Skin{{ID: 1001, Name: "测试皮肤"}}, Remaining: []Skin{}}
	record, err := store.saveSnapshot(snapshot, pool)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "snapshots", record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-puuid", "玩家", "summonerId", "accountId"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("snapshot leaked %q: %s", secret, data)
		}
	}
}

func TestSnapshotExportUsesSelectedHistoryRecord(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"snapshots", "pools", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store := &localStore{root: root, salt: bytes.Repeat([]byte{3}, 32)}
	pool, err := validatePoolManifest(PoolManifest{Name: "历史奖池", Version: "14.5", Names: []string{"已拥有皮肤", "剩余皮肤"}})
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.saveSnapshot(Snapshot{
		Summoner:  Summoner{PUUID: "private-puuid"},
		Owned:     []Skin{{ID: 1001, Name: "已拥有皮肤", ChampionName: "英雄甲", Owned: true}},
		Remaining: []Skin{{ID: 1002, Name: "剩余皮肤", ChampionName: "英雄乙"}},
	}, pool)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store}
	for _, format := range []string{"json", "csv", "html"} {
		t.Run(format, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/snapshots/record/export?format="+format, nil)
			request.SetPathValue("id", record.ID)
			recorder := httptest.NewRecorder()
			a.handleSnapshotExport(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			body := recorder.Body.String()
			for _, expected := range []string{"已拥有皮肤", "剩余皮肤"} {
				if !strings.Contains(body, expected) {
					t.Fatalf("%s export missing %q: %s", format, expected, body)
				}
			}
			if disposition := recorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, "."+format) {
				t.Fatalf("unexpected Content-Disposition: %q", disposition)
			}
		})
	}
}

func TestLockfileParsingValidatesProtocolAndClient(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid")
	if err := os.WriteFile(valid, []byte("LeagueClient:123:4567:token:https"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := clientFromLockfile(valid)
	if err != nil || client.port != 4567 {
		t.Fatalf("valid lockfile: client=%#v err=%v", client, err)
	}
	invalid := filepath.Join(dir, "invalid")
	if err := os.WriteFile(invalid, []byte("Other:123:4567:token:http"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := clientFromLockfile(invalid); err == nil {
		t.Fatal("foreign insecure lockfile must be rejected")
	}
	if _, ok := clientFromCommandLine(`C:\\Temp\\fake.exe --app-port=1234 --remoting-auth-token=x`); ok {
		t.Fatal("foreign command line must be rejected")
	}
}

func TestTencentClientCommandLineParsing(t *testing.T) {
	commandLine := `"D:\\WeGameApps\\英雄联盟\\LeagueClient\\LeagueClientUx.exe" --riotclient-tencent --region=TENCENT --rso_platform_id=HN1 --remoting-auth-token=private-token --app-port=63405 --install-directory="D:\\WeGameApps\\英雄联盟\\LeagueClient"`
	client, ok := clientFromCommandLine(commandLine)
	if !ok || client.port != 63405 {
		t.Fatalf("Tencent command line was not parsed: client=%#v ok=%v", client, ok)
	}
	client.Close()
	spaced := `C:\\LeagueClientUx.exe --app-port "54321" --remoting-auth-token "private-token"`
	client, ok = clientFromCommandLine(spaced)
	if !ok || client.port != 54321 {
		t.Fatalf("spaced command line was not parsed: client=%#v ok=%v", client, ok)
	}
	client.Close()
}

func TestSnapshotRecordJSONContainsOnlyExpectedAccountHash(t *testing.T) {
	record := SnapshotRecord{SchemaVersion: 1, ID: "20260807T000000Z", CapturedAt: time.Now(), AccountHash: "hash"}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PUUID") || strings.Contains(string(data), "Summoner") {
		t.Fatalf("unexpected private schema: %s", data)
	}
}

func TestPrivacyListsEveryClientWrite(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&app{}).handlePrivacy(recorder, httptest.NewRequest(http.MethodGet, "/api/privacy", nil))
	var privacy struct {
		Reads           []string `json:"reads"`
		ExplicitWrites  []string `json:"explicitWrites"`
		AutomaticWrites []string `json:"automaticWrites"`
		ExternalReads   []string `json:"externalReads"`
		Stores          []string `json:"stores"`
		NeverStores     []string `json:"neverStores"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &privacy); err != nil {
		t.Fatal(err)
	}
	if len(privacy.ExplicitWrites) != 3 {
		t.Fatalf("explicit client writes = %#v", privacy.ExplicitWrites)
	}
	if len(privacy.AutomaticWrites) != 3 {
		t.Fatalf("automatic client writes = %#v", privacy.AutomaticWrites)
	}
	automaticWrites := strings.Join(privacy.AutomaticWrites, "\n")
	for _, expected := range []string{"默认关闭", "ReadyCheck", "EndOfGame", "Reconnect"} {
		if !strings.Contains(automaticWrites, expected) {
			t.Fatalf("automatic write statement is missing %q: %s", expected, automaticWrites)
		}
	}
	writes := strings.Join(privacy.ExplicitWrites, "\n")
	for _, expected := range []string{"符文", "装备方案", "英雄选择", "[DL] ", "页数已满", "最旧符文页", "最多回收 5 页", "绝不删除其它符文页", "回放"} {
		if !strings.Contains(writes, expected) {
			t.Fatalf("privacy statement is missing %q: %s", expected, writes)
		}
	}
	externalReads := strings.Join(privacy.ExternalReads, "\n")
	for _, expected := range []string{"绝活哥", "OP.GG 韩服专家榜", "第三方玩家 Riot ID", "Riot 官方接口", "公开对局", "不携带本机账号", "6 小时"} {
		if !strings.Contains(externalReads, expected) {
			t.Fatalf("specialist external-read statement is missing %q: %s", expected, externalReads)
		}
	}
	reads := strings.Join(privacy.Reads, "\n")
	for _, expected := range []string{"League Client", "RiotClientServices", "aliases/v1/lookup", "独立端口和令牌", "PUUID"} {
		if !strings.Contains(reads, expected) {
			t.Fatalf("local read statement is missing %q: %s", expected, reads)
		}
	}
	if !strings.Contains(strings.Join(privacy.Stores, "\n"), "脱敏账号标识") || !strings.Contains(strings.Join(privacy.NeverStores, "\n"), "PUUID") {
		t.Fatalf("LP storage privacy boundary is incomplete: stores=%#v never=%#v", privacy.Stores, privacy.NeverStores)
	}
}

func TestFriendlyErrorDoesNotExposeLocalDetails(t *testing.T) {
	message := friendlyError(&LCUHTTPError{Path: "/secret/path?token=abc", StatusCode: 500})
	for _, secret := range []string{"/secret/path", "token=abc", "HTTP 500"} {
		if strings.Contains(message, secret) {
			t.Fatalf("friendly error leaked %q: %s", secret, message)
		}
	}
}

func TestFriendlyDiscoveryErrorsAreActionableAndRedacted(t *testing.T) {
	permissionMessage := friendlyError(errLCUCredentialsUnreadable)
	if !strings.Contains(permissionMessage, "管理员身份") || strings.Contains(permissionMessage, "token") {
		t.Fatalf("unexpected permission message: %s", permissionMessage)
	}
	probeMessage := friendlyError(errLCUProbeFailed)
	if !strings.Contains(probeMessage, "本机日志") || strings.Contains(probeMessage, "token") {
		t.Fatalf("unexpected probe message: %s", probeMessage)
	}
}

func TestDiagnosticsExposeOnlyDiscoveryCounts(t *testing.T) {
	a := &app{discovery: LCUDiscoveryStatus{
		AttemptAt: time.Now(), Method: "native", ProcessCount: 2, UnreadableProcesses: 1,
		CommandLineCount: 1, CredentialCandidates: 1, LockfilesChecked: 20, LockfilesFound: 0,
		ProbeFailures: 1, Result: "probe-failed", Detail: "已找到客户端凭据，但本地接口尚未响应",
	}}
	recorder := httptest.NewRecorder()
	a.handleDiagnostics(recorder, httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil))
	body := recorder.Body.String()
	for _, expected := range []string{`"method":"native"`, `"processCount":2`, `"result":"probe-failed"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("diagnostics missing %s: %s", expected, body)
		}
	}
	for _, secret := range []string{"private-token", "remoting-auth-token", "app-port"} {
		if strings.Contains(body, secret) {
			t.Fatalf("diagnostics leaked %q: %s", secret, body)
		}
	}
}

func TestConcurrentIdenticalSnapshotsAreDeduplicated(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"snapshots", "pools", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store := &localStore{root: root, salt: bytes.Repeat([]byte{2}, 32)}
	pool, _ := validatePoolManifest(PoolManifest{Name: "测试", Version: "1", Names: []string{"测试皮肤"}})
	snapshot := Snapshot{Summoner: Summoner{PUUID: "private"}}
	const count = 12
	ids := make(chan string, count)
	errs := make(chan error, count)
	var wait sync.WaitGroup
	for range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			record, err := store.saveSnapshot(snapshot, pool)
			if err != nil {
				errs <- err
				return
			}
			ids <- record.ID
		}()
	}
	wait.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	unique := map[string]bool{}
	for id := range ids {
		unique[id] = true
	}
	if len(unique) != 1 {
		t.Fatalf("unique snapshot IDs = %d, want 1 exact-state record", len(unique))
	}
	entries, err := os.ReadDir(filepath.Join(root, "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("snapshot files = %d, want 1", len(entries))
	}
}

func TestDiagnosticLogRotationRemainsBounded(t *testing.T) {
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, "diagnostics.jsonl")
	backup := filepath.Join(logs, "diagnostics.1.jsonl")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 2*1024*1024+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &localStore{root: root}
	if err := store.appendDiagnostic(map[string]any{"event": "test"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > 4096 {
		t.Fatalf("active diagnostic log was not rotated: info=%v err=%v", info, err)
	}
	if backupInfo, err := os.Stat(backup); err != nil || backupInfo.Size() <= 2*1024*1024 {
		t.Fatalf("diagnostic backup missing after rotation: info=%v err=%v", backupInfo, err)
	}
}

func TestDiagnosticLogDownloadUsesTrustedFixedFile(t *testing.T) {
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}
	store := &localStore{root: root}
	if err := store.appendDiagnostic(map[string]any{"event": "lcu_discovery", "result": "process-not-found"}); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store}
	recorder := httptest.NewRecorder()
	a.handleDiagnosticLog(recorder, httptest.NewRequest(http.MethodGet, "/api/diagnostics/log", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"event":"lcu_discovery"`) {
		t.Fatalf("unexpected diagnostic download: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, "lol-loot-diagnostics.jsonl") {
		t.Fatalf("unexpected disposition: %q", disposition)
	}
}

func TestDiagnosticLogReaderRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "outside.log")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(logs, "diagnostics.jsonl")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := (&localStore{root: root}).readDiagnosticLog(); err == nil {
		t.Fatal("diagnostic reader accepted a symlink")
	}
}

func TestDiagnosticLogWriterRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "outside.log")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(logs, "diagnostics.jsonl")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := (&localStore{root: root}).appendDiagnostic(map[string]any{"event": "test"}); err == nil {
		t.Fatal("diagnostic writer accepted a symlink")
	}
}
