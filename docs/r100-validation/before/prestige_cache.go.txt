package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	prestigeArtworkCacheDirectory  = "prestige-artwork"
	prestigeArtworkCacheMaxEntries = 512
	prestigeArtworkCacheMaxBytes   = 192 * 1024 * 1024
	prestigeArtworkCacheMaxAge     = 45 * 24 * time.Hour
	prestigeArtworkMaxEntryBytes   = 8 * 1024 * 1024
)

type prestigeArtworkFile struct {
	path   string
	size   int64
	usedAt time.Time
}

func prestigeArtworkCacheName(instanceID string) string {
	digest := sha256.Sum256([]byte("prestige:" + strings.ToLower(strings.TrimSpace(instanceID))))
	return hex.EncodeToString(digest[:]) + ".img"
}

func validPrestigeArtwork(data []byte) bool {
	if len(data) == 0 || len(data) > prestigeArtworkMaxEntryBytes {
		return false
	}
	contentType := http.DetectContentType(data)
	return strings.HasPrefix(contentType, "image/") && contentType != "image/svg+xml"
}

func (s *localStore) prestigeArtworkPath(instanceID string) string {
	return filepath.Join(s.root, prestigeArtworkCacheDirectory, prestigeArtworkCacheName(instanceID))
}

func (s *localStore) loadPrestigeArtwork(instanceID string) ([]byte, bool) {
	if s == nil || !prestigeUUID.MatchString(strings.ToLower(strings.TrimSpace(instanceID))) {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.prestigeArtworkPath(instanceID)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > prestigeArtworkMaxEntryBytes || time.Since(info.ModTime()) > prestigeArtworkCacheMaxAge {
		if err == nil {
			_ = os.Remove(path)
		}
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || !validPrestigeArtwork(data) {
		_ = os.Remove(path)
		return nil, false
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return data, true
}

func (s *localStore) storePrestigeArtwork(instanceID string, data []byte) error {
	if s == nil || !prestigeUUID.MatchString(strings.ToLower(strings.TrimSpace(instanceID))) || !validPrestigeArtwork(data) {
		return errors.New("invalid prestige artwork cache entry")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := atomicWriteFile(s.prestigeArtworkPath(instanceID), data, 0o600); err != nil {
		return err
	}
	return s.prunePrestigeArtworkLocked(time.Now())
}

func (s *localStore) prunePrestigeArtworkLocked(now time.Time) error {
	directory := filepath.Join(s.root, prestigeArtworkCacheDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	files := make([]prestigeArtworkFile, 0, len(entries))
	var total int64
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		info, infoErr := os.Lstat(path)
		if infoErr != nil {
			continue
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || now.Sub(info.ModTime()) > prestigeArtworkCacheMaxAge || info.Size() <= 0 || info.Size() > prestigeArtworkMaxEntryBytes {
			_ = os.Remove(path)
			continue
		}
		files = append(files, prestigeArtworkFile{path: path, size: info.Size(), usedAt: info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].usedAt.Before(files[j].usedAt) })
	for len(files) > 0 && (len(files) > prestigeArtworkCacheMaxEntries || total > prestigeArtworkCacheMaxBytes) {
		oldest := files[0]
		files = files[1:]
		_ = os.Remove(oldest.path)
		total -= oldest.size
	}
	return nil
}
