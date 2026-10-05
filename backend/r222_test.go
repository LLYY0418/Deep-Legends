package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func r222OfflineChampions() *championProvider {
	p := newChampionProvider()
	p.cache = newChampionDataCache(nil)
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return updateResponse(http.StatusNotFound, []byte(`{}`)), nil
	})}
	return p
}

func TestR222ArenaTruthRetryCancellation(t *testing.T) {
	// Run the production default wait too: cancellation cannot be bypassed.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitArenaTruthRetry(ctx, time.Second); err == nil {
		t.Fatal("canceled retry was accepted")
	}
}
