package main

import (
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestR70StaticAssetCompressionAndRevalidation(t *testing.T) {
	body := strings.Repeat("const message = 'Deep Legends';\n", 200)
	handler := newStaticAssetHandler(fstest.MapFS{"app.js": {Data: []byte(body)}})
	request := func(encoding, tag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/app.js", nil)
		r.Header.Set("Accept-Encoding", encoding)
		if tag != "" {
			r.Header.Set("If-None-Match", tag)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	raw := request("", "")
	zipped := request("gzip", "")
	if raw.Code != 200 || zipped.Code != 200 || zipped.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("static response: %d %d %v", raw.Code, zipped.Code, zipped.Header())
	}
	reader, err := gzip.NewReader(zipped.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil || string(decoded) != body {
		t.Fatal("gzip altered source")
	}
	_ = reader.Close()
	if raw.Header().Get("ETag") == zipped.Header().Get("ETag") {
		t.Fatal("encoding variants shared a strong validator")
	}
	cached := request("gzip", zipped.Header().Get("ETag"))
	if cached.Code != 304 || cached.Body.Len() != 0 {
		t.Fatal("cache hit retransmitted asset")
	}
	if request("gzip;q=0, *;q=1", "").Header().Get("Content-Encoding") != "" {
		t.Fatal("ignored gzip exclusion")
	}
	// Source changes invalidate even without a release build fingerprint.
	changed := newStaticAssetHandler(fstest.MapFS{"app.js": {Data: []byte(body + "//new")}})
	r := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	r.Header.Set("If-None-Match", raw.Header().Get("ETag"))
	w := httptest.NewRecorder()
	changed.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("new source incorrectly served 304")
	}
}

func TestR70StaticPathsAndEncodingNegotiation(t *testing.T) {
	handler := newStaticAssetHandler(fstest.MapFS{"index.html": {Data: []byte("hello")}})
	for _, name := range []string{"/missing.js", "/../riot_key.local.txt", "/app.test.cjs"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", name, nil))
		if w.Code != 404 {
			t.Fatalf("unexpected file served: %s", name)
		}
	}
	for header, want := range map[string]bool{"": false, "br": false, "gzip": true, "GZIP; q=0.5": true, "*": true, "gzip;q=0": false, "gzip;q=bogus": false, "gzip;q=2": false} {
		if acceptsGzip(header) != want {
			t.Errorf("Accept-Encoding %q", header)
		}
	}
}

func TestR70StaticRangeAndVariantLength(t *testing.T) {
	body := strings.Repeat("0123456789", 100)
	handler := newStaticAssetHandler(fstest.MapFS{"app.js": {Data: []byte(body)}})
	for _, method := range []string{"GET", "HEAD"} {
		r := httptest.NewRequest(method, "/app.js?version=one", nil)
		r.Header.Set("Accept-Encoding", "gzip")
		r.Header.Set("Range", "bytes=10-19")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 206 || w.Header().Get("Content-Encoding") != "" || w.Header().Get("Content-Length") != "10" {
			t.Fatalf("invalid raw range: %d %v", w.Code, w.Header())
		}
		if method == "GET" && w.Body.String() != body[10:20] {
			t.Fatal("wrong range body")
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD wrote body")
		}
	}
	if acceptsGzip("*;q=1, gzip;q=0") || acceptsGzip("gzip;q=0, *;q=1") {
		t.Fatal("explicit exclusion lost")
	}
}

func TestR70EmbeddedTextTransferBudget(t *testing.T) {
	files, err := fs.Sub(embedded, "web")
	if err != nil {
		t.Fatal(err)
	}
	handler := newStaticAssetHandler(files)
	var rawTotal, gzipTotal int
	// R128 §2.3：统计口径说明已从 UI 移除，shared.js 随之删除，不再出现在嵌入清单里。
	for _, name := range []string{"index.html", "runtime.js", "demo-data.js", "app.js", "favorites-facade.js", "gameplay.js", "champions.js", "friends.js", "suite.js", "app.css", "gameplay.css", "champions.css", "suite.css", "metrics.css"} {
		body, err := fs.ReadFile(files, name)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("GET", "/"+name, nil)
		r.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || w.Body.Len() >= len(body) {
			t.Fatalf("asset %s did not compress", name)
		}
		rawTotal += len(body)
		gzipTotal += w.Body.Len()
		t.Logf("%s raw=%d gzip=%d", name, len(body), w.Body.Len())
	}
	t.Logf("TOTAL raw=%d gzip=%d saved=%.2f%%", rawTotal, gzipTotal, 100*(1-float64(gzipTotal)/float64(rawTotal)))
}

func TestR117StaticAssetSecurityHeadersPreserveRevalidation(t *testing.T) {
	handler := securityHeaders(newStaticAssetHandler(fstest.MapFS{"app.js": {Data: []byte("const ready = true;\n")}}))
	request := func(tag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/app.js", nil)
		if tag != "" {
			r.Header.Set("If-None-Match", tag)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	first := request("")
	if first.Code != http.StatusOK || first.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("static cache policy through security headers: status=%d cache=%q", first.Code, first.Header().Get("Cache-Control"))
	}
	if first.Header().Get("X-Content-Type-Options") != "nosniff" || first.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("security headers missing: %v", first.Header())
	}
	if tag := first.Header().Get("ETag"); tag == "" {
		t.Fatal("static response omitted ETag")
	} else {
		cached := request(tag)
		if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
			t.Fatalf("revalidation through security headers: status=%d body=%d", cached.Code, cached.Body.Len())
		}
		if cached.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("304 cache policy = %q", cached.Header().Get("Cache-Control"))
		}
	}
}
