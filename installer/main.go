package main

// rsrc assigns ID 1 to the manifest and ID 2 to the icon group.
const windowIconResourceID = 2

// State is owned by the UI thread, including validation and the launch delay.
// Reserving the attempt before starting a goroutine prevents duplicate installs.
type installPhase uint8

const (
	phaseReady installPhase = iota
	phaseInstalling
	phaseFinishing
	phaseFailed
)

type installState struct{ phase installPhase }

func (s *installState) busy() bool {
	return s.phase == phaseInstalling || s.phase == phaseFinishing
}

func (s *installState) begin() bool {
	if s.busy() {
		return false
	}
	s.phase = phaseInstalling
	return true
}

type uiMessage struct {
	Type            string `json:"type"`
	Path            string `json:"path"`
	DesktopShortcut bool   `json:"desktopShortcut"`
}

type initMessage struct {
	Path      string `json:"path"`
	NeedBytes int64  `json:"needBytes"`
	FreeBytes int64  `json:"freeBytes"`
	Version   string `json:"version"`
}

type pathMessage struct {
	OK        bool    `json:"ok"`
	Path      *string `json:"path,omitempty"`
	FreeBytes int64   `json:"freeBytes"`
	Error     string  `json:"error,omitempty"`
}

type progressMessage struct {
	Percent int    `json:"percent"`
	Stage   string `json:"stage"`
}

type failureMessage struct {
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}
