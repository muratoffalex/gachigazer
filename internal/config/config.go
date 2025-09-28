package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
)

const (
	globalMessageRetentionDays      = "global.message_retention_days"
	globalLanguage                  = "global.interface_language"
	globalFixInstagramPreviews      = "global.fix_instagram_previews"
	globalFixXPreviews              = "global.fix_x_previews"
	currencyCode                    = "currency.code"
	currencySymbol                  = "currency.symbol"
	currencyPrecision               = "currency.precision"
	httpProxy                       = "http.proxy"
	aiSystemPrompt                  = "ai.system_prompt"
	aiUseStream                     = "ai.use_stream"
	aiLanguage                      = "ai.language"
	aiUtilityModel                  = "ai.utility_model"
	aiMultimodalModel               = "ai.multimodal_model"
	aiMaxTokens                     = "ai.model_params.max_tokens"
	aiMaxImagesInContext            = "ai.max_images_in_context"
	aiUseMultimodalAuto             = "ai.use_multimodal_auto"
	aiToolsMaxIterations            = "ai.tools_max_iterations"
	telegramToken                   = "telegram.token"
	telegramTdEnabled               = "telegram.td_enabled"
	telegramSessionPath             = "telegram.session_path"
	instagramUsername               = "instagram.username"
	instagramPassword               = "instagram.password"
	instagramSessionPath            = "instagram.session_path"
	instagramSessionRefreshInterval = "instagram.session_refresh_interval"
	chromeEnabled                   = "chrome.enabled"
	chromePath                      = "chrome.path"
	chromeOpts                      = "chrome.opts"
	ytdlpMaxSize                    = "ytdlp.max_size"
	ytdlpTempDirectory              = "ytdlp.temp_directory"
	ytdlpDownloadURL                = "ytdlp.download_url"
	databaseDsn                     = "database.dsn"
	loggingLevel                    = "logging.level"
	loggingWriteInFile              = "logging.write_in_file"
	loggingFilePath                 = "logging.file_path"
)

var defaultSQLiteParams = map[string]string{
	"_journal":      "WAL",
	"_busy_timeout": "10000",
	"_synchronous":  "NORMAL",
	"_cache":        "shared",
	"_auto_vacuum":  "INCREMENTAL",
}

type Config struct {
	k *koanf.Koanf
}

var configPath string

func init() {
	flag.StringVar(&configPath, "config", "", "Path to config file")
}

func Load() (*Config, error) {
	k := koanf.New(".")

	defaults := map[string]any{
		globalMessageRetentionDays: 1,
		globalLanguage:             "en",
		globalFixInstagramPreviews: true,
		globalFixXPreviews:         true,
		currencyPrecision:          7,
		telegramToken:              "",
		telegramTdEnabled:          false,
		telegramSessionPath:        "tg_session.json",
		httpProxy:                  nil,
		instagramSessionPath:       "instagram_session.json",
		databaseDsn:                "bot.db?_journal=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache=shared",
		loggingLevel:               "info",
		loggingWriteInFile:         false,
		ytdlpMaxSize:               "50M", // max size for normal bots without special permission
		ytdlpTempDirectory:         "",
		ytdlpDownloadURL:           "", // Leave empty to use GitHub + auto-detected os/arch.
		aiSystemPrompt:             "",
		aiLanguage:                 "English",
		aiUseStream:                true,
		aiMaxTokens:                850,
		aiMaxImagesInContext:       5,
		aiUtilityModel:             "",
		aiMultimodalModel:          "",
		aiUseMultimodalAuto:        false,
		aiToolsMaxIterations:       1,
		chromeEnabled:              false,
		chromePath:                 getDefaultChromePath(),
		chromeOpts: []string{
			"--headless",
			"--disable-gpu",
			"--no-sandbox",
			"--disable-dev-shm-usage",
			"--disable-crash-reporter",
			"--no-crashpad",
		},
		"commands.instagram.enabled":                        false,
		"commands.instagram.queue.enabled":                  true,
		"commands.instagram.queue.max_retries":              3,
		"commands.instagram.queue.retry_delay":              60 * time.Second,
		"commands.instagram.queue.throttle.period":          2 * time.Minute,
		"commands.instagram.queue.session_refresh_interval": 12 * time.Hour,
		"commands.start.enabled":                            true,
		"commands.start.queue.enabled":                      false,
		"commands.r.enabled":                                false,
		"commands.r.queue.enabled":                          true,
		"commands.r.queue.max_retries":                      3,
		"commands.r.queue.retry_limit":                      5 * time.Second,
		"commands.r.queue.throttle.period":                  10 * time.Second,
		"commands.youtube.enabled":                          true,
		"commands.youtube.queue.enabled":                    true,
		"commands.youtube.queue.max_retries":                0,
		"commands.youtube.queue.timeout":                    5 * time.Minute,
		"commands.youtube.queue.throttle.period":            30 * time.Second,
		"commands.youtube.queue.throttle.requests":          3,
		"commands.youtube.queue.throttle.concurrency":       3,
		"commands.model.enabled":                            true,
		"commands.model.queue.enabled":                      true,
		"commands.model.queue.max_retries":                  0,
		"commands.model.queue.throttle.period":              5 * time.Second,
		"commands.ask.enabled":                              true,
		"commands.ask.generate_title_with_ai":               false,
		"commands.ask.fetcher.enabled":                      true,
		"commands.ask.audio.enabled":                        true,
		"commands.ask.audio.max_in_history":                 0,
		"commands.ask.audio.max_size":                       2000,    // 2mb
		"commands.ask.audio.max_duration":                   60 * 10, // 10 min
		"commands.ask.files.enabled":                        true,
		"commands.ask.images.enabled":                       true,
		"commands.ask.images.max":                           5,
		"commands.ask.images.lifetime":                      0 * time.Minute,
		"commands.ask.queue.enabled":                        true,
		"commands.ask.queue.timeout":                        2 * time.Minute,
		"commands.ask.queue.max_retries":                    0,
		"commands.ask.queue.throttle.period":                20 * time.Second,
		"commands.ask.queue.throttle.concurrency":           2,
		"commands.ask.queue.throttle.requests":              2,
		"commands.ask.display.metadata":                     true,
		"commands.ask.display.context":                      true,
		"commands.ask.display.reasoning":                    true,
		"commands.ask.display.separator":                    "──────",
	}
	k.Load(confmap.Provider(defaults, "."), nil)

	for _, path := range getConfigPaths() {
		if _, err := os.Stat(path); err == nil {
			if err := k.Load(file.Provider(path), toml.Parser()); err != nil {
				return nil, fmt.Errorf("error loading config %s: %v", path, err)
			}
			break
		}
	}

	k.Load(env.Provider("GACHIGAZER_", ".", func(s string) string {
		return strings.ReplaceAll(
			strings.ToLower(strings.TrimPrefix(s, "GACHIGAZER_")),
			"_", ".",
		)
	}), nil)

	if k.Get(telegramToken) == "" {
		return nil, fmt.Errorf("telegram token is required")
	}

	return &Config{k: k}, nil
}

