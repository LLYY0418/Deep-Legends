package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	storageDirectory = "LOLLootAssistant"
	maxPoolEntries   = 10000
	maxSnapshots     = 30
)

type PoolManifest struct {
	SchemaVersion int         `json:"schemaVersion"`
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Source        string      `json:"source"`
	Version       string      `json:"version"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	Entries       []PoolEntry `json:"entries,omitempty"`
	Names         []string    `json:"names,omitempty"`
	Hash          string      `json:"hash"`
	BuiltIn       bool        `json:"builtIn"`
}

type PoolEntry struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type snapshotSkin struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ChampionName string `json:"championName,omitempty"`
	Rarity       string `json:"rarity,omitempty"`
	PoolName     string `json:"poolName,omitempty"`
}

type SnapshotRecord struct {
	SchemaVersion int            `json:"schemaVersion"`
	ID            string         `json:"id"`
	CapturedAt    time.Time      `json:"capturedAt"`
	AccountHash   string         `json:"accountHash"`
	PoolID        string         `json:"poolId"`
	PoolName      string         `json:"poolName"`
	PoolVersion   string         `json:"poolVersion"`
	PoolHash      string         `json:"poolHash"`
	Owned         []snapshotSkin `json:"owned"`
	Remaining     []snapshotSkin `json:"remaining"`
}

type SnapshotSummary struct {
	ID             string    `json:"id"`
	CapturedAt     time.Time `json:"capturedAt"`
	AccountHash    string    `json:"accountHash"`
	PoolID         string    `json:"poolId"`
	PoolName       string    `json:"poolName"`
	PoolVersion    string    `json:"poolVersion"`
	PoolHash       string    `json:"poolHash"`
	OwnedCount     int       `json:"ownedCount"`
	RemainingCount int       `json:"remainingCount"`
}

type SnapshotDiff struct {
	From         SnapshotSummary `json:"from"`
	To           SnapshotSummary `json:"to"`
	AddedOwned   []snapshotSkin  `json:"addedOwned"`
	RemovedOwned []snapshotSkin  `json:"removedOwned"`
	NewRemaining []snapshotSkin  `json:"newRemaining"`
	NoLongerPool []snapshotSkin  `json:"noLongerRemaining"`
}

type localStore struct {
	root string
	salt []byte
	mu   sync.Mutex
}

func openLocalStore() (*localStore, error) {
	root := strings.TrimSpace(os.Getenv("LOL_LOOT_DATA_DIR"))
	if root == "" {
		base, err := os.UserCacheDir()
		if err != nil || strings.TrimSpace(base) == "" {
			return nil, errors.New("local application data directory is unavailable")
		}
		root = filepath.Join(base, storageDirectory)
	}
	root = filepath.Clean(root)
	for _, path := range []string{root, filepath.Join(root, "pools"), filepath.Join(root, "snapshots"), filepath.Join(root, "logs"), filepath.Join(root, prestigeArtworkCacheDirectory), filepath.Join(root, championDataCacheDirectory)} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, fmt.Errorf("create local storage: %w", err)
		}
		if info, err := os.Lstat(path); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("local storage path is not a trusted directory")
		}
	}
	saltPath := filepath.Join(root, "account-salt")
	salt, err := os.ReadFile(saltPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read account salt: %w", err)
		}
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return nil, err
		}
		if err := atomicWriteFile(saltPath, []byte(hex.EncodeToString(salt)), 0o600); err != nil {
			return nil, err
		}
	} else {
		decoded, decodeErr := hex.DecodeString(strings.TrimSpace(string(salt)))
		if decodeErr != nil || len(decoded) != 32 {
			return nil, errors.New("invalid local account salt")
		}
		salt = decoded
	}
	store := &localStore{root: root, salt: salt}
	store.mu.Lock()
	store.deduplicateSnapshotsLocked()
	_ = store.prunePrestigeArtworkLocked(time.Now())
	store.mu.Unlock()
	return store, nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func normalizedPoolHash(names []string) string {
	keys := make([]string, 0, len(names))
	for _, name := range names {
		if key := normalizeName(canonicalPoolName(name)); key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	hash := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return hex.EncodeToString(hash[:])
}

func normalizedPoolEntryHash(entries []PoolEntry) string {
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, fmt.Sprintf("%d\x00%s", entry.ID, normalizeName(canonicalPoolName(entry.Name))))
	}
	sort.Strings(keys)
	hash := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return hex.EncodeToString(hash[:])
}

func validatePoolManifest(manifest PoolManifest) (PoolManifest, error) {
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Source = strings.TrimSpace(manifest.Source)
	manifest.Version = strings.TrimSpace(manifest.Version)
	if manifest.Name == "" || len([]rune(manifest.Name)) > 80 {
		return PoolManifest{}, errors.New("奖池名称为空或过长")
	}
	entryCount := len(manifest.Names)
	if len(manifest.Entries) > 0 {
		entryCount = len(manifest.Entries)
	}
	if entryCount == 0 || entryCount > maxPoolEntries {
		return PoolManifest{}, fmt.Errorf("奖池条目必须为 1 到 %d 条", maxPoolEntries)
	}
	if len(manifest.Entries) > 0 {
		seenIDs := map[int64]bool{}
		cleanEntries := make([]PoolEntry, 0, len(manifest.Entries))
		cleanNames := make([]string, 0, len(manifest.Entries))
		for _, raw := range manifest.Entries {
			name := strings.TrimSpace(raw.Name)
			if raw.ID <= 0 || seenIDs[raw.ID] {
				return PoolManifest{}, fmt.Errorf("奖池包含重复或无效皮肤 ID：%d", raw.ID)
			}
			if name == "" || len([]rune(name)) > 160 || strings.IndexFunc(name, func(r rune) bool { return unicode.IsControl(r) }) >= 0 {
				return PoolManifest{}, errors.New("奖池包含空名称、过长名称或控制字符")
			}
			seenIDs[raw.ID] = true
			cleanEntries = append(cleanEntries, PoolEntry{ID: raw.ID, Name: name})
			cleanNames = append(cleanNames, name)
		}
		manifest.SchemaVersion = 2
		manifest.Entries = cleanEntries
		manifest.Names = cleanNames
		manifest.Hash = normalizedPoolEntryHash(cleanEntries)
		if manifest.ID == "" || !manifest.BuiltIn {
			manifest.ID = "custom-" + manifest.Hash[:12]
		}
		if manifest.UpdatedAt.IsZero() {
			manifest.UpdatedAt = time.Now().UTC()
		}
		return manifest, nil
	}
	seen := map[string]bool{}
	clean := make([]string, 0, len(manifest.Names))
	for _, raw := range manifest.Names {
		name := strings.TrimSpace(raw)
		if name == "" || len([]rune(name)) > 160 || strings.IndexFunc(name, func(r rune) bool { return unicode.IsControl(r) }) >= 0 {
			return PoolManifest{}, errors.New("奖池包含空名称、过长名称或控制字符")
		}
		key := normalizeName(canonicalPoolName(name))
		if key == "" || seen[key] {
			return PoolManifest{}, fmt.Errorf("奖池包含重复或无效名称：%s", name)
		}
		seen[key] = true
		clean = append(clean, name)
	}
	manifest.SchemaVersion = 1
	manifest.Names = clean
	manifest.Hash = normalizedPoolHash(clean)
	if manifest.ID == "" || !manifest.BuiltIn {
		manifest.ID = "custom-" + manifest.Hash[:12]
	}
	if manifest.UpdatedAt.IsZero() {
		manifest.UpdatedAt = time.Now().UTC()
	}
	return manifest, nil
}

func (s *localStore) savePool(manifest PoolManifest) error {
	if s == nil || manifest.BuiltIn {
		return errors.New("custom pool storage is unavailable")
	}
	manifest, err := validatePoolManifest(manifest)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.root, "pools", manifest.ID+".json")
	if info, statErr := os.Lstat(path); statErr == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("custom pool path is not a regular file")
	}
	if existing, readErr := os.ReadFile(path); readErr == nil {
		var stored PoolManifest
		if json.Unmarshal(existing, &stored) == nil && stored.Hash == manifest.Hash {
			return nil
		}
		return errors.New("同一奖池 ID 已存在不同内容")
	}
	return atomicWriteFile(path, data, 0o600)
}

func (s *localStore) loadPools() []PoolManifest {
	if s == nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(s.root, "pools"))
	if err != nil {
		return nil
	}
	var pools []PoolManifest
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.root, "pools", entry.Name()))
		if err != nil || len(data) > 1024*1024 {
			continue
		}
		var manifest PoolManifest
		if json.Unmarshal(data, &manifest) != nil {
			continue
		}
		validated, err := validatePoolManifest(manifest)
		if err == nil && validated.Hash == manifest.Hash && validated.ID == strings.TrimSuffix(entry.Name(), ".json") {
			pools = append(pools, validated)
		}
	}
	return pools
}

func (s *localStore) accountHash(summoner Summoner) string {
	if s == nil {
		return ""
	}
	identity := summoner.PUUID
	if identity == "" {
		identity = fmt.Sprintf("summoner:%d", summoner.SummonerID)
	}
	hash := sha256.New()
	_, _ = hash.Write(s.salt)
	_, _ = hash.Write([]byte(identity))
	return hex.EncodeToString(hash.Sum(nil))[:16]
}

func snapshotSkins(skins []Skin) []snapshotSkin {
	out := make([]snapshotSkin, 0, len(skins))
	for _, skin := range skins {
		out = append(out, snapshotSkin{ID: skin.ID, Name: skin.Name, ChampionName: skin.ChampionName, Rarity: skin.Rarity, PoolName: skin.PoolName})
	}
	return out
}

func snapshotStateFingerprint(record SnapshotRecord) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%s\x00", record.AccountHash, record.PoolID, record.PoolHash)
	writeIDs := func(skins []snapshotSkin) {
		ids := make([]int64, 0, len(skins))
		seen := make(map[int64]bool, len(skins))
		for _, skin := range skins {
			if skin.ID > 0 && !seen[skin.ID] {
				seen[skin.ID] = true
				ids = append(ids, skin.ID)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			_, _ = fmt.Fprintf(hash, "%d,", id)
		}
		_, _ = hash.Write([]byte{0})
	}
	writeIDs(record.Owned)
	writeIDs(record.Remaining)
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *localStore) snapshotRecordsLocked() []SnapshotRecord {
	entries, err := os.ReadDir(filepath.Join(s.root, "snapshots"))
	if err != nil {
		return nil
	}
	records := make([]SnapshotRecord, 0, len(entries))
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if entry.IsDir() || id == entry.Name() {
			continue
		}
		if record, err := s.loadSnapshot(id); err == nil {
			records = append(records, record)
		}
	}
	return records
}

func (s *localStore) deduplicateSnapshotsLocked() []SnapshotRecord {
	records := s.snapshotRecordsLocked()
	sort.Slice(records, func(i, j int) bool { return records[i].CapturedAt.After(records[j].CapturedAt) })
	seen := make(map[string]bool, len(records))
	unique := records[:0]
	for _, record := range records {
		fingerprint := snapshotStateFingerprint(record)
		if seen[fingerprint] {
			_ = os.Remove(filepath.Join(s.root, "snapshots", record.ID+".json"))
			continue
		}
		seen[fingerprint] = true
		unique = append(unique, record)
	}
	return unique
}

func (s *localStore) saveSnapshot(snapshot Snapshot, pool PoolManifest) (SnapshotRecord, error) {
	if s == nil {
		return SnapshotRecord{}, errors.New("local history is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	record := SnapshotRecord{
		SchemaVersion: 1, CapturedAt: now, AccountHash: s.accountHash(snapshot.Summoner),
		PoolID: pool.ID, PoolName: pool.Name, PoolVersion: pool.Version, PoolHash: pool.Hash,
		Owned: snapshotSkins(snapshot.Owned), Remaining: snapshotSkins(snapshot.Remaining),
	}
	fingerprint := snapshotStateFingerprint(record)
	for _, existing := range s.deduplicateSnapshotsLocked() {
		if snapshotStateFingerprint(existing) == fingerprint {
			return existing, nil
		}
	}
	suffix, err := randomToken(6)
	if err != nil {
		return SnapshotRecord{}, err
	}
	record.ID = now.Format("20060102T150405.000000000Z") + "." + suffix
	data, err := json.Marshal(record)
	if err != nil {
		return SnapshotRecord{}, err
	}
	if err := atomicWriteFile(filepath.Join(s.root, "snapshots", record.ID+".json"), data, 0o600); err != nil {
		return SnapshotRecord{}, err
	}
	s.pruneSnapshots()
	return record, nil
}

func (s *localStore) pruneSnapshots() {
	entries, err := os.ReadDir(filepath.Join(s.root, "snapshots"))
	if err != nil {
		return
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > maxSnapshots {
		_ = os.Remove(filepath.Join(s.root, "snapshots", names[0]))
		names = names[1:]
	}
}

func validRecordID(id string) bool {
	if id == "" || len(id) > 64 || strings.ContainsAny(id, `/\\`) {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && r != 'T' && r != 'Z' && r != '.' {
			return false
		}
	}
	return true
}

func (s *localStore) loadSnapshot(id string) (SnapshotRecord, error) {
	if s == nil || !validRecordID(id) {
		return SnapshotRecord{}, errors.New("invalid snapshot ID")
	}
	path := filepath.Join(s.root, "snapshots", id+".json")
	if info, statErr := os.Lstat(path); statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return SnapshotRecord{}, errors.New("snapshot is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SnapshotRecord{}, err
	}
	if len(data) > 8*1024*1024 {
		return SnapshotRecord{}, errors.New("snapshot is too large")
	}
	var record SnapshotRecord
	if err := json.Unmarshal(data, &record); err != nil || record.ID != id {
		return SnapshotRecord{}, errors.New("invalid snapshot")
	}
	return record, nil
}

func snapshotSummary(record SnapshotRecord) SnapshotSummary {
	return SnapshotSummary{
		ID: record.ID, CapturedAt: record.CapturedAt, AccountHash: record.AccountHash,
		PoolID: record.PoolID, PoolName: record.PoolName, PoolVersion: record.PoolVersion, PoolHash: record.PoolHash,
		OwnedCount: len(record.Owned), RemainingCount: len(record.Remaining),
	}
}

func (s *localStore) listSnapshots() []SnapshotSummary {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	records := s.deduplicateSnapshotsLocked()
	s.mu.Unlock()
	var summaries []SnapshotSummary
	for _, record := range records {
		summaries = append(summaries, snapshotSummary(record))
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].CapturedAt.After(summaries[j].CapturedAt) })
	return summaries
}

func diffSnapshots(from, to SnapshotRecord) (SnapshotDiff, error) {
	if from.PoolHash != to.PoolHash {
		return SnapshotDiff{}, errors.New("奖池版本不同，不能直接比较")
	}
	if from.AccountHash != to.AccountHash {
		return SnapshotDiff{}, errors.New("快照来自不同账号，不能直接比较")
	}
	return SnapshotDiff{
		From: snapshotSummary(from), To: snapshotSummary(to),
		AddedOwned: differenceSkins(to.Owned, from.Owned), RemovedOwned: differenceSkins(from.Owned, to.Owned),
		NewRemaining: differenceSkins(to.Remaining, from.Remaining), NoLongerPool: differenceSkins(from.Remaining, to.Remaining),
	}, nil
}

func differenceSkins(left, right []snapshotSkin) []snapshotSkin {
	other := map[int64]bool{}
	for _, skin := range right {
		other[skin.ID] = true
	}
	var out []snapshotSkin
	for _, skin := range left {
		if !other[skin.ID] {
			out = append(out, skin)
		}
	}
	return out
}

func (s *localStore) appendDiagnostic(event map[string]any) error {
	if s == nil {
		return errors.New("local storage unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.root, "logs", "diagnostics.jsonl")
	if info, err := os.Stat(path); err == nil && info.Size() > 2*1024*1024 {
		backup := filepath.Join(s.root, "logs", "diagnostics.1.jsonl")
		_ = os.Remove(backup)
		if err := os.Rename(path, backup); err != nil {
			if truncateErr := os.Truncate(path, 0); truncateErr != nil {
				return truncateErr
			}
		}
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("diagnostic log is not a trusted regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	event["time"] = time.Now().UTC()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (s *localStore) readDiagnosticLog() ([]byte, error) {
	if s == nil {
		return nil, errors.New("local storage unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.root, "logs", "diagnostics.jsonl")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 2*1024*1024+64*1024 {
		return nil, errors.New("diagnostic log is not a trusted regular file")
	}
	return os.ReadFile(path)
}
