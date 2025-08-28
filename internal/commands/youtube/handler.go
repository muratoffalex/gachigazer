package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/lrstanley/go-ytdlp"
	"github.com/muratoffalex/gachigazer/internal/app/di"
	"github.com/muratoffalex/gachigazer/internal/commands/base"
	"github.com/muratoffalex/gachigazer/internal/fetch"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

const CommandName = "youtube"

var youtubeRegex = regexp.MustCompile(`^(https?:\/\/)?(www\.)?(youtube\.com|youtu\.be)\/.+$`)

type Command struct {
	*base.Command
}

func New(di *di.Container) (*Command, error) {
	_, err := ytdlp.Install(context.TODO(), nil)
	if err != nil {
		return nil, err
	}
	cmd := &Command{}
	cmd.Command = base.NewCommand(cmd, di)
	return cmd, nil
}

func (c *Command) Name() string {
	return CommandName
}

func (c *Command) Aliases() []string {
	return []string{"y", "yt", "v", "video"}
}

func (c *Command) Execute(update telegram.Update) error {
	text := update.Message.Text
	urls := c.ExtractURLsFromEntities(text, update.Message.Entities)
	if len(urls) == 0 {
		urls = c.ExtractURLsFromEntities(update.Message.Caption, update.Message.CaptionEntities)
		if len(urls) == 0 && update.Message.ReplyToMessage != nil {
			urls = c.ExtractURLsFromEntities(update.Message.ReplyToMessage.Text, update.Message.ReplyToMessage.Entities)
			if len(urls) == 0 {
				urls = c.ExtractURLsFromEntities(update.Message.ReplyToMessage.Caption, update.Message.ReplyToMessage.CaptionEntities)
			}
		}
	}
	chatID := update.Message.Chat.ID
	messageID := update.Message.MessageID
	url := ""
	if len(urls) > 0 {
		url = urls[0]
	} else {
		return c.handleError(chatID, 0, messageID, errors.New(c.L("youtube.errorURLNotFound", nil)))
	}
	url, err := cleanURL(url)
	if err != nil {
		return c.handleError(chatID, 0, messageID, errors.New(c.L("youtube.errorIncorrectURL", nil)))
	}

	tempDirectory := strings.TrimSuffix(c.Cfg.Youtube().TempDirectory, "/")
	if tempDirectory != "" {
		tempDirectory += "/"
	} else {
		tempDirectory = os.TempDir() + "/"
	}

	dl := ytdlp.New().
		FormatSort("res,ext:mp4:m4a").
		RecodeVideo("mp4").
		Output("%(id)s.%(ext)s").
		SetWorkDir(tempDirectory).
		MaxFileSize(c.Cfg.Youtube().MaxSize).
		AbortOnError().
		PrintJSON().
		WriteComments()
		// SkipDownload() // interesting option, so I can download all data separately, check the file size, and then download only it

	if proxy := c.Cfg.HTTP().GetProxy(); proxy != "" {
		dl.Proxy(proxy)
	}

	startMessage := telegram.NewMessage(
		chatID,
		c.Localizer.Localize("youtube.download.start", nil),
		messageID,
	)
	msg, err := c.Tg.Send(startMessage)
	if err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}
	startMessageID := msg.MessageID

	c.Logger.WithFields(logger.Fields{
		"url":       url,
		"directory": tempDirectory,
	}).Info("Started download video...")
	output, err := dl.Run(context.TODO(), url)
	if err != nil {
		return c.handleError(
			chatID,
			startMessageID,
			messageID,
			fmt.Errorf(
				c.Localizer.Localize("youtube.errorFailDownloadVideo", nil),
				err,
			),
		)
	}

	files, err := output.GetExtractedInfo()
	if err != nil {
		return c.handleError(
			chatID,
			startMessageID,
			messageID,
			fmt.Errorf(
				c.Localizer.Localize("youtube.errorFailGetFileInfo", nil),
				err,
			),
		)
	}

	if len(files) == 0 || files[0] == nil {
		return c.handleError(
			chatID,
			startMessageID,
			messageID,
			errors.New(c.Localizer.Localize("youtube.errorNoVideoFilesFound", nil)),
		)
	}

	file := files[0]
	filePath := fmt.Sprintf("%s%s.%s", tempDirectory, file.ID, file.Extension)

	defer func() {
		if err := os.Remove(filePath); err != nil {
			c.Logger.WithFields(logger.Fields{
				"file":  filePath,
				"error": err,
			}).Error("Failed to remove video file")
		}
	}()

	comments := ""
	validComments := make([]*ytdlp.ExtractedVideoComment, 0, len(file.Comments))
	for _, comment := range file.Comments {
		if comment.Text != nil && *comment.Text != "" {
			validComments = append(validComments, comment)
		}
	}
	if len(validComments) > 0 {
		// sort by likes
		if validComments[0].LikeCount != nil {
			sort.Slice(validComments, func(i, j int) bool {
				return *validComments[i].LikeCount > *validComments[j].LikeCount
			})
		}
		i := 0
		for _, comment := range validComments {
			if i == 3 {
				break
			}

			commentLines := strings.Split(c.Tg.EscapeText(*comment.Text), "\n")
			for i, line := range commentLines {
				commentLines[i] = ">" + line
			}
			var commentInfo []string
			if comment.Author != nil {
				commentInfo = append(
					commentInfo,
					fmt.Sprintf("*_%s_*", c.Tg.EscapeText(strings.TrimLeft(*comment.Author, "@"))),
				)
			}
			if comment.LikeCount != nil {
				commentInfo = append(
					commentInfo,
					fmt.Sprintf("👍🏻 *_%s_*", c.Tg.EscapeText(fetch.FormatCount(*comment.LikeCount))),
				)
			}

			if timestamp := comment.Timestamp; timestamp != nil {
				releaseDate := time.Unix(int64(*timestamp), 0)
				formattedDate := fmt.Sprintf("*%s*", releaseDate.Format("January 2\\, 2006"))
				commentInfo = append(commentInfo, fmt.Sprintf("*_%s_*", formattedDate))
			}

			commentInfoStr := strings.Join(commentInfo, " · ")

			quotedText := strings.Join(commentLines, "\n")
			commentStr := fmt.Sprintf(
				"**>%s\n%s\n",
				commentInfoStr,
				quotedText,
			)
			comments += commentStr
			i++
		}
		if comments != "" {
			comments = fmt.Sprintf("*%s*\n%s", c.Localizer.Localize("youtube.topComments", map[string]any{
				"Count": 3,
			}), comments)
			comments = "\n\n" + strings.TrimSpace(comments)
		}
	}

	uploaderInfo := ""
	if file.Uploader != nil && file.UploaderURL != nil {
		uploaderInfo = fmt.Sprintf(
			" · [👤 %s](%s)",
			c.Tg.EscapeText(*file.Uploader),
			c.Tg.EscapeText(*file.UploaderURL),
		)
	}

	titleLabel := ""
	if file.Title != nil {
		titleLabel = fmt.Sprintf("[%s](%s)", c.Tg.EscapeText(*file.Title), c.Tg.EscapeText(url))
	}

	var metadataArray []string
	if file.LikeCount != nil {
		likeInfo := fmt.Sprintf("👍🏻 *%s*", c.Tg.EscapeText(fetch.FormatCount(*file.LikeCount)))
		metadataArray = append(metadataArray, likeInfo)
	}

	if file.ViewCount != nil {
		viewInfo := fmt.Sprintf("👁 *%s*", c.Tg.EscapeText(fetch.FormatCount(*file.ViewCount)))
		metadataArray = append(metadataArray, viewInfo)
	}

	if file.CommentCount != nil {
		commentInfo := fmt.Sprintf("💬 *%s*", c.Tg.EscapeText(fetch.FormatCount(*file.CommentCount)))
		metadataArray = append(metadataArray, commentInfo)
	}

	if timestamp := file.Timestamp; timestamp != nil {
		releaseDate := time.Unix(int64(*timestamp), 0)
		formattedDate := fmt.Sprintf("*%s*", releaseDate.Format("January 2\\, 2006"))
		metadataArray = append(metadataArray, formattedDate)
	}

	metadata := strings.Join(metadataArray, " · ")

	caption := fmt.Sprintf(
		"%s%s\n%s%s",
		titleLabel,
		uploaderInfo,
		metadata,
		comments,
	)

	var fileSize int64
	fileSizeStr := ""
	if file.FileSize != nil {
		fileSize = int64(*file.FileSize)
		fileSizeStr = formatFileSize(fileSize)
	} else {
		fileInfo, err := os.Stat(filePath)
		if err == nil {
			fileSize = int64(fileInfo.Size())
			fileSizeStr = formatFileSize(fileSize)
		} else {
			fileSizeStr = c.Localizer.Localize("youtube.unknownSize", nil)
		}
	}

	maxSize, err := parseSize(c.Cfg.Youtube().MaxSize)
	mediaTooLarge := err == nil && maxSize > 0 && fileSize > 0 && fileSize > maxSize
	var outputMessage telegram.Chattable
	if !mediaTooLarge {
		message := telegram.NewVideoMessage(
			update.Message.Chat.ID,
			tgbotapi.FilePath(filePath),
			caption,
			update.Message.MessageID,
		)
		message.ParseMode = telegram.ModeMarkdownV2
		outputMessage = message.ToChattable()

		text := c.Localizer.Localize("youtube.uploadVideoInfo", map[string]any{
			"FileSize": fileSizeStr,
		})
		editedMessage := telegram.NewEditMessageText(
			chatID,
			startMessageID,
			text,
		)
		_, err = c.Tg.Send(&editedMessage)
		if err != nil {
			c.Logger.WithError(err).Error("error send message with file uploading status")
		}
	} else {
		caption = c.Localizer.Localize("youtube.fileTooBig", map[string]any{
			"Size":    c.Tg.EscapeText(fileSizeStr),
			"MaxSize": c.Cfg.Youtube().MaxSize,
			"Caption": caption,
		})
		message := telegram.NewMessage(chatID, caption, messageID)
		message.ParseMode = telegram.ModeMarkdownV2
		message.LinkPreviewDisabled = true
		outputMessage = message.ToChattable()
	}

	c.Logger.WithFields(logger.Fields{
		"url":   url,
		"size":  fileSizeStr,
		"title": *file.Title,
	}).Info("Started upload video...")
	c.Tg.SendChatAction(chatID, telegram.ActionUploadVideo)
	if _, err := c.Tg.RequestRaw(outputMessage); err != nil {
		return c.handleError(chatID, startMessageID, messageID, fmt.Errorf("failed to send video. Text: %s", caption))
	}

	_, err = c.Tg.DeleteMessage(chatID, startMessageID)
	if err != nil {
		c.Logger.WithError(err).WithFields(logger.Fields{
			"chatID":    chatID,
			"messageID": startMessageID,
		}).Error("failed to delete message")
	}

	return nil
}

