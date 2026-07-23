// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"chatic/config"
)

type TTSService struct {
	storageDir string
}

func NewTTSService(storageDir string) *TTSService {
	return &TTSService{storageDir: storageDir}
}

// ttsHTTPClient bounds every direct (non-websocket) TTS HTTP request with a hard timeout.
// Without it, a hung TTS provider would tie up the single FIFO queue worker indefinitely,
// stalling all other users (OWASP LLM10 — Unbounded Consumption / availability).
var ttsHTTPClient = &http.Client{Timeout: 20 * time.Second}

// levelToEdgeRate maps the CEFR level to the Edge TTS speed modifier.
// Beginners hear it slower for better comprehension; advanced learners at native pace.
func levelToEdgeRate(level string) string {
	switch level {
	case "A1":
		return "-20%"
	case "A2":
		return "-10%"
	case "B1":
		return "+0%"
	case "B2":
		return "+5%"
	case "C1":
		return "+10%"
	case "C2":
		return "+15%"
	default:
		return "+0%"
	}
}

// levelToGoogleRate maps the CEFR level to the Google Cloud TTS speakingRate (0.25–4.0).
func levelToGoogleRate(level string) float64 {
	switch level {
	case "A1":
		return 0.80
	case "A2":
		return 0.90
	case "B1":
		return 1.00
	case "B2":
		return 1.05
	case "C1":
		return 1.10
	case "C2":
		return 1.15
	default:
		return 1.00
	}
}

var (
	mdLinkPattern   = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	mdHeaderPattern = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdListPattern   = regexp.MustCompile(`(?m)^[ \t]*[-*]\s+`)
	mdEmphasisChars = strings.NewReplacer("**", "", "__", "", "*", "", "_", "", "~~", "", "`", "")
)

// stripMarkdownForSpeech removes Markdown markup (bold, italic, code, links,
// headers, lists) from the text before synthesizing audio, so the TTS does not "read"
// the symbols out loud (e.g. "asterisk how are you asterisk"). WhatsApp text messages
// keep the original Markdown, since the app renders *bold* and _italic_ natively.
func stripMarkdownForSpeech(text string) string {
	text = mdLinkPattern.ReplaceAllString(text, "$1")
	text = mdHeaderPattern.ReplaceAllString(text, "")
	text = mdListPattern.ReplaceAllString(text, "")
	text = mdEmphasisChars.Replace(text)
	return strings.TrimSpace(text)
}

// TextToSpeech converts text into MP3 audio and returns the path of the created file.
// Hierarchy: Google Cloud TTS → Edge TTS → Google Translate (fallback)
// The level parameter (CEFR: A1–C2) adjusts the speech speed to the student's level.
func (s *TTSService) TextToSpeech(text, langCode, level string) (string, error) {
	text = stripMarkdownForSpeech(text)
	if err := os.MkdirAll(s.storageDir, os.ModePerm); err != nil {
		return "", err
	}

	if config.CurrentConfig.GoogleTTSAPIKey != "" {
		return s.googleCloudTTS(text, langCode, level)
	}
	path, err := s.edgeTTS(text, langCode, level)
	if err == nil {
		return path, nil
	}
	return s.googleTranslateTTS(text, langCode)
}

