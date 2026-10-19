// Package releaseinfo exposes the release's bundled CHANGELOG without a second
// generated version file or a dependency on the source tree at runtime.
package releaseinfo

import (
	"embed"
	"strings"
)

//go:embed CHANGELOG.md
var files embed.FS

func Notes(version string) string {
	data, _ := files.ReadFile("CHANGELOG.md")
	active := false
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ") {
			if active {
				break
			}
			heading := strings.TrimPrefix(line, "## ")
			first := strings.Fields(heading)
			active = len(first) > 0 && strings.Trim(first[0], "[]v") == strings.TrimPrefix(version, "v")
		}
		if active {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
