package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/logger"
)

type baseHTTPClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
	logger  logger.Logger
}

func NewBaseHTTPClient(client *http.Client, baseURL, apiKey string, log logger.Logger) *baseHTTPClient {
	return &baseHTTPClient{
		client:  client,
		baseURL: baseURL,
		apiKey:  apiKey,
		logger:  log,
	}
}

func (c *baseHTTPClient) logRequest(req *http.Request, body []byte) {
	var bodyData any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &bodyData); err == nil {
			if m, ok := bodyData.(map[string]any); ok {
				truncateLargeFields(m)
			}
		}
	}

	logData := map[string]any{
		"url":    req.URL.String(),
		"method": req.Method,
		"body":   bodyData,
	}

	jsonData, err := json.Marshal(logData)
	if err != nil {
		c.logger.WithError(err).WithField("data", logData).Error("Fail marshal json for request")
	}
	c.logger.WithField("request", string(jsonData)).Debug("HTTP request")
}

func truncateLargeFields(data map[string]any) {
	for k, v := range data {
		switch val := v.(type) {
		case string:
			if (k == "url" || k == "content" || k == "text" || k == "file_data") && len(val) > 1000 {
				data[k] = val[:1000] + "...[truncated]"
			}
		case map[string]any:
			truncateLargeFields(val)
		case []any:
			for _, item := range val {
				if m, ok := item.(map[string]any); ok {
					truncateLargeFields(m)
				}
			}
		}
	}
}

func (c *baseHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if c.baseURL != "" && !strings.HasPrefix(req.URL.String(), "http") {
		req.URL, _ = url.Parse(fmt.Sprintf(
			"%s/%s",
			strings.TrimSuffix(c.baseURL, "/"),
			strings.TrimPrefix(req.URL.String(), "/"),
		))
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewBuffer(body))
	}

	c.logRequest(req, body)

	return c.client.Do(req)
}

type ModelParams struct {
	Stream           *bool                 `json:"stream,omitzero"`
	Temperature      *float32              `json:"temperature,omitzero"`
	MaxTokens        *int                  `json:"max_tokens,omitzero"`
	TopP             *float32              `json:"top_p,omitzero"`
	FrequencyPenalty *float32              `json:"frequency_penalty,omitzero"`
	PresencePenalty  *float32              `json:"presence_penalty,omitzero"`
	StopSequences    []string              `json:"stop_sequences,omitzero"`
	Reasoning        *ModelReasoningParams `json:"reasoning,omitzero"`
}

func NewModelParamsFromMap(params map[string]any) (ModelParams, error) {
	var result ModelParams
	for k, v := range params {
		switch k {
		case "stream":
			if val, ok := v.(bool); ok {
				result.Stream = &val
			}
		case "temperature":
			if val, ok := v.(float32); ok {
				result.Temperature = &val
			}
		case "max_tokens":
			if val, ok := v.(int); ok {
				result.MaxTokens = &val
			}
		case "top_p":
			if val, ok := v.(float32); ok {
				result.TopP = &val
			}
		case "frequency_penalty":
			if val, ok := v.(float32); ok {
				result.FrequencyPenalty = &val
			}
		case "presence_penalty":
			if val, ok := v.(float32); ok {
				result.PresencePenalty = &val
			}
		case "stop_sequences":
			if val, ok := v.([]string); ok {
				result.StopSequences = val
			}
		case "reasoning":
			if reasoningMap, ok := v.(map[string]any); ok {
				var reasoningParams ModelReasoningParams
				for rk, rv := range reasoningMap {
					switch rk {
					case "enabled":
						if val, ok := rv.(bool); ok {
							reasoningParams.Enabled = &val
						}
					case "exclude":
						if val, ok := rv.(bool); ok {
							reasoningParams.Exclude = &val
						}
					case "max_tokens":
						if val, ok := rv.(int); ok {
							reasoningParams.MaxTokens = &val
						}
					case "effort":
						if val, ok := rv.(string); ok {
							reasoningParams.Effort = &val
						}
					}
				}
				result.Reasoning = &reasoningParams
			}
		}
	}
	return result, nil
}

