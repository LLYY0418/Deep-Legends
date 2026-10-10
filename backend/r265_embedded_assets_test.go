package main

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// Build-tag templates are exposed through different public route names.
// CJS test and harness files are deliberately not production JS/CSS resources.
var r265SourceOnlyAssets = map[string]string{
	"default/index.html":    "default-tag HTML is exposed as web/index.html",
	"default/license-ui.js": "default-tag stub is exposed as web/license-ui.js",
	"loot-icons/README.md":  "local icon provenance notes; no runtime reference",
}

func r265EmbeddedAssetProblems(source, release fs.FS) []string {
	problems := []string{}
	required := map[string]bool{}
	add := func(reference string) {
		u, err := url.Parse(reference)
		if err != nil || u.IsAbs() || u.Host != "" || strings.HasPrefix(reference, "#") || strings.Contains(reference, "{") {
			return
		}
		name := strings.TrimPrefix(path.Clean(u.Path), "/")
		if name != "" && name != "." && !strings.HasPrefix(name, "api/") {
			required[name] = true
		}
	}
	htmlReferences := regexp.MustCompile("(?i)\\b(?:src|href)\\s*=\\s*[\"']([^\"']+)[\"']")
	staticResources := regexp.MustCompile("[\"'\\x60](/?[-a-zA-Z0-9_./]+\\.(?:js|css)(?:\\?[^\"'\\x60]*)?)[\"'\\x60]")
	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if _, ok := r265SourceOnlyAssets[name]; ok {
			return nil
		}
		extension := path.Ext(name)
		iconDirectory := strings.Contains(name, "-icons/") || strings.HasPrefix(name, "rune-styles/") || strings.HasPrefix(name, "rank-crests/")
		production := iconDirectory || extension == ".js" || extension == ".css" ||
			extension == ".svg" || extension == ".png" || extension == ".woff2"
		if production {
			required[name] = true
		}
		if name != "index.html" && extension != ".js" && extension != ".css" {
			return nil
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		if name == "index.html" {
			for _, match := range htmlReferences.FindAllStringSubmatch(string(data), -1) {
				add(match[1])
			}
		} else {
			for _, match := range staticResources.FindAllStringSubmatch(string(data), -1) {
				add(match[1])
			}
		}
		// Resolve the dynamic script/link names from the loader's source maps.
		if name == "section-loader.js" {
			maps := regexp.MustCompile("const (modules|styles) = \\{([^;]+)\\};")
			values := regexp.MustCompile("\\[([^]]+)\\]")
			names := regexp.MustCompile("\"([a-z-]+)\"")
			for _, mapping := range maps.FindAllStringSubmatch(string(data), -1) {
				suffix := ".js"
				if mapping[1] == "styles" {
					suffix = ".css"
				}
				for _, list := range values.FindAllStringSubmatch(mapping[2], -1) {
					for _, module := range names.FindAllStringSubmatch(list[1], -1) {
						required[module[1]+suffix] = true
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	if data, err := fs.ReadFile(release, "web/index.html"); err != nil {
		problems = append(problems, "release index.html: "+err.Error())
	} else {
		for _, match := range htmlReferences.FindAllStringSubmatch(string(data), -1) {
			add(match[1])
		}
	}
	for name := range required {
		data, err := fs.ReadFile(release, "web/"+name)
		if err != nil || len(data) == 0 {
			problems = append(problems, fmt.Sprintf("missing or empty release resource %s", name))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestR265AllReleaseEmbeddedAssets(t *testing.T) {
	if problems := r265EmbeddedAssetProblems(os.DirFS("web"), embedded); len(problems) != 0 {
		t.Fatal(strings.Join(problems, "\n"))
	}
}

func TestR265EmbeddedGuardFindsInventoryAndLazyReferences(t *testing.T) {
	source := fstest.MapFS{
		"index.html":        {Data: []byte("<script src='/entry.js'></script>")},
		"entry.js":          {Data: []byte("import('/lazy.js');fetch('/fetched.js');")},
		"section-loader.js": {Data: []byte("const modules = { demo: [\"unlisted\"] };const styles = { demo: [\"unlisted\"] };")},
		"unreferenced.js":   {Data: []byte("production")},
		"fixture.test.cjs":  {Data: []byte("not production")},
	}
	release := fstest.MapFS{"web/index.html": {Data: source["index.html"].Data}, "web/entry.js": {Data: []byte("ok")}, "web/section-loader.js": {Data: []byte("ok")}}
	problems := strings.Join(r265EmbeddedAssetProblems(source, release), "\n")
	for _, name := range []string{"lazy.js", "fetched.js", "unreferenced.js", "unlisted.js", "unlisted.css"} {
		if !strings.Contains(problems, name) {
			t.Fatalf("guard missed %s: %s", name, problems)
		}
	}
	if strings.Contains(problems, "fixture.test.cjs") {
		t.Fatal("test fixture entered production inventory")
	}
}
