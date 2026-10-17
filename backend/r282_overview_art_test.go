package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR282OverviewArtClientDiagnostic(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0755); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store}
	fields := `"video_path_present":true,"reduced_motion":true,"video_rendered":true,"src_set":true,"playing":false,"error_code":4,"media_http_status":404,"rerender_count":2`
	for _, item := range []struct {
		fields string
		status int
	}{
		{fields, 204},
		{strings.Replace(fields, `"error_code":4`, `"error_code":5`, 1), 400},
		{fields + `,"playerName":"private"`, 400},
	} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/client-diagnostic", strings.NewReader(`{"event":"overview_art_client","reason":"summary","r278":{`+item.fields+`}}`)))
		if w.Code != item.status {
			t.Fatalf("status=%d want=%d body=%s", w.Code, item.status, w.Body.String())
		}
	}
	events := readDiagnosticEvents(t, store, "overview_art_client")
	if len(events) != 1 || events[0]["media_http_status"] != float64(404) || events[0]["error_code"] != float64(4) {
		t.Fatalf("events=%v", events)
	}
}
