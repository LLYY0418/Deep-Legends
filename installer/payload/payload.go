package payload

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
)

// Keep .gitkeep embedded: developer builds must work without a release payload.
//
//go:embed all:files
var files embed.FS

type Metadata struct {
	Version        string `json:"version"`
	Fingerprint    string `json:"fingerprint"`
	InstalledBytes int64  `json:"installedBytes"`
	ExeName        string `json:"exeName"`
	ProductFolder  string `json:"productFolder"`
}

func Open() (fs.File, error) { return files.Open("files/setup.exe") }

func LoadMetadata() (Metadata, error) {
	var meta Metadata
	data, err := files.ReadFile("files/meta.json")
	if err != nil {
		return meta, err
	}
	if err = json.Unmarshal(data, &meta); err != nil {
		return meta, err
	}
	if meta.InstalledBytes <= 0 || meta.InstalledBytes > 1<<50 || meta.Version == "" ||
		meta.ExeName != "Deep Legends.exe" || meta.ProductFolder != "Deep Legends" || strings.TrimSpace(meta.Fingerprint) == "" {
		return meta, errors.New("invalid installer metadata")
	}
	f, err := Open()
	if err != nil {
		return meta, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return meta, errors.New("missing installer payload")
	}
	return meta, nil
}
