//go:build license

package main

import "bytes"

func frontendBuildAsset(name string, body []byte) []byte {
	if name != "index.html" {
		return body
	}
	body = bytes.Replace(body, []byte(`data-license="disabled"`), []byte(`data-license="pending"`), 1)
	return bytes.Replace(body, []byte(`id="app-frame" class="app-frame"`), []byte(`id="app-frame" class="app-frame" inert hidden`), 1)
}
