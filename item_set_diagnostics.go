package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Stable enums only: upstream errors may contain account IDs, paths or bodies.
// A successful readback proves persistence, never the game's rendered geometry.
type itemSetStageError struct {
	stage string
	err   error
}

func (e *itemSetStageError) Error() string { return e.err.Error() }
func (e *itemSetStageError) Unwrap() error { return e.err }
func itemSetFailureStage(err error, fallback string) string {
	var staged *itemSetStageError
	if errors.As(err, &staged) {
		return staged.stage
	}
	return fallback
}

func itemSetFailureCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, os.ErrPermission) {
		return "permission-denied"
	}
	var upstream *LCUHTTPError
	if errors.As(err, &upstream) {
		if upstream.StatusCode >= 100 && upstream.StatusCode <= 599 {
			return "lcu-http-" + strconv.Itoa(upstream.StatusCode)
		}
		return "lcu-http-error"
	}
	// Only our exact local messages are mapped. Never echo unknown error text.
	codes := map[string]string{
		"游戏安装路径无效":                      "invalid-install-path",
		"游戏安装目录不可用":                     "install-unavailable",
		"游戏目录超出客户端安装范围":                 "outside-install-root",
		"未找到游戏程序，已停止写入推荐方案":             "game-executable-missing",
		"推荐目录不可写":                       "directory-not-writable",
		"推荐目录不是安全的本地文件夹":                "unsafe-directory",
		"推荐文件不可安全替换":                    "unsafe-or-oversized-file",
		"推荐文件不可读取":                      "file-unreadable",
		"同名推荐文件不属于 Deep Legends，已保留原文件": "foreign-file",
		"装备方案账号已变化":                     "account-changed",
		"装备方案已变化，已保留现有方案":               "concurrent-document-change",
	}
	if code := codes[err.Error()]; code != "" {
		return code
	}
	return "operation-failed"
}

type itemSetWriteTrace struct {
	AuthorizedCleanupStage   string                   `json:"authorized_cleanup_stage,omitempty"`
	AuthorizedCleanupFailure string                   `json:"authorized_cleanup_failure,omitempty"`
	AuthorizedAccountRemoved int                      `json:"authorized_account_removed,omitempty"`
	AuthorizedDiskArchived   int                      `json:"authorized_disk_archived,omitempty"`
	DiskLegacyArchived       int                      `json:"disk_legacy_archived"`
	DiskCleanupStage         string                   `json:"disk_cleanup_stage"`
	DiskCleanupFailureCode   string                   `json:"disk_cleanup_failure_code,omitempty"`
	TraceID                  string                   `json:"trace_id,omitempty"`
	FailureCode              string                   `json:"failure_code,omitempty"`
	CleanupFailureCode       string                   `json:"cleanup_failure_code,omitempty"`
	DisplayConfig            map[string]any           `json:"display_config,omitempty"`
	Schema                   string                   `json:"schema"`
	Stage                    string                   `json:"stage"`
	Bytes                    int                      `json:"bytes"`
	SHA256                   string                   `json:"sha256,omitempty"`
	ExpectedBlocks           []itemSetDiagnosticBlock `json:"expected_blocks,omitempty"`
	ReadbackBlocks           []itemSetDiagnosticBlock `json:"readback_blocks,omitempty"`
	ExpectedOrderDigest      string                   `json:"expected_order_digest,omitempty"`
	ReadbackOrderDigest      string                   `json:"readback_order_digest,omitempty"`
	ReadbackOrderMatch       bool                     `json:"readback_order_match"`
	ReplacedPrevious         bool                     `json:"replaced_previous"`
	FileWritten              bool                     `json:"file_written"`
	ReadbackVerified         bool                     `json:"readback_verified"`
	CleanupStage             string                   `json:"cleanup_stage"`
}

type itemSetDiagnosticItem struct {
	Index int   `json:"index"`
	ID    int64 `json:"id"`
	Count int   `json:"count"`
}

type itemSetDiagnosticBlock struct {
	Index int                     `json:"index"`
	Type  string                  `json:"type"`
	Items []itemSetDiagnosticItem `json:"items"`
}

type itemSetItemMapping struct {
	BlockIndex     int    `json:"block_index"`
	RequestedIndex int    `json:"requested_index"`
	SortedIndex    int    `json:"sorted_index"`
	WrittenIndex   int    `json:"written_index"`
	RequestedID    int64  `json:"requested_id"`
	NormalizedID   int64  `json:"normalized_id"`
	Count          int    `json:"count"`
	Price          int64  `json:"price,omitempty"`
	PriceKnown     bool   `json:"price_known"`
	Result         string `json:"result"`
	Reason         string `json:"reason,omitempty"`
}

