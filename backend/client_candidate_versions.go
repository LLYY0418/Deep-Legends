package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type clientCandidateObservation struct {
	Source      string `json:"source"`
	FileVersion string `json:"file_version"`
	Exists      bool   `json:"exists"`
	Chosen      bool   `json:"chosen"`
}
type clientFileVersion struct {
	Parts    [4]uint16
	Modified time.Time
	Known    bool
	Exists   bool
}

func (v clientFileVersion) String() string {
	if !v.Known {
		return "unresolved"
	}
	return fmt.Sprintf("%d.%d.%d.%d", v.Parts[0], v.Parts[1], v.Parts[2], v.Parts[3])
}
func newerClientVersion(a, b clientFileVersion) bool {
	for i := range a.Parts {
		if a.Parts[i] != b.Parts[i] {
			return a.Parts[i] > b.Parts[i]
		}
	}
	return a.Modified.After(b.Modified)
}
func selectWeGameCandidate(candidates []clientLaunchCandidate, inspect func(string) clientFileVersion) (clientLaunchCandidate, []clientCandidateObservation, bool) {
	observations := make([]clientCandidateObservation, 0, len(candidates))
	chosen := -1
	var version clientFileVersion
	for i, c := range candidates {
		v := clientFileVersion{}
		if strings.EqualFold(filepath.Base(c.executable), "WeGame.exe") && filepath.IsAbs(c.executable) {
			v = inspect(c.executable)
		}
		observations = append(observations, clientCandidateObservation{Source: c.Source, FileVersion: v.String(), Exists: v.Exists})
		if v.Exists && v.Known && (chosen < 0 || newerClientVersion(v, version)) {
			chosen, version = i, v
		}
	}
	if chosen < 0 {
		return clientLaunchCandidate{}, observations, false
	}
	observations[chosen].Chosen = true
	c := candidates[chosen]
	c.shortcut = ""
	c.arguments = nil
	return c, observations, true
}

// DisplayIcon is a filename followed by an optional signed decimal resource
// index. Strip only a syntactically valid index; never split a path on spaces.
func displayIconExecutable(value string) string {
	value = strings.TrimSpace(value)
	if i := strings.LastIndex(value, ","); i >= 0 {
		if _, err := strconv.ParseInt(strings.TrimSpace(value[i+1:]), 10, 32); err == nil {
			value = strings.TrimSpace(value[:i])
		}
	}
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	return value
}