// googleCloudTTS uses the official Google Cloud Text-to-Speech API.
// Free tier: 4M chars/month (Standard) or 1M chars/month (Neural2/WaveNet).
func (s *TTSService) googleCloudTTS(text, langCode, level string) (string, error) {
	if len(text) > 4500 {
		text = text[:4497] + "..."
	}

	bcp47 := toBCP47(langCode)

	reqBody := map[string]interface{}{
		"input": map[string]string{"text": text},
		"voice": map[string]interface{}{
			"languageCode": bcp47,
			"ssmlGender":   "FEMALE",
		},
		"audioConfig": map[string]interface{}{
			"audioEncoding": "MP3",
			"speakingRate":  levelToGoogleRate(level),
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	apiURL := fmt.Sprintf("https://texttospeech.googleapis.com/v1/text:synthesize?key=%s", config.CurrentConfig.GoogleTTSAPIKey)
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ttsHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Google Cloud TTS HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.AudioContent == "" {
		return "", fmt.Errorf("Google Cloud TTS returned empty audio")
	}

	audioData, err := base64.StdEncoding.DecodeString(result.AudioContent)
	if err != nil {
		return "", err
	}

	tmpFile, err := os.CreateTemp(s.storageDir, "tts_*.mp3")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(audioData); err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

// edgeTTS uses the Microsoft Edge TTS WebSocket endpoint.
// Neural quality without authentication. Supports ~40 languages.
func (s *TTSService) edgeTTS(text, langCode, level string) (string, error) {
	if len(text) > 4000 {
		text = text[:3997] + "..."
	}

	voice := toEdgeVoice(langCode)
	connID := strings.ReplaceAll(uuid.New().String(), "-", "")
	wsURL := fmt.Sprintf(
		"wss://speech.platform.bing.com/consumer/speech/synthesize/realtimestreaming/v1/?TrustedClientToken=6A5AA1D4EAFF4E9FB37E23D68491D6F4&ConnectionId=%s",
		connID,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin":     []string{"chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"},
			"User-Agent": []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("edge TTS connect: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	timestamp := time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")

	configMsg := fmt.Sprintf(
		"X-Timestamp:%s\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n"+
			`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"true"},"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}`,
		timestamp,
	)
	if err := conn.Write(ctx, websocket.MessageText, []byte(configMsg)); err != nil {
		return "", fmt.Errorf("edge TTS config: %w", err)
	}

	reqID := strings.ReplaceAll(uuid.New().String(), "-", "")
	escapedText := html.EscapeString(text)
	ssml := fmt.Sprintf(
		"<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='%s'>"+
			"<voice name='%s'><prosody pitch='+0Hz' rate='%s' volume='+0%%'>%s</prosody></voice></speak>",
		toBCP47(langCode), voice, levelToEdgeRate(level), escapedText,
	)
	ssmlMsg := fmt.Sprintf(
		"X-RequestId:%s\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:%s\r\nPath:ssml\r\n\r\n%s",
		reqID, timestamp, ssml,
	)
	if err := conn.Write(ctx, websocket.MessageText, []byte(ssmlMsg)); err != nil {
		return "", fmt.Errorf("edge TTS ssml: %w", err)
	}

	audioSeparator := []byte("Path:audio\r\n\r\n")
	var audioData []byte

	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		switch msgType {
		case websocket.MessageText:
			if strings.Contains(string(data), "Path:turn.end") {
				goto done
			}
		case websocket.MessageBinary:
			if idx := bytes.Index(data, audioSeparator); idx != -1 {
				audioData = append(audioData, data[idx+len(audioSeparator):]...)
			}
		}
	}
done:

	if len(audioData) == 0 {
		return "", fmt.Errorf("edge TTS returned empty audio")
	}

	tmpFile, err := os.CreateTemp(s.storageDir, "tts_*.mp3")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(audioData); err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

// googleTranslateTTS uses the unofficial public Google Translate endpoint as a last fallback.
// Limit: ~200 characters per request.
func (s *TTSService) googleTranslateTTS(text, langCode string) (string, error) {
	if len(text) > 200 {
		text = text[:197] + "..."
	}

	bcp47 := toBCP47(langCode)
	apiURL := fmt.Sprintf(
		"https://translate.google.com/translate_tts?ie=UTF-8&tl=%s&client=tw-ob&q=%s",
		url.QueryEscape(bcp47),
		url.QueryEscape(text),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := ttsHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Translate TTS HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(s.storageDir, "tts_*.mp3")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

// toEdgeVoice maps the language code to the Edge TTS neural voice name.
func toEdgeVoice(langCode string) string {
	voices := map[string]string{
		"pt-br": "pt-BR-FranciscaNeural",
		"pt":    "pt-PT-RaquelNeural",
		"en":    "en-US-JennyNeural",
		"es":    "es-ES-ElviraNeural",
		"fr":    "fr-FR-DeniseNeural",
		"de":    "de-DE-KatjaNeural",
		"ja":    "ja-JP-NanamiNeural",
		"it":    "it-IT-ElsaNeural",
		"ko":    "ko-KR-SunHiNeural",
		"zh":    "zh-CN-XiaoxiaoNeural",
		"ru":    "ru-RU-SvetlanaNeural",
		"ar":    "ar-EG-SalmaNeural",
		"hi":    "hi-IN-SwaraNeural",
		"nl":    "nl-NL-ColetteNeural",
		"pl":    "pl-PL-ZofiaNeural",
		"tr":    "tr-TR-EmelNeural",
	}
	if v, ok := voices[strings.ToLower(langCode)]; ok {
		return v
	}
	return "en-US-JennyNeural"
}

// toBCP47 converts internal codes to the BCP-47 format required by the Google and Edge APIs.
func toBCP47(code string) string {
	mapping := map[string]string{
		"en":    "en-US",
		"es":    "es-ES",
		"fr":    "fr-FR",
		"de":    "de-DE",
		"ja":    "ja-JP",
		"it":    "it-IT",
		"ko":    "ko-KR",
		"zh":    "zh-CN",
		"ru":    "ru-RU",
		"ar":    "ar-EG",
		"hi":    "hi-IN",
		"nl":    "nl-NL",
		"pl":    "pl-PL",
		"tr":    "tr-TR",
		"pt-br": "pt-BR",
		"pt":    "pt-PT",
	}
	if bcp47, ok := mapping[strings.ToLower(code)]; ok {
		return bcp47
	}
	return code
}
