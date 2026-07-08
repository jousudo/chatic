// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"os/exec"
	"runtime"
	"sync"
)

var (
	ffmpegOnce sync.Once
	ffmpegOK   bool
)

// FFmpegAvailable reports whether the `ffmpeg` binary is available on PATH. The
// result is memoized (PATH does not change during execution). FFmpeg is required
// ONLY for the audio features — transcribing voice messages (input) and audio/TTS
// replies (output). The whole TEXT tutor works without it, so FFmpeg is an optional
// dependency, not a prerequisite.
func FFmpegAvailable() bool {
	ffmpegOnce.Do(func() {
		_, err := exec.LookPath("ffmpeg")
		ffmpegOK = err == nil
	})
	return ffmpegOK
}

// FFmpegInstallHint returns FFmpeg install instructions suited to the current
// operating system, for display in the startup warning.
func FFmpegInstallHint() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows: `winget install Gyan.FFmpeg` (or download from https://www.gyan.dev/ffmpeg/builds/ and add it to PATH)"
	case "darwin":
		return "macOS: `brew install ffmpeg`"
	default:
		return "Debian/Ubuntu: `sudo apt install ffmpeg` | Fedora: `sudo dnf install ffmpeg` | Alpine: `apk add ffmpeg`"
	}
}
