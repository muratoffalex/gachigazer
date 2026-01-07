package base

import (
	"time"

	"github.com/muratoffalex/gachigazer/internal/app/di"
	"github.com/muratoffalex/gachigazer/internal/commands"
	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/fetcher"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/queue"
	"github.com/muratoffalex/gachigazer/internal/service"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

type Command struct {
	command     commands.Command
	Tg          telegram.Client
	Logger      logger.Logger
	Cfg         *config.Config
	Queue       *queue.Queue
	ChatService *service.ChatService
	Localizer   *service.Localizer
}

func NewCommand(cmd commands.Command, di *di.Container) *Command {
	return &Command{
		command:     cmd,
		Tg:          di.BotClient,
		Logger:      di.Logger,
		Cfg:         di.Cfg,
		Queue:       di.Queue,
		ChatService: di.ChatService,
		Localizer:   di.Localizer,
	}
}

func (c *Command) Name() string {
	return ""
}

func (c *Command) Aliases() []string {
	return []string{}
}

func (c *Command) Handle(update telegram.Update) error {
	cfg := c.Cfg.GetCommandConfig(c.command.Name())
	if cfg.Queue.Enabled {
		config := c.command.GetQueueConfig()
		retryDelayMillis := int64(config.RetryDelay / time.Millisecond)
		return c.Queue.Add(c.command, update,
			config.MaxRetries,
			retryDelayMillis)
	} else {
		return c.command.Execute(update)
	}
}

func (c *Command) GetQueueConfig() commands.QueueConfig {
	cfg := c.Cfg.GetCommandConfig(c.command.Name())
	return commands.QueueConfig{
		MaxRetries: cfg.Queue.MaxRetries,
		RetryDelay: cfg.Queue.RetryDelay,
		Timeout:    cfg.Queue.Timeout,
		Throttle: commands.ThrottleConfig{
			Concurrency: cfg.Queue.Throttle.Concurrency,
			Period:      cfg.Queue.Throttle.Period,
			Requests:    cfg.Queue.Throttle.Requests,
		},
	}
}

func (c *Command) Execute(update telegram.Update) error {
	return nil
}

func (c *Command) L(messageID string, data map[string]any) string {
	return c.Localizer.Localize(messageID, data)
}

func (c *Command) ExtractURLsFromEntities(text string, entities []telegram.MessageEntity) []string {
	urls := []string{}
	runes := []rune(text)
	for _, entity := range entities {
		if (entity.Type == "url" || entity.Type == "text_link") &&
			entity.Offset >= 0 &&
			entity.Length > 0 &&
			entity.Offset+entity.Length <= len(runes) {

			url := string(runes[entity.Offset : entity.Offset+entity.Length])
			if entity.Type == "text_link" && entity.URL != "" {
				url = entity.URL
			}
			if fetcher.IsURL(url) {
				urls = append(urls, url)
			}
		}
	}
	return urls
}
