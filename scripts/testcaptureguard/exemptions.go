package testcaptureguard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ReviewedExemption binds a documented exception to the exact audited file.
// Any source edit invalidates it, including a new write on the same line.
type ReviewedExemption struct {
	File         string `json:"file"`
	Line         int    `json:"line"`
	Target       string `json:"target"`
	SourceSHA256 string `json:"source_sha256"`
	Comment      string `json:"comment"`
}

func ApplyReviewed(report *Report, source []byte, entries []ReviewedExemption) error {
	sum := sha256.Sum256(source)
	hash := hex.EncodeToString(sum[:])
	remaining := make([]Finding, 0, len(report.Issues))
	for _, issue := range report.Issues {
		accepted := false
		for _, entry := range entries {
			if entry.File != issue.File || entry.Line != issue.Line || entry.Target != issue.Target {
				continue
			}
			if entry.SourceSHA256 != hash {
				return fmt.Errorf("%s:%d: audited source changed; review the capture exception again", entry.File, entry.Line)
			}
			reason := strings.TrimPrefix(entry.Comment, "r276-capture-safe: ")
			if reason == entry.Comment || len(reason) < 32 {
				return fmt.Errorf("%s:%d: missing explained capture exception", entry.File, entry.Line)
			}
			issue.Reason = reason
			report.Whitelists = append(report.Whitelists, issue)
			accepted = true
			break
		}
		if !accepted {
			remaining = append(remaining, issue)
		}
	}
	report.Issues = remaining
	return nil
}
