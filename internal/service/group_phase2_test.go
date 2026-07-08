// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"strings"
	"testing"
)

// groupHelpText must advertise every group command so members can discover the activities.
func TestGroupHelpTextListsCommands(t *testing.T) {
	h := groupHelpText()
	for _, cmd := range []string{"/ask", "/correct", "/gquiz", "/greveal", "/gword", "/gchallenge", "/ghelp"} {
		if !strings.Contains(h, cmd) {
			t.Errorf("group help is missing %q", cmd)
		}
	}
}

// parseQuizPayload must accept a well-formed object (even wrapped in code fences or prose) and
// reject anything that can't build a valid single-answer poll.
func TestParseQuizPayload(t *testing.T) {
	valid := "```json\n{\"question\":\"He ___ to school.\",\"options\":[\"go\",\"goes\",\"going\"],\"answer_index\":1,\"explanation\":\"Terceira pessoa do singular usa -s.\"}\n```"
	q, err := parseQuizPayload(valid)
	if err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	if q.Options[q.AnswerIndex] != "goes" {
		t.Errorf("correct option = %q, want %q", q.Options[q.AnswerIndex], "goes")
	}

	bad := map[string]string{
		"no json":        "sorry, I can't do that",
		"one option":     `{"question":"q","options":["a"],"answer_index":0,"explanation":"e"}`,
		"too many":       `{"question":"q","options":["a","b","c","d","e"],"answer_index":0,"explanation":"e"}`,
		"index out":      `{"question":"q","options":["a","b"],"answer_index":5,"explanation":"e"}`,
		"empty question": `{"question":"  ","options":["a","b"],"answer_index":0,"explanation":"e"}`,
		"empty option":   `{"question":"q","options":["a",""],"answer_index":0,"explanation":"e"}`,
	}
	for name, raw := range bad {
		if _, err := parseQuizPayload(raw); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

// groupActivityAllowed must allow up to groupRateLimit activities per window, then throttle.
func TestGroupActivityRateLimit(t *testing.T) {
	const jid = "test-group@g.us"
	groupRateMu.Lock()
	delete(groupRateHits, jid)
	groupRateMu.Unlock()

	for i := 0; i < groupRateLimit; i++ {
		if !groupActivityAllowed(jid) {
			t.Fatalf("activity %d should be allowed within the limit", i+1)
		}
	}
	if groupActivityAllowed(jid) {
		t.Errorf("activity beyond the limit should be throttled")
	}
}
