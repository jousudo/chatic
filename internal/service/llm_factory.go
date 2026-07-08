// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"chatic/config"
	"chatic/internal/model"
)

type LLMConfigParams struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

type LLMProvider interface {
	GenerateResponse(ctx context.Context, systemInstruction string, history []model.Message, currentMessage string, params *LLMConfigParams) (string, error)
}

// LLMFactory manages generation attempts using dynamic failover.
type LLMFactory struct {
	providers map[string]LLMProvider
}

func NewLLMFactory() *LLMFactory {
	factory := &LLMFactory{
		providers: make(map[string]LLMProvider),
	}
	factory.providers["gemini"] = &GeminiProvider{}
	factory.providers["openai"] = &OpenAIProvider{}
	factory.providers["claude"] = &ClaudeProvider{}
	factory.providers["ollama"] = &OllamaProvider{}
	return factory
}

// GenerateResponseWithFailover tries to generate a response using the custom params (if any), or the system's primary provider, falling back to the contingency providers if needed.
func (f *LLMFactory) GenerateResponseWithFailover(systemInstruction string, history []model.Message, currentMessage string, customParams *LLMConfigParams) (string, string, error) {
	primary := config.CurrentConfig.PrimaryLLMProvider
	order := []string{"gemini", "openai", "claude", "ollama"}

	// Reorder to try the primary first
	var sortedList []string

	// 1. Try the user's/group's custom provider first (if defined)
	if customParams != nil && customParams.Provider != "" {
		sortedList = append(sortedList, "custom_"+customParams.Provider)
	}

	// 2. Add the system's primary provider
	sortedList = append(sortedList, primary)

	// 3. Add the contingency providers
	for _, p := range order {
		if p != primary {
			sortedList = append(sortedList, p)
		}
	}

	var lastErr error
	for _, pName := range sortedList {
		isCustom := strings.HasPrefix(pName, "custom_")
		actualProviderName := strings.TrimPrefix(pName, "custom_")

		provider, exists := f.providers[actualProviderName]
		if !exists {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.CurrentConfig.LLMTimeoutSeconds)*time.Second)
		log.Printf("Trying to generate response with provider: %s (Custom: %v)", actualProviderName, isCustom)

		// Pass params on the custom round, otherwise pass nil (uses system defaults)
		var runParams *LLMConfigParams
		if isCustom {
			runParams = customParams
		}

		resp, err := provider.GenerateResponse(ctx, systemInstruction, history, currentMessage, runParams)
		if err == nil {
			cancel()
			return resp, actualProviderName, nil
		}

		cancel()
		log.Printf("Error in provider %s: %v. Trying failover...", actualProviderName, err)
		lastErr = err
	}

	return "", "", fmt.Errorf("all AI providers failed. Last error: %v", lastErr)
}

// GenerateResponseFromAudio processes an MP3 audio file directly via multimodal Gemini.
// The model transcribes the speech and replies as the tutor in a single API call.
// Automatic fallback to generic text if Gemini is not configured.
func (f *LLMFactory) GenerateResponseFromAudio(
	systemInstruction string,
	history []model.Message,
	audioPath string,
	customParams *LLMConfigParams,
) (string, string, error) {
	apiKey := config.CurrentConfig.GeminiAPIKey
	if customParams != nil && customParams.Provider == "gemini" && customParams.APIKey != "" {
		apiKey = customParams.APIKey
	}
	if apiKey == "" {
		return "", "", fmt.Errorf("Gemini not configured: set GEMINI_API_KEY to process audio")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.CurrentConfig.LLMTimeoutSeconds)*time.Second)
	defer cancel()

	// Reuse the stored instance to keep the round-robin counter
	gemini := f.providers["gemini"].(*GeminiProvider)
	resp, err := gemini.generateFromAudio(ctx, systemInstruction, history, audioPath, apiKey)
	if err != nil {
		return "", "", err
	}
	return resp, "gemini", nil
}

