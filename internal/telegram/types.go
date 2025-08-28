package telegram

import (
	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

type ParseMode = string

const (
	ModeMarkdownV2 = "MarkdownV2"
)

type (
	// TODO: refactor
	MessageOriginal = tgbotapi.Message
	BaseMessage     = tgbotapi.BaseChatMessage
	Update          = tgbotapi.Update
	FileURL         = tgbotapi.FileURL
	FileBytes       = tgbotapi.FileBytes
	MessageEntity   = tgbotapi.MessageEntity
	MessageOrigin   = tgbotapi.MessageOrigin
	EditVideo       = tgbotapi.EditMessageMediaConfig
	Chattable       = tgbotapi.Chattable
	RequestFileData = tgbotapi.RequestFileData

	InlineKeyboardMarkup = tgbotapi.InlineKeyboardMarkup
	InlineKeyboardButton = tgbotapi.InlineKeyboardButton
)

func NewInlineKeyboardMarkup(rows ...[]InlineKeyboardButton) InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func NewInlineKeyboardRow(buttons ...InlineKeyboardButton) []InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardRow(buttons...)
}

func NewInlineKeyboardButtonData(text, data string) InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardButtonData(text, data)
}

type Message struct {
	MessageID     int
	Chat          Chat
	Text          string
	From          User
	ReplyTo       *Message
	Caption       string
	MediaGroupID  string
	Command       string
	ForwardOrigin *User
}

type User struct {
	ID        int64
	FirstName string
	UserName  string
}

type Chat struct {
	ID   int64
	Type string
}

type APIResponse struct {
	Ok          bool
	Description string
	ErrorCode   int
}

type MessageConfig interface {
	ToChattable() tgbotapi.Chattable
}

type CallbackConfig struct {
	CallbackQueryID string
	Text            string
	ShowAlert       bool
	URL             string
	CacheTime       int
}

func NewCallback(id, text string) CallbackConfig {
	return CallbackConfig{
		CallbackQueryID: id,
		Text:            text,
		ShowAlert:       false,
	}
}

func (c *CallbackConfig) ToChattable() tgbotapi.Chattable {
	config := tgbotapi.NewCallback(c.CallbackQueryID, c.Text)
	config.CacheTime = c.CacheTime
	config.ShowAlert = c.ShowAlert
	config.URL = c.URL
	return config
}

type TextMessage struct {
	ChatID              int64
	Text                string
	ReplyTo             int
	ReplyMarkup         *InlineKeyboardMarkup
	LinkPreviewDisabled bool
	ParseMode           ParseMode
}

func NewMessage(chatID int64, text string, replyTo int) TextMessage {
	return TextMessage{
		ChatID:              chatID,
		Text:                text,
		LinkPreviewDisabled: false,
		ReplyTo:             replyTo,
	}
}

func (m TextMessage) ToChattable() tgbotapi.Chattable {
	msg := tgbotapi.NewMessage(m.ChatID, m.Text)
	msg.ReplyParameters.MessageID = m.ReplyTo
	msg.ParseMode = m.ParseMode
	if m.ReplyMarkup != nil {
		msg.ReplyMarkup = m.ReplyMarkup
	}
	msg.LinkPreviewOptions.IsDisabled = m.LinkPreviewDisabled
	return msg
}

type PhotoMessage struct {
	ChatID      int64
	Photo       RequestFileData
	Caption     string
	ReplyTo     int
	ParseMode   string
	ReplyMarkup any
}

func NewPhotoMessage(chatID int64, photo RequestFileData, caption string, replyTo int) PhotoMessage {
	return PhotoMessage{
		ChatID:  chatID,
		Photo:   photo,
		Caption: caption,
		ReplyTo: replyTo,
	}
}

func (m PhotoMessage) ToChattable() tgbotapi.Chattable {
	msg := tgbotapi.NewPhoto(m.ChatID, m.Photo)
	msg.Caption = m.Caption
	msg.ReplyParameters.MessageID = m.ReplyTo
	msg.ParseMode = m.ParseMode
	msg.ReplyMarkup = m.ReplyMarkup
	return msg
}

type VideoMessage struct {
	ChatID      int64
	Video       RequestFileData
	Caption     string
	ReplyTo     int
	ParseMode   string
	ReplyMarkup any
}

func NewVideoMessage(chatID int64, video RequestFileData, caption string, replyTo int) VideoMessage {
	return VideoMessage{
		ChatID:  chatID,
		Video:   video,
		Caption: caption,
		ReplyTo: replyTo,
	}
}

func (m VideoMessage) ToChattable() tgbotapi.Chattable {
	msg := tgbotapi.NewVideo(m.ChatID, m.Video)
	msg.Caption = m.Caption
	msg.ReplyParameters.MessageID = m.ReplyTo
	msg.ParseMode = m.ParseMode
	msg.ReplyMarkup = m.ReplyMarkup
	return msg
}

type EditMessageVideoConfig struct {
	ChatID      int64
	MessageID   int
	Video       RequestFileData
	Caption     string
	ParseMode   string
	ReplyMarkup *InlineKeyboardMarkup
}

