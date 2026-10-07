//go:build license

package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// These are test-file container fields, never fields in the license protocol.
type licenseProtocolVectors struct {
	FixtureVersion  int               `json:"fixture_version"`
	ProtocolVersion int               `json:"protocol_version"`
	Ready           bool              `json:"ready"`
	Source          string            `json:"source"`
	PublicKeys      map[string]string `json:"public_keys"`
	DeviceSeedHex   string            `json:"device_seed_hex"`
	Normalization   []struct {
		ID     string `json:"id"`
		Input  string `json:"input"`
		Output string `json:"output"`
		Valid  bool   `json:"valid"`
	} `json:"normalization"`
	Base64URL []struct {
		ID       string `json:"id"`
		Input    string `json:"input"`
		Size     int    `json:"size"`
		BytesHex string `json:"bytes_hex"`
		Valid    bool   `json:"valid"`
	} `json:"base64url"`
	StrictJSON []struct {
		ID    string `json:"id"`
		Kind  string `json:"kind"`
		JSON  string `json:"json"`
		Valid bool   `json:"valid"`
	} `json:"strict_json"`
	Requests []struct {
		ID    string `json:"id"`
		Path  string `json:"path"`
		JSON  string `json:"json"`
		Text  string `json:"text"`
		Valid bool   `json:"valid"`
	} `json:"requests"`
	Responses []struct {
		ID        string `json:"id"`
		JSON      string `json:"json"`
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
		Payload   string `json:"payload"`
		Category  string `json:"category"`
		Valid     bool   `json:"valid"`
	} `json:"responses"`
}

