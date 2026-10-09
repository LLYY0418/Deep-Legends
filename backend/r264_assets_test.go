package main

import (
	"io/fs"
	"testing"
)

func TestR264LazyHistoryFilterScriptIsShipped(t *testing.T) {
	data, err := fs.ReadFile(embedded, "web/history-filters.js")
	if err != nil || len(data) < 100 {
		t.Fatalf("production lazy script missing: %v, bytes=%d", err, len(data))
	}
}
