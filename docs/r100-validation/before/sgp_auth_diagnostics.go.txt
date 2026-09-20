package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Only emit fixed categories and booleans. SGP error bodies and JWT claims may
// contain account identifiers or credentials and must never be logged verbatim.
func sgpAuthDiagnostic(body []byte, token string) map[string]any {
	fields := map[string]any{"auth_error_class": "unclassified", "auth_body_shape": diagnosticPayloadPrefixShape(body)}
	fields["auth_body_truncated"] = len(body) > 64<<10
	if len(body) > 64<<10 {
		body = body[:64<<10]
	}
	var payload map[string]json.RawMessage
	parsed := json.Unmarshal(body, &payload) == nil && payload != nil
	fields["auth_json_valid"] = parsed
	known, details := []string{}, map[string]any{}
	unknownFields, unknownCodes, depthLimited := 0, 0, 0
	unknownShapes, unknownSizes := map[string]int{}, map[string]int{}
	all := map[string]bool{}
	var inspect func(map[string]json.RawMessage, string, int)
	inspect = func(object map[string]json.RawMessage, prefix string, depth int) {
		unknownHere := len(object)
		for key, raw := range object {
			switch key {
			case "error", "message", "error_description", "errorCode", "code", "httpStatus", "status", "status_code", "details", "cause":
				continue
			}
			unknownShapes[diagnosticJSONValueKind(raw)]++
			size := "over-16k"
			switch {
			case len(raw) <= 64:
				size = "up-to-64"
			case len(raw) <= 1024:
				size = "up-to-1k"
			case len(raw) <= 16<<10:
				size = "up-to-16k"
			}
			unknownSizes[size]++
		}
		for _, key := range []string{"error", "message", "error_description", "errorCode", "code", "httpStatus", "status", "status_code", "details", "cause"} {
			raw, present := object[key]
			if !present {
				continue
			}
			unknownHere--
			name := prefix + key // fixed keys only; never arbitrary upstream keys
			known = append(known, name)
			detail := map[string]any{"shape": diagnosticPayloadPrefixShape(raw), "bytes": len(raw)}
			var value string
			if json.Unmarshal(raw, &value) == nil {
				// Exact, fixed protocol codes distinguish upstream errors without
				// exporting arbitrary messages, identifiers or credential values.
				if key == "errorCode" || key == "code" || key == "error" {
					switch value {
					case "UNAUTHORIZED", "FORBIDDEN", "METHOD_NOT_ALLOWED", "NOT_FOUND", "INVALID_TOKEN", "TOKEN_EXPIRED", "INSUFFICIENT_SCOPE", "ACCESS_DENIED", "INVALID_AUDIENCE", "INVALID_SIGNATURE", "INVALID_ISSUER", "TOKEN_NOT_YET_VALID", "INVALID_SESSION", "REGION_MISMATCH", "PRIVACY_RESTRICTED":
						detail["known_code"] = value
					default:
						detail["known_code"] = "unknown"
						unknownCodes++
					}
				}
				classes := sgpAuthClasses(value)
				detail["classes"] = classes
				for _, class := range classes {
					all[class] = true
				}
			}
			var status int
			if (key == "httpStatus" || key == "status" || key == "status_code") && json.Unmarshal(raw, &status) == nil && status >= 100 && status <= 599 {
				detail["http_status"] = status
			}
			details[name] = detail
			var nested map[string]json.RawMessage
			if json.Unmarshal(raw, &nested) == nil && nested != nil {
				if depth < 2 {
					inspect(nested, name+".", depth+1)
				} else {
					depthLimited++
				}
			}
		}
		unknownFields += unknownHere
	}
	if parsed {
		inspect(payload, "", 0)
	} else {
		// HTML/plaintext errors can still carry a useful category, never raw text.
		for _, class := range sgpAuthClasses(string(body)) {
			all[class] = true
		}
	}
	classes := []string{}
	for _, class := range sgpAuthCategoryOrder {
		if all[class] {
			classes = append(classes, class)
		}
	}
	if len(classes) > 0 {
		fields["auth_error_class"] = classes[0]
	}
	fields["auth_error_classes"], fields["auth_known_fields"], fields["auth_field_details"] = classes, known, details
	fields["auth_unknown_field_count"], fields["auth_unknown_code_count"], fields["auth_depth_limited_count"] = unknownFields, unknownCodes, depthLimited
	fields["auth_unknown_field_shapes"], fields["auth_unknown_field_size_buckets"] = unknownShapes, unknownSizes
	return mergeDiagnosticFields(fields, sgpTokenDiagnostic(token, ""))
}

