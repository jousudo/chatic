// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"path/filepath"
	"testing"
)

func TestEncryptDecryptSecretRoundTrip(t *testing.T) {
	// Initialize a temporary local master key for the test.
	keyPath := filepath.Join(t.TempDir(), ".masterkey")
	if err := LoadOrCreateMachineKey(keyPath); err != nil {
		t.Fatalf("falha ao criar chave-mestra de teste: %v", err)
	}

	secret := "AIzaSyD-super-secret-api-key-123"
	enc := EncryptSecret(secret)

	if enc == secret {
		t.Fatalf("EncryptSecret não alterou o valor (não criptografou): %q", enc)
	}
	if got := DecryptSecret(enc); got != secret {
		t.Errorf("round-trip falhou: DecryptSecret(EncryptSecret(x)) = %q, want %q", got, secret)
	}
}

func TestDecryptSecretLegacyPlaintext(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), ".masterkey")
	if err := LoadOrCreateMachineKey(keyPath); err != nil {
		t.Fatalf("falha ao criar chave-mestra de teste: %v", err)
	}
	// A legacy value without the "enc:" prefix must be returned as-is (backward compatibility).
	legacy := "plain-old-key"
	if got := DecryptSecret(legacy); got != legacy {
		t.Errorf("DecryptSecret(legacy) = %q, want %q (passthrough)", got, legacy)
	}
}

func TestEncryptSecretEmpty(t *testing.T) {
	if got := EncryptSecret(""); got != "" {
		t.Errorf("EncryptSecret(\"\") = %q, want empty", got)
	}
}
