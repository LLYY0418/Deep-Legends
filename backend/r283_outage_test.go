package main

import (
	"context"
	"errors"
	"testing"
)

func TestR283LiveOfficialOutage(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{{"503", &sgpHTTPError{StatusCode: 503}, true}, {"cooling", errSGPHistoryCooling, true}, {"timeout", context.DeadlineExceeded, false}, {"private", &sgpHTTPError{StatusCode: 403}, false}, {"empty", nil, false}, {"other", errors.New("network"), false}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := liveOfficialHistoryFailure(tc.err); got != tc.want {
				t.Fatalf("down=%v want %v", got, tc.want)
			}
		})
	}
}
