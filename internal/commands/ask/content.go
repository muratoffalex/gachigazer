package ask

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/ai"
	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/fetch"
	"github.com/muratoffalex/gachigazer/internal/markdown"
	"github.com/muratoffalex/gachigazer/internal/service"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

const (
	URLStatusUnprocessed = iota
	URLStatusProcessing
	URLStatusProcessed
)

type URLInfo struct {
	URL            string    `json:"url"`
	Status         int       `json:"status"`
	ProcessingAt   time.Time `json:"processing_at"`
	ProcessedAt    time.Time `json:"processed_at"`
	Error          string    `json:"error"`
	RetryCount     int       `json:"retry_count"`
	TrimmedContent string    `json:"trimmed_content"`
}

func (s *URLInfo) IsUnprocessed() bool {
	return s.Status == URLStatusUnprocessed
}

func (s *URLInfo) IsProcessing() bool {
	return s.Status == URLStatusProcessing
}

func (s *URLInfo) IsProcessed() bool {
	return s.Status == URLStatusProcessed
}

func (s *URLInfo) MarkProcessing() {
	s.Status = URLStatusProcessing
	s.ProcessingAt = time.Now()
}

func (s *URLInfo) MarkProcessed() {
	s.Status = URLStatusProcessed
	s.ProcessedAt = time.Now()
}

func (s *URLInfo) MarkFailed(err string) {
	s.MarkProcessed()
	s.Error = err
	s.RetryCount++
}

func (s *URLInfo) Duration() string {
	if s.ProcessedAt.IsZero() || s.ProcessingAt.IsZero() {
		return ""
	}
	dur := s.ProcessedAt.Sub(s.ProcessingAt)
	return fmt.Sprintf("%.2fs", dur.Seconds())
}

func (s *URLInfo) GetCurrentStatusSymbol(l *service.Localizer) (symbol string) {
	switch s.Status {
	case URLStatusProcessed:
		if s.Error != "" {
			symbol = l.Localize("error", nil)
		} else {
			symbol = l.Localize("ok", nil)
		}
	case URLStatusUnprocessed:
		symbol = l.Localize("waiting", nil)
	case URLStatusProcessing:
		symbol = l.Localize("processing", nil)
	default:
		symbol = ""
	}
	symbol = strings.ToLower(symbol)
	return
}

func (s *URLInfo) EscapedURL() string {
	return markdown.Escape(s.URL)
}

func (s *URLInfo) FormattedString(withContent bool, l *service.Localizer) string {
	metadata := []string{}
	if symbol := s.GetCurrentStatusSymbol(l); symbol != "" {
		metadata = append(metadata, symbol)
	}
	if dur := s.Duration(); dur != "" {
		metadata = append(metadata, dur)
	}
	metadataString := ""
	if len(metadata) != 0 {
		metadataString = "_" + markdown.Escape(
			fmt.Sprintf("(%s)", strings.Join(metadata, ", ")),
		) + "_"
	}
	formattedString := strings.TrimSpace(fmt.Sprintf(
		"%s %s",
		s.EscapedURL(),
		metadataString,
	))
	if withContent && s.TrimmedContent != "" {
		formattedString += fmt.Sprintf("\n>%s||", strings.ReplaceAll(markdown.Escape(s.TrimmedContent), "\n", "\n> "))
	}
	fmt.Println(formattedString)
	return formattedString
}

type prompt struct {
	Text    string
	Name    string
	Dynamic bool
}

type userInfo struct {
	Name      string
	EncodedID string
}

type forwardOrigin struct {
	Type      string
	Name      string
	EncodedID string
	Username  string
	ChatID    int64
	MessageID int
}

func (o *forwardOrigin) GetShareURL() string {
	if o.Username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", o.Username, o.MessageID)
	}

	return fmt.Sprintf("https://t.me/%d/%d", o.ChatID, o.MessageID)
}

type MessageContent struct {
	Text                      string
	URLs                      map[string]*URLInfo
	URLsContent               map[string]string
	ImageURLs                 []string
	FileURLs                  []string
	Media                     []ai.Content
	HistoryMedia              []ai.Content
	Command                   string
	Args                      map[string]string
	Quote                     string
	Prompt                    prompt
	Context                   []string
	UserInfo                  userInfo
	ReplyMsgContent           *MessageContent
	ForwardOrigin             *forwardOrigin
	Date                      time.Time
	Tools                     []ai.Tool
	ConversationHistory       []conversationMessage
	Summary                   string
	ConversationHistoryLength int
}

