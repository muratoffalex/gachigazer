package fetch

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/network"
	"github.com/muratoffalex/gachigazer/internal/telegram"
	"golang.org/x/net/html/charset"
)

const (
	ContentTypeText  = "text"
	ContentTypeURL   = "url"
	ContentTypeImage = "image"
)

type RequestPayload struct {
	URL     string
	Headers map[string]string
}

type Content struct {
	Type string
	Text string
}

type Response struct {
	Content []Content
	IsError bool
}

type Fetcher struct {
	client *http.Client
	logger logger.Logger
	proxy  string
}

var instance *Fetcher

func GetFetcher() *Fetcher {
	return instance
}

func NewFetcher(proxy string, logger logger.Logger) *Fetcher {
	httpConfig := network.NewDefaultHTTPClientConfig(proxy)
	httpConfig.DisableKeepAlives = true
	httpConfig.MaxIdleConns = 10
	httpConfig.IdleConnTimeout = 10 * time.Second
	httpConfig.Timeout = 30 * time.Second
	httpClient := network.SetupHTTPClient(httpConfig, logger)

	instance = &Fetcher{
		client: httpClient,
		logger: logger,
		proxy:  proxy,
	}
	return instance
}

func (r Response) GetURLs() []string {
	urls := []string{}
	for _, item := range r.Content {
		if item.Type == ContentTypeURL {
			urls = append(urls, item.Text)
		}
	}
	return urls
}

func (r Response) GetImages() []string {
	urls := []string{}
	for _, item := range r.Content {
		if item.Type == ContentTypeImage {
			urls = append(urls, item.Text)
		}
	}
	return urls
}

func (r Response) GetText() string {
	text := ""
	for _, item := range r.Content {
		if item.Type == ContentTypeText {
			text += item.Text + "\n"
		}
	}
	return strings.TrimSpace(text)
}

var UserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:138.0) Gecko/20100101 Firefox/138.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 YaBrowser/25.4.1.1015 Yowser/2.5 Safari/537.36",
}

func RandomUserAgent() string {
	return UserAgents[rand.Intn(len(UserAgents))]
}

func (f *Fetcher) fetch(payload RequestPayload) (*http.Response, string, error) {
	if _, err := url.ParseRequestURI(payload.URL); err != nil {
		return nil, "", fmt.Errorf("invalid URL: %w", err)
	}
	req, err := http.NewRequest("GET", payload.URL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", RandomUserAgent())
	for k, v := range payload.Headers {
		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "EOF") {
			return nil, "", fmt.Errorf("connection reset by peer (EOF) - possible server issue with %s", payload.URL)
		}
		return nil, "", fmt.Errorf("request failed: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusTooManyRequests: // 429
		return nil, "", fmt.Errorf("rate limit exceeded (429) for %s", payload.URL)
	case http.StatusForbidden: // 403
		return nil, "", fmt.Errorf("access forbidden (403) for %s", payload.URL)
	case http.StatusInternalServerError: // 500
		return nil, "", fmt.Errorf("server error (500) for %s", payload.URL)
	case http.StatusBadGateway: // 502
		return nil, "", fmt.Errorf("bad gateway (502) for %s", payload.URL)
	case http.StatusServiceUnavailable: // 503
		return nil, "", fmt.Errorf("service unavailable (503) for %s", payload.URL)
	case http.StatusGatewayTimeout: // 504
		return nil, "", fmt.Errorf("gateway timeout (504) for %s", payload.URL)
	}

	utf8Reader, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", fmt.Errorf("charset detection failed: %w", err)
	}

	bodyBytes, err := io.ReadAll(utf8Reader)
	if err != nil {
		return nil, "", fmt.Errorf("reading body failed: %w", err)
	}

	return resp, string(bodyBytes), nil
}

