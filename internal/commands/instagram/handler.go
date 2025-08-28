package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Davincible/goinsta/v3"

	"github.com/muratoffalex/gachigazer/internal/app/di"
	"github.com/muratoffalex/gachigazer/internal/cache"
	"github.com/muratoffalex/gachigazer/internal/commands"
	"github.com/muratoffalex/gachigazer/internal/commands/base"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

const (
	CommandName = "instagram"

	maxCaptionLength = 1024
)

type BotInterface interface {
	GetCommands() map[string]commands.Command
}

type Command struct {
	*base.Command
	insta                  *goinsta.Instagram
	cache                  cache.Cache
	sessionRefreshInterval time.Duration
	bot                    BotInterface
}

type instagramOperation[T any] func() (T, error)

func (c *Command) Name() string {
	return CommandName
}

func (c *Command) Aliases() []string {
	return []string{"ig", "i", "insta", "inst"}
}

type CachedInstagramPost struct {
	MediaURLs []string `json:"media_urls"`
	Caption   string   `json:"caption"`
}

func New(ctx context.Context, di *di.Container, bot BotInterface) (*Command, error) {
	proxyURL := di.Cfg.HTTP().GetProxy()

	if di.Cfg.Chrome().Path != "" {
		chromeOpts := di.Cfg.Chrome().Opts
		if proxyURL != "" {
			chromeOpts = append(chromeOpts,
				fmt.Sprintf("--proxy-server=%s", proxyURL),
				"--ignore-certificate-errors",
			)
		}
		os.Setenv("CHROME_PATH", di.Cfg.Chrome().Path)
		os.Setenv("CHROME_OPTS", strings.Join(chromeOpts, " "))
	}

	di.Logger.Info("Initializing Instagram command")
	cmd := &Command{
		cache: di.Cache,
	}
	cmd.Command = base.NewCommand(cmd, di)

	insta, err := initInstagramClient(di, proxyURL)
	if err != nil {
		return nil, err
	}

	cmd.insta = insta
	cmd.sessionRefreshInterval = di.Cfg.Instagram().SessionRefreshInterval
	cmd.bot = bot

	go cmd.startSessionRefresher()

	return cmd, nil
}

func initInstagramClient(di *di.Container, proxyURL string) (*goinsta.Instagram, error) {
	sessionPath := di.Cfg.Instagram().SessionPath

	if _, err := os.Stat(sessionPath); err == nil {
		insta, err := goinsta.Import(sessionPath)
		if err != nil {
			di.Logger.WithError(err).Warn("Failed to load Instagram session, creating new one")
		} else {
			setupProxyForInstagram(insta, proxyURL, di.Logger)
			di.Logger.Info("Instagram session loaded successfully")
			return insta, nil
		}
	}

	di.Logger.Info("No existing Instagram session found, creating a new one")

	username, password := di.Cfg.Instagram().Credentials()
	insta := goinsta.New(username, password)

	setupProxyForInstagram(insta, proxyURL, di.Logger)

	if err := insta.Login(); err != nil {
		return nil, fmt.Errorf("failed to login to Instagram: %w", err)
	}

	di.Logger.Info("Logged in to Instagram successfully")

	if err := insta.Export(sessionPath); err != nil {
		di.Logger.WithError(err).Warn("Failed to save Instagram session")
	} else {
		di.Logger.Info("Instagram session saved successfully")
	}

	return insta, nil
}

func setupProxyForInstagram(insta *goinsta.Instagram, proxyURL string, logger logger.Logger) {
	if proxy, err := url.Parse(proxyURL); err == nil && proxy.String() != "" {
		if err := insta.SetProxy(proxy.String(), false, true); err != nil {
			logger.WithError(err).Warn("Failed to set proxy for Instagram client")
		} else {
			logger.Info("Proxy configured for Instagram: " + proxy.Redacted())
		}
	}
}

