package ask

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/muratoffalex/gachigazer/internal/ai"
	"github.com/muratoffalex/gachigazer/internal/ai/tools"
	"github.com/muratoffalex/gachigazer/internal/app/di"
	"github.com/muratoffalex/gachigazer/internal/commands/base"
	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/database"
	fetch "github.com/muratoffalex/gachigazer/internal/fetcher"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/markdown"
	"github.com/muratoffalex/gachigazer/internal/service"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

const (
	CommandName      = "ask"
	BotMessageMarker = "\u200B"
)

type Argument struct {
	Name         string
	Description  string
	Type         string
	Values       []string
	Min          *float64
	Max          *float64
	DefaultValue string
}

func ptr(f float64) *float64 { return &f }

type Command struct {
	*base.Command
	ai            *ai.ProviderRegistry
	db            database.Database
	supportedArgs []Argument
	fetcher       *fetch.Manager
	httpClient    *http.Client
	retryCount    int
	args          *CommandArgs
	cmdCfg        *config.AskCommandConfig
	toolsRunner   *tools.Tools
}

func (c *Command) Name() string {
	return CommandName
}

func (c *Command) Aliases() []string {
	aliases := []string{"ai", "a", "info", "tools", "new", "help"}
	aliases = append(aliases, c.Cfg.AI().GetAllCommands()...)
	return aliases
}

func New(di *di.Container) *Command {
	promptAliases := di.Cfg.AI().GetAllAliases()
	promptAliases = append(promptAliases, "help")
	toolsCfg := di.Cfg.GetAskCommandConfig().Tools
	availableTools := strings.Join(tools.ToolNames(toolsCfg.Allowed, toolsCfg.Excluded), ", ")
	toolsRunner := tools.NewTools(di.HttpClient, di.Fetcher, di.YtService, di.Logger)
	cmd := &Command{
		fetcher:     di.Fetcher,
		httpClient:  di.HttpClient,
		cmdCfg:      di.Cfg.GetAskCommandConfig(),
		toolsRunner: toolsRunner,
		supportedArgs: []Argument{
			{
				Name:        "m",
				Description: "Model selection",
				Type:        "string",
				Values:      []string{"model name (e.g. `$m:or:deepseek/deepseek-r1`) or alias (available aliases: `rf` (random-free), `think`, `fast`, `multi`"},
			},
			{
				Name:        "c",
				Description: "Additional context",
				Type:        "string",
				Values:      []string{"how many recent messages to include in context, e.g.: $c:5. Or for a time period: $c:5m, $c:30s. Can also specify a user in format: $c:5m@username"},
			},
			{
				Name:         "search",
				Description:  "Internet search with openrouter engine",
				Type:         "bool",
				DefaultValue: "no",
			},
			{
				Name:        "i",
				Description: "Enable image processing",
				Type:        "bool",
			},
			{
				Name:        "u",
				Description: "Enable URL processing",
				Type:        "bool",
			},
			{
				Name:        "ni",
				Description: "Disable image processing",
				Type:        "bool",
			},
			{
				Name:        "nu",
				Description: "Disable URL processing",
				Type:        "bool",
			},
			{
				Name:        "f",
				Description: "Enable file processing",
				Type:        "bool",
			},
			{
				Name:        "nf",
				Description: "Disable file processing",
				Type:        "bool",
			},
			{
				Name:        "a",
				Description: "Enable audio processing",
				Type:        "bool",
			},
			{
				Name:        "na",
				Description: "Disable audio processing",
				Type:        "bool",
			},
			{
				Name:         "recursive",
				Description:  "Recursively process URLs and images (e.g., URL in website content)",
				Type:         "bool",
				DefaultValue: "no",
			},
			{
				Name:        "r",
				Description: "Enable reasoning",
				Type:        "bool",
			},
			{
				Name:        "nr",
				Description: "Disable reasoning",
				Type:        "bool",
			},
			{
				Name:        "tools",
				Description: "Which tools to use. Available tools: " + availableTools,
				Type:        "string",
				Values:      []string{"Tool list in format: `$tools:search,second_tool`, or just `$tools` to send all tools"},
			},
			{
				Name:        "think",
				Description: "Use thinking model (alias for $m:think)",
				Type:        "bool",
			},
			{
				Name:        "multi",
				Description: "Use multimodal model (alias for $m:multi)",
				Type:        "bool",
			},
			{
				Name:        "fast",
				Description: "Use fast model (alias for $m:fast)",
				Type:        "bool",
			},
			{
				Name:        "rf",
				Description: "Use random free model (alias for $m:rf)",
				Type:        "bool",
			},
			{
				Name:        "temp",
				Description: "Model temperature for query. Higher values make responses more creative. (default: 1.0)",
				Type:        "float",
				Min:         ptr(0.0),
				Max:         ptr(2.0),
			},
			{
				Name:        "topp",
				Description: "Parameter controlling response diversity. Higher values make responses more predictable. (default: 1.0)",
				Type:        "float",
				Min:         ptr(0.0),
				Max:         ptr(1.0),
			},
			{
				Name:        "stream",
				Description: "Get response as stream (default: yes)",
				Type:        "bool",
			},
			{
				Name:        "p",
				Description: "Prompt",
				Type:        "string",
				Values:      promptAliases,
			},
			{
				Name:        "new",
				Description: "Create conversation summary and start new one based on it",
				Type:        "bool",
			},
			{
				Name:        "id",
				Description: "Continue message chain with id. Example: $id:123456",
				Type:        "int",
				Values:      []string{"message id to continue chain from"},
			},
		},
	}
	cmd.Command = base.NewCommand(cmd, di)
	cmd.ai = di.AI
	cmd.db = di.DB
	return cmd
}