func (base ModelParams) Merge(override ModelParams) ModelParams {
	if override.Stream != nil {
		base.Stream = override.Stream
	}
	if override.Temperature != nil {
		base.Temperature = override.Temperature
	}
	if override.MaxTokens != nil {
		base.MaxTokens = override.MaxTokens
	}
	if override.TopP != nil {
		base.TopP = override.TopP
	}
	if override.FrequencyPenalty != nil {
		base.FrequencyPenalty = override.FrequencyPenalty
	}
	if override.PresencePenalty != nil {
		base.PresencePenalty = override.PresencePenalty
	}
	if override.StopSequences != nil {
		base.StopSequences = override.StopSequences
	}
	if override.Reasoning != nil {
		base.Reasoning = override.Reasoning
	}
	return base
}

type ModelReasoningParams struct {
	Enabled   *bool   `json:"enabled,omitzero"`
	Exclude   *bool   `json:"exclude,omitzero"`
	MaxTokens *int    `json:"max_tokens,omitzero"`
	Effort    *string `json:"effort,omitzero"`
}

type ChatService interface {
	GetCurrentModelSpec(ctx context.Context, chatID int64) (string, error)
	MergeModelParams(chatID int64, provider, alias, prompt string, requestParams ModelParams) (ModelParams, error)
}

type Provider interface {
	Name() string
	Ask(ctx context.Context, request CompletionRequest, headers map[string]string) (string, string, *CompletionResponse, *ModelInfo, error)
	AskStream(ctx context.Context, request CompletionRequest, headers map[string]string) (<-chan Chunk, *ModelInfo, error)
	CreateRequest(stream bool, messages []Message, tools []Tool, model *ModelInfo, params ModelParams, webSearch bool) CompletionRequest
	GetModels(ctx context.Context, onlyFree, fresh bool) (map[string]*ModelInfo, error)
	GetDefaultModel() string
	GetModelInfo(name string) (*ModelInfo, error)
}

type AnnotationContent struct {
	Type string `json:"type"` // "text", "image_url", "file"
	Text string `json:"text,omitzero"`
	File struct {
		Name    string    `json:"name"`
		Hash    string    `json:"hash"`
		Content []Content `json:"content"`
	} `json:"file,omitzero"`
}

type Content struct {
	Type     string `json:"type"` // "text", "image_url", "file"
	Text     string `json:"text,omitempty"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url,omitzero"`
	File struct {
		Filename string `json:"filename"`
		FileData string `json:"file_data"`
	} `json:"file,omitzero"`
	InputAudio struct {
		Data   string `json:"data"`
		Format string `json:"format"`
	} `json:"input_audio,omitzero"`
	Annotations []AnnotationContent `json:"annotations,omitempty"`
}

type Message struct {
	Role string `json:"role"`
	// for multimodal models (e.g. openrouter/openai)
	Content []Content `json:"-"`
	// for other (e.g. DeepSeek)
	Text string `json:"-"`

	Name       string     `json:"name,omitzero"`
	ToolCallID string     `json:"tool_call_id,omitzero"`
	ToolCalls  []ToolCall `json:"tool_calls,omitzero"`
}

func (m Message) HasFiles() bool {
	for _, content := range m.Content {
		if content.Type == "file" {
			return true
		}
	}
	return false
}

func (m Message) MarshalJSON() ([]byte, error) {
	type Alias Message
	aux := &struct {
		*Alias
		Content any `json:"content,omitzero"`
	}{
		Alias: (*Alias)(&m),
	}

	if len(m.Content) > 0 {
		aux.Content = m.Content
	} else {
		aux.Content = m.Text
	}

	if m.ToolCallID == "" {
		aux.ToolCallID = ""
	}

	return json.Marshal(aux)
}

