// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"chatic/internal/model"
	"chatic/internal/repository"
)

type OnboardingService struct {
	userRepo *repository.UserRepository
}

func NewOnboardingService(userRepo *repository.UserRepository) *OnboardingService {
	return &OnboardingService{userRepo: userRepo}
}

// ProcessOnboarding drives the state machine that sets up a new student.
func (s *OnboardingService) ProcessOnboarding(user *model.User, text string) (string, error) {
	text = strings.TrimSpace(text)

	// If it is a test state, intercept and handle it separately
	if strings.HasPrefix(user.FlowState, "TEST_") {
		return s.handleTestFlow(user, text)
	}

	switch user.FlowState {
	case "INIT":
		user.FlowState = "AWAIT_NAME"
		s.userRepo.Update(user)
		return "👋 Hello! I'm Chatic, your private language tutor.\nTo get started, please tell me your *name*:", nil

	case "AWAIT_NAME":
		if text == "" {
			return "Please enter a valid name:", nil
		}
		user.Name = text
		user.FlowState = "AWAIT_BIRTHYEAR"
		s.userRepo.Update(user)
		return fmt.Sprintf("Nice to meet you, %s! In what *year were you born*? (e.g.: 2010)\nI use this only to keep our topics and activities right for your age.", text), nil

	case "AWAIT_BIRTHYEAR":
		year, ok := parseBirthYear(text)
		if !ok {
			// Mandatory: re-ask on any invalid/empty input, staying in this state.
			return "Please tell me a valid 4-digit *year of birth* (e.g.: 2010):", nil
		}
		user.BirthYear = year
		user.FlowState = "AWAIT_NATIVE"
		s.userRepo.Update(user)
		return "Got it! What is your *native language* (or support language)?\nExamples: Portuguese, Spanish, English.", nil

	case "AWAIT_NATIVE":
		code := parseLanguageCode(text)
		user.NativeLanguage = code
		user.FlowState = "AWAIT_TARGET"
		s.userRepo.Update(user)
		return "And which *language do you want to practice/learn*?\nExamples: English, Spanish, French, German, Japanese.", nil

	case "AWAIT_TARGET":
		code := parseLanguageCode(text)
		user.TargetLanguage = code
		user.FlowState = "AWAIT_LEVEL_CHOICE"
		s.userRepo.Update(user)
		return "As for your proficiency level, what do you prefer?\n\nType:\n- *1* to take a Quick Placement Test (3 questions)\n- *2* to skip the test and start from scratch (Beginner A1)", nil

	case "AWAIT_LEVEL_CHOICE":
		if text == "2" {
			user.Level = "A1"
			user.FlowState = "AWAIT_TEACHER_NAME"
			s.userRepo.Update(user)
			return "Level set to *A1* (starting from scratch).\n\nWhat would you like to call your language teacher?", nil
		} else if text == "1" {
			q1, hasTest := getFirstQuestion(user.TargetLanguage)
			if !hasTest {
				// Language with no registered questions: manual level selection
				user.FlowState = "AWAIT_LEVEL_MANUAL"
				s.userRepo.Update(user)
				return fmt.Sprintf("Placement test not yet available for this language.\n\nWhat is your approximate level in *%s*?\n\n- *A1* – Never studied\n- *A2* – Know basic words\n- *B1* – Can communicate\n- *B2* – Read and listen well\n- *C1* – Speak fluently\n- *C2* – Native level", user.TargetLanguage), nil
			}
			user.FlowState = "TEST_Q1:0"
			s.userRepo.Update(user)
			return "Starting the quick test! Answer only with the option letter (A, B or C).\n\n" + q1, nil
		} else {
			return "Invalid option. Type:\n- *1* to take the Quick Test\n- *2* to skip and start at A1", nil
		}

	case "AWAIT_LEVEL_MANUAL":
		validLevels := map[string]bool{"A1": true, "A2": true, "B1": true, "B2": true, "C1": true, "C2": true}
		level := strings.ToUpper(strings.TrimSpace(text))
		if !validLevels[level] {
			return "Please reply with *A1*, *A2*, *B1*, *B2*, *C1* or *C2*:", nil
		}
		user.Level = level
		user.FlowState = "AWAIT_TEACHER_NAME"
		s.userRepo.Update(user)
		return fmt.Sprintf("Level set to *%s*.\n\nWhat would you like to call your language teacher?", level), nil

	case "AWAIT_TEACHER_NAME":
		if text == "" {
			return "Please enter a name for your teacher:", nil
		}
		user.TeacherName = text
		user.FlowState = "AWAIT_INTERESTS"
		s.userRepo.Update(user)
		return fmt.Sprintf("Great, I'll be called *%s*! 😊\n\nNow, what are your *main interests or hobbies*?\n(E.g.: Technology, Sports, Cooking, Travel, Business)\nThis will help me guide our topics!", text), nil

	case "AWAIT_INTERESTS":
		if text == "" {
			return "Please enter at least one interest (e.g.: Travel):", nil
		}
		user.Interests = text
		user.OnboardingDone = true
		user.FlowState = "COMPLETE"
		s.userRepo.Update(user)

		welcomeMsg := fmt.Sprintf(
			"🎉 *Registration Completed Successfully!*\n\n"+
				"👤 *Name:* %s\n"+
				"🎂 *Age:* %d\n"+
				"👩‍🏫 *Teacher:* %s\n"+
				"🌐 *Native Language:* %s\n"+
				"🗣️ *Target Language:* %s\n"+
				"📊 *Level:* %s\n"+
				"🎨 *Interests:* %s\n\n"+
				"I'm ready! From now on, we'll speak only in %s. Type or send an audio message to start practicing! (Or type '/help' if you need to reconfigure).",
			user.Name, user.Age(), user.TeacherName, user.NativeLanguage, user.TargetLanguage, user.Level, user.Interests, user.TargetLanguage,
		)
		return welcomeMsg, nil

	default:
		// If already complete, restart
		user.OnboardingDone = false
		user.FlowState = "INIT"
		s.userRepo.Update(user)
		return s.ProcessOnboarding(user, text)
	}
}

