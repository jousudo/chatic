// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"chatic/internal/model"

	"go.mau.fi/whatsmeow/types"
)

// --- PHASE 2: Group activities (see Section 16 of completo.md) ---
//
// Phase 2 adds triggered study ACTIVITIES to groups, on top of Phase 1's reactive replies:
//
//	/gquiz [theme]  -> ONE multiple-choice question posted as a NATIVE WhatsApp poll
//	/greveal        -> reveals the correct answer + explanation of the last /gquiz
//	/gword          -> a word of the day for the group
//	/gchallenge [t] -> a short shared practice challenge
//
// Cost is bounded by a per-group rolling rate limit (groupActivityAllowed), stronger than
// Phase 1's 3s cooldown — the guardrail Section 16.4 requires before heavier group modes.

var (
	groupRateMu     sync.Mutex
	groupRateHits   = make(map[string][]time.Time)
	groupRateLimit  = 4
	groupRateWindow = time.Minute
)

// groupActivityAllowed enforces a per-group rolling rate limit for Phase 2 activities: at most
// groupRateLimit activities per groupRateWindow. Returns false when the group has exceeded it.
func groupActivityAllowed(jid string) bool {
	groupRateMu.Lock()
	defer groupRateMu.Unlock()
	cutoff := time.Now().Add(-groupRateWindow)
	kept := groupRateHits[jid][:0] // reuse the backing array (safe under the lock)
	for _, t := range groupRateHits[jid] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= groupRateLimit {
		groupRateHits[jid] = kept
		return false
	}
	groupRateHits[jid] = append(kept, time.Now())
	return true
}

// groupQuizAnswer holds the answer key of the last /gquiz posted to a group so /greveal can
// disclose it. Kept in memory only (no schema change): a restart forgets pending quizzes.
type groupQuizAnswer struct {
	correct     string
	explanation string
	createdAt   time.Time
}

var (
	groupQuizMu    sync.Mutex
	groupQuizState = make(map[string]groupQuizAnswer)
)

// quizPayload is the strict JSON contract the LLM must emit for a group quiz (BuildGroupQuizPrompt).
type quizPayload struct {
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	AnswerIndex int      `json:"answer_index"`
	Explanation string   `json:"explanation"`
}

// parseQuizPayload extracts the JSON object from the model output (tolerating code fences or
// surrounding prose) and validates it enough to build a poll.
func parseQuizPayload(raw string) (*quizPayload, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object in quiz output")
	}
	var q quizPayload
	if err := json.Unmarshal([]byte(raw[start:end+1]), &q); err != nil {
		return nil, err
	}
	q.Question = strings.TrimSpace(q.Question)
	if q.Question == "" || len(q.Options) < 2 || len(q.Options) > 4 {
		return nil, fmt.Errorf("invalid quiz payload: question/options")
	}
	if q.AnswerIndex < 0 || q.AnswerIndex >= len(q.Options) {
		return nil, fmt.Errorf("answer_index out of range")
	}
	for i := range q.Options {
		q.Options[i] = strings.TrimSpace(q.Options[i])
		if q.Options[i] == "" {
			return nil, fmt.Errorf("empty option")
		}
	}
	return &q, nil
}

// throttleMsg is the friendly notice shown when a group hits the Phase 2 rate limit.
const throttleMsg = "⏳ Let's slow down a bit — too many activities right now. Try again in a minute."

// groupHelpText lists what the tutor can do inside a WhatsApp group (static, no LLM call).
func groupHelpText() string {
	var sb strings.Builder
	sb.WriteString("🤖 *Tutor — group commands:*\n\n")
	sb.WriteString("• *@mention* the bot or */ask <question>* — ask the tutor something\n")
	sb.WriteString("• */correct <phrase>* — get a quick correction\n")
	sb.WriteString("• */gquiz [theme]* — start a poll quiz (everyone votes)\n")
	sb.WriteString("• */greveal* — reveal the answer of the last quiz\n")
	sb.WriteString("• */gword* — a word of the day for the group\n")
	sb.WriteString("• */gchallenge [theme]* — a fun practice challenge to do together\n")
	sb.WriteString("• */ghelp* — show this list\n")
	sb.WriteString("\nℹ️ For private lessons, chat with the tutor in a direct message and send */help* there.")
	return sb.String()
}

