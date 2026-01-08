package ai

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

var ErrorModelNotFound = errors.New("model not found")

type OpenRouterClient struct {
	*OpenAICompatibleClient
	freeModels     []string
	rng            *rand.Rand
	onlyFreeModels bool
}

func NewOpenRouterClient(cfg config.AIProviderConfig, globalCfg *config.Config, log logger.Logger, httpClient *http.Client) *OpenRouterClient {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	baseClient := NewOpenAICompatibleClient(
		cfg.Name,
		baseURL,
		cfg.ChatURL,
		cfg.GetAPIKey(),
		cfg.DefaultModel,
		log,
		cfg.OverrideModels,
		globalCfg,
		httpClient,
	)

	return &OpenRouterClient{
		OpenAICompatibleClient: baseClient,
		rng:                    rand.New(rand.NewSource(time.Now().UnixNano())),
		onlyFreeModels:         cfg.OnlyFreeModels,
	}
}

func (c *OpenRouterClient) filterModels(onlyFree bool) map[string]*ModelInfo {
	c.modelsMutex.RLock()
	defer c.modelsMutex.RUnlock()

	filtered := map[string]*ModelInfo{}
	for _, model := range c.getCacheModels() {
		if !onlyFree || model.IsFree() {
			filtered[model.ID] = model
		}
	}
	return filtered
}

func (c *OpenRouterClient) GetModels(ctx context.Context, onlyFree, fresh bool) (map[string]*ModelInfo, error) {
	// Check if cache is still fresh (30 minutes)
	if !fresh {
		c.modelsMutex.RLock()
		if time.Since(c.lastSync) < modelsCacheDuration && len(c.modelsCache) > 0 && !fresh {
			c.modelsMutex.RUnlock()
			return c.filterModels(onlyFree), nil
		}
		c.modelsMutex.RUnlock()
	}
	models, err := c.getModelsFromAPI(ctx)
	if err != nil {
		return nil, err
	}

	if onlyFree || c.onlyFreeModels {
		models = c.filterModels(true)
	}

	// save only when
	if c.onlyFreeModels || !onlyFree {
		c.modelsMutex.Lock()
		defer c.modelsMutex.Unlock()
		c.modelsCache = models
		c.lastSync = time.Now()
	}

	return models, nil
}

func (c *OpenRouterClient) GetRandomFreeModel(ctx context.Context) (string, error) {
	models, err := c.GetModels(ctx, true, false)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", errors.New("no free models available")
	}
	randomIndex := c.rng.Intn(len(models))

	i := 0
	for modelID := range models {
		if i == randomIndex {
			return modelID, nil
		}
		i++
	}

	return "", errors.New("failed to select random model")
}

func (c *OpenRouterClient) Ask(ctx context.Context, request CompletionRequest, headers map[string]string) (string, string, *CompletionResponse, *ModelInfo, error) {
	var err error
	switch request.Model {
	case "random-free":
		request.Model, err = c.GetRandomFreeModel(ctx)
		if err != nil {
			return "", "", nil, nil, fmt.Errorf("failed to get random free model: %w", err)
		}
	case "":
		request.Model = c.defaultModel
	}

	headers = c.setHeaders(headers)

	return c.OpenAICompatibleClient.Ask(ctx, request, headers)
}

func (c *OpenRouterClient) AskStream(ctx context.Context, request CompletionRequest, headers map[string]string) (<-chan Chunk, *ModelInfo, error) {
	var err error
	switch request.Model {
	case "random-free":
		request.Model, err = c.GetRandomFreeModel(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get random free model: %w", err)
		}
	case "":
		request.Model = c.defaultModel
	}

	headers = c.setHeaders(headers)

	request.Provider = struct {
		Sort              string `json:"sort,omitzero"` // price, latency, throughput
		RequireParameters bool   `json:"require_parameters,omitzero"`
	}{
		Sort: "price",
	}
	if request.ModelInfo.IsFree() {
		request.Provider.Sort = "throughput"
	}

	return c.OpenAICompatibleClient.AskStream(ctx, request, headers)
}

func (c *OpenRouterClient) GetModelInfo(name string) (*ModelInfo, error) {
	if name == "random-free" {
		model, err := c.GetRandomFreeModel(context.Background())
		if err != nil {
			return &ModelInfo{ID: name, Provider: c.Name()}, err
		}
		name = model
	}
	return c.OpenAICompatibleClient.GetModelInfo(name)
}

func (c *OpenRouterClient) setHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["X-Title"] = "Gachigazer"
	// headers["HTTP-Referrer"] = ""

	return headers
}
