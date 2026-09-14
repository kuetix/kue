package transitions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

const (
	providerOpenAI    = "openai"
	providerAnthropic = "anthropic"
	providerGemini    = "gemini"
)

type promptTransitions struct {
	workflow.BaseServiceTransition
}

func NewPromptTransitions() interfaces.ServiceTransitions {
	return &promptTransitions{}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type openAIChoice struct {
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
	Message      struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type providerError struct {
	Message string      `json:"message"`
	Type    string      `json:"type"`
	Code    interface{} `json:"code"`
}

type openAIResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
	Error   *providerError `json:"error,omitempty"`
}

type anthropicRequest struct {
	Model       string        `json:"model"`
	System      string        `json:"system,omitempty"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResponse struct {
	ID         string             `json:"id"`
	Model      string             `json:"model"`
	Role       string             `json:"role"`
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      anthropicUsage     `json:"usage"`
	Error      *providerError     `json:"error,omitempty"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Contents          []geminiContent         `json:"contents"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata geminiUsage       `json:"usageMetadata"`
	Error         *providerError    `json:"error,omitempty"`
}

type providerConfig struct {
	Name    string
	Model   string
	APIKey  string
	BaseURL string
}

func resolveFirstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func normalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", providerOpenAI:
		return providerOpenAI
	case providerAnthropic:
		return providerAnthropic
	case providerGemini:
		return providerGemini
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func resolveProviderConfig(provider, model, apiKey, baseURL string) providerConfig {
	cfg := providerConfig{Name: normalizeProvider(provider)}
	switch cfg.Name {
	case providerAnthropic:
		cfg.APIKey = firstNonEmpty(apiKey, resolveFirstEnv("AI_API_KEY", "ANTHROPIC_API_KEY"))
		cfg.Model = firstNonEmpty(model, resolveFirstEnv("AI_MODEL", "ANTHROPIC_MODEL"), "claude-3-5-sonnet-latest")
		cfg.BaseURL = firstNonEmpty(baseURL, resolveFirstEnv("AI_BASE_URL", "ANTHROPIC_BASE_URL"))
	case providerGemini:
		cfg.APIKey = firstNonEmpty(apiKey, resolveFirstEnv("AI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"))
		cfg.Model = firstNonEmpty(model, resolveFirstEnv("AI_MODEL", "GEMINI_MODEL"), "gemini-2.0-flash")
		cfg.BaseURL = firstNonEmpty(baseURL, resolveFirstEnv("AI_BASE_URL", "GEMINI_BASE_URL"))
	default:
		cfg.Name = providerOpenAI
		cfg.APIKey = firstNonEmpty(apiKey, resolveFirstEnv("AI_API_KEY", "OPENAI_API_KEY"))
		cfg.Model = firstNonEmpty(model, resolveFirstEnv("AI_MODEL", "OPENAI_MODEL"), "gpt-4.1-mini")
		cfg.BaseURL = firstNonEmpty(baseURL, resolveFirstEnv("AI_BASE_URL", "OPENAI_BASE_URL"))
	}
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveOpenAIEndpoint(baseURL string) string {
	clean := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if clean == "" {
		return "https://api.openai.com/v1/chat/completions"
	}
	if strings.HasSuffix(clean, "/chat/completions") {
		return clean
	}
	if strings.HasSuffix(clean, "/v1") {
		return clean + "/chat/completions"
	}
	return clean + "/v1/chat/completions"
}

func resolveAnthropicEndpoint(baseURL string) string {
	clean := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if clean == "" {
		return "https://api.anthropic.com/v1/messages"
	}
	if strings.HasSuffix(clean, "/messages") {
		return clean
	}
	if strings.HasSuffix(clean, "/v1") {
		return clean + "/messages"
	}
	return clean + "/v1/messages"
}

func resolveGeminiEndpoint(baseURL, model string) string {
	clean := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if clean == "" {
		return "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":generateContent"
	}
	if strings.Contains(clean, ":generateContent") {
		return clean
	}
	if strings.Contains(clean, "/models/") {
		return clean + ":generateContent"
	}
	return clean + "/models/" + model + ":generateContent"
}

func buildPromptMessages(systemPrompt, prompt string) []chatMessage {
	messages := make([]chatMessage, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	messages = append(messages, chatMessage{
		Role:    "user",
		Content: prompt,
	})
	return messages
}

func buildAnthropicMessages(prompt string) []chatMessage {
	return []chatMessage{{
		Role:    "user",
		Content: prompt,
	}}
}

func buildGeminiContents(prompt string) []geminiContent {
	return []geminiContent{{
		Role: "user",
		Parts: []geminiPart{{
			Text: prompt,
		}},
	}}
}

func buildGeminiSystemInstruction(systemPrompt string) *geminiContent {
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		return nil
	}
	return &geminiContent{
		Parts: []geminiPart{{
			Text: systemPrompt,
		}},
	}
}

func applyAdditionalHeaders(request *http.Request, headers map[string]interface{}) {
	for name, rawValue := range headers {
		headerName := strings.TrimSpace(name)
		if headerName == "" {
			continue
		}
		switch value := rawValue.(type) {
		case string:
			request.Header.Set(headerName, value)
		case []interface{}:
			for _, item := range value {
				request.Header.Add(headerName, fmt.Sprintf("%v", item))
			}
		case []string:
			for _, item := range value {
				request.Header.Add(headerName, item)
			}
		default:
			request.Header.Set(headerName, fmt.Sprintf("%v", value))
		}
	}
}

func doJSONRequest(method, endpoint string, body []byte, headers map[string]interface{}) ([]byte, int, error) {
	request, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	applyAdditionalHeaders(request, headers)

	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, response.StatusCode, err
	}
	return responseBody, response.StatusCode, nil
}

func decodeProviderError(raw []byte) string {
	var envelope struct {
		Error *providerError `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Error != nil && strings.TrimSpace(envelope.Error.Message) != "" {
		return envelope.Error.Message
	}
	return strings.TrimSpace(string(raw))
}

func executeOpenAIRequest(cfg providerConfig, prompt, systemPrompt string, temperature float64, maxTokens int, headers map[string]interface{}) (domain.FlowStepResult, int, error) {
	requestBody := openAIRequest{
		Model:       cfg.Model,
		Messages:    buildPromptMessages(systemPrompt, prompt),
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return domain.FlowStepResult{}, http.StatusInternalServerError, fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	requestHeaders := copyHeaders(headers)
	requestHeaders["Authorization"] = "Bearer " + cfg.APIKey

	responseBody, statusCode, err := doJSONRequest(http.MethodPost, resolveOpenAIEndpoint(cfg.BaseURL), body, requestHeaders)
	if err != nil {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("failed to execute OpenAI request: %w", err)
	}

	var payload openAIResponse
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("failed to decode OpenAI response: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return domain.FlowStepResult{}, statusCode, fmt.Errorf("openai request failed: %s", decodeProviderError(responseBody))
	}
	if len(payload.Choices) == 0 {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("openai response did not contain any choices")
	}

	firstChoice := payload.Choices[0]
	return domain.FlowStepResult{
		Success:    true,
		StatusCode: http.StatusOK,
		Response: map[string]interface{}{
			"provider":     providerOpenAI,
			"id":           payload.ID,
			"model":        payload.Model,
			"text":         firstChoice.Message.Content,
			"role":         firstChoice.Message.Role,
			"finishReason": firstChoice.FinishReason,
			"usage": map[string]interface{}{
				"promptTokens":     payload.Usage.PromptTokens,
				"completionTokens": payload.Usage.CompletionTokens,
				"totalTokens":      payload.Usage.TotalTokens,
			},
			"choices": payload.Choices,
		},
	}, http.StatusOK, nil
}

func executeAnthropicRequest(cfg providerConfig, prompt, systemPrompt string, temperature float64, maxTokens int, headers map[string]interface{}) (domain.FlowStepResult, int, error) {
	if maxTokens == 0 {
		maxTokens = 1024
	}

	requestBody := anthropicRequest{
		Model:       cfg.Model,
		System:      strings.TrimSpace(systemPrompt),
		Messages:    buildAnthropicMessages(prompt),
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return domain.FlowStepResult{}, http.StatusInternalServerError, fmt.Errorf("failed to marshal Anthropic request: %w", err)
	}

	requestHeaders := copyHeaders(headers)
	requestHeaders["x-api-key"] = cfg.APIKey
	if _, ok := requestHeaders["anthropic-version"]; !ok {
		requestHeaders["anthropic-version"] = "2023-06-01"
	}

	responseBody, statusCode, err := doJSONRequest(http.MethodPost, resolveAnthropicEndpoint(cfg.BaseURL), body, requestHeaders)
	if err != nil {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("failed to execute Anthropic request: %w", err)
	}

	var payload anthropicResponse
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("failed to decode Anthropic response: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return domain.FlowStepResult{}, statusCode, fmt.Errorf("anthropic request failed: %s", decodeProviderError(responseBody))
	}

	textParts := make([]string, 0, len(payload.Content))
	for _, content := range payload.Content {
		if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
			textParts = append(textParts, content.Text)
		}
	}
	if len(textParts) == 0 {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("anthropic response did not contain any text content")
	}

	return domain.FlowStepResult{
		Success:    true,
		StatusCode: http.StatusOK,
		Response: map[string]interface{}{
			"provider":     providerAnthropic,
			"id":           payload.ID,
			"model":        payload.Model,
			"text":         strings.Join(textParts, "\n"),
			"role":         payload.Role,
			"finishReason": payload.StopReason,
			"usage": map[string]interface{}{
				"promptTokens":     payload.Usage.InputTokens,
				"completionTokens": payload.Usage.OutputTokens,
				"totalTokens":      payload.Usage.InputTokens + payload.Usage.OutputTokens,
			},
			"content": payload.Content,
		},
	}, http.StatusOK, nil
}

func executeGeminiRequest(cfg providerConfig, prompt, systemPrompt string, temperature float64, maxTokens int, headers map[string]interface{}) (domain.FlowStepResult, int, error) {
	requestBody := geminiRequest{
		SystemInstruction: buildGeminiSystemInstruction(systemPrompt),
		Contents:          buildGeminiContents(prompt),
	}
	if temperature > 0 || maxTokens > 0 {
		requestBody.GenerationConfig = &geminiGenerationConfig{
			Temperature:     temperature,
			MaxOutputTokens: maxTokens,
		}
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return domain.FlowStepResult{}, http.StatusInternalServerError, fmt.Errorf("failed to marshal Gemini request: %w", err)
	}

	requestHeaders := copyHeaders(headers)
	endpoint := resolveGeminiEndpoint(cfg.BaseURL, cfg.Model)
	if _, ok := requestHeaders["x-goog-api-key"]; ok {
		// caller supplied it explicitly
	} else {
		parsedURL, parseErr := url.Parse(endpoint)
		if parseErr != nil {
			return domain.FlowStepResult{}, http.StatusInternalServerError, fmt.Errorf("failed to parse Gemini endpoint: %w", parseErr)
		}
		query := parsedURL.Query()
		query.Set("key", cfg.APIKey)
		parsedURL.RawQuery = query.Encode()
		endpoint = parsedURL.String()
	}

	responseBody, statusCode, err := doJSONRequest(http.MethodPost, endpoint, body, requestHeaders)
	if err != nil {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("failed to execute Gemini request: %w", err)
	}

	var payload geminiResponse
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("failed to decode Gemini response: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return domain.FlowStepResult{}, statusCode, fmt.Errorf("gemini request failed: %s", decodeProviderError(responseBody))
	}
	if len(payload.Candidates) == 0 {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("gemini response did not contain any candidates")
	}

	firstCandidate := payload.Candidates[0]
	textParts := make([]string, 0, len(firstCandidate.Content.Parts))
	for _, part := range firstCandidate.Content.Parts {
		if strings.TrimSpace(part.Text) != "" {
			textParts = append(textParts, part.Text)
		}
	}
	if len(textParts) == 0 {
		return domain.FlowStepResult{}, http.StatusBadGateway, fmt.Errorf("gemini response did not contain any text content")
	}

	return domain.FlowStepResult{
		Success:    true,
		StatusCode: http.StatusOK,
		Response: map[string]interface{}{
			"provider":     providerGemini,
			"model":        cfg.Model,
			"text":         strings.Join(textParts, "\n"),
			"role":         firstCandidate.Content.Role,
			"finishReason": firstCandidate.FinishReason,
			"usage": map[string]interface{}{
				"promptTokens":     payload.UsageMetadata.PromptTokenCount,
				"completionTokens": payload.UsageMetadata.CandidatesTokenCount,
				"totalTokens":      payload.UsageMetadata.TotalTokenCount,
			},
			"candidates": payload.Candidates,
		},
	}, http.StatusOK, nil
}

func copyHeaders(headers map[string]interface{}) map[string]interface{} {
	if headers == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

// validatePromptInputs checks the shared prompt parameters. It returns ok=false
// together with a populated error result when validation fails.
func validatePromptInputs(prompt string, temperature float64, maxTokens int) (string, domain.FlowStepResult, bool) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", domain.FlowStepResult{Error: fmt.Errorf("prompt is required"), StatusCode: http.StatusBadRequest}, false
	}
	if temperature < 0 {
		return "", domain.FlowStepResult{Error: fmt.Errorf("temperature must be greater than or equal to 0"), StatusCode: http.StatusBadRequest}, false
	}
	if maxTokens < 0 {
		return "", domain.FlowStepResult{Error: fmt.Errorf("maxTokens must be greater than or equal to 0"), StatusCode: http.StatusBadRequest}, false
	}
	return prompt, domain.FlowStepResult{}, true
}

