//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	createDetachedProcess = 0x00000008
	createNewProcessGroup = 0x00000200
)

func detectClientInstallations() []clientInstallation {
	byID := make(map[string]clientInstallation)
	add := func(installation clientInstallation) {
		validShortcut := installation.shortcut != "" && filepath.IsAbs(installation.shortcut) && regularFile(installation.shortcut)
		validExecutable := installation.executable != "" && regularFile(installation.executable)
		if installation.ID == "" || (!validShortcut && !validExecutable) {
			return
		}
		installation.Available = true
		if validShortcut {
			installation.Location = "Windows 快捷方式 · " + strings.TrimSuffix(filepath.Base(installation.shortcut), filepath.Ext(installation.shortcut))
		} else if installation.Location == "" {
			installation.Location = filepath.Dir(installation.executable)
		}
		existing, exists := byID[installation.ID]
		if !exists || (validShortcut && existing.shortcut == "") {
			byID[installation.ID] = installation
		}
	}

	for _, installation := range detectClientShortcuts() {
		add(installation)
	}

	gameRoots := make([]string, 0, 28)
	if value := queryRegistryValue(`HKCU\Software\Tencent\LOL`, "InstallPath"); value != "" {
		gameRoots = append(gameRoots, value)
	}
	for drive := 'C'; drive <= 'Z'; drive++ {
		gameRoots = append(gameRoots, string(drive)+`:\WeGameApps\英雄联盟`)
	}
	for _, root := range uniquePaths(gameRoots) {
		add(clientInstallation{ID: "tcls", Name: "TCLS 客户端", Kind: "tcls", Description: "直接启动腾讯英雄联盟客户端", executable: filepath.Join(root, "Launcher", "Client.exe")})
		add(clientInstallation{ID: "wegame-lol", Name: "WeGame 英雄联盟", Kind: "wegame", Description: "通过当前英雄联盟安装目录启动 WeGame", executable: filepath.Join(root, "WeGameLauncher", "launcher.exe")})
	}

	wegameCandidates := []string{parseExecutableValue(queryRegistryDefault(`HKCU\wegame\DefaultIcon`))}
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
		if root == "" {
			continue
		}
		wegameCandidates = append(wegameCandidates,
			filepath.Join(root, "WeGame", "wegame.exe"),
			filepath.Join(root, "Tencent", "WeGame", "wegame.exe"),
		)
	}
	for _, candidate := range uniquePaths(wegameCandidates) {
		add(clientInstallation{ID: "wegame", Name: "WeGame", Kind: "wegame", Description: "打开 WeGame 后从游戏库启动英雄联盟", executable: candidate})
	}

	for _, candidate := range riotClientCandidates() {
		add(clientInstallation{ID: "riot", Name: "Riot 客户端", Kind: "riot", Description: "启动 Riot 英雄联盟客户端", executable: candidate, arguments: []string{"--launch-product=league_of_legends", "--launch-patchline=live"}})
	}

	order := map[string]int{"tcls": 0, "wegame-lol": 1, "wegame": 2, "riot": 3}
	result := make([]clientInstallation, 0, len(byID))
	for _, installation := range byID {
		result = append(result, installation)
	}
	sort.Slice(result, func(left, right int) bool { return order[result[left].ID] < order[result[right].ID] })
	return result
}

func launchClientInstallation(installation clientInstallation) error {
	if installation.shortcut != "" {
		if !filepath.IsAbs(installation.shortcut) || !regularFile(installation.shortcut) {
			return errors.New("client shortcut is unavailable")
		}
		cmd := exec.Command("explorer.exe", installation.shortcut)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createDetachedProcess | createNewProcessGroup}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
	if !regularFile(installation.executable) {
		return errors.New("client executable is unavailable")
	}
	cmd := exec.Command(installation.executable, installation.arguments...)
	cmd.Dir = filepath.Dir(installation.executable)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createDetachedProcess | createNewProcessGroup,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
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

func queryRegistryValue(key, name string) string {
	if key == "" || name == "" {
		return ""
	}
	cmd := exec.Command("reg.exe", "query", key, "/v", name)
	hideCommandWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseRegistryOutput(string(output), name)
}

func queryRegistryDefault(key string) string {
	cmd := exec.Command("reg.exe", "query", key, "/ve")
	hideCommandWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseRegistryOutput(string(output), "")
}

func parseRegistryOutput(output, name string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || (!strings.Contains(line, "REG_SZ") && !strings.Contains(line, "REG_EXPAND_SZ")) {
			continue
		}
		if name != "" && !strings.HasPrefix(strings.ToLower(line), strings.ToLower(name)) {
			continue
		}
		fields := strings.Fields(line)
		for index, field := range fields {
			if field != "REG_SZ" && field != "REG_EXPAND_SZ" {
				continue
			}
			value := strings.Join(fields[index+1:], " ")
			return strings.TrimSpace(os.ExpandEnv(value))
		}
	}
	return ""
}

func parseExecutableValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, `"`) {
		if end := strings.Index(value[1:], `"`); end >= 0 {
			return value[1 : end+1]
		}
	}
	if comma := strings.LastIndex(value, ","); comma > 0 {
		value = value[:comma]
	}
	return strings.Trim(value, ` "`)
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
