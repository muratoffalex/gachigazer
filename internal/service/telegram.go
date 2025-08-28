package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/muratoffalex/gachigazer/internal/cache"
	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

const cacheType = cache.PersistentPrefix

type ChannelInfo struct {
	ID         int64
	AccessHash int64
	Title      string
	Name       string
}

func (c *ChannelInfo) GetShareID() string {
	if c.Name != "" {
		return c.Name
	}
	return fmt.Sprint(c.ID)
}

type TelegramAPI struct {
	client      *telegram.Client
	apiID       int
	apiHash     string
	sessionPath string
	cache       cache.Cache
	log         logger.Logger
}

var (
	instance *TelegramAPI
	once     sync.Once
)

func InitTDInstance(cfg config.TelegramConfig, cache cache.Cache, logger logger.Logger) *TelegramAPI {
	once.Do(func() {
		instance = NewTelegramAPI(
			cfg.ApiID,
			cfg.ApiHash,
			cfg.SessionPath,
			cache,
			logger,
		)
		codePromptFunc := func(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
			fmt.Printf("Please enter telegram 2FA code for number %s: ", cfg.Phone)
			var code string
			fmt.Scanln(&code)
			return code, nil
		}
		err := instance.AuthIfNeeded(context.Background(), cfg.Phone, cfg.Password, codePromptFunc)
		if err != nil {
			logger.WithError(err).Fatal("TG auth failed")
		}
	})
	return instance
}

func GetTD() *TelegramAPI {
	return instance
}

func NewTelegramAPI(apiID int, apiHash, sessionPath string, cache cache.Cache, logger logger.Logger) *TelegramAPI {
	return &TelegramAPI{
		apiID:       apiID,
		apiHash:     apiHash,
		client:      nil,
		sessionPath: sessionPath,
		cache:       cache,
		log:         logger,
	}
}

// GetChannelPosts retrieves messages from a channel for the specified period
func (t *TelegramAPI) GetChannelPosts(
	ctx context.Context,
	channelName string,
	since time.Time,
	limit int,
	offset int,
) ([]string, error) {
	channel, err := t.ResolveChannelInput(ctx, channelName)
	if err != nil {
		return nil, fmt.Errorf("error resolve channel: %w", err)
	}

	if err := t.ensureClient(ctx); err != nil {
		return nil, err
	}
	var posts []string
	if limit == 0 || limit > 100 {
		limit = 100
	}
	err = t.client.Run(ctx, func(ctx context.Context) error {
		// 1. Get access to Telegram API
		api := tg.NewClient(t.client)

		// 2. Request message history
		messages, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer: &tg.InputPeerChannel{
				ChannelID:  channel.ID,
				AccessHash: channel.AccessHash,
			},
			Limit:     limit,
			AddOffset: offset,
		})
		if err != nil {
			return fmt.Errorf("get history: %w", err)
		}

		// 3. Format the result
		switch v := messages.(type) {
		case *tg.MessagesChannelMessages:
			posts = append(posts, t.handleMessages(v.Messages, channel.GetShareID(), since)...)
		case *tg.MessagesMessages:
			posts = append(posts, t.handleMessages(v.Messages, channel.GetShareID(), since)...)
		default:
			return fmt.Errorf("unexpected messages type: %T", messages)
		}

		return nil
	})
	if err != nil {
		log.Printf("Telegram API error: %v", err)
		return nil, err
	}

	return posts, nil
}