// callProvider validates inputs, resolves the config for a fixed provider, and
// dispatches to the matching provider request. It is the shared core behind the
// per-provider transitions and the Prompt router.
func callProvider(providerName, prompt, model, apiKey, baseURL, systemPrompt string, temperature float64, maxTokens int, headers map[string]interface{}) (r domain.FlowStepResult) {
	prompt, invalid, ok := validatePromptInputs(prompt, temperature, maxTokens)
	if !ok {
		return invalid
	}

	cfg := resolveProviderConfig(providerName, model, apiKey, baseURL)
	if cfg.Name != providerOpenAI && cfg.Name != providerAnthropic && cfg.Name != providerGemini {
		r.Error = fmt.Errorf("unsupported provider: %s", providerName)
		r.StatusCode = http.StatusBadRequest
		return
	}
	if cfg.APIKey == "" {
		r.Error = fmt.Errorf("apiKey is required for provider %s", cfg.Name)
		r.StatusCode = http.StatusUnauthorized
		return
	}

	var (
		result     domain.FlowStepResult
		statusCode int
		err        error
	)

	switch cfg.Name {
	case providerAnthropic:
		result, statusCode, err = executeAnthropicRequest(cfg, prompt, systemPrompt, temperature, maxTokens, headers)
	case providerGemini:
		result, statusCode, err = executeGeminiRequest(cfg, prompt, systemPrompt, temperature, maxTokens, headers)
	default:
		result, statusCode, err = executeOpenAIRequest(cfg, prompt, systemPrompt, temperature, maxTokens, headers)
	}
	if err != nil {
		r.Error = err
		r.StatusCode = statusCode
		return
	}
	return result
}

