package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR195InstallationPathNormalization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-user", "Deep Legends")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, location := range []string{`  "` + root + `"  `, root + string(filepath.Separator), strings.ToUpper(root)} {
		if !updateInstallationMatches(root, "Deep Legends", location, true) {
			t.Fatalf("location not normalized: %q", location)
		}
	}
	short := filepath.Join(filepath.Dir(root), "DEEPLE~1")
	var conversions int
	normalize := func(value string) string {
		return normalizeUpdateInstallationPathWith(value, func(value string) (string, error) {
			conversions++
			if value == short {
				return root, nil
			}
			return "", errors.New("not expanded")
		})
	}
	d := classifyUpdateInstallationWith(short, false, true, []updateRegistryInstallation{{"Deep Legends", root}}, "", normalize)
	if d.Result != "installed" || !d.MatchAfterNormalize || d.LocationMatches || d.directory != normalize(root) || conversions == 0 {
		t.Fatalf("short-path detection: %+v", d)
	}
	// Failed long-path and symlink expansion must retain the original path.
	missing := filepath.Join(t.TempDir(), "missing")
	if got := normalize(missing); got != missing {
		t.Fatalf("failed expansion changed path: %q", got)
	}
	if updateInstallationMatches(root, "Deep Legends", "", true) || updateInstallationMatches("", "Deep Legends", "", true) {
		t.Fatal("empty path matched")
	}
}

func TestR195InstallationSymlinkNormalization(t *testing.T) {
	base := t.TempDir()
	root, link := filepath.Join(base, "installed"), filepath.Join(base, "linked")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, link); err != nil {
		t.Skip("symlink unavailable on this host")
	}
	if !updateInstallationMatches(link, "Deep Legends", root, true) {
		t.Fatal("symlink root not resolved")
	}
}

func TestR195InstallationDefaultFallbackAndDiagnostics(t *testing.T) {
	base := t.TempDir()
	defaultDir := filepath.Join(base, "private-user", "Programs", "Deep Legends")
	other := filepath.Join(base, "old-private-user", "Deep Legends")
	if err := os.MkdirAll(defaultDir, 0700); err != nil {
		t.Fatal(err)
	}
	quoted := ` "` + defaultDir + string(filepath.Separator) + `" `
	d := classifyUpdateInstallationWith(defaultDir, false, true, []updateRegistryInstallation{{"Other", other}, {"Deep Legends", quoted}, {"Deep Legends", other}}, defaultDir, normalizeUpdateInstallationPath)
	if d.Result != "installed" || d.RegistryEntries != 2 || !d.LocationQuoted || !d.LocationTrailingSep || !d.LocationExists || !d.RootIsDefaultDir || !d.LocationIsDefaultDir || !d.MatchAfterNormalize {
		t.Fatalf("diagnostic flags: %+v", d)
	}
	for _, tc := range []struct {
		root                string
		portable, uninstall bool
		result              string
	}{
		{defaultDir, false, true, "installed"},
		{defaultDir, false, false, "no_uninstaller"},
		{defaultDir, true, true, "portable_env"},
		{filepath.Join(base, "custom-install"), false, true, "registry_location_mismatch"},
	} {
		got := classifyUpdateInstallationWith(tc.root, tc.portable, tc.uninstall, []updateRegistryInstallation{{"Deep Legends", other}}, defaultDir, normalizeUpdateInstallationPath)
		if got.Result != tc.result || got.MatchAfterNormalize {
			t.Fatalf("fallback gate: %+v", got)
		}
		if got.Result == "installed" && got.directory != normalizeUpdateInstallationPath(defaultDir) {
			t.Fatal("wrong installed destination")
		}
	}
	// The startup sender uses this exact fixed-field projection.
	event := d.diagnosticEvent()
	expected := map[string]bool{"event": true, "result": true, "registry_display_found": true, "location_matches": true, "registry_entries": true, "location_quoted": true, "location_trailing_sep": true, "location_exists": true, "root_is_default_dir": true, "location_is_default_dir": true, "match_after_normalize": true}
	if len(event) != len(expected) {
		t.Fatalf("unexpected diagnostic fields: %+v", event)
	}
	for key, value := range event {
		if !expected[key] {
			t.Fatalf("unexpected field %s", key)
		}
		if key != "event" && key != "result" && key != "registry_entries" {
			if _, ok := value.(bool); !ok {
				t.Fatalf("nonboolean field %s", key)
			}
		}
	}
	for _, value := range []any{d, event} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{base, "private-user", "old-private-user", "Programs", `\\`} {
			if strings.Contains(string(data), private) {
				t.Fatalf("path leaked: %s", data)
			}
		}
	}
}
