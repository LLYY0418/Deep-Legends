package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"lol-loot-assistant/scripts/testcaptureguard"
)

func main() {
	output := flag.String("json", "", "optional full audit report")
	exceptions := flag.String("exceptions", "scripts/testcaptureguard/reviewed-exceptions.json", "source-bound, explained callback exceptions")
	flag.Parse()
	var reviewed []testcaptureguard.ReviewedExemption
	data, err := os.ReadFile(*exceptions)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := json.Unmarshal(data, &reviewed); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	all := testcaptureguard.Report{Issues: []testcaptureguard.Finding{}, Protected: []testcaptureguard.Finding{}, Whitelists: []testcaptureguard.Finding{}}
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == "dist") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		r, err := testcaptureguard.Analyze(path, data)
		if err != nil {
			return err
		}
		if err := testcaptureguard.ApplyReviewed(&r, data, reviewed); err != nil {
			return err
		}
		all.Files += r.Files
		all.Callbacks += r.Callbacks
		all.Issues = append(all.Issues, r.Issues...)
		all.Protected = append(all.Protected, r.Protected...)
		all.Whitelists = append(all.Whitelists, r.Whitelists...)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *output != "" {
		data, _ := json.MarshalIndent(all, "", "  ")
		if err := os.WriteFile(*output, append(data, '\n'), 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	fmt.Printf("test capture audit: files=%d callbacks=%d mutex_protected=%d explicit_whitelists=%d unsafe=%d\n", all.Files, all.Callbacks, len(all.Protected), len(all.Whitelists), len(all.Issues))
	for _, x := range all.Issues {
		fmt.Printf("%s:%d: %s of captured %s lacks atomic/identified mutex synchronization or an explained whitelist\n", x.File, x.Line, x.Kind, x.Target)
	}
	if len(all.Issues) > 0 {
		os.Exit(1)
	}
}