func (mc *MessageContent) GetAllMedia(cfg *config.AskCommandConfig, args *CommandArgs) []ai.Content {
	media := mc.Media

	if len(mc.HistoryMedia) > 0 {
		media = append(media, mc.HistoryMedia...)
	} else {
		availableImagesInHistory := cfg.Images.Max - len(mc.GetImagesMedia())
		availableAudioInHistory := cfg.Audio.MaxInHistory - len(mc.GetAudioMedia())
		imageLifetime := cfg.Images.Lifetime
		historyMedia := []ai.Content{}
		for _, item := range mc.ConversationHistory {
			if availableImagesInHistory > 0 && args.HandleImages && (imageLifetime == 0 || time.Now().Before(item.CreatedAt.Add(imageLifetime))) {
				for _, img := range item.Images {
					media = append(media, img)
					availableImagesInHistory--
					if availableImagesInHistory == 0 {
						break
					}
				}
			}
			if availableAudioInHistory > 0 && args.HandleAudio {
				for _, audio := range item.Audio {
					media = append(media, audio)
					availableAudioInHistory--
					if availableAudioInHistory == 0 {
						break
					}
				}
			}
			if args.HandleFiles {
				historyMedia = append(historyMedia, item.Files...)
			}
		}
		mc.HistoryMedia = historyMedia
		media = append(media, historyMedia...)
	}

	return media
}

func (mc *MessageContent) AddConversationHistoryItems(items ...conversationMessage) {
	if mc.ConversationHistory == nil {
		mc.ConversationHistory = make([]conversationMessage, 0)
	}
	mc.ConversationHistory = append(mc.ConversationHistory, items...)
}

func (mc *MessageContent) GetConversationHistoryText() string {
	response := []string{}
	for _, item := range mc.ConversationHistory {
		if item.Text == "" || !item.Role.Supported() {
			continue
		}
		text := item.Text
		if item.Role.IsAssistant() {
			text = "[Assistant (You) answer]\n" + text
		}
		if item.Role.IsTool() {
			text = fmt.Sprintf("[Tool %s response]\n%s", item.ToolName.String, text)
		}
		if item.Role.IsUser() {
			text = "== User message started ==\n" + text + "\n== User message ended =="
		}
		text = strings.TrimSpace(text)
		response = append(response, text)
	}
	slices.Reverse(response)
	return strings.TrimSpace(strings.Join(response, "\n"))
}

func (mc *MessageContent) GetConversationHistoryWithSupportedRoles() []conversationMessage {
	response := []conversationMessage{}
	for _, item := range mc.ConversationHistory {
		if item.Role.Supported() {
			response = append(response, item)
		}
	}
	return response
}

func (mc *MessageContent) IsEmpty() bool {
	return mc.Text == "" && len(mc.Media) == 0 && mc.Prompt.Text == "" && mc.ReplyMsgContent == nil && len(mc.Context) == 0
}

func (mc *MessageContent) HasHistory() bool {
	return len(mc.ConversationHistory) > 0
}

func (mc *MessageContent) ContextTurnsCount() int {
	if mc.ConversationHistoryLength > 0 {
		return mc.ConversationHistoryLength
	}
	turns := map[string]bool{}
	for _, item := range mc.ConversationHistory {
		turns[item.ConversationChainID] = true
	}
	return len(turns) + 1
}

func (mc *MessageContent) GetLatestConversationMessage() *conversationMessage {
	if mc.HasHistory() {
		return &mc.ConversationHistory[0]
	}
	return nil
}

func (mc *MessageContent) GetOldestConversationMessage() *conversationMessage {
	if mc.HasHistory() {
		return &mc.ConversationHistory[len(mc.ConversationHistory)-1]
	}
	return nil
}

func (mc *MessageContent) GetImagesMedia() []ai.Content {
	images := []ai.Content{}
	for _, item := range mc.Media {
		if item.Type == "image_url" {
			images = append(images, item)
		}
	}
	return images
}

func (mc *MessageContent) GetFilesMedia() []ai.Content {
	images := []ai.Content{}
	for _, item := range mc.Media {
		if item.Type == "file" {
			images = append(images, item)
		}
	}
	return images
}

