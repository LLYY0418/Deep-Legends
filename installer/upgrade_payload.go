package main

import "os"

type payloadRelease struct {
	setup, directory string
	err              error
}

// Releasing the payload touches only this installer's private TEMP, so it can
// overlap the parent wait. Always join it, including failed waits, to retain
// ownership of TEMP cleanup and avoid leaving a writer behind.
func releasePayloadWhileWaiting(wait func() bool, emit func(progressMessage), release func() (string, string, error), mark func(string)) (payloadRelease, bool) {
	done := make(chan payloadRelease, 1)
	go func() {
		setup, directory, err := release()
		done <- payloadRelease{setup, directory, err}
	}()
	ok := waitUpgradeParent(wait, emit)
	if ok {
		mark("parent_exited")
	}
	payload := <-done
	if !ok {
		if payload.directory != "" {
			_ = os.RemoveAll(payload.directory)
		}
		return payloadRelease{}, false
	}
	if payload.err == nil {
		// This milestone is the post-parent join: the payload is ready to use.
		// Its preceding interval measures only release time left after exit,
		// never subtracts an earlier background completion from parent exit.
		mark("payload_released")
	}
	return payload, true
}
