// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port               string
	Env                string
	PrimaryLLMProvider string
	GeminiAPIKey       string   // first key (backward-compatible)
	GeminiAPIKeys      []string // full pool for round-robin
	OpenaiAPIKey       string   // first key (backward-compatible)
	OpenaiAPIKeys      []string // full pool for round-robin
	ClaudeAPIKey       string   // first key (backward-compatible)
	ClaudeAPIKeys      []string // full pool for round-robin
	LLMTimeoutSeconds  int
	DatabasePath       string
	InitialAdminNumber string
	OllamaAPIBase      string
	OllamaModel        string
	CustomSystemPrompt string
	SelfChatPrefix     string
	GoogleTTSAPIKey    string
	// MaxMessageAgeSeconds bounds how old an incoming WhatsApp message may be to still be
	// processed. On reconnect after downtime WhatsApp replays the offline backlog; without
	// this guard the bot floods every chat with stale replies. Live messages carry a
	// near-current timestamp and pass. Set to 0 to disable the guard (process all messages).
	MaxMessageAgeSeconds int
	// MultiAccountEnabled turns on the optional multi-account mode: it lets several
	// personal WhatsApps be paired on the same instance, each owner talking to the tutor
	// via self-chat, with no whitelist/admin. Already-paired devices always boot;
	// this flag only controls pairing of NEW accounts from the panel.
	MultiAccountEnabled bool
}

var CurrentConfig *Config

// LoadConfig loads the environment variables from the .env file or the system.
func LoadConfig() *Config {
	// Try to load the .env file manually to avoid heavy external dependencies
	file, err := os.Open(".env")
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				// If the variable is already set in the system, do not overwrite it
				if os.Getenv(key) == "" {
					os.Setenv(key, value)
				}
			}
		}
	}

	timeout, _ := strconv.Atoi(getEnv("LLM_TIMEOUT_SECONDS", "10"))
	maxMsgAge, _ := strconv.Atoi(getEnv("MAX_MESSAGE_AGE_SECONDS", "300"))

	// Build each provider's key pool: <PROVIDER>_API_KEYS (pool) + <PROVIDER>_API_KEY (backward-compatible)
	geminiKeys := buildKeyPool("GEMINI_API_KEYS", "GEMINI_API_KEY")
	openaiKeys := buildKeyPool("OPENAI_API_KEYS", "OPENAI_API_KEY")
	claudeKeys := buildKeyPool("CLAUDE_API_KEYS", "CLAUDE_API_KEY")

	CurrentConfig = &Config{
		Port:                 getEnv("PORT", "3030"),
		Env:                  getEnv("ENV", "production"),
		PrimaryLLMProvider:   getEnv("PRIMARY_LLM_PROVIDER", "gemini"),
		GeminiAPIKey:         firstKey(geminiKeys),
		GeminiAPIKeys:        geminiKeys,
		OpenaiAPIKey:         firstKey(openaiKeys),
		OpenaiAPIKeys:        openaiKeys,
		ClaudeAPIKey:         firstKey(claudeKeys),
		ClaudeAPIKeys:        claudeKeys,
		LLMTimeoutSeconds:    timeout,
		DatabasePath:         getEnv("DATABASE_PATH", "storage/tutor.db"),
		InitialAdminNumber:   os.Getenv("INITIAL_ADMIN_NUMBER"),
		OllamaAPIBase:        getEnv("OLLAMA_API_BASE", "http://localhost:11434"),
		OllamaModel:          getEnv("OLLAMA_MODEL", "llama3.2"),
		CustomSystemPrompt:   os.Getenv("CUSTOM_SYSTEM_PROMPT"),
		SelfChatPrefix:       getEnv("SELF_CHAT_PREFIX", "!"),
		GoogleTTSAPIKey:      os.Getenv("GOOGLE_TTS_API_KEY"),
		MaxMessageAgeSeconds: maxMsgAge,
		MultiAccountEnabled:  strings.EqualFold(getEnv("MULTI_ACCOUNT_ENABLED", "false"), "true"),
	}

	return CurrentConfig
}

func getEnv(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// buildKeyPool builds a provider key pool from its plural env var (comma-separated pool)
// plus its singular env var (backward-compatible), placing the singular first if not already
// present. Returns a de-duplicated, order-preserving list.
func buildKeyPool(poolEnv, singleEnv string) []string {
	keys := ParseKeyPool(os.Getenv(poolEnv))
	if single := strings.TrimSpace(os.Getenv(singleEnv)); single != "" {
		if !ContainsKey(keys, single) {
			keys = append([]string{single}, keys...)
		}
	}
	return keys
}

// firstKey returns the first key of a pool, or "" if the pool is empty.
func firstKey(keys []string) string {
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// ParseKeyPool splits a comma-separated string of keys, ignoring spaces and empty entries.
func ParseKeyPool(raw string) []string {
	var keys []string
	for _, k := range strings.Split(raw, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// ContainsKey reports whether a key already exists in the slice.
func ContainsKey(slice []string, key string) bool {
	for _, k := range slice {
		if k == key {
			return true
		}
	}
	return false
}
