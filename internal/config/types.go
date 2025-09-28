package config

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"
)

type globalConfig struct {
	MessageRetentionDays int    `koanf:"message_retention_days"`
	InterfaceLanguage    string `koanf:"interface_language"`
	FixInstagramPreviews bool   `koanf:"fix_instagram_previews"`
	FixXPreviews         bool   `koanf:"fix_x_previews"`
}

type CurrencyConfig struct {
	Code      string `koanf:"code"`
	Symbol    string `koanf:"symbol"`
	Precision int    `koanf:"precision"`
}

type HTTPConfig struct {
	proxy *string `koanf:"proxy"`
}

func (c HTTPConfig) GetProxy() string {
	if c.proxy != nil && *c.proxy != "" {
		return *c.proxy
	}
	if proxyURL := os.Getenv("HTTPS_PROXY"); proxyURL != "" {
		return proxyURL
	}
	if proxyURL := os.Getenv("https_proxy"); proxyURL != "" {
		return proxyURL
	}
	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		return proxyURL
	}
	if proxyURL := os.Getenv("http_proxy"); proxyURL != "" {
		return proxyURL
	}
	return ""
}

type LoggingConfig struct {
	LogLevel    string `koanf:"level"`
	WriteInFile bool   `koanf:"write_in_file"`
	FilePath    string `koanf:"file_path"`
}

func (c LoggingConfig) Level() string {
	return strings.ToLower(c.LogLevel)
}

func (c LoggingConfig) IsDebug() bool {
	return c.Level() == "debug" || c.Level() == "trace"
}

type TelegramConfig struct {
	Token                 string  `koanf:"token"`
	AllowedUsers          []int64 `koanf:"allowed_users"`
	AllowedChats          []int64 `koanf:"allowed_chats"`
	TelegramifyScriptPath string  `koanf:"telegramify_script_path"`

	// Telegram data API
	// https://my.telegram.org/
	TdEnabled   bool   `koanf:"td_enabled"`
	ApiID       int    `koanf:"api_id"`
	ApiHash     string `koanf:"api_hash"`
	Phone       string `koanf:"phone"`
	Password    string `koanf:"password"`
	SessionPath string `koanf:"session_path"`
}

func (c TelegramConfig) IsAllowed(userID int64, chatID int64) bool {
	return c.IsUserAllowed(userID) || c.IsChatAllowed(chatID)
}

func (c TelegramConfig) IsUserAllowed(userID int64) bool {
	allowedUsers := c.AllowedUsers
	if len(allowedUsers) == 0 {
		return false
	}

	return slices.Contains(allowedUsers, userID)
}

func (c TelegramConfig) IsChatAllowed(chatID int64) bool {
	allowedChats := c.AllowedChats
	if len(allowedChats) == 0 {
		return true
	}

	return slices.Contains(allowedChats, chatID)
}

type instagramConfig struct {
	Username               string        `koanf:"username"`
	Password               string        `koanf:"password"`
	SessionPath            string        `koanf:"session_path"`
	SessionRefreshInterval time.Duration `koanf:"session_refresh_interval"`
}

func (c instagramConfig) Credentials() (string, string) {
	return c.Username, c.Password
}

type chromeConfig struct {
	Enabled bool     `koanf:"enabled"`
	Path    string   `koanf:"path"`
	Opts    []string `koanf:"opts"`
}

type aiPrompt struct {
	Enabled       bool          `koanf:"enabled"`
	Name          string        `koanf:"name"`
	Description   string        `koanf:"description"`
	Text          string        `koanf:"text"`
	Aliases       []string      `koanf:"aliases"`
	Commands      []string      `koanf:"commands"`
	ModelParams   aiModelParams `koanf:"model_params"`
	DynamicPrompt bool          `koanf:"dynamic_prompt"`
}

