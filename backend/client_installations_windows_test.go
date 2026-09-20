//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestSplitRegistryPathSupportsNativeTencentKeys(t *testing.T) {
	tests := []struct {
		path   string
		root   registry.Key
		subkey string
		ok     bool
	}{
		{`HKCU\Software\Tencent\LOL`, registry.CURRENT_USER, `Software\Tencent\LOL`, true},
		{`HKEY_CURRENT_USER\Software\Tencent\LOL`, registry.CURRENT_USER, `Software\Tencent\LOL`, true},
		{`HKLM/Software/Tencent/LOL`, registry.LOCAL_MACHINE, `Software\Tencent\LOL`, true},
		{`HKCR\Unsupported`, 0, "", false},
		{"HKCU", 0, "", false},
	}
	for _, test := range tests {
		root, subkey, ok := splitRegistryPath(test.path)
		if root != test.root || subkey != test.subkey || ok != test.ok {
			t.Fatalf("splitRegistryPath(%q) = %v, %q, %t; want %v, %q, %t", test.path, root, subkey, ok, test.root, test.subkey, test.ok)
		}
	}
}
