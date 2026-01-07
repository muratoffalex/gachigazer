package fetcher

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

type RedditFetcher struct {
	BaseFetcher
}

func NewRedditFetcher(l logger.Logger, client HTTPClient) RedditFetcher {
	return RedditFetcher{
		BaseFetcher: NewBaseFetcher(
			FetcherNameReddit,
			"reddit\\.com|redd\\.it",
			client,
			l,
		),
	}
}

func (f RedditFetcher) Handle(request Request) (Response, error) {
	request, _ = NewRequestPayload(
		f.getOldRedditURL(request.URL()),
		request.Headers(),
		request.Options(),
	)

	doc, err := f.getHTML(request)
	if err != nil {
		return f.errorResponse(err)
	}

	siteTable := doc.Find("div#siteTable").First()
	title := doc.Find("a.title").First().Text()

	content := []Content{}
	metadata := f.extractMetadata(siteTable)
	postText, urlContent := f.extractPostTextAndURL(siteTable)
	images := f.extractImages(doc, siteTable)
	comments := f.extractComments(doc)

	fullText := f.buildFullText(title, metadata, postText, images, comments)

	if fullText == "" {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "No old.reddit content found"}},
			IsError: true,
		}, nil
	}

	content = append(content, Content{Type: ContentTypeText, Text: fullText})
	if urlContent != nil {
		content = append(content, *urlContent)
	}

	return Response{
		Content: content,
		IsError: false,
	}, nil
}

type postMetadata struct {
	score         string
	metadataParts []string
}

func (f RedditFetcher) extractMetadata(siteTable *goquery.Selection) postMetadata {
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

	return postMetadata{
		score:         score,
		metadataParts: metadataParts,
	}
}

func (f RedditFetcher) extractPostTextAndURL(siteTable *goquery.Selection) (string, *Content) {
	var postText strings.Builder

	siteTable.Find("div.usertext-body").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			postText.WriteString(strings.TrimSpace(s.Text()))
		}
	})

	var urlContent *Content
	siteTable.Find("a[data-event-action='title']").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			dataURL, exists := s.Attr("data-href-url")
			if exists && !telegram.IsImageURL(dataURL) && !strings.Contains(dataURL, "v.redd.it") {
				formattedURL := strings.TrimSpace(dataURL)
				postText.WriteString(" URL: " + formattedURL)
				urlContent = &Content{Type: ContentTypeURL, Text: formattedURL}
			}
		}
	})

	return postText.String(), urlContent
}

func (f RedditFetcher) extractImages(doc *goquery.Document, siteTable *goquery.Selection) string {
	var images strings.Builder

	// Attempt 1: searching for preview in gallery
	siteTable.Find(".post-link img.preview, .gallery-item-thumbnail-link img.preview").Each(func(i int, s *goquery.Selection) {
		if src, exists := s.Attr("src"); exists {
			if ok, err := telegram.IsAvailableImageURL(src); ok && err == nil {
				fmt.Fprintf(&images, "Image: %s\n", src)
			}
		}
	})

	// Attempt 2: looking for image references
	if images.Len() == 0 {
		siteTable.Find("a").Each(func(i int, s *goquery.Selection) {
			if href, exists := s.Attr("href"); exists {
				if ok, err := telegram.IsAvailableImageURL(href); ok && err == nil {
					fmt.Fprintf(&images, "Image: %s\n", href)
				}
			}
		})
	}

	// Attempt 3: using Open Graph meta tags
	if images.Len() == 0 {
		f.extractOGImages(doc, &images)
	}

	return images.String()
}

func (f RedditFetcher) extractOGImages(doc *goquery.Document, images *strings.Builder) {
	contentType, exists := doc.Find("meta[property='og:type']").First().Attr("content")
	videoExists := doc.Find("meta[property='og:video']").Length() > 0

	imageType := "Image"
	if exists && contentType == "video" || videoExists {
		imageType = "Video preview"
	}

	doc.Find("meta[property='og:image']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("content")
		if !exists {
			return
		}

		ok, err := telegram.IsAvailableImageURL(href)
		isExcluded := strings.Contains(href, "redditstatic") ||
			strings.Contains(href, "redditmedia") ||
			strings.Contains(href, "external-preview")

		if !isExcluded && ok && err == nil {
			fmt.Fprintf(images, "%s: %s\n", imageType, href)
		}
	})
}

func (f RedditFetcher) extractComments(doc *goquery.Document) string {
	var comments strings.Builder
	count := 0

	doc.Find("div.commentarea div.sitetable div.comment").Each(func(i int, s *goquery.Selection) {
		count++
		if count > 50 {
			return
		}

		author := s.Find("a.author").First().Text()
		text := strings.TrimSpace(s.Find("div.usertext-body").Text())
		commentScore := strings.TrimSpace(s.Find("span.score.unvoted").First().Text())

		if text == "" {
			return
		}

		if author == "" {
			author = "Anonymous"
		}

		if commentScore != "" {
			fmt.Fprintf(&comments, "👤 %s (%s): %s\n", author, commentScore, text)
		} else {
			fmt.Fprintf(&comments, "👤 %s: %s\n", author, text)
		}
	})

	return comments.String()
}

func (f RedditFetcher) buildFullText(title string, metadata postMetadata, postText, images, comments string) string {
	var parts []string

	if title := strings.TrimSpace(title); title != "" {
		parts = append(parts, "TITLE: "+title)
	}

	info := strings.Join(metadata.metadataParts, " ")
	if len(info) > 0 {
		if metadata.score != "" {
			info += ", " + metadata.score + " points"
		}
		parts = append(parts, "INFO: "+info)
	}

	if text := strings.TrimSpace(postText); text != "" {
		parts = append(parts, "TEXT:\n"+text)
	}

	if imgs := strings.TrimSpace(images); imgs != "" {
		parts = append(parts, "IMAGES:\n"+imgs)
	}

	if cmts := strings.TrimSpace(comments); cmts != "" {
		parts = append(parts, "COMMENTS:\n"+cmts)
	}

	return strings.Join(parts, "\n")
}

func (f RedditFetcher) parseRedditURL(redditURL string) (string, error) {
	req, err := NewRequestPayload(redditURL, nil, nil)
	if err != nil {
		return "", fmt.Errorf("incorrect request: %w", err)
	}
	resp, body, err := f.fetch(req)
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

func (f RedditFetcher) getOldRedditURL(URL string) string {
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
			}
			parsedNew.Host = "old.reddit.com"
			return parsedNew.String()
		}
	}
	return URL
}
