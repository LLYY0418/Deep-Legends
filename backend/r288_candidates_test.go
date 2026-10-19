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
