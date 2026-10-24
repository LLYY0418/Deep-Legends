package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func newPublicDataTestProvider(t *testing.T, root string, transport http.RoundTripper) *championProvider {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, championDataCacheDirectory), 0700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	p := newChampionProvider()
	p.cache = newChampionDataCache(store)
	p.client = &http.Client{Transport: transport}
	return p
}
func publicDataResponse(request *http.Request, data []byte) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: request}
}