type aiModelParams struct {
	Temperature      *float32                `koanf:"temperature"`
	Stream           *bool                   `koanf:"stream"`
	Reasoning        *aiModelReasoningParams `koanf:"reasoning"`
	MaxTokens        *int                    `koanf:"max_tokens"`
	TopP             *float32                `koanf:"top_p"`
	FrequencyPenalty *float32                `koanf:"frequency_penalty"`
	PresencePenalty  *float32                `koanf:"presence_penalty"`
	StopSequences    []string                `koanf:"stop_sequences"`
	Timeout          *time.Duration          `koanf:"timeout"`
}

type aiModelReasoningParams struct {
	// https://openrouter.ai/docs/use-cases/reasoning-tokens
	// One of the following (MaxTokens has priority)
	MaxTokens *int    // Specific token limit (Anthropic-style)
	Effort    *string // Can be "high", "medium", or "low" (OpenAI-style)

	Enabled *bool `koanf:"enabled"` // Default: inferred from `effort` or `max_tokens`
	Exclude *bool `koanf:"exclude"` // Set to true to exclude reasoning tokens from response
}

func (p *aiModelReasoningParams) ToRequestParams() map[string]any {
	params := make(map[string]any)
	if p.Enabled != nil {
		params["enabled"] = *p.Enabled
	}
	if p.Exclude != nil {
		params["exclude"] = *p.Exclude
	}
	if p.MaxTokens != nil {
		params["max_tokens"] = *p.MaxTokens
	} else if p.Effort != nil {
		params["effort"] = *p.Effort
	}
	return params
}

func (p aiModelParams) ToRequestParams() map[string]any {
	params := make(map[string]any)
	if p.Stream != nil {
		params["stream"] = *p.Stream
	}
	if p.Temperature != nil {
		params["temperature"] = *p.Temperature
	}
	if p.MaxTokens != nil {
		params["max_tokens"] = *p.MaxTokens
	}
	if p.TopP != nil {
		params["top_p"] = *p.TopP
	}
	if p.FrequencyPenalty != nil {
		params["frequency_penalty"] = *p.FrequencyPenalty
	}
	if p.PresencePenalty != nil {
		params["presence_penalty"] = *p.PresencePenalty
	}
	if len(p.StopSequences) > 0 {
		params["stop"] = p.StopSequences
	}
	if p.Timeout != nil {
		params["timeout"] = *p.Timeout
	}
	if p.Reasoning != nil {
		params["reasoning"] = p.Reasoning.ToRequestParams()
	}
	return params
}

type ModelInfoConfig struct {
	Model               string   `koanf:"model"`
	InputModalities     []string `koanf:"input_modalities"`
	OutputModalities    []string `koanf:"output_modalities"`
	SupportedParameters []string `koanf:"supported_parameters"`
	IsFree              bool     `koanf:"is_free"`
}

type aiModelAlias struct {
	Alias       string        `koanf:"alias"`
	Model       string        `koanf:"model"`
	ModelParams aiModelParams `koanf:"model_params"`
}

type ModelsBehavior string

const (
	ModelsBehaviorReplace ModelsBehavior = "replace"
	ModelsBehaviorMerge   ModelsBehavior = "merge"
)

type AIProviderConfig struct {
	Type           string            `koanf:"type"`
	Name           string            `koanf:"name"`
	BaseURL        string            `koanf:"base_url"`
	APIKey         string            `koanf:"api_key"`
	EnvAPIKey      string            `koanf:"env_api_key"`
	DefaultModel   string            `koanf:"default_model"`
	ChatURL        string            `koanf:"chat_url"`
	OnlyFreeModels bool              `koanf:"only_free_models"`
	ModelParams    aiModelParams     `koanf:"model_params"`
	Models         []ModelInfoConfig `koanf:"models"`
	OverrideModels bool              `koanf:"override_models"`
}

func (c *AIProviderConfig) GetAPIKey() string {
	var apiKey string
	if key := c.APIKey; key != "" {
		apiKey = key
	} else {
		apiKey = os.Getenv(c.EnvAPIKey)
	}
	return apiKey
}

