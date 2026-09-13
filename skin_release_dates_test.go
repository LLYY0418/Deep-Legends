package main

import (
	"testing"
	"time"
)

func TestFacadeQuestReleaseProjection(t *testing.T) {
	if s := projectFacadeSkin(Skin{ID: 22084}); s.ReleaseDate != "2026-02-04" || s.ReleaseSortDate != "2026-02-04" {
		t.Fatal("Ashe 2026 release missing from embedded projection")
	}
	parent := Skin{ID: 103085, Name: "殿堂传奇 阿狸", Owned: true}
	variant := Skin{ID: 103086, ParentSkinID: 103085, IsVariant: true, Name: "联盟不朽 阿狸"}
	p, v := projectFacadeSkin(parent), projectFacadeSkin(variant)
	if p.ReleaseDate != "2024-06-12" || v.ReleaseSortDate != p.ReleaseDate || v.ReleaseDate != "" {
		t.Fatalf("unverified dates: parent=%+v variant=%+v", p, v)
	}
	if !v.IsVariant || v.Owned || v.Name != variant.Name {
		t.Fatalf("variant or ownership lost: %+v", v)
	}
	unknown := projectFacadeSkin(Skin{ID: 9999999})
	if unknown.ReleaseDate != "" || unknown.ReleaseSortDate != "" {
		t.Fatal("invented date")
	}
	for id, date := range skinReleaseDates {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			t.Fatalf("invalid release %s: %s", id, date)
		}
	}
}