func executeWithRelogin[T any](c *Command, op instagramOperation[T]) (T, error) {
	result, err := op()
	if err != nil && (strings.Contains(err.Error(), "logged out") || strings.Contains(err.Error(), "login required") || strings.Contains(err.Error(), "not authorized") || strings.Contains(err.Error(), "checkpoint required") || strings.Contains(err.Error(), "challenge required")) {
		// Try to relogin and retry once
		if rerr := c.relogin(); rerr != nil {
			var zero T
			return zero, fmt.Errorf("failed to relogin: %w", rerr)
		}
		// Retry operation after relogin
		result, err = op()
		if err != nil {
			var zero T
			return zero, fmt.Errorf("failed after relogin: %w", err)
		}
	}
	return result, err
}

func (c *Command) relogin() error {
	c.Logger.Info("Attempting to relogin to Instagram")

	username, password := c.Cfg.Instagram().Credentials()
	insta := goinsta.New(
		username,
		password,
	)

	if err := insta.Login(); err != nil {
		return fmt.Errorf("failed to relogin to Instagram: %w", err)
	}

	// Save new session
	if err := insta.Export(c.Cfg.Instagram().SessionPath); err != nil {
		c.Logger.WithError(err).Warn("Failed to save Instagram session after relogin")
	} else {
		c.Logger.Info("Instagram session saved successfully after relogin")
	}

	c.insta = insta
	return nil
}

func (c *Command) Execute(update telegram.Update) error {
	var url string
	if update.Message.IsCommand() {
		url = update.Message.CommandArguments()
	} else {
		url = ExtractInstagramURL(update.Message.Text)
	}

	if url == "" {
		msg := telegram.NewMessage(
			update.Message.Chat.ID,
			"Please provide Instagram link",
			update.Message.MessageID,
		)
		_, err := c.Tg.Send(msg)
		return err
	}

	cacheKey := fmt.Sprintf("db:instagram:%s", ExtractShortcode(url))

	if data, found := c.cache.Get(cacheKey); found {
		var cachedPost CachedInstagramPost
		if err := json.Unmarshal(data, &cachedPost); err != nil {
			c.Logger.WithError(err).Error("Failed to unmarshal cached Instagram post")
		} else {
			c.Logger.WithFields(logger.Fields{
				"url": url,
			}).Debug("Retrieved Instagram post from cache")

			if len(cachedPost.MediaURLs) > 1 {
				return c.sendMediaGroup(update.Message.Chat.ID, cachedPost.MediaURLs, cachedPost.Caption, update.Message.MessageID)
			}
			return c.sendMedia(update.Message.Chat.ID, cachedPost.MediaURLs[0], cachedPost.Caption, update.Message.MessageID)
		}
	}

	var mediaType string
	switch {
	case strings.Contains(url, "/stories/"):
		mediaType = "story"
	case strings.Contains(url, "/share/"):
		mediaType = "share"
	default:
		mediaType = "post"
	}

	var mediaURLs []string
	var caption string
	var err error

	switch mediaType {
	case "story":
		mediaURLs, caption, err = c.getStoryContent(url)
	case "share":
		var shortcode string
		shortcode, err = c.getMediaFromShareURL(getShareID(url))
		if err != nil {
			c.Logger.WithError(err).Error("Failed to get content from share link")
			errMsg := telegram.NewMessage(
				update.Message.Chat.ID,
				fmt.Sprintf("Failed to get content from share link: %v", err),
				update.Message.MessageID,
			)
			_, _ = c.Tg.Send(errMsg)
			return err
		}
		url = fmt.Sprintf("https://www.instagram.com/reel/%s/", shortcode)
		mediaURLs, caption, err = c.getMediaFromPost(url)
	default:
		mediaURLs, caption, err = c.getMediaFromPost(url)
	}

	if err != nil {
		// if strings.Contains(err.Error(), "challenge") {
		// 	if cmd, exists := c.bot.GetCommands()[youtube.CommandName]; exists {
		// 		return cmd.Handle(update)
		// 	}
		// 	return err
		// }
		return err
	}

	cachedPost := CachedInstagramPost{
		MediaURLs: mediaURLs,
		Caption:   caption,
	}

	if data, err := json.Marshal(cachedPost); err == nil {
		if err := c.cache.Set(cacheKey, data, 24*time.Hour); err != nil {
			c.Logger.WithError(err).Error("Failed to cache Instagram post")
		} else {
			c.Logger.WithFields(logger.Fields{
				"url": url,
			}).Debug("Successful cached")
		}
	}

	// sending media
	if len(mediaURLs) > 1 {
		return c.sendMediaGroup(update.Message.Chat.ID, mediaURLs, caption, update.Message.MessageID)
	}
	return c.sendMedia(update.Message.Chat.ID, mediaURLs[0], caption, update.Message.MessageID)
}

