package webviewhost

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

const PageTimeout = 20 * time.Second

//go:embed startup.js
var startupScript string

// Inline the initial state before the application's script. Registering a
// document-created script is asynchronous and must not race NavigateToString.
func Document(html string, initial any) (string, error) {
	data, err := json.Marshal(initial)
	if err != nil {
		return "", err
	}
	if !strings.Contains(html, "<head>") {
		return "", errors.New("installer document has no head")
	}
	html = strings.Replace(html, "<head>", "<head><script>window.__INIT__="+string(data)+";\n"+startupScript+"</script>", 1)
	if len(utf16.Encode([]rune(html)))*2 > 2*1024*1024 {
		return "", errors.New("installer document exceeds WebView2 NavigateToString limit")
	}
	return html, nil
}

type Surface interface {
	Prepare() error
	Navigate(string) error
	Show() error
}

// Session is owned by the UI thread. DOM readiness and successful navigation
// are independent signals; neither alone is permission to expose the window.
type Session struct {
	surface                            Surface
	onReady                            func()
	onFailure                          func(error)
	navigated, documentReady, terminal bool
}

func New(surface Surface, ready func(), failed func(error)) *Session {
	return &Session{surface: surface, onReady: ready, onFailure: failed}
}
func (s *Session) Start(html string) {
	if err := s.surface.Prepare(); err != nil {
		s.Fail(fmt.Errorf("prepare WebView: %w", err))
		return
	}
	if err := s.surface.Navigate(html); err != nil {
		s.Fail(fmt.Errorf("navigate installer: %w", err))
	}
}
func (s *Session) NavigationCompleted(err error) {
	if s.terminal {
		return
	}
	if err != nil {
		s.Fail(err)
		return
	}
	s.navigated = true
	log.Print("WebView document navigation completed")
	s.finish()
}
func (s *Session) Message(raw string) bool {
	var message struct{ Type, Detail string }
	if len(raw) > 4096 || json.Unmarshal([]byte(raw), &message) != nil {
		return false
	}
	switch message.Type {
	case "shell-ready":
		if !s.terminal {
			s.documentReady = true
			log.Print("WebView document initialization confirmed")
			s.finish()
		}
		return true
	case "shell-error":
		detail := strings.Join(strings.Fields(message.Detail), " ")
		if len(detail) > 256 {
			detail = detail[:256]
		}
		s.Fail(fmt.Errorf("installer page initialization failed: %s", detail))
		return true
	}
	return false
}
func (s *Session) finish() {
	if s.terminal || !s.navigated || !s.documentReady {
		return
	}
	if err := s.surface.Show(); err != nil {
		s.Fail(fmt.Errorf("show WebView: %w", err))
		return
	}
	s.terminal = true
	s.onReady()
}
func (s *Session) Fail(err error) {
	if s.terminal {
		return
	}
	s.terminal = true
	s.onFailure(err)
}
func (s *Session) Timeout() {
	s.Fail(fmt.Errorf("WebView page startup timed out after 20 seconds (navigation=%t, document=%t)", s.navigated, s.documentReady))
}
func (s *Session) Cancel() { s.terminal = true }

// GUI binaries have no console. Preserve startup stages locally so a failed
// machine can report the real HRESULT; never log the initial state or keys.
func StartupLog(name string) func() {
	file, err := os.OpenFile(StartupLogPath(name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return func() {}
	}
	previous := log.Writer()
	previousPrefix := log.Prefix()
	previousFlags := log.Flags()
	// A windowsgui executable launched from Explorer has no usable stderr.
	// MultiWriter(stderr, file) stops on that first error, leaving an empty file.
	log.SetOutput(file)
	log.SetPrefix(fmt.Sprintf("[pid=%d] ", os.Getpid()))
	// Deliberately omit Lmsgprefix: collectors require [pid=N] at line start.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Print("starting ", name)
	_ = file.Sync()
	return func() {
		_ = file.Sync()
		log.SetOutput(previous)
		log.SetPrefix(previousPrefix)
		log.SetFlags(previousFlags)
		_ = file.Close()
	}
}

func StartupLogPath(name string) string {
	return filepath.Join(os.TempDir(), name+"-startup.log")
}