func itemSetOrderDigest(blocks []itemSetDiagnosticBlock) string {
	data, _ := json.Marshal(blocks)
	return itemSetDigest(data)
}

func diagnosticBlocksFromRequest(blocks []gameplayItemSetBlockRequest) []itemSetDiagnosticBlock {
	out := make([]itemSetDiagnosticBlock, 0, len(blocks))
	for blockIndex, block := range blocks {
		items := make([]itemSetDiagnosticItem, 0, len(block.Items))
		for itemIndex, item := range block.Items {
			items = append(items, itemSetDiagnosticItem{Index: itemIndex, ID: item.ID, Count: item.Count})
		}
		out = append(out, itemSetDiagnosticBlock{Index: blockIndex, Type: strings.TrimSpace(block.Type), Items: items})
	}
	return out
}

func diagnosticBlocksFromLCU(blocks []lcuItemSetBlock) []itemSetDiagnosticBlock {
	out := make([]itemSetDiagnosticBlock, 0, len(blocks))
	for blockIndex, block := range blocks {
		items := make([]itemSetDiagnosticItem, 0, len(block.Items))
		for itemIndex, item := range block.Items {
			id, _ := strconv.ParseInt(strings.TrimSpace(item.ID), 10, 64)
			items = append(items, itemSetDiagnosticItem{Index: itemIndex, ID: id, Count: item.Count})
		}
		out = append(out, itemSetDiagnosticBlock{Index: blockIndex, Type: strings.TrimSpace(block.Type), Items: items})
	}
	return out
}

func diagnosticBlocksFromRecommended(blocks []recommendedItemSetBlock) []itemSetDiagnosticBlock {
	lcuBlocks := make([]lcuItemSetBlock, 0, len(blocks))
	for _, block := range blocks {
		lcuBlocks = append(lcuBlocks, lcuItemSetBlock{Type: block.Type, Items: block.Items})
	}
	return diagnosticBlocksFromLCU(lcuBlocks)
}

func itemSetDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// The export checkpoint never creates a directory or changes game settings.
// It runs only on explicit export, with one shared deadline and bounded reads.
func (a *app) recordItemSetExportSnapshot(parent context.Context) {
	started := time.Now()
	event := map[string]any{"event": "item_set_export_snapshot", "diagnostic_schema": 2,
		"game_render_geometry": "unobservable", "game_loaded_recommendation": "unknown"}
	defer func() {
		event["duration_ms"] = time.Since(started).Milliseconds()
		a.recordDiagnostic(event)
	}()
	client, current, err := a.gameplayClient()
	if err != nil {
		event["result"] = "not-connected"
		return
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	var phase string
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase); err == nil {
		switch phase {
		case "None", "Lobby", "Matchmaking", "ReadyCheck", "ChampSelect", "GameStart", "InProgress", "Reconnect", "WaitingForStats", "PreEndOfGame", "EndOfGame":
			event["phase"] = phase
		default:
			event["phase"] = "unknown"
		}
	} else {
		event["phase"] = "unavailable"
	}
	location, err := locateGameSettings(ctx, client)
	if err != nil {
		event["result"] = "locate-failed"
		return
	}
	event["result"] = "sampled"
	event["recommendation"] = readRecommendationDiagnostic(location)
	event["recommendation_inventory"] = readRecommendationInventory(ctx, location)
	event["display_config"] = readDisplayConfigDiagnostic(location)
	// Reuse the guarded read-only migration scan. A failed scan is unknown,
	// not evidence that no legacy entries remain. Never invoke migration here.
	event["legacy"] = readLegacyItemSetDiagnostic(ctx, client, current)
}

