package main

import (
	"encoding/json"
	releaseinfo "lol-loot-assistant"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR288AutomaticUpdateDisableBlocksOnlyAutomatic(t *testing.T) {
	u := updateTestManager(t, []byte("fixture"))
	u.status.Supported = true
	u.status.State = "idle"
	var requests atomic.Int32
	manifest, _ := json.Marshal(updateTestManifest([]byte("fixture")))
	u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return updateResponse(200, manifest), nil
	})}
	automatic := false
	if err := u.SetSettings(updateSettings{Mirrors: []string{""}, Automatic: &automatic}); err != nil {
		t.Fatal(err)
	}
	if u.Check(false) || requests.Load() != 0 {
		t.Fatal("disabled startup/timer check made a request")
	}
	if !u.Check(true) {
		t.Fatal("manual check was disabled by automatic preference")
	}
	waitUpdateCheck(t, u)
	if requests.Load() == 0 {
		t.Fatal("manual check never reached transport")
	}
	if u.Settings().Automatic == nil || *u.Settings().Automatic {
		t.Fatal("automatic preference lost")
	}
	restored := newUpdateManager("0.11.2", u.store, nil)
	defer restored.Close()
	if restored.Settings().Automatic == nil || *restored.Settings().Automatic {
		t.Fatal("disabled preference did not survive restart")
	}

}
func TestR288ReleaseNotesAreCurrentBundledChangelogSection(t *testing.T) {
	raw, err := os.ReadFile("../desktop/package.json")
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		t.Fatal("invalid package version")
	}
	notes := releaseinfo.Notes(pkg.Version)
	if !strings.HasPrefix(notes, "## "+pkg.Version) || strings.Count(notes, "\n## ") != 0 {
		t.Fatalf("wrong version section: %q", notes)
	}
	if releaseinfo.Notes("999.0.0") != "" {
		t.Fatal("unknown version borrowed another release")
	}
}
