package random

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/app/di"
	"github.com/muratoffalex/gachigazer/internal/cache"
	"github.com/muratoffalex/gachigazer/internal/commands/base"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

const (
	CommandName = "r"

	maxCaptionLength = 1024
	maxTagsLength    = 500
)

type Command struct {
	*base.Command
	api   *api
	cache cache.Cache
}

func New(di *di.Container) *Command {
	cmd := &Command{
		api:   newAPI(di),
		cache: di.Cache,
	}
	cmd.Command = base.NewCommand(cmd, di)
	return cmd
}

func (c *Command) Name() string {
	return CommandName
}

func escapeMarkdown(text string, ignoreChars ...string) string {
	specialChars := []string{
		"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|",
		"{", "}", ".", "!", "?", "<", ">", ":", ";", "&", ",",
	}

	ignore := make(map[string]bool)
	for _, char := range ignoreChars {
		ignore[char] = true
	}

	escaped := text
	escaped = strings.ReplaceAll(escaped, "\\", "\\\\")

	for _, char := range specialChars {
		if !ignore[char] {
			escaped = strings.ReplaceAll(escaped, char, "\\"+char)
		}
	}
	return escaped
}

// HandleCallback handles inline buttons for the random command
func (c *Command) HandleCallback(callbackData string, update telegram.Update) error {
	parts := strings.SplitN(callbackData, " ", 2)
	if len(parts) < 2 {
		return fmt.Errorf("invalid callback data format: %s", callbackData)
	}

	actionData := strings.SplitN(parts[1], ":", 2)
	if len(actionData) < 2 {
		return fmt.Errorf("invalid action data format: %s", parts[1])
	}

	action := actionData[0]
	data := actionData[1]

	msg := update.CallbackQuery.Message

	switch action {
	case "tag", "next":
		// Both buttons do the same thing - request a post by tags
		// Form an artificial update to execute the command
		update.Message = &telegram.MessageOriginal{
			MessageID: msg.MessageID,
			From:      update.CallbackQuery.From,
			Chat:      msg.Chat,
			Text:      "/r " + data,
			Entities: []telegram.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 2, // "/r"
				},
			},
		}
		return c.Execute(update)
	default:
		return fmt.Errorf("unknown callback action: %s", action)
	}
}

