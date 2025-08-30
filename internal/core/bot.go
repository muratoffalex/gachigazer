package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/commands"
	"github.com/muratoffalex/gachigazer/internal/commands/ask"
	"github.com/muratoffalex/gachigazer/internal/commands/instagram"
	"github.com/muratoffalex/gachigazer/internal/commands/random"
	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/database"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/queue"
	"github.com/muratoffalex/gachigazer/internal/service"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

type Bot struct {
	commands  map[string]commands.Command
	logger    logger.Logger
	queue     *queue.Queue
	db        database.Database
	tg        telegram.Client
	cfg       *config.Config
	localizer *service.Localizer
}

func NewBot(
	tg telegram.Client,
	queue *queue.Queue,
	logger logger.Logger,
	db database.Database,
	cfg *config.Config,
	localizer *service.Localizer,
) (*Bot, error) {
	return &Bot{
		commands:  make(map[string]commands.Command),
		tg:        tg,
		queue:     queue,
		cfg:       cfg,
		logger:    logger,
		db:        db,
		localizer: localizer,
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	u := b.tg.NewUpdate(0, 60, 0)

	b.queue.RegisterHandlers(b.commands)
	go b.queue.Start(ctx, b.commands)

	updates := b.tg.GetUpdatesChan(u)

	b.logger.Info("Bot started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			jsonData, _ := json.Marshal(update)
			b.logger.WithFields(logger.Fields{
				"update_structure": string(jsonData),
			}).Debug("Received update")
			if callbackQuery := update.CallbackQuery; callbackQuery != nil {
				chatID := callbackQuery.Message.Chat.ID
				params := strings.Split(callbackQuery.Data, " ")
				commandName := params[0]
				if cmd, exists := b.commands[commandName]; exists {
					args := strings.Split(params[1], ":")
					switch commandName {
					case ask.CommandName:
						if args[0] == "retry" {
							messageIDArg := args[1]
							messageID, err := strconv.ParseInt(messageIDArg, 10, 64)
							if err != nil {
								b.logger.WithError(err).WithField("arg", messageIDArg).Error("Parse int from message ID failed")
								b.sendErrorMessage(err, chatID, callbackQuery.Message.MessageID)
								continue
							}
							dbMessage, err := b.db.GetMessage(chatID, int(messageID))
							if err != nil {
								b.logger.WithError(err).WithFields(logger.Fields{
									"message_id": messageID,
									"chat_id":    chatID,
								}).Error("Get message from DB failed")
								b.sendErrorMessage(err, chatID, callbackQuery.Message.MessageID)
								continue
							}
							dbMessage.CallbackQuery = update.CallbackQuery
							update = *dbMessage
						}
					case random.CommandName:
						update.Message = &telegram.MessageOriginal{
							MessageID: callbackQuery.Message.MessageID,
							From:      callbackQuery.From,
							Chat:      callbackQuery.Message.Chat,
							Text:      "/" + callbackQuery.Data,
							Entities: []telegram.MessageEntity{
								{
									Type:   "bot_command",
									Offset: 0,
									Length: len(strings.Split(callbackQuery.Data, " ")[0]) + 1,
								},
							},
						}
					}

					go func(cmd commands.Command, update telegram.Update) {
						if err := cmd.Handle(update); err != nil {
							b.logger.WithError(err).Error("Failed to handle command from callback")
							b.sendErrorMessage(err, chatID, callbackQuery.Message.MessageID)
						}
					}(cmd, update)

					callback := telegram.NewCallback(callbackQuery.ID, "")
					if _, err := b.tg.Request(&callback); err != nil {
						b.logger.WithError(err).Error("Failed to answer callback query")
					}
				}
				continue
			}

			if update.EditedMessage != nil {
				currentMessage, err := b.db.GetMessage(
					update.EditedMessage.Chat.ID,
					update.EditedMessage.MessageID,
				)
				if err != nil {
					b.logger.WithError(err).Error("Failed to get message from database")
				}
				if currentMessage != nil {
					currentMessage.Message = update.EditedMessage
					editedJSONMessage, err := json.Marshal(currentMessage)
					if err != nil {
						b.logger.WithError(err).Error("Failed to marshal edited message")
					} else {
						if err := b.db.UpdateMessage(
							update.EditedMessage.Chat.ID,
							update.EditedMessage.MessageID,
							editedJSONMessage,
						); err != nil {
							b.logger.WithError(err).Error("Failed to update message in database")
						} else {
							b.logger.WithFields(logger.Fields{
								"chat_id": update.EditedMessage.Chat.ID,
								"message": update.EditedMessage.MessageID,
							}).Debug("Updated message in database")
						}
					}
				} else {
					b.logger.Warn("Message not found in database for update")
				}
			}

			msg := update.Message
			if msg == nil {
				continue
			}

			storedUser, err := b.db.GetUser(msg.From.ID)
			user := database.User{
				ID:        msg.From.ID,
				FirstName: msg.From.FirstName,
				Username:  msg.From.UserName,
			}
			if err != nil {
				if err == sql.ErrNoRows {
					b.logger.WithField("user", user).Info("Store new user")
					err = b.db.SaveUser(user)
					if err != nil {
						b.logger.WithError(err).WithField("user", user).Error("Error save new user")
					}
				} else {
					b.logger.WithError(err).Error("Error get user by id")
				}
			} else {
				if !user.Equal(*storedUser) {
					err = b.db.SaveUser(user)
					if err != nil {
						b.logger.WithError(err).WithField("user", user).Error("Error update user")
					}
				}
			}

			commandText := msg.Text
			if commandText == "" && msg.Caption != "" {
				commandText = msg.Caption
			}

			if !instagram.ContainsInstagramURL(commandText) &&
				(!msg.From.IsBot || msg.From.IsBot && msg.MediaGroupID != "") &&
				(((msg.Sticker != nil ||
					msg.Video != nil ||
					msg.Document != nil) &&
					(msg.MediaGroupID != "" || commandText != "")) ||
					(msg.Sticker == nil &&
						msg.Video == nil &&
						msg.Document == nil)) &&
				!isIgnoreMessage(commandText) {
				replyToBot := false
				if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.IsBot {
					replyToBot = true
				}
				isMain := (msg.MediaGroupID != "" && commandText != "") ||
					(msg.MediaGroupID == "" && !isCommand(commandText) && !replyToBot)
				if err := b.db.SaveMessage(
					msg.Chat.ID,
					msg.MessageID,
					msg.From.UserName,
					msg.MediaGroupID,
					isMain,
					jsonData,
				); err != nil {
					b.logger.WithError(err).Error("Failed to save message to database")
				} else {
					b.logger.WithFields(logger.Fields{
						"chat_id":        msg.Chat.ID,
						"message":        msg.MessageID,
						"username":       msg.From.UserName,
						"media_group_id": msg.MediaGroupID,
						"main":           isMain,
					}).Debug("Saved message to database")
				}
			}

			// Check permissions
			if !b.cfg.Telegram().IsAllowed(msg.From.ID, msg.Chat.ID) {
				b.logger.WithFields(logger.Fields{
					"user_id":  msg.From.ID,
					"username": msg.From.UserName,
					"chat_id":  msg.Chat.ID,
				}).Warn("Unauthorized access attempt")
				continue
			}

			botUsername := b.tg.Self().UserName
			if !isCommand(commandText) && b.containsBotMention(commandText, botUsername) {
				update.Message.Text = strings.ReplaceAll(strings.ToLower(update.Message.Text), "@"+strings.ToLower(botUsername), "")
				update.Message.Caption = strings.ReplaceAll(strings.ToLower(update.Message.Caption), "@"+strings.ToLower(botUsername), "")
				if cmd, ok := b.commands[ask.CommandName]; ok {
					go func(cmd commands.Command, update telegram.Update) {
						if err := cmd.Handle(update); err != nil {
							b.logger.WithError(err).Error("Failed to handle mention as ask command")
							b.sendErrorMessage(err, msg.Chat.ID, msg.MessageID)
						}
					}(cmd, update)
					continue
				}
			}

			if msg.ForwardOrigin != nil {
				continue
			}

			if commandText == "" && msg.Caption != "" {
				commandText = msg.Caption
				if strings.HasPrefix(commandText, "/") {
					parts := strings.Fields(commandText)
					cmdParts := strings.Split(strings.TrimPrefix(parts[0], "/"), "@")
					if len(cmdParts) > 1 && !strings.EqualFold(cmdParts[1], b.tg.Self().UserName) {
						continue
					}
					commandText = "/" + cmdParts[0]
					if len(parts) > 1 {
						commandText += " " + strings.Join(parts[1:], " ")
					}
				}
			}

			// Handle commands
			if commandText != "" && strings.HasPrefix(commandText, "/") {
				parts := strings.Fields(commandText)
				if len(parts) == 0 {
					continue
				}
				cmdParts := strings.Split(strings.TrimPrefix(parts[0], "/"), "@")
				command := cmdParts[0]
				if len(cmdParts) > 1 && !strings.EqualFold(cmdParts[1], b.tg.Self().UserName) {
					continue // skip commands addressed to other bots
				}
				var cmd commands.Command
				for name, c := range b.commands {
					if name == command || slices.Contains(c.Aliases(), command) {
						cmd = c
						break
					}
				}

				if cmd != nil {
					b.logger.WithFields(logger.Fields{
						"command":  command,
						"user_id":  msg.From.ID,
						"username": msg.From.UserName,
						"args":     msg.CommandArguments(),
					}).Info("Handling command")

					go func(cmd commands.Command, update telegram.Update) {
						if err := cmd.Handle(update); err != nil {
							b.logger.WithError(err).Error("Failed to handle command")
							b.sendErrorMessage(err, msg.Chat.ID, msg.MessageID)
						}
					}(cmd, update)
				}
				continue
			}

			replyText := ""
			if msg.ReplyToMessage != nil {
				replyText = msg.ReplyToMessage.Text
				if replyText == "" {
					replyText = msg.ReplyToMessage.Caption
				}
			}
			// Handle replies to bot's Ask responses
			if msg.ReplyToMessage != nil &&
				msg.ReplyToMessage.From.ID == b.tg.Self().ID &&
				strings.Contains(replyText, ask.BotMessageMarker) &&
				// Only handle text messages (not media, documents, etc)
				(msg.Text != "" || msg.Caption != "" || msg.Photo != nil || msg.Voice != nil || msg.Audio != nil || (msg.Document != nil && strings.HasSuffix(strings.ToLower(msg.Document.FileName), ".pdf"))) &&
				// Skip media messages
				msg.Video == nil &&
				msg.Sticker == nil &&
				msg.Animation == nil && !isIgnoreMessage(commandText) {

				var cmd commands.Command
				if command, ok := b.commands[ask.CommandName]; ok {
					cmd = command
				}

				if cmd != nil {
					b.logger.WithFields(logger.Fields{
						"command":  "a",
						"user_id":  msg.From.ID,
						"username": msg.From.UserName,
					}).Info("Handling reply to AI as /a command")

					go func(cmd commands.Command, update telegram.Update) {
						if err := cmd.Handle(update); err != nil {
							b.logger.WithError(err).Error("Failed to handle AI reply")
							b.sendErrorMessage(err, msg.Chat.ID, msg.MessageID)
						}
					}(cmd, update)
				}
				continue
			}

			if b.cfg.Global().FixInstagramPreviews && instagram.ContainsInstagramURL(commandText) {
				instaURL := instagram.ExtractInstagramURL(commandText)
				if instaURL != "" {
					modifiedURL := strings.Replace(instaURL, "www.instagram", "ddinstagram", 1)
					b.tg.SendWithRetry(telegram.NewMessage(msg.Chat.ID, modifiedURL, msg.MessageID), 0)
				}
			}
		}
	}
}

