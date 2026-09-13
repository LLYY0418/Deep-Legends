package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

type staticAsset struct {
	once                              sync.Once
	body, compressed                  []byte
	etag, compressedETag, contentType string
	err                               error
}

// The cache key space is limited to files in the embedded FS. Query strings and
// missing paths never create cache entries. Compression is lazy and single-flight.
func newStaticAssetHandler(files fs.FS) http.Handler {
	entries := make(map[string]*staticAsset)
	_ = fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			entries[name] = &staticAsset{}
		}
		return err
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		asset := entries[name]
		if asset == nil {
			http.NotFound(w, r)
			return
		}
		asset.once.Do(func() {
			asset.body, asset.err = fs.ReadFile(files, name)
			if asset.err != nil {
				return
			}
			asset.etag = fmt.Sprintf(`"%x"`, sha256.Sum256(asset.body))
			asset.contentType = mime.TypeByExtension(path.Ext(name))
			switch path.Ext(name) {
			case ".js", ".css", ".html", ".svg":
				var output bytes.Buffer
				writer, _ := gzip.NewWriterLevel(&output, gzip.BestSpeed)
				_, _ = writer.Write(asset.body)
				_ = writer.Close()
				if output.Len() < len(asset.body) {
					asset.compressed = output.Bytes()
					asset.compressedETag = fmt.Sprintf(`"%x"`, sha256.Sum256(asset.compressed))
				}
			}
		})
		if asset.err != nil {
			http.NotFound(w, r)
			return
		}
		body, etag := asset.body, asset.etag
		if len(asset.compressed) > 0 {
			w.Header().Set("Vary", "Accept-Encoding")
			if acceptsGzip(r.Header.Get("Accept-Encoding")) && r.Header.Get("Range") == "" {
				body, etag = asset.compressed, asset.compressedETag
				w.Header().Set("Content-Encoding", "gzip")
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", etag)
		if asset.contentType != "" {
			w.Header().Set("Content-Type", asset.contentType)
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
	})
}

func acceptsGzip(header string) bool {
	wildcard := false
	for _, token := range strings.Split(header, ",") {
		parts := strings.Split(token, ";")
		coding := strings.ToLower(strings.TrimSpace(parts[0]))
		if coding != "gzip" && coding != "*" {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, _ := strings.Cut(strings.TrimSpace(parameter), "=")
			if strings.EqualFold(key, "q") {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil || parsed < 0 || parsed > 1 {
					quality = 0
				} else {
					quality = parsed
				}
			}
		}
		if coding == "gzip" {
			return quality > 0
		}
		wildcard = quality > 0
	}
	return wildcard
}
