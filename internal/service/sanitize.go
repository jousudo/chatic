// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"regexp"
	"strings"
)

// maxUserInputLen caps the size of user text reaching the LLM/DB. Prevents token
// abuse and reduces the injection surface from huge payloads.
const maxUserInputLen = 2000

var (
	// controlChars removes control characters (except \n and \t), which have no
	// legitimate use in a conversation and can be used to obfuscate injection payloads.
	controlChars = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)

	// promptInjectionPatterns neutralizes common attempts to hijack the system prompt
	// (prompt injection). It is not a perfect barrier — the main defense is the security
	// clause in the system prompt itself (see TutorEngine) — but it strips the most
	// obvious triggers before the text reaches the model. Matches: an "override/ignore"
	// verb followed, within a few words, by a target (instructions, rules, prompt,
	// "above"/"previous"); role swaps ("you are now"); role tags (<system>, [INST]);
	// and explicit mentions of "system prompt".
	promptInjectionPatterns = regexp.MustCompile(`(?i)\b(ignore|disregard|forget|override|bypass|esque[cç]a|ignore|desconsidere)\b[^.\n]{0,40}\b(instructions?|instru[çc][õo]es|prompts?|rules?|regras?|context|contexto|above|previous|prior|acima|anterior(?:es)?)\b|\bsystem\s*prompt\b|\b(you\s+are\s+now|voc[êe]\s+agora\s+[ée]|act\s+as|aja\s+como)\b|</?(system|assistant|user|inst)>|\[/?(system|inst|assistant|user)\]`)
)

// sanitizeUserInput normalizes and sanitizes free user text before using it
// (LLM or persistence). It strips control characters, caps the length, and
// neutralizes obvious prompt-injection triggers by replacing them with a marker.
func sanitizeUserInput(text string) string {
	text = controlChars.ReplaceAllString(text, "")
	text = promptInjectionPatterns.ReplaceAllString(text, "[filtered]")
	text = strings.TrimSpace(text)
	if len(text) > maxUserInputLen {
		text = text[:maxUserInputLen]
	}
	return text
}

// sanitizePhone keeps only digits from a phone number supplied in administrative
// commands. Beyond data hygiene, it removes any character that could be used in
// injection attempts (DB access is already parameterized via GORM, but validating
// the input shape is defense in depth).
func sanitizePhone(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
