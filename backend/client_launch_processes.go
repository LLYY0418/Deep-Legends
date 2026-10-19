package main

import "net/http"

type clientLauncherProcesses struct {
	Known         bool            `json:"known"`
	Launchers     map[string]bool `json:"launchers"`
	LeagueRunning bool            `json:"leagueRunning"`
}

func (a *app) handleClientLaunchStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, nativeClientLauncherProcesses(a.detectedClientInstallations()))
}
