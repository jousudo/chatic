// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"chatic/config"
	"chatic/internal/database"
	"chatic/internal/model"
)

// machineKey is the local master key (32 bytes) used to encrypt the API keys at
// rest in SQLite. It lives in a file separate from the database, with permission
// 0600, so that stealing the .db file alone does not allow decrypting the secrets
// (the key and the ciphertext live in distinct, access-restricted files).
var (
	machineKey   []byte
	machineKeyMu sync.RWMutex
)

// LoadOrCreateMachineKey loads the local master key from the given path or generates
// a new one (32 random bytes) on first run, writing it with 0600.
func LoadOrCreateMachineKey(path string) error {
	machineKeyMu.Lock()
	defer machineKeyMu.Unlock()

	if data, err := os.ReadFile(path); err == nil && len(data) == 32 {
		machineKey = data
		return nil
	}

	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return err
	}
	_ = os.Chmod(path, 0600)
	machineKey = key
	return nil
}

// getMachineKey returns the loaded local master key (or nil if not initialized).
func getMachineKey() []byte {
	machineKeyMu.RLock()
	defer machineKeyMu.RUnlock()
	return machineKey
}

// secretEncPrefix marks a value already encrypted in the vault. It lets us tell new
// (encrypted) secrets apart from legacy plaintext values in the database, migrating
// transparently without breaking old rows.
const secretEncPrefix = "enc:"

// EncryptSecret encrypts a per-user/group secret (e.g. a personal API key) with the
// local master key, returning "enc:<base64>". If the master key is unavailable or the
// text is empty, it returns the original value (safe degradation).
func EncryptSecret(plain string) string {
	if plain == "" {
		return ""
	}
	key := getMachineKey()
	if key == nil {
		return plain
	}
	enc, err := EncryptAESGCM(plain, key)
	if err != nil {
		return plain
	}
	return secretEncPrefix + enc
}

// DecryptSecret reverses EncryptSecret. Values without the "enc:" prefix are treated as
// legacy plaintext and returned as-is (backward compatibility).
func DecryptSecret(stored string) string {
	if !strings.HasPrefix(stored, secretEncPrefix) {
		return stored
	}
	key := getMachineKey()
	if key == nil {
		return ""
	}
	dec, err := DecryptAESGCM(strings.TrimPrefix(stored, secretEncPrefix), key)
	if err != nil {
		return ""
	}
	return dec
}

// LoadEncryptedKeysIntoConfig decrypts the API keys stored in SQLite
// (SystemConfig.EncryptedKeys) using the local master key and populates
// CurrentConfig. It lets the bot operate headless after a restart, without relying
// on plaintext secrets in the .env. Keys read from the .env at boot take
// precedence only when the encrypted vault has not been filled yet.
func LoadEncryptedKeysIntoConfig() {
	key := getMachineKey()
	if key == nil {
		return
	}

	var cfg model.SystemConfig
	if err := database.DB.First(&cfg).Error; err != nil || cfg.EncryptedKeys == "" {
		return
	}

	decJSON, err := DecryptAESGCM(cfg.EncryptedKeys, key)
	if err != nil {
		// Old ciphertext (previous scheme) or a rotated master key: ignore and keep
		// what came from the .env. It will be re-encrypted when saved via the panel.
		log.Printf("Warning: could not decrypt keys from the local vault (they will be reloaded when saved via the panel).")
		return
	}

	var keys map[string]string
	if json.Unmarshal([]byte(decJSON), &keys) != nil {
		return
	}

	if v := keys["gemini"]; v != "" {
		config.CurrentConfig.GeminiAPIKey = v
	}
	if v := keys["gemini_pool"]; v != "" {
		config.CurrentConfig.GeminiAPIKeys = config.ParseKeyPool(v)
	}
	if v := keys["openai"]; v != "" {
		config.CurrentConfig.OpenaiAPIKey = v
	}
	if v := keys["openai_pool"]; v != "" {
		config.CurrentConfig.OpenaiAPIKeys = config.ParseKeyPool(v)
	}
	if v := keys["claude"]; v != "" {
		config.CurrentConfig.ClaudeAPIKey = v
	}
	if v := keys["claude_pool"]; v != "" {
		config.CurrentConfig.ClaudeAPIKeys = config.ParseKeyPool(v)
	}
	if v := keys["google_tts"]; v != "" {
		config.CurrentConfig.GoogleTTSAPIKey = v
	}

	// Ensure each provider's primary key is also present in its round-robin pool.
	ensurePrimaryInPool(&config.CurrentConfig.GeminiAPIKeys, config.CurrentConfig.GeminiAPIKey)
	ensurePrimaryInPool(&config.CurrentConfig.OpenaiAPIKeys, config.CurrentConfig.OpenaiAPIKey)
	ensurePrimaryInPool(&config.CurrentConfig.ClaudeAPIKeys, config.CurrentConfig.ClaudeAPIKey)
}

// ensurePrimaryInPool prepends the primary key to the pool if it is set but missing,
// keeping the "primary == pool[0]" invariant consistent across providers.
func ensurePrimaryInPool(pool *[]string, primary string) {
	if primary != "" && !config.ContainsKey(*pool, primary) {
		*pool = append([]string{primary}, *pool...)
	}
}
