package ui

import (
	"embed"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

//go:embed uninstaller.html logo.png
var uninstallFiles embed.FS

// RenderUninstaller only substitutes assets; the supplied design is unchanged.
func RenderUninstaller() (string, error) {
	html, err := uninstallFiles.ReadFile("uninstaller.html")
	if err != nil {
		return "", err
	}
	logo, err := uninstallFiles.ReadFile("logo.png")
	if err != nil {
		return "", err
	}
	result := strings.NewReplacer("__LOGO__", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(logo),
		"__INIT__", `\u005f\u005fINIT\u005f\u005f`).Replace(string(html))
	if token := regexp.MustCompile(`__[A-Z][A-Z0-9_]*__`).FindString(result); token != "" {
		return "", fmt.Errorf("unresolved uninstaller UI token: %s", token)
	}
	return result, nil
}