func (c *Config) GetCommandConfig(name string) *commandConfig {
	concurrency := c.k.Int(fmt.Sprintf("commands.%s.queue.throttle.concurrency", name))
	if concurrency == 0 {
		concurrency = 1
	}
	requests := c.k.Int(fmt.Sprintf("commands.%s.queue.throttle.requests", name))
	if requests == 0 {
		requests = 1
	}
	period := c.k.Duration(fmt.Sprintf("commands.%s.queue.throttle.period", name))
	if period == 0 {
		period = 10 * time.Second
	}
	timeout := c.k.Duration(fmt.Sprintf("commands.%s.queue.timeout", name))
	if timeout == 0 {
		timeout = 1 * time.Minute
	}
	return &commandConfig{
		Enabled: c.k.Bool(fmt.Sprintf("commands.%s.enabled", name)),
		Queue: queueOptions{
			Enabled:    c.k.Bool(fmt.Sprintf("commands.%s.queue.enabled", name)),
			MaxRetries: c.k.Int(fmt.Sprintf("commands.%s.queue.max_retries", name)),
			RetryDelay: c.k.Duration(fmt.Sprintf("commands.%s.queue.retry_delay", name)),
			Timeout:    timeout,
			Throttle: queueThrottleOptions{
				Concurrency: concurrency,
				Period:      period,
				Requests:    requests,
			},
		},
	}
}

func (c *Config) GetAskCommandConfig() *AskCommandConfig {
	return &AskCommandConfig{
		CommandConfig:       *c.GetCommandConfig("ask"),
		GenerateTitleWithAI: c.k.Bool("commands.ask.generate_title_with_ai"),
		Images: askImagesOptions{
			Enabled:  c.k.Bool("commands.ask.images.enabled"),
			Max:      c.k.Int("commands.ask.images.max"),
			Lifetime: c.k.Duration("commands.ask.images.lifetime"),
		},
		Audio: askAudioOptions{
			Enabled:      c.k.Bool("commands.ask.audio.enabled"),
			MaxInHistory: c.k.Int("commands.ask.audio.max_in_history"),
			MaxDuration:  c.k.Int("commands.ask.audio.max_duration"),
			MaxSize:      c.k.Int("commands.ask.audio.max_size"),
		},
		Files: askFilesOptions{
			Enabled: c.k.Bool("commands.ask.files.enabled"),
		},
		Fetcher: askFetcherOptions{
			Enabled:   c.k.Bool("commands.ask.fetcher.enabled"),
			MaxLength: c.k.Int("commands.ask.fetcher.max_length"),
			Whitelist: c.k.Strings("commands.ask.fetcher.whitelist"),
			Blacklist: c.k.Strings("commands.ask.fetcher.blacklist"),
		},
		Display: askDisplayOptions{
			Metadata:  c.k.Bool("commands.ask.display.metadata"),
			Context:   c.k.Bool("commands.ask.display.context"),
			Reasoning: c.k.Bool("commands.ask.display.reasoning"),
			Separator: c.k.String("commands.ask.display.separator"),
		},
	}
}

