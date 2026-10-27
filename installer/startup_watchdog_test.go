package main

import "testing"

func TestR313ControllerWatchdogStopsAfterSuccessfulFactory(t *testing.T) {
	callbacks := make(chan func(), 1)
	cancelled := make(chan bool, 1)
	w := newStartupWatchdog(func(fn func()) func() bool {
		callbacks <- fn
		return func() bool { cancelled <- true; return true }
	}, func() { t.Error("successful factory recovered an application") })
	fire := <-callbacks
	if !w.stop() || !<-cancelled {
		t.Fatal("factory did not cancel watchdog")
	}
	fire()
	if !w.stop() {
		t.Fatal("completion changed after a late timer callback")
	}
	<-w.done
}

func TestR313ControllerWatchdogRecoversOnceBeforeFactoryCanContinue(t *testing.T) {
	callbacks := make(chan func(), 1)
	entered, release, events := make(chan struct{}), make(chan struct{}), make(chan string, 2)
	w := newStartupWatchdog(func(fn func()) func() bool {
		callbacks <- fn
		return func() bool { t.Error("expired watchdog cancelled"); return false }
	}, func() { close(entered); <-release; events <- "recover original"; events <- "report failure and exit" })
	fire := <-callbacks
	go fire()
	<-entered
	if w.stop() {
		t.Fatal("expired factory was allowed to continue")
	}
	select {
	case <-w.done:
		t.Fatal("recovery was not awaited")
	default:
	}
	fire()
	close(release)
	<-w.done
	if <-events != "recover original" || <-events != "report failure and exit" || len(events) != 0 {
		t.Fatal("recovery order changed")
	}
}
