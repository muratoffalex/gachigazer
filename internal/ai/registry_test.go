package ai

import (
	"context"
	"os"
	"testing"

	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestResolvedAliasesDoNotModifyCachedModel(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("config.toml", []byte(`
[telegram]
token = "test-token"
[[ai.aliases]]
alias = "think"
model = "or:shared"
model_params = {reasoning = {effort = "max"}}
[[ai.aliases]]
alias = "v4"
model = "or:shared"
model_params = {reasoning = {effort = "none"}}
`), 0600))
	cfg, err := config.Load()
	require.NoError(t, err)
	cachedModel := &ModelInfo{ID: "shared", Provider: "or"}
	provider := &OpenAICompatibleClient{
		cfg:         cfg,
		modelsCache: map[string]*ModelInfo{"shared": cachedModel},
	}
	registry := NewProviderRegistry(cfg, logger.NewTestLogger())
	registry.RegisterProvider("or", provider)
	think, err := registry.GetFormattedModel(context.Background(), "think", "")
	require.NoError(t, err)
	v4, err := registry.GetFormattedModel(context.Background(), "v4", "")
	require.NoError(t, err)
	require.Equal(t, "think", think.Alias)
	require.Equal(t, "v4", v4.Alias)
	require.Empty(t, cachedModel.Alias)
	params, err := cfg.AI().GetFullModelParams("or", think.Alias, "")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"effort": "max"}, params["reasoning"])
}
