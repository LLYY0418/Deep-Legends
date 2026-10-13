package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

type overviewHeadProbeKey struct{}

func overviewHeadProbe(ctx context.Context) bool {
	v, _ := ctx.Value(overviewHeadProbeKey{}).(bool)
	return v
}
func (a *app) handleGameplayHistoryHead(w http.ResponseWriter, r *http.Request) {
	var request gameplayOverviewRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&request); err != nil {
		http.Error(w, "invalid head probe", 400)
		return
	}
	request.Count = 5
	request.BegIndex = 0
	request.Force = false
	fresh := true
	request.FreshHistory = &fresh
	raw, _ := json.Marshal(request)
	r = r.Clone(context.WithValue(r.Context(), overviewHeadProbeKey{}, true))
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.Header = r.Header.Clone()
	r.Header.Del("Accept")
	a.handleGameplayOverview(w, r)
}
