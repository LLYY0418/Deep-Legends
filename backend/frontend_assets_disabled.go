//go:build !license

package main

import "embed"

//go:embed web/default/index.html web/default/license-ui.js
var frontendEmbedded embed.FS

const frontendAssetDirectory = "web/default"
