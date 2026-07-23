// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import "testing"

// userActivityAllowed must allow up to userRateLimit messages per window, then throttle —
// and it must signal the "notify" reply only once per window (not on every dropped message).
func TestUserActivityRateLimit(t *testing.T) {
	const number = "5511999990000"
	userRateMu.Lock()
	delete(userRateHits, number)
	delete(userRateNotify, number)
	userRateMu.Unlock()

	for i := 0; i < userRateLimit; i++ {
		allowed, _ := userActivityAllowed(number)
		if !allowed {
			t.Fatalf("message %d should be allowed within the limit", i+1)
		}
	}

	// First over-limit message: throttled AND flagged to notify the user once.
	allowed, notify := userActivityAllowed(number)
	if allowed {
		t.Errorf("message beyond the limit should be throttled")
	}
	if !notify {
		t.Errorf("first throttle in the window should request a notify")
	}

	// Subsequent over-limit messages: still throttled, but no repeated notify (avoid spam).
	allowed, notify = userActivityAllowed(number)
	if allowed {
		t.Errorf("message still beyond the limit should stay throttled")
	}
	if notify {
		t.Errorf("notify should fire only once per window, not on every dropped message")
	}

	// A different user is tracked independently.
	if allowed, _ := userActivityAllowed("5511888880000"); !allowed {
		t.Errorf("an unrelated user should not be affected by another user's rate limit")
	}
}
