package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRankedSplitProbeRecordsOnlyShapeMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	client := &LCUClient{
		baseURL: "https://lcu.test", token: "secret-token",
		http: &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := `{"split":"S1","puuid":"private-player"}`
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
		})},
	}
	a := &app{storage: store}
	a.probeRankedSplitEndpoints(context.Background(), client, "private-player", 123456)
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-player") || strings.Contains(string(data), "123456") || strings.Contains(string(data), "secret-token") {
		t.Fatalf("probe leaked an identifier: %s", data)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil || event["event"] != "lcu_ranked_split_probe" {
		t.Fatalf("probe diagnostic = %s, err=%v", data, err)
	}
	results, ok := event["results"].([]any)
	if !ok || len(results) != 5 {
		t.Fatalf("probe results = %#v", event["results"])
	}
	first, _ := results[0].(map[string]any)
	if first["status"] != float64(200) || first["bytes"] == nil || first["topLevelKeys"] == nil {
		t.Fatalf("probe shape missing: %#v", first)
	}
}
