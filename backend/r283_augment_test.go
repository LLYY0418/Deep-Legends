package main

import "testing"

func TestR283Augment391Chinese(t *testing.T) {
	item := augmentDescriptionFromRows(391, bundledAugmentDescriptions(), "bundled")
	if item.Status != "ok" || item.Description != "当白手起家被移除或被替换时，获得2500金币。" {
		t.Fatalf("missing reviewed Chinese description: %+v", item)
	}
}
