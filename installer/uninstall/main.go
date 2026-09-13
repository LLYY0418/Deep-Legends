package main

const windowIconResourceID = 2

var version = "dev"

const coreFilename = "Uninstall Deep Legends Core.exe"
const corePayloadName = "uninstall-core.dat"
const shellFilename = "Uninstall Deep Legends.exe"

type uninstallState struct{ running bool }

func (s *uninstallState) busy() bool { return s.running }
func (s *uninstallState) begin() bool {
	if s.running {
		return false
	}
	s.running = true
	return true
}

type uiMessage struct {
	Type       string `json:"type"`
	DeleteData bool   `json:"deleteData"`
}

type initMessage struct {
	Path       string `json:"path"`
	SizeBytes  int64  `json:"sizeBytes"`
	CacheBytes int64  `json:"cacheBytes"`
	Version    string `json:"version"`
}

type progressMessage struct {
	Percent int    `json:"percent"`
	Stage   string `json:"stage"`
}

type failureMessage struct {
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// Silent dispatch precedes every runtime probe, window and WebView operation.
func dispatchMode(options uninstallOptions, silent, interactive func() int) int {
	if options.silent() {
		return silent()
	}
	return interactive()
}
