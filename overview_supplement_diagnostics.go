package main

import (
	"context"
	"errors"
)

// Never log arbitrary upstream errors: they may contain a player's identity or
// request URL. Each caller supplies its stable pipeline stage separately.
func supplementFailureCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	switch err.Error() {
	case "opgg-profile-identity", "opgg-profile-redirect", "opgg-flight-reference", "opgg-flight-limit":
		return err.Error()
	case "opgg-current-json", "opgg-current-required-fields", "opgg-current-time", "opgg-current-roster-size", "opgg-current-roster-identity", "opgg-current-target-mismatch", "opgg-current-champion-identity":
		return err.Error()
	}
	return "source-or-validation-failed"
}