func (mc *MessageContent) GetAudioMedia() []ai.Content {
	items := []ai.Content{}
	for _, item := range mc.Media {
		if item.Type == "input_audio" {
			items = append(items, item)
		}
	}
	return items
}

func (mc *MessageContent) FilterMedia(images, audio, files bool) []ai.Content {
	media := make([]ai.Content, 0)
	if audio {
		media = append(media, mc.GetAudioMedia()...)
	}
	if images {
		media = append(media, mc.GetImagesMedia()...)
	}
	if files {
		media = append(media, mc.GetFilesMedia()...)
	}
	return media
}

func (mc *MessageContent) AddMedia(media ...ai.Content) {
	if mc.Media == nil {
		mc.Media = make([]ai.Content, 0)
	}

	for _, m := range media {
		exists := false
		for _, existing := range mc.Media {
			if existing.Type == m.Type {
				switch m.Type {
				case "image_url":
					if existing.ImageURL.URL == m.ImageURL.URL {
						exists = true
						break
					}
				case "file":
					if existing.File.FileData == m.File.FileData {
						exists = true
						break
					}
				case "text":
					if existing.Text == m.Text {
						exists = true
						break
					}
				case "input_audio":
					if existing.InputAudio.Data == m.InputAudio.Data {
						exists = true
						break
					}
				}
			}
			if exists {
				break
			}
		}

		if !exists {
			mc.Media = append(mc.Media, m)
		}
	}
}

func (mc *MessageContent) AddURLsFromMap(urls map[string]*URLInfo) {
	if mc.URLs == nil {
		mc.URLs = make(map[string]*URLInfo)
	}
	for url := range urls {
		if _, exists := mc.URLs[url]; !exists {
			mc.URLs[url] = &URLInfo{
				URL:    url,
				Status: URLStatusUnprocessed,
			}
		}
	}
}

func (mc *MessageContent) AddURLs(urls ...string) {
	if mc.URLs == nil {
		mc.URLs = make(map[string]*URLInfo)
	}
	for _, url := range urls {
		if _, exists := mc.URLs[url]; !exists {
			mc.URLs[url] = &URLInfo{
				URL:    url,
				Status: URLStatusUnprocessed,
			}
		}
	}
}

func (mc *MessageContent) AddImageURLs(urls ...string) {
	if mc.ImageURLs == nil {
		mc.ImageURLs = []string{}
	}
	mc.ImageURLs = append(mc.ImageURLs, urls...)
}

func (mc *MessageContent) AddFileURLs(urls ...string) {
	if mc.FileURLs == nil {
		mc.FileURLs = []string{}
	}
	mc.FileURLs = append(mc.FileURLs, urls...)
}

func (mc *MessageContent) GetAllURLs() []string {
	var items []string
	for url := range mc.URLs {
		items = append(items, url)
	}
	return items
}

func (mc *MessageContent) GetUnprocessedURLs() []*URLInfo {
	var unprocessed []*URLInfo
	for _, url := range mc.URLs {
		if url.IsUnprocessed() {
			unprocessed = append(unprocessed, url)
		}
	}
	return unprocessed
}

func (mc *MessageContent) GetProcessingURLs() []*URLInfo {
	var processing []*URLInfo
	for _, url := range mc.URLs {
		if url.IsProcessing() {
			processing = append(processing, url)
		}
	}
	return processing
}

func (mc *MessageContent) GetProcessedURLs() []*URLInfo {
	var processed []*URLInfo
	for _, url := range mc.URLs {
		if url.IsProcessed() {
			processed = append(processed, url)
		}
	}
	return processed
}

func (mc *MessageContent) GetProcessedURLsString() []string {
	var processed []string
	for _, url := range mc.URLs {
		if url.IsProcessed() {
			processed = append(processed, url.URL)
		}
	}
	return processed
}

func (mc *MessageContent) GetTextForTitleGenerating() string {
	if text := mc.Text; text != "" {
		finalText := text
		if len(mc.URLsContent) > 0 {
			for url, content := range mc.URLsContent {
				finalText += "[CONTENT from " + url + "]:\n" + content
			}
		}
		return finalText
	}
	return ""
}