func (c *Command) Execute(update telegram.Update) error {
	if !update.Message.IsCommand() {
		return fmt.Errorf("not a command")
	}

	args := strings.Fields(update.Message.CommandArguments())
	userID := fmt.Sprintf("%d", update.Message.From.ID)

	if len(args) == 0 {
		if data, found := c.cache.Get("db:r:last_args:" + userID); found {
			args = strings.Fields(string(data))
			c.Logger.WithFields(logger.Fields{
				"user_id": userID,
				"args":    args,
			}).Debug("Restored last args from cache")
		}

		if len(args) == 0 {
			msg := telegram.NewMessage(
				update.Message.Chat.ID,
				c.L("r.pleaseProvideAtLeastOneTag", nil),
				update.Message.MessageID,
			)
			_, err := c.Tg.Send(msg)
			return err
		}
	} else {
		err := c.cache.Set(
			"db:r:last_args:"+userID,
			[]byte(strings.Join(args, " ")),
			24*time.Hour,
		)
		if err != nil {
			c.Logger.WithError(err).Error("Failed to cache last args")
		} else {
			c.Logger.WithFields(logger.Fields{
				"user_id": userID,
				"args":    args,
			}).Debug("Saved last args to cache")
		}
	}

	var tags []string
	var minScore int

	for _, arg := range args {
		// We support both options: score>100 and s>100
		if strings.HasPrefix(arg, "score>") || strings.HasPrefix(arg, "s>") {
			scoreStr := ""
			if after, ok := strings.CutPrefix(arg, "score>"); ok {
				scoreStr = after
			} else {
				scoreStr = strings.TrimPrefix(arg, "s>")
			}
			if score, err := strconv.Atoi(scoreStr); err == nil {
				minScore = score
			}
		} else {
			tags = append(tags, arg)
		}
	}

	posts, err := c.api.getPosts(tags)
	if err != nil {
		message := c.L("r.failedToGetPosts", map[string]any{
			"Error": err.Error(),
		})
		if errors.Is(err, ErrorAuth) {
			message = c.L("r.missingAuthentication", nil)
		}
		msg := telegram.NewMessage(
			update.Message.Chat.ID,
			message,
			update.Message.MessageID,
		)
		_, err := c.Tg.Send(msg)
		return err
	}

	if minScore > 0 {
		posts = c.api.getPostsWithScore(posts, minScore)
	}

	if len(posts) == 0 {
		var similarTagsFound []string
		for _, tag := range tags {
			c.Logger.WithFields(logger.Fields{
				"searching_similar_for": tag,
			}).Debug("Searching for similar tags")

			if similar := c.api.findSimilarTags(tag); len(similar) > 0 {
				similarTagsFound = append(similarTagsFound, similar...)
				c.Logger.WithFields(logger.Fields{
					"tag":           tag,
					"similar_found": similar,
				}).Debug("Found similar tags")
			}
		}

		if len(similarTagsFound) > 0 {
			uniqueTags := make(map[string]bool)
			for _, tag := range similarTagsFound {
				uniqueTags[tag] = true
			}

			similarTagsFound = similarTagsFound[:0]
			for tag := range uniqueTags {
				similarTagsFound = append(similarTagsFound, tag)
			}

			var buttons [][]telegram.InlineKeyboardButton
			for i := 0; i < len(similarTagsFound); i += 2 {
				var row []telegram.InlineKeyboardButton
				row = append(row, telegram.NewInlineKeyboardButtonData(
					"#"+similarTagsFound[i],
					fmt.Sprintf("r tag:%s", similarTagsFound[i]),
				))
				if i+1 < len(similarTagsFound) {
					row = append(row, telegram.NewInlineKeyboardButtonData(
						"#"+similarTagsFound[i+1],
						fmt.Sprintf("r tag:%s", similarTagsFound[i+1]),
					))
				}
				buttons = append(buttons, row)
			}

			msg := telegram.NewMessage(
				update.Message.Chat.ID,
				c.L("r.noPostsFoundForTagsWithSuggest", map[string]any{
					"Tags": strings.Join(tags, " "),
				}),
				update.Message.MessageID,
			)
			msg.ReplyMarkup = &telegram.InlineKeyboardMarkup{
				InlineKeyboard: buttons,
			}
			_, err := c.Tg.Send(msg)
			return err
		}

		msg := telegram.NewMessage(
			update.Message.Chat.ID,
			c.L("r.noPostsFoundForTagsWithSuggest", map[string]any{
				"Tags": strings.Join(tags, " "),
			}),
			update.Message.MessageID,
		)
		_, err := c.Tg.Send(msg)
		return err
	}

	post := posts[rand.Intn(len(posts))]

	postTags := strings.Fields(strings.TrimSpace(post.Tags))
	var clickableTags []string
	for _, tag := range postTags {
		cleanTag := strings.ReplaceAll(tag, " ", "_")
		escapedTag := escapeMarkdown(cleanTag)
		clickableTags = append(clickableTags, escapedTag)
	}

	var requestedHashtags []string
	for _, tag := range tags {
		cleanTag := strings.ReplaceAll(tag, " ", "_")
		cleanTag = escapeMarkdown(cleanTag)
		requestedHashtags = append(requestedHashtags, "\\#"+cleanTag)
	}

	var stats string
	if len(posts) > 1 {
		minPostScore := posts[len(posts)-1].Score
		maxPostScore := posts[0].Score
		avgScore := calculateAverageScore(posts)
		scoreRange := fmt.Sprintf(
			"%d-%d, %s: %d",
			minPostScore,
			maxPostScore,
			c.L("r.scoreAvg", nil),
			avgScore,
		)
		stats = fmt.Sprintf(
			"\n*%s:* %s",
			c.L("r.scoreRange", nil),
			escapeMarkdown(scoreRange),
		)
	}

	postURL := fmt.Sprintf(
		"%s?page=post&s=view&id=%d",
		strings.ReplaceAll(c.api.baseURL, "api.", ""),
		post.ID,
	)
	postURL = escapeMarkdown(postURL, "=", "&")
	stats += fmt.Sprintf("\n[%s](%s)", c.L("r.fullLabel", nil), postURL)

	// Check and truncate clickableTags if necessary
	tagsPrefix := fmt.Sprintf("*%s*: ", c.L("r.tags", nil))
	scoreAndStats := fmt.Sprintf(
		"\n*%s*: %d%s\n\n",
		c.L("r.score", nil),
		post.Score,
		stats,
	)
	hashtagsPart := strings.Join(requestedHashtags, " ")

	fixedPartsLength := len(tagsPrefix) + len(scoreAndStats) + len(hashtagsPart)
	maxTagsSpace := maxCaptionLength - fixedPartsLength

	tagsString := "`" + strings.Join(clickableTags, "` `") + "`"

	for len(tagsString) > maxTagsSpace && len(clickableTags) > 0 {
		clickableTags = clickableTags[:len(clickableTags)-1]
		if len(clickableTags) > 0 {
			tagsString = "`" + strings.Join(clickableTags, "` `") + "`"
		} else {
			tagsString = ""
		}
	}

	caption := fmt.Sprintf("*%s*: %s\n*%s*: %d%s\n\n%s",
		c.L("r.tags", nil),
		tagsString,
		c.L("r.score", nil),
		post.Score,
		stats,
		strings.Join(requestedHashtags, " "))

	randomTags := getRandomTags(postTags, 4)

	var tagButtons []telegram.InlineKeyboardButton
	for _, tag := range randomTags {
		cleanTag := strings.ReplaceAll(tag, " ", "_")
		tagButtons = append(tagButtons,
			telegram.NewInlineKeyboardButtonData(
				"#"+cleanTag,
				fmt.Sprintf("r tag:%s", cleanTag),
			),
		)
	}

	nextCommand := fmt.Sprintf("r next:%s", strings.Join(tags, " "))
	if minScore > 0 {
		nextCommand += fmt.Sprintf(" s>%d", minScore)
	}

	var keyboard telegram.InlineKeyboardMarkup
	if len(tagButtons) == 0 {
		keyboard = telegram.NewInlineKeyboardMarkup(
			telegram.NewInlineKeyboardRow(
				telegram.NewInlineKeyboardButtonData("🔄 Next", nextCommand),
			),
		)
	} else if len(tagButtons) <= 2 {
		// If tags are 1-2, we place them in one row
		keyboard = telegram.NewInlineKeyboardMarkup(
			telegram.NewInlineKeyboardRow(tagButtons...),
			telegram.NewInlineKeyboardRow(
				telegram.NewInlineKeyboardButtonData("🔄 Next", nextCommand),
			),
		)
	} else {
		// If tags 3-4, place them in two rows
		keyboard = telegram.NewInlineKeyboardMarkup(
			telegram.NewInlineKeyboardRow(tagButtons[:len(tagButtons)/2]...),
			telegram.NewInlineKeyboardRow(tagButtons[len(tagButtons)/2:]...),
			telegram.NewInlineKeyboardRow(
				telegram.NewInlineKeyboardButtonData("🔄 Next", nextCommand),
			),
		)
	}

	isVideo := strings.HasSuffix(strings.ToLower(post.FileURL), ".mp4") ||
		strings.HasSuffix(strings.ToLower(post.FileURL), ".webm")

	c.Logger.WithFields(logger.Fields{
		"caption":  caption,
		"file_url": post.FileURL,
		"is_video": isVideo,
		"post_id":  post.ID,
	}).Debug("Preparing to send media")

	if isVideo {
		video := telegram.NewVideoMessage(update.Message.Chat.ID, telegram.FileURL(post.FileURL), caption, update.Message.MessageID)
		video.ParseMode = telegram.ModeMarkdownV2
		video.ReplyMarkup = keyboard

		_, err = c.Tg.Send(video)
		if strings.Contains(err.Error(), "Bad Request: wrong type") {
			caption = fmt.Sprintf(
				"%s\n%s\n\n%s",
				escapeMarkdown(err.Error()),
				escapeMarkdown(post.FileURL),
				caption,
			)
			msg := telegram.NewMessage(update.Message.Chat.ID, caption, update.Message.MessageID)
			msg.ParseMode = telegram.ModeMarkdownV2
			msg.ReplyMarkup = &keyboard

			_, err = c.Tg.Send(msg)
		}
		return err
	}
	photo := telegram.NewPhotoMessage(update.Message.Chat.ID, telegram.FileURL(post.FileURL), caption, update.Message.MessageID)
	photo.Caption = caption
	photo.ParseMode = telegram.ModeMarkdownV2
	photo.ReplyMarkup = keyboard

	_, err = c.Tg.Send(photo)
	return err
}

func getRandomTags(tags []string, count int) []string {
	if len(tags) <= count {
		return tags
	}

	shuffled := make([]string, len(tags))
	copy(shuffled, tags)

	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled[:count]
}

func calculateAverageScore(posts []Post) int {
	if len(posts) == 0 {
		return 0
	}

	sum := 0
	for _, post := range posts {
		sum += post.Score
	}
	return sum / len(posts)
}