// handleTestFlow manages the questions and correct-answer count for the placement test.
func (s *OnboardingService) handleTestFlow(user *model.User, text string) (string, error) {
	parts := strings.Split(user.FlowState, ":")
	currentState := parts[0]
	score, _ := strconv.Atoi(parts[1])

	ans := strings.ToUpper(strings.TrimSpace(text))
	if ans != "A" && ans != "B" && ans != "C" {
		return "Invalid option. Please reply only with *A*, *B* or *C*:", nil
	}

	_, q2, q3, a1, a2, a3 := getQuestions(user.TargetLanguage)

	switch currentState {
	case "TEST_Q1":
		if ans == a1 {
			score++
		}
		user.FlowState = fmt.Sprintf("TEST_Q2:%d", score)
		s.userRepo.Update(user)
		return q2, nil

	case "TEST_Q2":
		if ans == a2 {
			score++
		}
		user.FlowState = fmt.Sprintf("TEST_Q3:%d", score)
		s.userRepo.Update(user)
		return q3, nil

	case "TEST_Q3":
		if ans == a3 {
			score++
		}

		// Compute the level based on the score (0 to 3)
		lvl := "A1"
		switch score {
		case 1:
			lvl = "A2"
		case 2:
			lvl = "B1"
		case 3:
			lvl = "B2"
		}

		user.Level = lvl
		user.FlowState = "AWAIT_TEACHER_NAME"
		s.userRepo.Update(user)

		return fmt.Sprintf("Very good! You got %d out of 3 questions right. Your estimated level is *%s*.\n\nWhat would you like to call your language teacher?", score, lvl), nil
	}

	return "Test error. Restarting onboarding...", nil
}

// getFirstQuestion returns the first placement-test question and whether the language has a test available.
func getFirstQuestion(targetLang string) (q1 string, hasTest bool) {
	q1full, _, _, _, _, _ := getQuestions(targetLang)
	if q1full == "" {
		return "", false
	}
	return q1full, true
}

