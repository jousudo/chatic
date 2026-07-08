// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"testing"
)

func TestPasswordHashing(t *testing.T) {
	pass := "SenhaSegura123!"
	hash, err := HashPassword(pass)
	if err != nil {
		t.Fatalf("Erro ao gerar hash: %v", err)
	}

	if !CheckPasswordHash(pass, hash) {
		t.Error("Falha ao validar senha correta com hash")
	}

	if CheckPasswordHash("SenhaIncorreta", hash) {
		t.Error("Validou hash incorreto com senha incorreta")
	}
}

func TestSecureCompare(t *testing.T) {
	if !SecureCompare("token123", "token123") {
		t.Error("Falha ao comparar tokens identicos")
	}

	if SecureCompare("token123", "token1234") {
		t.Error("Tokens diferentes marcados como iguais")
	}

	if SecureCompare("token123", "") {
		t.Error("Token e string vazia marcados como iguais")
	}
}

func TestAESGCM(t *testing.T) {
	password := "ChaveMestraSegura"
	salt := []byte("SaltTeste")
	key := DeriveKey(password, salt)

	originalText := "SegredoSuperSecretoDasConfiguracoes"
	ciphertext, err := EncryptAESGCM(originalText, key)
	if err != nil {
		t.Fatalf("Falha na encriptacao: %v", err)
	}

	decryptedText, err := DecryptAESGCM(ciphertext, key)
	if err != nil {
		t.Fatalf("Falha na desencriptacao: %v", err)
	}

	if decryptedText != originalText {
		t.Errorf("Texto desencriptado '%s' nao bate com original '%s'", decryptedText, originalText)
	}
}