func loadLicenseProtocolVectors(t *testing.T) licenseProtocolVectors {
	t.Helper()
	data, err := os.ReadFile("testdata/license-protocol-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	if digest := sha256.Sum256(data); hex.EncodeToString(digest[:]) != "39377b860440d8f609f9d28162afe086e4e01ac91d3d2ff39da2a45d5eebfcc7" {
		t.Fatal("shared vectors differ from the unmodified service handoff")
	}
	var vectors licenseProtocolVectors
	if err := strictLicenseJSON(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.FixtureVersion != 1 || vectors.ProtocolVersion != 1 || !vectors.Ready || vectors.Source == "" || vectors.Source == "pending-deep-legends-manage" {
		t.Fatal("unknown shared vector format/protocol/source")
	}
	if len(vectors.Normalization) != 9 || len(vectors.Base64URL) != 5 || len(vectors.StrictJSON) != 11 || len(vectors.Requests) != 10 || len(vectors.Responses) != 34 || len(vectors.PublicKeys) == 0 {
		t.Fatal("shared vector coverage is missing from the 69-case R246 service handoff")
	}
	return vectors
}
func TestR233ProtocolParserVectors(t *testing.T) {
	v := loadLicenseProtocolVectors(t)
	if len(v.Normalization) == 0 || len(v.Base64URL) == 0 || len(v.StrictJSON) == 0 {
		t.Fatal("parser coverage cannot silently disappear")
	}
	for _, row := range v.Normalization {
		t.Run("normalize/"+row.ID, func(t *testing.T) {
			value, err := normalizeLicenseCode(row.Input)
			if (err == nil) != row.Valid || row.Valid && value != row.Output {
				t.Fatal(value, err)
			}
		})
	}
	for _, row := range v.Base64URL {
		t.Run("base64/"+row.ID, func(t *testing.T) {
			value, err := rawURLDecode(row.Input, row.Size)
			if (err == nil) != row.Valid || row.Valid && hex.EncodeToString(value) != row.BytesHex {
				t.Fatal("base64 vector mismatch", err)
			}
		})
	}
	for _, row := range v.StrictJSON {
		t.Run("json/"+row.ID, func(t *testing.T) {
			var target any
			switch row.Kind {
			case "request":
				target = &licenseRequest{}
			case "payload":
				target = &licensePayload{}
			case "envelope":
				target = &signedLicenseEnvelope{}
			default:
				t.Fatal("unknown vector parser")
			}
			err := strictLicenseJSON([]byte(row.JSON), target)
			if (err == nil) != row.Valid {
				t.Fatal("strict JSON vector mismatch", err)
			}
		})
	}
}
func TestR233RequestSignatureTextContract(t *testing.T) {
	loadLicenseProtocolVectors(t) // All three S15 entries require the final fixture.
	request := licenseRequest{Version: 1, RequestID: strings.Repeat("0", 32), DevicePub: "DEVICE_PUBLIC_KEY_PLACEHOLDER", TS: 1791244800, Counter: "42", Payload: "e30"}
	for _, path := range []string{"/v1/activate", "/v1/renew"} {
		text, err := licenseRequestSignatureText(path, request)
		want := "DL-LICENSE-REQUEST-V1\nPOST\n" + path + "\n00000000000000000000000000000000\nDEVICE_PUBLIC_KEY_PLACEHOLDER\n1791244800\n42\n44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
		if err != nil || text != want {
			t.Fatal("request signature contract changed", text, err)
		}
	}
	request.Payload = "e30="
	if _, err := licenseRequestSignatureText("/v1/activate", request); err == nil {
		t.Fatal("noncanonical signed payload accepted")
	}
}
func TestR233SharedSignedProtocolVectors(t *testing.T) {
	v := loadLicenseProtocolVectors(t)
	seed, err := hex.DecodeString(v.DeviceSeedHex)
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatal("shared test device seed is missing")
	}
	key := ed25519.NewKeyFromSeed(seed)
	keys := map[string]ed25519.PublicKey{}
	for kid, encoded := range v.PublicKeys {
		pub, err := rawURLDecode(encoded, ed25519.PublicKeySize)
		if err != nil {
			t.Fatal("invalid shared test public key")
		}
		keys[kid] = pub
	}
	if len(keys) == 0 || len(v.Requests) == 0 || len(v.Responses) == 0 {
		t.Fatal("ready shared vectors require public keys, requests and responses")
	}
	paths := map[string]bool{}
	outcomes := map[string]bool{}
	negative := map[string]bool{}
	kinds := map[string]bool{}
	for _, row := range v.Requests {
		t.Run("request/"+row.ID, func(t *testing.T) {
			var request licenseRequest
			err := strictLicenseJSON([]byte(row.JSON), &request)
			text := ""
			if err == nil {
				text, err = licenseRequestSignatureText(row.Path, request)
			}
			pub, pubErr := rawURLDecode(request.DevicePub, ed25519.PublicKeySize)
			sig, sigErr := rawURLDecode(request.Signature, ed25519.SignatureSize)
			requestID, idErr := hex.DecodeString(request.RequestID)
			counter, counterErr := strconv.ParseUint(request.Counter, 10, 64)
			valid := err == nil && pubErr == nil && sigErr == nil && request.Version == 1 && request.TS > 0 && idErr == nil && len(requestID) == 16 && hex.EncodeToString(requestID) == request.RequestID && counterErr == nil && counter > 0 && strconv.FormatUint(counter, 10) == request.Counter && (row.Path == "/v1/activate" || row.Path == "/v1/renew") && ed25519.Verify(pub, []byte(text), sig)
			if valid != row.Valid || row.Valid && (text != row.Text || !ed25519.Verify(key.Public().(ed25519.PublicKey), []byte(text), sig) || !strings.EqualFold(hex.EncodeToString(ed25519.Sign(key, []byte(text))), hex.EncodeToString(sig))) {
				t.Fatal("shared request vector mismatch")
			}
			if valid {
				paths[row.Path] = true
			}
		})
	}
	m := &licenseManager{options: licenseOptions{Keys: keys}, disk: licenseDiskState{PrivateKey: key}}
	for _, row := range v.Responses {
		t.Run("response/"+row.ID, func(t *testing.T) {
			var envelope signedLicenseEnvelope
			err := strictLicenseJSON([]byte(row.JSON), &envelope)
			var payload licensePayload
			if err == nil {
				payload, err = m.verify(envelope, row.RequestID)
			}
			if (err == nil) != row.Valid || row.Valid && payload.Error != row.Error {
				t.Fatal("shared response vector mismatch", err)
			}
			if err == nil {
				var expected licensePayload
				if strictLicenseJSON([]byte(row.Payload), &expected) != nil || !reflect.DeepEqual(payload, expected) {
					t.Fatal("shared response decoded payload differs from expected fields")
				}
				if payload.Status == "ACTIVE" {
					outcomes["ACTIVE"] = true
					kinds[payload.Kind] = true
				} else {
					outcomes[payload.Error] = true
				}
			} else {
				negative[row.Category] = true
			}
		})
	}
	if !paths["/v1/activate"] || !paths["/v1/renew"] || !kinds["standard"] || !kinds["admin"] {
		t.Fatal("shared vectors do not cover both request paths and license kinds")
	}
	for _, category := range []string{"noncanonical_base64url", "duplicate_json", "invalid_kid", "unknown_kid", "bad_signature", "binding", "lease", "license_expiry", "trailing_json", "unknown_field"} {
		if !negative[category] {
			t.Fatal("shared vectors missing rejection category", category)
		}
	}
	for _, name := range []string{"ACTIVE", "INVALID_CODE", "REPLACED", "REVOKED", "EXPIRED", "REPLAY", "TIMESTAMP_INVALID", "RATE_LIMITED", "SERVICE_UNAVAILABLE", "UNSUPPORTED_VERSION"} {
		if !outcomes[name] {
			t.Fatal("shared vectors missing outcome", name)
		}
	}
}
