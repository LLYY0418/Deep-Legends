//go:build !license

package main

import "io/fs"

func licenseUIFiles(files fs.FS) fs.FS { return files }
