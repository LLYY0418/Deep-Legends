package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSafeProbeTaskRetainsOnlyPanicSchema(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = write
	defer func() { os.Stderr = previous; write.Close(); read.Close() }()
	done := make(chan map[string]any, 1)
	go func() { var event map[string]any; _ = json.NewDecoder(read).Decode(&event); done <- event }()
	goSafe("window-probe.test", func() { panic("fake-puuid-private-value") })
	select {
	case event := <-done:
		keys := map[string]bool{}
		for key := range event {
			keys[key] = true
		}
		if !reflect.DeepEqual(keys, map[string]bool{"event": true, "site": true, "panic_kind": true, "frames": true}) {
			t.Fatalf("unexpected keys=%#v", keys)
		}
		if event["event"] != "backend_panic" || event["site"] != "window-probe.test" || event["panic_kind"] != "other" {
			t.Fatalf("missing panic=%#v", event)
		}
		data, _ := json.Marshal(event)
		if strings.Contains(string(data), "fake-puuid-private-value") {
			t.Fatal("panic value leaked")
		}
	case <-time.After(time.Second):
		t.Fatal("safe task lost panic evidence")
	}
}