func (c *Command) Execute(update telegram.Update) error {
	chainID := uuid.New()
	ctx := context.Background()

	var err error
	var editedMessage int

	msg := update.Message
	if msg == nil {
		if update.CallbackQuery != nil {
			msg = update.CallbackQuery.Message
		} else {
			return errors.New("message in update not found")
		}
	}
	chatID := msg.Chat.ID
	messageID := msg.MessageID
	c.Logger = c.Logger.WithField("message_id", messageID)
	replyToMessageID := int64(0)

	var attempt uint8
	var historyMessage *conversationMessage
	toolFromCallback := false
	if callback := update.CallbackQuery; callback != nil {
		if strings.Contains(callback.Data, "retry:") {
			editedMessage = callback.Message.MessageID
			historyMessage, err = c.getMessageFromHistory(msg.Chat.ID, int64(msg.MessageID))
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					c.Logger.Debug("Message in conversation history not found")
				}
				attempt = 1
			} else {
				c.Logger.Info("Message in conversation history found")
				attempt = historyMessage.AttemptsCount + 1
			}
		} else if strings.Contains(callback.Data, "$tools") {
			parts := strings.Split(callback.Data, " ")
			tool := parts[1]
			args := strings.Join(parts[2:], " ")
			if tool == "all" {
				msg.Text = "Run tools from your previous message " + args
			} else {
				msg.Text = fmt.Sprintf("Run only tool %s %s", tool, args)
			}
			msg.Caption = ""
			msg.ReplyToMessage = nil
			msg.From = callback.From

			editMsg := telegram.NewEditMessageReplyMarkup(msg.Chat.ID, msg.MessageID, &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{}})
			_, err := c.Tg.Send(editMsg)
			if err != nil {
				c.Logger.WithError(err).Error("Delete reply markup from message failed")
			}
			toolFromCallback = true
		}
	}

	userID := msg.From.ID
	encodedUserID := c.getUserPublicID(userID)

	currentContent := c.ExtractMessageContent(msg, true)
	command := currentContent.Command
	switch command {
	case "info":
		return c.handleInfoCommand(update)
	case "new":
		command = "a"
		currentContent.Args["new"] = "yes"
	case "tools":
		command = "a"
		currentContent.Args["tools"] = "yes"
	case "help":
		command = "a"
		currentContent.Args["p"] = "help"
	}
	if !c.cmdCfg.Tools.Enabled {
		delete(currentContent.Args, "tools")
	} else if _, exists := currentContent.Args["tools"]; !exists && c.cmdCfg.Tools.AutoRun {
		currentContent.Args["tools"] = "yes"
	}
	c.args, err = c.mapArgsToStruct(currentContent.Args)
	if err != nil {
		currentContent.Text = currentContent.Text + "\n" + err.Error()
		c.Logger.WithError(err).Error("Map args to struct error, add error text in message text")
	}

	c.sendTypingMessage(chatID)

	c.Logger.WithFields(logger.Fields{
		"args": currentContent.Args,
	}).Debug("Parsed arguments")

	model, err := c.ChatService.GetCurrentModelForChat(ctx, chatID, userID, c.args.Model)
	if err != nil || (!model.IsFree() && !c.Cfg.Telegram().IsAllowed(userID, chatID)) {
		modelName := c.args.Model
		if model != nil {
			modelName = model.FullName()
		}
		c.Logger.WithError(err).WithFields(logger.Fields{
			"model": modelName,
		}).Error("Failed to get a model for LLM")
		modelName = c.Tg.EscapeText(modelName)
		text := c.L("ask.modelNotAvailable", map[string]any{
			"ModelName": modelName,
		})
		_, errSend := c.sendOrEditMessage(chatID, messageID, editedMessage, text, &telegram.TextMessage{
			ParseMode: telegram.ModeMarkdownV2,
		})
		if errSend != nil {
			c.Logger.WithError(err).WithFields(logger.Fields{
				"chatID": chatID,
				"text":   text,
			}).Error("Failed to send a message")
		}
		return err
	}

	if command == "" || !slices.Contains(c.Aliases(), command) {
		command = "a"
	}

	if c.args.Prompt == "help" {
		currentContent.Prompt = prompt{
			Text: c.composeHelpMessage(),
			Name: "Help",
		}
	} else if aiPrompt, exists := c.Cfg.AI().GetPromptByAliasOrName(c.args.Prompt); exists {
		currentContent.Prompt = prompt{
			Text:    aiPrompt.Text,
			Name:    aiPrompt.Name,
			Dynamic: aiPrompt.DynamicPrompt,
		}
	} else if aiPrompt, exists := c.Cfg.AI().GetPromptByCommand(command); exists {
		currentContent.Prompt = prompt{
			Text:    aiPrompt.Text,
			Name:    aiPrompt.Name,
			Dynamic: aiPrompt.DynamicPrompt,
		}
	}

	var dynamicPrompt string
	if prompt := currentContent.Prompt; prompt.Name != "" {
		if prompt.Dynamic {
			cleanedText, extractedPattern := extractAndRemovePattern(currentContent.Text)
			currentContent.Text = cleanedText
			sentMsgID, err := c.sendOrEditMessage(
				chatID,
				messageID,
				editedMessage,
				c.L("ask.generatingPerson", nil),
				nil,
			)
			if err != nil {
				c.Logger.WithError(err).Error("Failed to send generating person message")
				_, _ = c.sendOrEditMessage(
					chatID,
					messageID,
					editedMessage,
					c.L("ask.errorSendMessage", nil),
					nil,
				)
				return err
			}
			editedMessage = sentMsgID
			dynamicPrompt, err = c.generate(ctx, prompt.Text, extractedPattern, chatID)
			if err != nil {
				return err
			}
			currentContent.Prompt.Text = dynamicPrompt
		}
	}

	// TODO: mb move thinking message there?

	currentContent.UserInfo.Name = msg.From.FirstName
	currentContent.UserInfo.EncodedID = encodedUserID

	// Determine the primary text content and the message ID to potentially fetch history from
	historyStartMessageID := int64(0)

	var replyMsg *telegram.MessageOriginal
	if msg.ReplyToMessage != nil {
		replyMsg = msg.ReplyToMessage
		replyToMessageID = int64(replyMsg.MessageID)
		// get history if reply to bot message, not user
		if msg.ReplyToMessage.From.ID == c.Tg.Self().ID {
			historyStartMessageID = replyToMessageID
		}
	}

	var latestMessage *conversationMessage
	continueChainID := c.args.ChainID
	if continueChainID != 0 {
		historyStartMessageID = int64(continueChainID)
	}

	latestMessage, err = c.getMessageFromHistory(chatID, historyStartMessageID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	} else {
		if latestMessage != nil {
			c.Logger.WithFields(logger.Fields{
				"latest_message_id": latestMessage.ID,
			}).Debug("LATEST MESSAGE")
		}
	}
	// --- Fetch Conversation History ---
	totalUsage := &MetadataUsage{}
	if latestMessage != nil {
		var conversationHistory []conversationMessage
		c.Logger.WithFields(logger.Fields{
			"chat_id":    chatID,
			"message_id": historyStartMessageID,
		}).Info("Fetching conversation history")
		conversationHistory, err = c.getConversationHistory(chatID, latestMessage.MessageID)
		if err != nil {
			// Log error but continue processing, maybe with just the current message
			c.Logger.WithError(err).Error("Failed to retrieve conversation history")
			conversationHistory = []conversationMessage{} // Ensure it's empty on error
		}
		for _, msg := range conversationHistory {
			if msg.Role.IsAssistant() {
				totalUsage.Add(msg.Usage)
			}
		}

		c.Logger.WithFields(logger.Fields{
			"history_messages": len(conversationHistory),
		}).Debug("Fetched conversation history")

		currentContent.ConversationHistory = conversationHistory
		currentContent.ConversationHistoryLength = currentContent.ContextTurnsCount()
		currentContent.ConversationHistory = nil

		if c.args.Tools != "" {
			if currentContent.Text == "" {
				currentContent.Text = "Run tools from your previous message"
			}
			// get only first chain
			currentChainID := latestMessage.ConversationChainID
			for _, item := range conversationHistory {
				if currentChainID != item.ConversationChainID {
					break
				}
				item.Images = []ai.Content{}
				item.Files = []ai.Content{}
				item.Audio = []ai.Content{}
				currentContent.AddConversationHistoryItems(item)
			}
		} else {
			currentContent.AddConversationHistoryItems(conversationHistory...)
		}
	}

	var additionalContext []telegram.Update
	if additionalContextArg := c.args.Context; additionalContextArg != "" {
		count, duration, username, contextMessageID, _ := parseAdditionalContextArg(additionalContextArg)

		c.Logger.WithFields(logger.Fields{
			"chat_id":          chatID,
			"count":            count,
			"duration":         duration,
			"contextMessageID": contextMessageID,
			"username":         username,
		}).Debug("Fetching additional context")

		msgs, err := c.db.GetMessagesBy(chatID, int64(msg.MessageID), duration, count, username)
		if err != nil {
			_, _ = c.sendOrEditMessage(
				chatID,
				messageID,
				editedMessage,
				c.L("ask.errorRetrieveMessages", nil),
				nil,
			)
			return err
		}
		additionalContext = msgs
		if len(msgs) > 0 {
			for _, m := range msgs {
				msg := m.Message
				messageContent := c.ExtractMessageContent(msg, false)
				contextLine := fmt.Sprintf("[msg:%d] ", msg.MessageID)

				if rm := m.Message.ReplyToMessage; rm != nil {
					contextLine += fmt.Sprintf("[reply:%d] ", rm.MessageID)
				}

				contextLine += fmt.Sprintf(
					"%s(%s)",
					msg.From.FirstName,
					encodedUserID,
				)

				metadata := ""
				if fo := c.createForwardOrigin(msg.ForwardOrigin); fo != nil {
					contextLine += formatForwardOrigin(fo)
					// NOTE: only photo supported for additional context
					if msg.Photo != nil {
						metadata += "🖼️ "
					}
				}
				contextLine += ": " + metadata + messageContent.Text
				currentContent.Context = append(currentContent.Context, contextLine)
				// TODO: maybe no need to add URLs and images to the main request,
				// because there can be a lot of them
				currentContent.Media = append(currentContent.Media, messageContent.Media...)
				currentContent.AddURLsFromMap(messageContent.URLs)
				currentContent.ImageURLs = append(currentContent.ImageURLs, messageContent.ImageURLs...)
			}
		}
	}

	// if this is first message and default prompt exists then use it
	if currentContent.Prompt.Name == "" && !currentContent.HasHistory() {
		if aiPrompt, exists := c.Cfg.AI().GetPromptByCommand("default"); exists {
			currentContent.Prompt = prompt{
				Text: aiPrompt.Text,
				Name: aiPrompt.Name,
			}
		}
	}

	if replyMsg != nil {
		// Auto-detect if replying to bot's Ask completed response (contains BotMessageMarker)
		isReplyingToAsk := strings.Contains(replyMsg.Text, BotMessageMarker) &&
			replyMsg.From.ID == c.Tg.Self().ID

		if !isReplyingToAsk {
			replyMsgContent := &MessageContent{
				Date: time.Unix(int64(replyMsg.Date), 0),
			}

			replyContent := c.ExtractMessageContent(msg.ReplyToMessage, false)
			replyMsgContent.Text = replyContent.Text
			replyMsgContent.UserInfo.Name = replyMsg.From.FirstName
			replyMsgContent.UserInfo.EncodedID = c.getUserPublicID(replyMsg.From.ID)
			replyMsgContent.ForwardOrigin = c.createForwardOrigin(replyMsg.ForwardOrigin)
			currentContent.AddMedia(replyContent.Media...)
			currentContent.AddURLsFromMap(replyContent.URLs)
			currentContent.AddImageURLs(replyContent.ImageURLs...)
			currentContent.AddFileURLs(replyContent.FileURLs...)

			if mediaGroupID := replyMsg.MediaGroupID; mediaGroupID != "" {
				c.Logger.WithFields(logger.Fields{
					"media_group_id": mediaGroupID,
					"chat_id":        chatID,
				}).Debug("Fetching messages with media group ID")
				msgs, err := c.db.GetMessagesWithMediaGroupID(chatID, mediaGroupID)
				if err != nil {
					c.Logger.WithError(err).Warn("Failed to get messages with media group ID")
				}

				c.Logger.WithFields(logger.Fields{
					"media_group_id": mediaGroupID,
					"messages_count": len(msgs),
				}).Info("Fetched messages with media group ID")

				for _, m := range msgs {
					if m.Message.MessageID == replyMsg.MessageID {
						continue
					}
					messageContent := c.ExtractMessageContent(m.Message, false)
					currentContent.AddMedia(messageContent.Media...)
					currentContent.AddURLsFromMap(messageContent.URLs)
					if m.Message.Caption != "" {
						replyContent.Text = m.Message.Caption
					}
				}
			}
			currentContent.ReplyMsgContent = replyMsgContent
		}
	}

	if msg.Quote != nil && msg.Quote.Text != "" {
		currentContent.Quote = msg.Quote.Text
	}

	if currentContent.Text == "" && len(currentContent.GetImagesMedia()) > 0 && model.SupportsImageRecognition() {
		currentContent.Text = "What is shown in the picture?"
	}
	// if currentContent.Text == "" && len(currentContent.GetAudioMedia()) > 0 && model.SupportsAudioRecognition() {
	// 	currentContent.Text = "What is this audio about?"
	// }
	if currentContent.Text == "" && len(currentContent.GetFilesMedia()) > 0 && model.SupportsFiles() {
		currentContent.Text = "Please analyze this file."
	}

	// --- Input Validation ---
	if currentContent.IsEmpty() && !currentContent.HasHistory() {
		msg := telegram.NewMessage(
			chatID,
			c.L("ask.errorPleaseSpecifyText", nil),
			messageID,
		)
		_, err := c.Tg.Send(msg)
		return err
	}

	// --- Prepare for AI ---
	if c.args.New && currentContent.HasHistory() {
		sentMsgID, err := c.sendOrEditMessage(
			chatID,
			messageID,
			editedMessage,
			c.L("ask.generatingSummary", nil),
			nil,
		)
		if err != nil {
			c.Logger.WithError(err).Error("Generating summary message send failed")
		} else {
			editedMessage = sentMsgID
		}
		currentContent.Summary, err = c.summarize(ctx, currentContent.GetConversationHistoryText(), chatID)
		if err != nil {
			c.Logger.WithError(err).Error("Summary generating failed")
			return c.handleErrorWithRetry(
				chatID,
				c.L("ask.errorFailGeneratingSummary", nil),
				editedMessage,
				messageID,
				err,
				false,
			)
		}
		c.Logger.WithFields(logger.Fields{
			"summary": currentContent.Summary,
			"context": currentContent.GetConversationHistoryText(),
		}).Info("Summary generated")
		err = c.updateConversationSummary(
			currentContent.Summary,
			chatID,
			currentContent.GetLatestConversationMessage().ConversationID,
		)
		if err != nil {
			c.Logger.WithError(err).WithFields(logger.Fields{
				"message_id":      messageID,
				"summary":         currentContent.Summary,
				"conversation_id": currentContent.GetLatestConversationMessage().ConversationID,
			}).Error("Save conversation summary failed")
		}
		currentContent.ConversationHistory = []conversationMessage{}
		historyStartMessageID = 0
		totalUsage = &MetadataUsage{}
	}

	if c.args.HandleURLs {
		urls := currentContent.GetAllURLs()
		if len(urls) > 0 {
			urlsString := strings.Join(urls, "\n")
			msgText := fmt.Sprintf("%s\n\n%s", c.L("ask.handleURLs", nil), urlsString)
			newMsgID, err := c.sendOrEditMessage(
				chatID,
				messageID,
				editedMessage,
				msgText,
				&telegram.TextMessage{LinkPreviewDisabled: true},
			)
			if err != nil {
				c.Logger.WithError(err).Error("Failed to send handle URLs message")
			} else {
				editedMessage = newMsgID
			}
			currentContent, _ = c.handleURLs(currentContent, chatID, c.args.Recursive)
		}
	}

	thinkingText := c.L("ask.thinking", nil)
	botMessageID, err := c.sendOrEditMessage(chatID, messageID, editedMessage, thinkingText, nil)
	if err != nil {
		c.Logger.WithError(err).Error("Failed to send thinking message")
		return err
	}

	uniqueImageUrls := make(map[string]bool)
	for _, url := range currentContent.ImageURLs {
		uniqueImageUrls[url] = true
	}

	conversationID := int64(messageID)
	if item := currentContent.GetLatestConversationMessage(); item != nil {
		conversationID = item.ConversationID
	}

	for url := range uniqueImageUrls {
		if isURLValid(url) {
			currentContent.AddMedia(ai.Content{
				Type: "image_url",
				ImageURL: struct {
					URL string `json:"url"`
				}{
					URL: url,
				},
			})
		} else {
			delete(uniqueImageUrls, url)
		}
	}

	currentContent.Media = currentContent.FilterMedia(c.args.HandleImages, c.args.HandleAudio, c.args.HandleFiles)

	if c.args.Tools != "" {
		toolsList := []string{}
		if c.args.Tools != "all" {
			toolsList = strings.Split(c.args.Tools, ",")
			for i, item := range toolsList {
				toolsList[i] = strings.TrimSpace(item)
			}
		}
		currentContent.Tools = c.getTools(toolsList)
	}

	var latestMessageID int64
	if latestMessage != nil {
		latestMessageID = latestMessage.ID
	}
	// --- Save User Message to History ---
	userMessage := historyMessage
	if userMessage == nil {
		userMessage = NewUserConversationMessage(
			latestMessageID,
			chatID,
			messageID,
			historyStartMessageID,
			conversationID,
			userID,
			chainID.String(),
			currentContent.GetMessageContent(),
			attempt,
			currentContent.GetImagesMedia(),
			currentContent.GetFilesMedia(),
			currentContent.GetAudioMedia(),
			currentContent.GetProcessedURLs(),
			currentContent.HasHistory(),
			currentContent.Tools,
		)
	}
	userMessage.AttemptsCount = attempt
	userMessage, err = c.saveMessage(userMessage)
	if err != nil {
		c.Logger.WithError(err).Error("Failed to save user message to history")
		return err
	} else {
		c.Logger.WithFields(logger.Fields{
			"attempts": attempt,
		}).Info("Saved user message")
	}

	if c.Cfg.AI().UseMultimodalAuto && len(currentContent.GetAllMedia(c.cmdCfg, c.args)) > 0 && c.args.Model == "" && len(currentContent.Tools) == 0 {
		modelName := c.Cfg.AI().MultimodalModel
		multiModel, err := c.ai.GetFormattedModel(ctx, modelName, "")
		if err != nil {
			c.Logger.WithError(err).Error("Failed get multimodal model. Fallback to current chat model")
		} else {
			model = multiModel
		}
	}

	// --- Call AI ---
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	messages := c.buildPromptWithHistory(model, currentContent, c.args, false)
	logMessages := c.Logger.WithFields(logger.Fields{
		"urls":   currentContent.GetProcessedURLs(),
		"images": currentContent.ImageURLs,
	})
	if c.Cfg.Log().IsDebug() {
		messagesLog := prepareMessagesForLog(messages, 50, 0)
		logMessages.WithField("messages", messagesLog).Debug("Prepared messages for AI")
	} else {
		logMessages.WithField("messages", len(messages)).Info("Prepared messages for AI")
	}

	params := &ai.ModelParams{}
	if item := currentContent.GetLatestConversationMessage(); item != nil && item.Params != nil {
		params = item.Params
	}
	if temp := c.args.Temperature; temp != nil {
		params.Temperature = temp
	}
	if topp := c.args.TopP; topp != nil {
		params.TopP = topp
	}
	useStreamArg := c.args.Stream
	useStreamConf := c.Cfg.AI().UseStream
	useStream := useStreamConf
	if useStreamArg != nil {
		useStream = *useStreamArg
	} else if params.Stream != nil {
		useStream = *params.Stream
	}
	params.Stream = &useStream

	c.Logger.WithFields(logger.Fields{
		"model": model.FullName(),
	}).Info("Send request to AI")

	response := NewResponse()
	response.Context.SetSeparatedModelForTools(c.Cfg.AI().ToolsModel != "")
	var usageInfo *MetadataUsage

	usageInfo, params, err = c.handleRequest(
		ctx,
		userMessage,
		chatID,
		messages,
		currentContent,
		model,
		params,
		botMessageID,
		messageID,
		response,
		toolFromCallback,
	)
	if err != nil {
		return err
	}

	if usageInfo != nil {
		totalUsage.Add(usageInfo)
	}

	if c.Cfg.Log().IsDebug() {
		c.Logger.WithFields(logger.Fields{
			"content":   response.Content,
			"reasoning": response.Reasoning,
		}).Debug("AI response received")
	} else {
		c.Logger.WithFields(logger.Fields{
			"content_length":   len(response.Content),
			"reasoning_length": len(response.Reasoning),
		}).Info("AI response received")
	}
	// TODO: handle errors from response
	if !response.HasContent() && !response.HasReasoning() {
		text := c.L("ask.emptyReplyFromAI", nil)
		return c.handleErrorWithRetry(
			chatID,
			text,
			botMessageID,
			messageID,
			err,
			toolFromCallback,
		)
	}

	if !response.HasReasoning() {
		response.Content, response.Reasoning = ai.HandleContentReasoning(response.Content)
	}

	// --- Finalize and Save AI Response ---
	finalText := response.Content
	finalText = strings.TrimSpace(finalText)

	// Set content and reasoning
	if response.HasReasoning() {
		reasoningContent := strings.TrimSpace(response.Reasoning)
		reasoningContent = strings.ReplaceAll(strings.ReplaceAll(reasoningContent, "\r", ""), "\n", " ")
		if !response.HasContent() {
			finalText = reasoningContent
		} else {
			response.SetReasoning(reasoningContent)
		}
	}
	if dynamicPrompt != "" {
		response.SetPrompt(dynamicPrompt)
	}
	response.SetContent(finalText)

	// Add URLs to context
	if urls := currentContent.GetProcessedURLs(); len(urls) > 0 {
		for _, url := range urls {
			response.Context.AddURL(url)
		}
	}

	currentContent.Media = currentContent.GetAllMedia(c.cmdCfg, c.args)
	// Add media info to context
	if len(currentContent.Media) > 0 {
		for _, media := range currentContent.GetImagesMedia() {
			response.Context.AddImageURL(media.ImageURL.URL)
		}
		for _, media := range currentContent.GetFilesMedia() {
			response.Context.AddFile(media.File.Filename)
		}
		for _, media := range currentContent.GetAudioMedia() {
			response.Context.AddAudio(media.InputAudio.Format)
		}
	}

	// Add additional context messages
	if len(additionalContext) > 0 {
		maxMessages := 4
		if len(additionalContext) > maxMessages {
			for _, update := range additionalContext[:2] {
				msg := update.Message
				text := formatContextMessageText(msg)
				response.Context.AddAdditional(c.formatMessageLink(msg, text))
			}

			skipped := len(additionalContext) - 4
			response.Context.AddAdditional(c.Tg.EscapeText(fmt.Sprintf("\n...%d more...", skipped)))

			for _, update := range additionalContext[len(additionalContext)-2:] {
				msg := update.Message
				text := formatContextMessageText(msg)
				response.Context.AddAdditional(c.formatMessageLink(msg, text))
			}
		} else {
			for _, update := range additionalContext {
				msg := update.Message
				text := formatContextMessageText(msg)
				response.Context.AddAdditional(c.formatMessageLink(msg, text))
			}
		}
	}

	provider, _ := c.ai.GetProvider(model.Provider)
	currencyConfig := c.Cfg.Currency()
	// Set metadata
	response.Metadata = NewMetadata(
		model,
		provider,
		usageInfo,
		int(botMessageID),
		totalUsage,
		currentContent.ContextTurnsCount(),
		continueChainID,
		getChatID(msg),
		conversationID,
		params,
		&currencyConfig,
		c.Localizer,
	)

	// Build final message using builder
	builder := NewMessageBuilder(c.Tg, c.Localizer).
		SetResponse(response).
		WithMetadata(c.cmdCfg.Display.Metadata).
		WithContext(c.cmdCfg.Display.Context).
		WithReasoning(c.cmdCfg.Display.Reasoning).
		SetSeparator(c.cmdCfg.Display.Separator)

	if reasoning := c.args.Reasoning; reasoning != nil {
		builder.config.ShowReasoning = *reasoning
	}

	var replyMarkup *telegram.InlineKeyboardMarkup
	toolsPattern := regexp.MustCompile(`\**(\w+)(\@\d)*\**\s*(\{[^}]*\})`)
	matches := toolsPattern.FindAllStringSubmatch(finalText, -1)
	buttonRows := [][]telegram.InlineKeyboardButton{}
	toolCallNumber := 0
	for _, match := range matches {
		if len(match) >= 4 {
			functionName := match[1]
			toolNumber := match[2]
			functionCallName := functionName
			if toolNumber != "" {
				functionCallName = functionName + toolNumber
			}
			c.Logger.Debug("Tool name " + functionName)
			if slices.Contains(tools.ToolNames(c.cmdCfg.Tools.Allowed, c.cmdCfg.Tools.Excluded), functionName) {
				toolCallNumber++
				button := telegram.NewInlineKeyboardButtonData(
					fmt.Sprintf(ai.Tools+" Run %s", functionCallName),
					fmt.Sprintf("ask %s $tools $id:%d", functionCallName, botMessageID),
				)
				if len(buttonRows) == 0 || len(buttonRows[len(buttonRows)-1]) == 2 {
					buttonRows = append(buttonRows, []telegram.InlineKeyboardButton{})
				}
				buttonRows[len(buttonRows)-1] = append(buttonRows[len(buttonRows)-1], button)
			}
		}
	}

	if toolCallNumber > 0 {
		toolsButton := []telegram.InlineKeyboardButton{
			telegram.NewInlineKeyboardButtonData(
				ai.Tools+" Run all tools",
				fmt.Sprintf("ask all $tools $id:%d", botMessageID),
			),
		}
		if toolCallNumber == 1 {
			buttonRows = [][]telegram.InlineKeyboardButton{}
		}
		buttonRows = append(buttonRows, toolsButton)
		replyMarkup = &telegram.InlineKeyboardMarkup{
			InlineKeyboard: buttonRows,
		}
	}

	finalMessageEscaped := builder.Build()
	c.Logger.WithField("text", finalMessageEscaped).Trace("Escaped final message")
	textForSend := finalMessageEscaped
	firstAttempt := telegram.NewEditMessageText(
		chatID,
		botMessageID,
		textForSend,
	)
	firstAttempt.ParseMode = telegram.ModeMarkdownV2
	firstAttempt.LinkPreviewDisabled = true

	secondAttempt := telegram.NewEditMessageText(
		chatID,
		botMessageID,
		textForSend,
	)
	secondAttempt.LinkPreviewDisabled = true

	if replyMarkup != nil {
		firstAttempt.ReplyMarkup = replyMarkup
		secondAttempt.ReplyMarkup = replyMarkup
	}

	if _, err = c.Tg.SendWithRetry(&firstAttempt, 0); err != nil {
		c.Logger.WithError(err).WithFields(logger.Fields{
			"full_answer":  firstAttempt.Text,
			"is_reasoning": response.HasReasoning(),
		}).Error("Failed to send final message (1/2)")
		if _, err = c.Tg.SendWithRetry(&secondAttempt, 0); err != nil {
			c.Logger.WithError(err).WithFields(logger.Fields{
				"full_answer":  secondAttempt.Text,
				"is_reasoning": response.HasReasoning(),
			}).Error("Failed to send final message (2/2)")
			return c.handleErrorWithRetry(chatID, "", botMessageID, messageID, err, toolFromCallback)
		}
	}

	if !currentContent.HasHistory() {
		title := c.L("ask.emptyConversationTitle", nil)
		source := "initial"
		if text := currentContent.GetTextForTitleGenerating(); text != "" {
			title, source = c.generateConversationTitle(ctx, text, chatID)
		}

		err := c.updateConversationTitle(title, source, chatID, conversationID)
		if err != nil {
			c.Logger.WithError(err).Error("Update conversation title failed")
		} else {
			c.Logger.WithFields(logger.Fields{
				"title":           title,
				"source":          source,
				"conversation_id": conversationID,
			}).Info("Conversation title updated")
		}
	}

	return nil
}

