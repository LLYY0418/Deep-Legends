//go:build license

package main

import "embed"

// Preserve the dormant activation UI; only an explicit license build embeds it.
//
//go:embed web/index.html web/license-ui.js
var frontendEmbedded embed.FS

const frontendAssetDirectory = "web"