func getQuestions(targetLang string) (q1, q2, q3 string, a1, a2, a3 string) {
	switch targetLang {
	case "en":
		q1 = "Question 1/3 (Basic): What is the translation of 'Cachorro' (Portuguese for dog)?\n- A) Cat\n- B) Dog\n- C) Bird"
		a1 = "B"
		q2 = "Question 2/3 (Intermediate): Complete: 'She ___ to the gym every day.'\n- A) go\n- B) goes\n- C) going"
		a2 = "B"
		q3 = "Question 3/3 (Advanced): Complete: 'Had I known, I ___ you.'\n- A) would tell\n- B) will tell\n- C) would have told"
		a3 = "C"
	case "es":
		q1 = "Pregunta 1/3 (Básico): ¿Cómo se dice 'perro' en español?\n- A) Gato\n- B) Perro\n- C) Pájaro"
		a1 = "B"
		q2 = "Pregunta 2/3 (Intermedio): Completa: 'Ella ___ al gimnasio todos los días.'\n- A) ir\n- B) va\n- C) va a"
		a2 = "B"
		q3 = "Pregunta 3/3 (Avanzado): Completa: 'Si ___ sabido, te lo habría dicho.'\n- A) hubiera\n- B) habría\n- C) habré"
		a3 = "A"
	case "fr":
		q1 = "Question 1/3 (Basique): Comment dit-on 'chien' en français?\n- A) Chat\n- B) Chien\n- C) Oiseau"
		a1 = "B"
		q2 = "Question 2/3 (Intermédiaire): Complétez: 'Elle ___ au gymnase tous les jours.'\n- A) aller\n- B) va\n- C) vont"
		a2 = "B"
		q3 = "Question 3/3 (Avancé): Complétez: 'Si j'___ su, je te l'aurais dit.'\n- A) avais\n- B) aurais\n- C) aurais eu"
		a3 = "A"
	case "de":
		q1 = "Frage 1/3 (Einfach): Was bedeutet 'Hund'?\n- A) Katze\n- B) Hund\n- C) Vogel"
		a1 = "B"
		q2 = "Frage 2/3 (Mittel): Ergänze: 'Er ___ jeden Tag ins Fitnessstudio.'\n- A) gehe\n- B) geht\n- C) gehen"
		a2 = "B"
		q3 = "Frage 3/3 (Fortgeschritten): Ergänze: 'Wenn ich das gewusst ___, hätte ich dich angerufen.'\n- A) habe\n- B) hätte\n- C) wäre"
		a3 = "B"
	case "it":
		q1 = "Domanda 1/3 (Base): Come si dice 'cane' in italiano?\n- A) Gatto\n- B) Cane\n- C) Uccello"
		a1 = "B"
		q2 = "Domanda 2/3 (Intermedio): Completa: 'Lei ___ in palestra ogni giorno.'\n- A) va\n- B) vai\n- C) vanno"
		a2 = "A"
		q3 = "Domanda 3/3 (Avanzato): Completa: 'Se avessi saputo, ___ chiamato.'\n- A) avrei\n- B) avevo\n- C) avrò"
		a3 = "A"
	case "ja":
		q1 = "質問 1/3 (基礎): 「Thank you」は日本語で何と言いますか？\n- A) さようなら (goodbye)\n- B) ありがとう (thank you)\n- C) おはよう (good morning)"
		a1 = "B"
		q2 = "質問 2/3 (中級): 正しい助詞を選んでください: 「私は毎日ジム___行きます。」\n- A) を\n- B) に\n- C) の"
		a2 = "B"
		q3 = "質問 3/3 (上級): 「言う」の尊敬語はどれですか？\n- A) 申す (humble)\n- B) おっしゃる (respectful)\n- C) 言われる (passive)"
		a3 = "B"
	case "ko":
		q1 = "질문 1/3 (기초): 'Thank you'는 한국어로 무엇입니까?\n- A) 안녕히 가세요 (goodbye)\n- B) 감사합니다 (thank you)\n- C) 안녕하세요 (hello)"
		a1 = "B"
		q2 = "질문 2/3 (중급): 올바른 조사를 고르세요: '저는 매일 헬스장___ 갑니다.'\n- A) 을\n- B) 에\n- C) 로"
		a2 = "B"
		q3 = "질문 3/3 (고급): '말하다'의 높임말(존댓말)은 무엇입니까?\n- A) 말씀하시다 (honorific)\n- B) 여쭈다 (humble ask)\n- C) 드리다 (humble give)"
		a3 = "A"
	case "zh":
		q1 = "问题 1/3 (基础): 'Thank you' 用中文怎么说？\n- A) 再见 (goodbye)\n- B) 谢谢 (thank you)\n- C) 你好 (hello)"
		a1 = "B"
		q2 = "问题 2/3 (中级): 选择正确的量词: '一___书'\n- A) 个\n- B) 本\n- C) 只"
		a2 = "B"
		q3 = "问题 3/3 (高级): 选择正确的助词: '他累___走不动了。'\n- A) 得\n- B) 的\n- C) 地"
		a3 = "A"
	case "ru":
		q1 = "Вопрос 1/3 (базовый): Как сказать 'Thank you' по-русски?\n- A) До свидания (goodbye)\n- B) Спасибо (thank you)\n- C) Здравствуйте (hello)"
		a1 = "B"
		q2 = "Вопрос 2/3 (средний): Выберите правильную форму: 'Она ходит в спортзал каждый ___.'\n- A) день\n- B) дня\n- C) дне"
		a2 = "A"
		q3 = "Вопрос 3/3 (продвинутый): Выберите правильный падеж: 'Я горжусь ___.'\n- A) тебя\n- B) тобой\n- C) тебе"
		a3 = "B"
	default:
		// Language with no registered test: return empty to trigger manual level selection
		return "", "", "", "", "", ""
	}
	return
}

