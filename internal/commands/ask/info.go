package ask

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

func (c *Command) handleInfoCommand(update telegram.Update) error {
	originalMsgID := int(0)
	if update.Message.ReplyToMessage != nil {
		originalMsgID = update.Message.ReplyToMessage.MessageID
	}
	chatID := update.Message.Chat.ID

	if originalMsgID == 0 {
		latestMsg, err := c.getLatestMessageFromHistory(chatID)
		if err == nil {
			originalMsgID = latestMsg.MessageID
		}
	}

	c.Logger.WithFields(logger.Fields{
		"chat_id":    chatID,
		"message_id": originalMsgID,
	}).Debug("Get info for message")
	conversationHistory, err := c.getConversationHistory(chatID, originalMsgID)
	if err != nil {
		c.Logger.WithError(err).Error("Get conversation history failed")
		errMsg := telegram.NewMessage(
			chatID,
			c.L("ask.info.metadataNotFound", nil),
			update.Message.MessageID,
		)
		_, err := c.Tg.Send(errMsg)
		return err
	}

	if len(conversationHistory) == 0 {
		msg := telegram.NewMessage(
			update.Message.Chat.ID,
			c.L("ask.info.replyToAIResponse", nil),
			update.Message.MessageID,
		)
		_, err := c.Tg.Send(msg)
		return err
	}

	msg := conversationHistory[0]
	totalUsage := &MetadataUsage{}
	usage := &MetadataUsage{}
	currentContext := Context{}
	// TODO: add checks for max and lifetime for images
	maxImages := c.Cfg.GetAskCommandConfig().Images.Max
	imageLifetime := c.Cfg.GetAskCommandConfig().Images.Lifetime
	maxAudio := c.Cfg.GetAskCommandConfig().Audio.MaxInHistory
	for index, message := range conversationHistory {
		if message.Role == "assistant" {
			totalUsage.Add(message.Usage)
			if message.MessageID == originalMsgID {
				usage.Add(message.Usage)
			}
		}

		if len(message.Audio) > 0 && (index == 1 || maxAudio > 0) {
			for _, file := range message.Audio {
				currentContext.Audio = append(currentContext.Audio, AudioInput{Type: file.InputAudio.Format})
				maxAudio--
			}
		}

		for _, file := range message.Files {
			currentContext.Files = append(currentContext.Files, file.File.Filename)
		}
		if maxImages > 0 && (imageLifetime == 0 || time.Now().Before(message.CreatedAt.Add(imageLifetime))) {
			for _, image := range message.Images {
				currentContext.Images = append(currentContext.Images, image.ImageURL.URL)
				maxImages--
				if maxImages == 0 {
					break
				}
			}
		}

		currentContext.URLs = append(currentContext.URLs, message.URLs...)
		if message.MessageID == originalMsgID && message.ToolName.Valid {
			currentContext.AddDetailedTool(ContextToolDetailed{
				Name:     message.ToolName.String,
				Params:   message.ToolParams,
				Response: message.Text,
			})
		}
	}

	// Reconstruct metadata
	model, err := c.ai.GetFormattedModel(context.Background(), msg.ModelName.String, "")
	if err != nil {
		return err
	}

	provider, _ := c.ai.GetProvider(model.Provider)

	content := MessageContent{ConversationHistory: conversationHistory}
	allContext := currentContext.GetFormattedString(model, provider, c.Localizer, true)
	currencyConfig := c.Cfg.Currency()
	metadata := NewMetadata(
		model,
		provider,
		usage,
		int(msg.MessageID),
		totalUsage,
		content.ContextTurnsCount()-1,
		0,
		0,
		msg.ConversationID,
		msg.Params,
		&currencyConfig,
		c.Localizer,
	)

	blocks := []string{}

	if allContext != "" {
		blocks = append(blocks, c.L("ask.context", nil)+"\n"+allContext)
	}

	blocks = append(blocks, metadata.GetDetailedInfo())
	blocks = append(blocks, metadata.GetFormattedString())

	response := strings.Join(blocks, "\n\n")
	response = strings.ToValidUTF8(response, "")

	images := currentContext.Images
	messageID := update.Message.MessageID
	infoMsg := telegram.NewMessage(chatID, response, messageID)
	infoMsg.ParseMode = telegram.ModeMarkdownV2
	infoMsg.LinkPreviewDisabled = true
	if len(images) > 0 {
		inputs := []telegram.InputMedia{}
		for i, image := range images {
			var data telegram.RequestFileData
			if strings.HasPrefix(image, "data:") {
				parts := strings.SplitN(image, ",", 2)
				if len(parts) == 2 {
					image = parts[1]
				}
				decodedImage, _ := base64.StdEncoding.DecodeString(image)
				data = telegram.FileBytes{Name: fmt.Sprintf("img%d.jpg", i), Bytes: decodedImage}
			} else {
				data = telegram.FileURL(image)
			}
			inputs = append(inputs, telegram.NewPhotoMedia(data))
		}
		msg := telegram.NewMediaGroupMessage(chatID, inputs)
		msg.ReplyTo = update.Message.MessageID
		resp, err := c.Tg.Send(msg)
		if err != nil {
			c.Logger.WithError(err).Error("Failed send media-group")
		} else {
			infoMsg.ReplyTo = resp.MessageID
			_, err = c.Tg.Send(infoMsg)
			return err
		}
	}

	_, err = c.Tg.Send(infoMsg)
	return err
}
