package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

const modelsCacheDuration = 30 * time.Minute

type OpenAICompatibleClient struct {
	name           string
	chatURL        string
	logger         logger.Logger
	defaultModel   string
	cfg            *config.Config
	lastModelSync  time.Time
	modelsCache    map[string]*ModelInfo
	modelsMutex    sync.RWMutex
	lastSync       time.Time
	httpClient     *baseHTTPClient
	overrideModels bool
}

func NewOpenAICompatibleClient(
	name string,
	baseURL string,
	chatURL string,
	apiKey string,
	defaultModel string,
	log logger.Logger,
	overrideModels bool,
	cfg *config.Config,
	httpClient *http.Client,
) *OpenAICompatibleClient {
	if chatURL == "" {
		chatURL = "/chat/completions"
	}
	baseHTTPClient := NewBaseHTTPClient(httpClient, baseURL, apiKey, log)

	return &OpenAICompatibleClient{
		name:           name,
		chatURL:        strings.TrimPrefix(chatURL, "/"),
		httpClient:     baseHTTPClient,
		defaultModel:   defaultModel,
		logger:         log,
		cfg:            cfg,
		overrideModels: overrideModels,
	}
}

func (c *OpenAICompatibleClient) Name() string {
	return c.name
}

func (c *OpenAICompatibleClient) makeRawRequest(ctx context.Context, method string, endpoint string, body any, headers map[string]string) (*http.Response, error) {
	requestBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("create request error: %w", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return c.httpClient.Do(req)
}

func (c *OpenAICompatibleClient) Ask(
	ctx context.Context,
	request CompletionRequest,
	headers map[string]string,
) (
	string,
	string,
	*CompletionResponse,
	*ModelInfo,
	error,
) {
	_, body, aiErr := c.doRequest(ctx, "POST", "/chat/completions", request, headers, false)
	if aiErr != nil {
		aiErr.ModelName = request.Model
		return "", "", nil, nil, aiErr
	}

	var result CompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", nil, nil, &AIError{
			OriginalErr:  err,
			ProviderName: c.Name(),
			ModelName:    request.Model,
			Message:      "failed to unmarshal response",
		}
	}

	// ERROR HANDLING FOR OPENROUTER and similar cases that might be in 200 OK!
	if result.Error != nil {
		return "", "", nil, nil, &AIError{
			ProviderName: c.Name(),
			ModelName:    request.Model,
			ErrorCode:    result.Error.Code,
			Message:      result.Error.Message,
		}
	}

	if len(result.Choices) == 0 {
		return "", "", nil, nil, &AIError{
			ProviderName: c.Name(),
			ModelName:    request.Model,
			Message:      "no choices in response",
		}
	}

	choice := result.Choices[0]
	message := choice.Message
	reasoning := message.Reasoning
	if reasoning == "" {
		reasoning = message.ReasoningContent
	}
	return message.Content, reasoning, &result, request.ModelInfo, nil
}

