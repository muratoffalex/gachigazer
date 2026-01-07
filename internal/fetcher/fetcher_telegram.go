package fetcher

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

type TelegramFetcher struct {
	BaseFetcher
}

func NewTelegramFetcher(l logger.Logger, client HTTPClient) TelegramFetcher {
	return TelegramFetcher{
		BaseFetcher: NewBaseFetcher(
			FetcherNameTelegram,
			"t\\.me",
			client,
			l,
		),
	}
}

func (f TelegramFetcher) Handle(request Request) (Response, error) {
	doc, err := f.getHTML(request)
	if err != nil {
		return f.errorResponse(err)
	}
	// extract channel/post_id from URL
	u, err := url.Parse(request.URL())
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
		}, nil
	}

	text := postDiv.Find(".tgme_widget_message_text").Text()
	text = strings.TrimSpace(text)

	// If the text is empty, check the media caption
	if text == "" {
		text = postDiv.Find(".tgme_widget_message_caption").Text()
		text = strings.TrimSpace(text)
	}

	if text == "" {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "Empty message or failed to extract text"}},
			IsError: true,
		}, nil
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

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: text}},
		IsError: false,
	}, nil
}
