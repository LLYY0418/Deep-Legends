//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"strings"
)

type windowsStartupApprovalStore struct{}

func startupHive(name string) registry.Key {
	if name == "HKLM" {
		return registry.LOCAL_MACHINE
	}
	return registry.CURRENT_USER
}
func (windowsStartupApprovalStore) Read(t startupApprovalTarget) ([]byte, bool, error) {
	key, err := registry.OpenKey(startupHive(t.Hive), t.Key, registry.QUERY_VALUE|t.View)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer key.Close()
	raw, _, err := key.GetBinaryValue(t.Name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, false, nil
	}
	return raw, err == nil, err
}
func (windowsStartupApprovalStore) Write(t startupApprovalTarget, b []byte) error {
	key, _, err := registry.CreateKey(startupHive(t.Hive), t.Key, registry.SET_VALUE|t.View)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetBinaryValue(t.Name, b)
}
func (windowsStartupApprovalStore) Remove(t startupApprovalTarget) error {
	key, err := registry.OpenKey(startupHive(t.Hive), t.Key, registry.SET_VALUE|t.View)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	err = key.DeleteValue(t.Name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func ownedClientStartupTarget(target string, items []clientInstallation) (string, bool) {
	if !filepath.IsAbs(target) {
		return "", false
	}
	for _, item := range items {
		for _, candidate := range item.candidates() {
			exe := candidate.executable
			if exe == "" && candidate.shortcut != "" {
				exe, _ = shellLinkTarget(candidate.shortcut)
			}
			if exe == "" {
				continue
			}
			root := filepath.Dir(exe)
			if item.ID == "tcls" && (strings.EqualFold(filepath.Base(root), "TCLS") || strings.EqualFold(filepath.Base(root), "Launcher")) {
				root = filepath.Dir(root)
			}
			if item.ID != "wegame" && item.ID != "tcls" {
				continue
			}
			rel, err := filepath.Rel(strings.ToLower(root), strings.ToLower(target))
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			switch strings.ToLower(filepath.Base(target)) {
			case "wegame.exe":
				return "wegame", true
			case "leagueclient.exe", "leagueclientux.exe":
				return "league", true
			case "client.exe":
				if strings.EqualFold(filepath.Base(filepath.Dir(target)), "TCLS") {
					return "tcls", true
				}
				if strings.EqualFold(filepath.Base(filepath.Dir(target)), "Launcher") {
					return "launcher", true
				}
			}
			return "other", true
		}
	}
	return "", false
}
func nativeClientAutostarts(items []clientInstallation) ([]clientAutostartEntry, startupApprovalStore) {
	const current = `Software\Microsoft\Windows\CurrentVersion\`
	var entries []clientAutostartEntry
	for _, hive := range []string{"HKCU", "HKLM"} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			for _, source := range []string{"Run", "RunOnce"} {
				key, err := registry.OpenKey(startupHive(hive), current+source, registry.QUERY_VALUE|view)
				if err != nil {
					continue
				}
				names, _ := key.ReadValueNames(-1)
				for _, name := range names {
					command, kind, err := key.GetStringValue(name)
					if err != nil {
						continue
					}
					if kind == registry.EXPAND_SZ {
						command, err = registry.ExpandString(command)
						if err != nil {
							continue
						}
					}
					args, err := windows.DecomposeCommandLine(command)
					if err != nil || len(args) == 0 {
						continue
					}
					class, owned := ownedClientStartupTarget(args[0], items)
					if !owned {
						continue
					}
					approvalKey := "Run"
					// StartupApproved is read by the native shell. Redirected
					// machine-wide 32-bit Run entries use its Run32 bucket.
					if hive == "HKLM" && view == registry.WOW64_32KEY {
						approvalKey = "Run32"
					}
					entries = append(entries, clientAutostartEntry{Target: startupApprovalTarget{Hive: hive, View: registry.WOW64_64KEY, Key: current + `Explorer\StartupApproved\` + approvalKey, Name: name}, Source: strings.ToLower(source), TargetClass: class, Manage: source == "Run"})
				}
				key.Close()
			}
		}
	}
	for _, folder := range []struct{ env, hive string }{{"APPDATA", "HKCU"}, {"PROGRAMDATA", "HKLM"}} {
		base := os.Getenv(folder.env)
		if base == "" {
			continue
		}
		directory := filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		files, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.EqualFold(filepath.Ext(file.Name()), ".lnk") {
				continue
			}
			target, err := shellLinkTarget(filepath.Join(directory, file.Name()))
			if err != nil {
				continue
			}
			class, owned := ownedClientStartupTarget(target, items)
			if !owned {
				continue
			}
			entries = append(entries, clientAutostartEntry{Target: startupApprovalTarget{Hive: folder.hive, Key: current + `Explorer\StartupApproved\StartupFolder`, Name: file.Name()}, Source: "startup_folder", TargetClass: class, Manage: true})
		}
	}
	return entries, windowsStartupApprovalStore{}
}
