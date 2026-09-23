package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// R116-A P3：hexdata- 前缀文件永久豁免磁盘预算（pruneDiskLocked 里的 protected
// 判断）导致静默增长，评审第 4.5 节推算年增量 6.64 GB。这里验证新增的按 buildID
// 回收路径，同时验证既有 protected 保护没有被改动（Anti-scope 第 4 条）。

const (
	r116aCurrentBuild  = "hexdata-2026-09-18-167d464cf859"
	r116aPreviousBuild = "hexdata-2026-09-04-aaaaaaaaaaaa"
	r116aAncientBuild  = "hexdata-2026-06-11-bbbbbbbbbbbb"
)

func r116aCache(t *testing.T, root string) *championDataCache {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, championDataCacheDirectory), 0o700); err != nil {
		t.Fatal(err)
	}
	return newChampionDataCache(trackTestStore(t, &localStore{root: root}))
}

// r116aWriteHexdataEntry 走既有的 writeDisk 通道落盘，文件名因此带 hexdata- 前缀，
// 与生产环境完全一致（key = {buildID}|{kind}|{id}）。
func r116aWriteHexdataEntry(t *testing.T, cache *championDataCache, key string, fetchedAt time.Time, payload string) string {
	t.Helper()
	data := []byte(payload)
	entry := championCacheEnvelope{
		Schema: championCacheSchema, Key: key, FetchedAt: fetchedAt,
		ExpiresAt: fetchedAt.Add(hexdataCacheTTL), StaleUntil: fetchedAt.Add(hexdataCacheTTL),
		Hash: sha256Bytes(data), Data: data,
	}
	if err := cache.writeDisk(entry); err != nil {
		t.Fatal(err)
	}
	path := cache.pathFor(key)
	// key 以 buildID 打头，buildID 本身带 hexdata- 前缀，所以落盘文件名也带这个
	// 前缀（pathFor 的规则）；bootstrap key 没有这个前缀，走普通 LRU。
	if strings.HasPrefix(key, "hexdata-") != strings.HasPrefix(filepath.Base(path), "hexdata-") {
		t.Fatalf("cache file %s does not match the hexdata- prefix rule for key %q", path, key)
	}
	return path
}

func r116aDirBytes(t *testing.T, dir string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		total += info.Size()
	}
	return total
}

func r116aExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// r116aDrainDiskPrune 等 writeDisk 异步排出的那次目录清理跑完。测试在改 cache 的
// 注入字段（migrationErr / readFile / diskMax*）之前必须先调它，否则 -race 会
// 正确地报数据竞争——生产代码不会在构造之后再改这些字段。
func r116aDrainDiskPrune(cache *championDataCache) {
	for {
		cache.pruneMu.Lock()
		done := cache.pruneDone
		cache.pruneMu.Unlock()
		if done == nil {
			return
		}
		<-done
	}
}

