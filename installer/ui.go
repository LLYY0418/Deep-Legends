package main

import (
	"embed"
	"encoding/base64"
	"fmt"
	"html"
	"io/fs"
	"regexp"
	"strings"
)

//go:embed ui/installer.html ui/license.html ui/notice.html ui/logo.png
var uiFiles embed.FS

var templateToken = regexp.MustCompile(`__[A-Z][A-Z0-9_]*__`)

func docsAreTemplateSafe() bool { return checkDocsTemplateSafe(uiFiles) == nil }

func checkDocsTemplateSafe(files fs.FS) error {
	for _, name := range []string{"license", "notice"} {
		data, err := fs.ReadFile(files, "ui/"+name+".html")
		if err != nil {
			return err
		}
		text := string(data)
		if strings.Contains(text, "`") || strings.Contains(text, "${") || strings.Contains(strings.ToLower(text), "</script") {
			return fmt.Errorf("unsafe template literal in %s", name)
		}
	}
	return nil
}

func renderUI(version string) (string, error) { return renderUIFrom(uiFiles, version) }

func renderUIFrom(files fs.FS, version string) (string, error) {
	if err := checkDocsTemplateSafe(files); err != nil {
		return "", err
	}
	contents := make(map[string]string)
	for _, name := range []string{"installer.html", "license.html", "notice.html", "logo.png"} {
		data, err := fs.ReadFile(files, "ui/"+name)
		if err != nil {
			return "", err
		}
		contents[name] = string(data)
	}
	result := strings.NewReplacer(
		"__LOGO__", "data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte(contents["logo.png"])),
		"__VERSION__", html.EscapeString(version),
		"__LICENSE__", contents["license.html"],
		"__NOTICE__", contents["notice.html"],
		// This is a runtime JS identifier, not a template slot. Unicode escapes
		// preserve window.__INIT__ while allowing strict leftover-token checking.
		"__INIT__", `\u005f\u005fINIT\u005f\u005f`,
	).Replace(contents["installer.html"])
	if token := templateToken.FindString(result); token != "" {
		return "", fmt.Errorf("unresolved UI token: %s", token)
	}
	return result, nil
}
