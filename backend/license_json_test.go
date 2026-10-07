//go:build license

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

type licenseR234Embedded struct {
	Status string `json:"status"`
}
type LicenseR234Pointer = licenseR234Embedded
type licenseR234Nested struct {
	Version int `json:"version"`
}
type licenseR234Container struct {
	licenseR234Embedded
	Nested  *licenseR234Nested           `json:"nested,omitempty"`
	Rows    []licenseR234Nested          `json:"rows"`
	ByID    map[string]licenseR234Nested `json:"by_id"`
	Ignored string                       `json:"-"`
}

func TestR234ExactJSONNames(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		target      any
	}{
		{"title-request", `{"Version":1}`, &licenseRequest{}},
		{"upper-request", `{"VERSION":1}`, &licenseRequest{}},
		{"status-and-alias", `{"status":"ACTIVE","Status":"ERROR"}`, &licensePayload{}},
		{"envelope", `{"Algorithm":"Ed25519"}`, &signedLicenseEnvelope{}},
		{"disk", `{"last_wall_time":1,"Last_Wall_Time":2}`, &licenseDiskState{}},
		{"disk-envelope", `{"lease":{"Kid":"test"}}`, &licenseDiskState{}},
		{"activation-input", `{"Code":"test"}`, &struct {
			Code string `json:"code"`
		}{}},
		{"fixture", `{"Ready":true}`, &licenseProtocolVectors{}},
		{"fixture-row", `{"strict_json":[{"Valid":true}]}`, &licenseProtocolVectors{}},
		{"embedded", `{"Status":"ACTIVE"}`, &licenseR234Container{}},
		{"nested", `{"nested":{"Version":1}}`, &licenseR234Container{}},
		{"array-element", `{"rows":[{"VERSION":1}]}`, &licenseR234Container{}},
		{"map-value", `{"by_id":{"Kid-Is-Data":{"Version":1}}}`, &licenseR234Container{}},
		{"ignored", `{"Ignored":"hidden"}`, &licenseR234Container{}},
		{"escaped-alias", `{"\u0056ersion":1}`, &licenseRequest{}},
		{"escaped-duplicate", `{"version":1,"\u0076ersion":2}`, &licenseRequest{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strictLicenseJSON([]byte(tc.input), tc.target) == nil {
				t.Fatal("non-contract field name accepted")
			}
		})
	}
	var exact licenseR234Container
	if err := strictLicenseJSON([]byte(`{"status":"ACTIVE","nested":{"version":1},"rows":[{"version":2}],"by_id":{"Kid-Is-Data":{"version":3}}}`), &exact); err != nil || exact.Status != "ACTIVE" || exact.Nested.Version != 1 || exact.Rows[0].Version != 2 || exact.ByID["Kid-Is-Data"].Version != 3 {
		t.Fatal("exact names or dynamic map keys rejected", err)
	}
	// An explicitly tagged outer field dominates a promoted inner field.
	var shadow struct {
		licenseR234Embedded
		Outer string `json:"status"`
	}
	if strictLicenseJSON([]byte(`{"status":"outer"}`), &shadow) != nil || shadow.Outer != "outer" || shadow.Status != "" {
		t.Fatal("embedded field precedence differs from typed JSON decoding")
	}
	var pointer struct{ *LicenseR234Pointer }
	if strictLicenseJSON([]byte(`{"status":"ACTIVE"}`), &pointer) != nil || pointer.LicenseR234Pointer == nil || pointer.Status != "ACTIVE" {
		t.Fatal("anonymous pointer fields no longer decode")
	}
	// Construct the deliberately invalid schema at runtime so vet can still
	// reject accidental duplicate tags in ordinary source declarations.
	ambiguous := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(""), Tag: `json:"status"`},
		{Name: "B", Type: reflect.TypeOf(""), Tag: `json:"status"`},
	})
	if strictLicenseJSON([]byte(`{"status":"ACTIVE"}`), reflect.New(ambiguous).Interface()) == nil {
		t.Fatal("ambiguous promoted field accepted")
	}
	var mapped map[string][]licenseR234Nested
	if strictLicenseJSON([]byte(`{"DynamicID":[{"version":1}]}`), &mapped) != nil || mapped["DynamicID"][0].Version != 1 {
		t.Fatal("map slice schema rejected")
	}
	if strictLicenseJSON([]byte(`{"DynamicID":[{"Version":1}]}`), &mapped) == nil {
		t.Fatal("map slice element alias accepted")
	}
	for _, target := range []any{nil, (*licenseRequest)(nil), licenseRequest{}} {
		if strictLicenseJSON([]byte(`{}`), target) == nil {
			t.Fatal("invalid target accepted")
		}
	}
}

