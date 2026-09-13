package main

import (
	"fmt"
	"strings"
)

type uninstallOptions struct {
	Arguments  string `json:"arguments"`
	InstallDir string `json:"installDir"`
}

// _?= is an NSIS tail parameter: it is unquoted and consumes even spaces.
// electron-builder copies our executable to old-uninstaller.exe on upgrades,
// so os.Executable's directory alone cannot locate the installed Core then.
func parseOptions(parameters string) (uninstallOptions, error) {
	options := uninstallOptions{Arguments: strings.TrimSpace(parameters)}
	for offset := 0; offset < len(parameters); {
		start, end := nextToken(parameters, offset)
		if start == end {
			break
		}
		if strings.HasPrefix(parameters[start:], "_?=") {
			options.Arguments = strings.TrimSpace(parameters[:start])
			options.InstallDir = strings.TrimSpace(parameters[start+3:])
			if options.InstallDir == "" || strings.ContainsAny(options.InstallDir, "\"\x00\r\n") {
				return options, fmt.Errorf("invalid NSIS installation directory")
			}
			break
		}
		offset = end
	}
	return options, nil
}

// Tokens retain their original spelling/quoting for forwarding unknown flags.
func nextToken(text string, offset int) (int, int) {
	for offset < len(text) && (text[offset] == ' ' || text[offset] == '\t') {
		offset++
	}
	start, quoted, slashes := offset, false, 0
	for offset < len(text) {
		ch := text[offset]
		if ch == '"' && slashes%2 == 0 {
			quoted = !quoted
		}
		if !quoted && (ch == ' ' || ch == '\t') {
			break
		}
		if ch == '\\' {
			slashes++
		} else {
			slashes = 0
		}
		offset++
	}
	return start, offset
}

func (o uninstallOptions) hasFlag(flag string) bool {
	for offset := 0; offset < len(o.Arguments); {
		start, end := nextToken(o.Arguments, offset)
		if start == end {
			break
		}
		if strings.EqualFold(strings.Trim(o.Arguments[start:end], `"`), flag) {
			return true
		}
		offset = end
	}
	return false
}

func (o uninstallOptions) silent() bool { return o.hasFlag("/S") }

func (o uninstallOptions) coreCommandLine(core, directory string, silent, deleteData bool) string {
	var parts []string
	parts = append(parts, `"`+core+`"`)
	for offset := 0; offset < len(o.Arguments); {
		start, end := nextToken(o.Arguments, offset)
		if start == end {
			break
		}
		token := o.Arguments[start:end]
		flag := strings.Trim(token, `"`)
		if !strings.EqualFold(flag, "/S") && !strings.EqualFold(flag, "--delete-app-data") {
			parts = append(parts, token)
		}
		offset = end
	}
	if silent {
		parts = append(parts, "/S")
	}
	if deleteData {
		parts = append(parts, "--delete-app-data")
	}
	// Prevent NSIS's own self-copy/relaunch, so Wait observes the actual worker.
	return strings.Join(parts, " ") + " _?=" + strings.TrimRight(directory, `\`)
}
