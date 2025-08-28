package model

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/ai"
	"github.com/muratoffalex/gachigazer/internal/app/di"
	"github.com/muratoffalex/gachigazer/internal/commands/base"
	"github.com/muratoffalex/gachigazer/internal/database"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/markdown"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

const CommandName = "model"

type Command struct {
	*base.Command
	ai *ai.ProviderRegistry
	db database.Database
}

func New(di *di.Container) *Command {
	cmd := &Command{
		ai: di.AI,
		db: di.DB,
	}
	cmd.Command = base.NewCommand(cmd, di)
	return cmd
}

func (c *Command) Name() string {
	return CommandName
}

func (c *Command) Aliases() []string {
	return []string{"m"}
}

func (c *Command) Execute(update telegram.Update) error {
	if update.Message == nil {
		return nil
	}

	ctx := context.Background()

	command := update.Message.Command()
	args := strings.TrimSpace(strings.TrimPrefix(
		update.Message.Text,
		"/"+command,
	))
	chatID := update.Message.Chat.ID

	if args == "" {
		currentModel, _ := c.ChatService.GetCurrentModelForChat(context.Background(), chatID, update.Message.From.ID, "")
		currentModelStr := fmt.Sprintf(
			"`%s` %s",
			c.Tg.EscapeText(currentModel.FullName()),
			currentModel.GetFormattedModalities(),
		)
		provider := c.Cfg.AI().GetProvider(currentModel.Provider)
		if provider == nil {
			_, err := c.Tg.Send(telegram.NewMessage(
				chatID,
				c.Localizer.Localize("model.errorProviderNotFound", map[string]any{
					"Provider": currentModel.Provider,
				}),
				update.Message.MessageID,
			))
			if err != nil {
				return err
			}
		}

		// Show current model and permissions
		isAllowedUser := c.Cfg.Telegram().IsUserAllowed(update.Message.From.ID) && !provider.OnlyFreeModels

		permissionInfo := ""
		if isAllowedUser {
			permissionInfo = "\n\n" + c.Localizer.Localize("model.permissionAllowed", nil)
		} else {
			permissionInfo = "\n\n" + c.Localizer.Localize("model.permissionRestricted", nil)
		}

		availableCommands := c.Localizer.Localize("model.allowedCommands", nil)

		msg := telegram.NewMessage(
			chatID,
			c.Localizer.Localize("model.currentStatus", map[string]any{
				"Model":      currentModelStr,
				"Permission": permissionInfo,
				"Commands":   availableCommands,
			}),
			update.Message.MessageID,
		)
		msg.ParseMode = telegram.ModeMarkdownV2
		_, err := c.Tg.Send(msg)
		if err != nil {
			c.Logger.WithFields(logger.Fields{
				"text": msg.Text,
			}).Debug("Send message with text")
		}
		return err
	}

	if strings.HasPrefix(args, "list") {
		searchTerm := strings.TrimSpace(strings.TrimPrefix(args, "list "))
		if searchTerm == "" {
			msg := telegram.NewMessage(
				chatID,
				c.Localizer.Localize("model.specifyListTerm", nil),
				update.Message.MessageID,
			)
			_, err := c.Tg.Send(msg)
			return err
		}

		// Show all models matching search, marking free ones
		allModels, err := c.ai.GetAllModels(context.Background(), false, false)
		if err != nil {
			return err
		}
		allProviderNames := make([]string, 0, len(allModels))
		modelsCount := 0
		for name := range allModels {
			allProviderNames = append(allProviderNames, name)
			modelsCount += len(allModels[name])
		}
		sort.Strings(allProviderNames)
		c.Logger.WithFields(logger.Fields{
			"providers":    allProviderNames,
			"models_count": modelsCount,
		}).Debug("All models fetched")
		freeModels := map[string][]*ai.ModelInfo{}
		for providerName, models := range allModels {
			suitableModels := []*ai.ModelInfo{}
			for _, model := range models {
				if model.IsFree() {
					suitableModels = append(suitableModels, model)
				}
			}
			if len(suitableModels) > 0 {
				freeModels[providerName] = suitableModels
			}
		}

		filteredModels := map[string][]*ai.ModelInfo{}
		switch searchTerm {
		case "free":
			filteredModels = freeModels
		default:
			for providerName, models := range allModels {
				suitableModels := []*ai.ModelInfo{}
				for _, model := range models {
					if strings.Contains(strings.ToLower(model.FullName()), strings.ToLower(searchTerm)) {
						suitableModels = append(suitableModels, model)
					}
				}
				if len(suitableModels) > 0 {
					filteredModels[providerName] = suitableModels
				}
			}
		}

		if searchTerm == "free" {
			filteredModels = freeModels
		}

		if len(filteredModels) == 0 {
			msg := telegram.NewMessage(
				chatID,
				c.Localizer.Localize("model.search.notFound", map[string]any{
					"Query": searchTerm,
				}),
				update.Message.MessageID,
			)
			_, err := c.Tg.Send(msg)
			return err
		}

		providerNames := make([]string, 0, len(filteredModels))
		for name := range filteredModels {
			providerNames = append(providerNames, name)
		}
		sort.Strings(providerNames)

		for _, models := range filteredModels {
			sort.Slice(models, func(i, j int) bool {
				return models[i].ID < models[j].ID
			})
		}

		var modelList strings.Builder
		sentModelsCount := 0
		for _, providerName := range providerNames {
			modelList.WriteString(fmt.Sprintf("*%s*\n", markdown.Escape(providerName)))
			for _, model := range filteredModels[providerName] {
				safeModel := c.Tg.EscapeText(model.FullName())

				modelList.WriteString(fmt.Sprintf(
					"• `%s` %s\n",
					safeModel,
					model.GetFormattedModalities(),
				))
				sentModelsCount++

				if modelList.Len() > 3900 {
					modelList.WriteString("\n" + c.Tg.EscapeText(c.Localizer.Localize("telegram.tgOutputTruncatedWarning", nil)) + "\n")
					break
				}
			}
		}

		header := c.Tg.EscapeText(
			c.Localizer.Localize("model.search.resultsHeader", map[string]any{
				"Count": sentModelsCount,
			}),
		)

		footer := c.Tg.EscapeText(
			c.Localizer.Localize("model.search.switchInstructions", nil),
		)
		messageText := header + modelList.String() + footer

		msg := telegram.NewMessage(chatID, messageText, update.Message.MessageID)
		msg.ParseMode = telegram.ModeMarkdownV2
		_, err = c.Tg.Send(msg)
		if err != nil {
			return err
		}

		c.Logger.WithFields(logger.Fields{
			"chat_id": chatID,
			"models":  sentModelsCount,
		}).Info("Sent model list")

		return nil
	}

	if strings.HasPrefix(args, "reset") {
		err := c.db.DeleteChatModel(chatID)
		if err != nil {
			c.Logger.WithFields(logger.Fields{
				"chat_id": chatID,
			}).WithError(err).Error("Failed to reset chat model")
			msg := telegram.NewMessage(chatID, c.Localizer.Localize("model.reset.fail", nil), update.Message.MessageID)
			_, _ = c.Tg.Send(msg)
			return err
		}

		c.ChatService.ResetChatModel(context.Background(), chatID)

		c.Logger.WithFields(logger.Fields{
			"chat_id": chatID,
		}).Info("Model reset to default")

		msg := telegram.NewMessage(
			chatID,
			c.Localizer.Localize("model.reset.success", nil),
			update.Message.MessageID,
		)
		_, err = c.Tg.Send(msg)
		return err
	}

	modelSpec := args
	model, err := c.ai.GetFormattedModel(ctx, modelSpec, "")
	if err != nil {
		msg := telegram.NewMessage(
			chatID,
			c.Localizer.Localize("model.modelNotExist", map[string]any{
				"ModelName": modelSpec,
			}),
			update.Message.MessageID,
		)
		_, err = c.Tg.Send(msg)
		return err
	}
	provider := c.Cfg.AI().GetProvider(model.Provider)

	// Check if user is allowed to use paid models
	isAllowedUser := c.Cfg.Telegram().IsUserAllowed(update.Message.From.ID) && !provider.OnlyFreeModels

	// For regular users, check if model is free
	if !isAllowedUser {
		freeModels, err := c.ai.GetAllModels(context.Background(), true, true)
		if err != nil {
			return err
		}

		modelIsFree := false
		for providerName, models := range freeModels {
			for _, m := range models {
				if m.FullName() == modelSpec || (provider.Name == providerName && m.ID == modelSpec) {
					modelIsFree = true
					break
				}
			}
		}

		if !modelIsFree {
			var examples []string

			i := 0
			for _, models := range freeModels {
				for _, m := range models {
					if i >= 3 {
						break
					}
					examples = append(examples, fmt.Sprintf("`%s`", markdown.Escape(m.FullName())))
					i++
				}
			}

			msg := telegram.NewMessage(
				chatID,
				c.Localizer.Localize("model.freeUsageInstructions", map[string]any{
					"Examples": strings.Join(examples, "\n"),
				}),
				update.Message.MessageID,
			)
			msg.ParseMode = telegram.ModeMarkdownV2
			_, err := c.Tg.Send(msg)
			return err
		}
	}

	// Save model to DB and cache
	err = c.ChatService.SetChatModel(context.Background(), chatID, modelSpec)
	if err != nil {
		c.Logger.WithFields(logger.Fields{
			"chat_id": chatID,
			"model":   modelSpec,
		}).WithError(err).Error("Failed to save chat model")
		msg := telegram.NewMessage(
			chatID,
			c.Localizer.Localize("model.notFoundError", map[string]any{
				"Model": modelSpec,
			}),
			update.Message.MessageID,
		)
		_, _ = c.Tg.Send(msg)
		return err
	}

	c.Logger.WithFields(logger.Fields{
		"chat_id": chatID,
		"model":   model.FullName(),
	}).Info("Model switched")

	msg := telegram.NewMessage(
		chatID,
		c.Localizer.Localize("model.switchSuccess", map[string]any{
			"ModelName": c.Tg.EscapeText(model.FullName()),
		}),
		update.Message.MessageID,
	)
	msg.ParseMode = telegram.ModeMarkdownV2
	_, err = c.Tg.Send(msg)
	return err
}