// isTransientStatus reports whether an HTTP status represents a temporary server
// failure worth retrying (e.g. Gemini 503 "high demand", rate limit 429).
func isTransientStatus(code int) bool {
	switch code {
	case http.StatusServiceUnavailable, // 503
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusGatewayTimeout:      // 504
		return true
	}
	return false
}

// doWithRetry runs the request, retrying on transient server errors with backoff
// (0.7s, 1.4s). It resets the body on each attempt from bodyBytes, since req.Body is
// consumed on Do. On the last attempt it returns the response as-is (even if transient)
// so the caller can report the real error; it respects the context deadline.
func doWithRetry(req *http.Request, bodyBytes []byte) (*http.Response, error) {
	const maxAttempts = 2
	var resp *http.Response
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-req.Context().Done():
				return resp, req.Context().Err()
			case <-time.After(time.Duration(attempt) * 700 * time.Millisecond):
			}
		}
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			req.ContentLength = int64(len(bodyBytes))
		}
		resp, err = (&http.Client{}).Do(req)
		if err != nil {
			continue // network/timeout error: try again
		}
		if isTransientStatus(resp.StatusCode) && attempt < maxAttempts-1 {
			resp.Body.Close()
			log.Printf("Provider returned %d (transient); attempt %d/%d, waiting for backoff", resp.StatusCode, attempt+1, maxAttempts)
			continue
		}
		return resp, nil
	}
	return resp, err
}

