package main

import (
	"context"
	"os"
	"strings"
	"time"
)

func (a *app) refreshFeatureGates(business context.Context) {
	if gateURL := strings.TrimSpace(os.Getenv("DEEP_LEGENDS_FEATURE_GATES_URL")); gateURL != "" {
		go func() {
			defer recoverPanic("main.main.1")

			ctx, cancel := context.WithTimeout(business, 8*time.Second)
			defer cancel()
			if gateErr := a.champions.featureGates.refresh(ctx, a.champions.httpClient(), gateURL); gateErr != nil {
				if a.champions.diag != nil {
					a.champions.diag(map[string]any{"event": "feature_gates_failed", "reason": safeDiagnosticReason(gateErr)})
				}
				return
			}
			if a.champions.diag != nil {
				a.champions.diag(map[string]any{"event": "feature_gates_loaded"})
			}
		}()
	}
}
