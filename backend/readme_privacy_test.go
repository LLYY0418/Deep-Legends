package main

import (
	"os"
	"strings"
	"testing"
)

func TestSessionTokenPrivacyDocumentationMatchesDiskPersistence(t *testing.T) {
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	sessionLine := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "桌面主程序会隐藏启动") {
			sessionLine = line
			break
		}
	}
	if sessionLine == "" {
		t.Fatal("README is missing the desktop session-token paragraph")
	}
	for _, forbidden := range []string{"每次随机生成的会话令牌", "令牌只保存在进程内存中", "不写入磁盘"} {
		if strings.Contains(sessionLine, forbidden) {
			t.Fatalf("README session-token paragraph contains obsolete claim %q", forbidden)
		}
	}
	for _, required := range []string{"session-token", "0600", "0700", "127.0.0.1", "不会上传服务器"} {
		if !strings.Contains(text, required) {
			t.Fatalf("README is missing session-token disclosure %q", required)
		}
	}
}