// Fixed JSON kinds only; never return a field name or its value.
func diagnosticJSONValueKind(raw []byte) string {
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "missing"
	}
	switch value[0] {
	case '"':
		return "string"
	case '{':
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// Specific causes precede generic HTTP categories, independent of JSON field order.
var sgpAuthCategoryOrder = []string{"expired", "not-yet-valid", "audience", "issuer", "signature", "scope", "privacy", "permission", "region", "session", "invalid token", "missing-token", "method-not-allowed", "forbidden", "unauthorized"}

func sgpAuthClasses(value string) []string {
	value = strings.NewReplacer("_", " ", "-", " ").Replace(strings.ToLower(value))
	aliases := map[string][]string{
		"expired": {"expired", "expiration"}, "not-yet-valid": {"not yet valid", "not before"},
		"audience": {"audience"}, "issuer": {"issuer"}, "signature": {"signature"},
		"scope": {"scope"}, "permission": {"permission", "access denied", "not authorized"},
		"privacy": {"privacy", "private profile", "private history"},
		"region":  {"region", "platform", "shard"}, "session": {"invalid session", "session not found", "session expired"},
		"invalid token": {"invalid token", "invalid bearer"}, "missing-token": {"missing token", "missing authorization"},
		"method-not-allowed": {"method not allowed", "method is not allowed"},
		"forbidden":          {"forbidden"}, "unauthorized": {"unauthorized", "unauthorised"},
	}
	result := []string{}
	for _, class := range sgpAuthCategoryOrder {
		for _, alias := range aliases[class] {
			if strings.Contains(value, alias) {
				result = append(result, class)
				break
			}
		}
	}
	return result
}

func sgpTokenDiagnostic(token, server string) map[string]any {
	fields := map[string]any{"token_claims_verified": false, "token_present": token != ""}
	parts := strings.Split(token, ".")
	fields["token_has_outer_whitespace"] = strings.TrimSpace(token) != token
	fields["token_jwt_shape"] = len(parts) == 3
	if len(parts) == 3 && len(parts[0]) < 4<<10 {
		decoded, err := base64.RawURLEncoding.DecodeString(parts[0])
		var header struct {
			Algorithm string          `json:"alg"`
			KeyID     json.RawMessage `json:"kid"`
		}
		valid := err == nil && json.Unmarshal(decoded, &header) == nil
		fields["token_header_decodable"] = valid
		if valid {
			algorithm := "other"
			switch header.Algorithm {
			case "RS256", "RS384", "RS512", "ES256", "ES384", "ES512", "HS256", "none":
				algorithm = header.Algorithm
			}
			fields["token_algorithm"], fields["token_key_id_present"] = algorithm, len(header.KeyID) > 0
		}
	}
	if len(parts) == 3 && len(parts[1]) < 16<<10 {
		decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
		var claims map[string]json.RawMessage
		valid := err == nil && json.Unmarshal(decoded, &claims) == nil && claims != nil
		fields["token_claims_decodable"] = valid
		if valid {
			now := time.Now().Unix()
			for _, key := range []string{"exp", "nbf", "iat"} {
				var stamp int64
				raw, present := claims[key]
				ok := present && json.Unmarshal(raw, &stamp) == nil && stamp > 0
				fields["token_"+key+"_present"], fields["token_"+key+"_valid"] = present, ok
				if ok {
					fields["token_"+key+"_relative"] = sgpTimeBucket(stamp - now)
				}
				if key == "exp" {
					fields["token_has_expiry"], fields["token_expired"] = ok, ok && stamp <= now
				}
				if key == "nbf" {
					fields["token_not_yet_valid"] = ok && stamp > now
				}
			}
			fields["token_issuer_present"] = len(claims["iss"]) > 0
			var issuer string
			_ = json.Unmarshal(claims["iss"], &issuer)
			issuerKind := "other"
			if u, err := url.Parse(issuer); err == nil {
				host := strings.ToLower(u.Hostname())
				if host == "riotgames.com" || strings.HasSuffix(host, ".riotgames.com") {
					issuerKind = "riot"
				}
				if host == "lol.qq.com" || strings.HasSuffix(host, ".lol.qq.com") {
					issuerKind = "tencent-lol"
				}
			}
			fields["token_issuer_kind"] = issuerKind
			var audiences []string
			if json.Unmarshal(claims["aud"], &audiences) != nil {
				var audience string
				if json.Unmarshal(claims["aud"], &audience) == nil {
					audiences = []string{audience}
				}
			}
			fields["token_audience_present"], fields["token_audience_count"] = len(claims["aud"]) > 0, len(audiences)
			audienceKinds := map[string]bool{}
			for _, audience := range audiences {
				switch audience {
				case "lol", "sgp", "gsm", "spectator", "entitlements", "rso", "league", "league-session":
					audienceKinds[audience] = true
				default:
					audienceKinds["other"] = true
				}
			}
			fields["token_audience_kinds"] = audienceKinds
			for _, key := range []string{"platform_id", "rso_platform_id", "platformId", "region", "shard"} {
				if raw, exists := claims[key]; exists {
					var value string
					valid := json.Unmarshal(raw, &value) == nil && value != ""
					fields["token_scope_"+key] = map[string]any{"valid": valid, "matches_requested_server": valid && server != "" && strings.EqualFold(value, server)}
				}
			}
		}
	}
	return fields
}

func sgpTimeBucket(seconds int64) string {
	switch {
	case seconds < -300:
		return "past-over-5m"
	case seconds < 0:
		return "past-under-5m"
	case seconds <= 60:
		return "within-1m"
	case seconds <= 300:
		return "within-5m"
	case seconds <= 3600:
		return "within-1h"
	default:
		return "over-1h"
	}
}

func sgpResponseDiagnostic(response *http.Response) map[string]any {
	fields := map[string]any{"content_type": "other", "content_length": response.ContentLength, "auth_challenge_present": response.Header.Get("WWW-Authenticate") != "", "location_present": response.Header.Get("Location") != ""}
	contentType := strings.ToLower(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	for _, known := range []string{"application/json", "application/problem+json", "text/html", "text/plain"} {
		if contentType == known {
			fields["content_type"] = known
		}
	}
	fields["auth_challenge_classes"] = sgpAuthClasses(response.Header.Get("WWW-Authenticate"))
	allowed := map[string]bool{}
	allowHeaders := response.Header.Values("Allow")
	fields["allow_header_present"] = len(allowHeaders) > 0
	for _, header := range allowHeaders {
		if len(header) > 1024 {
			continue
		}
		for _, method := range strings.Split(header, ",") {
			switch strings.TrimSpace(method) {
			case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
				allowed[strings.TrimSpace(method)] = true
			}
		}
	}
	fields["allowed_methods"] = allowed // Evidence only; never authorizes a different method.
	if date, err := http.ParseTime(response.Header.Get("Date")); err == nil {
		fields["server_clock_relative"] = sgpTimeBucket(date.Unix() - time.Now().Unix())
	}
	fields["retry_after_present"] = response.Header.Get("Retry-After") != ""
	// Header values can contain opaque credentials. Record only fixed names
	// and presence, so a missing server correlation/challenge is explicit.
	correlation := map[string]bool{}
	for _, name := range []string{"X-Request-Id", "X-Correlation-Id", "Traceparent", "X-B3-Traceid"} {
		correlation[name] = response.Header.Get(name) != ""
	}
	fields["correlation_headers_present"] = correlation
	return fields
}