func (c *Command) handleError(chatID int64, startMessageID int, messageID int, orErr error) error {
	var err error
	if startMessageID != 0 {
		_, err = c.Tg.DeleteMessage(chatID, startMessageID)
		if err != nil {
			c.Logger.WithError(err).WithFields(logger.Fields{
				"chatID":    chatID,
				"messageID": startMessageID,
			}).Error("failed to delete message")
		}
	}
	text := orErr.Error()
	if text != "" {
		text = capitalizeFirst(text)
	}
	answer := telegram.NewMessage(chatID, text, messageID)
	answer.LinkPreviewDisabled = true
	_, err = c.Tg.Send(&answer)
	if err != nil {
		c.Logger.WithError(err).WithFields(logger.Fields{
			"text": text,
		}).Error("failed to send message")
	}
	return orErr
}

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func cleanURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.RawQuery != "" {
		query := u.Query()
		query.Del("si")      // YouTube session ID
		query.Del("pp")      // Paid promotion
		query.Del("feature") // source
		query.Del("clid")
		query.Del("rid")
		query.Del("referrer_clid")
		u.RawQuery = query.Encode()
	}
	u.Fragment = ""
	return u.String(), nil
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func parseSize(sizeStr string) (int64, error) {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))
	if sizeStr == "" {
		return 0, nil
	}

	var multiplier float64 = 1
	switch {
	case strings.HasSuffix(sizeStr, "K"):
		multiplier = 1024
		sizeStr = strings.TrimSuffix(sizeStr, "K")
	case strings.HasSuffix(sizeStr, "M"):
		multiplier = 1024 * 1024
		sizeStr = strings.TrimSuffix(sizeStr, "M")
	case strings.HasSuffix(sizeStr, "G"):
		multiplier = 1024 * 1024 * 1024
		sizeStr = strings.TrimSuffix(sizeStr, "G")
	}

	size, err := strconv.ParseFloat(sizeStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size format: %v", err)
	}

	return int64(size * multiplier), nil
}
