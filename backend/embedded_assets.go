package main

import (
	"embed"
	"io"
	"io/fs"
	"sort"
	"strings"
)

// Keep explicit JS names: the activation client and HTML enter the executable
// only through the license-tagged frontend. Do not embed tests or documents.
// Beaufort is the official, non-open-source font; its original copyright/EULA
// notice accompanies the font in every build, as required by the R14 boundary.
//
//go:embed web/app.js web/augment-artwork.js web/champions.js web/demo-data.js
//go:embed web/favorites-facade.js web/friends.js web/gameplay.js web/history-filters.js web/image-queue.js
//go:embed web/overview-art.js web/pro-players.js web/runtime.js web/section-loader.js web/suite.js
//go:embed web/*.css web/*.png web/*.svg
//go:embed web/arena-team-icons/*.svg web/position-icons/*.svg web/tier-icons/*.svg
//go:embed web/rune-styles/*.svg web/loot-icons/*.svg web/loot-icons/*.png web/rank-crests/*.png
//go:embed web/beaufort-for-lol-bold.woff2 web/beaufort-for-lol-notice.txt
//go:embed data/reroll_pool_14_5.txt data/reroll_pool_14_5.json
var commonEmbedded embed.FS

type embeddedAssetFiles struct{ frontend fs.FS }

var embedded = func() embeddedAssetFiles {
	frontend, err := fs.Sub(frontendEmbedded, frontendAssetDirectory)
	if err != nil {
		panic(err)
	}
	return embeddedAssetFiles{frontend: frontend}
}()

func (e embeddedAssetFiles) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if relative, ok := strings.CutPrefix(name, "web/"); ok {
		if file, err := e.frontend.Open(relative); err == nil {
			return file, nil
		}
	}
	return commonEmbedded.Open(name)
}

func (e embeddedAssetFiles) ReadFile(name string) ([]byte, error) {
	file, err := e.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func (e embeddedAssetFiles) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := commonEmbedded.ReadDir(name)
	if err != nil {
		return nil, err
	}
	if name == "web" {
		frontend, err := fs.ReadDir(e.frontend, ".")
		if err != nil {
			return nil, err
		}
		entries = append(entries, frontend...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	}
	return entries, nil
}
