package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type binaryDiskEntry struct {
	size     int64
	modified time.Time
}

// The strict binary caches own their directories. Scan once, then account every
// atomic write under diskMu, rather than stat thousands of files per image.
func (c *championDataCache) initBinaryDiskIndexLocked() {
	if c.strictEntries != nil {
		return
	}
	c.strictEntries = map[string]binaryDiskEntry{}
	entries, _ := os.ReadDir(c.dir)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		c.strictEntries[filepath.Join(c.dir, entry.Name())] = binaryDiskEntry{info.Size(), info.ModTime()}
		c.strictBytes += info.Size()
	}
}

func (c *championDataCache) accountBinaryDiskWriteLocked(path string, size int64) error {
	c.strictBytes += size - c.strictEntries[path].size
	c.strictEntries[path] = binaryDiskEntry{size, time.Now()}
	for len(c.strictEntries) > c.diskMaxEntries || c.strictBytes > c.diskMaxBytes {
		oldest := ""
		var entry binaryDiskEntry
		for key, item := range c.strictEntries {
			if strings.HasPrefix(filepath.Base(key), "proseed-") {
				continue
			}
			if oldest == "" || item.modified.Before(entry.modified) {
				oldest, entry = key, item
			}
		}
		if oldest == "" {
			return errors.New("protected cache entries exceed disk budget")
		}
		if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
			return err
		}
		c.strictBytes -= entry.size
		delete(c.strictEntries, oldest)
	}
	return nil
}