func (t *TelegramAPI) GetPostComments(
	ctx context.Context,
	channelInput string,
	postID int,
	limit int,
) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer func() {
		cancel()
		t.log.Debug("Context canceled, all pending operations terminated")
	}()

	channelInfo, err := t.ResolveChannelInput(ctx, channelInput)
	if err != nil {
		return nil, fmt.Errorf("resolve channel: %w", err)
	}
	t.log.WithField("channelInfo", channelInfo).Debug("Get channel info")

	var allComments []string
	chunkSize := 100
	if limit > 0 && limit < chunkSize {
		chunkSize = limit
	}

	offset := 0
	for {
		var commentCount int
		var comments []string
		if len(allComments) > 0 {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if err := t.ensureClient(ctx); err != nil {
			return nil, err
		}
		t.log.Debug("Ensure client")
		err := t.client.Run(ctx, func(ctx context.Context) error {
			api := tg.NewClient(t.client)
			t.log.Debug("New client created")

			replies, err := api.MessagesGetReplies(ctx, &tg.MessagesGetRepliesRequest{
				Peer: &tg.InputPeerChannel{
					ChannelID:  channelInfo.ID,
					AccessHash: channelInfo.AccessHash,
				},
				MsgID:    postID,
				Limit:    chunkSize,
				OffsetID: offset,
			})
			if err != nil {
				return err
			}
			t.log.Debug("Get comments")

			switch v := replies.(type) {
			case *tg.MessagesChannelMessages:
				commentCount = len(v.Messages)
				comments = t.handleMessages(v.Messages, "", time.Time{})
				if len(v.Messages) > 0 {
					offset = v.Messages[len(v.Messages)-1].(*tg.Message).ID
				}
			case *tg.MessagesMessages:
				commentCount = len(v.Messages)
				comments = t.handleMessages(v.Messages, "", time.Time{})
				if len(v.Messages) > 0 {
					offset = v.Messages[len(v.Messages)-1].(*tg.Message).ID
				}
			default:
				return fmt.Errorf("unexpected replies type: %T", replies)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("get comments chunk: %w", err)
		}

		t.log.WithField("count", len(allComments)).Debug("comments count")

		allComments = append(allComments, comments...)
		if (limit > 0 && len(allComments) >= limit) ||
			len(comments) == 0 || commentCount < chunkSize {
			break
		}
	}

	if limit > 0 && len(allComments) > limit {
		allComments = allComments[:limit]
	}

	t.log.WithField("total_comments", len(allComments)).Info("Successfully fetched comments")
	return allComments, nil
}

func (t *TelegramAPI) ResolveChannel(ctx context.Context, channelName string) (*ChannelInfo, error) {
	if err := t.ensureClient(ctx); err != nil {
		return nil, err
	}

	var channelInfo *ChannelInfo
	err := t.client.Run(ctx, func(ctx context.Context) error {
		api := tg.NewClient(t.client)

		channelName = strings.TrimPrefix(channelName, "@")
		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: channelName})
		if err != nil {
			return fmt.Errorf("resolve username: %w", err)
		}

		for _, chat := range resolved.Chats {
			if c, ok := chat.(*tg.Channel); ok {
				channelInfo = &ChannelInfo{
					ID:         c.ID,
					AccessHash: c.AccessHash,
					Title:      c.Title,
					Name:       c.Username,
				}
				return nil
			}
		}
		return fmt.Errorf("channel not found")
	})

	return channelInfo, err
}

func (t *TelegramAPI) GetChannelByID(ctx context.Context, channelID int64) (*ChannelInfo, error) {
	if err := t.ensureClient(ctx); err != nil {
		return nil, err
	}

	var channelInfo *ChannelInfo
	err := t.client.Run(ctx, func(ctx context.Context) error {
		api := tg.NewClient(t.client)

		dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			Limit: 100,
		})
		if err != nil {
			return fmt.Errorf("get dialogs: %w", err)
		}

		switch d := dialogs.(type) {
		case *tg.MessagesDialogs:
			for _, chat := range d.Chats {
				if c, ok := chat.(*tg.Channel); ok && c.ID == channelID {
					channelInfo = &ChannelInfo{
						ID:         c.ID,
						AccessHash: c.AccessHash,
						Title:      c.Title,
						Name:       c.Username,
					}
					return nil
				}
			}
		case *tg.MessagesDialogsSlice:
			for _, chat := range d.Chats {
				if c, ok := chat.(*tg.Channel); ok && c.ID == channelID {
					channelInfo = &ChannelInfo{
						ID:         c.ID,
						AccessHash: c.AccessHash,
						Title:      c.Title,
						Name:       c.Username,
					}
					return nil
				}
			}
		}

		return fmt.Errorf("channel with ID %d not found", channelID)
	})

	return channelInfo, err
}

// ResolveChannelInput automatically resolves channel information and cached it
// Supported formats:
// - Post URL ("https://t.me/channel/123")
// - Username + post ID ("channel", 123)
// - Channel ID + post ID (123456, 123)
func (t *TelegramAPI) ResolveChannelInput(ctx context.Context, input string) (*ChannelInfo, error) {
	cacheKey := fmt.Sprintf("%s:channel:%s", cacheType, input)

	if cached, ok := t.cache.Get(cacheKey); ok {
		var channel *ChannelInfo
		if err := json.Unmarshal(cached, &channel); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cached channel: %w", err)
		}
		return channel, nil
	}

	var (
		channelInfo *ChannelInfo
		err         error
	)

	if strings.HasPrefix(input, "https://t.me/") {
		parts := strings.Split(input, "/")
		if len(parts) >= 4 {
			channelInfo, err = t.ResolveChannel(ctx, parts[3])
		}
	} else if id, parseErr := strconv.ParseInt(input, 10, 64); parseErr == nil {
		channelInfo, err = t.GetChannelByID(ctx, id)
	} else {
		channelInfo, err = t.ResolveChannel(ctx, input)
	}

	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(channelInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal channel for cache: %w", err)
	}
	t.cache.Set(cacheKey, data, 30*24*time.Hour)

	return channelInfo, nil
}