func (c *Config) GetRCommandConfig() *rCommandConfig {
	return &rCommandConfig{
		CommandConfig: *c.GetCommandConfig("ask"),
		APIURL:        c.k.String("commands.r.api_url"),
		APIKey:        c.k.String("commands.r.api_key"),
		APIUserID:     c.k.String("commands.r.api_user_id"),
	}
}

func (c *Config) Telegram() TelegramConfig {
	var cfg TelegramConfig
	if err := c.k.Unmarshal("telegram", &cfg); err != nil {
		log.Fatalf("telegramConfig unmarshal error: %v", err)
		return TelegramConfig{}
	}
	return cfg
}

func (c *Config) Instagram() instagramConfig {
	return instagramConfig{
		Username:               c.k.String(instagramUsername),
		Password:               c.k.String(instagramPassword),
		SessionPath:            c.k.String(instagramSessionPath),
		SessionRefreshInterval: c.k.Duration(instagramSessionRefreshInterval),
	}
}

func (c *Config) YtDlp() ytdlpConfig {
	return ytdlpConfig{
		MaxSize:       c.k.String(ytdlpMaxSize),
		TempDirectory: c.k.String(ytdlpTempDirectory),
		DownloadURL:   c.k.String(ytdlpDownloadURL),
	}
}

func (c *Config) Chrome() chromeConfig {
	return chromeConfig{
		Enabled: c.k.Bool(chromeEnabled),
		Path:    c.k.String(chromePath),
		Opts:    c.k.Strings(chromeOpts),
	}
}

func (c *Config) Currency() CurrencyConfig {
	return CurrencyConfig{
		Code:      c.k.String(currencyCode),
		Symbol:    c.k.String(currencySymbol),
		Precision: c.k.Int(currencyPrecision),
	}
}

func (c *Config) Log() LoggingConfig {
	return LoggingConfig{
		LogLevel:    c.k.String(loggingLevel),
		WriteInFile: c.k.Bool(loggingWriteInFile),
		FilePath:    c.k.String(loggingFilePath),
	}
}

func (c *Config) GetDatabaseDSN() string {
	dsn := c.k.String(databaseDsn)
	parts := strings.Split(dsn, "?")
	path := parts[0]

	params := make(map[string]string)
	if len(parts) > 1 {
		for param := range strings.SplitSeq(parts[1], "&") {
			if kv := strings.Split(param, "="); len(kv) == 2 {
				params[kv[0]] = kv[1]
			}
		}
	}

	for k, v := range defaultSQLiteParams {
		if _, exists := params[k]; !exists {
			params[k] = v
		}
	}

	var queryParams []string
	for k, v := range params {
		queryParams = append(queryParams, k+"="+v)
	}
	sort.Strings(queryParams)

	if len(queryParams) > 0 {
		return path + "?" + strings.Join(queryParams, "&")
	}
	return path
}

func (c *Config) Global() globalConfig {
	return globalConfig{
		MessageRetentionDays: c.k.Int(globalMessageRetentionDays),
		InterfaceLanguage:    c.k.String(globalLanguage),
		FixInstagramPreviews: c.k.Bool(globalFixInstagramPreviews),
		FixXPreviews:         c.k.Bool(globalFixXPreviews),
	}
}

func (c *Config) HTTP() HTTPConfig {
	var proxy string
	if proxyValue := c.k.Get(httpProxy); proxyValue != nil {
		proxy = proxyValue.(string)
	}

	return HTTPConfig{
		proxy: &proxy,
	}
}

func (c *Config) AI() aiConfig {
	var cfg aiConfig
	if err := c.k.Unmarshal("ai", &cfg); err != nil {
		log.Fatalf("aiConfig unmarshal error: %v", err)
		return aiConfig{}
	}
	return cfg
}

func (c *Config) GetModelParams(provider, alias, prompt string) (map[string]any, error) {
	return c.AI().GetFullModelParams(provider, alias, prompt)
}

func getDefaultChromePath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Applications/Chromium.app/Contents/MacOS/Chromium"
	case "linux":
		return "/usr/bin/chromium"
	default:
		return ""
	}
}

func getConfigPaths() []string {
	if configPath != "" {
		return []string{configPath}
	}

	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		home, _ := os.UserHomeDir()
		xdgConfig = filepath.Join(home, ".config")
	}

	return []string{
		"gachigazer.toml",
		"config.toml",
		filepath.Join(xdgConfig, "gachigazer", "config.toml"),
		"/etc/gachigazer/config.toml",
	}
}
