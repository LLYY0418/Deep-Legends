package main

import (
	"bytes"
	"testing"
)

type r288ApprovalMemory map[string][]byte

func (m r288ApprovalMemory) Read(t startupApprovalTarget) ([]byte, bool, error) {
	v, ok := m[t.id()]
	return append([]byte(nil), v...), ok, nil
}
func (m r288ApprovalMemory) Write(t startupApprovalTarget, v []byte) error {
	m[t.id()] = append([]byte(nil), v...)
	return nil
}
func (m r288ApprovalMemory) Remove(t startupApprovalTarget) error { delete(m, t.id()); return nil }
func TestR288AutostartDisableAndRestore(t *testing.T) {
	a := r175App(t)
	s := &a.clientAutostart
	s.load(a)
	target := startupApprovalTarget{Hive: "HKCU", Key: "fixture/StartupApproved/Run", Name: "owned"}
	other := startupApprovalTarget{Hive: "HKCU", Key: "fixture/StartupApproved/Run", Name: "unrelated"}
	enabled := make([]byte, 12)
	enabled[0] = 2
	memory := r288ApprovalMemory{target.id(): enabled, other.id(): append([]byte(nil), enabled...)}
	entries := []clientAutostartEntry{{Target: target, Source: "run", TargetClass: "tcls", Manage: true}}
	if err := s.apply(a, memory, entries); err != nil {
		t.Fatal(err)
	}
	if yes, known := startupApprovalEnabled(memory[target.id()], true); !known || yes {
		t.Fatal("new owned entry not disabled")
	}
	if !bytes.Equal(memory[other.id()], enabled) {
		t.Fatal("other software changed")
	}
	// The backup survives restart before restoring, including exact bytes.
	next := clientAutostartState{}
	next.load(a)
	off := false
	next.settings.Block = &off
	if err := next.apply(a, memory, entries); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(memory[target.id()], enabled) {
		t.Fatal("original state not restored")
	}
	delete(memory, target.id())
	s.settings.Originals = map[string]startupApprovalBackup{}
	if err := s.apply(a, memory, entries); err != nil {
		t.Fatal(err)
	}
	s.settings.Block = &off
	if err := s.apply(a, memory, entries); err != nil {
		t.Fatal(err)
	}
	if _, exists := memory[target.id()]; exists {
		t.Fatal("originally absent approval not restored")
	}
}
func TestR288AutostartUnknownFormatAndUserChangesSurvive(t *testing.T) {
	a := r175App(t)
	s := &a.clientAutostart
	s.load(a)
	target := startupApprovalTarget{Name: "owned"}
	entry := []clientAutostartEntry{{Target: target, Manage: true}}
	m := r288ApprovalMemory{target.id(): {99, 0, 0, 0}}
	if err := s.apply(a, m, entry); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(m[target.id()], []byte{99, 0, 0, 0}) {
		t.Fatal("unknown format changed")
	}
	enabled := make([]byte, 12)
	enabled[0] = 2
	m[target.id()] = enabled
	s.apply(a, m, entry)
	user := make([]byte, 12)
	user[0] = 3
	user[7] = 123
	m[target.id()] = user
	off := false
	s.settings.Block = &off
	s.apply(a, m, entry)
	if !bytes.Equal(m[target.id()], user) {
		t.Fatal("user's Task Manager change overwritten")
	}
}
