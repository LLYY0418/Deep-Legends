package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSGPAuthDiagnosticsPreserveSpecificCausesAndPrivacy(t *testing.T) {
	claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "nbf": time.Now().Add(time.Minute).Unix(), "iat": "invalid", "iss": "https://auth.riotgames.com/private-path", "aud": []string{"sgp", "sensitive-audience"}, "rso_platform_id": "HN10", "sub": "sensitive-account"})
	token := "header." + base64.RawURLEncoding.EncodeToString(claims) + ".sensitive-signature"
	for _, body := range []string{
		`{"message":"Audience mismatch sensitive-account", "errorCode":"UNAUTHORIZED", "error":{"message":"Invalid signature sensitive-secret"}, "httpStatus":401}`,
		`{"errorCode":"UNAUTHORIZED", "error":{"message":"Invalid signature sensitive-secret"}, "message":"Audience mismatch sensitive-account", "httpStatus":401}`,
	} {
		diagnostic := sgpAuthDiagnostic([]byte(body), token)
		if diagnostic["auth_error_class"] != "audience" {
			t.Fatal(diagnostic)
		}
		classes := diagnostic["auth_error_classes"].([]string)
		if strings.Join(classes, ",") != "audience,signature,unauthorized" {
			t.Fatal(classes)
		}
		details := diagnostic["auth_field_details"].(map[string]any)
		if details["message"] == nil || details["error.message"] == nil || details["errorCode"] == nil {
			t.Fatal(details)
		}
		if diagnostic["token_expired"] != false || diagnostic["token_not_yet_valid"] != true || diagnostic["token_claims_verified"] != false {
			t.Fatal(diagnostic)
		}
		encoded, _ := json.Marshal(diagnostic)
		for _, secret := range []string{"sensitive-", "private-path", token} {
			if strings.Contains(string(encoded), secret) {
				t.Fatal(string(encoded))
			}
		}
	}
	diagnostic := sgpTokenDiagnostic(token, "HN10")
	if diagnostic["token_issuer_kind"] != "riot" || diagnostic["token_iat_valid"] != false || diagnostic["token_scope_rso_platform_id"].(map[string]any)["matches_requested_server"] != true {
		t.Fatal(diagnostic)
	}
	for _, token := range []string{"", " opaque ", "a.%%%%.z", "a.bnVsbA.z", "a." + strings.Repeat("X", 16<<10) + ".z"} {
		if _, err := json.Marshal(sgpTokenDiagnostic(token, "HN10")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSGPErrorBodiesAndHeadersStayBounded(t *testing.T) {
	for _, body := range []string{"", "<html>signature validation failed sensitive-secret</html>", `{"message":`, `{"message":"` + strings.Repeat("x", 70<<10) + `"}`} {
		encoded, err := json.Marshal(sgpAuthDiagnostic([]byte(body), "sensitive-token"))
		if err != nil || len(encoded) > 4096 || strings.Contains(string(encoded), "sensitive-") {
			t.Fatalf("err=%v log=%s", err, encoded)
		}
	}
	fields := sgpResponseDiagnostic(&http.Response{Header: http.Header{"Www-Authenticate": {`Bearer error="invalid_token", error_description="audience sensitive-secret"`}, "Location": {"https://sensitive-host/path"}, "Date": {time.Now().UTC().Format(http.TimeFormat)}}})
	encoded, _ := json.Marshal(fields)
	if strings.Contains(string(encoded), "sensitive-") || fields["auth_challenge_present"] != true || fields["location_present"] != true {
		t.Fatal(string(encoded))
	}
}

func TestSGPMethodEvidenceIsFixedAndDoesNotAuthorizeWrites(t *testing.T) {
	response := &http.Response{Header: http.Header{"Allow": {"GET, POST, sensitive-user-token", "OPTIONS"}}}
	fields := sgpResponseDiagnostic(response)
	allowed := fields["allowed_methods"].(map[string]bool)
	if fields["allow_header_present"] != true || len(allowed) != 3 || !allowed["POST"] {
		t.Fatal(fields)
	}
	diagnostic := sgpAuthDiagnostic([]byte(`{"status":{"message":"Method not allowed", "status_code":405}, "errorCode":"METHOD_NOT_ALLOWED", "details":{"code":"sensitive-user-token"}}`), "sensitive-token")
	details := diagnostic["auth_field_details"].(map[string]any)
	if diagnostic["auth_error_class"] != "method-not-allowed" || details["status.status_code"].(map[string]any)["http_status"] != 405 || details["errorCode"].(map[string]any)["known_code"] != "METHOD_NOT_ALLOWED" {
		t.Fatal(diagnostic)
	}
	encoded, _ := json.Marshal([]any{fields, diagnostic})
	if strings.Contains(string(encoded), "sensitive-") {
		t.Fatal(string(encoded))
	}
}
