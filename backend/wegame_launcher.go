package main

import (
	"path/filepath"
	"strings"
)

func wegameRegistryRoots(query func(string, string) []string) []string {
	var roots []string
	for _, key := range []string{`HKLM\SOFTWARE\Tencent\WeGame`, `HKCU\SOFTWARE\Tencent\WeGame`} {
		roots = append(roots, query(key, "InstallPath")...)
	}
	seen := map[string]bool{}
	var out []string
	for _, root := range roots {
		key := strings.ToLower(filepath.Clean(root))
		if root != "" && !seen[key] {
			seen[key] = true
			out = append(out, root)
		}
	}
	return out
}
func wegameRegistryInstallations(roots []string, regular func(string) bool) []clientInstallation {
	var items []clientInstallation
	for _, root := range roots {
		path := filepath.Join(root, "WeGame.exe")
		if filepath.IsAbs(path) && regular(path) {
			items = append(items, clientInstallation{ID: "wegame", Name: "WeGame", Kind: "wegame", Available: true, executable: path})
		}
	}
	return items
}