func TestR234LocalLicenseStateRoundTrip(t *testing.T) {
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	data, err := json.Marshal(m.disk)
	if err != nil {
		t.Fatal(err)
	}
	var restored licenseDiskState
	if err := strictLicenseJSON(data, &restored); err != nil || !reflect.DeepEqual(restored, m.disk) {
		t.Fatal("local state no longer round-trips with exact tagged names", err)
	}
	// Readable cache still restores offline after the stricter decoder.
	store := &licenseTestStore{data: data}
	s.offline.Store(true)
	if !s.Manager(store).Allowed() {
		t.Fatal("valid exact-name cached lease rejected")
	}
}

func TestR234SignedUpdateJSONNames(t *testing.T) {
	m := updateTestManifest([]byte("setup fixture"))
	unsigned := m
	unsigned.SignedManifest = nil
	payload, _ := json.Marshal(unsigned)
	for _, field := range []string{"schema", "version", "asset", "sha256", "publishedAt", "minSupported"} {
		t.Run("signed-payload/"+field, func(t *testing.T) {
			badBytes := bytes.Replace(payload, []byte(`"`+field+`"`), []byte(`"`+strings.ToUpper(field)+`"`), 1)
			encoded := base64.RawURLEncoding.EncodeToString(badBytes)
			sig := ed25519.Sign(testUpdatePrivate, []byte("DL-UPDATE-MANIFEST-V1\ntest-update\n"+encoded))
			envelope := signedLicenseEnvelope{"Ed25519", "test-update", encoded, base64.RawURLEncoding.EncodeToString(sig)}
			bad := m
			bad.SignedManifest = &envelope
			if verifyUpdateManifestTrust(bad, testUpdateTrust()) == nil {
				t.Fatal("signed alias accepted")
			}
		})
	}
	outer, _ := json.Marshal(m)
	u := updateTestManager(t, []byte("setup fixture"))
	for _, field := range []string{"schema", "signed_manifest", "algorithm", "kid", "asset", "url"} {
		t.Run("network-envelope/"+field, func(t *testing.T) {
			bad := bytes.Replace(outer, []byte(`"`+field+`"`), []byte(`"`+strings.ToUpper(field)+`"`), 1)
			u.client = &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(200, bad), nil })}
			if _, err := u.fetchManifestSource(context.Background(), updateManifestPath); err == nil {
				t.Fatal("network alias accepted")
			}
		})
	}
	u.client = &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(200, outer), nil })}
	if _, err := u.fetchManifestSource(context.Background(), updateManifestPath); err != nil {
		t.Fatal("exact signed manifest rejected", err)
	}
}

func TestR234VectorKeysExcludedFromTrustConfiguration(t *testing.T) {
	v := loadLicenseProtocolVectors(t)
	for kid, encoded := range v.PublicKeys {
		pub, err := rawURLDecode(encoded, ed25519.PublicKeySize)
		if err != nil {
			t.Fatal(err)
		}
		for _, keys := range []map[string]ed25519.PublicKey{licenseTrustKeys(), updateTrustKeys()} {
			if _, ok := keys[kid]; ok {
				t.Fatal("vector kid is trusted by " + licenseBuildLabel)
			}
			for _, trusted := range keys {
				if bytes.Equal(pub, trusted) {
					t.Fatal("vector public key is trusted by " + licenseBuildLabel)
				}
			}
		}
		// Audit both mutually exclusive configurations in either build mode.
		for _, file := range []string{"license_config_release.go", "license_config_staging.go"} {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(source, []byte(kid)) || bytes.Contains(source, []byte(encoded)) {
				t.Fatal("vector trust marker in " + file)
			}
		}
	}
}
