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
	directory            string
}

func classifyUpdateInstallation(root string, portable, uninstall bool, entries []updateRegistryInstallation) updateInstallDetection {
	d := updateInstallDetection{Result: "registry_missing"}
	for _, entry := range entries {
		if entry.DisplayName != "Deep Legends" {
			continue
		}
		d.RegistryDisplayFound = true
		if updateInstallationMatches(root, entry.DisplayName, entry.Location, true) {
			d.LocationMatches = true
		}
	}
	switch {
	case portable:
		d.Result = "portable_env"
	case !uninstall:
		d.Result = "no_uninstaller"
	case d.LocationMatches:
		d.Result = "installed"
		d.directory = root
	case d.RegistryDisplayFound:
		d.Result = "registry_location_mismatch"
	}
	return d
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
