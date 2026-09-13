package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type uninstallLaunch struct {
	Options   uninstallOptions `json:"options"`
	Directory string           `json:"directory"`
	Workspace string           `json:"workspace"`
	ParentPID uint32           `json:"parentPID"`
}

func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	var signature [2]byte
	if _, err := io.ReadFull(input, signature[:]); err != nil || string(signature[:]) != "MZ" {
		return fmt.Errorf("invalid executable: %s", source)
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func prepareLaunch(executable string, options uninstallOptions) (uninstallLaunch, error) {
	directory := options.InstallDir
	if directory == "" {
		directory = filepath.Dir(executable)
	}
	directory = filepath.Clean(directory)
	if !filepath.IsAbs(directory) || filepath.Dir(directory) == directory || strings.ContainsAny(directory, "\"\x00\r\n") {
		return uninstallLaunch{}, fmt.Errorf("invalid installation directory")
	}
	launch := uninstallLaunch{Options: options, Directory: directory}
	workspace, err := os.MkdirTemp("", "DeepLegendsUninstall-")
	if err != nil {
		return launch, err
	}
	launch.Workspace = workspace
	if err := copyExecutable(filepath.Join(directory, "resources", corePayloadName), filepath.Join(workspace, coreFilename)); err != nil {
		_ = os.RemoveAll(workspace)
		return launch, err
	}
	return launch, nil
}