// Resolve normalizes the requested provider and resolves its effective model
// and base URL (from arguments or environment) without calling the model. It is
// the dispatcher step used by the decomposed prompt workflow to branch to the
// correct provider transition. It never returns the API key.
func (t *promptTransitions) Resolve(provider, model, baseURL string) (r domain.FlowStepResult) {
	switch normalizeProvider(provider) {
	case providerOpenAI, providerAnthropic, providerGemini:
	default:
		r.Error = fmt.Errorf("unsupported provider: %s", provider)
		r.StatusCode = http.StatusBadRequest
		return
	}
	cfg := resolveProviderConfig(provider, model, "", baseURL)
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"provider": cfg.Name,
		"model":    cfg.Model,
		"baseURL":  cfg.BaseURL,
	}
	return
}

// OpenAI sends a prompt to an OpenAI-compatible chat completions endpoint.
func (t *promptTransitions) OpenAI(prompt, model, apiKey, baseURL, systemPrompt string, temperature float64, maxTokens int, headers map[string]interface{}) (r domain.FlowStepResult) {
	return callProvider(providerOpenAI, prompt, model, apiKey, baseURL, systemPrompt, temperature, maxTokens, headers)
}

// Anthropic sends a prompt to the Anthropic messages endpoint.
func (t *promptTransitions) Anthropic(prompt, model, apiKey, baseURL, systemPrompt string, temperature float64, maxTokens int, headers map[string]interface{}) (r domain.FlowStepResult) {
	return callProvider(providerAnthropic, prompt, model, apiKey, baseURL, systemPrompt, temperature, maxTokens, headers)
}

