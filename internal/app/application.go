package app

import (
	"context"
	"flag"
	"time"

	"github.com/muratoffalex/gachigazer/internal/app/di"
	"github.com/muratoffalex/gachigazer/internal/commands/ask"
	"github.com/muratoffalex/gachigazer/internal/commands/instagram"
	"github.com/muratoffalex/gachigazer/internal/commands/model"
	"github.com/muratoffalex/gachigazer/internal/commands/random"
	"github.com/muratoffalex/gachigazer/internal/commands/start"
	"github.com/muratoffalex/gachigazer/internal/commands/youtube"
	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/core"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

const FailedToInit = "Failed to init"

type Application struct {
	Logger logger.Logger
	cfg    *config.Config
	bot    *core.Bot
	di     *di.Container
	ctx    context.Context
	cancel context.CancelFunc
}

func New() (*Application, error) {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.Load()
	if err != nil {
		cancel()
		return nil, err
	}

	di, err := di.NewContainer(cfg)
	if err != nil {
		cancel()
		return nil, err
	}
	di.Logger.Info("DI Container created")

	botInstance, err := core.NewBot(
		di.BotClient,
		di.Queue,
		di.Logger,
		di.DB,
		cfg,
		di.Localizer,
	)
	if err != nil {
		di.Logger.Fatal(err)
	}
	di.Logger.Info("Bot instance created")

	app := &Application{
		cfg:    cfg,
		bot:    botInstance,
		di:     di,
		Logger: di.Logger,
		ctx:    ctx,
		cancel: cancel,
	}

	app.registerCommands(ctx)

	return app, nil
}

func (a *Application) Start() error {
	a.Logger.Info("Starting application")
	a.StartMessageCleaner()
	return a.bot.Start(a.ctx)
}

func (a *Application) registerCommands(ctx context.Context) {
	if a.cfg.GetCommandConfig(ask.CommandName).Enabled {
		a.bot.RegisterCommand(ask.New(a.di))
	}
	if a.cfg.GetCommandConfig(model.CommandName).Enabled {
		a.bot.RegisterCommand(model.New(a.di))
	}
	if a.cfg.GetCommandConfig(youtube.CommandName).Enabled {
		if cmd, err := youtube.New(a.di); err == nil {
			a.bot.RegisterCommand(cmd)
		} else {
			a.Logger.WithError(err).WithField("command", youtube.CommandName).Error(FailedToInit)
		}
	}
	if a.cfg.GetCommandConfig(start.CommandName).Enabled {
		a.bot.RegisterCommand(start.New(a.di))
	}
	if cfg := a.cfg.GetRCommandConfig(); cfg.CommandConfig.Enabled {
		if cfg.ApiURL == "" {
			a.Logger.Warn("R command enabled, but api_url doesn't set")
		} else {
			a.bot.RegisterCommand(random.New(a.di))
		}
	}
	if a.cfg.GetCommandConfig(instagram.CommandName).Enabled {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		cmdChan := make(chan *instagram.Command, 1)
		errChan := make(chan error, 1)

		go func() {
			cmd, err := instagram.New(ctx, a.di, a.bot)
			if err != nil {
				errChan <- err
				return
			}
			cmdChan <- cmd
		}()

		select {
		case cmd := <-cmdChan:
			a.bot.RegisterCommand(cmd)
		case err := <-errChan:
			a.Logger.WithError(err).WithField("command", instagram.CommandName).Error(FailedToInit)
		case <-ctx.Done():
			a.Logger.WithField("command", instagram.CommandName).Error("Instagram initialization timed out")
		}
	}
}

func (a *Application) WaitForShutdown() {
	<-a.ctx.Done()
	a.Logger.Info("Application stopped")
}

func (c *Application) StartMessageCleaner() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := c.di.DB.PurgeOldMessages(c.di.Cfg.Global().MessageRetentionDays); err != nil {
				c.di.Logger.Error("Failed to purge old messages: ", err)
			}
		}
	}()
}
