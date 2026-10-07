//go:build license

package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type licenseContextKey struct{}
type licenseAdmission struct {
	manager    *licenseManager
	business   context.Context
	generation uint64
}

func licenseContextValid(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	if bound, ok := ctx.Value(licenseContextKey{}).(licenseAdmission); ok {
		_, generation, allowed := bound.manager.admission()
		return allowed && bound.generation == generation && bound.business.Err() == nil
	}
	return true
}

func licensePublicRoute(r *http.Request) bool {
	if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /api/license/status", "POST /api/license/activate", "GET /api/privacy", "GET /api/diagnostics/log", "POST /api/diagnostics/client", "POST /api/diagnostics/startup", "POST /api/diagnostics/startup-stage", "POST /api/quit":
		return true
	}
	return licenseUpdateRoute(r)
}
func licenseUpdateRoute(r *http.Request) bool {
	// Update status/check/download/apply/settings are explicitly enumerated by
	// the updater; no arbitrary /api prefix becomes a business whitelist.
	switch r.Method + " " + r.URL.Path {
	case "GET /api/update/status", "POST /api/update/check", "POST /api/update/download", "POST /api/update/cancel", "POST /api/update/apply", "GET /api/update/settings", "POST /api/update/settings":
		return true
	}
	return false
}

type licenseResponseWriter struct {
	http.ResponseWriter
	manager    *licenseManager
	admission  context.Context
	generation uint64
}

func (w *licenseResponseWriter) valid() bool {
	_, generation, ok := w.manager.admission()
	return ok && generation == w.generation && w.admission.Err() == nil
}
func (w *licenseResponseWriter) WriteHeader(status int) {
	if w.valid() {
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *licenseResponseWriter) Write(data []byte) (int, error) {
	if !w.valid() {
		return 0, errLicenseLocked
	}
	return w.ResponseWriter.Write(data)
}
func (w *licenseResponseWriter) Flush() {
	if w.valid() {
		if f, ok := w.ResponseWriter.(http.Flusher); ok {
			f.Flush()
		}
	}
}
func (w *licenseResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (a *app) withLicenseProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if licensePublicRoute(r) {
			next.ServeHTTP(w, r)
			return
		}
		if a.license == nil {
			http.Error(w, "软件尚未激活", http.StatusForbidden)
			return
		}
		business, generation, ok := a.license.admission()
		if !ok {
			http.Error(w, a.license.Snapshot().Message, http.StatusForbidden)
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		ctx = context.WithValue(ctx, licenseContextKey{}, licenseAdmission{a.license, business, generation})
		stop := context.AfterFunc(business, cancel)
		defer stop()
		defer cancel()
		next.ServeHTTP(&licenseResponseWriter{w, a.license, business, generation}, r.WithContext(ctx))
	})
}

func (a *app) runLicensedBusiness(ctx context.Context) {
	var done chan struct{}
	var active context.Context
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	stop := func() {
		if active == nil {
			return
		}
		if a.watch != nil {
			a.watch.cancelLicenseTasks()
		}
		a.mu.RLock()
		client := a.lcu
		a.mu.RUnlock()
		if client != nil {
			a.disconnectClient(client, "软件授权已失效")
		}
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
			}
		}
		active = nil
		done = nil
	}
	defer stop()
	for {
		business, _, allowed := a.license.admission()
		if !allowed || ctx.Err() != nil {
			stop()
		} else if active != business {
			stop()
			active = business
			a.mu.Lock()
			a.proRefreshContext = business
			a.mu.Unlock()
			a.proPlayers.mu.Lock()
			a.proPlayers.refreshStarted = false
			a.proPlayers.mu.Unlock()
			done = make(chan struct{})
			finished := done
			a.goSafe("license_business", func() { defer close(finished); a.runConnectionManager(business) })
			a.warmProPlayersCaches()
			a.refreshFeatureGates(business)
		}
		select {
		case <-ctx.Done():
			return
		case <-a.license.changed:
		case <-ticker.C:
		}
	}
}
func (r *watchRunner) cancelLicenseTasks() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writeCancel != nil {
		r.writeCancel()
		r.writeContext = nil
		r.writeCancel = nil
	}
	for key, pending := range r.pending {
		pending.cancel()
		delete(r.pending, key)
	}
	r.resetChampSelectRuntimeLocked()
	r.autoMatchInFlight = false
	r.deferredPlayAgain = false
	r.autoMatchStarted = false
	r.acceptedForReadyCheck = false
}
func (c *LCUClient) licenseRequestContext(ctx context.Context) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return ctx, func() {}, err
	}
	if !licenseContextValid(ctx) {
		return ctx, func() {}, errLicenseLocked
	}
	if c.license == nil {
		return ctx, func() {}, nil
	} // reusable transport also used by isolated tools/tests; main always attaches its manager
	business, generation, allowed := c.license.admission()
	if !allowed {
		return ctx, func() {}, errLicenseLocked
	}
	request, cancel := context.WithCancel(ctx)
	request = context.WithValue(request, licenseContextKey{}, licenseAdmission{c.license, business, generation})
	stop := context.AfterFunc(business, cancel)
	if business.Err() != nil {
		stop()
		cancel()
		return request, func() {}, errLicenseLocked
	}
	return request, func() { stop(); cancel() }, nil
}
func (c *LCUClient) licenseResult(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !licenseContextValid(ctx) {
		return errLicenseLocked
	}
	if c.license != nil && !c.license.Allowed() {
		return errLicenseLocked
	}
	return nil
}
func (a *app) licensedDiscovery() (*LCUClient, LCUDiscoveryStatus, error) {
	if a.license == nil || !a.license.Allowed() {
		return nil, LCUDiscoveryStatus{}, errLicenseLocked
	}
	client, status, err := a.discoverColdLCU()
	if client != nil {
		client.license = a.license
		if !a.license.Allowed() {
			client.Close()
			return nil, status, errLicenseLocked
		}
	}
	return client, status, err
}
func (a *app) proBusinessContext() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.proRefreshContext
}

// HTTP admission is not enough for work queued behind a lock or disk scan.
// Check again immediately before the local side effect. Standalone handler
// fixtures have no manager; the production router always requires one.
func (a *app) licenseSideEffect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !licenseContextValid(ctx) {
		return errLicenseLocked
	}
	if a != nil && a.license != nil && !a.license.Allowed() {
		return errLicenseLocked
	}
	return nil
}

func (a *app) licenseBusinessContext() context.Context {
	if a.license == nil {
		return context.Background()
	}
	ctx, _, _ := a.license.admission()
	return ctx
}

func (a *app) startApplicationBusiness(ctx context.Context) {
	a.license = productionLicenseManager(a.recordDiagnostic)
	goSafe("license_manager", func() { a.license.Run(ctx) })
	goSafe("license_business", func() { a.runLicensedBusiness(ctx) })
}