type aiConfig struct {
	SystemPrompt      string             `koanf:"system_prompt"`
	ExtraSystemPrompt string             `koanf:"extra_system_prompt"`
	Language          string             `koanf:"language"`
	UseStream         bool               `koanf:"use_stream"`
	ModelParams       aiModelParams      `koanf:"model_params"`
	DefaultModel      string             `koanf:"default_model"`
	UtilityModel      string             `koanf:"utility_model"`    // generating titles and summaries
	MultimodalModel   string             `koanf:"multimodal_model"` // use for handle context with images
	ToolsModel        string             `koanf:"tools_model"`      // use for handle tools
	UseMultimodalAuto bool               `koanf:"use_multimodal_auto"`
	ImageRouterAPIKey string             `koanf:"imagerouter_api_key"`
	ImageRouterModel  string             `koanf:"imagerouter_model"`
	Providers         []AIProviderConfig `koanf:"providers"`
	Prompts           []aiPrompt         `koanf:"prompts"`
	Aliases           []aiModelAlias     `koanf:"aliases"`
}

func (c aiConfig) GetPromptText() string {
	var sb strings.Builder
	for _, prompt := range c.Prompts {
		if !prompt.Enabled {
			continue
		}
		sb.WriteString(fmt.Sprintf("Prompt name: %s\n", prompt.Name))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", prompt.Description))
		}
		if len(prompt.Aliases) > 0 {
			sb.WriteString(fmt.Sprintf("Aliases: %s\n", strings.Join(prompt.Aliases, ",")))
		}
		if len(prompt.Commands) > 0 {
			sb.WriteString("Commands:")
			for _, cmd := range prompt.Commands {
				sb.WriteString(fmt.Sprintf(" /%s", cmd))
			}
			sb.WriteString("\n")
		}
		if prompt.DynamicPrompt {
			sb.WriteString("Dynamic: yes\n")
		}
		sb.WriteString("---\n")
	}
	return strings.TrimSpace(sb.String())
}

func (c aiConfig) GetUtilityModel() string {
	if model := c.UtilityModel; model != "" {
		return model
	}
	return c.DefaultModel
}

func (c aiConfig) GetToolsModel() string {
	if model := c.ToolsModel; model != "" {
		return model
	}
	return ""
}

func (c aiConfig) GetDefaultProviderAndModel() (provider, model string) {
	parts := strings.SplitN(c.DefaultModel, ":", 2)
	if len(parts) < 2 {
		return "", c.DefaultModel
	}
	return parts[0], parts[1]
}

func (p aiModelParams) Validate() error {
	if p.Temperature != nil && (*p.Temperature < 0 || *p.Temperature > 2) {
		return fmt.Errorf("temperature must be between 0 and 2, got %.2f", *p.Temperature)
	}

	if p.MaxTokens != nil && *p.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be positive, got %d", *p.MaxTokens)
	}

	if p.TopP != nil && (*p.TopP < 0 || *p.TopP > 1) {
		return fmt.Errorf("top_p must be between 0 and 1, got %.2f", *p.TopP)
	}

	if p.FrequencyPenalty != nil && (*p.FrequencyPenalty < -2 || *p.FrequencyPenalty > 2) {
		return fmt.Errorf("frequency_penalty must be between -2 and 2, got %.2f", *p.FrequencyPenalty)
	}

	if p.PresencePenalty != nil && (*p.PresencePenalty < -2 || *p.PresencePenalty > 2) {
		return fmt.Errorf("presence_penalty must be between -2 and 2, got %.2f", *p.PresencePenalty)
	}

	return nil
}

func (c aiConfig) ValidateAll() error {
	if err := c.ModelParams.Validate(); err != nil {
		return fmt.Errorf("global model params: %w", err)
	}

	for _, p := range c.Providers {
		if err := p.ModelParams.Validate(); err != nil {
			return fmt.Errorf("provider %s: %w", p.Name, err)
		}
	}

	for _, a := range c.Aliases {
		if err := a.ModelParams.Validate(); err != nil {
			return fmt.Errorf("alias %s: %w", a.Alias, err)
		}
	}

	return nil
}