func (b *Bot) RegisterCommand(cmd commands.Command) {
	if cmd == nil {
		b.logger.Error("Attempting to register nil command")
		return
	}

	name := cmd.Name()
	if name == "" {
		b.logger.Error("Attempting to register command with empty name")
		return
	}

	b.logger.WithFields(logger.Fields{
		"command": name,
	}).Debug("Registering command")

	b.commands[name] = cmd
}

func isIgnoreMessage(text string) bool {
	return strings.HasPrefix(text, ">")
}

func isCommand(commandText string) bool {
	return strings.HasPrefix(commandText, "/")
}

func (b *Bot) sendErrorMessage(err error, chatID int64, messageID int) error {
	errorMsg := telegram.NewMessage(
		chatID,
		fmt.Sprintf("%s: %v", b.localizer.Localize("error", nil), err),
		messageID,
	)
	if _, sendErr := b.tg.Send(errorMsg); sendErr != nil {
		b.logger.WithError(sendErr).Error("Failed to send error message")
		return sendErr
	}
	return nil
}

func (b *Bot) GetCommands() map[string]commands.Command {
	return b.commands
}

func (b *Bot) containsBotMention(text string, botUsername string) bool {
	if !strings.Contains(text, "@") {
		return false
	}
	return strings.Contains(strings.ToLower(text), "@"+strings.ToLower(botUsername))
}