// splitKeys splits a comma-separated string of keys.
func splitKeys(raw string) []string {
	var keys []string
	for _, k := range strings.Split(raw, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// rotateKey selects the next key round-robin from the pool using the given atomic
// counter. Returns "" for an empty pool and the sole key for a single-key pool
// (without touching the counter).
func rotateKey(idx *uint64, keys []string) string {
	switch len(keys) {
	case 0:
		return ""
	case 1:
		return keys[0]
	default:
		n := atomic.AddUint64(idx, 1)
		return keys[n%uint64(len(keys))]
	}
}

// --- Gemini Provider implementation (HTTP) ---

// GeminiProvider keeps an atomic counter to rotate the pool keys round-robin.
type GeminiProvider struct {
	keyIdx uint64
}

// nextKey selects the next pool key round-robin.
// If customKey is set (may be comma-separated), it rotates among those.
// Otherwise it uses the system pool (config.GeminiAPIKeys).
func (g *GeminiProvider) nextKey(customKey string) string {
	if customKey != "" {
		return rotateKey(&g.keyIdx, splitKeys(customKey))
	}
	if keys := config.CurrentConfig.GeminiAPIKeys; len(keys) > 0 {
		return rotateKey(&g.keyIdx, keys)
	}
	return config.CurrentConfig.GeminiAPIKey
}

func (g *GeminiProvider) GenerateResponse(ctx context.Context, systemInstruction string, history []model.Message, currentMessage string, params *LLMConfigParams) (string, error) {
	customKey := ""
	if params != nil {
		customKey = params.APIKey
	}
	apiKey := g.nextKey(customKey)
	if apiKey == "" {
		return "", fmt.Errorf("GeminiAPIKey not configured")
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", apiKey)

	type Part struct {
		Text string `json:"text"`
	}
	type Content struct {
		Role  string `json:"role"`
		Parts []Part `json:"parts"`
	}
	type SystemInstruction struct {
		Parts []Part `json:"parts"`
	}
	type RequestBody struct {
		Contents          []Content         `json:"contents"`
		SystemInstruction SystemInstruction `json:"systemInstruction"`
	}

	var contents []Content

	// Convert history to the Gemini format
	for _, msg := range history {
		// Correct handling of the Gemini role
		geminiRole := "user"
		if msg.Sender == "bot" {
			geminiRole = "model"
		}
		contents = append(contents, Content{
			Role:  geminiRole,
			Parts: []Part{{Text: msg.Content}},
		})
	}

	// Add the current message
	contents = append(contents, Content{
		Role:  "user",
		Parts: []Part{{Text: currentMessage}},
	})

	reqBody := RequestBody{
		Contents: contents,
		SystemInstruction: SystemInstruction{
			Parts: []Part{{Text: systemInstruction}},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := doWithRetry(req, jsonBytes)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("invalid HTTP status: %d. Details: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("empty response from the Gemini API")
}

// generateFromAudio sends an MP3 file as inlineData to Gemini.
// The model understands the speech and replies as a language tutor in a single call.
func (g *GeminiProvider) generateFromAudio(ctx context.Context, systemInstruction string, history []model.Message, audioPath string, apiKey string) (string, error) {
	audioData, err := os.ReadFile(audioPath)
	if err != nil {
		return "", fmt.Errorf("error reading audio: %w", err)
	}
	b64Audio := base64.StdEncoding.EncodeToString(audioData)

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", apiKey)

	type TextPart struct {
		Text string `json:"text"`
	}
	type InlineData struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	}
	type AudioPart struct {
		InlineData InlineData `json:"inlineData"`
	}
	type TextContent struct {
		Role  string     `json:"role"`
		Parts []TextPart `json:"parts"`
	}
	type AudioContent struct {
		Role  string      `json:"role"`
		Parts interface{} `json:"parts"`
	}

	// Text history
	var historyContents []interface{}
	for _, msg := range history {
		role := "user"
		if msg.Sender == "bot" {
			role = "model"
		}
		historyContents = append(historyContents, TextContent{
			Role:  role,
			Parts: []TextPart{{Text: msg.Content}},
		})
	}

	// Current message: inline audio + text instruction
	type MixedPart struct {
		Text       *string     `json:"text,omitempty"`
		InlineData *InlineData `json:"inlineData,omitempty"`
	}
	type MixedContent struct {
		Role  string      `json:"role"`
		Parts []MixedPart `json:"parts"`
	}
	hint := "Listen to the voice message above and respond as a language tutor, naturally continuing the conversation."
	audioContent := MixedContent{
		Role: "user",
		Parts: []MixedPart{
			{InlineData: &InlineData{MimeType: "audio/mp3", Data: b64Audio}},
			{Text: &hint},
		},
	}
	historyContents = append(historyContents, audioContent)

	type SystemPart struct {
		Parts []TextPart `json:"parts"`
	}
	reqBody := map[string]interface{}{
		"contents":          historyContents,
		"systemInstruction": SystemPart{Parts: []TextPart{{Text: systemInstruction}}},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := doWithRetry(req, jsonBytes)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gemini audio HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}
	return "", fmt.Errorf("Gemini audio returned an empty response")
}

// --- OpenAI Provider implementation (HTTP) ---

// OpenAIProvider keeps an atomic counter to rotate the pool keys round-robin.
type OpenAIProvider struct {
	keyIdx uint64
}

// nextKey selects the next pool key round-robin (custom keys first, then the system pool).
func (o *OpenAIProvider) nextKey(customKey string) string {
	if customKey != "" {
		return rotateKey(&o.keyIdx, splitKeys(customKey))
	}
	if keys := config.CurrentConfig.OpenaiAPIKeys; len(keys) > 0 {
		return rotateKey(&o.keyIdx, keys)
	}
	return config.CurrentConfig.OpenaiAPIKey
}

func (o *OpenAIProvider) GenerateResponse(ctx context.Context, systemInstruction string, history []model.Message, currentMessage string, params *LLMConfigParams) (string, error) {
	customKey := ""
	modelName := "gpt-4o-mini"
	if params != nil {
		customKey = params.APIKey
		if params.Model != "" {
			modelName = params.Model
		}
	}
	apiKey := o.nextKey(customKey)
	if apiKey == "" {
		return "", fmt.Errorf("OpenaiAPIKey not configured")
	}

	url := "https://api.openai.com/v1/chat/completions"

	type MessageObj struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type RequestBody struct {
		Model    string       `json:"model"`
		Messages []MessageObj `json:"messages"`
	}

	var messages []MessageObj
	messages = append(messages, MessageObj{Role: "system", Content: systemInstruction})

	for _, msg := range history {
		role := "user"
		if msg.Sender == "bot" {
			role = "assistant"
		}
		messages = append(messages, MessageObj{Role: role, Content: msg.Content})
	}

	messages = append(messages, MessageObj{Role: "user", Content: currentMessage})

	reqBody := RequestBody{
		Model:    modelName,
		Messages: messages,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("invalid HTTP status OpenAI: %d. Details: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("empty response from the OpenAI API")
}

// --- Claude Provider implementation (HTTP) ---

// ClaudeProvider keeps an atomic counter to rotate the pool keys round-robin.
type ClaudeProvider struct {
	keyIdx uint64
}

// nextKey selects the next pool key round-robin (custom keys first, then the system pool).
func (c *ClaudeProvider) nextKey(customKey string) string {
	if customKey != "" {
		return rotateKey(&c.keyIdx, splitKeys(customKey))
	}
	if keys := config.CurrentConfig.ClaudeAPIKeys; len(keys) > 0 {
		return rotateKey(&c.keyIdx, keys)
	}
	return config.CurrentConfig.ClaudeAPIKey
}

func (c *ClaudeProvider) GenerateResponse(ctx context.Context, systemInstruction string, history []model.Message, currentMessage string, params *LLMConfigParams) (string, error) {
	customKey := ""
	modelName := "claude-3-5-haiku-latest"
	if params != nil {
		customKey = params.APIKey
		if params.Model != "" {
			modelName = params.Model
		}
	}
	apiKey := c.nextKey(customKey)
	if apiKey == "" {
		return "", fmt.Errorf("ClaudeAPIKey not configured")
	}

	url := "https://api.anthropic.com/v1/messages"

	type MessageObj struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type RequestBody struct {
		Model     string       `json:"model"`
		System    string       `json:"system"`
		Messages  []MessageObj `json:"messages"`
		MaxTokens int          `json:"max_tokens"`
	}

	var messages []MessageObj
	for _, msg := range history {
		role := "user"
		if msg.Sender == "bot" {
			role = "assistant"
		}
		messages = append(messages, MessageObj{Role: role, Content: msg.Content})
	}

	messages = append(messages, MessageObj{Role: "user", Content: currentMessage})

	reqBody := RequestBody{
		Model:     modelName,
		System:    systemInstruction,
		Messages:  messages,
		MaxTokens: 1024,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("invalid HTTP status Claude: %d. Details: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}

	return "", fmt.Errorf("empty response from the Claude API")
}

// --- Ollama Provider implementation (Local) ---
type OllamaProvider struct{}

func (ol *OllamaProvider) GenerateResponse(ctx context.Context, systemInstruction string, history []model.Message, currentMessage string, params *LLMConfigParams) (string, error) {
	apiBase := config.CurrentConfig.OllamaAPIBase
	modelName := config.CurrentConfig.OllamaModel
	if params != nil && params.BaseURL != "" {
		apiBase = params.BaseURL
	}
	if params != nil && params.Model != "" {
		modelName = params.Model
	}
	if apiBase == "" {
		return "", fmt.Errorf("Ollama API base URL not configured")
	}

	url := fmt.Sprintf("%s/api/chat", apiBase)

	type MessageObj struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type RequestBody struct {
		Model    string       `json:"model"`
		Messages []MessageObj `json:"messages"`
		Stream   bool         `json:"stream"`
	}

	var messages []MessageObj
	messages = append(messages, MessageObj{Role: "system", Content: systemInstruction})

	for _, msg := range history {
		role := "user"
		if msg.Sender == "bot" {
			role = "assistant"
		}
		messages = append(messages, MessageObj{Role: role, Content: msg.Content})
	}

	messages = append(messages, MessageObj{Role: "user", Content: currentMessage})

	reqBody := RequestBody{
		Model:    modelName,
		Messages: messages,
		Stream:   false,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("invalid HTTP status Ollama: %d. Details: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	if result.Message.Content != "" {
		return result.Message.Content, nil
	}

	return "", fmt.Errorf("empty response from the Ollama API")
}