func (mc *MessageContent) GetMessageContent() string {
	request := []string{}
	now := time.Now()
	if mc.Summary != "" {
		mc.Context = append(mc.Context, mc.Summary)
	}

	if userPrompt := mc.Prompt.Text; userPrompt != "" {
		request = append(request, "[PROMPT]\n"+userPrompt)
	}

	if context := mc.Context; len(context) > 0 {
		contextStr := strings.Join(context, "\n")
		request = append(request, "[CONTEXT]\n"+contextStr)
	}

	if replyMsg := mc.ReplyMsgContent; replyMsg != nil {
		replyHeader := fmt.Sprintf("[REPLY TO: %s(%s)",
			replyMsg.UserInfo.Name,
			replyMsg.UserInfo.EncodedID,
		)

		if fo := replyMsg.ForwardOrigin; fo != nil {
			replyHeader += formatForwardOrigin(fo)
		}

		replyHeader += fmt.Sprintf(" @%s]\n%s",
			now.Format("Jan02 15:04"),
			replyMsg.Text)

		request = append(request, replyHeader)
	}

	if quote := mc.Quote; quote != "" {
		request = append(request, "[QUOTE]\n"+quote)
	}

	userInfo := mc.UserInfo
	userHeader := fmt.Sprintf("[USER: %s(%s) @%s]",
		userInfo.Name,
		userInfo.EncodedID,
		now.Format("Jan02 15:04"),
	)

	finalText := ""
	if text := mc.Text; text != "" {
		finalText = userHeader + "\n" + text
	} else {
		finalText = userHeader + " [NO TEXT]"
	}

	if len(mc.URLsContent) > 0 {
		for url, content := range mc.URLsContent {
			finalText += "\n\n[CONTENT from " + url + "]:\n" + content
		}
	}

	request = append(request, finalText)

	finalContent := strings.Join(request, "\n")
	finalContent = strings.TrimSpace(finalContent)
	return finalContent
}

type MessageContext struct {
	ChatID    int64
	MessageID int
	Content   MessageContent
}

// ExtractMessageContent processes a message and extracts all relevant content
func (c *Command) ExtractMessageContent(msg *telegram.MessageOriginal, isCurrentMessage bool) *MessageContent {
	content := &MessageContent{
		URLsContent: map[string]string{},
		Args:        map[string]string{},
	}

	if msg == nil {
		return content
	}

	// Extract and clean text content
	content.Text, content.Command, content.Args = extractMessageText(msg, isCurrentMessage)

	// Extract URLs and images
	var imageURLs []string
	var fileURLs []string
	var URLs []string
	URLs, imageURLs, fileURLs = c.extractURLsFromMessage(msg)

	content.AddURLs(URLs...)
	content.AddImageURLs(imageURLs...)
	content.AddFileURLs(fileURLs...)

	// Extract media files
	content.Media = c.extractMediaFromMessage(msg)

	return content
}

func extractMessageText(msg *telegram.MessageOriginal, isCurrentMessage bool) (string, string, map[string]string) {
	var text string
	switch {
	case msg.Text != "":
		text = msg.Text
	case msg.Caption != "":
		text = msg.Caption
	default:
		text = ""
	}

	if msg.Poll != nil {
		pollInfo := fmt.Sprintf("\n\n[POLL: %s]\n", msg.Poll.Question)
		for _, option := range msg.Poll.Options {
			pollInfo += fmt.Sprintf("- %s (%d votes)\n", option.Text, option.VoterCount)
		}
		text += pollInfo
	}

	if text == "" {
		return "", "", nil
	}

	// For history messages, remove any command prefix
	command := ""
	if IsCommand(text) {
		parts := strings.Fields(text)
		if len(parts) > 0 && strings.HasPrefix(parts[0], "/") {
			command = strings.TrimPrefix(parts[0], "/")
			text = strings.TrimSpace(strings.TrimPrefix(text, parts[0]))
		}
	}

	if isCurrentMessage {
		// For current message, parse and remove arguments
		args, cleanedText := parseArgs(text)
		return cleanedText, command, args
	}

	return strings.TrimSpace(text), "", nil
}

func IsCommand(text string) bool {
	return strings.HasPrefix(text, "/")
}

