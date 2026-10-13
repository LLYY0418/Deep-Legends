package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func (a *app) registerUpdateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/update/status", a.authorized(func(w http.ResponseWriter, r *http.Request) { respondJSON(w, a.updates.Status()) }))
	for _, action := range []string{"check", "download", "cancel", "apply"} {
		mux.HandleFunc("POST /api/update/"+action, a.authorized(a.handleUpdateAction))
	}
	mux.HandleFunc("GET /api/update/settings", a.authorized(a.handleUpdateSettings))
	mux.HandleFunc("POST /api/update/settings", a.authorized(a.handleUpdateSettings))
}
func (a *app) handleUpdateAction(w http.ResponseWriter, r *http.Request) {
	if !licenseOnlineUpdates || a.updates == nil || !a.updates.Status().Supported {
		http.Error(w, "当前构建不支持更新", http.StatusBadRequest)
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/update/")
	var err error
	switch action {
	case "check":
		a.updates.Check(true)
	case "download":
		err = a.updates.Download()
	case "cancel":
		err = a.updates.Cancel()
	case "apply":
		if a.updates.Status().Portable {
			err = a.updates.ApplyAsync(func() { a.scheduleQuit(300*time.Millisecond, "update") })
		} else {
			err = a.updates.Apply()
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	respondJSON(w, a.updates.Status())
	if action == "apply" && !a.updates.Status().Portable {
		a.scheduleQuit(300*time.Millisecond, "update")
	}
}
func (a *app) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if !licenseOnlineUpdates {
		http.Error(w, "当前构建不支持更新", http.StatusBadRequest)
		return
	}
	if a.updates == nil {
		http.Error(w, "更新模块未初始化", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodPost {
		var settings updateSettings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&settings) != nil {
			http.Error(w, "更新线路格式不正确", http.StatusBadRequest)
			return
		}
		if err := a.updates.SetSettings(settings); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	respondJSON(w, a.updates.Settings())
}
func (a *app) broadcastUpdateEvent(kind string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	a.broadcastEvent(kind + "\n" + string(data))
}
func writeLiveEvent(w io.Writer, event string) error {
	kind, data, named := strings.Cut(event, "\n")
	if named && (kind == "update:progress" || kind == "update:status") {
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data)
		return err
	}
	_, err := fmt.Fprintf(w, "data: %s\n\n", event)
	return err
}
func (a *app) scheduleQuit(delay time.Duration, reason string) {
	a.quitOnce.Do(func() {
		if reason == "update" {
			fmt.Fprintln(os.Stdout, "LOOT_UPDATE_STARTED")
		}
		go func() {
			defer a.recoverPanic("update_http.scheduleQuit.1")

			time.Sleep(delay)
			if a.runtimeCancel != nil {
				a.runtimeCancel()
			}
			if a.runtimeMetricsDone != nil {
				<-a.runtimeMetricsDone
			}
			a.updates.Close()
			a.mu.Lock()
			client := a.lcu
			a.clearSnapshotLocked("软件正在退出")
			a.mu.Unlock()
			if client != nil {
				client.Close()
			}
			// Persist completed public details before the shell terminates us.
			// A slow disk gets a bounded grace period; atomic writes keep older
			// valid files intact if pending cache work cannot finish in time.
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
			a.closePublicCacheDisks(flushCtx)
			flushCancel()
			// Flush before announcing quit: the shell may terminate the backend
			// as soon as it receives the stdout protocol message.
			closeDiagnosticStore(a.storage)
			// Electron consumes this on its existing trusted backend stdout channel.
			fmt.Fprintf(os.Stdout, "LOOT_QUIT %s\n", reason)
			os.Exit(0)
		}()
	})
}

// Capture initialized owners only; shutdown must not create directories or
// hold application/cache locks while draining a slow filesystem.
func (a *app) closePublicCacheDisks(ctx context.Context) {
	owners := map[*championDataCache]bool{}
	add := func(caches ...*championDataCache) {
		for _, cache := range caches {
			if cache != nil {
				owners[cache] = true
			}
		}
	}
	if a.storage != nil {
		a.storage.mu.Lock()
		add(a.storage.auxiliaryDisk)
		a.storage.mu.Unlock()
	}
	a.mu.Lock()
	add(a.matchTierCache, a.profileIconImages)
	a.mu.Unlock()
	a.perkCatalogMu.Lock()
	add(a.perkCatalogDisk, a.clientCatalogCache)
	a.perkCatalogMu.Unlock()
	a.proProfiles.mu.Lock()
	add(a.proProfiles.disk)
	a.proProfiles.mu.Unlock()
	a.proPlayers.mu.Lock()
	add(a.proPlayers.disk)
	a.proPlayers.mu.Unlock()
	if a.champions != nil {
		add(a.champions.cache, a.champions.imageCache, a.champions.communityImageCache)
	}
	if a.mayhemRatings != nil {
		add(a.mayhemRatings.disk)
	}
	if a.riot != nil {
		providers, unlock := a.riot.lockCacheProviders()
		for _, p := range providers {
			add(p.matchDisk, p.identityDisk)
			p.specialistMu.Lock()
			add(p.starterCache)
			p.specialistMu.Unlock()
		}
		unlock()
	}
	for cache := range owners {
		_ = cache.closeDisk(ctx)
	}
}