// Gemini sends a prompt to the Gemini generateContent endpoint.
func (t *promptTransitions) Gemini(prompt, model, apiKey, baseURL, systemPrompt string, temperature float64, maxTokens int, headers map[string]interface{}) (r domain.FlowStepResult) {
	return callProvider(providerGemini, prompt, model, apiKey, baseURL, systemPrompt, temperature, maxTokens, headers)
}

// Prompt routes to the requested provider. It is a thin dispatcher over the
// per-provider transitions (OpenAI, Anthropic, Gemini) and is kept for callers
// that prefer a single entrypoint.
func (t *promptTransitions) Prompt(provider, prompt, model, apiKey, baseURL, systemPrompt string, temperature float64, maxTokens int, headers map[string]interface{}) (r domain.FlowStepResult) {
	switch normalizeProvider(provider) {
	case providerAnthropic:
		return t.Anthropic(prompt, model, apiKey, baseURL, systemPrompt, temperature, maxTokens, headers)
	case providerGemini:
		return t.Gemini(prompt, model, apiKey, baseURL, systemPrompt, temperature, maxTokens, headers)
	case providerOpenAI:
		return t.OpenAI(prompt, model, apiKey, baseURL, systemPrompt, temperature, maxTokens, headers)
	default:
		r.Error = fmt.Errorf("unsupported provider: %s", provider)
		r.StatusCode = http.StatusBadRequest
		return
	}
}