func (c *Command) updateConversationTitle(title, source string, chatID, conversationID int64) error {
	query := `UPDATE conversation_history set conversation_title = ?, conversation_title_source = ?
	where chat_id = ? and message_id = ?`

	_, err := c.db.Exec(query, title, source, chatID, conversationID)
	return err
}

func (c *Command) updateConversationSummary(summary string, chatID, conversationID int64) error {
	query := `UPDATE conversation_history set conversation_summary = ?
	where chat_id = ? and conversation_id = ? and is_first = 1`

	_, err := c.db.Exec(query, summary, chatID, conversationID)
	return err
}

func (c *Command) updateConversationMessageAttempts(id int64, attempts uint8) error {
	query := `UPDATE conversation_history set attempts_count = ? where id = ?`

	_, err := c.db.Exec(query, attempts, id)
	return err
}

func (c *Command) saveMessage(
	msg *conversationMessage,
) (*conversationMessage, error) {
	var err error
	if msg.ID != 0 {
		err = c.updateConversationMessageAttempts(msg.ID, msg.AttemptsCount)
		if err != nil {
			c.Logger.WithError(err).Error("Update message attempts count failed")
		}
		return msg, nil
	}
	var query string
	paramsJSON := []byte("{}")
	if msg.Params != nil {
		paramsJSON, err = json.Marshal(msg.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	imagesJSON := []byte("[]")
	if len(msg.Images) > 0 {
		imagesJSON, err = json.Marshal(msg.Images)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal images: %w", err)
		}
	}

	audioJSON := []byte("[]")
	if len(msg.Audio) > 0 {
		audioJSON, err = json.Marshal(msg.Audio)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal audio: %w", err)
		}
	}

	filesJSON := []byte("[]")
	if len(msg.Files) > 0 {
		filesJSON, err = json.Marshal(msg.Files)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal files: %w", err)
		}
	}

	urlsJSON := []byte("[]")
	if len(msg.URLs) > 0 {
		urlsJSON, err = json.Marshal(msg.URLs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal urls: %w", err)
		}
	}
	annotationsJSON := []byte("[]")
	if len(msg.Annotations) > 0 {
		annotationsJSON, err = json.Marshal(msg.Annotations)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal annotations: %w", err)
		}
	}
	toolCallsJSON := []byte("[]")
	if len(msg.ToolCalls) > 0 {
		toolCallsJSON, err = json.Marshal(msg.ToolCalls)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool calls: %w", err)
		}
	}
	toolResponsesJSON := []byte("[]")
	if len(msg.ToolResponses) > 0 {
		toolResponsesJSON, err = json.Marshal(msg.ToolResponses)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool responses: %w", err)
		}
	}
	toolParamsJSON := []byte("[]")
	if len(msg.ToolParams) > 0 {
		toolParamsJSON, err = json.Marshal(msg.ToolParams)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool responses: %w", err)
		}
	}
	if msg.AttemptsCount == 0 {
		msg.AttemptsCount = 1
	}

	var insertedID int64
	if msg.Usage == nil {
		query = `INSERT INTO conversation_history (parent_message_id, conversation_chain_id, chat_id, message_id, reply_to_message_id, user_id, role, text, is_first, conversation_id, attempts_count, params, images, files, audio, urls, annotations, tool_calls, tool_responses, tool_name, tool_params)
				  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				  RETURNING id`
		err = c.db.QueryRow(
			query,
			msg.ParentMessageID,
			msg.ConversationChainID,
			msg.ChatID,
			msg.MessageID,
			msg.ReplyToMessageID,
			msg.UserID,
			msg.Role,
			msg.Text,
			msg.IsFirst,
			msg.ConversationID,
			msg.AttemptsCount,
			paramsJSON,
			imagesJSON,
			filesJSON,
			audioJSON,
			urlsJSON,
			annotationsJSON,
			toolCallsJSON,
			toolResponsesJSON,
			msg.ToolName,
			toolParamsJSON,
		).Scan(&insertedID)
	} else {
		query = `INSERT INTO conversation_history (parent_message_id, conversation_chain_id, chat_id, message_id, reply_to_message_id, user_id, role, text, is_first, conversation_id, total_tokens, completion_tokens, prompt_tokens, total_cost, model_name, attempts_count, params, images, files, audio, urls, annotations, tool_calls, tool_responses, tool_name, tool_params)
				  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				  RETURNING id`
		err = c.db.QueryRow(
			query,
			msg.ParentMessageID,
			msg.ConversationChainID,
			msg.ChatID,
			msg.MessageID,
			msg.ReplyToMessageID,
			msg.UserID,
			msg.Role,
			msg.Text,
			msg.IsFirst,
			msg.ConversationID,
			msg.Usage.Total,
			msg.Usage.Output,
			msg.Usage.Input,
			msg.Usage.Cost,
			msg.ModelName,
			msg.AttemptsCount,
			paramsJSON,
			imagesJSON,
			filesJSON,
			audioJSON,
			urlsJSON,
			annotationsJSON,
			toolCallsJSON,
			toolResponsesJSON,
			msg.ToolName,
			toolParamsJSON,
		).Scan(&insertedID)
	}
	if err != nil {
		c.Logger.WithError(err).WithFields(logger.Fields{
			"chat_id":             msg.ChatID,
			"message_id":          msg.MessageID,
			"reply_to_message_id": msg.ReplyToMessageID,
			"role":                msg.Role,
		}).Error("Failed to save message to conversation history")
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	msg.ID = insertedID
	return msg, nil
}

