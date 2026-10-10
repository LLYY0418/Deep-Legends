package main

import (
	"context"
	"net/http"
	"time"
)

type proBackgroundGateKey struct{}
type proRefreshOwnerKey struct{}

func (a *app) proBackgroundContext(ctx context.Context) context.Context {
	if a.riot == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, proBackgroundGateKey{}, a.riot)
	return context.WithValue(ctx, proRefreshOwnerKey{}, a)
}
func waitProBackground(ctx context.Context) error {
	if provider, _ := ctx.Value(proBackgroundGateKey{}).(*riotProvider); provider != nil {
		return provider.waitForRiotForeground(ctx)
	}
	return ctx.Err()
}
func (p *riotProvider) proDirectoryHidden() bool {
	if p == nil || p.foreground == nil {
		return false
	}
	p.foreground.Lock()
	defer p.foreground.Unlock()
	return p.foreground.visibilityKnown && !p.foreground.directoryVisible
}
func (a *app) handleProVisibility(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Visible *bool `json:"visible"`
	}
	if decodeJSONRequest(r, &body, 128) != nil || body.Visible == nil {
		http.Error(w, "invalid visibility", 400)
		return
	}
	if a.riot != nil && a.riot.foreground != nil {
		s := a.riot.foreground
		s.Lock()
		s.visibilityKnown = true
		s.directoryVisible = *body.Visible
		s.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}

// Admission is per account and supplement, so failures do not restart a request every minute.
func (a *app) admitProAccountRefresh(key string, now time.Time) bool {
	c := &a.proPlayers
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refreshAttempted == nil {
		c.refreshAttempted = map[string]time.Time{}
	}
	if at := c.refreshAttempted[key]; !at.IsZero() && now.Sub(at) < 10*time.Minute {
		return false
	}
	c.refreshAttempted[key] = now
	return true
}