func (c aiConfig) GetFullModelParams(providerName, aliasName, promptName string) (map[string]any, error) {
	params := c.ModelParams.ToRequestParams()

	if providerName != "" {
		if provider := c.GetProvider(providerName); provider != nil {
			if err := provider.ModelParams.Validate(); err != nil {
				return nil, fmt.Errorf("provider params: %w", err)
			}
			params = mergeParams(params, provider.ModelParams.ToRequestParams())
		}
	}

	if aliasName != "" {
		if alias, exists := c.GetAlias(aliasName); exists {
			if err := alias.ModelParams.Validate(); err != nil {
				return nil, fmt.Errorf("alias params: %w", err)
			}
			params = mergeParams(params, alias.ModelParams.ToRequestParams())
		}
	}

	if promptName != "" {
		if prompt, exists := c.GetPromptByAliasOrName(promptName); exists {
			if err := prompt.ModelParams.Validate(); err != nil {
				return nil, fmt.Errorf("prompt params: %w", err)
			}
			params = mergeParams(params, prompt.ModelParams.ToRequestParams())
		}
	}

	return params, nil
}

func (c aiConfig) GetProvider(name string) *AIProviderConfig {
	for _, p := range c.Providers {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

func (c aiConfig) GetAlias(alias string) (aiModelAlias, bool) {
	for _, a := range c.Aliases {
		if a.Alias == alias {
			return a, true
		}
	}
	return aiModelAlias{}, false
}

func mergeParams(base, override map[string]any) map[string]any {
	result := make(map[string]any)
	maps.Copy(result, base)

	for k, v := range override {
		if k == "stop" {
			if existing, ok := result[k].([]string); ok {
				result[k] = append(existing, v.([]string)...)
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}
	return result
}

func (c aiConfig) FindPrompt(query string) (aiPrompt, bool) {
	for _, prompt := range c.Prompts {
		if prompt.Name == query {
			return prompt, true
		}
		if slices.Contains(prompt.Aliases, query) {
			return prompt, true
		}
	}
	return aiPrompt{}, false
}

func (c aiConfig) GetAllCommands() []string {
	var commands []string
	for _, prompt := range c.Prompts {
		if prompt.Enabled {
			commands = append(commands, prompt.Name)
			commands = append(commands, prompt.Commands...)
		}
	}
	return commands
}

func (c aiConfig) GetAllAliases() []string {
	var commands []string
	for _, prompt := range c.Prompts {
		if prompt.Enabled {
			commands = append(commands, prompt.Name)
			commands = append(commands, prompt.Aliases...)
		}
	}
	return commands
}

func (c aiConfig) GetPromptByCommand(command string) (aiPrompt, bool) {
	for _, prompt := range c.Prompts {
		if !prompt.Enabled {
			continue
		}
		if slices.Contains(prompt.Commands, command) || command == prompt.Name {
			return prompt, true
		}
	}
	return aiPrompt{}, false
}

func (c aiConfig) GetPromptBy(name string) (aiPrompt, bool) {
	if prompt, exists := c.GetPromptByAliasOrName(name); exists {
		return prompt, true
	}
	if prompt, exists := c.GetPromptByCommand(name); exists {
		return prompt, true
	}
	return aiPrompt{}, false
}

func (c aiConfig) GetPromptByAliasOrName(alias string) (aiPrompt, bool) {
	for _, prompt := range c.Prompts {
		if !prompt.Enabled {
			continue
		}
		if slices.Contains(prompt.Aliases, alias) || alias == prompt.Name {
			return prompt, true
		}
	}
	return aiPrompt{}, false
}

func (c aiConfig) GetPromptByName(name string) (aiPrompt, bool) {
	for _, prompt := range c.Prompts {
		if !prompt.Enabled {
			continue
		}
		if prompt.Name == name {
			return prompt, true
		}
	}
	return aiPrompt{}, false
}

type ytdlpConfig struct {
	MaxSize       string `koanf:"max_size"`
	TempDirectory string `koanf:"temp_directory"`
	DownloadURL   string `koanf:"download_url"`
}

type queueThrottleOptions struct {
	Period      time.Duration `koanf:"period"`
	Concurrency int           `koanf:"concurrency"`
	Requests    int           `koanf:"requests"`
}

type queueOptions struct {
	Enabled    bool                 `koanf:"enabled"`
	MaxRetries int                  `koanf:"max_retries"`
	RetryDelay time.Duration        `koanf:"retry_delay"`
	Timeout    time.Duration        `koanf:"timeout"`
	Throttle   queueThrottleOptions `koanf:"throttle"`
}

type askDisplayOptions struct {
	Context   bool   `koanf:"context"`
	Metadata  bool   `koanf:"metadata"`
	Reasoning bool   `koanf:"reasoning"`
	Separator string `koanf:"separator"`
}

type askImagesOptions struct {
	Enabled  bool          `koanf:"enabled"`
	Max      int           `koanf:"max"`
	Lifetime time.Duration `koanf:"lifetime"`
}

type askAudioOptions struct {
	Enabled      bool `koanf:"enabled"`
	MaxInHistory int  `koanf:"max_in_history"`
	MaxSize      int  `koanf:"max_size"`     // in kb
	MaxDuration  int  `koanf:"max_duration"` // in seconds
}

type askFilesOptions struct {
	Enabled bool `koanf:"enabled"`
}

type askFetcherOptions struct {
	Enabled   bool     `koanf:"enabled"`
	MaxLength int      `koanf:"max_length"`
	Whitelist []string `koanf:"whitelist"`
	Blacklist []string `koanf:"blacklist"`
}

type askToolsOptions struct {
	Enabled       bool     `koanf:"enabled"`
	AutoRun       bool     `koanf:"auto_run"`
	MaxIterations int      `koanf:"max_iterations"`
	Allowed       []string `koanf:"allowed"`
	Excluded      []string `koanf:"excluded"`
}

func (f askFetcherOptions) inWhitelist(URL string) bool {
	for _, part := range f.Whitelist {
		if strings.Contains(URL, part) {
			return true
		}
	}
	return false
}

func (f askFetcherOptions) inBlacklist(URL string) bool {
	for _, part := range f.Blacklist {
		if strings.Contains(URL, part) {
			return true
		}
	}
	return false
}

func (f askFetcherOptions) CheckURL(URL string) bool {
	switch {
	case len(f.Whitelist) > 0 && len(f.Blacklist) == 0:
		return f.inWhitelist(URL)
	case len(f.Blacklist) > 0 && len(f.Whitelist) == 0:
		return !f.inBlacklist(URL)
	case len(f.Blacklist) > 0 && len(f.Whitelist) > 0:
		return f.inWhitelist(URL) && !f.inBlacklist(URL)
	default:
		return true
	}
}

type commandConfig struct {
	Enabled bool         `koanf:"enabled"`
	Queue   queueOptions `koanf:"queue"`
}

type AskCommandConfig struct {
	CommandConfig       commandConfig
	GenerateTitleWithAI bool              `koanf:"generate_title_with_ai"`
	Display             askDisplayOptions `koanf:"display"`
	Fetcher             askFetcherOptions `koanf:"fetcher"`
	Images              askImagesOptions  `koanf:"images"`
	Audio               askAudioOptions   `koanf:"audio"`
	Files               askFilesOptions   `koanf:"files"`
	Tools               askToolsOptions   `koanf:"tools"`
}

type rCommandConfig struct {
	CommandConfig commandConfig
	APIURL        string `koanf:"api_url"`
	APIKey        string `koanf:"api_key"`
	APIUserID     string `koanf:"api_user_id"`
}
