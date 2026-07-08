// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePathJoin joins a base directory with an untrusted relative path (e.g. a filename derived from external input).
// It strictly ensures the resulting path stays contained within the base directory,
// preventing Directory Traversal attacks (like ../../../etc/passwd) and timing leaks.
func SafePathJoin(baseDir, unsafePath string) (string, error) {
	// Reject absolute paths to avoid directory escape or ambiguity
	if filepath.IsAbs(unsafePath) || strings.HasPrefix(unsafePath, "/") || strings.HasPrefix(unsafePath, "\\") {
		return "", fmt.Errorf("absolute paths are forbidden for security reasons: %s", unsafePath)
	}

	// Convert to absolute and clean paths
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base path: %w", err)
	}

	// Clean the untrusted path to remove redundancies like . and ..
	cleanUnsafe := filepath.Clean(unsafePath)

	// Join the two paths
	joined := filepath.Join(absBase, cleanUnsafe)

	// Get the final resolved absolute path
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("invalid destination path: %w", err)
	}

	// Ensure the final path starts with the base directory's absolute prefix.
	// We append the path separator to baseDir so "storage-bypass" does not pass the "storage" prefix test.
	basePrefix := absBase
	if !strings.HasSuffix(basePrefix, string(filepath.Separator)) {
		basePrefix += string(filepath.Separator)
	}

	// The final path is safe if it is exactly the baseDir or starts with the baseDir prefix
	if absJoined != absBase && !strings.HasPrefix(absJoined, basePrefix) {
		return "", fmt.Errorf("security violation detected: attempt to access outside the authorized base directory (%s)", absJoined)
	}

	return absJoined, nil
}
