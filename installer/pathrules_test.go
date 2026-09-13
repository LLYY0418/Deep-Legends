package main

import (
	"strings"
	"testing"
)

func TestNormalizeInstallDir(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`  "d:/Games//./Deep Legends/"  `, `D:\Games\Deep Legends`},
		{`c:\Games\old\..\Deep Legends\\`, `C:\Games\Deep Legends`},
		{`e:\中文 文件夹\Deep Legends`, `E:\中文 文件夹\Deep Legends`},
		{`C:\COM10\NUL-safe`, `C:\COM10\NUL-safe`},
		{`C:\` + strings.Repeat("a", 177), `C:\` + strings.Repeat("a", 177)},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := normalizeInstallDir(tc.input)
			if err != nil || got != tc.want {
				t.Fatalf("normalize(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
			}
		})
	}
}

func TestRejectInvalidInstallDirs(t *testing.T) {
	inputs := []string{"", `D:\`, `D:\a\..`, `C:\a\..\..\x`, `C:\..\x`, `C:relative`, `\\server\share\app`, `/tmp/app`, `1:\app`, `C:\bad.`, `C:\bad \app`, `C:\a:b`, `C:\<app>`, `C:\a|b`, `C:\a?b`, `C:\a*b`, `C:\a"b`, "C:\\a\x00b", "C:\\a\nb", "C:\\a\tb", `C:\` + strings.Repeat("a", 178), `C:\` + strings.Repeat("🦁", 89)}
	for _, device := range []string{"CON", "prn", "AUX", "nul", "COM1", "COM9", "lpt1", "LPT9", "NUL.txt", "con .txt"} {
		inputs = append(inputs, `C:\`+device+`\app`)
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			if result, err := normalizeInstallDir(input); err == nil {
				t.Fatalf("accepted invalid path %q as %q", input, result)
			}
		})
	}
}

func TestProductAndDefaultFolders(t *testing.T) {
	for input, want := range map[string]string{
		`D:\`: `D:\Deep Legends`, `D:\Games`: `D:\Games\Deep Legends`,
		`D:\Games\Deep Legends`:  `D:\Games\Deep Legends`,
		`D:\Games\deep legends\`: `D:\Games\deep legends`,
	} {
		if got := appendProductFolder(input); got != want {
			t.Errorf("append(%q) = %q; want %q", input, got, want)
		}
	}
	if got := defaultInstallDir(`C:\Users\玩家\AppData\Local\`); got != `C:\Users\玩家\AppData\Local\Programs\Deep Legends` {
		t.Fatal(got)
	}
}