func (c *Command) extractURLsFromMessage(msg *telegram.MessageOriginal) ([]string, []string, []string) {
	var urls []string

	if msg.Text != "" && msg.Entities != nil {
		urls = append(urls, c.ExtractURLsFromEntities(msg.Text, msg.Entities)...)
	}
	if msg.Caption != "" && msg.CaptionEntities != nil {
		urls = append(urls, c.ExtractURLsFromEntities(msg.Caption, msg.CaptionEntities)...)
	}

	urls = append(urls, fetch.ExtractStrictURLs(msg.Text)...)
	urls = append(urls, fetch.ExtractStrictURLs(msg.Caption)...)

	return c.filterURLs(urls)
}

func (c *Command) filterURLs(urls []string) ([]string, []string, []string) {
	var imageURLs []string
	var fileURLs []string
	var filteredURLs []string
	// Filter image URLs
	for _, url := range urls {
		isImg, imgErr := telegram.IsAvailableImageURL(url)
		isFile, fileErr := telegram.IsAvailableFileURL(url)
		if isImg && imgErr == nil {
			imageURLs = append(imageURLs, url)
		} else if isFile && fileErr == nil {
			fileURLs = append(fileURLs, url)
		} else if !isImg && !isFile {
			filteredURLs = append(filteredURLs, url)
		}
	}

	// Post-process URLs
	filteredURLs = c.postProcessURLs(filteredURLs)

	// Deduplicate
	filteredURLs = uniqueSlice(filteredURLs)
	imageURLs = uniqueSlice(imageURLs)
	fileURLs = uniqueSlice(fileURLs)

	return filteredURLs, imageURLs, fileURLs
}

func (c *Command) postProcessURLs(urls []string) []string {
	var processed []string
	for _, u := range urls {
		u = strings.TrimRight(u, "),")
		parsed, err := url.Parse(u)
		if err != nil {
			continue
		}
		if !c.Cfg.GetAskCommandConfig().Fetcher.CheckURL(parsed.String()) {
			continue
		}

		if strings.Contains(u, ".gz") ||
			strings.Contains(u, ".tar") ||
			strings.Contains(u, ".zip") ||
			strings.Contains(u, ".rar") ||
			strings.Contains(u, ".7z") ||
			strings.Contains(u, ".exe") ||
			strings.Contains(u, ".apk") {
			continue
		}

		if strings.Contains(parsed.Host, "vk.com") ||
			strings.Contains(parsed.Host, "instagram") ||
			strings.Contains(parsed.Host, "rule34.xxx") {
			continue
		}

		if strings.Contains(parsed.Host, "youtube.com") {
			if strings.HasPrefix(parsed.Path, "/@") {
				continue
			}
		}

		if parsed.RawQuery != "" {
			query := parsed.Query()
			query.Del("si")      // YouTube session ID
			query.Del("pp")      // Paid promotion
			query.Del("feature") // source
			query.Del("clid")
			query.Del("rid")
			query.Del("utm_source")
			query.Del("utm_medium")
			query.Del("utm_name")
			query.Del("utm_term")
			query.Del("utm_content")
			parsed.RawQuery = query.Encode()
		}

		parsed.Fragment = ""

		if strings.HasPrefix(parsed.Host, "youtu.be") {
			parsed.RawQuery = ""
		}

		if strings.HasPrefix(parsed.Host, "telegram.me") {
			parsed.Host = "t.me"
		}
		if strings.HasPrefix(parsed.Host, "t.me") && (strings.HasPrefix(parsed.Path, "/+") || strings.HasPrefix(parsed.Path, "/c/")) {
			continue
		}
		// Telegram processing
		if strings.HasPrefix(parsed.Host, "t.me") {
			pathParts := strings.Split(parsed.Path, "/")
			// link to channel, handle this with tools if needed
			if len(pathParts) == 2 {
				continue
			}
			if len(pathParts) >= 3 && pathParts[1] != "s" {
				parsed.Path = "/s" + parsed.Path
			}
		}

		processed = append(processed, strings.TrimSuffix(parsed.String(), "/"))
	}
	return processed
}

