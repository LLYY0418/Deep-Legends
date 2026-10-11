package main

import (
	"context"
	"errors"
)

// A background leader may be parked in the foreground recovery window. Live
// foreground followers cancel that leader and retry with their own context.
// The background caller receives the foreground result after the handoff,
// rather than surfacing the internal cancellation or racing to lead again.
type riotFlightPriority struct {
	cancel   context.CancelCauseFunc
	done     chan struct{}
	promoted bool // guarded by the enclosing cache/flight mutex
}
type riotFlightHandoff struct{ done <-chan struct{} }

func (*riotFlightHandoff) Error() string { return "Riot background request yielded to foreground" }
func (*riotFlightHandoff) Unwrap() error { return context.Canceled }

type riotIdentityFlightPriorityKey struct{}

func newRiotFlightPriority(ctx context.Context) (context.Context, *riotFlightPriority) {
	if !isRiotBackground(ctx) {
		return ctx, nil
	}
	bounded, cancel := context.WithCancelCause(ctx)
	return bounded, &riotFlightPriority{cancel: cancel, done: make(chan struct{})}
}
func promoteRiotFlight(ctx context.Context, priority *riotFlightPriority) bool {
	if priority == nil || priority.promoted || isRiotBackground(ctx) || ctx.Err() != nil {
		return false
	}
	priority.promoted = true
	priority.cancel(&riotFlightHandoff{done: priority.done})
	return true
}
func finishRiotFlight(priority *riotFlightPriority) {
	if priority != nil {
		priority.cancel(nil)
	}
}
func riotFlightResultError(ctx context.Context, err error) error {
	var handoff *riotFlightHandoff
	if errors.As(context.Cause(ctx), &handoff) {
		return handoff
	}
	return err
}
func awaitRiotFlightHandoff(ctx context.Context, err error) (bool, error) {
	var handoff *riotFlightHandoff
	if ctx.Err() != nil || !errors.As(err, &handoff) {
		return false, nil
	}
	select {
	case <-handoff.done:
		return true, nil
	case <-ctx.Done():
		return true, ctx.Err()
	}
}