func parseLanguageCode(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	switch {
	case strings.Contains(l, "portug"):
		return "pt-BR"
	case strings.Contains(l, "ingl") || strings.Contains(l, "engl"):
		return "en"
	case strings.Contains(l, "espan") || strings.Contains(l, "span"):
		return "es"
	case strings.Contains(l, "franc") || strings.Contains(l, "frenc"):
		return "fr"
	case strings.Contains(l, "alem") || strings.Contains(l, "german") || strings.Contains(l, "deutsch"):
		return "de"
	case strings.Contains(l, "japon") || l == "ja" || strings.Contains(l, "japanese"):
		return "ja"
	case strings.Contains(l, "ital"):
		return "it"
	case strings.Contains(l, "coreano") || strings.Contains(l, "korean") || l == "ko":
		return "ko"
	case strings.Contains(l, "mandarin") || strings.Contains(l, "chines") || strings.Contains(l, "chinês") || l == "zh":
		return "zh"
	case strings.Contains(l, "russo") || strings.Contains(l, "russian") || l == "ru":
		return "ru"
	case strings.Contains(l, "árabe") || strings.Contains(l, "arabe") || strings.Contains(l, "arabic") || l == "ar":
		return "ar"
	case strings.Contains(l, "hindi") || l == "hi":
		return "hi"
	case strings.Contains(l, "holandes") || strings.Contains(l, "holandês") || strings.Contains(l, "dutch") || l == "nl":
		return "nl"
	case strings.Contains(l, "turco") || strings.Contains(l, "turkish") || l == "tr":
		return "tr"
	case strings.Contains(l, "polones") || strings.Contains(l, "polonês") || strings.Contains(l, "polish") || l == "pl":
		return "pl"
	default:
		// Accept any language code or name directly — the LLM will understand
		return l
	}
}

// birthYearRe extracts a 4-digit year starting with 19 or 20 from free text,
// so inputs like "2010", "born in 2010" or "05/2010" all resolve to the year.
var birthYearRe = regexp.MustCompile(`\b(19|20)\d{2}\b`)

// parseBirthYear returns a plausible year of birth from the student's text.
// Valid = a 19xx/20xx year within [1900, current year] yielding an age of at least 3.
// Anything else returns ok=false so the onboarding step re-asks (the field is mandatory).
func parseBirthYear(text string) (int, bool) {
	match := birthYearRe.FindString(strings.TrimSpace(text))
	if match == "" {
		return 0, false
	}
	year, err := strconv.Atoi(match)
	if err != nil {
		return 0, false
	}
	currentYear := time.Now().Year()
	if year < 1900 || year > currentYear || currentYear-year < 3 {
		return 0, false
	}
	return year, true
}
