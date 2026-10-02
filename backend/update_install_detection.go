package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type updateRegistryInstallation struct{ DisplayName, Location string }
type updateInstallDetection struct {
	Result               string `json:"result"`
	RegistryDisplayFound bool   `json:"registry_display_found"`
	LocationMatches      bool   `json:"location_matches"`
	RegistryEntries      int    `json:"registry_entries"`
	LocationQuoted       bool   `json:"location_quoted"`
	LocationTrailingSep  bool   `json:"location_trailing_sep"`
	LocationExists       bool   `json:"location_exists"`
	RootIsDefaultDir     bool   `json:"root_is_default_dir"`
	LocationIsDefaultDir bool   `json:"location_is_default_dir"`
	MatchAfterNormalize  bool   `json:"match_after_normalize"`
	directory            string
}

func classifyUpdateInstallation(root string, portable, uninstall bool, entries []updateRegistryInstallation) updateInstallDetection {
	defaultDirectory, _ := defaultPortableInstallDirectory()
	return classifyUpdateInstallationWith(root, portable, uninstall, entries, defaultDirectory, normalizeUpdateInstallationPath)
}

func classifyUpdateInstallationWith(root string, portable, uninstall bool, entries []updateRegistryInstallation, defaultDirectory string, normalize func(string) string) updateInstallDetection {
	d := updateInstallDetection{Result: "registry_missing"}
	d.RootIsDefaultDir = updatePathsMatch(root, defaultDirectory, normalize)
	for _, entry := range entries {
		if entry.DisplayName != "Deep Legends" {
			continue
		}
		d.RegistryDisplayFound = true
		d.RegistryEntries++
		location := strings.TrimSpace(entry.Location)
		d.LocationQuoted = d.LocationQuoted || strings.HasPrefix(location, `"`) || strings.HasSuffix(location, `"`) || strings.HasPrefix(location, "'") || strings.HasSuffix(location, "'")
		location = trimUpdateInstallationPath(location)
		d.LocationTrailingSep = d.LocationTrailingSep || strings.HasSuffix(location, "/") || strings.HasSuffix(location, `\`)
		if info, err := os.Stat(normalize(location)); err == nil && info.IsDir() {
			d.LocationExists = true
		}
		d.LocationIsDefaultDir = d.LocationIsDefaultDir || updatePathsMatch(entry.Location, defaultDirectory, normalize)
		if root != "" && entry.Location != "" && strings.EqualFold(filepath.Clean(root), filepath.Clean(entry.Location)) {
			d.LocationMatches = true
		}
		d.MatchAfterNormalize = d.MatchAfterNormalize || updatePathsMatch(root, entry.Location, normalize)
	}
	switch {
	case portable:
		d.Result = "portable_env"
	case !uninstall:
		d.Result = "no_uninstaller"
	case d.MatchAfterNormalize || d.RootIsDefaultDir:
		d.Result = "installed"
		d.directory = normalize(root)
	case d.RegistryDisplayFound:
		d.Result = "registry_location_mismatch"
	}
	return d
}

func (d updateInstallDetection) diagnosticEvent() map[string]any {
	// Only fixed reasons, counts and booleans; installation paths stay private.
	return map[string]any{"event": "update_install_detection", "result": d.Result,
		"registry_display_found": d.RegistryDisplayFound, "location_matches": d.LocationMatches,
		"registry_entries": d.RegistryEntries, "location_quoted": d.LocationQuoted,
		"location_trailing_sep": d.LocationTrailingSep, "location_exists": d.LocationExists,
		"root_is_default_dir": d.RootIsDefaultDir, "location_is_default_dir": d.LocationIsDefaultDir,
		"match_after_normalize": d.MatchAfterNormalize}
}
func portableUpdateEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, value := range env {
		name, _, _ := strings.Cut(value, "=")
		switch strings.ToUpper(name) {
		case "PORTABLE_EXECUTABLE_FILE", "PORTABLE_EXECUTABLE_DIR", "LOL_LOOT_DATA_DIR":
			continue
		}
		out = append(out, value)
	}
	return out
}

func defaultPortableInstallDirectory() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" || !filepath.IsAbs(base) {
		return "", errors.New("无法定位当前用户安装目录")
	}
	return filepath.Join(base, "Programs", "Deep Legends"), nil
}
