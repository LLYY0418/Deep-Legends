package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestR278P12NoticeSchema(t *testing.T) {
	good := []byte(`[{"id":"outage","revision":"1","popup":true,"title":"公告","summary":"测试","content":"## 内容","images":[],"publishedAt":"2026-10-14T00:00:00+08:00","updatedAt":"2026-10-14T00:00:00+08:00"}]`)
	if _, err := decodeAnnouncements(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`null`, `{}`, `[{"id":"x"}]`, string(good[:len(good)-1]) + `,{"id":"outage","revision":"1"}]`} {
		if _, err := decodeAnnouncements([]byte(bad)); err == nil {
			t.Fatalf("accepted invalid schema: %s", bad)
		}
	}
	var list []map[string]any
	json.Unmarshal(good, &list)
	for _, key := range []string{"popup", "summary", "content", "images", "expiresAt"} {
		saved := list[0][key]
		list[0][key] = nil
		raw, _ := json.Marshal(list)
		if _, err := decodeAnnouncements(raw); err == nil {
			t.Fatalf("null %s accepted", key)
		}
		if key == "expiresAt" {
			delete(list[0], key)
		} else {
			list[0][key] = saved
		}
	}
	list[0]["images"] = []string{"https://evil.test/x.png"}
	raw, _ := json.Marshal(list)
	if _, err := decodeAnnouncements(raw); err == nil {
		t.Fatal("external image accepted")
	}
	_ = time.Now()
}
func TestR278P14LocalSuccessWindow(t *testing.T) {
	a := &app{}
	at := time.Unix(1, 0)
	for i := 0; i < 10; i++ {
		out, handled := a.successfulRequestDiagnostics(map[string]any{"event": "local_request_client", "endpoint": "gameplay", "http_status": 200, "error_kind": "none", "duration_ms": i, "response_bytes": 100}, at, false)
		if !handled || len(out) != 0 {
			t.Fatalf("success was not aggregated: %#v %v", out, handled)
		}
	}
	out, _ := a.successfulRequestDiagnostics(nil, at.Add(time.Minute), true)
	if len(out) != 1 || out[0]["count"] != 10 {
		t.Fatalf("summary %#v", out)
	}
	_, handled := a.successfulRequestDiagnostics(map[string]any{"event": "local_request_client", "http_status": 503}, at, false)
	if handled {
		t.Fatal("failure suppressed")
	}
}

func TestR278P12ActualBuildHasAnnouncementUI(t *testing.T) {
	html, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{`id="announcements-button"`, `id="announcements-dialog"`, `src="/app.js"`} {
		if !strings.Contains(string(html), token) {
			t.Fatalf("actual build lacks %s", token)
		}
	}
	for _, name := range []string{"announcements.js", "announcements.css"} {
		if data, err := embedded.ReadFile("web/" + name); err != nil || len(data) == 0 {
			t.Fatalf("runtime asset absent %s", name)
		}
	}
}
