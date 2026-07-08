// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"testing"

	"chatic/config"
)

// TestProviderNextKeyRoundRobin verifies that OpenAI and Claude now rotate their system
// key pools round-robin (the pool-for-all-providers behavior), and that a single-key pool
// always returns that key.
func TestProviderNextKeyRoundRobin(t *testing.T) {
	config.CurrentConfig = &config.Config{
		OpenaiAPIKeys: []string{"o1", "o2", "o3"},
		ClaudeAPIKeys: []string{"c1"},
	}

	openai := &OpenAIProvider{}
	seen := map[string]bool{}
	for i := 0; i < 9; i++ {
		seen[openai.nextKey("")] = true
	}
	for _, k := range []string{"o1", "o2", "o3"} {
		if !seen[k] {
			t.Errorf("OpenAI round-robin never returned %q (seen=%v)", k, seen)
		}
	}

	claude := &ClaudeProvider{}
	for i := 0; i < 3; i++ {
		if got := claude.nextKey(""); got != "c1" {
			t.Errorf("single-key pool: got %q, want c1", got)
		}
	}
}

// TestNextKeyCustomOverridesPool verifies a custom (per-user/group) key takes priority over
// the system pool and is itself rotated when comma-separated.
func TestNextKeyCustomOverridesPool(t *testing.T) {
	config.CurrentConfig = &config.Config{OpenaiAPIKeys: []string{"system"}}
	openai := &OpenAIProvider{}
	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		seen[openai.nextKey("x1,x2")] = true
	}
	if seen["system"] {
		t.Error("custom key should override the system pool")
	}
	if !seen["x1"] || !seen["x2"] {
		t.Errorf("custom keys not rotated: seen=%v", seen)
	}
}

// TestNextKeyEmptyPoolFallsBackToSingle verifies the backward-compatible single-key fallback.
func TestNextKeyEmptyPoolFallsBackToSingle(t *testing.T) {
	config.CurrentConfig = &config.Config{OpenaiAPIKey: "legacy", OpenaiAPIKeys: nil}
	openai := &OpenAIProvider{}
	if got := openai.nextKey(""); got != "legacy" {
		t.Errorf("fallback: got %q, want legacy", got)
	}
}

// TestProviderPools verifies the panel accessor points at the right in-memory fields and
// returns (nil, nil) for providers without an API-key pool.
func TestProviderPools(t *testing.T) {
	config.CurrentConfig = &config.Config{
		GeminiAPIKeys: []string{"g"},
		OpenaiAPIKeys: []string{"o"},
		ClaudeAPIKeys: []string{"c"},
	}
	cases := map[string][]string{"gemini": {"g"}, "openai": {"o"}, "claude": {"c"}}
	for provider, want := range cases {
		pool, primary := providerPools(provider)
		if pool == nil || primary == nil {
			t.Fatalf("%s: unexpected nil accessor", provider)
		}
		if len(*pool) != 1 || (*pool)[0] != want[0] {
			t.Errorf("%s: pool = %v, want %v", provider, *pool, want)
		}
		// Mutating through the pointer must reflect in config.
		*pool = append(*pool, "extra")
		*primary = (*pool)[0]
	}
	if len(config.CurrentConfig.OpenaiAPIKeys) != 2 {
		t.Errorf("pointer mutation not reflected: %v", config.CurrentConfig.OpenaiAPIKeys)
	}

	if pool, primary := providerPools("ollama"); pool != nil || primary != nil {
		t.Error("ollama has no key pool; expected (nil, nil)")
	}
	if pool, _ := providerPools("unknown"); pool != nil {
		t.Error("unknown provider expected (nil, nil)")
	}
}