func (c *Command) getMediaFromPost(url string) ([]string, string, error) {
	c.Logger.WithFields(logger.Fields{
		"url": url,
	}).Info("Extracting media from post")

	shortcode := ExtractShortcode(url)
	if shortcode == "" {
		return nil, "", fmt.Errorf("invalid Instagram URL")
	}

	c.Logger.WithFields(logger.Fields{
		"shortcode": shortcode,
	}).Debug("Extracting media")

	media, err := executeWithRelogin(c, func() (*goinsta.FeedMedia, error) {
		return c.insta.GetMedia(shortcode)
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get media: %w", err)
	}

	var urls []string
	var caption string
	var username string

	for _, item := range media.Items {
		if username == "" && item.User.Username != "" {
			username = item.User.Username
		}
		if caption == "" && item.Caption.Text != "" {
			caption = item.Caption.Text
		}

		switch item.MediaType {
		case 1: // Photo
			if len(item.Images.Versions) > 0 {
				urls = append(urls, goinsta.GetBest(item.Images.Versions))
			}
		case 2: // Video
			if len(item.Videos) > 0 {
				urls = append(urls, goinsta.GetBest(item.Videos))
			}
		case 8: // Carousel
			for _, carousel := range item.CarouselMedia {
				if len(carousel.Videos) > 0 {
					urls = append(urls, goinsta.GetBest(carousel.Videos))
				} else if len(carousel.Images.Versions) > 0 {
					urls = append(urls, goinsta.GetBest(carousel.Images.Versions))
				}
			}
		}
	}

	if len(urls) == 0 {
		return nil, "", fmt.Errorf("no media found in post")
	}

	if len(caption) > maxCaptionLength {
		caption = caption[:maxCaptionLength-3] + "..."
	}

	if username != "" {
		sanitizedUsername := username
		if caption != "" {
			caption = caption + "\n\n#" + sanitizedUsername
		} else {
			caption = "#" + sanitizedUsername
		}
	}

	return urls, caption, nil
}

func (c *Command) sendMedia(chatID int64, url string, caption string, replyToID int) error {
	if strings.Contains(url, ".mp4") {
		video := telegram.NewVideoMessage(chatID, telegram.FileURL(url), caption, replyToID)
		_, err := c.Tg.Send(video)
		return err
	}

	photo := telegram.NewPhotoMessage(chatID, telegram.FileURL(url), caption, replyToID)
	_, err := c.Tg.Send(photo)
	return err
}

func (c *Command) sendMediaGroup(chatID int64, mediaURLs []string, caption string, replyToID int) error {
	const maxItemsPerGroup = 10
	const baseRetryDelay = 5 * time.Second
	const maxRetries = 3

	numGroups := (len(mediaURLs) + maxItemsPerGroup - 1) / maxItemsPerGroup
	var firstGroupMessageID int

	for i := range numGroups {
		start := i * maxItemsPerGroup
		end := min((i+1)*maxItemsPerGroup, len(mediaURLs))
		currentBatch := mediaURLs[start:end]

		var lastErr error
		for retry := range maxRetries {
			if retry > 0 {
				delay := baseRetryDelay * time.Duration(1<<uint(retry))
				c.Logger.WithFields(logger.Fields{
					"group": i + 1,
					"retry": retry,
					"delay": delay,
					"total": numGroups,
				}).Info("Retrying media group send after delay")
				time.Sleep(delay)
			}

			var mediaGroup []telegram.InputMedia
			for j, url := range currentBatch {
				var inputMedia telegram.InputMedia

				if strings.Contains(url, ".mp4") {
					video := telegram.NewVideoMedia(telegram.FileURL(url))
					inputMedia = &video
				} else {
					photo := telegram.NewPhotoMedia(telegram.FileURL(url))
					inputMedia = &photo
				}

				if i == 0 && j == 0 && caption != "" {
					if video, ok := inputMedia.(*telegram.VideoMedia); ok {
						video.Caption = caption
						inputMedia = video
					} else if photo, ok := inputMedia.(*telegram.PhotoMedia); ok {
						photo.Caption = caption
						inputMedia = photo
					}
				}

				mediaGroup = append(mediaGroup, inputMedia)
			}

			config := telegram.NewMediaGroupMessage(chatID, mediaGroup)

			if i == 0 {
				config.ReplyTo = replyToID
			} else if firstGroupMessageID != 0 {
				config.ReplyTo = firstGroupMessageID
			}

			rawResp, err := c.Tg.Request(config)
			if err != nil {
				lastErr = err
				if strings.Contains(err.Error(), "Too Many Requests") {
					if retryAfter := extractRetryAfter(err.Error()); retryAfter > 0 {
						delay := time.Duration(retryAfter) * time.Second
						c.Logger.WithFields(logger.Fields{
							"group": i + 1,
							"retry": retry,
							"delay": delay,
							"total": numGroups,
							"error": err,
						}).Debug("Rate limit hit, waiting specified time")
						time.Sleep(delay)
					}
					continue
				}
				continue
			}

			var msgs []telegram.Message
			if err := json.Unmarshal(rawResp.Result, &msgs); err != nil {
				return fmt.Errorf("failed to parse response for group %d/%d: %w", i+1, numGroups, err)
			}

			// Save ID of the first message of the first group
			if i == 0 && len(msgs) > 0 {
				firstGroupMessageID = msgs[0].MessageID
			}

			c.Logger.WithFields(logger.Fields{
				"group":            i + 1,
				"total":            numGroups,
				"items":            len(currentBatch),
				"chat_id":          chatID,
				"reply_to":         replyToID,
				"first_message_id": firstGroupMessageID,
			}).Info("Sent media group")

			if i < numGroups-1 {
				time.Sleep(2 * time.Second)
			}

			lastErr = nil
			break
		}

		if lastErr != nil {
			return fmt.Errorf("failed to send media group %d/%d after %d retries: %w",
				i+1, numGroups, maxRetries, lastErr)
		}
	}

	return nil
}

// extractRetryAfter attempts to extract the retry_after value from Telegram's error message
func extractRetryAfter(errMsg string) int {
	// Try to find "retry after X" in the error message
	parts := strings.Split(errMsg, "retry after ")
	if len(parts) != 2 {
		return 0
	}

	// Try to parse the number
	seconds, err := strconv.Atoi(strings.Split(parts[1], ":")[0])
	if err != nil {
		return 0
	}

	return seconds
}

func (c *Command) getMediaFromShareURL(shareID string) (string, error) {
	c.Logger.WithFields(logger.Fields{
		"share_id": shareID,
		"method":   "getMediaFromShareURL",
	}).Debug("Starting share URL processing")

	url := fmt.Sprintf("https://www.instagram.com/share/reel/%s/", shareID)

	req, err := http.NewRequest("GET", url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// using web browser headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")

	// Creating a client that will follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	c.Logger.WithFields(logger.Fields{
		"status": resp.StatusCode,
		"url":    resp.Request.URL.String(),
	}).Debug("Final URL after redirects")

	finalURL := resp.Request.URL.String()

	if !ContainsInstagramURL(finalURL) {
		return "", fmt.Errorf("invalid Instagram URL: %s", finalURL)
	}

	shortcode := ExtractShortcode(finalURL)
	if shortcode == "" {
		return "", fmt.Errorf("could not extract shortcode from URL: %s", finalURL)
	}

	c.Logger.WithFields(logger.Fields{
		"shortcode": shortcode,
	}).Debug("Got redirected URL")

	return shortcode, nil
}

