package main

import (
	"net/url"
	"strings"
)

// Treat client diagnostic fields as untrusted: never persist credentials,
// resource paths, payloads, query strings, or fragments from a blocked URL.
func safeBrowserBlockedOrigin(raw string) string {
	if len(raw) > 4096 {
		return ""
	}
	raw = strings.TrimSpace(raw)
	for _, scheme := range []string{"data", "blob", "file"} {
		lower := strings.ToLower(raw)
		if lower == scheme || strings.HasPrefix(lower, scheme+":") {
			return scheme + ":"
		}
	}
	value, err := url.Parse(raw)
	if err != nil || value.Host == "" || value.Scheme != "http" && value.Scheme != "https" || len(value.Host) > 255 {
		return ""
	}
	host := strings.ToLower(value.Host)
	if strings.IndexFunc(host, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune(".-:[]", r))
	}) >= 0 {
		return ""
	}
	return value.Scheme + "://" + host
}
