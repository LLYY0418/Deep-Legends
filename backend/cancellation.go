package main

import (
	"context"
	"errors"
)

// isCancellation intentionally excludes DeadlineExceeded: a caller leaving a
// page is silent cancellation, while an upstream timeout is a real failure.
func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}