func (c *Command) getConversationHistory(chatID int64, startMessageID int) ([]conversationMessage, error) {
	history := []conversationMessage{}
	visited := make(map[int]struct{}) // To prevent infinite loops in case of weird reply chains

	if startMessageID == 0 {
		return history, nil
	}

	// count context turns without current
	maxContextTurns := c.cmdCfg.MaxContextTurns - 1

	// Get the full conversation thread starting from the replied message
	currentMessageID := startMessageID
	uniqueChains := make(map[string]struct{}, maxContextTurns)
	for len(uniqueChains) < maxContextTurns && currentMessageID != 0 {
		if _, exists := visited[currentMessageID]; exists {
			break
		}
		visited[currentMessageID] = struct{}{}
		messages, err := c.getMessagesFromHistoryByID(chatID, currentMessageID)
		if err != nil || len(messages) == 0 {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, err
		}

		history = append(history, messages...)
		for _, msg := range messages {
			uniqueChains[msg.ConversationChainID] = struct{}{}
		}

		// Move to the message this one replied to
		lastMessage := messages[len(messages)-1]
		if lastMessage.ReplyToMessageID.Valid {
			currentMessageID = int(lastMessage.ReplyToMessageID.Int64)
		} else {
			currentMessageID = 0
		}
	}

	c.Logger.WithFields(logger.Fields{
		"chat_id":          chatID,
		"start_message_id": startMessageID,
		"history_length":   len(history),
		"history":          history,
	}).Trace("Fetched conversation history")

	return history, nil
}

func (c *Command) getLatestMessageFromHistory(chatID int64) (*conversationMessage, error) {
	query := `SELECT id, chat_id, parent_message_id, conversation_chain_id, message_id, reply_to_message_id, role, text, conversation_id, is_first, created_at, model_name, 
              prompt_tokens, completion_tokens, total_tokens, total_cost, attempts_count,
              params, images, files, audio, urls, annotations, tool_calls, tool_responses, tool_name, tool_params
              FROM conversation_history
              WHERE chat_id = ?
              ORDER BY id DESC LIMIT 1`

	row := c.db.QueryRow(query, chatID)
	return c.mapHistoryMessageToStruct(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func (c *Command) mapHistoryMessageToStruct(row scanner) (*conversationMessage, error) {
	var paramsJSON, imagesJSON, filesJSON, audioJSON, urlsJSON, annotationsJSON, toolCallsJSON, toolResponsesJSON, toolParamsJSON []byte
	var msg conversationMessage
	var usageInput, usageOutput, usageTotal sql.NullInt64
	var usageCost sql.NullFloat64
	err := row.Scan(
		&msg.ID,
		&msg.ChatID,
		&msg.ParentMessageID,
		&msg.ConversationChainID,
		&msg.MessageID,
		&msg.ReplyToMessageID,
		&msg.Role,
		&msg.Text,
		&msg.ConversationID,
		&msg.IsFirst,
		&msg.CreatedAt,
		&msg.ModelName,
		&usageInput,
		&usageOutput,
		&usageTotal,
		&usageCost,
		&msg.AttemptsCount,
		&paramsJSON,
		&imagesJSON,
		&filesJSON,
		&audioJSON,
		&urlsJSON,
		&annotationsJSON,
		&toolCallsJSON,
		&toolResponsesJSON,
		&msg.ToolName,
		&toolParamsJSON,
	)
	if err != nil {
		return nil, err
	}

	msg.Usage = &MetadataUsage{
		Input:  usageInput.Int64,
		Output: usageOutput.Int64,
		Total:  usageTotal.Int64,
		Cost:   usageCost.Float64,
	}

	if len(paramsJSON) > 0 {
		json.Unmarshal(paramsJSON, &msg.Params)
	}
	if len(imagesJSON) > 0 {
		json.Unmarshal(imagesJSON, &msg.Images)
	}
	if len(filesJSON) > 0 {
		json.Unmarshal(filesJSON, &msg.Files)
	}
	if len(audioJSON) > 0 {
		json.Unmarshal(audioJSON, &msg.Audio)
	}
	if len(urlsJSON) > 0 {
		json.Unmarshal(urlsJSON, &msg.URLs)
	}
	if len(annotationsJSON) > 0 {
		json.Unmarshal(annotationsJSON, &msg.Annotations)
	}
	if len(toolCallsJSON) > 0 {
		json.Unmarshal(toolCallsJSON, &msg.ToolCalls)
	}
	if len(toolResponsesJSON) > 0 {
		json.Unmarshal(toolResponsesJSON, &msg.ToolResponses)
	}
	if len(toolParamsJSON) > 0 {
		json.Unmarshal(toolParamsJSON, &msg.ToolParams)
	}

	return &msg, nil
}

func (c *Command) getMessageFromHistory(chatID, messageID int64) (*conversationMessage, error) {
	query := `SELECT id, chat_id, parent_message_id, conversation_chain_id, message_id, reply_to_message_id, role, text, conversation_id, is_first, created_at, model_name, 
              prompt_tokens, completion_tokens, total_tokens, total_cost, attempts_count,
              params, images, files, audio, urls, annotations, tool_calls, tool_responses, tool_name, tool_params
              FROM conversation_history
              WHERE chat_id = ? AND message_id = ?
              ORDER BY id DESC LIMIT 1`

	row := c.db.QueryRow(query, chatID, messageID)
	return c.mapHistoryMessageToStruct(row)
}

func (c *Command) getMessagesFromHistoryByID(chatID int64, messageID int) ([]conversationMessage, error) {
	query := `SELECT id, chat_id, parent_message_id, conversation_chain_id, message_id, reply_to_message_id, role, text, conversation_id, is_first, created_at, model_name, 
              prompt_tokens, completion_tokens, total_tokens, total_cost, attempts_count,
              params, images, files, audio, urls, annotations, tool_calls, tool_responses, tool_name, tool_params
              FROM conversation_history
              WHERE chat_id = ? AND message_id = ?
              ORDER BY id desc`

	messages := []conversationMessage{}
	rows, err := c.db.Query(query, chatID, messageID)
	if err != nil {
		return messages, err
	}
	defer rows.Close()
	for rows.Next() {
		message, err := c.mapHistoryMessageToStruct(rows)
		if err != nil {
			c.Logger.WithError(err).WithFields(logger.Fields{
				"chat_id":    chatID,
				"message_id": messageID,
			}).Error("Failed to scan message from conversation history")
			return nil, err
		}

		messages = append(messages, *message)
	}
	return messages, nil
}

// --- Modified Prompt Builder ---
func (c *Command) buildPromptWithHistory(model *ai.ModelInfo, currentContent *MessageContent, args *CommandArgs, withoutUserMessage bool) []ai.Message {
	var messages []ai.Message
	history := currentContent.ConversationHistory
	provider, _ := c.ai.GetProvider(model.Provider)
	_, isOpenrouter := provider.(*ai.OpenRouterClient)

	now := time.Now()
	dateStr := now.Format("Monday, 02 January 2006")
	timeStr := now.Format("15:04")

	defaultSystemInstructions := `
[User request message format]
[USER: Name(ID) @MonDD HH:MM]
Text here
Technical notes:
1. NEVER include these technical markers (like [USER:] or [REPLY TO:]) in your responses
2. When you see [NO TEXT] marker, it means the user didn't provide any text
3. [forwarded from] indicates third-party content - maintain original context
4. IDs in parentheses are for tracking only - never mention them
5. Keep responses under 4000 characters (Telegram limit)
6. Use tools with parameters in English`

	if c.cmdCfg.Tools.Enabled && len(currentContent.Tools) == 0 && len(tools.AvailableTools(c.cmdCfg.Tools.Allowed, c.cmdCfg.Tools.Excluded)) > 0 {
		runToolsInstruction := ""
		if !c.cmdCfg.Tools.AutoRun {
			runToolsInstruction = `
How to activate tools:
1. Reply to this message with "$tools" or "/tools"
2. OR use the activation button below

Tool format:
**name@N** {parameters} (one-line JSON)

Critical tool format rules:
1. Place activation instructions before or after tools
2. Show complete tool syntax
3. Always number tools when multiple tools are provided, number must be part of the tool name (e.g., name@1)
4. Always include both text and button options
5. Tools always in correct format (one-line without json tags)
`
		}

		defaultSystemInstructions += fmt.Sprintf(`
[Tool Integration Protocol]
Available functions:
%s
Tools key principles:
1. I'll suggest tools (single or multiple) ONLY when strictly necessary and clearly beneficial
2. Priority given to text responses when sufficient
3. Strict "rare but precise" policy
4. Execution requires your explicit approval

Valid triggers:
- Question requires live/current data
- Facts are outside my training cutoff
- Clear user instruction
%s
Format examples:
• Friendly style:
"I can help with both! To activate, please:
- Reply to this with $tools
- OR tap the button below

**weather** {"location":"Tokyo","days":1}
• Professional style:
"Data retrieval prepared. To execute:
- Reply with /tools
- OR use the activation control

**search@1** {"query":"market trends 2025","max_results":5}"
**fetch_url@2** {"url":"https://reports.example.com/Q3"}"

Critical format rules:
1. Maintain conversation style`, tools.AvailableToolsText(c.cmdCfg.Tools.Allowed, c.cmdCfg.Tools.Excluded), runToolsInstruction)
	}

	systemInstructions := `You are Gachigazer⭐, a Telegram AI assistant. Current date: {{date}}, time: {{time}}.
You MUST follow the Markdown rules. STRICTLY RESPOND IN: {{language}}. NEVER switch to other languages regardless of the input language.`

	if system := c.Cfg.AI().SystemPrompt; system != "" {
		systemInstructions = system
	}
	if extra := c.Cfg.AI().ExtraSystemPrompt; extra != "" {
		systemInstructions += " " + extra
	}
	systemInstructions += defaultSystemInstructions

	systemInstructions = strings.TrimSpace(systemInstructions)
	systemInstructions = strings.ReplaceAll(systemInstructions, "{{date}}", dateStr)
	systemInstructions = strings.ReplaceAll(systemInstructions, "{{time}}", timeStr)
	systemInstructions = strings.ReplaceAll(systemInstructions, "{{language}}", c.Cfg.AI().Language)

	systemMessage := ai.Message{
		Role: ai.RoleSystem,
	}
	// system instructions
	if model.IsMultimodal() {
		systemMessage.Content = []ai.Content{
			{
				Type: "text",
				Text: systemInstructions,
			},
		}
	} else {
		systemMessage.Text = systemInstructions
	}
	messages = append(messages, systemMessage)

	maxImages := c.cmdCfg.Images.Max
	maxAudio := c.cmdCfg.Audio.MaxInHistory
	allowedImagesCount := maxImages - len(currentContent.GetImagesMedia())
	allowedAudioCount := maxAudio - len(currentContent.GetAudioMedia())

	// history
	imagesInHistoryCount := 0
	audioInHistoryCount := 0
	historyMessages := []ai.Message{}
	imageLifetime := c.cmdCfg.Images.Lifetime
	for _, msg := range history {
		if !msg.Role.Supported() {
			c.Logger.WithField("role", msg.Role).Warn("Unsupported role")
			continue
		}
		if msg.Role.IsTool() && !model.SupportsTools() {
			// if the model does not support tools, then we add the output of the tool from the user
			if len(msg.ToolResponses) > 0 && msg.ToolName.Valid {
				toolResponses := []ai.Message{}
				for _, item := range msg.ToolResponses {
					item.Text = fmt.Sprintf("[Tool %s]\n%s", msg.ToolName.String, item.Text)
					item.Role = ai.RoleUser
					item.ToolCallID = ""
					toolResponses = append(toolResponses, item)
				}
				historyMessages = append(historyMessages, toolResponses...)
			}
			continue
		}
		if msg.Role.IsAssistant() && len(msg.ToolCalls) > 0 && !model.SupportsTools() {
			continue
		}
		message := ai.Message{Role: string(msg.Role)}
		if model.SupportsTools() {
			if msg.Role.IsTool() {
				if len(msg.ToolResponses) > 0 && model.SupportsTools() {
					historyMessages = append(historyMessages, msg.ToolResponses...)
				}
				continue
			}

			if len(msg.ToolCalls) > 0 && model.SupportsTools() {
				message.ToolCalls = msg.ToolCalls
				historyMessages = append(historyMessages, message)
				continue
			}
		}
		if model.IsMultimodal() {
			contentList := []ai.Content{}
			content := ai.Content{
				Type: "text",
				Text: msg.Text,
			}
			if len(msg.Annotations) > 0 {
				content.Annotations = msg.Annotations
			}
			contentList = append(contentList, content)

			if args.HandleImages && model.SupportsImageRecognition() && (imageLifetime == 0 || now.Before(msg.CreatedAt.Add(imageLifetime))) {
				if len(msg.Images) > 0 && model.IsMultimodal() && imagesInHistoryCount <= allowedImagesCount {
					for _, image := range msg.Images {
						contentList = append(contentList, image)
						imagesInHistoryCount++
						if imagesInHistoryCount == allowedImagesCount {
							break
						}
					}
				}
			}

			if args.HandleFiles && model.SupportsFiles() && len(msg.Files) > 0 {
				contentList = append(contentList, msg.Files...)
			}
			if args.HandleAudio && model.SupportsAudioRecognition() && len(msg.Audio) > 0 && audioInHistoryCount < allowedAudioCount {
				contentList = append(contentList, msg.Audio...)
				audioInHistoryCount += len(msg.Audio)
			}

			message.Content = contentList
		} else {
			message.Text = msg.Text
		}
		historyMessages = append(historyMessages, message)
	}

	slices.Reverse(historyMessages)
	messages = append(messages, historyMessages...)

	if withoutUserMessage {
		return messages
	}
	// user message
	userMessage := ai.Message{
		Role:    ai.RoleUser,
		Content: []ai.Content{},
	}

	finalContent := currentContent.GetMessageContent()
	if finalContent != "" {
		if model.IsMultimodal() {
			userMessage.Content = append(userMessage.Content, ai.Content{
				Type: "text",
				Text: finalContent,
			})
		} else {
			userMessage.Text = finalContent
		}
	}
	imagesCount := 0
	if model.SupportsImageRecognition() && len(currentContent.GetImagesMedia()) > 0 {
		for _, image := range currentContent.GetImagesMedia() {
			userMessage.Content = append(userMessage.Content, image)
			imagesCount++
			if imagesCount == (maxImages - imagesInHistoryCount) {
				break
			}
		}
	}
	if model.SupportsAudioRecognition() && len(currentContent.GetAudioMedia()) > 0 {
		audioItems := []ai.Content{}
		for _, media := range currentContent.GetAudioMedia() {
			switch strings.ToLower(media.InputAudio.Format) {
			case "audio/mpeg":
				media.InputAudio.Format = "mp3"
			case "audio/wav", "audio/x-wav":
				media.InputAudio.Format = "wav"
			case "audio/ogg", "audio/opus":
				audioBytes, _ := base64.StdEncoding.DecodeString(media.InputAudio.Data)
				var mp3Data []byte
				mp3Data, err := service.ConvertOggToMP3(audioBytes)
				if err != nil {
					c.Logger.WithError(err).Error("error convert audio to mp3")
					continue
				}
				media.InputAudio.Format = "mp3"
				media.InputAudio.Data = base64.StdEncoding.EncodeToString(mp3Data)
			default:
				c.Logger.Error("unsupported audio format: " + media.InputAudio.Format)
				continue
			}
			audioItems = append(audioItems, media)
		}
		userMessage.Content = append(userMessage.Content, audioItems...)
	}
	if (model.SupportsFiles() || isOpenrouter) && len(currentContent.GetFilesMedia()) > 0 {
		userMessage.Content = append(userMessage.Content, currentContent.GetFilesMedia()...)
	}

	messages = append(messages, userMessage)

	return messages
}

func parseArgs(text string) (map[string]string, string) {
	args := make(map[string]string)

	// Regular expression for arguments in $key:value format
	reWithValue := regexp.MustCompile(`(?:^|\s)\$([a-zA-Z]+):([^\s]+)`)
	// Regular expression for flags in $key format
	reFlag := regexp.MustCompile(`(?:^|\s)\$([a-zA-Z]+)\b`)

	matches := reWithValue.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			key := strings.ToLower(match[1])
			value := match[2]
			if value == "n" {
				value = "no"
			}
			if value == "y" {
				value = "yes"
			}
			args[key] = value
		}
	}
	text = reWithValue.ReplaceAllString(text, "")

	matches = reFlag.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			key := strings.ToLower(match[1])
			args[key] = "yes"
		}
	}
	text = reFlag.ReplaceAllString(text, "")

	text = strings.TrimSpace(text)
	text = strings.Join(strings.Fields(text), " ")

	return args, text
}

