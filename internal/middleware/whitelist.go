// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package middleware

import (
	"sync"
)

type WhitelistManager struct {
	mu        sync.RWMutex
	whitelist map[string]bool
}

var Instance *WhitelistManager

func InitWhitelist(numbers []string) {
	Instance = &WhitelistManager{
		whitelist: make(map[string]bool),
	}
	for _, num := range numbers {
		Instance.Add(num)
	}
}

// Add inserts a number into the in-memory whitelist.
func (wm *WhitelistManager) Add(number string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.whitelist[number] = true
}

// Remove deletes a number from the in-memory whitelist.
func (wm *WhitelistManager) Remove(number string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	delete(wm.whitelist, number)
}

// Check reports whether the number is authorized in the whitelist.
func (wm *WhitelistManager) Check(number string) bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.whitelist[number]
}

// Reset clears the entire in-memory whitelist.
func (wm *WhitelistManager) Reset() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.whitelist = make(map[string]bool)
}
