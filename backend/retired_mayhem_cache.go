package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// This migration only removes verified files inside the application's own caches.
// Keep the retired identifier isolated here; it is never a request destination.
const retiredMayhemCacheTag = "hex" + "data"

func removeRetiredMayhemCache(store *localStore) {
	if store == nil || store.root == "" {
		return
	}
	marker := filepath.Join(store.root, "retired-mayhem-cache-r307")
	if info, e := os.Lstat(marker); e == nil && info.Mode().IsRegular() {
		return
	}
	files, bytes, failed := 0, int64(0), false
	for _, directory := range []string{championDataCacheDirectory, "champion-images"} {
		dir := filepath.Join(store.root, directory)
		if info, e := os.Lstat(dir); e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		entries, e := os.ReadDir(dir)
		if e != nil {
			failed = true
			continue
		}
		for _, entry := range entries {
			target := filepath.Join(dir, entry.Name())
			info, e := os.Lstat(target)
			if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			owned := directory == championDataCacheDirectory && entry.Name() == retiredMayhemCacheTag+"-state.json"
			if !owned && directory == championDataCacheDirectory && strings.HasPrefix(entry.Name(), retiredMayhemCacheTag+"-") && strings.HasSuffix(entry.Name(), ".json") {
				hash := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), retiredMayhemCacheTag+"-"), ".json")
				decoded, e := hex.DecodeString(hash)
				owned = e == nil && len(decoded) == sha256.Size
			}
			if !owned {
				key := retiredCacheEnvelopeKey(target)
				if !strings.Contains(strings.ToLower(key), retiredMayhemCacheTag) {
					continue
				}
				sum := sha256.Sum256([]byte(key))
				hashName := hex.EncodeToString(sum[:]) + ".json"
				owned = entry.Name() == hashName || directory == championDataCacheDirectory && entry.Name() == retiredMayhemCacheTag+"-"+hashName
			}
			if !owned {
				continue
			}
			if e = os.Remove(target); e != nil {
				failed = true
				continue
			}
			files++
			bytes += info.Size()
		}
	}
	_ = store.appendDiagnostic(map[string]any{"event": retiredMayhemCacheTag + "_cache_removed", "files": files, "bytes": bytes})
	if !failed {
		if f, e := os.CreateTemp(store.root, ".retired-cache-*"); e == nil {
			temp := f.Name()
			_ = f.Close()
			_ = os.Rename(temp, marker)
			_ = os.Remove(temp)
		}
	}
}

func retiredCacheEnvelopeKey(name string) string {
	f, e := os.Open(name)
	if e != nil {
		return ""
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 4096))
	token, e := decoder.Token()
	if e != nil || token != json.Delim('{') {
		return ""
	}
	for decoder.More() {
		token, e := decoder.Token()
		if e != nil {
			return ""
		}
		key, ok := token.(string)
		if !ok {
			return ""
		}
		if key == "key" {
			var value string
			if decoder.Decode(&value) == nil {
				return value
			}
			return ""
		}
		var discard json.RawMessage
		if decoder.Decode(&discard) != nil {
			return ""
		}
	}
	return ""
}
