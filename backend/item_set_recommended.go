package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

const recommendedItemSetUID = "deep-legends-v2-recommended"

// Game-consumed schema, matching the documented Akari recommendation-file
// approach. No account collection metadata or empty visibility conditions.
type recommendedItemSet struct {
	UID                 string                    `json:"uid"`
	Title               string                    `json:"title"`
	SortRank            int                       `json:"sortrank"`
	Type                string                    `json:"type"`
	Map                 string                    `json:"map"`
	Mode                string                    `json:"mode"`
	Blocks              []recommendedItemSetBlock `json:"blocks"`
	AssociatedChampions []int64                   `json:"associatedChampions"`
	AssociatedMaps      []int64                   `json:"associatedMaps"`
	PreferredItemSlots  []lcuPreferredItemSlot    `json:"preferredItemSlots"`
}
type recommendedItemSetBlock struct {
	Type  string           `json:"type"`
	Items []lcuItemSetItem `json:"items"`
}

func asRecommendedItemSet(set lcuItemSet) recommendedItemSet {
	result := recommendedItemSet{UID: recommendedItemSetUID, Title: set.Title, Type: "global", Map: "any", Mode: "any", AssociatedChampions: []int64{}, AssociatedMaps: []int64{}, PreferredItemSlots: []lcuPreferredItemSlot{}, Blocks: []recommendedItemSetBlock{}}
	for _, block := range set.Blocks {
		result.Blocks = append(result.Blocks, recommendedItemSetBlock{Type: block.Type, Items: block.Items})
	}
	return result
}

// Validate the client-reported root and each destination component. Never
// follow a link out of the install or touch game.cfg/PersistedSettings.json.
func recommendedDirectory(location settingsLocation) (string, error) {
	root := filepath.Clean(location.allowedRoot)
	game := filepath.Dir(location.configRoot)
	if !filepath.IsAbs(root) || !pathWithin(root, game) {
		return "", errors.New("游戏安装路径无效")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("游戏安装目录不可用")
	}
	resolvedGame, err := filepath.EvalSymlinks(game)
	if err != nil || !pathWithin(resolvedRoot, resolvedGame) {
		return "", errors.New("游戏目录超出客户端安装范围")
	}
	// The recommendation root and executable root differ on international
	// installs: Config sits beside Game, whose executable is one level down.
	// Tencent's Config already lives in Game. Accept only these two layouts.
	executableFound := false
	for _, directory := range []string{resolvedGame, filepath.Join(resolvedGame, "Game")} {
		entry, err := os.Lstat(directory)
		if err != nil || !entry.IsDir() || entry.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if stat, err := os.Lstat(filepath.Join(directory, "League of Legends.exe")); err == nil && stat.Mode().IsRegular() {
			executableFound = true
			break
		}
	}
	if !executableFound {
		return "", errors.New("未找到游戏程序，已停止写入推荐方案")
	}
	directory := resolvedGame
	for _, component := range []string{"Config", "Global", "Recommended"} {
		directory = filepath.Join(directory, component)
		if err := os.Mkdir(directory, 0755); err != nil && !os.IsExist(err) {
			return "", errors.New("推荐目录不可写")
		}
		stat, err := os.Lstat(directory)
		if err != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("推荐目录不是安全的本地文件夹")
		}
	}
	return directory, nil
}

