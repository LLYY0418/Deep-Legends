package main

import (
	"os"
	"strings"
	"testing"
)

func TestRiotKeyConfiguredValueRejectsMissingAndCorruptCiphertext(t *testing.T) {
	ciphertext, err := encryptRiotKey("RGAPI-test-key")
	if err != nil {
		t.Fatalf("encryptRiotKey: %v", err)
	}
	if !riotKeyConfiguredValue(ciphertext) {
		t.Fatal("valid Riot key ciphertext was rejected")
	}
	if riotKeyConfiguredValue("") {
		t.Fatal("empty Riot key ciphertext was accepted")
	}
	if riotKeyConfiguredValue("not-a-valid-ciphertext") {
		t.Fatal("corrupt Riot key ciphertext was accepted")
	}
}

func TestSelfCheckRiotKeyUsesConfiguredGuard(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "if !riotKeyConfigured()") {
		t.Fatal("-self-check-riot-key no longer enforces riotKeyConfigured")
	}
}
