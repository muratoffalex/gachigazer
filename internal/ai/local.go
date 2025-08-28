package ai

import (
	"context"
	"net/http"

	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

type LocalAIClient struct {
	*OpenAICompatibleClient
}

func NewLocalAIClient(cfg config.AIProviderConfig, globalCfg *config.Config, log logger.Logger, httpClient *http.Client) *LocalAIClient {
	baseClient := NewOpenAICompatibleClient(
		cfg.Name,
		cfg.BaseURL,
		cfg.ChatURL,
		"",
		cfg.DefaultModel,
		log,
		cfg.OverrideModels,
		globalCfg,
		httpClient,
	)

	return &LocalAIClient{
		OpenAICompatibleClient: baseClient,
	}
}

func (c *LocalAIClient) GetModels(ctx context.Context, onlyFree, fresh bool) (map[string]*ModelInfo, error) {
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
	return models, nil
}