func ownedRecommendedFile(file string) ([]byte, error) {
	stat, err := os.Lstat(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > 64<<10 {
		return nil, errors.New("推荐文件不可安全替换")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.New("推荐文件不可读取")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(stat, opened) {
		return nil, errors.New("推荐文件已变化")
	}
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return nil, errors.New("推荐文件不可读取或过大")
	}
	var previous recommendedItemSet
	if json.Unmarshal(data, &previous) != nil || previous.UID != recommendedItemSetUID {
		return nil, errors.New("同名推荐文件不属于 Deep Legends，已保留原文件")
	}
	return data, nil
}

func writeRecommendedItemSet(location settingsLocation, set recommendedItemSet, beforeWrite func() error) error {
	return writeRecommendedItemSetTraced(location, set, beforeWrite, &itemSetWriteTrace{})
}

func writeRecommendedItemSetTraced(location settingsLocation, set recommendedItemSet, beforeWrite func() error, trace *itemSetWriteTrace) (err error) {
	expectedBlocks := trace.ExpectedBlocks
	if len(expectedBlocks) == 0 {
		expectedBlocks = diagnosticBlocksFromRecommended(set.Blocks)
	}
	expectedDigest := trace.ExpectedOrderDigest
	if expectedDigest == "" {
		expectedDigest = itemSetOrderDigest(expectedBlocks)
	}
	*trace = itemSetWriteTrace{
		TraceID: trace.TraceID, Schema: "recommended-global-any-v2", DisplayConfig: trace.DisplayConfig, CleanupStage: "not-attempted",
		ExpectedBlocks: expectedBlocks, ExpectedOrderDigest: expectedDigest,
	}
	trace.Stage = "validate-uid"
	defer func() {
		if err != nil {
			trace.FailureCode = itemSetFailureCode(err)
			err = &itemSetStageError{stage: trace.Stage, err: err}
		}
	}()
	if set.UID != recommendedItemSetUID {
		return errors.New("推荐方案标识无效")
	}
	// Check session before any filesystem mutation, then again before replacement.
	trace.Stage = "initial-session-guard"
	if beforeWrite != nil {
		if err := beforeWrite(); err != nil {
			return err
		}
	}
	trace.Stage = "recommended-directory"
	directory, err := recommendedDirectory(location)
	if err != nil {
		return err
	}
	file := filepath.Join(directory, recommendedItemSetUID+".json")
	trace.Stage = "previous-file-ownership"
	previous, err := ownedRecommendedFile(file)
	if err != nil {
		return err
	}
	trace.ReplacedPrevious = len(previous) > 0
	trace.Stage = "backup-ownership"
	backup := file + ".bak"
	if _, err := ownedRecommendedFile(backup); err != nil {
		return err
	}
	trace.Stage = "serialize"
	data, err := json.Marshal(set)
	if err != nil || len(data) > 64<<10 {
		return errors.New("推荐方案数据无效")
	}
	trace.Bytes = len(data)
	trace.SHA256 = itemSetDigest(data)
	trace.Stage = "replace-session-guard"
	if beforeWrite != nil {
		if err := beforeWrite(); err != nil {
			return err
		}
	}
	trace.Stage = "backup-write"
	if len(previous) > 0 {
		if err := atomicWriteFile(backup, previous, 0644); err != nil {
			return errors.New("无法备份上次推荐方案")
		}
	}
	trace.Stage = "file-write"
	if err := atomicWriteFile(file, data, 0644); err != nil {
		return errors.New("游戏推荐文件写入失败，请检查目录权限")
	}
	trace.FileWritten = true
	trace.Stage = "file-readback"
	verifiedData, err := ownedRecommendedFile(file)
	if err != nil {
		return errors.New("推荐文件写后校验失败，未清理旧方案，请检查推荐文件")
	}
	var verified recommendedItemSet
	if err := json.Unmarshal(verifiedData, &verified); err != nil {
		return errors.New("推荐文件写后校验失败，未清理旧方案，请检查推荐文件")
	}
	trace.ReadbackBlocks = diagnosticBlocksFromRecommended(verified.Blocks)
	trace.ReadbackOrderDigest = itemSetOrderDigest(trace.ReadbackBlocks)
	trace.ReadbackOrderMatch = trace.ExpectedOrderDigest == trace.ReadbackOrderDigest
	if !reflect.DeepEqual(set, verified) || !trace.ReadbackOrderMatch {
		// A mismatch after atomic replacement means another writer may have
		// intervened. Do not overwrite/delete its data in a blind rollback.
		// The previous verified recommendation remains in our bounded backup.
		return errors.New("推荐文件写后校验失败，未清理旧方案，请检查推荐文件")
	}
	trace.ReadbackVerified = true
	trace.Stage = "verified"
	return nil
}

func applyRecommendedItemSet(ctx context.Context, client *LCUClient, current Summoner, request gameplayItemSetApplyRequest, purchasable map[int64]bool, beforeWrite func() error) (string, string, gameplayItemSetNormalization, error) {
	set, stats := newLCUItemSet(request, purchasable)
	stats.Storage = "recommended-file"
	stats.WriteTrace = &itemSetWriteTrace{
		TraceID: request.TraceID, Schema: "recommended-global-any-v2", Stage: "locate-installation", CleanupStage: "not-attempted",
		ExpectedBlocks: stats.WrittenBlocks, ExpectedOrderDigest: stats.WrittenOrderDigest,
	}
	stats.WrittenBlockStats, stats.WrittenItemCount = itemSetStats(set)
	location, err := locateGameSettings(ctx, client)
	if err != nil {
		stats.WriteTrace.FailureCode = "locate-failed"
		return "", "", stats, err
	}
	stats.WriteTrace.DisplayConfig = readDisplayConfigDiagnostic(location)
	guard := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if beforeWrite != nil {
			return beforeWrite()
		}
		return nil
	}
	if err := writeRecommendedItemSetTraced(location, asRecommendedItemSet(set), guard, stats.WriteTrace); err != nil {
		stats.ReadbackBlocks = stats.WriteTrace.ReadbackBlocks
		stats.ReadbackOrderDigest = stats.WriteTrace.ReadbackOrderDigest
		stats.ReadbackOrderMatch = stats.WriteTrace.ReadbackOrderMatch
		return "", "", stats, err
	}
	stats.ReadbackBlocks = stats.WriteTrace.ReadbackBlocks
	stats.ReadbackOrderDigest = stats.WriteTrace.ReadbackOrderDigest
	stats.ReadbackOrderMatch = stats.WriteTrace.ReadbackOrderMatch
	// Migrate only this champion's legacy account entries after the new game file is verified.
	// If the optional LCU cleanup fails, keep the valid file and report partial state.
	stats.RemovedStaleCount, err = removeLegacyItemSets(ctx, client, current, request.ChampionID, guard)
	stats.LegacyCleanupFailed = err != nil
	stats.WriteTrace.CleanupStage = "verified"
	stats.WriteTrace.CleanupFailureCode = itemSetFailureCode(err)
	if err != nil {
		stats.WriteTrace.CleanupStage = itemSetFailureStage(err, "unknown")
	}
	stats.WriteTrace.DiskCleanupStage = "verified"
	stats.WriteTrace.DiskLegacyArchived, err = archiveLegacyRecommendedFiles(ctx, location, request.ChampionID, guard)
	if err != nil {
		stats.LegacyCleanupFailed = true
		stats.WriteTrace.DiskCleanupStage = "incomplete"
		stats.WriteTrace.DiskCleanupFailureCode = itemSetFailureCode(err)
	}
	// User-authorized, exact screenshot/working-file identity; not a generic
	// OP.GG/Akari cleanup. This runs only after an explicit successful Apply.
	stats.WriteTrace.AuthorizedCleanupStage = "not-pending"
	if stats.LegacyCleanupFailed {
		stats.WriteTrace.AuthorizedCleanupStage = "skipped-legacy-incomplete"
	}
	if !stats.LegacyCleanupFailed && authorizedOPGGRemovalPending(location) {
		stats.WriteTrace.AuthorizedCleanupStage = "verified-no-match"
		n, accountErr := removeItemSetsMatching(ctx, client, current, authorizedOPGGItemSet, guard)
		d, diskErr := archiveRecommendedFilesMatching(ctx, location, authorizedOPGGItemSet, guard)
		stats.WriteTrace.AuthorizedAccountRemoved = n
		stats.WriteTrace.AuthorizedDiskArchived = d
		stats.RemovedStaleCount += n
		stats.WriteTrace.DiskLegacyArchived += d
		if accountErr != nil || diskErr != nil {
			stats.LegacyCleanupFailed = true
			stats.WriteTrace.AuthorizedCleanupStage = "incomplete"
			if accountErr != nil {
				stats.WriteTrace.AuthorizedCleanupFailure = "account:" + itemSetFailureCode(accountErr)
			}
			if diskErr != nil {
				stats.WriteTrace.AuthorizedCleanupFailure += ";disk:" + itemSetFailureCode(diskErr)
			}
		} else if n+d > 0 {
			stats.WriteTrace.AuthorizedCleanupStage = "removed"
			if err := markAuthorizedOPGGRemoval(location, guard); err != nil {
				stats.WriteTrace.AuthorizedCleanupStage = "marker-failed"
				stats.WriteTrace.AuthorizedCleanupFailure = itemSetFailureCode(err)
				stats.LegacyCleanupFailed = true
			}
		}
	}
	return recommendedItemSetUID, set.Title, stats, nil
}

