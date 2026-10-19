package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type startupApprovalTarget struct {
	Hive string
	View uint32
	Key  string
	Name string
}

func (t startupApprovalTarget) id() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%s", t.Hive, t.View, t.Key, t.Name))))
}

type startupApprovalBackup struct {
	Target  startupApprovalTarget
	Before  []byte
	Absent  bool
	Written []byte
}
type clientAutostartSettings struct {
	Block     *bool                            `json:"block"`
	Originals map[string]startupApprovalBackup `json:"originals,omitempty"`
}
type clientAutostartState struct {
	mu       sync.Mutex
	loaded   bool
	settings clientAutostartSettings
	previous map[string]bool
}
type clientAutostartEntry struct {
	Target              startupApprovalTarget
	Source, TargetClass string
	Manage              bool
}
type startupApprovalStore interface {
	Read(startupApprovalTarget) ([]byte, bool, error)
	Write(startupApprovalTarget, []byte) error
	Remove(startupApprovalTarget) error
}

// StartupApproved REG_BINARY is a 32-bit state followed by a FILETIME.
// Unknown sizes/states are observations only, never rewritten.
func startupApprovalEnabled(value []byte, exists bool) (bool, bool) {
	if !exists {
		return true, true
	}
	if len(value) != 12 {
		return false, false
	}
	state := binary.LittleEndian.Uint32(value)
	if state != 2 && state != 3 {
		return false, false
	}
	return state == 2, true
}
func disabledStartupApproval(at time.Time) []byte {
	value := make([]byte, 12)
	binary.LittleEndian.PutUint32(value, 3)
	ticks := uint64(at.UnixNano()/100 + 116444736000000000)
	binary.LittleEndian.PutUint64(value[4:], ticks)
	return value
}
func (s *clientAutostartState) load(a *app) {
	if s.loaded {
		return
	}
	s.loaded = true
	s.settings.Originals = map[string]startupApprovalBackup{}
	s.previous = map[string]bool{}
	if a.storage != nil {
		if data, err := readLocalStoreFile(a.storage, "client-autostart.json"); err == nil && len(data) <= 128<<10 {
			var saved clientAutostartSettings
			if json.Unmarshal(data, &saved) == nil {
				s.settings = saved
			}
		}
	}
	if s.settings.Originals == nil {
		s.settings.Originals = map[string]startupApprovalBackup{}
	}
}
func (s *clientAutostartState) persist(a *app) error {
	if a.storage == nil {
		return fmt.Errorf("autostart storage unavailable")
	}
	data, err := json.Marshal(s.settings)
	if err != nil {
		return err
	}
	return writeLocalStoreFile(a.storage, "client-autostart.json", data)
}
func (s *clientAutostartState) blocking() bool { return s.settings.Block == nil || *s.settings.Block }

// Persist the original bytes before changing Windows state. Restore only the
// exact value we wrote, so a later change made in Task Manager wins.
func (s *clientAutostartState) apply(a *app, store startupApprovalStore, entries []clientAutostartEntry) error {
	if !s.blocking() {
		for id, backup := range s.settings.Originals {
			current, exists, err := store.Read(backup.Target)
			if err != nil {
				return err
			}
			if exists && bytes.Equal(current, backup.Written) {
				if backup.Absent {
					err = store.Remove(backup.Target)
				} else {
					err = store.Write(backup.Target, backup.Before)
				}
				if err != nil {
					return err
				}
			}
			delete(s.settings.Originals, id)
			if err = s.persist(a); err != nil {
				return err
			}
		}
	}
	for _, entry := range entries {
		current, exists, err := store.Read(entry.Target)
		if err != nil {
			a.recordDiagnostic(map[string]any{"event": "client_autostart_scan", "source": entry.Source, "target_class": entry.TargetClass, "status": "unresolved"})
			continue
		}
		enabled, resolved := startupApprovalEnabled(current, exists)
		id := entry.Target.id()
		change := "unchanged"
		if !s.previous[id] {
			change = "new"
		}
		s.previous[id] = true
		status := "resolved"
		if !resolved {
			status = "unresolved"
		}
		a.recordDiagnostic(map[string]any{"event": "client_autostart_scan", "source": entry.Source, "target_class": entry.TargetClass, "enabled": enabled, "change": change, "status": status})
		if !s.blocking() || !entry.Manage || !resolved || !enabled {
			continue
		}
		backup, already := s.settings.Originals[id]
		if !already {
			backup = startupApprovalBackup{Target: entry.Target, Before: append([]byte(nil), current...), Absent: !exists, Written: disabledStartupApproval(time.Now())}
			s.settings.Originals[id] = backup
			if err = s.persist(a); err != nil {
				delete(s.settings.Originals, id)
				return err
			}
		}
		if err = store.Write(entry.Target, backup.Written); err != nil {
			a.recordDiagnostic(map[string]any{"event": "client_autostart_scan", "source": entry.Source, "target_class": entry.TargetClass, "status": "unresolved", "enabled": true, "change": change})
		}
	}
	return nil
}
func (a *app) scanClientAutostart() {
	entries, store := nativeClientAutostarts(a.detectedClientInstallations())
	s := &a.clientAutostart
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load(a)
	a.recordDiagnostic(map[string]any{"event": "client_autostart_scan", "source": "tasks_services", "status": "not_checked"})
	if store != nil {
		if err := s.apply(a, store, entries); err != nil {
			a.recordDiagnostic(map[string]any{"event": "client_autostart_scan", "source": "approval", "status": "unresolved"})
		}
	}
}
func (a *app) scheduleClientAutostartScans(id string) {
	if a.proBusinessContext() == nil || id != "tcls" && id != "wegame" {
		return
	}
	a.goSafe("client-autostart-after-launch", func() {
		ctx := a.licenseBusinessContext()
		for _, delay := range []time.Duration{30 * time.Second, 90 * time.Second} {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				a.scanClientAutostart()
			}
		}
	})
}
func (a *app) handleClientAutostart(w http.ResponseWriter, r *http.Request) {
	s := &a.clientAutostart
	s.mu.Lock()
	s.load(a)
	if r.Method == http.MethodPost {
		var body struct {
			Block *bool `json:"block"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || body.Block == nil {
			s.mu.Unlock()
			http.Error(w, "invalid settings", 400)
			return
		}
		previous := s.settings.Block
		s.settings.Block = body.Block
		if s.persist(a) != nil {
			s.settings.Block = previous
			s.mu.Unlock()
			http.Error(w, "settings unavailable", 500)
			return
		}
	}
	block := s.blocking()
	s.mu.Unlock()
	if r.Method == http.MethodPost {
		a.scanClientAutostart()
	}
	respondJSON(w, map[string]any{"block": block})
}