func (c *Command) extractMediaFromMessage(msg *telegram.MessageOriginal) []ai.Content {
	var media []ai.Content
	if msg == nil {
		return media
	}

	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		if fileURL, err := c.Tg.GetFileURL(photo.FileID); err == nil {
			if content, err := createImageContentFromTelegram(fileURL); err == nil {
				media = append(media, content)
			} else {
				c.Logger.WithError(err).Error("Error creating image content from telegram")
			}
		}
	}

	if msg.Document != nil {
		if strings.HasSuffix(strings.ToLower(msg.Document.FileName), ".pdf") {
			if fileURL, err := c.Tg.GetFileURL(msg.Document.FileID); err == nil {
				if content, err := createFileContentFromTelegram(msg.Document.FileName, fileURL, "application/pdf"); err == nil {
					media = append(media, content)
				} else {
					c.Logger.WithError(err).Error("Error creating file content from telegram")
				}
			}
		}
	}

	if msg.Audio != nil {
		file := msg.Audio
		audio, err := c.handleAudioFile(file.FileID, file.MimeType, file.Duration, file.FileSize)
		if err == nil {
			media = append(media, *audio)
		} else {
			c.Logger.WithError(err).Error("")
		}
	}

	if msg.Voice != nil {
		file := msg.Voice
		audio, err := c.handleAudioFile(file.FileID, file.MimeType, file.Duration, file.FileSize)
		if err == nil {
			media = append(media, *audio)
		} else {
			c.Logger.WithError(err).Error("")
		}
	}

	return media
}

func (c *Command) handleAudioFile(fileID, mimeType string, duration int, size int64) (*ai.Content, error) {
	maxSize := c.Cfg.GetAskCommandConfig().Audio.MaxSize * 1000 // convert kb in bytes
	maxDuration := c.Cfg.GetAskCommandConfig().Audio.MaxDuration
	if duration > maxDuration {
		return nil, fmt.Errorf("audio duration bigger than max duration (%d > %d)", duration, maxDuration)
	}
	if int(size) > maxSize {
		return nil, fmt.Errorf("file size bigger than max size (%d > %d)", size, maxSize)
	}
	if fileURL, err := c.Tg.GetFileURL(fileID); err == nil {
		if content, err := convertAndCreateAudioContent(fileURL, mimeType); err == nil {
			return &content, nil
		} else {
			c.Logger.WithError(err).Error("Error creating audio content from telegram")
			return nil, err
		}
	} else {
		c.Logger.WithError(err).Error("Fail get file url")
		return nil, err
	}
}

func createImageContent(urlOrBase64 string) ai.Content {
	return ai.Content{
		Type: "image_url",
		ImageURL: struct {
			URL string `json:"url"`
		}{URL: urlOrBase64},
	}
}

func createFileContent(filename, url string) ai.Content {
	return ai.Content{
		Type: "file",
		File: struct {
			Filename string `json:"filename"`
			FileData string `json:"file_data"`
		}{
			Filename: filename,
			FileData: url,
		},
	}
}

func uniqueSlice(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func downloadFile(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func fileToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func createImageContentFromTelegram(url string) (ai.Content, error) {
	data, err := downloadFile(url)
	if err != nil {
		return ai.Content{}, err
	}

	var mimeType string
	switch {
	case len(data) > 8 && string(data[0:8]) == "\x89PNG\r\n\x1a\n":
		mimeType = "png"
	case len(data) > 2 && string(data[0:2]) == "\xff\xd8":
		mimeType = "jpeg"
	case len(data) > 4 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		mimeType = "webp"
	default:
		return ai.Content{}, errors.New("unsupported image type")
	}

	base64Str := fileToBase64(data)
	return createImageContent("data:image/" + mimeType + ";base64," + base64Str), nil
}

func createFileContentFromTelegram(filename, url, mimeType string) (ai.Content, error) {
	data, err := downloadFile(url)
	if err != nil {
		return ai.Content{}, err
	}

	base64Str := fileToBase64(data)
	return createFileContent(filename, fmt.Sprintf("data:%s;base64,%s", mimeType, base64Str)), nil
}

func convertAndCreateAudioContent(url, mimeType string) (ai.Content, error) {
	data, err := downloadFile(url)
	if err != nil {
		return ai.Content{}, err
	}

	base64Str := base64.StdEncoding.EncodeToString(data)
	return ai.Content{
		Type: "input_audio",
		InputAudio: struct {
			Data   string "json:\"data\""
			Format string "json:\"format\""
		}{
			Data:   base64Str,
			Format: mimeType,
		},
	}, nil
}