func TestPruneStaleHexdataBuildsRemovesExpiredBuildsOnly(t *testing.T) {
	root := t.TempDir()
	cache := r116aCache(t, root)
	now := time.Now()
	cache.now = func() time.Time { return now }
	dir := filepath.Join(root, championDataCacheDirectory)

	current := r116aWriteHexdataEntry(t, cache, r116aCurrentBuild+"|hero-json|157", now.Add(-time.Hour), `{"items":[]}`)
	currentMeta := r116aWriteHexdataEntry(t, cache, r116aCurrentBuild+"|meta|site", now.Add(-time.Hour), `{"buildId":"x"}`)
	previous := r116aWriteHexdataEntry(t, cache, r116aPreviousBuild+"|hero-json|157", now.Add(-9*24*time.Hour), `{"items":[]}`)
	ancient := r116aWriteHexdataEntry(t, cache, r116aAncientBuild+"|postmatch|all", now.Add(-80*24*time.Hour), `{"157":{}}`)
	// bootstrap key 不带 hexdata- 文件名前缀，不归这条清理路径管（走普通 LRU）。
	bootstrap := r116aWriteHexdataEntry(t, cache, "bootstrap|hero-json|157", now.Add(-40*24*time.Hour), `{"items":[]}`)
	if strings.HasPrefix(filepath.Base(bootstrap), "hexdata-") {
		t.Fatalf("bootstrap entry unexpectedly got the hexdata- prefix: %s", bootstrap)
	}
	// 熔断/validator 状态文件与被保护的另一类文件都不能被碰。
	statePath := filepath.Join(dir, "hexdata-state.json")
	if err := os.WriteFile(statePath, []byte(`{"buildId":"`+r116aCurrentBuild+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	proseedPath := filepath.Join(dir, "proseed-deadbeef.json")
	if err := os.WriteFile(proseedPath, []byte(`{"schema":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := cache.pruneStaleHexdataBuilds(r116aCurrentBuild); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{current, currentMeta} {
		if !r116aExists(path) {
			t.Fatalf("current build file was deleted: %s", path)
		}
	}
	for _, path := range []string{previous, ancient} {
		if r116aExists(path) {
			t.Fatalf("stale build file survived: %s", path)
		}
	}
	for _, path := range []string{bootstrap, statePath, proseedPath} {
		if !r116aExists(path) {
			t.Fatalf("unrelated file was deleted: %s", path)
		}
	}
}

// 判据 2：宽限期内（buildID 刚变化）不能立刻删掉上一个 buildID 的文件，
// 否则 checkedPage 的 PreviousBuildID 回退路径会被抽掉，造成回源风暴。
// 时间通过 cache.now 注入，不需要真等 8 天。
func TestPruneStaleHexdataBuildsHonorsGracePeriod(t *testing.T) {
	root := t.TempDir()
	cache := r116aCache(t, root)
	now := time.Now()
	cache.now = func() time.Time { return now }
	fresh := r116aWriteHexdataEntry(t, cache, r116aPreviousBuild+"|hero-json|157", now.Add(-24*time.Hour), `{"items":[]}`)
	if err := cache.pruneStaleHexdataBuilds(r116aCurrentBuild); err != nil {
		t.Fatal(err)
	}
	if !r116aExists(fresh) {
		t.Fatal("previous build file was deleted inside the grace period")
	}
	// 刚好卡在宽限期边界内（文件年龄 = 8 天 − 1 小时）：仍然保留。
	cache.now = func() time.Time { return now.Add(hexdataStaleBuildGrace - 25*time.Hour) }
	if err := cache.pruneStaleHexdataBuilds(r116aCurrentBuild); err != nil {
		t.Fatal(err)
	}
	if !r116aExists(fresh) {
		t.Fatal("previous build file was deleted one hour before the grace period ended")
	}
	// 过了宽限期（文件年龄 = 8 天 + 24 小时）：删除。
	cache.now = func() time.Time { return now.Add(hexdataStaleBuildGrace) }
	if err := cache.pruneStaleHexdataBuilds(r116aCurrentBuild); err != nil {
		t.Fatal(err)
	}
	if r116aExists(fresh) {
		t.Fatal("previous build file survived past the grace period")
	}
	if hexdataStaleBuildGrace != 8*24*time.Hour {
		t.Fatalf("grace period = %v, want 8 days (2 个 buildID 周期)", hexdataStaleBuildGrace)
	}
}

// 判据 3：173 英雄 × 24 个 patch 的等比例缩小版（20 个 kind × 24 个 build），
// 确认磁盘占用不会随 patch 历史无限增长。
func TestPruneStaleHexdataBuildsBoundsDiskAcrossPatchHistory(t *testing.T) {
	root := t.TempDir()
	cache := r116aCache(t, root)
	now := time.Now()
	cache.now = func() time.Time { return now }
	dir := filepath.Join(root, championDataCacheDirectory)
	payload := `{"items":[` + strings.Repeat(`{"itemId":"3153","games":1000},`, 40) + `{"itemId":"6333","games":900}]}`
	builds := make([]string, 0, 24)
	for patch := 0; patch < 24; patch++ {
		build := fmt.Sprintf("hexdata-2026-%02d-01-%012d", patch%12+1, patch)
		builds = append(builds, build)
		// 越旧的 build 落盘时间越早，全部超出 8 天宽限期。
		fetchedAt := now.Add(-time.Duration(24-patch)*9*24*time.Hour - 24*time.Hour)
		for kind := 0; kind < 20; kind++ {
			r116aWriteHexdataEntry(t, cache, fmt.Sprintf("%s|hero-json|%d", build, kind+1), fetchedAt, payload)
		}
	}
	before := r116aDirBytes(t, dir)
	files, err := filepath.Glob(filepath.Join(dir, "hexdata-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 24*20 {
		t.Fatalf("seeded %d files, want %d", len(files), 24*20)
	}
	newest := builds[len(builds)-1]
	// 最新 build 自己也超过宽限期没关系：prune 只删「不等于 currentBuildID」的文件。
	if err := cache.pruneStaleHexdataBuilds(newest); err != nil {
		t.Fatal(err)
	}
	remaining, err := filepath.Glob(filepath.Join(dir, "hexdata-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 20 {
		t.Fatalf("after prune %d files remain, want 20 (only the current build)", len(remaining))
	}
	after := r116aDirBytes(t, dir)
	if after >= before/10 {
		t.Fatalf("disk usage did not shrink: before=%d after=%d", before, after)
	}
	for _, path := range remaining {
		entry, err := cache.readCacheFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var envelope championCacheEnvelope
		if err := json.Unmarshal(entry, &envelope); err != nil {
			t.Fatal(err)
		}
		if got := strings.SplitN(envelope.Key, "|", 2)[0]; got != newest {
			t.Fatalf("surviving file belongs to build %q, want %q", got, newest)
		}
	}
}

// 读不出 buildID 的文件一律不动：无法确认归属，删了就是猜。
func TestPruneStaleHexdataBuildsIgnoresUnreadableFiles(t *testing.T) {
	root := t.TempDir()
	cache := r116aCache(t, root)
	now := time.Now()
	cache.now = func() time.Time { return now }
	dir := filepath.Join(root, championDataCacheDirectory)
	garbage := filepath.Join(dir, "hexdata-garbage.json")
	if err := os.WriteFile(garbage, []byte("这不是 JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	noKey := filepath.Join(dir, "hexdata-nokey.json")
	if err := os.WriteFile(noKey, []byte(`{"schema":1,"data":"e30="}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// 没有 fetchedAt 的旧信封退回 mtime 判断宽限期，语义不变。
	stale := r116aWriteHexdataEntry(t, cache, r116aAncientBuild+"|hero-json|1", now.Add(-30*24*time.Hour), `{}`)
	if err := os.Chtimes(stale, now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var envelope championCacheEnvelope
	data, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.FetchedAt = time.Time{}
	rewritten, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, rewritten, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cache.pruneStaleHexdataBuilds(r116aCurrentBuild); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{garbage, noKey, stale} {
		if !r116aExists(path) {
			t.Fatalf("unreadable/undated file was deleted: %s", path)
		}
	}
}

// buildID 不合法（空、非 hexdata- 前缀）时必须整体不动手，避免误删。
func TestPruneStaleHexdataBuildsRequiresValidBuildID(t *testing.T) {
	for _, buildID := range []string{"", "bootstrap", r116aCurrentBuild[1:]} {
		root := t.TempDir()
		cache := r116aCache(t, root)
		now := time.Now()
		cache.now = func() time.Time { return now }
		path := r116aWriteHexdataEntry(t, cache, r116aAncientBuild+"|hero-json|157", now.Add(-90*24*time.Hour), `{}`)
		if err := cache.pruneStaleHexdataBuilds(buildID); err != nil {
			t.Fatalf("buildID %q returned %v", buildID, err)
		}
		if !r116aExists(path) {
			t.Fatalf("buildID %q deleted a file it should not have touched", buildID)
		}
	}
	// 目录不存在时安静返回，不报错。
	missing := &championDataCache{dir: filepath.Join(t.TempDir(), "nope")}
	if err := missing.pruneStaleHexdataBuilds(r116aCurrentBuild); err != nil {
		t.Fatalf("missing cache dir returned %v", err)
	}
}

// 真机验证清单第三项的自动化版本：模拟一次 buildID 变化，确认旧文件被清理。
// 触发点是 adoptMeta / adoptCitation 成功之后，异步执行，不阻塞请求主路径。
func TestAdoptingNewBuildIDPrunesStaleHexdataFilesAsynchronously(t *testing.T) {
	root := t.TempDir()
	var events []map[string]any
	var mu sync.Mutex
	recorder := &r116aRecorder{}
	mock := &r116aMock{bodies: map[string][]byte{hexdataMetaPath: r116aMetaFixture(t, r116aCurrentBuild)}}
	provider := newHexdataBudgetProvider(t, root, mock.roundTrip(recorder))
	provider.diag = func(event map[string]any) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}
	cache := provider.cache
	now := time.Now()
	cache.now = func() time.Time { return now }
	stale := r116aWriteHexdataEntry(t, cache, r116aAncientBuild+"|hero-json|157", now.Add(-30*24*time.Hour), `{"items":[]}`)
	previous := r116aWriteHexdataEntry(t, cache, r116aPreviousBuild+"|hero-json|157", now.Add(-time.Hour), `{"items":[]}`)
	statePath := filepath.Join(root, championDataCacheDirectory, "hexdata-state.json")

	// 走生产路径：loadHexdataMeta 成功后 adoptMeta 触发异步清理。
	snapshot, err := provider.loadHexdataMeta(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BuildID != r116aCurrentBuild {
		t.Fatalf("snapshot buildID = %q, want %q", snapshot.BuildID, r116aCurrentBuild)
	}
	provider.hexdata.pruneWait.Wait()
	if r116aExists(stale) {
		t.Fatal("stale build file survived the async prune")
	}
	if !r116aExists(previous) {
		t.Fatal("previous build file was deleted inside the grace period")
	}
	if !r116aExists(statePath) {
		t.Fatal("hexdata-state.json was deleted by the prune")
	}
	mu.Lock()
	var prune map[string]any
	for _, event := range events {
		if event["event"] == "hexdata_build_prune" {
			prune = event
		}
	}
	mu.Unlock()
	if prune == nil {
		t.Fatalf("no hexdata_build_prune diagnostic was recorded: %v", events)
	}
	if prune["buildId"] != r116aCurrentBuild || prune["removed"] != 1 {
		t.Fatalf("prune diagnostic = %#v", prune)
	}

	// 同一个 buildID 不重复扫盘；adoptCitation 这条触发点同样生效。
	another := r116aWriteHexdataEntry(t, cache, r116aAncientBuild+"|hero-json|222", now.Add(-30*24*time.Hour), `{"items":[]}`)
	if !provider.hexdata.adoptCitation(championSourceCitation{Source: "Hexdata", Patch: "16.18", ReportDate: "2026-09-18", BuildID: r116aCurrentBuild, CanonicalURL: "https://" + hexdataHost + "/heroes"}) {
		t.Fatal("adoptCitation rejected a complete citation")
	}
	provider.hexdata.pruneWait.Wait()
	if !r116aExists(another) {
		t.Fatal("adopting the same buildID triggered a second disk scan; prune must run at most once per (process, buildID)")
	}
	// 换一个新 buildID 才会再扫一次。
	if !provider.hexdata.adoptCitation(championSourceCitation{Source: "Hexdata", Patch: "16.20", ReportDate: "2026-10-02", BuildID: "hexdata-2026-10-02-cccccccccccc", CanonicalURL: "https://" + hexdataHost + "/heroes"}) {
		t.Fatal("adoptCitation rejected the rotated citation")
	}
	provider.hexdata.pruneWait.Wait()
	if r116aExists(another) {
		t.Fatal("rotating the buildID did not prune the ancient build")
	}
	if !r116aExists(previous) {
		t.Fatal("rotating the buildID deleted a file inside the grace period")
	}
}

// Anti-scope 第 4 条：既有 protected 保护逻辑不许动。普通 LRU 仍然不能按 mtime
// 误杀 hexdata- 文件，P3 只是并行的第二条清理路径。
func TestPruneDiskStillProtectsHexdataFilesFromGenericLRU(t *testing.T) {
	root := t.TempDir()
	cache := r116aCache(t, root)
	now := time.Now()
	cache.now = func() time.Time { return now }
	protected := r116aWriteHexdataEntry(t, cache, r116aCurrentBuild+"|hero-json|157", now.Add(-60*24*time.Hour), `{"items":[]}`)
	proseed := filepath.Join(root, championDataCacheDirectory, "proseed-cafebabe.json")
	if err := os.WriteFile(proseed, []byte(`{"schema":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	victim := r116aWriteHexdataEntry(t, cache, "bootstrap|hero-json|157", now.Add(-time.Hour), `{"items":[]}`)
	r116aDrainDiskPrune(cache)
	cache.diskMaxEntries = 1
	cache.diskMaxBytes = 1
	if err := cache.pruneDisk(); err != nil {
		t.Fatal(err)
	}
	if !r116aExists(protected) {
		t.Fatalf("protected hexdata body was pruned by the generic LRU: %s", protected)
	}
	if !r116aExists(proseed) {
		t.Fatalf("protected proseed file was pruned: %s", proseed)
	}
	if r116aExists(victim) {
		t.Fatal("the generic LRU stopped working: an unprotected file survived an over-budget prune")
	}
	source, err := os.ReadFile("champion_cache.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `protected := strings.HasPrefix(item.Name(), "hexdata-") || strings.HasPrefix(item.Name(), "proseed-")`) {
		t.Fatal("the protected rule in pruneDiskLocked was modified (Anti-scope 4)")
	}
}

// 上游 500 时清理不能把请求主路径拖垮：prune 是异步的，失败只记诊断。
func TestPruneStaleHexdataBuildsReportsMigrationFailure(t *testing.T) {
	root := t.TempDir()
	cache := r116aCache(t, root)
	now := time.Now()
	cache.now = func() time.Time { return now }
	path := r116aWriteHexdataEntry(t, cache, r116aAncientBuild+"|hero-json|157", now.Add(-30*24*time.Hour), `{}`)
	r116aDrainDiskPrune(cache)
	cache.migrationErr = fmt.Errorf("fixture migration failure")
	if err := cache.pruneStaleHexdataBuilds(r116aCurrentBuild); err == nil {
		t.Fatal("migration failure was swallowed")
	}
	if !r116aExists(path) {
		t.Fatal("a failed prune deleted files")
	}
	// 读文件失败时（注入 readFile）不报错也不删，交给下一次清理。
	cache.migrationErr = nil
	cache.readFile = func(string) ([]byte, error) { return nil, os.ErrPermission }
	if err := cache.pruneStaleHexdataBuilds(r116aCurrentBuild); err != nil {
		t.Fatalf("unreadable cache files returned %v", err)
	}
	if !r116aExists(path) {
		t.Fatal("an unreadable file was deleted")
	}
}

// 清理不许碰网络：整条路径只有本地文件操作。
func TestPruneStaleHexdataBuildsMakesNoRequests(t *testing.T) {
	root := t.TempDir()
	recorder := &r116aRecorder{}
	provider := newHexdataBudgetProvider(t, root, championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder.add(request)
		return nil, fmt.Errorf("prune must not touch the network: %s", request.URL)
	}))
	now := time.Now()
	provider.cache.now = func() time.Time { return now }
	r116aWriteHexdataEntry(t, provider.cache, r116aAncientBuild+"|hero-json|157", now.Add(-30*24*time.Hour), `{}`)
	provider.hexdata.adoptMeta(hexdataMetaSnapshot{BuildID: r116aCurrentBuild, ReportPatch: "16.18", ReportDate: "2026-09-18", HeroCount: 173, AugmentCount: 211})
	provider.hexdata.pruneWait.Wait()
	if got := len(recorder.hexdataPaths()); got != 0 {
		t.Fatalf("prune issued %d requests", got)
	}
}
