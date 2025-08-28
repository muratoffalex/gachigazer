package service

import (
	"context"
	"fmt"

	"github.com/muratoffalex/gachigazer/internal/ai"
	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/database"
)

type ChatService struct {
	db         database.Database
	aiRegistry *ai.ProviderRegistry
	cfg        *config.Config
}

func NewChatService(db database.Database, registry *ai.ProviderRegistry, cfg *config.Config) *ChatService {
	return &ChatService{
		db:         db,
		aiRegistry: registry,
		cfg:        cfg,
	}
}

func (s *ChatService) GetCurrentModelForChat(ctx context.Context, chatID int64, userID int64, name string) (*ai.ModelInfo, error) {
	model := &ai.ModelInfo{ID: name}
	if name != "" {
		return s.resolveModelByName(ctx, userID, name)
	}

	modelSpec, err := s.db.GetChatModel(chatID)
	if err != nil {
		return model, fmt.Errorf("failed to get chat model: %w", err)
	}

	if modelSpec == "" {
		model, err = s.aiRegistry.GetFormattedModel(ctx, s.cfg.AI().DefaultModel, "")
		if err != nil {
			return model, fmt.Errorf("failed to get default model: %w", err)
		}
	} else {
		model, err = s.aiRegistry.GetFormattedModel(ctx, modelSpec, "")
		if err != nil {
			return model, fmt.Errorf("failed to get model: %w", err)
		}
	}

	return model, nil
}

func (s *ChatService) SetChatModel(ctx context.Context, chatID int64, modelSpec string) error {
	model, err := s.aiRegistry.GetFormattedModel(ctx, modelSpec, "")
	if err != nil {
		return fmt.Errorf("invalid model: %w", err)
	}

	return s.db.SaveChatModel(chatID, model.FullName())
}

func (s *ChatService) ResetChatModel(ctx context.Context, chatID int64) error {
	return s.db.DeleteChatModel(chatID)
}

func (s *ChatService) GetCurrentModelSpec(ctx context.Context, chatID int64) (string, error) {
	return s.db.GetChatModel(chatID)
}

func (s *ChatService) MergeModelParams(chatID int64, provider, alias, prompt string, requestParams ai.ModelParams) (ai.ModelParams, error) {
	configParams, err := s.cfg.AI().GetFullModelParams(provider, alias, prompt)
	if err != nil {
		return ai.ModelParams{}, err
	}

	baseParams, err := mapToModelParams(configParams)
	if err != nil {
		return ai.ModelParams{}, err
	}

	return mergeParams(baseParams, requestParams), nil
}

func mapToModelParams(params map[string]any) (ai.ModelParams, error) {
	var result ai.ModelParams
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
		}
	}
	return result, nil
}

func mergeParams(base, override ai.ModelParams) ai.ModelParams {
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
	return base
}

func (s *ChatService) resolveModelByName(ctx context.Context, userID int64, name string) (*ai.ModelInfo, error) {
	model, err := s.aiRegistry.GetFormattedModel(ctx, name, "")
	if err != nil {
		return model, err
	}

	if !model.IsFree() && !s.cfg.Telegram().IsUserAllowed(userID) {
		return model, fmt.Errorf("not allowed")
	}

	return model, nil
}