func (m *Message) UnmarshalJSON(data []byte) error {
	type Alias Message
	aux := &struct {
		*Alias
		Content any `json:"content,omitzero"`
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Process content
	switch content := aux.Content.(type) {
	case string:
		m.Text = content
	case []any:
		// Convert []interface{} to []Content
		var contents []Content
		raw, _ := json.Marshal(content)
		if err := json.Unmarshal(raw, &contents); err != nil {
			return err
		}
		m.Content = contents
	case nil:
		// Do nothing, leave fields empty
	default:
		return fmt.Errorf("unexpected content type: %T", content)
	}

	return nil
}

type Plugin struct {
	ID  string `json:"id"`
	PDF struct {
		Engine string `json:"engine,omitempty"`
	} `json:"pdf,omitzero"`
	MaxResults   int    `json:"max_results,omitempty"`
	SearchPrompt string `json:"search_prompt,omitempty"`
}

type CompletionRequest struct {
	Model            string                `json:"model"`
	Messages         []Message             `json:"messages"`
	Tools            []Tool                `json:"tools,omitzero"`
	Stream           bool                  `json:"stream,omitempty"`
	Temperature      *float32              `json:"temperature,omitzero"`
	Reasoning        *ModelReasoningParams `json:"reasoning,omitzero"`
	MaxTokens        *int                  `json:"max_tokens,omitzero"`
	TopP             *float32              `json:"top_p,omitzero"`
	FrequencyPenalty *float32              `json:"frequency_penalty,omitzero"`
	PresencePenalty  *float32              `json:"presence_penalty,omitzero"`
	Plugins          []Plugin              `json:"plugins,omitzero"`
	Provider         struct {
		Sort              string `json:"sort,omitzero"` // price, latency, throughput
		RequireParameters bool   `json:"require_parameters,omitzero"`
	} `json:"provider,omitzero"`
	Usage struct {
		Include bool `json:"include"`
	} `json:"usage,omitzero"`

	WebSearch bool       `json:"-"`
	ModelInfo *ModelInfo `json:"-"`
}

type UsageDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	CachedTokens    int `json:"cached_tokens,omitempty"`
}

type ModelUsage struct {
	CompletionTokens       int64        `json:"completion_tokens"`
	CompletionTokensDetail UsageDetails `json:"completion_tokens_details"`
	Cost                   float64      `json:"cost"`
	PromptTokens           int64        `json:"prompt_tokens"`
	PromptTokensDetail     UsageDetails `json:"prompt_tokens_details"`
	TotalTokens            int64        `json:"total_tokens"`
}

func (u *ModelUsage) GetCostInDollars() float64 {
	return float64(u.Cost) / 1000
}

type MessageResponse struct {
	Content          string     `json:"content"`
	Reasoning        string     `json:"reasoning,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

type CompletionResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message MessageResponse `json:"message"`
	} `json:"choices"`
	Usage       ModelUsage          `json:"usage,omitzero"`
	Annotations []AnnotationContent `json:"annotations,omitzero"`
	Error       *ProviderError      `json:"error,omitzero"`
}

type StreamResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta struct {
			Content          string              `json:"content"`
			Reasoning        string              `json:"reasoning,omitempty"`
			ReasoningContent string              `json:"reasoning_content,omitempty"`
			Annotations      []AnnotationContent `json:"annotations,omitempty"`
			ToolCalls        []ToolCall          `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage ModelUsage `json:"usage,omitzero"`
}

type ModelPricing struct {
	Completion string `json:"completion"`
	Prompt     string `json:"prompt"`
	Image      string `json:"image"`
	WebSearch  string `json:"web_search"`
}

func (p *ModelPricing) GetCompletionPrice() (float64, error) {
	return strconv.ParseFloat(p.Completion, 64)
}

func (p *ModelPricing) GetPromptPrice() (float64, error) {
	return strconv.ParseFloat(p.Prompt, 64)
}

