package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func migrationFixture(t *testing.T) settingsLocation {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "League of Legends.exe"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	return settingsLocation{allowedRoot: root, configRoot: filepath.Join(root, "Config")}
}
func migrationFile(t *testing.T, loc settingsLocation, name, contents string) string {
	t.Helper()
	file := filepath.Join(loc.configRoot, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}
func TestLegacyDiskMigrationPreservesForeignAndArchivesExactUIDs(t *testing.T) {
	loc := migrationFixture(t)
	files := map[string]string{
		"Champions/Lucian/Recommended/other-position.json": `{"uid":"deep-legends-v1-236-top","blocks":[]}`,
		"Global/Recommended/old.json":                      `{"uid":"deep-legends-v1-236-bottom","blocks":[]}`,
		"Champions/Ahri/Recommended/old.json":              `{"uid":"deep-legends-v1-103-middle","blocks":[]}`,
		"Global/Recommended/user.json":                     `{"uid":"private","title":"DL","startedFrom":"DL"}`,
		"Global/Recommended/prefix.json":                   `{"uid":"deep-legends-v1-103-middle-user"}`,
		"Global/Recommended/current.json":                  `{"uid":"deep-legends-v2-recommended"}`,
	}
	for name, data := range files {
		migrationFile(t, loc, name, data)
	}
	count, err := archiveLegacyRecommendedFiles(context.Background(), loc, 236, nil)
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	backups, err := filepath.Glob(filepath.Join(loc.configRoot, "DeepLegendsItemSetBackups", "v1-*", "*.json"))
	if err != nil || len(backups) != 2 {
		t.Fatalf("backups=%v err=%v", backups, err)
	}
	preserved := map[string]bool{}
	for _, file := range backups {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		preserved[string(data)] = true
	}
	for name, data := range files {
		got, err := os.ReadFile(filepath.Join(loc.configRoot, filepath.FromSlash(name)))
		if preserved[data] {
			if !os.IsNotExist(err) {
				t.Fatalf("legacy still scanned: %s", name)
			}
		} else if err != nil || string(got) != data {
			t.Fatalf("foreign modified: %s", name)
		}
	}
	if count, err := archiveLegacyRecommendedFiles(context.Background(), loc, 236, nil); count != 0 || err != nil {
		t.Fatalf("not idempotent: %d %v", count, err)
	}
}
func TestLegacyDiskMigrationRejectsBackupSymlinkAndSessionChange(t *testing.T) {
	for _, scenario := range []string{"symlink", "session", "edit"} {
		t.Run(scenario, func(t *testing.T) {
			loc := migrationFixture(t)
			file := migrationFile(t, loc, "Global/Recommended/old.json", `{"uid":"deep-legends-v1-236-bottom"}`)
			var guard func() error
			switch scenario {
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(loc.configRoot, "DeepLegendsItemSetBackups")); err != nil {
					t.Fatal(err)
				}
			case "session":
				guard = func() error { return context.Canceled }
			case "edit":
				guard = func() error { return os.WriteFile(file, []byte(`{"uid":"foreign"}`), 0600) }
			}
			count, err := archiveLegacyRecommendedFiles(context.Background(), loc, 236, guard)
			if err == nil || count != 0 {
				t.Fatalf("unsafe archive: %d %v", count, err)
			}
			if _, err := os.Stat(file); err != nil {
				t.Fatal("source lost", err)
			}
			if scenario == "session" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestLegacyDiskMigrationDoesNotFollowChampionSymlinkOrAlterSettings(t *testing.T) {
	loc := migrationFixture(t)
	cfg := migrationFile(t, loc, "game.cfg", "[HUD]\nGlobalScale=0.85\nShopScale=0.75\n")
	old := migrationFile(t, loc, "Global/Recommended/old.json", `{"uid":"deep-legends-v1-236-bottom"}`)
	_ = old
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(loc.configRoot, "Champions"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(external, "Recommended"), 0755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(external, "Recommended", "old.json")
	if err := os.WriteFile(foreign, []byte(`{"uid":"deep-legends-v1-103-middle"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(loc.configRoot, "Champions", "Ahri")); err != nil {
		t.Fatal(err)
	}
	if count, err := archiveLegacyRecommendedFiles(context.Background(), loc, 236, nil); err != nil || count != 1 {
		t.Fatalf("%d %v", count, err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("followed external symlink")
	}
	if got, err := os.ReadFile(cfg); err != nil || string(got) != "[HUD]\nGlobalScale=0.85\nShopScale=0.75\n" {
		t.Fatal("changed user shop settings")
	}
}
