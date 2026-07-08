// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"strings"
	"testing"
)

func TestFFmpegInstallHintNonEmpty(t *testing.T) {
	hint := FFmpegInstallHint()
	if strings.TrimSpace(hint) == "" {
		t.Fatal("FFmpegInstallHint() retornou vazio")
	}
	// Must mention ffmpeg somehow (command or name).
	if !strings.Contains(strings.ToLower(hint), "ffmpeg") {
		t.Errorf("hint não menciona ffmpeg: %q", hint)
	}
}

// FFmpegAvailable memoizes via sync.Once; here we only ensure it is callable and
// idempotent (same value on repeated calls), without depending on the environment.
func TestFFmpegAvailableIdempotent(t *testing.T) {
	first := FFmpegAvailable()
	if second := FFmpegAvailable(); first != second {
		t.Errorf("FFmpegAvailable não é idempotente: %v depois %v", first, second)
	}
}
