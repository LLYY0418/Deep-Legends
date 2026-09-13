package main

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func copyUI(t *testing.T) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	for _, name := range []string{"installer.html", "license.html", "notice.html", "logo.png"} {
		data, err := uiFiles.ReadFile("ui/" + name)
		if err != nil {
			t.Fatal(err)
		}
		files["ui/"+name] = &fstest.MapFile{Data: data, Mode: 0644}
	}
	return files
}

func TestRenderUIAndUnsafeOrMissingInputs(t *testing.T) {
	result, err := renderUI(`0.11.2 <test>`)
	if err != nil || templateToken.MatchString(result) || !strings.Contains(result, "data:image/png;base64,") || !strings.Contains(result, "0.11.2 &lt;test&gt;") {
		t.Fatalf("render failed: %v", err)
	}
	for _, name := range []string{"license", "notice"} {
		body, _ := uiFiles.ReadFile("ui/" + name + ".html")
		if !strings.Contains(result, string(body)) {
			t.Errorf("%s missing from output", name)
		}
	}
	files := copyUI(t)
	files["ui/installer.html"].Data = append(files["ui/installer.html"].Data, []byte("__FORGOTTEN_SLOT__")...)
	if _, err := renderUIFrom(files, "test"); err == nil {
		t.Fatal("unresolved template token accepted")
	}
	delete(files, "ui/logo.png")
	if _, err := renderUIFrom(files, "test"); !fsIsNotExist(err) {
		t.Fatalf("missing asset accepted: %v", err)
	}
}

func fsIsNotExist(err error) bool {
	return err != nil && strings.Contains(err.Error(), fs.ErrNotExist.Error())
}

func TestDocsAreTemplateSafe(t *testing.T) {
	if !docsAreTemplateSafe() {
		t.Fatal("embedded terms break the UI template literals")
	}
	for _, bad := range []string{"`", "${broken}", "</script>", "</ScRiPt >"} {
		files := copyUI(t)
		files["ui/notice.html"].Data = append(files["ui/notice.html"].Data, []byte(bad)...)
		if checkDocsTemplateSafe(files) == nil {
			t.Errorf("unsafe terms accepted: %q", bad)
		}
		if _, err := renderUIFrom(files, "test"); err == nil {
			t.Errorf("render accepted unsafe terms: %q", bad)
		}
	}
}

func TestUIBridgeContract(t *testing.T) {
	data, _ := uiFiles.ReadFile("ui/installer.html")
	source := string(data)
	if !strings.Contains(source, "window.host") || !strings.Contains(source, "{ type }") {
		t.Fatal("missing JS bridge")
	}
	for _, name := range []string{"drag", "minimize", "close", "browse", "path", "install"} {
		if !strings.Contains(source, `send("`+name+`"`) {
			t.Errorf("missing outbound %s", name)
		}
	}
	for _, name := range []string{"init", "path", "installing", "progress", "done", "failed"} {
		if !regexp.MustCompile(`(?m)^  ` + name + `\([^)]*\) \{`).MatchString(source) {
			t.Errorf("missing host.%s callback", name)
		}
	}
}
