package ai

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

var (
	ErrInvalidModelFormat = errors.New("invalid model format, expected provider:model")
	ErrProviderNotFound   = errors.New("provider not found")
	ErrModelNotFound      = errors.New("model not found")
)

type ProviderRegistry struct {
	providers      map[string]Provider
	providersMutex sync.RWMutex
	logger         logger.Logger
	chatService    ChatService
	cfg            *config.Config
}

func NewProviderRegistry(cfg *config.Config, log logger.Logger) *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
		logger:    log,
		cfg:       cfg,
	}
}

func (r *ProviderRegistry) SetChatService(service ChatService) {
	r.chatService = service
}

func (r *ProviderRegistry) RegisterProvider(name string, provider Provider) {
	r.providersMutex.Lock()
	defer r.providersMutex.Unlock()
	r.providers[name] = provider
}

func (r *ProviderRegistry) GetProvider(name string) (Provider, error) {
	r.providersMutex.RLock()
	defer r.providersMutex.RUnlock()

	if provider, ok := r.providers[name]; ok {
		return provider, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
}

func (r *ProviderRegistry) Providers() []string {
	r.providersMutex.RLock()
	defer r.providersMutex.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// ResolveModel determines the provider and model by priority:
// 1. Explicitly specified model (modelSpec)
// 2. Model from chat settings
// 3. Provider's default model
// 4. Default provider and its model
func (r *ProviderRegistry) ResolveModel(ctx context.Context, modelSpec string, chatID int64) (Provider, string, error) {
	if modelSpec != "" {
		providerName, modelName, err := ParseModelSpec(modelSpec)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %s", ErrInvalidModelFormat, modelSpec)
		}

		provider, err := r.GetProvider(providerName)
		if err != nil {
			return nil, "", err
		}
		return provider, modelName, nil
	}

	// get a chat settings model
	if chatID != 0 {
		modelSpec, err := r.chatService.GetCurrentModelSpec(ctx, chatID)
		if err == nil && modelSpec != "" {
			providerName, modelName, err := ParseModelSpec(modelSpec)
			if err == nil {
				if provider, err := r.GetProvider(providerName); err == nil {
					return provider, modelName, nil
				}
			}
		}
	}

	// use a default
	defaultModelSpec := r.cfg.AI().DefaultModel
	providerName, modelName, err := ParseModelSpec(defaultModelSpec)
	if err != nil {
		return nil, "", err
	}
	if provider, err := r.GetProvider(providerName); err == nil {
		return provider, modelName, nil
	}
	return nil, "", err
}

// Ask performs a request with automatic provider and model resolution
func (r *ProviderRegistry) Ask(ctx context.Context, messages []Message, tools []Tool, model *ModelInfo, promptName string, chatID int64, webSearch bool, requestParams ModelParams) (string, string, *CompletionResponse, *ModelInfo, *ModelParams, error) {
	provider, _, err := r.ResolveModel(ctx, model.FullName(), chatID)
	if err != nil {
		return "", "", nil, nil, nil, err
	}

	mergedParams, err := r.chatService.MergeModelParams(chatID, model.Provider, model.Alias, promptName, requestParams)
	if err != nil {
		return "", "", nil, nil, nil, fmt.Errorf("failed to merge model params: %w", err)
	}
	request := provider.CreateRequest(false, messages, tools, model, mergedParams, webSearch)
	content, reasoning, response, modelInfo, err := provider.Ask(ctx, request, nil)

	return content, reasoning, response, modelInfo, &mergedParams, err
}

// AskStream performs a streaming request with automatic provider and model resolution
func (r *ProviderRegistry) AskStream(ctx context.Context, messages []Message, tools []Tool, model *ModelInfo, promptName string, chatID int64, webSearch bool, params ModelParams) (<-chan Chunk, *ModelInfo, *ModelParams, error) {
	provider, _, err := r.ResolveModel(ctx, model.FullName(), chatID)
	if err != nil {
		return nil, nil, nil, err
	}

	mergedParams, err := r.chatService.MergeModelParams(chatID, model.Provider, model.Alias, promptName, params)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to merge model params: %w", err)
	}
	request := provider.CreateRequest(true, messages, tools, model, mergedParams, webSearch)
	chunk, modelInfo, err := provider.AskStream(ctx, request, nil)

	return chunk, modelInfo, &mergedParams, err
}

// GetFormattedModel validate model and return correct value
// if the value doesn't match provider:model format,
// it sets the default provider and tries to get a model with that name,
// this is done to allow getting default provider's model without
// specifying the provider
func (r *ProviderRegistry) GetFormattedModel(ctx context.Context, modelName, providerName string) (*ModelInfo, error) {
	r.logger.WithFields(logger.Fields{
		"model":    modelName,
		"provider": providerName,
	}).Debug("Get model info")
	var aliasName string
	if alias, exists := r.cfg.AI().GetAlias(modelName); exists {
		aliasName = modelName
		modelName = alias.Model
	}
	var model *ModelInfo
	var err error
	if providerName == "" {
		providerName, modelName, _ = ParseModelSpec(modelName)
	}
	if providerName != "" {
		provider, providerErr := r.GetProvider(providerName)
		if providerErr == nil {
			model, err = provider.GetModelInfo(modelName)
			if err != nil {
				return model, err
			}
		}
	}
	if model == nil {
		for _, provider := range r.providers {
			model, err = provider.GetModelInfo(modelName)
			if err != nil {
				continue
			}
			err = nil
			break
		}
	}
	model.Alias = aliasName
	return model, err
}

func (r *ProviderRegistry) GetAllModels(ctx context.Context, free, fresh bool) (map[string][]*ModelInfo, error) {
	models := map[string][]*ModelInfo{}
	for _, providerName := range r.Providers() {
		provider, err := r.GetProvider(providerName)
		if err != nil {
			r.logger.WithError(err).WithFields(logger.Fields{
				"provider": providerName,
			}).Error("get provider by name error")
			continue
		}
		providerModels, err := provider.GetModels(ctx, free, fresh)

		sliceModels := make([]*ModelInfo, 0, len(providerModels))
		for _, model := range providerModels {
			sliceModels = append(sliceModels, model)
		}
		if err != nil {
			r.logger.WithError(err).WithFields(logger.Fields{
				"provider": providerName,
			}).Error("get models error")
			continue
		}
		if len(sliceModels) > 0 {
			models[providerName] = sliceModels
		}
	}
	return models, nil
}
