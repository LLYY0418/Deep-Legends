//go:build license

package main

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
)

// Native-window telemetry is deliberately separate from the signed license
// protocol. Only finite geometry, fixed state names and flags enter the export.
type licenseWindowBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}
type licenseWindowDiagnostic struct {
	FromState        string              `json:"fromState"`
	ToState          string              `json:"toState"`
	WasMaximized     bool                `json:"wasMaximized"`
	WasFullscreen    bool                `json:"wasFullscreen"`
	RequestedContent licenseWindowBounds `json:"requestedContent"`
	ActualContent    licenseWindowBounds `json:"actualContent"`
	DisplayScale     float64             `json:"displayScale"`
	ElapsedMS        int                 `json:"elapsedMs"`
	Retried          bool                `json:"retried"`
	WaitTimedOut     bool                `json:"waitTimedOut"`
	RenderTimedOut   bool                `json:"renderTimedOut"`
	RenderMismatch   bool                `json:"renderMismatch"`
	FallbackUsed     bool                `json:"fallback_used"`
	StatusReadMS     int                 `json:"statusReadMs"`
	NativeMS         int                 `json:"nativeMs"`
	RenderMS         int                 `json:"renderMs"`
	NativeError      *string             `json:"native_error"`
	SizeMismatch     bool                `json:"sizeMismatch"`
}

func licenseWindowStateName(state string) bool {
	switch state {
	case "PENDING", "ACTIVE", "LOCKED", "NETWORK_LOCKED", "REPLACED", "REVOKED", "DEVICE_ERROR":
		return true
	}
	return false
}
func validLicenseWindowBounds(b licenseWindowBounds) bool {
	return b.X >= -1000000 && b.X <= 1000000 && b.Y >= -1000000 && b.Y <= 1000000 && b.Width > 0 && b.Width <= 1000000 && b.Height > 0 && b.Height <= 1000000
}
func (a *app) recordLicenseWindowDiagnostic(raw json.RawMessage) bool {
	var v *licenseWindowDiagnostic
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decodeErr := decoder.Decode(&v)
	invalid := licenseWindowInvalidFields(raw)
	if decodeErr != nil || len(invalid) > 0 {
		if len(invalid) == 0 {
			invalid = []map[string]any{{"field": "licenseWindow", "value_type": licenseWindowValueType(raw), "reason": "missing"}}
		}
		a.recordDiagnostic(map[string]any{"event": "license_window_state_invalid", "invalid_fields": invalid})
		return false
	}

	if v == nil || !licenseWindowStateName(v.FromState) || !licenseWindowStateName(v.ToState) || !validLicenseWindowBounds(v.RequestedContent) || !validLicenseWindowBounds(v.ActualContent) || math.IsNaN(v.DisplayScale) || math.IsInf(v.DisplayScale, 0) || v.DisplayScale <= 0 || v.DisplayScale > 8 || !validLicenseNativeError(v.NativeError) || v.ElapsedMS < 0 || v.ElapsedMS > 120000 || v.StatusReadMS < 0 || v.StatusReadMS > 120000 || v.NativeMS < 0 || v.NativeMS > 120000 || v.RenderMS < 0 || v.RenderMS > 120000 {
		a.recordDiagnostic(map[string]any{"event": "license_window_state_invalid", "invalid_fields": []map[string]any{{"field": "licenseWindow", "value_type": "object", "reason": "out_of_range"}}})
		return false
	}
	a.recordDiagnostic(map[string]any{"event": "license_window_state", "from_state": v.FromState, "to_state": v.ToState, "was_maximized": v.WasMaximized, "was_fullscreen": v.WasFullscreen, "requested_content": v.RequestedContent, "actual_content": v.ActualContent, "display_scale": v.DisplayScale, "scale_factor": v.DisplayScale, "native_error": v.NativeError, "elapsed_ms": v.ElapsedMS, "retried": v.Retried, "wait_timed_out": v.WaitTimedOut, "render_timed_out": v.RenderTimedOut, "render_mismatch": v.RenderMismatch, "size_mismatch": v.SizeMismatch, "fallback_used": v.FallbackUsed, "status_read_ms": v.StatusReadMS, "native_ms": v.NativeMS, "render_ms": v.RenderMS})
	return true
}

