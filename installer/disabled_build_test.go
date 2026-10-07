//go:build !license

package main

import (
	"strings"
	"testing"
)

func TestR248DefaultInstallerNoActivationText(t *testing.T) {
	page, err := renderUI("0.12.75")
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"注册码", "授权凭据", "license.yinxiaobia.net", "license-staging.yinxiaobia.net"} {
		if strings.Contains(page, term) {
			t.Fatal("default installer contains activation text", term)
		}
	}
}
