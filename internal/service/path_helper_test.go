// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"path/filepath"
	"testing"
)

func TestSafePathJoin(t *testing.T) {
	baseDir := "./storage"

	tests := []struct {
		name       string
		unsafePath string
		shouldErr  bool
	}{
		{"Normal file", "chat_history.db", false},
		{"Subdirectory file", "media/audio.mp3", false},
		{"Directory traversal simple", "../etc/passwd", true},
		{"Directory traversal nested", "media/../../../etc/passwd", true},
		{"Directory traversal bypass attempt", "media/../../storage_other/test.txt", true},
		{"Prefix bypass attempt", "../storage_secret/test.txt", true},
		{"Absolute path attempt", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SafePathJoin(baseDir, tt.unsafePath)
			if tt.shouldErr {
				if err == nil {
					t.Errorf("Esperava erro para o caminho '%s', mas a operacao passou. Caminho gerado: %s", tt.unsafePath, result)
				}
			} else {
				if err != nil {
					t.Errorf("Nao esperava erro para o caminho '%s', mas ocorreu: %v", tt.unsafePath, err)
				}
				// Check that it resolved correctly to the absolute path
				absBase, _ := filepath.Abs(baseDir)
				if !filepath.HasPrefix(result, absBase) {
					t.Errorf("Caminho gerado '%s' nao contem a raiz '%s'", result, absBase)
				}
			}
		})
	}
}