func licenseWindowValueType(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "missing"
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return "invalid_json"
	}
	if value == nil {
		return "null"
	}
	switch value.(type) {
	case json.Number:
		return "number"
	case string:
		return "string"
	case bool:
		return "boolean"
	case []any:
		return "array"
	default:
		return "object"
	}
}
func licenseWindowInvalidFields(raw json.RawMessage) []map[string]any {
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil || root == nil {
		return []map[string]any{{"field": "licenseWindow", "value_type": licenseWindowValueType(raw), "reason": "missing"}}
	}
	var invalid []map[string]any
	add := func(field string, value json.RawMessage) {
		invalid = append(invalid, map[string]any{"field": field, "value_type": licenseWindowValueType(value), "reason": "missing"})
	}
	for _, field := range []string{"fromState", "toState"} {
		var value string
		if json.Unmarshal(root[field], &value) != nil || !licenseWindowStateName(value) {
			add(field, root[field])
		}
	}
	for _, field := range []string{"requestedContent", "actualContent"} {
		var bounds map[string]json.RawMessage
		if json.Unmarshal(root[field], &bounds) != nil || bounds == nil {
			add(field, root[field])
			continue
		}
		for _, dimension := range []string{"x", "y", "width", "height"} {
			valueRaw := bounds[dimension]
			// Coordinates were optional in the existing telemetry schema.
			if len(valueRaw) == 0 && (dimension == "x" || dimension == "y") {
				continue
			}
			var number json.Number
			if licenseWindowValueType(valueRaw) != "number" || json.Unmarshal(valueRaw, &number) != nil {
				add(field+"."+dimension, valueRaw)
				continue
			}
			value, err := number.Float64()
			reason := ""
			switch {
			case err != nil || math.IsInf(value, 0) || math.Abs(value) > 1000000:
				reason = "out_of_range"
			case value != math.Trunc(value):
				reason = "non_integer"
			case (dimension == "width" || dimension == "height") && value <= 0:
				reason = "non_positive"
			}
			if reason != "" {
				invalid = append(invalid, map[string]any{"field": field + "." + dimension, "value_type": "number", "value": number, "reason": reason})
			}

		}
	}
	for _, field := range []string{"displayScale", "elapsedMs", "nativeMs", "renderMs", "statusReadMs"} {
		if (field == "elapsedMs" || field == "nativeMs" || field == "renderMs" || field == "statusReadMs") && len(root[field]) == 0 {
			continue
		}
		var value float64
		maxValue := 120000.
		minValue := 0.
		if field == "displayScale" {
			maxValue = 8
			minValue = math.SmallestNonzeroFloat64
		}
		if json.Unmarshal(root[field], &value) != nil || licenseWindowValueType(root[field]) != "number" || value < minValue || value > maxValue {
			add(field, root[field])
		}
	}
	if len(root["native_error"]) > 0 {
		var value *string
		if json.Unmarshal(root["native_error"], &value) != nil || !validLicenseNativeError(value) {
			add("native_error", root["native_error"])
		}
	}
	// Fixed schema names only. Never echo an unknown key or its contents.
	typ := reflect.TypeOf(licenseWindowDiagnostic{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type.Kind() != reflect.Bool {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if len(root[name]) > 0 && licenseWindowValueType(root[name]) != "boolean" {
			add(name, root[name])
		}
	}
	return invalid
}

func validLicenseNativeError(value *string) bool {
	if value == nil {
		return true
	}
	switch *value {
	case "Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError":
		return true
	}
	return false
}