func cleanText(text string) string {
	return strings.TrimSpace(strings.ToValidUTF8(text, ""))
}

func isURLValid(url string) bool {
	resp, err := http.Head(url)
	if err != nil {
		return false
	}
	return resp.StatusCode == http.StatusOK
}

func (c *Command) generateArgumentsHelpText() string {
	var help strings.Builder
	help.WriteString("📚 *Available arguments:*\n\n")

	for _, arg := range c.supportedArgs {
		fmt.Fprintf(&help, "➤ *$%s:* %s\n", markdown.Escape(arg.Name), markdown.Escape(arg.Description))
		fmt.Fprintf(&help, "  └ Type: `%s`\n", arg.Type)

		if arg.Type == "int" || arg.Type == "float" {
			var constraints []string
			if arg.Min != nil {
				constraints = append(constraints, markdown.Escape(fmt.Sprintf("min: %.1f", *arg.Min)))
			}
			if arg.Max != nil {
				constraints = append(constraints, markdown.Escape(fmt.Sprintf("max: %.1f", *arg.Max)))
			}
			if len(constraints) > 0 {
				fmt.Fprintf(&help, "  └ Restrictions: %s\n", strings.Join(constraints, ", "))
			}
		}

		if len(arg.Values) > 0 {
			vals := make([]string, 0, len(arg.Values))
			for _, v := range arg.Values {
				vals = append(vals, markdown.Escape(v))
			}
			fmt.Fprintf(&help, "  └ Valid values: %s\n", strings.Join(vals, ", "))
		}

		if arg.DefaultValue != "" {
			fmt.Fprintf(&help, "  └ By default: `%s`\n", markdown.Escape(arg.DefaultValue))
		}
	}

	help.WriteString("\nExample usage:\n`/a $think $i:no $temp:1.5 Explain quantum physics`")
	return help.String()
}

func parseAdditionalContextArg(contextStr string) (count int, duration time.Duration, username string, messageID string, err error) {
	atPos := strings.Index(contextStr, "@")

	if strings.HasPrefix(contextStr, "id") && len(contextStr) > 2 {
		messageID = contextStr[2:]
		return 0, 0, "", messageID, nil
	}

	if atPos > 0 {
		username = contextStr[atPos+1:]
		contextStr = contextStr[:atPos]
	}

	if num, err := strconv.Atoi(contextStr); err == nil {
		count = num
		return count, 0, username, "", nil
	}

	if dur, err := time.ParseDuration(contextStr); err == nil {
		return 0, dur, username, "", nil
	}

	return 0, 0, "", "", fmt.Errorf("invalid context format: expected a number, duration (e.g. 5 or 5m) or id (e.g. id123)")
}

func (c *Command) handleURLs(currentContent *MessageContent, chatID int64, recursive bool) (*MessageContent, error) {
	c.Logger.WithFields(logger.Fields{
		"chat_id": chatID,
	}).Info("Handling URLs in message content")

	var wg sync.WaitGroup
	var mu sync.Mutex
	urlsToProcess := make([]string, 0)

	for url, state := range currentContent.URLs {
		urlInHistory := false
		for _, msg := range currentContent.ConversationHistory {
			if strings.Contains(msg.Text, url) {
				c.Logger.WithFields(logger.Fields{
					"chat_id": chatID,
					"url":     url,
				}).Debug("URL already in conversation history, skipping fetch")
				delete(currentContent.URLs, url)
				urlInHistory = true
				break
			}
		}

		if !urlInHistory {
			urlsToProcess = append(urlsToProcess, url)
			state.MarkProcessing()
		}
	}
	for _, url := range urlsToProcess {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			c.Logger.WithFields(logger.Fields{
				"chat_id": chatID,
				"url":     url,
			}).Debug("Fetching URL content")
			content, _ := c.fetcher.Fetch(fetch.MustNewRequestPayload(url, nil, nil))

			mu.Lock()
			defer mu.Unlock()

			state := currentContent.URLs[url]
			if content.IsError {
				c.Logger.WithField("content", content.Content).Error("Fail fetch URL")
				state.MarkFailed(content.GetText())
			} else {
				c.Logger.WithFields(logger.Fields{
					"chat_id": chatID,
					"url":     url,
				}).Debug("Fetched URL content successfully")
				state.MarkProcessed()
			}
			text := content.GetText()
			if utf8.RuneCountInString(text) > 500 {
				state.TrimmedContent = string([]rune(text)[:500]) +
					fmt.Sprintf(
						"... [%s]",
						c.L("ask.response.truncated", nil),
					)
			} else {
				state.TrimmedContent = text
			}
			maxLength := c.cmdCfg.Fetcher.MaxLength
			if maxLength != 0 && utf8.RuneCountInString(content.Content[0].Text) > maxLength {
				content.Content[0].Text = string([]rune(content.Content[0].Text)[:maxLength]) + "...[truncated]"
			}
			if strings.Contains(url, "t.me") || strings.Contains(url, "reddit.com") || strings.Contains(url, "habr") {
				c.extractImageURLs(content.Content, currentContent)
			}
			// Mark URL as handled
			currentContent.URLsContent[url] = content.GetText()
			if recursive {
				if strings.Contains(url, "t.me") || strings.Contains(url, "reddit.com") || strings.Contains(url, "habr") {
					urls := fetch.ExtractStrictURLs(content.Content[0].Text)
					urls, _, _ = c.filterURLs(urls)
					currentContent.AddURLs(urls...)
				}
			}

			// handle important urls
			URLs := content.GetURLs()
			if len(URLs) > 0 {
				urls, _, _ := c.filterURLs(URLs)
				currentContent.AddURLs(urls...)
			}

			images := content.GetImages()
			if len(images) > 0 {
				for _, imgURL := range images {
					currentContent.AddImageURLs(imgURL)
				}
			}
		}(url)
	}

	wg.Wait()

	if recursive {
		var newURLs []string
		for url, state := range currentContent.URLs {
			if !state.IsProcessed() {
				newURLs = append(newURLs, url)
			}
		}
		if len(newURLs) > 0 {
			currentContent, _ = c.handleURLs(currentContent, chatID, false)
		}
	}

	return currentContent, nil
}

func (c *Command) extractImageURLs(content []fetch.Content, currentContent *MessageContent) {
	var urls []string
	for _, c := range content {
		if strings.Contains(c.Text, "Image:") || strings.Contains(c.Text, "Video preview:") {
			lines := strings.SplitSeq(c.Text, "\n")
			for line := range lines {
				if strings.HasPrefix(line, "Image:") || strings.HasPrefix(line, "Video preview:") {
					prefix := "Image:"
					if strings.HasPrefix(line, "Video preview:") {
						prefix = "Video preview:"
					}
					imgURL := strings.TrimSpace(strings.TrimPrefix(line, prefix))
					urls = append(urls, imgURL)
				}
			}
		}
	}

	_, urls, _ = c.filterURLs(urls)
	currentContent.ImageURLs = append(currentContent.ImageURLs, urls...)
}