func (c *OpenAICompatibleClient) AskStream(
	ctx context.Context,
	request CompletionRequest,
	headers map[string]string,
) (<-chan Chunk, *ModelInfo, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Accept"] = "text/event-stream"
	resp, _, aiErr := c.doRequest(ctx, "POST", "chat/completions", request, headers, true)
	if aiErr != nil {
		aiErr.ModelName = request.Model
		return nil, nil, aiErr
	}

	chunkCh := make(chan Chunk)
	go func() {
		defer close(chunkCh)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		tempToolCalls := make(map[int]*ToolCall, 0)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					log.Printf("stream read error: %v", err)
				}
				return
			}

			c.logger.WithFields(logger.Fields{
				"raw_data":   string(line),
				"model":      request.Model,
				"web_search": request.WebSearch,
			}).Trace("Raw SSE event")

			if len(line) <= 6 || string(line[:6]) != "data: " {
				continue
			}

			jsonData := line[6 : len(line)-1]
			if string(jsonData) == "[DONE]" {
				return
			}

			var event StreamResponse
			if err := json.Unmarshal(jsonData, &event); err != nil {
				c.logger.WithFields(logger.Fields{
					"error": err,
					"data":  string(jsonData),
				}).Error("stream decode error")
				continue
			}

			if len(event.Choices) > 0 {
				delta := event.Choices[0].Delta

				if delta.ToolCalls != nil {
					for _, partialCall := range delta.ToolCalls {
						if _, exists := tempToolCalls[partialCall.Index]; !exists {
							tempToolCalls[partialCall.Index] = &ToolCall{
								ID:       partialCall.ID,
								Type:     partialCall.Type,
								Function: partialCall.Function,
							}
						} else {
							if partialCall.ID != "" {
								tempToolCalls[partialCall.Index].ID = partialCall.ID
							}
							if partialCall.Type != "" {
								tempToolCalls[partialCall.Index].Type = partialCall.Type
							}
							if partialCall.Function.Name != "" {
								tempToolCalls[partialCall.Index].Function.Name = partialCall.Function.Name
							}
							if partialCall.Function.Arguments != "" {
								tempToolCalls[partialCall.Index].Function.Arguments += partialCall.Function.Arguments
							}
						}
					}
				}

				chunk := Chunk{
					Content:     delta.Content,
					Usage:       &event.Usage,
					Annotations: delta.Annotations,
				}

				reasoning := delta.Reasoning
				if reasoning == "" {
					reasoning = delta.ReasoningContent
				}
				switch event.Choices[0].FinishReason {
				case "tool_calls":
					values := make([]*ToolCall, 0, len(tempToolCalls))
					for _, value := range tempToolCalls {
						values = append(values, value)
					}
					chunk.Tools = toolPtrsToValues(values)
					tempToolCalls = nil
				case "error":
					chunk.Error = &AIError{
						ProviderName: c.Name(),
						ModelName:    request.Model,
						Message:      "stream generation failed: " + event.Choices[0].FinishReason,
					}
				}

				chunk.Reasoning = reasoning
				chunkCh <- chunk
			}
		}
	}()

	return chunkCh, request.ModelInfo, nil
}

func (c *OpenAICompatibleClient) doRequest(
	ctx context.Context,
	method string,
	endpoint string,
	body any,
	headers map[string]string,
	stream bool,
) (*http.Response, []byte, *AIError) {
	resp, err := c.makeRawRequest(ctx, method, endpoint, body, headers)
	if err != nil {
		return nil, nil, &AIError{
			OriginalErr:  err,
			ProviderName: c.Name(),
			Message:      "network request failed",
		}
	}

	invalidStatusCode := resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices

	var responseBody []byte
	if !stream || invalidStatusCode {
		defer resp.Body.Close()
		responseBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, &AIError{
				OriginalErr:  err,
				ProviderName: c.Name(),
				Message:      "failed to read response body",
			}
		}
	}

	if invalidStatusCode {
		var providerError struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}

		aiError := &AIError{
			ProviderName:   c.Name(),
			HTTPStatusCode: resp.StatusCode,
			Message:        fmt.Sprintf("HTTP request failed with status code: %d", resp.StatusCode),
		}

		if len(responseBody) > 0 {
			json.Unmarshal(responseBody, &providerError)
			if providerError.Error.Message != "" {
				aiError.Message = providerError.Error.Message
				aiError.ErrorCode = providerError.Error.Code
			}
		}

		return nil, responseBody, aiError
	}

	return resp, responseBody, nil
}

func (c *OpenAICompatibleClient) getModelsFromAPI(ctx context.Context) (map[string]*ModelInfo, error) {
	_, body, err := c.doRequest(ctx, "GET", "models", nil, nil, false)
	if err != nil {
		return nil, fmt.Errorf("models request error: %w", err)
	}

	var modelsResponse struct {
		Data []*ModelInfo `json:"data"`
	}

	if err := json.Unmarshal(body, &modelsResponse); err != nil {
		return nil, fmt.Errorf("decode models error: %w", err)
	}

	models := map[string]*ModelInfo{}
	for _, model := range modelsResponse.Data {
		model.Provider = c.Name()
		models[model.ID] = model
	}

	return models, nil
}