// sendGroupHelp posts the group command list. It is rate-limited like the other activities so
// it can't be used to flood the group, but costs nothing (no LLM call).
func (s *WhatsAppService) sendGroupHelp(groupJID string) {
	if !groupActivityAllowed(groupJID) {
		s.sendToGroup(groupJID, throttleMsg)
		return
	}
	s.sendToGroup(groupJID, groupHelpText())
}

// sendGroupQuiz posts ONE multiple-choice question as a native WhatsApp poll and stores the
// answer key for /greveal. The LLM output is parsed as strict JSON (BuildGroupQuizPrompt).
func (s *WhatsAppService) sendGroupQuiz(user *model.User, groupJID, theme string) {
	if !groupActivityAllowed(groupJID) {
		s.sendToGroup(groupJID, throttleMsg)
		return
	}
	prompt := s.engine.BuildGroupQuizPrompt(user, sanitizeUserInput(theme))
	raw, _, err := s.factory.GenerateResponseWithFailover(prompt, []model.Message{}, "[Generate the group quiz now as strict JSON.]", s.resolveGroupLLM(user))
	if err != nil {
		s.sendToGroup(groupJID, "I couldn't create the quiz right now. Please try again in a moment. 🙏")
		return
	}
	q, err := parseQuizPayload(raw)
	if err != nil {
		log.Printf("group quiz parse failed (group %s): %v", groupJID, err)
		s.sendToGroup(groupJID, "I couldn't create the quiz right now. Please try again in a moment. 🙏")
		return
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		log.Printf("invalid group JID %s: %v", groupJID, err)
		return
	}
	// BuildPollCreation returns a *waE2E.Message (aliased to waproto.Message); selectable=1 (single answer).
	poll := s.GetClient().BuildPollCreation(q.Question, q.Options, 1)
	if _, err := s.GetClient().SendMessage(context.Background(), jid, poll); err != nil {
		log.Printf("error sending poll to group %s: %v", groupJID, err)
		return
	}

	groupQuizMu.Lock()
	groupQuizState[groupJID] = groupQuizAnswer{
		correct:     q.Options[q.AnswerIndex],
		explanation: strings.TrimSpace(q.Explanation),
		createdAt:   time.Now(),
	}
	groupQuizMu.Unlock()

	s.sendToGroup(groupJID, "🗳️ Vote above! When you're ready, send */greveal* to see the answer.")
}

// revealGroupQuiz discloses the correct answer and explanation of the group's last /gquiz,
// then clears it (one reveal per quiz).
func (s *WhatsAppService) revealGroupQuiz(groupJID string) {
	groupQuizMu.Lock()
	ans, ok := groupQuizState[groupJID]
	if ok {
		delete(groupQuizState, groupJID)
	}
	groupQuizMu.Unlock()
	if !ok {
		s.sendToGroup(groupJID, "There's no quiz to reveal yet. Start one with */gquiz*.")
		return
	}
	msg := "✅ " + ans.correct
	if ans.explanation != "" {
		msg += "\n\n" + ans.explanation
	}
	s.sendToGroup(groupJID, msg)
}

// sendGroupWord posts a word of the day to the group (reuses the word-of-day prompt).
func (s *WhatsAppService) sendGroupWord(user *model.User, groupJID string) {
	if !groupActivityAllowed(groupJID) {
		s.sendToGroup(groupJID, throttleMsg)
		return
	}
	s.runGroupPrompt(user, groupJID, s.engine.BuildWordOfDayPrompt(user), "[Teach the group's word of the day now.]")
}

// sendGroupChallenge posts a short shared practice challenge to the group.
func (s *WhatsAppService) sendGroupChallenge(user *model.User, groupJID, theme string) {
	if !groupActivityAllowed(groupJID) {
		s.sendToGroup(groupJID, throttleMsg)
		return
	}
	s.runGroupPrompt(user, groupJID, s.engine.BuildChallengePrompt(user, sanitizeUserInput(theme)), "[Propose the group challenge now.]")
}

// runGroupPrompt runs a stateless group activity: one LLM call resolving the group's AI, and
// sends the text reply to the group. Mirrors runStudyPrompt but targets the group JID.
func (s *WhatsAppService) runGroupPrompt(user *model.User, groupJID, systemPrompt, trigger string) {
	response, _, err := s.factory.GenerateResponseWithFailover(systemPrompt, []model.Message{}, trigger, s.resolveGroupLLM(user))
	if err != nil {
		s.sendToGroup(groupJID, "I couldn't do that right now. Please try again in a moment. 🙏")
		return
	}
	s.sendToGroup(groupJID, response)
}
