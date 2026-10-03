package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func updateRootForExecutable(executable string) string {
	directory := filepath.Dir(executable)
	if strings.EqualFold(filepath.Base(directory), "backend") && strings.EqualFold(filepath.Base(filepath.Dir(directory)), "app.asar.unpacked") && strings.EqualFold(filepath.Base(filepath.Dir(filepath.Dir(directory))), "resources") {
		return filepath.Dir(filepath.Dir(filepath.Dir(directory)))
	}
	return directory
}
func updateInstallationMatches(root, displayName, location string, uninstallExists bool) bool {
	return uninstallExists && displayName == "Deep Legends" && updatePathsMatch(root, location, normalizeUpdateInstallationPath)
}

func trimUpdateInstallationPath(value string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(value), `"'`))
}

func normalizeUpdateInstallationPath(value string) string {
	return normalizeUpdateInstallationPathWith(value, updateLongPathName)
}

func normalizeUpdateInstallationPathWith(value string, longPath func(string) (string, error)) string {
	value = trimUpdateInstallationPath(value)
	if value == "" {
		return ""
	}
	value = filepath.Clean(value)
	if expanded, err := longPath(value); err == nil && expanded != "" {
		value = filepath.Clean(expanded)
	}
	if resolved, err := filepath.EvalSymlinks(value); err == nil {
		value = filepath.Clean(resolved)
	}
	return value
}

func updatePathsMatch(left, right string, normalize func(string) string) bool {
	left, right = normalize(left), normalize(right)
	return left != "" && right != "" && strings.EqualFold(left, right)
}
func quoteUpdateArgument(value string) string {
	// Always quote; double trailing backslashes and those preceding a quote for
	// the Windows CRT parser, including installation at a drive root.
	var out strings.Builder
	out.WriteByte('"')
	slashes := 0
	for _, c := range value {
		if c == '\\' {
			slashes++
			continue
		}
		if c == '"' {
			out.WriteString(strings.Repeat("\\", slashes*2+1))
		} else {
			out.WriteString(strings.Repeat("\\", slashes))
		}
		slashes = 0
		out.WriteRune(c)
	}
	out.WriteString(strings.Repeat("\\", slashes*2))
	out.WriteByte('"')
	return out.String()
}
func updateParentPID() int {
	if value, err := strconv.Atoi(os.Getenv("LOOT_DESKTOP_PID")); err == nil && value > 0 {
		return value
	}
	return os.Getpid()
}
func updateCommandLine(setup, destination string, parent ...int) string {
	pid := updateParentPID()
	if len(parent) > 0 {
		pid = parent[0]
	}
	return quoteUpdateArgument(setup) + " --update --parent-first --parent-pid " + strconv.Itoa(pid) + " --dest " + quoteUpdateArgument(destination)
}

func portableUpdateCommandLine(setup, destination string, parent int) string {
	return updateCommandLine(setup, destination, parent) + " --fresh-install"
}
