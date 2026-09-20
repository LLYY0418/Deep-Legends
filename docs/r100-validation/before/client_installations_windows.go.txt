//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func detectClientInstallationsWithScan() ([]clientInstallation, clientInstallationScan) {
	report := clientInstallationScan{PlatformSupported: true}
	gameRoots := make([]string, 0, 28)
	for _, registryRoot := range queryRegistryValues(`HKCU\Software\Tencent\LOL`, "InstallPath") {
		report.RegistryRootFound = true
		report.RegistryLauncherFound = report.RegistryLauncherFound || regularFile(filepath.Join(registryRoot, "Launcher", "Client.exe"))
		report.RegistryTCLSFound = report.RegistryTCLSFound || regularFile(filepath.Join(registryRoot, "TCLS", "Client.exe"))
		report.RegistryLeagueClientFound = report.RegistryLeagueClientFound || regularFile(filepath.Join(registryRoot, "LeagueClient", "LeagueClient.exe")) || regularFile(filepath.Join(registryRoot, "LeagueClient.exe"))
		gameRoots = append(gameRoots, registryRoot)
	}
	for drive := 'C'; drive <= 'Z'; drive++ {
		gameRoots = append(gameRoots, string(drive)+`:\WeGameApps\英雄联盟`)
	}
	shortcuts := detectClientShortcuts()
	report.ShortcutCandidates = len(shortcuts)
	return buildDetectedClientInstallations(uniquePaths(gameRoots), riotClientCandidates(), shortcuts, regularFile), report
}

func launchClientInstallation(installation clientInstallation) (clientLaunchResult, error) {
	return launchClientCandidates(installation, launchWindowsClientCandidate)
}

func launchWindowsClientCandidate(candidate clientLaunchCandidate) (clientLaunchFailure, error) {
	failure := clientLaunchFailure{Source: candidate.Source}
	path := candidate.executable
	if candidate.shortcut != "" {
		path = candidate.shortcut
	}
	if path == "" || !filepath.IsAbs(path) || !regularFile(path) {
		failure.Stage = "validate"
		return failure, errors.New("client launch candidate is unavailable")
	}
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		failure.Stage = "encode"
		return failure, err
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		failure.Stage = "encode"
		return failure, err
	}
	cwd, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		failure.Stage = "encode"
		return failure, err
	}
	var parameters *uint16
	if len(candidate.arguments) != 0 {
		parts := make([]string, len(candidate.arguments))
		for index, argument := range candidate.arguments {
			parts[index] = windows.EscapeArg(argument)
		}
		parameters, err = windows.UTF16PtrFromString(strings.Join(parts, " "))
		if err != nil {
			failure.Stage = "encode"
			return failure, err
		}
	}
	if err := windows.ShellExecute(0, verb, file, parameters, cwd, windows.SW_SHOWNORMAL); err != nil {
		failure.Stage = "shell-execute"
		failure.ErrorCode = windowsErrorCode(err)
		return failure, err
	}
	return failure, nil
}

func windowsErrorCode(err error) uint32 {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return uint32(errno)
	}
	return 0
}

func detectClientShortcuts() []clientInstallation {
	roots := make([]string, 0, 4)
	for _, candidate := range []struct{ env, suffix string }{
		{"USERPROFILE", "Desktop"},
		{"PUBLIC", "Desktop"},
		{"APPDATA", filepath.Join("Microsoft", "Windows", "Start Menu", "Programs")},
		{"PROGRAMDATA", filepath.Join("Microsoft", "Windows", "Start Menu", "Programs")},
	} {
		if base := strings.TrimSpace(os.Getenv(candidate.env)); base != "" {
			roots = append(roots, filepath.Join(base, candidate.suffix))
		}
	}
	roots = uniquePaths(roots)
	result := make([]clientInstallation, 0, 8)
	for _, root := range roots {
		scanClientShortcutDirectory(root, 0, &result)
	}
	return result
}

func scanClientShortcutDirectory(root string, depth int, result *[]clientInstallation) {
	if root == "" || depth > 4 {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			scanClientShortcutDirectory(path, depth+1, result)
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".lnk") {
			continue
		}
		id, name, kind, description := classifyClientShortcut(entry.Name())
		if id == "" {
			continue
		}
		*result = append(*result, clientInstallation{ID: id, Name: name, Kind: kind, Description: description, shortcut: path})
	}
}

func queryRegistryValues(path, name string) []string {
	root, subkey, ok := splitRegistryPath(path)
	if !ok {
		return nil
	}
	values := make([]string, 0, 2)
	seen := make(map[string]bool)
	for _, access := range []uint32{
		registry.QUERY_VALUE | registry.WOW64_64KEY,
		registry.QUERY_VALUE | registry.WOW64_32KEY,
		registry.QUERY_VALUE,
	} {
		key, err := registry.OpenKey(root, subkey, access)
		if err != nil {
			continue
		}
		value, valueType, err := key.GetStringValue(name)
		key.Close()
		if err == nil && valueType == registry.EXPAND_SZ {
			if expanded, expandErr := registry.ExpandString(value); expandErr == nil {
				value = expanded
			}
		}
		if err == nil && strings.TrimSpace(value) != "" {
			value = strings.TrimSpace(value)
			lookup := strings.ToLower(value)
			if !seen[lookup] {
				seen[lookup] = true
				values = append(values, value)
			}
		}
	}
	return values
}

func splitRegistryPath(path string) (registry.Key, string, bool) {
	path = strings.TrimSpace(strings.ReplaceAll(path, "/", `\`))
	separator := strings.Index(path, `\`)
	if separator <= 0 || separator == len(path)-1 {
		return 0, "", false
	}
	prefix, subkey := strings.ToUpper(path[:separator]), path[separator+1:]
	switch prefix {
	case "HKCU", "HKEY_CURRENT_USER":
		return registry.CURRENT_USER, subkey, true
	case "HKLM", "HKEY_LOCAL_MACHINE":
		return registry.LOCAL_MACHINE, subkey, true
	default:
		return 0, "", false
	}
}

func riotClientCandidates() []string {
	values := make([]string, 0, 8)
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	data, err := os.ReadFile(filepath.Join(programData, "Riot Games", "RiotClientInstalls.json"))
	if err == nil {
		var payload any
		if json.Unmarshal(data, &payload) == nil {
			collectRiotExecutables(payload, &values)
		}
	}
	for _, drive := range []string{"C:", "D:", "E:", "F:"} {
		values = append(values, filepath.Join(drive+string(filepath.Separator), "Riot Games", "Riot Client", "RiotClientServices.exe"))
	}
	return uniquePaths(values)
}

func collectRiotExecutables(value any, result *[]string) {
	switch typed := value.(type) {
	case string:
		candidate := filepath.Clean(strings.Trim(typed, ` "`))
		if strings.EqualFold(filepath.Base(candidate), "RiotClientServices.exe") {
			*result = append(*result, candidate)
		}
	case []any:
		for _, item := range typed {
			collectRiotExecutables(item, result)
		}
	case map[string]any:
		for key, item := range typed {
			collectRiotExecutables(key, result)
			collectRiotExecutables(item, result)
		}
	}
}

func uniquePaths(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(os.ExpandEnv(value))
		if value == "" {
			continue
		}
		clean := filepath.Clean(value)
		key := strings.ToLower(clean)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, clean)
	}
	return result
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