func removeLegacyItemSets(ctx context.Context, client *LCUClient, current Summoner, championID int64, guard func() error) (count int, err error) {
	return removeItemSetsMatching(ctx, client, current, func(uid string) bool { return managedItemSetForChampion(uid, championID) }, guard)
}

func removeItemSetsMatching(ctx context.Context, client *LCUClient, current Summoner, matches func(string) bool, guard func() error) (count int, err error) {
	stage := "legacy-read"
	defer func() {
		if err != nil {
			err = &itemSetStageError{stage: stage, err: err}
		}
	}()
	endpoint := fmt.Sprintf("/lol-item-sets/v1/item-sets/%d/sets", current.SummonerID)
	var document lcuItemSetDocument
	if err := client.RequestJSON(ctx, http.MethodGet, endpoint, nil, &document); err != nil {
		return 0, err
	}
	stage = "legacy-account-check"
	var accountID int64
	if raw, present := document["accountId"]; !present || json.Unmarshal(raw, &accountID) != nil || current.AccountID <= 0 || accountID != current.AccountID {
		return 0, errors.New("装备方案账号已变化")
	}
	stage = "legacy-schema-check"
	var sets []json.RawMessage
	if json.Unmarshal(document["itemSets"], &sets) != nil {
		return 0, errors.New("装备方案结构无法确认")
	}
	kept := make([]json.RawMessage, 0, len(sets))
	removed := 0
	for _, raw := range sets {
		var meta struct {
			UID string `json:"uid"`
		}
		if json.Unmarshal(raw, &meta) != nil || string(raw) == "null" {
			return 0, errors.New("装备方案包含无法确认的条目")
		}
		// Exact generated UID shape, not an arbitrary user's title or category.
		if matches(meta.UID) {
			removed++
			continue
		}
		kept = append(kept, raw)
	}
	if removed == 0 {
		return 0, nil
	}
	// The endpoint replaces the entire account document. Re-read before the
	// optional migration rather than overwrite a user's intervening edit.
	// LCU exposes no conditional update here; this is an optimistic guard,
	// not a server-side transaction, so any observed change cancels cleanup.
	stage = "legacy-concurrent-read"
	var latest lcuItemSetDocument
	if err := client.RequestJSON(ctx, http.MethodGet, endpoint, nil, &latest); err != nil {
		return 0, err
	}
	stage = "legacy-concurrent-check"
	if !sameItemSetDocument(document, latest) {
		return 0, errors.New("装备方案已变化，已保留现有方案")
	}
	document["itemSets"], _ = json.Marshal(kept)
	stage = "legacy-session-guard"
	if guard != nil {
		if err := guard(); err != nil {
			return 0, err
		}
	}
	stage = "legacy-write"
	if err := client.RequestJSON(ctx, http.MethodPut, endpoint, document, nil); err != nil {
		return 0, err
	}
	stage = "legacy-readback"
	var verified lcuItemSetDocument
	if err := client.RequestJSON(ctx, http.MethodGet, endpoint, nil, &verified); err != nil {
		return 0, err
	}
	stage = "legacy-readback-account"
	var verifiedAccountID int64
	if json.Unmarshal(verified["accountId"], &verifiedAccountID) != nil || verifiedAccountID != current.AccountID {
		return 0, errors.New("旧方案清理后账号已变化")
	}
	stage = "legacy-readback-content"
	var remaining []json.RawMessage
	if json.Unmarshal(verified["itemSets"], &remaining) != nil || len(remaining) != len(kept) {
		return 0, errors.New("旧方案清理未确认")
	}
	for index := range kept {
		var want, got any
		if json.Unmarshal(kept[index], &want) != nil || json.Unmarshal(remaining[index], &got) != nil || !reflect.DeepEqual(want, got) {
			return 0, errors.New("旧方案清理后集合发生变化")
		}
	}
	return removed, nil
}

