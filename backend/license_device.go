//go:build license

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type licenseFileStore struct{ root string }

func newLicenseFileStore() (licenseStore, error) {
	root, err := licensePlatformDirectory()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &licenseFileStore{root: root}, nil
}
func (s *licenseFileStore) Load() (licenseDiskState, error) {
	var state licenseDiskState
	key, err := os.ReadFile(filepath.Join(s.root, "device.key"))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	key, err = licenseUnprotect(key)
	// DPAPI loss means a new device, never a portable plaintext fallback.
	if err != nil {
		return state, nil
	}
	data, err := os.ReadFile(filepath.Join(s.root, "lease.json"))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if len(data) > 64*1024 {
		return state, errors.New("invalid license storage")
	}
	data, err = licenseUnprotect(data)
	if err != nil {
		return state, nil
	}
	if strictLicenseJSON(data, &state) != nil {
		return licenseDiskState{}, nil
	}
	state.PrivateKey = key
	return state, nil
}
func (s *licenseFileStore) Save(state licenseDiskState) error {
	key, err := licenseProtect(state.PrivateKey)
	if err != nil {
		return err
	}
	state.PrivateKey = nil
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	data, err = licenseProtect(data)
	if err != nil {
		return err
	}
	if err = writeLicenseFile(filepath.Join(s.root, "device.key"), key); err != nil {
		return err
	}
	return writeLicenseFile(filepath.Join(s.root, "lease.json"), data)
}
func writeLicenseFile(path string, data []byte) error {
	return retryLicenseFileWrite(func() error { return writeLicenseFileOnce(path, data, os.Rename) }, time.Sleep)
}
func retryLicenseFileWrite(write func() error, sleep func(time.Duration)) error {
	var err error
	for attempt, delay := range []time.Duration{0, 50 * time.Millisecond, 200 * time.Millisecond, 800 * time.Millisecond} {
		if attempt > 0 {
			sleep(delay)
		}
		if err = write(); err == nil {
			return nil
		}
	}
	return err
}
func writeLicenseFileOnce(path string, data []byte, rename func(string, string) error) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".license-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return rename(name, path)
}
