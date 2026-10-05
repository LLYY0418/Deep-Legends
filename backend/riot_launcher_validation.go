package main

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

var errRiotProductUnavailable = errors.New("Riot client has no usable League installation")

func isTencentInstallPath(value string) bool {
	value = strings.ToLower(strings.ReplaceAll(value, `\`, "/"))
	for _, marker := range []string{"wegame", "tencent", "腾讯", "英雄联盟", "/tcls/"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func riotCandidatesFromJSON(data []byte, guesses []string) []clientLaunchCandidate {
	var payload map[string]any
	_ = json.Unmarshal(data, &payload)
	result := []clientLaunchCandidate{}
	seen := map[string]bool{}
	add := func(value, source string) {
		value = strings.Trim(value, ` "`)
		normalized := strings.ToLower(strings.ReplaceAll(value, `\`, "/"))
		if !strings.HasSuffix(normalized, "/riotclientservices.exe") || isTencentInstallPath(value) || seen[normalized] {
			return
		}
		seen[normalized] = true
		result = append(result, clientLaunchCandidate{Source: source, executable: value})
	}
	for _, key := range []string{"rc_default", "rc_live"} {
		if value, ok := payload[key].(string); ok {
			add(value, key)
		}
	}
	var collect func(any)
	collect = func(value any) {
		switch v := value.(type) {
		case string:
			add(v, "installs-json-other")
		case []any:
			for _, entry := range v {
				collect(entry)
			}
		case map[string]any:
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				collect(key)
				collect(v[key])
			}
		}
	}
	collect(payload)
	for _, guess := range guesses {
		add(guess, "drive-guess")
	}
	return result
}

// Read the single scalar without loading or logging unrelated metadata.
func riotProductInstallPath(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && key == "product_install_full_path" {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return ""
}
func classifyRiotProductInstall(data []byte, exists func(string) bool) string {
	value := riotProductInstallPath(data)
	if value == "" {
		return "missing"
	}
	if isTencentInstallPath(value) {
		return "tencent"
	}
	if !exists(value) {
		return "missing"
	}
	return "riot"
}