// Compare semantic JSON while retaining full integer precision for account IDs
// and extensions. Object key order and whitespace are not user edits.
func sameItemSetDocument(a, b lcuItemSetDocument) bool {
	if len(a) != len(b) {
		return false
	}
	for key, raw := range a {
		other, exists := b[key]
		if !exists {
			return false
		}
		var left, right any
		leftDecoder, rightDecoder := json.NewDecoder(bytes.NewReader(raw)), json.NewDecoder(bytes.NewReader(other))
		leftDecoder.UseNumber()
		rightDecoder.UseNumber()
		if leftDecoder.Decode(&left) != nil || rightDecoder.Decode(&right) != nil || !reflect.DeepEqual(left, right) {
			return false
		}
	}
	return true
}

var managedItemSetPattern = regexp.MustCompile(`^deep-legends-v1-[1-9][0-9]*-(top|jungle|middle|bottom|utility|none|all)$`)

func managedItemSetUID(uid string) bool { return managedItemSetPattern.MatchString(uid) }

// Ownership and champion must both be proven by our exact generated UID.
func managedItemSetForChampion(uid string, championID int64) bool {
	return championID > 0 && managedItemSetUID(uid) && strings.HasPrefix(uid, fmt.Sprintf("deep-legends-v1-%d-", championID))
}