func (c *OpenAICompatibleClient) CreateRequest(
	stream bool,
	messages []Message,
	tools []Tool,
	model *ModelInfo,
	params ModelParams,
	webSearch bool,
) CompletionRequest {
	reqBody := CompletionRequest{
		Model:            model.ID,
		Messages:         messages,
		Stream:           stream,
		Temperature:      params.Temperature,
		MaxTokens:        params.MaxTokens,
		Reasoning:        params.Reasoning,
		TopP:             params.TopP,
		FrequencyPenalty: params.FrequencyPenalty,
		PresencePenalty:  params.PresencePenalty,
		Usage: struct {
			Include bool "json:\"include\""
		}{Include: true},
		ModelInfo: model,
		WebSearch: webSearch,
	}

	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	plugins := []Plugin{}

	if webSearch {
		plugins = append(plugins, Plugin{
			ID:         "web",
			MaxResults: 2,
		})
	}

	hasFiles := false
	for _, message := range messages {
		if message.HasFiles() {
			hasFiles = true
		}
	}

	if hasFiles {
		engine := "pdf-text"
		if model.SupportsFiles() {
			engine = "native"
		}
		plugins = append(plugins, Plugin{
			ID: "file-parser",
			PDF: struct {
				Engine string "json:\"engine,omitempty\""
			}{
				Engine: engine,
			},
		})
	}

	if len(plugins) > 0 {
		reqBody.Plugins = plugins
	}

	return reqBody
}

func (c *OpenAICompatibleClient) GetModels(ctx context.Context, onlyFree, fresh bool) (map[string]*ModelInfo, error) {
	if c.overrideModels {
		return c.getModelsFromConfig(), nil
	}
	c.modelsMutex.RLock()
	if time.Since(c.lastSync) < modelsCacheDuration && len(c.modelsCache) > 0 && !fresh {
		c.modelsMutex.RUnlock()
		return c.getCacheModels(), nil
	}
	c.modelsMutex.RUnlock()

	models, err := c.getModelsFromAPI(ctx)
	if err != nil {
		return nil, err
	}

	maps.Copy(models, c.getModelsFromConfig())

	c.modelsMutex.Lock()
	defer c.modelsMutex.Unlock()
	c.modelsCache = make(map[string]*ModelInfo)
	for _, model := range models {
		c.modelsCache[model.ID] = model
	}
	c.lastSync = time.Now()

	return models, nil
}

func (c *OpenAICompatibleClient) GetModelInfo(name string) (*ModelInfo, error) {
	// from config
	if model, exists := c.getModelsFromConfig()[name]; exists {
		return model, nil
	}

	// from cache
	if model, exists := c.getModelFromCache(name); exists {
		return model, nil
	}
	models, err := c.GetModels(context.TODO(), false, true)
	if err != nil {
		return nil, err
	}
	if model, exists := models[name]; exists {
		return model, nil
	}

	return &ModelInfo{
		ID:       name,
		Provider: c.Name(),
	}, ErrModelNotFound
}

func (c *OpenAICompatibleClient) getModelsFromConfig() map[string]*ModelInfo {
	models := map[string]*ModelInfo{}
	if cfg := c.cfg.AI().GetProvider(c.name); cfg != nil {
		for _, modelCfg := range cfg.Models {
			model := &ModelInfo{
				ID:       modelCfg.Model,
				Provider: cfg.Name,
				Architecture: &ModelArchitecture{
					InputModalities:  modelCfg.InputModalities,
					OutputModalities: modelCfg.OutputModalities,
				},
				SupportedParameters: modelCfg.SupportedParameters,
			}
			if modelCfg.IsFree {
				model.Pricing = &ModelPricing{
					Completion: "0",
					Prompt:     "0",
					Image:      "0",
					WebSearch:  "0",
				}
			}
			models[model.ID] = model
		}
	}
	return models
}

func (c *OpenAICompatibleClient) getCacheModels() map[string]*ModelInfo {
	return c.modelsCache
}

func (c *OpenAICompatibleClient) getModelFromCache(name string) (*ModelInfo, bool) {
	c.modelsMutex.Lock()
	defer c.modelsMutex.Unlock()
	if model, exists := c.modelsCache[name]; exists {
		return model, true
	}
	return nil, false
}

func (c *OpenAICompatibleClient) GetDefaultModel() string {
	return c.defaultModel
}

func toolPtrsToValues(ptrs []*ToolCall) []ToolCall {
	result := make([]ToolCall, len(ptrs))
	for i, ptr := range ptrs {
		if ptr != nil {
			result[i] = *ptr
		}
	}
	return result
}