func (t *TelegramAPI) Auth(ctx context.Context, phone, password string, codePromptFunc auth.CodeAuthenticatorFunc) error {
	if err := t.ensureClient(ctx); err != nil {
		return err
	}
	return t.client.Run(ctx, func(ctx context.Context) error {
		return auth.NewFlow(
			auth.Constant(phone, password, codePromptFunc),
			auth.SendCodeOptions{},
		).Run(ctx, t.client.Auth())
	})
}

func (t *TelegramAPI) AuthIfNeeded(ctx context.Context, phone, password string, codePromptFunc auth.CodeAuthenticatorFunc) error {
	if t.isLoggedIn(t.sessionPath) {
		return nil
	}

	return t.Auth(ctx, phone, password, codePromptFunc)
}

func (t *TelegramAPI) isLoggedIn(sessionPath string) bool {
	_, err := os.Stat(sessionPath)
	return !os.IsNotExist(err)
}

func (t *TelegramAPI) handleMessages(messages []tg.MessageClass, channelName string, since time.Time) []string {
	posts := []string{}
	for _, msg := range messages {
		message, ok := msg.(*tg.Message)
		if !ok || message.Message == "" { // skip without text
			continue
		}
		if !since.IsZero() {
			msgTime := time.Unix(int64(message.Date), 0)
			if msgTime.Before(since) {
				continue
			}
		}

		// Count media attachments
		var (
			photoCount int
			videoCount int
			audioCount int
		)
		if message.Media != nil {
			switch m := message.Media.(type) {
			case *tg.MessageMediaPhoto:
				photoCount = 1
			case *tg.MessageMediaDocument:
				doc := m.Document.(*tg.Document)
				for _, attr := range doc.Attributes {
					switch attr.(type) {
					case *tg.DocumentAttributeVideo:
						videoCount++
					case *tg.DocumentAttributeAudio:
						audioCount++
					}
				}
			}
		}

		// Build media prefix
		var mediaPrefix string
		if photoCount > 0 {
			mediaPrefix += fmt.Sprintf("🖼️×%d ", photoCount)
		}
		if videoCount > 0 {
			mediaPrefix += fmt.Sprintf("🎥×%d ", videoCount)
		}
		if audioCount > 0 {
			mediaPrefix += fmt.Sprintf("🎵×%d ", audioCount)
		}

		meta := ""
		if message.Views > 0 {
			meta += fmt.Sprintf("%d views | ", message.Views)
		}
		if len(message.Reactions.Results) > 0 {
			meta += "Reactions: "
			for _, r := range message.Reactions.Results {
				if emoji, ok := r.Reaction.(*tg.ReactionEmoji); ok {
					meta += fmt.Sprintf("%s×%d ", emoji.Emoticon, r.Count)
				}
			}
		}
		if message.Replies.Replies > 0 {
			meta += fmt.Sprintf(" | %d comments", message.Replies.Replies)
		}
		if meta != "" {
			meta = " | " + meta
		}

		messageText := mediaPrefix + message.Message
		var post string
		if channelName == "" { // comment
			post = fmt.Sprintf(
				"%s%s\n%s",
				time.Unix(int64(message.Date), 0).Format("2006-01-02 15:04:05"),
				meta,
				messageText,
			)
		} else { // post
			post = fmt.Sprintf(
				"%s | https://t.me/%s/%d%s\n%s",
				time.Unix(int64(message.Date), 0).Format("2006-01-02 15:04:05"),
				channelName,
				message.ID,
				meta,
				messageText,
			)
		}
		posts = append(posts, post)
	}
	return posts
}

func (t *TelegramAPI) ensureClient(ctx context.Context) error {
	t.client = telegram.NewClient(t.apiID, t.apiHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{
			Path: t.sessionPath,
		},
	})
	return nil
}