func (f *Fetcher) Txt(payload RequestPayload) Response {
	payload.URL = f.preProcessingURL(payload.URL)
	resp, body, err := f.fetch(payload)
	if err != nil {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: err.Error()}},
			IsError: true,
		}
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return f.errorResponse(fmt.Errorf("failed to parse HTML: %v", err))
	}

	u, err := url.Parse(payload.URL)
	if err != nil {
		return f.errorResponse(fmt.Errorf("invalid URL: %v", err))
	}

	switch {
	case strings.Contains(u.Host, "old.reddit.com"):
		return f.parseOldReddit(doc)
	case strings.Contains(u.Host, "reddit.com"):
		return f.parseReddit(doc)
	case strings.Contains(u.Host, "avito.ru"):
		return f.parseAvito(doc)
	case strings.Contains(u.Host, "boosty"):
		return f.parseBoosty(doc)
	case strings.Contains(u.Host, "opennet.ru"):
		return f.parseOpennet(doc)
	case strings.Contains(u.Host, "lenincrew.com"):
		return f.parseLenincrew(doc)
	case strings.Contains(u.Host, "github.com"):
		return f.parseGithubRepo(u.String())
	case u.Host == "t.me":
		return f.parseTelegram(doc, payload.URL)
	case strings.Contains(u.Host, "habr"):
		return f.parseHabr(doc, payload.URL)
	case strings.Contains(u.Host, "youtube.com") || strings.Contains(u.Host, "youtu.be"):
		return f.parseYoutube(payload.URL)
	default:
		return f.parseDefault(doc)
	}
}

func (f *Fetcher) preProcessingURL(URL string) string {
	parsed, _ := url.Parse(URL)
	if strings.HasPrefix(parsed.Host, "reddit.com") ||
		strings.HasPrefix(parsed.Host, "www.reddit.com") {
		parsed.RawQuery = "" // remove all get params
		if strings.Contains(parsed.Path, "/comments/") {
			parsed.Host = "old.reddit.com"
		}
		canonicalURL, err := f.parseRedditURL(URL)
		if err != nil {
			log.Println("Failed to parse Reddit URL: "+URL, err)
		}
		if canonicalURL != "" {
			parsedNew, err := url.Parse(canonicalURL)
			parsedNew.Fragment = ""
			parsedNew.RawQuery = ""
			if err != nil {
				log.Println("Failed to parse canonical Reddit URL: "+URL+". Back to current", err)
			} else {
				parsedNew.Host = "old.reddit.com"
				return parsedNew.String()
			}
		}
	}
	return URL
}

func (f *Fetcher) parseDefault(doc *goquery.Document) Response {
	f.cleanDoc(doc)
	text := doc.Text()
	normalizedText := f.cleanText(text)

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: normalizedText}},
		IsError: false,
	}
}

func (f *Fetcher) cleanText(text string) string {
	lines := strings.Split(text, "\n")
	var chunks []string
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}
		phrases := strings.SplitSeq(trimmedLine, "  ")
		for phrase := range phrases {
			trimmedPhrase := strings.TrimSpace(phrase)
			if trimmedPhrase != "" {
				chunks = append(chunks, trimmedPhrase)
			}
		}
	}
	normalizedText := strings.Join(chunks, " ")
	normalizedText = regexp.MustCompile(`\s+`).ReplaceAllString(normalizedText, " ")
	normalizedText = strings.TrimSpace(normalizedText)
	return normalizedText
}

func (f *Fetcher) cleanDoc(doc *goquery.Document) {
	doc.Find("script, style, footer, nav, aside, .cookie-consent, .promoted-link, .sidebar, .login-form, .signup-form").Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})
}