func NewEditMessageVideo(chatID int64, messageID int, video RequestFileData) EditMessageVideoConfig {
	return EditMessageVideoConfig{
		ChatID:    chatID,
		MessageID: messageID,
		Video:     video,
	}
}

func (m EditMessageVideoConfig) ToChattable() tgbotapi.Chattable {
	videoMedia := tgbotapi.NewInputMediaVideo(m.Video)
	msg := tgbotapi.NewEditMessageMedia(m.ChatID, m.MessageID, &videoMedia)
	msg.ReplyMarkup = m.ReplyMarkup
	return msg
}

type EditMessageTextConfig struct {
	ChatID              int64
	MessageID           int
	Text                string
	ParseMode           string
	ReplyMarkup         *InlineKeyboardMarkup
	LinkPreviewDisabled bool
}

func NewEditMessageText(chatID int64, messageID int, text string) EditMessageTextConfig {
	return EditMessageTextConfig{
		ChatID:              chatID,
		MessageID:           messageID,
		Text:                text,
		LinkPreviewDisabled: false,
	}
}

func (m EditMessageTextConfig) ToChattable() tgbotapi.Chattable {
	msg := tgbotapi.NewEditMessageText(m.ChatID, m.MessageID, m.Text)
	msg.LinkPreviewOptions.IsDisabled = m.LinkPreviewDisabled
	msg.ParseMode = m.ParseMode
	msg.ReplyMarkup = m.ReplyMarkup
	return msg
}

type EditMessageReplyMarkupConfig struct {
	ChatID      int64
	MessageID   int
	ReplyMarkup *InlineKeyboardMarkup
}

func NewEditMessageReplyMarkup(chatID int64, messageID int, replyMarkup *InlineKeyboardMarkup) EditMessageReplyMarkupConfig {
	return EditMessageReplyMarkupConfig{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: replyMarkup,
	}
}

func (c EditMessageReplyMarkupConfig) ToChattable() tgbotapi.Chattable {
	return tgbotapi.NewEditMessageReplyMarkup(c.ChatID, c.MessageID, *c.ReplyMarkup)
}

type InputMedia interface {
	ToMedia() tgbotapi.InputMedia
}

type PhotoMedia struct {
	Media     RequestFileData
	Caption   string
	ParseMode string
}

func NewPhotoMedia(media RequestFileData) PhotoMedia {
	return PhotoMedia{
		Media: media,
	}
}

func (p PhotoMedia) ToMedia() tgbotapi.InputMedia {
	media := tgbotapi.NewInputMediaPhoto(p.Media)
	media.Caption = p.Caption
	media.ParseMode = p.ParseMode
	return &media
}

type VideoMedia struct {
	Media     RequestFileData
	Caption   string
	ParseMode string
	Width     int
	Height    int
}

func NewVideoMedia(media RequestFileData) VideoMedia {
	return VideoMedia{
		Media: media,
	}
}

func (v VideoMedia) ToMedia() tgbotapi.InputMedia {
	media := tgbotapi.NewInputMediaVideo(v.Media)
	media.Caption = v.Caption
	media.ParseMode = v.ParseMode
	if v.Width != 0 && v.Height != 0 {
		media.Width = v.Width
		media.Height = v.Height
	}
	return &media
}

type MediaGroupMessage struct {
	ChatID  int64
	Media   []InputMedia
	ReplyTo int
}

func NewMediaGroupMessage(chatID int64, media []InputMedia) MediaGroupMessage {
	return MediaGroupMessage{
		ChatID: chatID,
		Media:  media,
	}
}

func (m MediaGroupMessage) ToChattable() tgbotapi.Chattable {
	media := make([]tgbotapi.InputMedia, 0, len(m.Media))
	for _, item := range m.Media {
		media = append(media, item.ToMedia())
	}

	return tgbotapi.NewMediaGroup(m.ChatID, media)
}

type UpdateConfig struct {
	Offset  int
	Limit   int
	Timeout int
}

type ChatAction string

const (
	ActionTyping      ChatAction = "typing"
	ActionUploadPhoto ChatAction = "upload_photo"
	ActionUploadVideo ChatAction = "upload_video"
)

type Client interface {
	Send(msg MessageConfig) (*Message, error)
	SendWithRetry(msg MessageConfig, maxRetryCount int) (*Message, error)
	DeleteMessage(chatID int64, messageID int) (*tgbotapi.APIResponse, error)
	SendMediaGroup(mediaGroup MediaGroupMessage) (*tgbotapi.APIResponse, error)
	GetFileURL(fileID string) (string, error)
	EscapeText(text string) string
	GetUpdatesChan(config UpdateConfig) <-chan tgbotapi.Update
	Request(message MessageConfig) (*tgbotapi.APIResponse, error)
	RequestRaw(message tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
	SendChatAction(chatID int64, action ChatAction) error
	TelegramifyMarkdown(text string) (string, error)
	NewUpdate(offset, timeout, limit int) UpdateConfig
	Self() User
}
