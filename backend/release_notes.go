package main

import (
	releaseinfo "lol-loot-assistant"
	"net/http"
)

func (a *app) handleReleaseNotes(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{"version": version, "notes": releaseinfo.Notes(version)})
}
