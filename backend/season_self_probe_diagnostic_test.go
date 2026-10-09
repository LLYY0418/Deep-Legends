package main

import (
	"errors"
	"testing"
)

func TestSeasonSelfProbeDiagnosticOmitsReasonOnSuccess(t *testing.T) {
	ok := seasonSelfProbeDiagnostic("career", 0, 3, 2, 1775889886204, 512, nil)
	if ok["failed"] != false {
		t.Fatalf("successful probe marked failed: %#v", ok)
	}
	if _, present := ok["reason"]; present {
		t.Fatalf("successful probe must not carry a failure reason: %#v", ok)
	}
	if ok["event"] != "season_self_source_probe" || ok["source"] != "career" || ok["matches"] != 3 || ok["added"] != 2 || ok["oldest_created_at"] != int64(1775889886204) || ok["bytes"] != 512 {
		t.Fatalf("probe fields changed: %#v", ok)
	}
	failed := seasonSelfProbeDiagnostic("history-1000", 3, 0, 0, 0, 0, errors.New("HTTP 404"))
	if failed["failed"] != true || failed["reason"] != "HTTP 404" {
		t.Fatalf("failed probe must keep its reason: %#v", failed)
	}
}
