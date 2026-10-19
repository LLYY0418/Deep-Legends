//go:build windows

package main

import (
	"golang.org/x/sys/windows/registry"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func windowsNamedProcessImages(names ...string) map[string][]string {
	accepted := map[string]bool{}
	for _, n := range names {
		accepted[strings.ToLower(n)] = true
	}
	result := map[string][]string{}
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(snapshot)
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	if syscall.Process32First(snapshot, &entry) != nil {
		return nil
	}
	for {
		name := strings.ToLower(syscall.UTF16ToString(entry.ExeFile[:]))
		if accepted[name] {
			result[name] = append(result[name], windowsProcessImagePath(entry.ProcessID))
		}
		if err = syscall.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	if err != syscall.ERROR_NO_MORE_FILES {
		return nil
	}
	return result
}
func nativeClientLauncherProcesses(items []clientInstallation) clientLauncherProcesses {
	images := windowsNamedProcessImages("WeGame.exe", "Client.exe", "RiotClientServices.exe", "Riot Client.exe", "LeagueClient.exe")
	result := clientLauncherProcesses{Known: images != nil, Launchers: map[string]bool{"wegame": len(images["wegame.exe"]) > 0, "riot": len(images["riotclientservices.exe"])+len(images["riot client.exe"]) > 0}, LeagueRunning: len(images["leagueclient.exe"]) > 0}
	for _, image := range images["client.exe"] {
		if image == "" {
			continue
		}
		for _, item := range items {
			if item.ID != "tcls" {
				continue
			}
			for _, c := range item.candidates() {
				target := c.executable
				if target == "" && c.shortcut != "" {
					target, _ = shellLinkTarget(c.shortcut)
				}
				if target != "" && strings.EqualFold(filepath.Clean(image), filepath.Clean(target)) {
					result.Launchers["tcls"] = true
				}
			}
		}
	}
	return result
}
func wegameUninstallCandidates() []clientLaunchCandidate {
	var result []clientLaunchCandidate
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			base, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\Uninstall`, registry.ENUMERATE_SUB_KEYS|view)
			if err != nil {
				continue
			}
			names, _ := base.ReadSubKeyNames(-1)
			for _, name := range names {
				key, err := registry.OpenKey(base, name, registry.QUERY_VALUE|view)
				if err != nil {
					continue
				}
				display, _, _ := key.GetStringValue("DisplayName")
				if strings.Contains(strings.ToLower(display), "wegame") {
					icon, _, _ := key.GetStringValue("DisplayIcon")
					location, _, _ := key.GetStringValue("InstallLocation")
					if icon != "" {
						result = append(result, clientLaunchCandidate{Source: "uninstall", executable: displayIconExecutable(icon)})
					}
					if location != "" {
						result = append(result, clientLaunchCandidate{Source: "uninstall", executable: filepath.Join(location, "WeGame.exe")})
					}
				}
				key.Close()
			}
			base.Close()
		}
	}
	return result
}