func getChatID(msg *telegram.MessageOriginal) int64 {
	if msg.Chat.IsSuperGroup() {
		chatID := -msg.Chat.ID
		chatID %= 10000000000
		return chatID
	}
	return msg.Chat.ID
}

func formatContextMessageText(msg *telegram.MessageOriginal) string {
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}

	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", "")

	switch {
	case msg.Photo != nil:
		text = "📷 " + text
	case msg.Video != nil:
		text = "📹 " + text
	case msg.Audio != nil:
		text = "🎵 " + text
	case msg.Document != nil:
		text = "📄 " + text
	case msg.Voice != nil:
		text = "🎤 " + text
	case msg.Sticker != nil:
		text = "🖼️ " + text
	case msg.Location != nil:
		text = "📍 " + text
	case msg.Contact != nil:
		text = "👤 " + text
	case msg.Poll != nil:
		text = "🗳️ " + text
	}

	if msg.ForwardOrigin != nil {
		text = "↪️ " + text
	}

	if utf8.RuneCountInString(text) > 25 {
		text = string([]rune(text)[:22]) + "..."
	}
	return text
}

func (c *Command) formatMessageLink(msg *telegram.MessageOriginal, text string) string {
	return fmt.Sprintf(
		"\n%s: [%s](https://t.me/c/%d/%d)",
		msg.From.FirstName,
		c.Tg.EscapeText(text),
		getChatID(msg),
		msg.MessageID,
	)
}

func prepareMessagesForLog(messages []ai.Message, maxURLLen, maxTextLen int) []ai.Message {
	messagesCopy := make([]ai.Message, len(messages))
	for i, msg := range messages {
		messagesCopy[i] = ai.Message{
			Role:    msg.Role,
			Content: msg.Content,
			Text:    msg.Text,
		}

		if len(msg.Content) > 0 {
			messagesCopy[i].Content = make([]ai.Content, len(msg.Content))
			for j, content := range msg.Content {
				messagesCopy[i].Content[j] = content

				if content.Type == "image_url" && len(content.ImageURL.URL) > maxURLLen {
					messagesCopy[i].Content[j].ImageURL.URL = content.ImageURL.URL[:maxURLLen] + "..."
				}

				if content.Type == "file" && len(content.File.FileData) > maxURLLen {
					messagesCopy[i].Content[j].File.FileData = content.File.FileData[:maxURLLen] + "..."
				}

				if content.Type == "text" && maxTextLen > 0 && len(content.Text) > maxTextLen {
					messagesCopy[i].Content[j].Text = content.Text[:maxTextLen] + "..."
				}
			}
		}
		if len(msg.Text) > 0 {
			// for not multimodal mode, e.g. deepseek
			if maxTextLen > 0 && len(msg.Text) > maxTextLen {
				messagesCopy[i].Text = msg.Text[:maxTextLen] + "..."
			}
		}
	}
	return messagesCopy
}

// encoding user ID for privacy
// this can be done via a field in the database when adding a user
func (c *Command) getUserPublicID(userID int64) string {
	user, err := c.db.GetUser(userID)
	if err != nil {
		return ""
	}

	return user.PublicID
}

func (c *Command) Ask(
	ctx context.Context,
	messages []ai.Message,
	tools []ai.Tool,
	model *ai.ModelInfo,
	promptName string,
	chatID int64,
	webSearch bool,
	params ai.ModelParams,
) (
	content string,
	reasoning string,
	requestedTools []ai.ToolCall,
	response *ai.CompletionResponse,
	usage *ai.ModelUsage,
	annotations []ai.AnnotationContent,
	finalParams *ai.ModelParams,
	err error,
) {
	content, reasoning, response, _, finalParams, err = c.ai.Ask(ctx, messages, tools, model, promptName, chatID, webSearch, params)
	if response != nil {
		annotations = response.Annotations
		usage = &response.Usage
		requestedTools = response.Choices[0].Message.ToolCalls
	}
	return
}

func (c *Command) AskStream(
	ctx context.Context,
	messages []ai.Message,
	tools []ai.Tool,
	model *ai.ModelInfo,
	promptName string,
	chatID int64,
	webSearch bool,
	params ai.ModelParams,
	sentMsgID int,
) (content string, reasoning string, requestedTools []ai.ToolCall, usage *ai.ModelUsage, annotations []ai.AnnotationContent, finalParams *ai.ModelParams, err error) {
	stream, _, finalParams, err := c.ai.AskStream(ctx, messages, tools, model, promptName, chatID, webSearch, params)
	if err != nil {
		return "", "", nil, nil, nil, nil, err
	}

	var (
		fullResponse             strings.Builder
		reasoningBuffer          strings.Builder
		lastUpdateReasoning      = time.Now()
		lastUpdate               = time.Now()
		hasContent               = false
		updateThreshold          = 2 * time.Second
		reasoningUpdateThreshold = 3 * time.Second
		errorCount               = 0
	)

	for chunk := range stream {
		if chunk.Error != nil {
			err = chunk.Error
			return
		}
		if len(chunk.Annotations) > 0 {
			annotations = chunk.Annotations
		}

		if chunk.Usage != nil {
			usage = chunk.Usage
		}

		if len(chunk.Tools) > 0 {
			requestedTools = chunk.Tools
			c.Logger.WithField("tools", requestedTools).Debug("Requested tools")
		}

		var editMsg *telegram.EditMessageTextConfig
		if chunk.Reasoning != "" {
			reasoningBuffer.WriteString(chunk.Reasoning)

			if !hasContent && time.Since(lastUpdateReasoning) > reasoningUpdateThreshold && reasoningBuffer.Len() > 3 {
				msg := telegram.NewEditMessageText(
					chatID,
					int(sentMsgID),
					c.L("ask.reasoningContent", map[string]any{
						"Reasoning": reasoningBuffer.String(),
					}),
				)
				editMsg = &msg
				lastUpdateReasoning = time.Now()
			}
		}

		if chunk.Content != "" {
			hasContent = true
			fullResponse.WriteString(chunk.Content)

			if time.Since(lastUpdate) > updateThreshold && fullResponse.Len() > 3 {
				msgText := fullResponse.String() + "..."

				if utf8.RuneCountInString(msgText) > 4000 {
					msgText = string([]rune(msgText)[:4000]) + "... " + c.L(
						"ask.telegramLengthRestriction",
						nil,
					)
				}

				msgText = cleanText(msgText)
				msgText, _ = c.Tg.TelegramifyMarkdown(msgText)

				msg := telegram.NewEditMessageText(
					chatID,
					int(sentMsgID),
					msgText,
				)
				msg.ParseMode = telegram.ModeMarkdownV2
				editMsg = &msg
				lastUpdate = time.Now()
			}
		}

		if editMsg != nil && errorCount < 3 {
			editMsg.LinkPreviewDisabled = true
			_, err = c.Tg.Send(editMsg)
			if err != nil {
				c.Logger.WithError(err).Warn("Failed to edit message during stream")
				if strings.Contains(err.Error(), "Too Many Requests: retry after") {
					errorCount = 3
					c.Logger.Warn("Telegram rate limit hit. Send only final message")
				} else {
					errorCount++
					if errorCount >= 3 {
						c.Logger.Error("Too many edit errors, stopping stream updates")
					}
				}
			}
		}
	}

	// --- Finalize and Save AI Response ---
	content = fullResponse.String()
	reasoning = reasoningBuffer.String()
	err = nil

	return
}

func (c *Command) mapArgsToStruct(argsMap map[string]string) (*CommandArgs, error) {
	args := &CommandArgs{
		HandleImages: c.cmdCfg.Images.Enabled,
		HandleAudio:  c.cmdCfg.Audio.Enabled,
		HandleFiles:  c.cmdCfg.Files.Enabled,
		HandleURLs:   c.cmdCfg.Fetcher.Enabled,
	}
	if argsMap == nil {
		argsMap = map[string]string{}
	}

	for _, argDef := range c.supportedArgs {
		if _, exists := argsMap[argDef.Name]; !exists && argDef.DefaultValue != "" {
			argsMap[argDef.Name] = argDef.DefaultValue
		}
	}

	for key, value := range argsMap {
		var argDef *Argument
		for i, a := range c.supportedArgs {
			if a.Name == key {
				argDef = &c.supportedArgs[i]
				break
			}
		}

		if argDef == nil {
			continue
		}

		if err := validateArg(argDef, value); err != nil {
			helpText := c.generateArgumentsHelpText()
			return args, fmt.Errorf(
				"%s: %s\n\n%s",
				markdown.Escape(key),
				markdown.Escape(err.Error()),
				helpText,
			)
		}

		switch argDef.Name {
		case "m":
			args.Model = value
		case "c":
			args.Context = value
		case "search":
			args.SearchWeb = value == "yes"
		case "i":
			args.HandleImages = value == "yes"
		case "ni":
			args.HandleImages = value != "yes"
		case "f":
			args.HandleFiles = value == "yes"
		case "nf":
			args.HandleFiles = value != "yes"
		case "audio":
			args.HandleAudio = value == "yes"
		case "noaudio":
			args.HandleAudio = value != "yes"
		case "u":
			args.HandleURLs = value == "yes"
		case "nu":
			args.HandleURLs = value != "yes"
		case "recursive":
			args.Recursive = value == "yes"
		case "r":
			val := value == "yes"
			args.Reasoning = &val
		case "nr":
			val := value != "yes"
			args.Reasoning = &val
		case "tools":
			if value == "yes" {
				args.Tools = "all"
			} else {
				args.Tools = value
			}
		case "new":
			args.New = value == "yes"
		case "think":
			args.Think = value == "yes"
			if args.Think {
				args.Model = "think"
			}
		case "multi":
			args.Multi = value == "yes"
			if args.Multi {
				args.Model = "multi"
			}
		case "fast":
			args.Fast = value == "yes"
			if args.Fast {
				args.Model = "fast"
			}
		case "rf":
			args.RF = value == "yes"
			if args.RF {
				args.Model = "rf"
			}
		case "temp":
			temp, _ := strconv.ParseFloat(value, 32)
			temp32 := float32(temp)
			args.Temperature = &temp32
		case "topp":
			topp, _ := strconv.ParseFloat(value, 32)
			topp32 := float32(topp)
			args.TopP = &topp32
		case "stream":
			if value != "" {
				stream := value == "yes"
				args.Stream = &stream
			}
		case "p":
			args.Prompt = value
		case "id":
			id, _ := strconv.Atoi(value)
			args.ChainID = id
		}
	}

	return args, nil
}

func validateArg(arg *Argument, value string) error {
	switch arg.Type {
	case "bool":
		if value != "yes" && value != "no" {
			return fmt.Errorf("requires yes/no")
		}
	case "int":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("integer required")
		}
		if arg.Min != nil && v < int(*arg.Min) {
			return fmt.Errorf("value must be ≥ %d", int(*arg.Min))
		}
		if arg.Max != nil && v > int(*arg.Max) {
			return fmt.Errorf("value must be ≤ %d", int(*arg.Max))
		}
	case "float":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("float required")
		}
		if arg.Min != nil && f < *arg.Min {
			return fmt.Errorf("value must be ≥ %.1f", *arg.Min)
		}
		if arg.Max != nil && f > *arg.Max {
			return fmt.Errorf("value must be ≤ %.1f", *arg.Max)
		}
	case "string":
		if len(arg.Values) > 1 && !slices.Contains(arg.Values, value) {
			return fmt.Errorf("allowed values: %v", strings.Join(arg.Values, ", "))
		}
	}

	if arg.Name == "c" {
		if _, _, _, _, err := parseAdditionalContextArg(value); err != nil {
			return fmt.Errorf("context format: %v", err)
		}
	}

	return nil
}

func (c *Command) createForwardOrigin(fo *telegram.MessageOrigin) *forwardOrigin {
	if fo == nil {
		return nil
	}

	origin := &forwardOrigin{
		Type: fo.Type,
	}

	switch fo.Type {
	case "user":
		origin.Name = fo.SenderUser.FirstName
		origin.EncodedID = c.getUserPublicID(fo.SenderUser.ID)
	case "chat", "channel":
		chat := fo.SenderChat
		if chat == nil {
			chat = fo.Chat
		}
		origin.Name = chat.Title
		origin.EncodedID = c.getUserPublicID(chat.ID)
		if fo.Type == "channel" {
			origin.ChatID = fo.Chat.ID
			origin.Username = fo.Chat.UserName
			origin.MessageID = fo.MessageID
		}
	case "hidden_user":
		origin.Name = fo.SenderUserName
	}

	return origin
}