type ModelArchitecture struct {
	Modality         string   `json:"modality"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	Tokenizer        string   `json:"tokenizer"`
	InstructType     *string  `json:"instruct_type"`
}

type ModelInfo struct {
	ID                  string             `json:"id"`
	Provider            string             `json:"provider,omitzero"`
	Alias               string             `json:"alias,omitzero"`
	Architecture        *ModelArchitecture `json:"architecture,omitzero"`
	Pricing             *ModelPricing      `json:"pricing,omitzero"`
	SupportedParameters []string           `json:"supported_parameters,omitzero"`
	Created             *int64             `json:"created,omitzero"`
}

func (m *ModelInfo) SupportsTools() bool {
	return slices.Contains(m.SupportedParameters, "tools")
}

func (m *ModelInfo) FullName() string {
	return fmt.Sprintf("%s:%s", m.Provider, m.ID)
}

func (m *ModelInfo) SupportsInputModality(modality string) bool {
	if m.Architecture == nil {
		return false
	}
	return slices.Contains(m.Architecture.InputModalities, modality)
}

func (m *ModelInfo) SupportsOutputModality(modality string) bool {
	if m.Architecture == nil {
		return false
	}
	return slices.Contains(m.Architecture.OutputModalities, modality)
}

func (m *ModelInfo) SupportsImageRecognition() bool {
	return m.SupportsInputModality("image")
}

func (m *ModelInfo) SupportsImageGeneration() bool {
	return m.SupportsOutputModality("image")
}

func (m *ModelInfo) SupportsFiles() bool {
	return m.SupportsInputModality("file")
}

func (m *ModelInfo) SupportsAudioRecognition() bool {
	return m.SupportsInputModality("audio")
}

func (m *ModelInfo) SupportsText() bool {
	return m.SupportsInputModality("text") && m.SupportsOutputModality("text")
}

func (m *ModelInfo) IsMultimodal() bool {
	return m.SupportsImageRecognition() || m.SupportsFiles() || m.SupportsAudioRecognition()
}

func (m *ModelInfo) GetFormattedTime() time.Time {
	if m.Created != nil {
		return time.Unix(*m.Created, 0)
	} else {
		return time.Time{}
	}
}

func (m *ModelInfo) IsFree() bool {
	if m.Pricing == nil {
		return false
	}
	return m.Pricing.Completion == "0" && m.Pricing.Prompt == "0" && m.Pricing.Image == "0" && m.Pricing.WebSearch == "0"
}

func (m *ModelInfo) GetFormattedInputModalities() (result string) {
	if m.Architecture == nil {
		return
	}
	for _, modality := range m.Architecture.InputModalities {
		if modality == "text" {
			result += TextModality
		}
		if modality == "image" {
			result += ImageRecognitionModality
		}
		if modality == "file" {
			result += FileModality
		}
		if modality == "audio" {
			result += AudioModality
		}
	}
	return
}

func (m *ModelInfo) GetFormattedOutputModalities() (result string) {
	if m.Architecture == nil {
		return
	}
	for _, modality := range m.Architecture.OutputModalities {
		if modality == "text" {
			result += TextModality
		}
		if modality == "image" {
			result += ImageGenerationModality
		}
		if modality == "file" {
			result += FileModality
		}
		if modality == "audio" {
			result += AudioModality
		}
	}
	return
}

func (m *ModelInfo) GetFormattedModalities() string {
	inputModalities := m.GetFormattedInputModalities()
	outputModalities := m.GetFormattedOutputModalities()
	modalitiesStr := "❓"
	if inputModalities != "" && outputModalities != "" {
		modalitiesStr = fmt.Sprintf("%s \\> %s", inputModalities, outputModalities)
	}
	freeStr := ""
	if m.IsFree() {
		freeStr = Free
	}
	toolsStr := ""
	if m.SupportsTools() {
		toolsStr = Tools
	}
	return freeStr + modalitiesStr + toolsStr
}

type ProviderError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
	Type    string `json:"type"`
}

type Chunk struct {
	Content     string
	Reasoning   string
	Usage       *ModelUsage
	Tools       []ToolCall
	Annotations []AnnotationContent
	Error       *AIError
}

// AIError represents an enriched error from an AI provider
type AIError struct {
	// OriginalErr is the original error (if any)
	OriginalErr error `json:"-"`
	// ProviderName is the provider name (e.g. "openrouter", "openai-compatible")
	ProviderName string `json:"provider_name"`
	// ModelName is the model name where the error occurred
	ModelName string `json:"model_name"`
	// HTTPStatusCode is the HTTP response status code (if applicable)
	HTTPStatusCode int `json:"http_status_code"`
	// ErrorCode is the provider's error code (e.g. "insufficient_quota", "model_not_found")
	ErrorCode string `json:"error_code"`
	// Message is a human-readable error message
	Message string `json:"message"`
}

// Error implements the error interface
func (e *AIError) Error() string {
	msg := e.Message
	if msg == "" && e.OriginalErr != nil {
		msg = e.OriginalErr.Error()
	}
	if e.ProviderName != "" && e.ModelName != "" {
		msg = fmt.Sprintf("[%s:%s] %s", e.ProviderName, e.ModelName, msg)
	}
	if e.ErrorCode != "" {
		msg = fmt.Sprintf("%s (code: %s)", msg, e.ErrorCode)
	}
	if e.HTTPStatusCode != 0 {
		msg = fmt.Sprintf("%d %s", e.HTTPStatusCode, msg)
	}
	return msg
}

// Unwrap for compatibility with errors.Is and errors.As
func (e *AIError) Unwrap() error {
	return e.OriginalErr
}

// ErrorType returns the error type based on HTTP status code and error code
func (e *AIError) ErrorType() ErrorType {
	switch {
	case e.HTTPStatusCode == 429:
		return ErrorTypeRateLimit
	case e.HTTPStatusCode >= 500:
		return ErrorTypeServer
	case e.HTTPStatusCode == 400 && strings.Contains(strings.ToLower(e.Message), "policy"):
		return ErrorTypeContentPolicy
	case e.HTTPStatusCode >= 400 && e.HTTPStatusCode < 500:
		return ErrorTypeClient
	default:
		return ErrorTypeUnknown
	}
}

// IsRetryable determines if a request can be safely retried
func (e *AIError) IsRetryable() bool {
	switch e.ErrorType() {
	case ErrorTypeNetwork, ErrorTypeRateLimit, ErrorTypeServer:
		return true
	case ErrorTypeClient, ErrorTypeContentPolicy, ErrorTypeUnknown:
		return false
	default:
		return false
	}
}

// ErrorType for errors classification
type ErrorType string

const (
	ErrorTypeNetwork       ErrorType = "network"        // Network error, timeout
	ErrorTypeRateLimit     ErrorType = "rate_limit"     // 429, provider limits
	ErrorTypeServer        ErrorType = "server"         // 5xx, provider-side error
	ErrorTypeClient        ErrorType = "client"         // 4xx (except 429), invalid request, API key, model not found
	ErrorTypeContentPolicy ErrorType = "content_policy" // 400/403, content policy violation
	ErrorTypeUnknown       ErrorType = "unknown"        // Unknown error
)

// Helper functions for error analysis

func IsRetryableError(err error) bool {
	var aiErr *AIError
	if errors.As(err, &aiErr) {
		return aiErr.IsRetryable()
	}
	return false
}

func GetErrorType(err error) ErrorType {
	var aiErr *AIError
	if errors.As(err, &aiErr) {
		return aiErr.ErrorType()
	}
	return ErrorTypeUnknown
}

// IsErrorType checks if an error is of a specific type
func IsErrorType(err error, errorType ErrorType) bool {
	return GetErrorType(err) == errorType
}
