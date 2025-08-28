package telegram

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/markdown"
)

type BotClient struct {
	bot      *tgbotapi.BotAPI
	markdown *markdown.MarkdownProcessor
	logger   logger.Logger
}

func NewBotClient(
	bot *tgbotapi.BotAPI,
	logger logger.Logger,
	markdown markdown.MarkdownProcessor,
) Client {
	return &BotClient{
		bot:      bot,
		markdown: &markdown,
		logger:   logger,
	}
}

func (c *BotClient) Send(msg MessageConfig) (*Message, error) {
	sentMsg, err := c.bot.Send(msg.ToChattable())
	if err != nil {
		return nil, err
	}
	return adaptMessage(&sentMsg), nil
}

func (c *BotClient) SendWithRetry(msg MessageConfig, maxRetryCount int) (*Message, error) {
	maxRetries := 1
	if maxRetryCount > 0 {
		maxRetries = maxRetryCount
	}
	retryCount := 0

	for {
		sentMsg, err := c.bot.Send(msg.ToChattable())
		if err == nil {
			return adaptMessage(&sentMsg), nil
		}

		if strings.Contains(err.Error(), "Too Many Requests: retry after") {
			retryAfter := extractRetryAfter(err.Error())
			waitTime := time.Duration(retryAfter+2) * time.Second

			c.logger.WithFields(logger.Fields{
				"retry_after": retryAfter,
				"wait_time":   waitTime,
				"attempt":     retryCount + 1,
			}).Warn("Rate limit hit, waiting before retry")

			time.Sleep(waitTime)
			retryCount++

			if retryCount > maxRetries {
				c.logger.Error("Max retries reached for rate limited message")
				return nil, err
			}
			continue
		}

		return nil, err
	}
}

func (c *BotClient) SendMediaGroup(mediaGroup MediaGroupMessage) (*tgbotapi.APIResponse, error) {
	return c.Request(mediaGroup)
}

func (c *BotClient) GetFileURL(fileID string) (string, error) {
	file, err := c.bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", err
	}
	return file.Link(c.bot.Token), nil
}

func (c *BotClient) EscapeText(text string) string {
	return tgbotapi.EscapeText(ModeMarkdownV2, text)
}

func (c *BotClient) GetUpdatesChan(config UpdateConfig) <-chan tgbotapi.Update {
	tgConfig := tgbotapi.UpdateConfig{
		Offset:  config.Offset,
		Limit:   config.Limit,
		Timeout: config.Timeout,
	}
	srcChan := c.bot.GetUpdatesChan(tgConfig)
	dstChan := make(chan tgbotapi.Update)

	go func() {
		for update := range srcChan {
			dstChan <- update
		}
		close(dstChan)
	}()

	return dstChan
}

func (c *BotClient) Request(message MessageConfig) (*tgbotapi.APIResponse, error) {
	return c.bot.Request(message.ToChattable())
}

func (c *BotClient) RequestRaw(message tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return c.bot.Request(message)
}

func (c *BotClient) SendChatAction(chatID int64, action ChatAction) error {
	_, err := c.bot.Request(tgbotapi.NewChatAction(chatID, string(action)))
	return err
}

func (c *BotClient) TelegramifyMarkdown(text string) (string, error) {
	return c.markdown.Convert(text)
}

func (c *BotClient) NewUpdate(offset, timeout, limit int) UpdateConfig {
	return UpdateConfig{
		Offset:  offset,
		Limit:   limit,
		Timeout: timeout,
	}
}

func (c *BotClient) DeleteMessage(chatID int64, messageID int) (*tgbotapi.APIResponse, error) {
	return c.RequestRaw(tgbotapi.NewDeleteMessage(chatID, messageID))
}

func (c *BotClient) Self() User {
	return adaptUser(&c.bot.Self)
}

func FormatMessageLink(chatID int64, messageID int) string {
	return fmt.Sprintf("https://t.me/c/%d/%d", chatID, messageID)
}

func extractRetryAfter(errMsg string) int {
	re := regexp.MustCompile(`retry after (\d+)`)
	matches := re.FindStringSubmatch(errMsg)
	if len(matches) > 1 {
		retryAfter, _ := strconv.Atoi(matches[1])
		return retryAfter
	}
	return 0
}

func adaptMessage(msg *tgbotapi.Message) *Message {
	if msg == nil {
		return nil
	}

	return &Message{
		MessageID: msg.MessageID,
		Chat:      adaptChat(&msg.Chat),
		Text:      msg.Text,
		From:      adaptUser(msg.From),
		ReplyTo:   adaptMessage(msg.ReplyToMessage),
		Command:   msg.Command(),
	}
}

func adaptUser(user *tgbotapi.User) User {
	if user == nil {
		return User{}
	}
	return User{
		ID:        int64(user.ID),
		FirstName: user.FirstName,
		UserName:  user.UserName,
	}
}

func adaptChat(chat *tgbotapi.Chat) Chat {
	if chat == nil {
		return Chat{}
	}
	return Chat{
		ID:   chat.ID,
		Type: chat.Type,
	}
}

// func adaptUpdate(update tgbotapi.Update) Update {
// 	return Update{
// 		ID:      update.UpdateID,
// 		Message: adaptMessage(update.Message),
// 	}
// }
