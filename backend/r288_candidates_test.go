package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestR288WeGameVersionSelection(t *testing.T) {
	old := filepath.Join(t.TempDir(), "old", "WeGame.exe")
	latest := filepath.Join(t.TempDir(), "new", "WeGame.exe")
	candidates := []clientLaunchCandidate{{Source: "shortcut", executable: old}, {Source: "uninstall", executable: latest}}
	selected, observed, ok := selectWeGameCandidate(candidates, func(p string) clientFileVersion {
		v := clientFileVersion{Exists: true, Known: true, Parts: [4]uint16{1, 0, 0, 0}}
		if p == latest {
			v.Parts[0] = 2
		}
		return v
	})
	if !ok || selected.executable != latest || selected.shortcut != "" || !observed[1].Chosen {
		t.Fatal(selected, observed)
	}
	selected, _, _ = selectWeGameCandidate(candidates, func(p string) clientFileVersion {
		v := clientFileVersion{Exists: true, Known: true, Modified: time.Unix(1, 0)}
		if p == latest {
			v.Modified = time.Unix(2, 0)
		}
		return v
	})
	if selected.executable != latest {
		t.Fatal("mtime tie-break")
	}
	if _, _, ok = selectWeGameCandidate(candidates, func(string) clientFileVersion { return clientFileVersion{} }); ok {
		t.Fatal("missing executables selectable")
	}
	if got := displayIconExecutable(`"C:\Program Files\WeGame\WeGame.exe",0`); got != `C:\Program Files\WeGame\WeGame.exe` {
		t.Fatal(got)
	}
}

func TestR290WeGameUnresolvedVersionFallback(t *testing.T) {
	paths := []string{filepath.Join(t.TempDir(), "old", "WeGame.exe"), filepath.Join(t.TempDir(), "new", "WeGame.exe")}
	candidates := []clientLaunchCandidate{{Source: "shortcut", executable: paths[0]}, {Source: "uninstall", executable: paths[1]}}
	for _, scenario := range []string{"all-unresolved", "single-existing", "known-preferred"} {
		t.Run(scenario, func(t *testing.T) {
			selected, observations, ok := selectWeGameCandidate(candidates, func(path string) clientFileVersion {
				v := clientFileVersion{Exists: true, Modified: time.Unix(1, 0)}
				if path == paths[1] {
					v.Modified = time.Unix(2, 0)
				}
				if scenario == "single-existing" && path == paths[0] {
					v.Exists = false
				}
				if scenario == "known-preferred" && path == paths[0] {
					v.Known = true
					v.Parts[0] = 1
				}
				return v
			})
			want := 1
			if scenario == "known-preferred" {
				want = 0
			}
			if !ok || selected.executable != paths[want] || !observations[want].Chosen {
				t.Fatal(selected, observations)
			}
			if scenario != "known-preferred" && observations[want].FileVersion != "unresolved" {
				t.Fatal(observations)
			}
		})
	}
}