func (f *Fetcher) parseOldReddit(doc *goquery.Document) Response {
	content := []Content{}
	title := doc.Find("a.title").First().Text()

	siteTable := doc.Find("div#siteTable")

	score := siteTable.Find("div.score.unvoted").First().Text()
	score = strings.TrimSpace(score)

	var metadataParts []string
	siteTable.Find("p.tagline").First().Each(func(i int, s *goquery.Selection) {
		author := s.Find("a.author").Text()
		absoluteTime, timeExists := s.Find("time").Attr("datetime")

		if author != "" {
			metadataParts = append(metadataParts, "Posted by "+author)
		}
		if timeExists {
			metadataParts = append(metadataParts, "at "+absoluteTime)
		}
	})

	var postText strings.Builder
	siteTable.Find("div.usertext-body").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			postText.WriteString(strings.TrimSpace(s.Text()))
		}
	})
	siteTable.Find("a[data-event-action='title']").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			if dataURL, exists := s.Attr("data-href-url"); exists && !telegram.IsImageURL(dataURL) && !strings.Contains(dataURL, "v.redd.it") {
				formattedURL := strings.TrimSpace(dataURL)
				postText.WriteString(" URL: " + strings.TrimSpace(formattedURL))
				content = append(content, Content{Type: ContentTypeURL, Text: formattedURL})
			}
		}
	})

	var images strings.Builder
	siteTable.Find(".post-link img.preview, .gallery-item-thumbnail-link img.preview").Each(func(i int, s *goquery.Selection) {
		if src, exists := s.Attr("src"); exists {
			if ok, err := telegram.IsAvailableImageURL(src); ok && err == nil {
				images.WriteString(fmt.Sprintf("Image: %s\n", src))
			}
		}
	})
	if images.Len() == 0 {
		siteTable.Find("a").Each(func(i int, s *goquery.Selection) {
			if href, exists := s.Attr("href"); exists {
				if ok, err := telegram.IsAvailableImageURL(href); ok && err == nil {
					images.WriteString(fmt.Sprintf("Image: %s\n", href))
				}
			}
		})
	}
	if images.Len() == 0 {
		contentType, exists := doc.Find("meta[property='og:type']").First().Attr("content")
		videoExists := doc.Find("meta[property='og:video']").Length() > 0

		imageType := "Image"
		if exists && contentType == "video" || videoExists {
			imageType = "Video preview"
		}

		doc.Find("meta[property='og:image']").Each(func(i int, s *goquery.Selection) {
			if href, exists := s.Attr("content"); exists {
				ok, err := telegram.IsAvailableImageURL(href)
				// redditstatic - external site logo, redditmedia - external site thumb
				if !strings.Contains(href, "redditstatic") && !strings.Contains(href, "redditmedia") && !strings.Contains(href, "external-preview") && ok && err == nil {
					images.WriteString(fmt.Sprintf("%s: %s\n", imageType, href))
				}
			}
		})
	}

	var comments strings.Builder
	count := 0
	doc.Find("div.commentarea div.sitetable div.comment").Each(func(i int, s *goquery.Selection) {
		count += 1
		if count > 50 {
			return
		}
		author := s.Find("a.author").First().Text()
		text := s.Find("div.usertext-body").Text()
		text = strings.TrimSpace(text)

		commentScore := s.Find("span.score.unvoted").First().Text()
		commentScore = strings.TrimSpace(commentScore)

		if text != "" {
			if author == "" {
				author = "Anonymous"
			}
			if commentScore != "" {
				comments.WriteString(fmt.Sprintf("👤 %s (%s): %s\n", author, commentScore, text))
			} else {
				comments.WriteString(fmt.Sprintf("👤 %s: %s\n", author, text))
			}
		}
	})

	var parts []string

	if title := strings.TrimSpace(title); title != "" {
		parts = append(parts, "TITLE: "+title)
	}

	info := strings.Join(metadataParts, " ")
	if score != "" {
		info += ", " + score + " points"
	}

	parts = append(parts, "INFO: "+info)

	if text := strings.TrimSpace(postText.String()); text != "" {
		parts = append(parts, "TEXT:\n"+text)
	}

	if imgs := strings.TrimSpace(images.String()); imgs != "" {
		parts = append(parts, "IMAGES:\n"+imgs)
	}

	if cmts := strings.TrimSpace(comments.String()); cmts != "" {
		parts = append(parts, "COMMENTS:\n"+cmts)
	}

	fullText := strings.Join(parts, "\n")

	if fullText == "" {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "No old.reddit content found"}},
			IsError: true,
		}
	}

	content = append(content, Content{Type: ContentTypeText, Text: fullText})

	return Response{
		Content: content,
		IsError: false,
	}
}

func (f *Fetcher) parseRedditURL(redditURL string) (string, error) {
	resp, body, err := f.fetch(RequestPayload{URL: redditURL})
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s: %w", redditURL, err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %v", err)
	}

	canonicalDiv := doc.Find("div#canonical-url-updater")
	if canonicalDiv.Length() == 0 {
		return "", fmt.Errorf("canonical URL div not found")
	}

	canonicalURL, exists := canonicalDiv.Attr("value")
	if !exists {
		return "", fmt.Errorf("value attribute not found")
	}
	return canonicalURL, nil
}

func (f *Fetcher) parseReddit(doc *goquery.Document) Response {
	title := doc.Find("h1").First().Text()
	content := doc.Find("div[slot='text-body']").First().Text()

	fullText := strings.TrimSpace(title + "\n" + content)
	normalizedText := strings.Join(strings.Fields(fullText), " ")

	if normalizedText == "" {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "No Reddit post content found"}},
			IsError: true,
		}
	}

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: normalizedText}},
		IsError: false,
	}
}