func (c *Command) handleErrorWithRetry(chatID int64, text string, messageID int, originalMessageID int, err error, toolFromCallback bool) error {
	if text == "" {
		text, _ = c.Tg.TelegramifyMarkdown(c.L("ask.failedToProcessAIRequest", nil))
		text = fmt.Sprintf("%s\n_%s_", text, markdown.Escape(err.Error()))
	}
	if toolFromCallback {
		messageID = 0
		text = c.L("ask.toolUsageHint", nil)
	}
	errorMsg := telegram.NewEditMessageText(chatID, messageID, text)
	errorMsg.ParseMode = telegram.ModeMarkdownV2

	// Add the button only if there is originalMessageID (the message that we answer)
	if originalMessageID != 0 {
		callbackData := fmt.Sprintf("ask retry:%d", originalMessageID)
		errorMsg.ReplyMarkup = &telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{
				{telegram.NewInlineKeyboardButtonData(
					c.L("ask.retryButtonText", nil),
					callbackData,
				)},
			},
		}
	}
	c.Tg.SendWithRetry(&errorMsg, 0)

	return err
}

func (c *Command) generateConversationTitle(ctx context.Context, text string, chatID int64) (string, string) {
	var title string
	var err error
	if words := strings.Fields(text); len(words) <= 5 {
		return text, "initial"
	}

	generateTitle := c.cmdCfg.GenerateTitleWithAI
	if generateTitle {
		modelName := c.Cfg.AI().GetUtilityModel()
		model, _ := c.ai.GetFormattedModel(ctx, modelName, "")

		prompt := `Generate a brief title (no more than 5 words) for the following text.
The title should be informative and reflect the essence.
Do not use quotes or add explanations.

Text:
` + text

		title, _, _, _, _, err = c.ai.Ask(ctx, []ai.Message{
			{Role: ai.RoleSystem, Text: "You are an assistant for generating short titles. Respond only with a title."},
			{Role: ai.RoleUser, Text: prompt},
		}, nil, model, "", chatID, false, ai.ModelParams{})
		if err != nil {
			c.Logger.WithError(err).Error("Generating conversation title failed")
		}
	} else {
		if utf8.RuneCountInString(title) > 40 {
			return string([]rune(title)[40:]) + "...", "initial"
		} else {
			return title, "initial"
		}
	}

	return title, "llm"
}

func (c *Command) getTools(toolsList []string) []ai.Tool {
	responseTools := []ai.Tool{}
	for name, tool := range tools.AvailableTools(c.cmdCfg.Tools.Allowed, c.cmdCfg.Tools.Excluded) {
		if len(toolsList) == 0 || slices.Contains(toolsList, name) {
			responseTools = append(responseTools, tool)
		}
	}
	return responseTools
}

func (c *Command) handleRequest(
	ctx context.Context,
	userConversationMessage *conversationMessage,
	chatID int64,
	messages []ai.Message,
	currentContent *MessageContent,
	model *ai.ModelInfo,
	customParams *ai.ModelParams,
	sentMsgID int,
	messageID int,
	response *Response,
	toolFromCallback bool,
) (totalUsage *MetadataUsage, params *ai.ModelParams, err error) {
	// TODO: move to config
	maxRetries := 2
	// first iteration - basic tools request, second iteration - request with tools results
	maxIterations := c.cmdCfg.Tools.MaxIterations + 1
	params = customParams
	totalUsage = &MetadataUsage{}
	requestTools := currentContent.Tools

	currentModel := model
	toolsModelName := c.Cfg.AI().ToolsModel
	toolsModel := currentModel
	if toolsModelName != "" {
		toolsModel, err = c.ai.GetFormattedModel(ctx, toolsModelName, "")
	}

	isStream := *params.Stream
	for iteration := range maxIterations {
		var annotations []ai.AnnotationContent

		var tools []ai.ToolCall
		var usage *ai.ModelUsage
		// don't use tools model on last iteration
		if iteration+1 == maxIterations {
			currentModel = model

			conversationHistory, err := c.getConversationHistory(chatID, sentMsgID)
			if err == nil {
				// HACK: needed since metadata's ConversationHistoryLength is calculated from
				// ConversationHistory. When loading from DB, we get 2 but actually want 1 here.
				if len(currentContent.ConversationHistory) == 0 {
					currentContent.ConversationHistoryLength = 1
				}
				currentContent.ConversationHistory = conversationHistory
				currentContent.Tools = nil
				requestTools = nil
				messages = c.buildPromptWithHistory(model, currentContent, c.args, true)
			}
		} else if len(requestTools) > 0 {
			currentModel = toolsModel
		}
		if isStream {
			response.Content, response.Reasoning, tools, usage, annotations, params, err = c.AskStream(
				ctx, messages, requestTools, currentModel, currentContent.Prompt.Name,
				chatID, false, *params, sentMsgID,
			)
		} else {
			response.Content, response.Reasoning, tools, _, usage, annotations, params, err = c.Ask(
				ctx, messages, requestTools, currentModel, currentContent.Prompt.Name,
				chatID, false, *params,
			)
		}
		if err != nil {
			if ai.IsRetryableError(err) && c.retryCount < maxRetries {
				c.retryCount++
				time.Sleep(time.Second)
				c.Logger.Warn("RETRY " + fmt.Sprint(c.retryCount))
				return c.handleRequest(
					ctx,
					userConversationMessage,
					chatID,
					messages,
					currentContent,
					model,
					customParams,
					sentMsgID,
					messageID,
					response,
					toolFromCallback,
				)
			}
			c.handleErrorWithRetry(
				chatID,
				"",
				sentMsgID,
				0, // messageID if want add reply button
				err,
				toolFromCallback,
			)
			return nil, nil, err
		}
		usageInfo := NewMetadataUsageFrom(usage)
		totalUsage.Add(usageInfo)

		assistantMessage, saveErr := c.saveMessage(NewAssistantConversationMessage(
			userConversationMessage,
			sentMsgID,
			c.Tg.Self().ID,
			response.Content,
			model.FullName(),
			params,
			usageInfo,
			annotations,
			tools,
		))
		if saveErr != nil {
			c.Logger.WithError(saveErr).WithFields(logger.Fields{
				"chat_id":    chatID,
				"message_id": assistantMessage.MessageID,
				"reply_to":   messageID,
				"model":      model.FullName(),
			}).Error("Failed to save assistant message to history")
			// TODO: maybe handle error too, but message already sent to chat...
		} else {
			c.Logger.WithFields(logger.Fields{
				"chat_id":    chatID,
				"message_id": assistantMessage.MessageID,
				"reply_to":   messageID,
				"model":      model.FullName(),
			}).Debug("Successfully saved assistant message")
		}

		if len(tools) == 0 {
			break
		}

		for _, tool := range tools {
			response.Context.AddTool(tool.Function.Name)
		}

		toolsNames := make([]string, len(tools))
		for i, toolCall := range tools {
			toolsNames[i] = toolCall.Function.Name
		}

		tgMsg := telegram.NewEditMessageText(
			chatID,
			sentMsgID,
			c.L("ask.runningToolsText", map[string]any{
				"Tools": strings.Join(toolsNames, ", "),
			}),
		)
		c.Tg.Send(tgMsg)

		messagesTools, saveErr := c.handleTools(ctx, tools, assistantMessage)
		if saveErr != nil {
			c.Logger.WithError(saveErr).Warn("Partial tool execution failure")
		}
		messages = append(messages, ai.Message{
			Role:      ai.RoleAssistant,
			ToolCalls: tools,
		})
		messages = append(messages, messagesTools...)
		tgMsg = telegram.NewEditMessageText(
			chatID,
			sentMsgID,
			c.L("ask.stillThinking", nil),
		)
		c.Tg.Send(tgMsg)

		c.Logger.WithFields(logger.Fields{
			"iteration": iteration + 1,
			"tools":     len(tools),
			"responses": len(messagesTools),
		}).Debug("Tool processing iteration completed")
	}

	return
}

