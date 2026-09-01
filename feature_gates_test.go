package main

import "testing"

func TestFeatureGatesUseDefaultsAndApplyKnownOverrides(t *testing.T) {
	gates := newFeatureGates()
	for key := range defaultFeatureGateValues {
		if !gates.enabled(key) {
			t.Fatalf("default gate %q is disabled", key)
		}
	}
	gates.apply(map[string]bool{featureGateOPGG: false, "unknown": true})
	if gates.enabled(featureGateOPGG) || gates.enabled("unknown") {
		t.Fatalf("gate overrides = %#v", gates.values)
	}
	if !gates.enabled(featureGateHexdata) {
		t.Fatal("unmentioned gate lost its default")
	}
}

func TestFeatureGateRejectsNonHTTPSRemote(t *testing.T) {
	if err := newFeatureGates().refresh(t.Context(), nil, "http://example.com/gates.json"); err == nil {
		t.Fatal("non-HTTPS feature gate URL was accepted")
	}
}

func TestFeatureGateMapsQQ101Host(t *testing.T) {
	if got := featureGateForChampionHost(qq101Host); got != featureGateQQ101 {
		t.Fatalf("QQ101 host gate = %q", got)
	}
	gates := newFeatureGates()
	gates.apply(map[string]bool{featureGateQQ101: false})
	if gates.enabled(featureGateQQ101) {
		t.Fatal("QQ101 gate override did not apply")
	}
}
