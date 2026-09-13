package webviewhost

import (
	"errors"
	"strings"
	"testing"
)

type fakeSurface struct {
	prepared, navigated, visible     bool
	prepareErr, navigateErr, showErr error
	shows                            int
}

func (f *fakeSurface) Prepare() error { f.prepared = true; return f.prepareErr }
func (f *fakeSurface) Navigate(string) error {
	if !f.prepared {
		panic("navigation before controller setup")
	}
	f.navigated = true
	return f.navigateErr
}
func (f *fakeSurface) Show() error {
	if !f.navigated {
		panic("window exposed before navigation")
	}
	f.shows++
	f.visible = f.showErr == nil
	return f.showErr
}

func TestWindowRequiresDocumentAndNavigation(t *testing.T) {
	for _, documentFirst := range []bool{false, true} {
		f := &fakeSurface{}
		ready, failed := 0, 0
		s := New(f, func() { ready++ }, func(error) { failed++ })
		s.Start("installer")
		if f.visible {
			t.Fatal("empty window shown during startup")
		}
		if documentFirst {
			s.Message(`{"type":"shell-ready"}`)
		} else {
			s.NavigationCompleted(nil)
		}
		if f.visible || ready != 0 {
			t.Fatal("one signal exposed the empty window")
		}
		if documentFirst {
			s.NavigationCompleted(nil)
		} else {
			s.Message(`{"type":"shell-ready"}`)
		}
		if !f.visible || ready != 1 || failed != 0 {
			t.Fatal("healthy UI never appeared")
		}
		s.NavigationCompleted(nil)
		s.Message(`{"type":"shell-ready"}`)
		s.Timeout()
		if f.shows != 1 || ready != 1 || failed != 0 {
			t.Fatal("late events duplicated startup or triggered fallback")
		}
	}
}

func TestStartupFailuresFallbackExactlyOnce(t *testing.T) {
	broken := errors.New("native call failed")
	for _, stage := range []string{"prepare", "navigate", "navigation-event", "script", "no-navigation", "no-document", "no-events", "show"} {
		t.Run(stage, func(t *testing.T) {
			f := &fakeSurface{}
			if stage == "prepare" {
				f.prepareErr = broken
			}
			if stage == "navigate" {
				f.navigateErr = broken
			}
			if stage == "show" {
				f.showErr = broken
			}
			ready, failures := 0, 0
			s := New(f, func() { ready++ }, func(error) { failures++ })
			s.Start("installer")
			switch stage {
			case "navigation-event":
				s.NavigationCompleted(broken)
			case "script":
				s.Message(`{"type":"shell-error","detail":"broken script"}`)
			case "no-navigation":
				s.Message(`{"type":"shell-ready"}`)
				s.Timeout()
			case "no-document":
				s.NavigationCompleted(nil)
				s.Timeout()
			case "no-events":
				s.Timeout()
			case "show":
				s.NavigationCompleted(nil)
				s.Message(`{"type":"shell-ready"}`)
			}
			s.Message(`{"type":"shell-ready"}`)
			s.NavigationCompleted(nil)
			s.Timeout()
			if ready != 0 || f.visible || failures != 1 {
				t.Fatalf("ready=%d visible=%v failures=%d", ready, f.visible, failures)
			}
		})
	}
}

func TestClosingStartupDoesNotLaunchFallbackOrShowLateWindow(t *testing.T) {
	f := &fakeSurface{}
	s := New(f, func() { t.Error("show after close") }, func(error) { t.Error("fallback after close") })
	s.Start("installer")
	s.Cancel()
	s.Timeout()
	s.NavigationCompleted(nil)
	s.Message(`{"type":"shell-ready"}`)
	if f.visible {
		t.Fatal("closed window reappeared")
	}
}

func TestDocumentInitialStateIsInlineAndBounded(t *testing.T) {
	html, err := Document("<html><head></head><body><script>window.host.init(window.__INIT__)</script></body></html>", map[string]string{"path": `C:\游戏\</script><script>bad()</script>`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(html, "window.__INIT__=") > strings.Index(html, "window.host.init(window.__INIT__)") {
		t.Fatal("initial state arrives too late")
	}
	if strings.Contains(html, "<script>bad()") || !strings.Contains(html, `\u003c/script\u003e`) {
		t.Fatal("initial state escaped the script")
	}
	if !strings.Contains(html, startupScript) {
		t.Fatal("readiness bridge omitted")
	}
	if _, err = Document("no document", nil); err == nil {
		t.Fatal("missing head accepted")
	}
	if _, err = Document("<head>"+strings.Repeat("x", 1024*1024)+"</head>", nil); err == nil {
		t.Fatal("oversize NavigateToString accepted")
	}
}

func TestStartupFailureRetainsDiagnosticStage(t *testing.T) {
	var failure error
	s := New(&fakeSurface{}, func() {}, func(err error) { failure = err })
	s.Message(`{"type":"shell-error","detail":"脚本失败（12:3）\n下一行"}`)
	if failure == nil || !strings.Contains(failure.Error(), "12:3") || strings.Contains(failure.Error(), "\n") {
		t.Fatalf("script location lost or log injection allowed: %v", failure)
	}
	s = New(&fakeSurface{}, func() {}, func(err error) { failure = err })
	s.NavigationCompleted(nil)
	s.Timeout()
	if failure == nil || !strings.Contains(failure.Error(), "navigation=true, document=false") {
		t.Fatalf("timeout omitted the missing startup signal: %v", failure)
	}
}