func (c *Command) getStoryContent(url string) ([]string, string, error) {
	c.Logger.WithFields(logger.Fields{
		"url": url,
	}).Info("Processing story URL")

	username := ExtractUsernameFromStoryURL(url)
	if username == "" {
		return nil, "", fmt.Errorf("could not extract username from story URL")
	}

	c.Logger.WithFields(logger.Fields{
		"username": username,
	}).Debug("Extracted username")

	storyID := ExtractShortcode(url)
	if storyID == "" {
		return nil, "", fmt.Errorf("could not extract story ID")
	}

	c.Logger.WithFields(logger.Fields{
		"story_id": storyID,
		"type":     "story",
	}).Debug("Extracting story media")

	// get user profile
	user, err := executeWithRelogin(c, func() (*goinsta.User, error) {
		return c.insta.Profiles.ByName(username)
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// get user stories
	stories, err := executeWithRelogin(c, func() (*goinsta.StoryMedia, error) {
		return user.Stories()
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get user stories: %w", err)
	}

	var urls []string
	var caption string

	c.Logger.WithFields(logger.Fields{
		"stories_count": len(stories.Reel.Items),
	}).Debug("Got stories")

	for _, item := range stories.Reel.Items {
		itemID := fmt.Sprintf("%v", item.ID)
		c.Logger.WithFields(logger.Fields{
			"item_id":  itemID,
			"story_id": storyID,
		}).Debug("Comparing story IDs")

		if strings.Split(itemID, "_")[0] == storyID {
			if item.Caption.Text != "" {
				caption = item.Caption.Text
			}

			if len(item.Videos) > 0 {
				urls = append(urls, goinsta.GetBest(item.Videos))
			} else if len(item.Images.Versions) > 0 {
				urls = append(urls, goinsta.GetBest(item.Images.Versions))
			}
			break
		}
	}

	if len(urls) == 0 {
		return nil, "", fmt.Errorf("no media found in story or story expired")
	}

	return urls, caption, nil
}

func getShareID(url string) string {
	shareID := ""
	if strings.Contains(url, "/share/reel/") {
		shareID = strings.TrimPrefix(url, "https://www.instagram.com/share/reel/")
	} else if strings.Contains(url, "/share/p/") {
		shareID = strings.TrimPrefix(url, "https://www.instagram.com/share/p/")
	}
	return strings.TrimSuffix(shareID, "/")
}

// Helper function to find minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Command) startSessionRefresher() {
	interval := c.sessionRefreshInterval
	if interval <= 0 {
		interval = 12 * time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	stopCh := make(chan struct{})
	c.Logger.Info("Instagram session refresher started")

	for {
		select {
		case <-ticker.C:
			if err := c.checkAndRefreshSession(); err != nil {
				c.Logger.WithError(err).Error("Failed to refresh Instagram session")
			} else {
				c.Logger.Info("Instagram session refreshed successfully")
			}
		case <-stopCh:
			c.Logger.Info("Instagram session refresher stopped")
			return
		}
	}
}

func (c *Command) checkAndRefreshSession() error {
	if !c.isSessionValid() {
		c.Logger.Info("Instagram session is invalid, refreshing...")
		return c.relogin()
	}

	c.Logger.Info("Refreshing Instagram session proactively")

	backupPath := c.Cfg.Instagram().SessionPath + ".bak"
	if err := c.insta.Export(backupPath); err != nil {
		c.Logger.WithError(err).Warn("Failed to backup Instagram session")
	}

	return c.relogin()
}

func (c *Command) isSessionValid() bool {
	_, err := executeWithRelogin(c, func() (*goinsta.Account, error) {
		return c.insta.Account, nil
	})
	if err != nil {
		c.Logger.WithError(err).Warn("Instagram session validation failed")
		return false
	}

	return true
}