// Read-only, bounded inventory of the two game recommendation namespaces.
// Do not export filenames, titles, account IDs or contents, and do not delete
// user/third-party sets just because their title happens to contain "DL".
func readRecommendationInventory(ctx context.Context, location settingsLocation) map[string]any {
	out := map[string]any{"scope": "disk-not-game-loaded", "files": 0, "managed_v2": 0, "legacy_uid": 0, "dl_marker_only": 0, "other": 0, "unreadable": 0, "truncated": false}
	readDir := func(dir string, limit int) []os.DirEntry {
		if ctx.Err() != nil {
			out["truncated"] = true
			return nil
		}
		resolved, err := filepath.EvalSymlinks(dir)
		if os.IsNotExist(err) {
			return nil
		}
		root, rootErr := filepath.EvalSymlinks(location.configRoot)
		allowed, allowedErr := filepath.EvalSymlinks(location.allowedRoot)
		if err != nil || rootErr != nil || allowedErr != nil || !pathWithin(allowed, root) || !pathWithin(root, resolved) {
			out["unreadable"] = out["unreadable"].(int) + 1
			return nil
		}
		f, err := os.Open(resolved)
		if err != nil {
			out["unreadable"] = out["unreadable"].(int) + 1
			return nil
		}
		defer f.Close()
		entries, err := f.ReadDir(limit + 1)
		if err != nil && err != io.EOF {
			out["unreadable"] = out["unreadable"].(int) + 1
		}
		if len(entries) > limit {
			out["truncated"] = true
			entries = entries[:limit]
		}
		return entries
	}
	dirs := []string{filepath.Join(location.configRoot, "Global", "Recommended")}
	championRoot := filepath.Join(location.configRoot, "Champions")
	for _, entry := range readDir(championRoot, 256) {
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			dirs = append(dirs, filepath.Join(championRoot, entry.Name(), "Recommended"))
		}
	}
	for _, dir := range dirs {
		for _, entry := range readDir(dir, 128) {
			if ctx.Err() != nil || out["files"].(int) >= 128 {
				out["truncated"] = true
				return out
			}
			if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				continue
			}
			out["files"] = out["files"].(int) + 1
			data, status := readBoundedItemSetDiagnosticFile(location, filepath.Join(dir, entry.Name()), 64<<10)
			var meta struct {
				UID         string `json:"uid"`
				StartedFrom string `json:"startedFrom"`
			}
			key := "other"
			if status != "ok" || json.Unmarshal(data, &meta) != nil {
				key = "unreadable"
			} else if meta.UID == recommendedItemSetUID {
				key = "managed_v2"
			} else if strings.HasPrefix(meta.UID, "deep-legends-v1-") {
				key = "legacy_uid"
			} else if strings.EqualFold(strings.TrimSpace(meta.StartedFrom), "DL") {
				key = "dl_marker_only"
			}
			out[key] = out[key].(int) + 1
		}
	}
	return out
}

func readBoundedItemSetDiagnosticFile(location settingsLocation, file string, limit int64) ([]byte, string) {
	location.file = file
	resolved, info, err := safeSettingsFile(location)
	if os.IsNotExist(err) {
		return nil, "missing"
	}
	if err != nil {
		return nil, "unsafe-or-unreadable"
	}
	if info.Size() > limit {
		return nil, "too-large"
	}
	f, err := os.Open(resolved)
	if err != nil {
		return nil, "unreadable"
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, "changed-during-read"
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, "unreadable"
	}
	if int64(len(data)) > limit {
		return nil, "too-large"
	}
	return data, "ok"
}

func readRecommendationDiagnostic(location settingsLocation) map[string]any {
	data, result := readBoundedItemSetDiagnosticFile(location,
		filepath.Join(location.configRoot, "Global", "Recommended", recommendedItemSetUID+".json"), 64<<10)
	out := map[string]any{"result": result}
	if result != "ok" {
		return out
	}
	var set recommendedItemSet
	if json.Unmarshal(data, &set) != nil || set.UID != recommendedItemSetUID {
		out["result"] = "invalid-or-foreign"
		return out
	}
	out["sha256"] = itemSetDigest(data)
	out["bytes"] = len(data)
	out["block_count"] = len(set.Blocks)
	counts := make([]int, 0, len(set.Blocks))
	for _, block := range set.Blocks {
		counts = append(counts, len(block.Items))
	}
	blocks := diagnosticBlocksFromRecommended(set.Blocks)
	out["block_item_counts"] = counts
	out["blocks"] = blocks
	out["order_digest"] = itemSetOrderDigest(blocks)
	out["global_any_schema"] = set.Type == "global" && set.Map == "any" && set.Mode == "any" && len(set.AssociatedChampions) == 0 && len(set.AssociatedMaps) == 0 && len(set.PreferredItemSlots) == 0
	return out
}

// Numeric allowlist only, with source keys preserved. These are observed disk
// settings, not an assertion that this game version uses them or has loaded them.
// Missing keys remain absent; never invent default scale, coordinates or units.
func readDisplayConfigDiagnostic(location settingsLocation) map[string]any {
	data, result := readBoundedItemSetDiagnosticFile(location, filepath.Join(location.configRoot, "game.cfg"), 256<<10)
	out := map[string]any{"result": result, "source": "game.cfg", "scope": "disk-values-not-live-geometry", "persisted": readPersistedDisplayDiagnostic(location)}
	if result != "ok" {
		return out
	}
	values, invalid := parseItemSetDisplayConfig(data)
	out["values"] = values
	out["invalid_or_duplicate_fields"] = invalid
	return out
}

