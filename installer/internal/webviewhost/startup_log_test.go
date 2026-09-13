package webviewhost

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type unavailableConsole struct{}

func (unavailableConsole) Write([]byte) (int, error) {
	return 0, errors.New("invalid console handle")
}

func TestStartupLogSurvivesMissingGUIConsoleAndRepeatedLaunch(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, directory)
	}
	previous := log.Writer()
	t.Cleanup(func() { log.SetOutput(previous) })
	log.SetOutput(unavailableConsole{})
	for _, marker := range []string{"first launch error", "second launch"} {
		closeLog := StartupLog("DeepLegendsSetup")
		log.Print(marker)
		closeLog()
		if _, ok := log.Writer().(unavailableConsole); !ok {
			t.Fatal("previous logger was not restored")
		}
	}
	data, err := os.ReadFile(filepath.Join(directory, "DeepLegendsSetup-startup.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"starting DeepLegendsSetup", "first launch error", "second launch"} {
		if !strings.Contains(string(data), marker) {
			t.Fatalf("missing %q in GUI startup log: %q", marker, data)
		}
	}
}
