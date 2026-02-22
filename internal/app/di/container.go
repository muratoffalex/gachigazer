package di

import (
	"context"
	"net/http"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/muratoffalex/gachigazer/internal/ai"
	"github.com/muratoffalex/gachigazer/internal/ai/tools"
	"github.com/muratoffalex/gachigazer/internal/cache"
	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/database"
	"github.com/muratoffalex/gachigazer/internal/fetcher"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/markdown"
	"github.com/muratoffalex/gachigazer/internal/network"
	"github.com/muratoffalex/gachigazer/internal/queue"
	"github.com/muratoffalex/gachigazer/internal/service"
	"github.com/muratoffalex/gachigazer/internal/service/youtube"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

type Container struct {
	BotClient   telegram.Client
	TD          *service.TelegramAPI
	Logger      logger.Logger
	DB          database.Database
	Cache       cache.Cache
	Cfg         *config.Config
	Queue       *queue.Queue
	AI          *ai.ProviderRegistry
	ChatService *service.ChatService
	HttpClient  *http.Client
	Localizer   *service.Localizer
	Fetcher     *fetcher.Manager
	YtService   *youtube.Service
}

func NewContainer(cfg *config.Config) (*Container, error) {
	logCfg := cfg.Log()
	l := logger.NewLogrusLogger(&logCfg)
	if cfg.GetAskCommandConfig().CommandConfig.Enabled && len(cfg.AI().Providers) == 0 {
		l.Fatal("Ask command enabled, but AI providers not found")
	}
	db, err := database.NewSQLiteDB(cfg, l)
	if err != nil {
		return nil, err
	}

	memoryCache := cache.NewMemoryCache()
	dbCache := cache.NewDBCache(db)
	c := cache.NewMultiLevelCache(memoryCache, dbCache, l)
	q := queue.NewQueue(db, l)
	localizer, err := service.NewLocalizer(cfg.Global().InterfaceLanguage)
	if err != nil {
		l.WithError(err).Fatal("Error create localizer")
	}

	container := &Container{
		Logger:    l,
		DB:        db,
		Cache:     c,
		Cfg:       cfg,
		Queue:     q,
		Localizer: localizer,
	}

	httpCfg := network.NewDefaultHTTPClientConfig(cfg.HTTP())
	container.HttpClient = network.SetupHTTPClient(httpCfg, l)

	ytService := youtube.NewService(l, container.HttpClient, youtube.Config{
		Proxy:       cfg.HTTP().GetProxy(),
		MaxComments: 50,
	})
	container.YtService = &ytService

	fetcherHTTPCfg := network.NewHTTPClientConfigForFetcher(cfg.HTTP())
	fetcherHTTPClient := network.SetupHTTPClient(fetcherHTTPCfg, l)
	fetcherManager := fetcher.NewManager(l)
	fetcherManager.RegisterFetcher(fetcher.NewFragranticaFetcher(l, fetcherHTTPClient))
	fetcherManager.RegisterFetcher(fetcher.NewRedditFetcher(l, fetcherHTTPClient))
	fetcherManager.RegisterFetcher(fetcher.NewHabrFetcher(l, fetcherHTTPClient))
	fetcherManager.RegisterFetcher(fetcher.NewGithubFetcher(l, fetcherHTTPClient))
	fetcherManager.RegisterFetcher(fetcher.NewOpennetFetcher(l, fetcherHTTPClient))
	fetcherManager.RegisterFetcher(fetcher.NewTelegramFetcher(l, fetcherHTTPClient))
	fetcherManager.RegisterFetcher(fetcher.NewYoutubeFetcher(l, fetcherHTTPClient, &ytService))
	fetcherManager.SetDefaultFetcher(fetcher.NewDefaultFetcher(l, fetcherHTTPClient))
	container.Fetcher = fetcherManager

	providerRegistry := ai.NewProviderRegistry(cfg, l)
	for _, providerCfg := range cfg.AI().Providers {
		providerName := providerCfg.Name
		providerLog := l.WithField("provider", providerName)
		var provider ai.Provider

		switch providerCfg.Type {
		case ai.ProviderOpenrouter:
			provider = ai.NewOpenRouterClient(providerCfg, cfg, l, container.HttpClient)
		case ai.ProviderOpenai:
			provider = ai.NewOpenAICompatibleClient(
				providerName,
				providerCfg.BaseURL,
				providerCfg.ChatURL,
				providerCfg.GetAPIKey(),
				providerCfg.DefaultModel,
				l,
				providerCfg.OverrideModels,
				cfg,
				container.HttpClient,
			)
		case ai.ProviderLocal:
			provider = ai.NewLocalAIClient(providerCfg, cfg, l, container.HttpClient)
		default:
			l.Error("Unsupported AI provider type: " + providerCfg.Type)
			continue
		}

		providerRegistry.RegisterProvider(providerName, provider)
		providerLog.WithField("type", providerCfg.Type).Info("Initialized AI provider")
		// load models in goroutine
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			models, err := provider.GetModels(ctx, false, true)
			if err != nil {
				providerLog.WithError(err).Error("Initial model sync failed")
				return
			}
			providerLog.WithField("models_count", len(models)).Info("Initial models sync successful")
		}()
	}
	chatService := service.NewChatService(db, providerRegistry, cfg)
	providerRegistry.SetChatService(chatService)

	api, err := tgbotapi.NewBotAPI(cfg.Telegram().Token)
	if err != nil {
		l.WithError(err).Fatal("Bot API client initialization error")
	}
	l.Info("Bot API initialized")

	markdownProcessor := markdown.NewMarkdownProcessor(cfg.Telegram().TelegramifyScriptPath, l)
	botClient := telegram.NewBotClient(api, l, *markdownProcessor)

	if cfg.Telegram().TdEnabled {
		td := service.InitTDInstance(cfg.Telegram(), c, l)
		container.TD = td
		tools.AllTools[tools.ToolFetchTgPosts] = tools.ToolFetchTgPostsSpec
		tools.AllTools[tools.ToolFetchTgPostComments] = tools.ToolFetchTgPostCommentsSpec
	}

	container.BotClient = botClient
	container.ChatService = chatService
	container.AI = providerRegistry

	return container, nil
}
