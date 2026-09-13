package main

import (
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
	if a.updates == nil || !a.updates.Status().Supported {
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
		err = a.updates.Apply()
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	respondJSON(w, a.updates.Status())
	if action == "apply" {
		a.scheduleQuit(300*time.Millisecond, "update")
	}
}
func (a *app) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
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
		go func() {
			time.Sleep(delay)
			if a.runtimeCancel != nil {
				a.runtimeCancel()
			}
			a.updates.Close()
			a.mu.Lock()
			client := a.lcu
			a.lcu = nil
			a.mu.Unlock()
			if client != nil {
				client.Close()
			}
			// Electron consumes this on its existing trusted backend stdout channel.
			fmt.Fprintf(os.Stdout, "LOOT_QUIT %s\n", reason)
			os.Exit(0)
		}()
	})
}
