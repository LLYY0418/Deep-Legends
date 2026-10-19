package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestR288AnnouncementV2AndLegacyFallback(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "v2", true: "legacy"}[fallback], func(t *testing.T) {
			var calls []string
			client := &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls = append(calls, r.URL.String())
				if fallback && strings.HasSuffix(r.URL.Path, "announcements.v2.json") {
					return updateResponse(404, nil), nil
				}
				return updateResponse(200, bundledAnnouncements), nil
			})}
			_, list, err := fetchAnnouncementDocument(context.Background(), client)
			if err != nil || len(list) == 0 {
				t.Fatalf("%v %#v", err, list)
			}
			if !strings.HasSuffix(calls[0], "announcements.v2.json") {
				t.Fatalf("v2 not preferred: %v", calls)
			}
			if fallback && !strings.HasSuffix(calls[len(calls)-1], "announcements.json") {
				t.Fatalf("no legacy fallback: %v", calls)
			}
			if !fallback && len(calls) != 1 {
				t.Fatalf("unexpected requests: %v", calls)
			}
		})
	}
}

func TestR288AnnouncementSameRevisionReplacesOldBody(t *testing.T) {
	list, err := decodeAnnouncements(bundledAnnouncements)
	if err != nil {
		t.Fatal(err)
	}
	old := list[0]
	old.Content = "近期晚间时段"
	old.UpdatedAt = "2026-10-14T00:00:00+08:00"
	for _, pair := range [][2][]announcement{{list, {old}}, {{old}, list}} {
		got := mergeAnnouncements(pair[0], pair[1])
		if len(got) != 1 || got[0].Revision != "1" || !strings.Contains(got[0].Content, "从每晚**21**点左右") {
			t.Fatalf("stale body %#v", got)
		}
	}
}