func (c *Command) handleTools(ctx context.Context, toolsList []ai.ToolCall, assistantMessage *conversationMessage) ([]ai.Message, error) {
	if len(toolsList) == 0 {
		return nil, errors.New("tools empty")
	}

	// NOTE: HANDLE TOOLS
	const maxRetries = 3
	response := []ai.Message{}

	type toolResult struct {
		index    int
		response ai.Message
		err      error
	}

	resultChan := make(chan toolResult, len(toolsList))
	var wg sync.WaitGroup

	for i, tool := range toolsList {
		wg.Add(1)
		go func(idx int, tool ai.ToolCall) {
			defer wg.Done()

			var toolResponse string
			var toolResponseMsg ai.Message
			toolLog := c.Logger.WithFields(logger.Fields{
				"function":  tool.Function.Name,
				"tool_id":   tool.ID,
				"tool_body": tool,
			})
			args, err := tool.Function.GetArguments()
			if err != nil {
				toolLog.WithError(err).Error("Args unmarshal error")
				resultChan <- toolResult{index: idx, err: err}
				return
			}

			var lastErr error
			retryCount := 0
			for attempt := 1; attempt <= maxRetries; attempt++ {
				retryCount++
				toolLog.WithField("attempt", attempt).Info("Running tool...")

				toolResponse, lastErr = c.runSingleTool(ctx, tool, args, assistantMessage, toolLog)
				if lastErr == nil {
					break
				}

				if strings.Contains(lastErr.Error(), "403") {
					break
				}

				toolLog.WithError(lastErr).Warn(fmt.Sprintf("Tool attempt %d failed", attempt))
				if attempt < maxRetries {
					time.Sleep(time.Second * time.Duration(attempt)) // Exponential backoff
				}
			}

			if lastErr != nil {
				toolLog.WithError(lastErr).Error("All tool attempts failed")
				toolResponse = fmt.Sprintf(
					"Tool error after %d attempts: %v",
					retryCount,
					lastErr,
				)
			}

			toolResponseMsg = ai.Message{
				Role:       ai.RoleTool,
				ToolCallID: tool.ID,
				Text:       toolResponse,
			}

			_, err = c.saveMessage(NewToolConversationMessage(
				assistantMessage,
				toolResponse,
				tool.Function.Name,
				args,
				[]ai.Message{toolResponseMsg},
			))
			if err != nil {
				toolLog.WithFields(logger.Fields{
					"tool_response": toolResponse,
				}).Error("Error saving tool response to database. Skip tool")
				resultChan <- toolResult{index: idx, err: err}
				return
			}

			resultChan <- toolResult{index: idx, response: toolResponseMsg}
		}(i, tool)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	results := make([]*toolResult, len(toolsList))
	for result := range resultChan {
		results[result.index] = &result
	}

	for _, result := range results {
		if result != nil && result.err == nil {
			response = append(response, result.response)
		}
	}

	return response, nil
}

func (c *Command) runSingleTool(ctx context.Context, tool ai.ToolCall, args map[string]any, assistantMessage *conversationMessage, toolLog logger.Logger) (string, error) {
	toolName := capitalizeFirst(tool.Function.Name)
	method := reflect.ValueOf(c.toolsRunner).MethodByName(toolName)
	if !method.IsValid() {
		return "", fmt.Errorf("tool method %s not found", toolName)
	}

	// TODO: replace reflection with direct tool access
	var results []reflect.Value
	var argsReflect []reflect.Value
	switch tool.Function.Name {
	case tools.ToolWeather:
		locationArg := args["location"]
		daysFloat, ok := args["days"].(float64)
		if !ok {
			daysFloat = 1
		}
		daysArg := int(daysFloat)
		argsReflect = []reflect.Value{
			reflect.ValueOf(locationArg),
			reflect.ValueOf(daysArg),
		}
		results = method.Call(argsReflect)
	case tools.ToolFetchTgPosts:
		channelNameArg := args["channel_name"]
		durationArg, ok := args["duration"]
		if !ok {
			durationArg = ""
		}
		limitFloat, ok := args["limit"].(float64)
		if !ok {
			limitFloat = 0
		}
		limitArg := int(limitFloat)
		argsReflect = []reflect.Value{
			reflect.ValueOf(channelNameArg),
			reflect.ValueOf(durationArg),
			reflect.ValueOf(limitArg),
		}
		results = method.Call(argsReflect)
	case tools.ToolFetchTgPostComments:
		channelNameArg := args["channel_name"]
		postID := args["post_id"].(float64)
		postIDArg := int(postID)
		argsReflect = []reflect.Value{
			reflect.ValueOf(channelNameArg),
			reflect.ValueOf(postIDArg),
			reflect.ValueOf(0),
		}
		results = method.Call(argsReflect)
	case tools.ToolFetchYtComments:
		urlArg := args["url"]
		maxComments, ok := args["max"].(float64)
		if !ok {
			maxComments = 0
		}
		maxCommentsArg := int(maxComments)
		argsReflect = []reflect.Value{
			reflect.ValueOf(urlArg),
			reflect.ValueOf(maxCommentsArg),
		}
		results = method.Call(argsReflect)
	case tools.ToolSearch:
		queryArg := args["query"]
		maxResultsFloat, ok := args["max_results"].(float64)
		if !ok {
			maxResultsFloat = 1
		}
		maxResultsArg := int(maxResultsFloat)
		timeLimitArg, ok := args["time_limit"]
		if !ok {
			timeLimitArg = ""
		}
		argsReflect = []reflect.Value{
			reflect.ValueOf(queryArg),
			reflect.ValueOf(maxResultsArg),
			reflect.ValueOf(timeLimitArg),
		}
		results = method.Call(argsReflect)
	case tools.ToolFetchURL:
		urlArg := args["url"]
		argsReflect := []reflect.Value{
			reflect.ValueOf(urlArg),
		}
		results = method.Call(argsReflect)
	case tools.ToolGenerateImage:
		prompt := args["prompt"].(string)
		argsReflect := []reflect.Value{
			reflect.ValueOf(prompt),
			reflect.ValueOf(c.Cfg.AI().ImageRouterModel),
			reflect.ValueOf(c.Cfg.AI().ImageRouterAPIKey),
		}
		results = method.Call(argsReflect)
		if !results[3].IsNil() {
			err := results[3].Interface().(error)
			toolLog.WithError(err).Error("Generate image failed")
		} else {
			image := results[1].String()
			decodedImage, err := base64.StdEncoding.DecodeString(image)
			if err == nil {
				model := "Unknown"
				if results[2].IsValid() {
					model = results[2].String()
				}
				text := fmt.Sprintf(
					"*Model:* `%s`\n*Prompt*\n`%s`%s",
					markdown.Escape(model),
					markdown.Escape(prompt),
					BotMessageMarker,
				)
				tgMsg := telegram.NewPhotoMessage(
					assistantMessage.ChatID,
					telegram.FileBytes{Name: "generated_image.jpg", Bytes: decodedImage},
					text,
					assistantMessage.MessageID,
				)
				tgMsg.ParseMode = telegram.ModeMarkdownV2
				resp, err := c.Tg.Send(tgMsg)
				if err != nil {
					toolLog.WithError(err).Error("Send generated image failed")
				} else if _, err := c.saveMessage(NewInternalConversationMessage(assistantMessage, resp.MessageID)); err != nil {
					toolLog.WithError(err).Error("Save internal message failed")
				}
			} else {
				toolLog.WithError(err).Error("Decode base64 generated image failed")
			}
		}
	case tools.ToolSearchImages:
		keywords := args["keywords"]
		maxResultsFloat, ok := args["max_results"].(float64)
		if !ok {
			maxResultsFloat = 1
		}
		maxResultsArg := int(maxResultsFloat)
		timeLimitArg, ok := args["time_limit"]
		if !ok {
			timeLimitArg = ""
		}
		argsReflect = []reflect.Value{
			reflect.ValueOf(keywords),
			reflect.ValueOf(maxResultsArg),
			reflect.ValueOf(timeLimitArg),
		}
		results = method.Call(argsReflect)
		if !results[1].IsNil() && results[1].Len() > 0 {
			images := extractStringSlice(results[1])
			mediaInputs := []telegram.InputMedia{}
			for _, image := range images {
				mediaInputs = append(mediaInputs, telegram.NewPhotoMedia(telegram.FileURL(image)))
			}
			tgMsg := telegram.NewMediaGroupMessage(assistantMessage.ChatID, mediaInputs)
			tgMsg.ReplyTo = assistantMessage.MessageID
			_, err := c.Tg.Send(tgMsg)
			if err != nil {
				toolLog.WithError(err).Error("Send media group failed")
			}
		} else {
			toolLog.Warn("Images not found")
		}
	}

	toolResponse := ""
	if results[0].IsValid() && results[0].String() != "" {
		toolResponse = results[0].String()
	}

	if len(results) > 0 && results[len(results)-1].IsValid() && !results[len(results)-1].IsNil() {
		if err, ok := results[len(results)-1].Interface().(error); ok {
			return "", fmt.Errorf("%w", err)
		}
	}

	return toolResponse, nil
}

func (c *Command) summarize(ctx context.Context, text string, chatID int64) (summary string, err error) {
	modelName := c.Cfg.AI().GetUtilityModel()
	model, err := c.ai.GetFormattedModel(ctx, modelName, "")
	if err != nil {
		return "", fmt.Errorf("error parse model: %v", err)
	}
	prompt := `Generate a professional executive summary that preserves:
1. Core thesis and key arguments
2. Important names and their contributions
3. Critical evidence and supporting data
4. Significant facts and figures
5. Context and conclusions

Structure requirements:
- Use complete, coherent paragraphs
- Maintain logical flow between ideas
- Prioritize substance over style
- Keep 3-5 key points maximum

Avoid:
- Markdown formatting
- Direct quotations
- Bullet points/lists
- Editorial commentary
- Examples/analogies

Text to summarize:
` + text

	summary, _, _, _, _, err = c.ai.Ask(ctx, []ai.Message{
		{Role: ai.RoleSystem, Text: "You are a senior analyst. Produce concise yet comprehensive summaries that capture essential information while maintaining readability and context."},
		{Role: ai.RoleUser, Text: prompt},
	}, nil, model, "", chatID, false, ai.ModelParams{})
	if err != nil {
		c.Logger.WithError(err).Error("Generating conversation summary failed")
	}

	return
}

func (c *Command) generate(ctx context.Context, prompt, addition string, chatID int64) (answer string, err error) {
	model, err := c.ai.GetFormattedModel(ctx, "fast", "")
	if err != nil {
		return "", fmt.Errorf("error parse model: %v", err)
	}

	prompt = fmt.Sprintf("[TASK STARTED]%s[TASK ENDED]\n\nTask must satisfy these criteria: %s", prompt, addition)

	// TODO: move to config
	var temp float32 = 2.0
	// TODO: move prompts (generate, summarize, title generation) in config prompts.
	// E.g., if exists prompt with name "generate",
	// then use it, else use default
	answer, _, _, _, _, err = c.ai.Ask(ctx, []ai.Message{
		{Role: ai.RoleSystem, Text: `You are a raw text processor. Strictly follow:
1. EXECUTE the user's task LITERALLY
2. NEVER add:
   - Explanations ("Here is...")
   - Dialog ("How can I help?")
   - Markers (` + "```" + `, >)
3. Format: ONLY the requested content

Example correct output for "Write a haiku":
Silent winter night
Whispers of snow in the wind
Darkness breathes softly`},
		{Role: ai.RoleUser, Text: prompt},
	}, nil, model, "", chatID, false, ai.ModelParams{Temperature: &temp})
	if err != nil {
		c.Logger.WithError(err).Error("Generating conversation summary failed")
	}

	return
}

func (c *Command) sendTypingMessage(chatID int64) {
	// Send typing action
	err := c.Tg.SendChatAction(chatID, telegram.ActionTyping)
	if err != nil {
		c.Logger.WithError(err).Warn("Failed to send typing action")
	}
}

func (c *Command) sendOrEditMessage(chatID int64, messageID int, editedMessage int, text string, msgTemplate *telegram.TextMessage) (int, error) {
	var msgToSend telegram.MessageConfig
	if editedMessage != 0 {
		msg := telegram.NewEditMessageText(chatID, editedMessage, text)
		if msgTemplate != nil {
			msg.LinkPreviewDisabled = msgTemplate.LinkPreviewDisabled
			msg.ParseMode = msgTemplate.ParseMode
		}
		msgToSend = msg
	} else {
		msg := telegram.NewMessage(chatID, text, messageID)
		if msgTemplate != nil {
			msg.LinkPreviewDisabled = msgTemplate.LinkPreviewDisabled
			msg.ParseMode = msgTemplate.ParseMode
		}
		msgToSend = msg
	}

	sentMsg, err := c.Tg.Send(msgToSend)
	if err != nil {
		return 0, err
	}
	return sentMsg.MessageID, nil
}

func (c *Command) composeHelpMessage() string {
	return fmt.Sprintf(`This bot is deeply integrated with Telegram. It was designed to be useful in conversations while being very cost-effective (using free models)

Response anatomy. The response consists of:
- Prompt (for dynamic prompts; optional)
- Reasoning (for reasoning models; optional)
- The actual response
- Context info (images, links, files, tools, additional context; optional)
- Metadata (model, tokens/money spent on message, total tokens/money spent, context length; optional)
* Optional parts may be missing and can be disabled in config

To interact with the bot, use one of the commands (explained later), mention @gachigazer_bot with your message, or reply to any message you want the bot to process. To continue dialogue, reply to the bot's message. This works in both private chats and group chats. The bot maintains full conversation history including images and links regardless of the model used, allowing model switching. For more thoughtful responses, you can switch to a reasoning model using appropriate arguments ($think if default reasoning model is set).
Important! If the bot responds in streaming mode, you can only reply to completed responses.
When replying to the bot's message, the context is taken from that message's state. This adds flexibility - you can reply to the same message twice for different results, repeat requests, or easily discard parts of the message chain by replying to older messages. Multiple people can ask different questions without interfering. The bot stores all message info in context - sender, timestamp, content, forwarding info, channel info (if applicable), images, polls etc. This allows natural conversation flow, remembering participants by name and characterizing them within message chains. Useful for scenarios like turn-based games with the bot, group discussions, and fun interactions. No user IDs or usernames are sent to the bot unless explicitly included in messages.
Responses are limited to ~850 tokens and contained in a single message.

The bot can process various content types:

1. Links. Extracts text content from URLs. Be cautious with paid models as content may be too long (configurable max length). Link processing can be disabled by default and enabled manually via $u argument. Custom fetchers optimize content for LLMs:  
- YouTube: Video title and transcript
- Reddit: Post content (including images) and comments
- GitHub: Readme content, repo info (stars, activity, issues, author), file contents
- Habr: Article content and rated comments
- Telegram: Post content and images
- etc.
2. Images. Accepts any images (model must support vision). Config can specify multimodal model for automatic switching when images are detected, with limits on image count and lifetime.
3. PDF files. Processed via Openrouter's free engine or natively by supporting models.


The bot supports various prompts usable during conversation (listed below). Examples include personality prompts, TLDR prompts, and more. Dynamic prompts generate other prompts (e.g., for random personality generation). These can be configured with parameters like temperature, top_p, stream etc. Usage methods:
  - Via arguments: $p:help
  - Via commands: /help
The "default" prompt defines behavior for @mentions and standard /ask or /a commands.
Available prompts:
%s

The bot supports multiple providers and models, switchable per-request or for entire chats. Current model is shown in metadata (clickable), displaying supported input/output types. Providers/models are configurable - any OpenAI-compatible providers can be added, with custom aliases switchable via $m argument. Shortcuts include $fast, $multi, $think, $rf (random-free model, Openrouter only). Model format: provider:model (provider can be omitted if only one exists).
Model management via /model (/m) command:
  - Search all models (/m list <query>)
  - Switch models (/m <model>)
  - Reset to default (/m reset)
Only trusted users (telegram.allowed_users in config) can switch to paid models. Openrouter has "Free models only" mode (ai.providers.<openrouter-provider>.only_free_models)

The bot supports native model tools for additional functionality: search, image generation, fetching Telegram channel posts/comments for analysis, weather, image search etc. Free models handle tools poorly, so a dedicated tools model can be specified in config. This model receives only relevant context portions. For non-tool models, full tool list and instructions are included in system prompt. When free models request tools, specification appears with activation buttons, minimizing token usage. Tools can be enabled/disabled via ai.allowed_tools and ai.excluded_tools. Base tool info (like default city for weather) can be set in ai.system_prompt.
Telegram tools: Two tools - one fetches posts (including comments, reactions, timestamps) from specified channels by time range or count. Another fetches all comments from posts. Useful for analysis or summaries.
Image generation: Uses random free models from https://imagerouter.io/ (50 images/day limit). Paid models can be configured.
Available tools:
%s

Use /info on bot messages to view context images, tool responses, and fetched link content.

The bot supports various message arguments (all starting with $). Some model behavior arguments ($stream, $temp, $topp) persist in subsequent messages. The $c argument injects additional context from previous chat messages (requires bot access to all messages).
Available arguments:
%s

The *video* command

Fetches videos from yt-dlp supported sources (YouTube, Twitch clips, Instagram reels etc). Includes metadata (views, likes, date) and top comments (if available). Max size 50MB - suitable for short clips and medium YouTube videos (fetches in lowest quality).`,
		c.Cfg.AI().GetPromptText(),
		tools.AvailableToolsText(c.cmdCfg.Tools.Allowed, c.cmdCfg.Tools.Excluded),
		c.generateArgumentsHelpText(),
	)
}
