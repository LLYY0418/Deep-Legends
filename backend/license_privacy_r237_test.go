//go:build license

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR237LicensePrivacyDisclosureFinal(t *testing.T) {
	const final = "服务端保留授权事件记录（时间、设备摘要前8位、版本号、结果），180 天后自动清理；授权记录与防重放计数器持续保留，用于授权校验。"
	response := httptest.NewRecorder()
	(&app{}).handlePrivacy(response, httptest.NewRequest(http.MethodGet, "/api/privacy", nil))
	var privacy struct {
		LicenseDisclosure string `json:"licenseDisclosure"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &privacy); err != nil {
		t.Fatal(err)
	}
	installer, err := os.ReadFile(filepath.Join("..", "installer", "ui", "license.html"))
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"licenseDisclosure": privacy.LicenseDisclosure, "installer": string(installer)} {
		t.Run(name, func(t *testing.T) {
			if strings.Count(text, final) != 1 {
				t.Fatal("client must contain the exact confirmed retention sentence once")
			}
			for _, forbidden := range []string{"由授权发放方", "删除方式", "联系", "365"} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("client privacy includes an unapproved placeholder, channel or admin audit disclosure: %s", forbidden)
				}
			}
		})
	}
}