func (f *Fetcher) parseBoosty(doc *goquery.Document) Response {
	articleContent := doc.Find("article").First().Text()
	comments := doc.Find("div#comments").First().Text()

	fullText := strings.TrimSpace(articleContent)
	if comments != "" {
		fullText += "\n\nComments:\n" + comments
	}

	if fullText == "" {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "No Boosty content found"}},
			IsError: true,
		}
	}

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: fullText}},
		IsError: false,
	}
}

func (f *Fetcher) parseHabr(doc *goquery.Document, url string) Response {
	title := doc.Find("h1.tm-title").First().Text()
	body := doc.Find("div.article-body").First()
	text := f.cleanText(body.Text())
	var images string
	body.Find("img").Each(func(i int, s *goquery.Selection) {
		if src, exists := s.Attr("src"); exists {
			if strings.HasSuffix(src, ".jpeg") || strings.HasSuffix(src, ".jpg") || strings.HasSuffix(src, ".webp") || strings.HasSuffix(src, ".png") {
				images += fmt.Sprintf("Image: %s\n", src)
			}
		}
	})
	rating := doc.Find("span[data-test-id='votes-score-counter']").First().Text()
	date := doc.Find(".tm-article-datetime-published time").First().Text()
	comments := f.parseHabrComments(url)

	if title == "" && text == "" && images == "" && comments == "" {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "No Habr content found"}},
			IsError: true,
		}
	}

	fullText := fmt.Sprintf(
		"Title: %s\nDate: %s\nRating: %s\nText:\n%s\n\nImages:\n%s\nComments:\n%s",
		title,
		date,
		rating,
		text,
		images,
		comments,
	)

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: fullText}},
		IsError: false,
	}
}

func (f *Fetcher) parseHabrComments(articleURL string) string {
	// https://habr.com/ru/articles/717912/
	re := regexp.MustCompile(`\d{6}`)
	articleID := re.FindString(articleURL)
	if articleID == "" {
		return "No valid article ID found in URL"
	}

	resp, body, err := f.fetch(RequestPayload{
		URL: "https://habr.com/kek/v2/articles/" + articleID + "/comments/",
	})
	if err != nil {
		return fmt.Sprintf("Get comments error: %v", err.Error())
	}
	defer resp.Body.Close()

	if len(body) > 0 {
		var comments struct {
			Comments map[string]struct {
				ID       string `json:"id"`
				ParentID string `json:"parentId,omitempty"`
				Message  string `json:"message"`
				Score    int    `json:"score"`
			} `json:"comments"`
		}
		err := json.Unmarshal([]byte(body), &comments)
		if err == nil {
			var result strings.Builder
			i := 0
			for _, comment := range comments.Comments {
				message := strings.TrimPrefix(comment.Message, "<div xmlns=\"http://www.w3.org/1999/xhtml\">")
				message = strings.TrimSuffix(message, "</div>")
				message = strings.TrimPrefix(message, "<p>")
				message = strings.TrimSuffix(message, "</p>")
				if comment.Message == "UFO just landed and posted this here" || comment.Message == "НЛО прилетело и опубликовало эту надпись здесь" {
					continue
				}
				parentIDStr := ""
				if comment.ParentID != "" {
					parentIDStr = fmt.Sprintf(" parentId: %s;", comment.ParentID)
				}
				result.WriteString(fmt.Sprintf("- id: %s;%s score: %d; text: %s\n", comment.ID, parentIDStr, comment.Score, message))
				i++
				if i > 50 {
					break
				}
			}
			return result.String()
		} else {
			return "Unmarshal error " + err.Error()
		}
	}

	return "No comments found"
}

