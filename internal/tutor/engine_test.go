// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package tutor

import (
	"strings"
	"testing"
	"time"

	"chatic/internal/model"
)

func testUser() *model.User {
	return &model.User{
		NativeLanguage: "pt-BR",
		TargetLanguage: "en",
		Level:          "B1",
		Interests:      "futebol e tecnologia",
		TeacherName:    "Emma",
	}
}

// Every study prompt must: name the native and target languages, carry the tutor's
// name, and include the anti-injection security clause.
func TestStudyPromptsIncludeLanguagesAndSecurity(t *testing.T) {
	e := NewTutorEngine()
	u := testUser()

	prompts := map[string]string{
		"grammar":   e.BuildGrammarPrompt(u),
		"word":      e.BuildWordOfDayPrompt(u),
		"vocab":     e.BuildVocabPrompt(u),
		"quiz":      e.BuildQuizPrompt(u),
		"fix":       e.BuildFixPrompt(u),
		"gquiz":     e.BuildGroupQuizPrompt(u, "food"),
		"challenge": e.BuildChallengePrompt(u, ""),
	}

	for name, p := range prompts {
		if !strings.Contains(p, "English") {
			t.Errorf("%s: prompt does not mention the target language (English): %q", name, p)
		}
		if !strings.Contains(p, "Emma") {
			t.Errorf("%s: prompt does not mention the tutor's name (Emma)", name)
		}
		if !strings.Contains(p, "SECURITY") {
			t.Errorf("%s: prompt does not include the security clause", name)
		}
	}

	// grammar, word and vocab must also name the native language (explanation/translation).
	for _, name := range []string{"grammar", "word", "vocab"} {
		if !strings.Contains(prompts[name], "Portuguese") {
			t.Errorf("%s: prompt should mention the native language (Portuguese)", name)
		}
	}
}

// The engine must stay language-agnostic: changing the language pair changes the output.
func TestStudyPromptsAreLanguageAgnostic(t *testing.T) {
	e := NewTutorEngine()
	u := testUser()
	u.TargetLanguage = "ja" // Japanese
	u.NativeLanguage = "es"

	p := e.BuildVocabPrompt(u)
	if !strings.Contains(p, "Japanese") || !strings.Contains(p, "Spanish") {
		t.Errorf("prompt did not reflect the new language pair: %q", p)
	}
	if strings.Contains(p, "English") {
		t.Errorf("prompt should not contain a hardcoded language (English): %q", p)
	}
}

// The age clause must tailor prompts by age and add a minor-safety clause only for
// students under 18, and must be absent entirely when the birth year is unknown.
func TestAgeClauseByBand(t *testing.T) {
	e := NewTutorEngine()
	year := time.Now().Year()

	minor := testUser()
	minor.BirthYear = year - 10 // ~10 years old
	adult := testUser()
	adult.BirthYear = year - 30 // ~30 years old
	unknown := testUser()       // BirthYear 0

	minorPrompt := e.BuildSystemInstruction(minor, "")
	if !strings.Contains(minorPrompt, "10 years old") || !strings.Contains(minorPrompt, "a child") {
		t.Errorf("minor prompt should mention the age/band: %q", minorPrompt)
	}
	if !strings.Contains(minorPrompt, "age-appropriate") || !strings.Contains(minorPrompt, "minor") {
		t.Errorf("minor prompt should include the minor-safety clause: %q", minorPrompt)
	}

	adultPrompt := e.BuildSystemInstruction(adult, "")
	if !strings.Contains(adultPrompt, "30 years old") || !strings.Contains(adultPrompt, "an adult") {
		t.Errorf("adult prompt should mention the age/band: %q", adultPrompt)
	}
	if strings.Contains(adultPrompt, "age-appropriate") || strings.Contains(adultPrompt, "safe for a minor") {
		t.Errorf("adult prompt must NOT include the minor-safety clause: %q", adultPrompt)
	}

	// Unknown age: no age text at all — the study prompts (via personaIntro) also stay clean.
	if got := e.BuildVocabPrompt(unknown); strings.Contains(got, "years old") {
		t.Errorf("unknown-age prompt should not mention age: %q", got)
	}
	// Minor safety must also reach the study prompts (shared personaIntro).
	if got := e.BuildVocabPrompt(minor); !strings.Contains(got, "safe for a minor") {
		t.Errorf("study prompt for a minor should carry the safety clause: %q", got)
	}
}

// CalculateXP must award the link bonus even when the URL is embedded mid-sentence,
// not only when the message starts with "http" (regression: it used strings.HasPrefix).
func TestCalculateXPEmbeddedLink(t *testing.T) {
	e := NewTutorEngine()

	cases := []struct {
		name string
		text string
		want int
	}{
		{"embedded https", "Read this: https://example.com/article", 10},
		{"embedded http", "look here http://example.com please", 10},
		{"prefix https", "https://example.com is great", 10},
		{"no link, long", "I really enjoyed the football match yesterday", 3},
		{"three words no link", "I like football", 3},
		{"two words", "hi there", 1},
		{"one word", "hello", 1},
	}
	for _, c := range cases {
		if got := e.CalculateXP(c.text, false); got != c.want {
			t.Errorf("%s: CalculateXP(%q) = %d, want %d", c.name, c.text, got, c.want)
		}
	}

	// Audio always wins the speaking bonus regardless of text.
	if got := e.CalculateXP("", true); got != 5 {
		t.Errorf("audio CalculateXP = %d, want 5", got)
	}
}

// BuildSystemInstruction must use the injected custom prompt (interpolating the placeholders)
// when one is supplied, and fall back to the builtin prompt when it is empty — the engine no
// longer reads the global config, so the override is a pure function of its argument.
func TestBuildSystemInstructionCustomPrompt(t *testing.T) {
	e := NewTutorEngine()
	u := testUser()

	custom := "Persona {NomeProfessor} teaches {IdiomaAlvo} to a {IdiomaNativo} speaker at {Nivel} about {Interesses}."
	got := e.BuildSystemInstruction(u, custom)
	want := "Persona Emma teaches English to a Portuguese speaker at B1 about futebol e tecnologia."
	if got != want {
		t.Errorf("custom prompt not interpolated:\n got: %q\nwant: %q", got, want)
	}

	// Empty override => builtin prompt (must carry the security clause, not the custom text).
	builtin := e.BuildSystemInstruction(u, "")
	if strings.Contains(builtin, "Persona Emma teaches") {
		t.Errorf("empty override should fall back to the builtin prompt, got custom text: %q", builtin)
	}
	if !strings.Contains(builtin, "SECURITY") {
		t.Errorf("builtin prompt should include the security clause: %q", builtin)
	}
}

// The security clause must explicitly refuse persona swaps (OWASP LLM01 hardening).
func TestSecurityClausePersonaLock(t *testing.T) {
	e := NewTutorEngine()
	clause := e.securityClause()
	for _, want := range []string{"you are now", "act as", "break character", "another role or persona"} {
		if !strings.Contains(clause, want) {
			t.Errorf("security clause missing persona-lock phrase %q: %s", want, clause)
		}
	}
}

// personaIntro should degrade gracefully when optional fields are empty.
func TestPersonaIntroDefaults(t *testing.T) {
	e := NewTutorEngine()
	u := &model.User{NativeLanguage: "pt-BR", TargetLanguage: "en"}
	got := e.personaIntro(u)
	if !strings.Contains(got, "Teacher") {
		t.Errorf("personaIntro should use the default name 'Teacher' when empty: %q", got)
	}
	if !strings.Contains(got, "A1") {
		t.Errorf("personaIntro should use the default level 'A1' when empty: %q", got)
	}
}