func parseItemSetDisplayConfig(data []byte) (map[string]float64, int) {
	values := map[string]float64{}
	seen := map[string]bool{}
	invalid := 0
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 256<<10+1)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		key = section + "." + strings.TrimSpace(key)
		if !ok || !allowedItemSetDisplayField(key) {
			continue
		}
		invalid += addItemSetDisplayValue(values, seen, key, strings.TrimSpace(raw))
	}
	if scanner.Err() != nil {
		invalid++
	}
	return values, invalid
}

func allowedItemSetDisplayField(key string) bool {
	switch key {
	case "General.Width", "General.Height", "General.WindowMode",
		"HUD.GlobalScale", "HUD.ShopScale", "HUD.ItemShopResizeWidth", "HUD.ItemShopResizeHeight",
		"HUD.ItemShopPrevResizeWidth", "HUD.ItemShopPrevResizeHeight", "HUD.ItemShopPrevX", "HUD.ItemShopPrevY",
		"HUD.ItemShopStartPane", "HUD.ItemShopItemDisplayMode":
		return true
	}
	return false
}

func addItemSetDisplayValue(values map[string]float64, seen map[string]bool, key, raw string) int {
	value, err := strconv.ParseFloat(raw, 64)
	duplicate := seen[key]
	seen[key] = true
	if duplicate || err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > 100000 {
		delete(values, key)
		return 1
	}
	values[key] = value
	return 0
}

func readPersistedDisplayDiagnostic(location settingsLocation) map[string]any {
	data, result := readBoundedItemSetDiagnosticFile(location, filepath.Join(location.configRoot, "PersistedSettings.json"), 256<<10)
	out := map[string]any{"result": result, "source": "PersistedSettings.json"}
	if result != "ok" {
		return out
	}
	// Only Game.cfg sections, never Input.ini bindings or unrelated preferences.
	var document struct {
		Files []struct {
			Name     string `json:"name"`
			Sections []struct {
				Name     string `json:"name"`
				Settings []struct {
					Name  string          `json:"name"`
					Value json.RawMessage `json:"value"`
				} `json:"settings"`
			} `json:"sections"`
		} `json:"files"`
	}
	if json.Unmarshal(data, &document) != nil || document.Files == nil {
		out["result"] = "invalid-schema"
		return out
	}
	values := map[string]float64{}
	seen := map[string]bool{}
	invalid := 0
	foundGameConfig := false
	for _, file := range document.Files {
		if !strings.EqualFold(file.Name, "Game.cfg") {
			continue
		}
		foundGameConfig = true
		for _, section := range file.Sections {
			for _, setting := range section.Settings {
				key := section.Name + "." + setting.Name
				if !allowedItemSetDisplayField(key) {
					continue
				}
				var value string
				if json.Unmarshal(setting.Value, &value) != nil {
					value = string(setting.Value)
				}
				invalid += addItemSetDisplayValue(values, seen, key, value)
			}
		}
	}
	if !foundGameConfig {
		out["result"] = "game-config-missing"
	}
	out["values"] = values
	out["invalid_or_duplicate_fields"] = invalid
	return out
}

func readLegacyItemSetDiagnostic(ctx context.Context, client *LCUClient, current Summoner) map[string]any {
	out := map[string]any{"result": "unavailable"}
	if current.AccountID <= 0 || current.SummonerID <= 0 {
		return out
	}
	endpoint := "/lol-item-sets/v1/item-sets/" + strconv.FormatInt(current.SummonerID, 10) + "/sets"
	var doc lcuItemSetDocument
	if client.RequestJSON(ctx, http.MethodGet, endpoint, nil, &doc) != nil {
		return out
	}
	var accountID int64
	var sets []json.RawMessage
	if json.Unmarshal(doc["accountId"], &accountID) != nil || accountID != current.AccountID || json.Unmarshal(doc["itemSets"], &sets) != nil {
		out["result"] = "account-or-schema-mismatch"
		return out
	}
	count, unrecognized := 0, 0
	for _, raw := range sets {
		var set struct {
			UID         string `json:"uid"`
			StartedFrom string `json:"startedFrom"`
		}
		if json.Unmarshal(raw, &set) != nil || string(raw) == "null" {
			out["result"] = "invalid-entry"
			return out
		}
		if managedItemSetUID(set.UID) {
			count++
		} else if strings.HasPrefix(set.UID, "deep-legends-v1-") || strings.EqualFold(strings.TrimSpace(set.StartedFrom), "DL") {
			unrecognized++
		}
	}
	out["result"] = "ok"
	out["managed_legacy_count"] = count
	out["unrecognized_dl_marker_count"] = unrecognized
	out["total_count"] = len(sets)
	return out
}