func (f *Fetcher) parseOpennet(doc *goquery.Document) Response {
	// Extract header
	title := doc.Find("table.thdr2").First().Text()
	title = f.cleanOpennetText(title)

	// Main content
	content := doc.Find("td.chtext").First().Text()
	content = f.cleanOpennetText(content)

	// Parsing comments
	var comments strings.Builder
	count := 0
	doc.Find("table.cblk").Each(func(i int, s *goquery.Selection) {
		count += 1
		if count > 50 {
			return
		}
		// Extract number and author
		commentNum := s.Find("td.chdr a[href^='/openforum/'] font").First().Text()
		author := s.Find("td.chdr a.nick").First().Text()

		// Extract date and time
		dateTime := s.Find("td.chdr").Contents().FilterFunction(func(i int, s *goquery.Selection) bool {
			return !s.Is("a, script, img, span")
		}).Text()
		dateTime = regexp.MustCompile(`\d{2}:\d{2}, \d{2}/\d{2}/\d{4}`).FindString(dateTime)

		// Extract rating
		rating := s.Find("span.vt_pp").AttrOr("title", "")

		// Forming information about the commentator
		var commentInfo strings.Builder
		if commentNum != "" {
			commentInfo.WriteString(commentNum)
		}
		if author != "" {
			if commentInfo.Len() > 0 {
				commentInfo.WriteString(", ")
			}
			commentInfo.WriteString(author)
		}
		if dateTime != "" {
			if commentInfo.Len() > 0 {
				commentInfo.WriteString(", ")
			}
			commentInfo.WriteString(dateTime)
		}
		if rating != "" {
			if commentInfo.Len() > 0 {
				commentInfo.WriteString(" | ")
			}
			commentInfo.WriteString(rating)
		}

		// Clear comment text
		commentText := s.Find("td.ctxt").Text()
		commentText = f.cleanOpennetText(commentText)

		// Format the output
		if commentInfo.Len() > 0 || commentText != "" {
			comments.WriteString(fmt.Sprintf(
				"——\n%s\n%s\n",
				commentInfo.String(),
				commentText,
			))
		}
	})

	// Collect the result
	fullText := strings.TrimSpace(
		"TITLE: " + title +
			"\n\nCONTENT:\n" + content +
			"\n\nCOMMENTS:\n" + comments.String(),
	)

	if fullText == "" {
		return f.errorResponse(fmt.Errorf("no opennet content found"))
	}

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: fullText}},
		IsError: false,
	}
}

func (f *Fetcher) cleanOpennetText(text string) string {
	// remove html tags
	text = strings.ReplaceAll(text, "\n", " ")
	// remove square brackets and their contents
	re := regexp.MustCompile(`\[.*?\]`)
	text = re.ReplaceAllString(text, "")
	// remove extra spaces
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}

func (f *Fetcher) parseLenincrew(doc *goquery.Document) Response {
	article := doc.Find("article").First()
	if article.Length() == 0 {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "No article content found"}},
			IsError: true,
		}
	}

	article.Find("script, style, iframe, noscript, .ads").Remove()

	text := article.Text()
	normalizedText := strings.Join(strings.Fields(text), " ")

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: normalizedText}},
		IsError: false,
	}
}

func (f *Fetcher) parseTelegram(doc *goquery.Document, postURL string) Response {
	// extract channel/post_id from URL
	u, err := url.Parse(postURL)
	if err != nil {
		return f.errorResponse(fmt.Errorf("invalid Telegram URL: %v", err))
	}

	urlPath := strings.TrimPrefix(strings.Trim(u.Path, "/"), "s/")

	pathParts := strings.Split(urlPath, "/")
	if len(pathParts) < 2 {
		return f.errorResponse(fmt.Errorf("invalid Telegram URL format"))
	}

	channel := pathParts[0]
	postID := pathParts[1]
	dataPostValue := fmt.Sprintf("%s/%s", channel, postID)

	// looking for a div with the corresponding data-post attribute
	selector := fmt.Sprintf("div[data-post='%s']", dataPostValue)
	postDiv := doc.Find(selector).First()
	if postDiv.Length() == 0 {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: fmt.Sprintf("Failed to find a post %s", dataPostValue)}},
			IsError: true,
		}
	}

	text := postDiv.Find(".tgme_widget_message_text").Text()
	text = strings.TrimSpace(text)

	// If the text is empty, check the media caption
	if text == "" {
		text = postDiv.Find(".tgme_widget_message_caption").Text()
		text = strings.TrimSpace(text)
	}

	text = "Post text: " + text

	// extract the image
	postDiv.Find(".tgme_widget_message_photo_wrap").Each(func(i int, s *goquery.Selection) {
		style, exists := s.Attr("style")
		if !exists {
			return
		}

		re := regexp.MustCompile(`url\(["']?(.*?)["']?\)`)
		matches := re.FindStringSubmatch(style)
		if len(matches) > 1 {
			text += "\nImage: " + matches[1]
		}
	})

	if text == "" {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "Empty message or failed to extract text"}},
			IsError: true,
		}
	}

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: text}},
		IsError: false,
	}
}

func (f *Fetcher) errorResponse(err error) Response {
	return Response{
		Content: []Content{{Type: ContentTypeText, Text: err.Error()}},
		IsError: true,
	}
}
