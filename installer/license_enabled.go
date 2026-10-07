//go:build license

package main

import (
	"embed"
	"io/fs"
)

//go:embed ui/license-activation.html
var activationUI embed.FS

type activationUIFiles struct{ fs.FS }

func (f activationUIFiles) Open(name string) (fs.File, error) {
	if name == "ui/license.html" {
		return activationUI.Open("ui/license-activation.html")
	}
	return f.FS.Open(name)
}
func licenseUIFiles(files fs.FS) fs.FS { return activationUIFiles{files} }
