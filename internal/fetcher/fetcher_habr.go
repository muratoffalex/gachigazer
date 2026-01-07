package fetcher

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

type HabrFetcher struct {
	BaseFetcher
}

func NewHabrFetcher(l logger.Logger, client HTTPClient) HabrFetcher {
	return HabrFetcher{
		BaseFetcher: NewBaseFetcher(FetcherNameHabr, "habr\\.com", client, l),
	}
}

func (f HabrFetcher) Handle(request Request) (Response, error) {
	doc, err := f.getHTML(request)
	if err != nil {
		return f.errorResponse(err)
	}
	title := doc.Find("h1.tm-title").First().Text()
	body := doc.Find("div.article-body").First()
	text := f.cleanText(body.Text())

	if title == "" && text == "" {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "No Habr content found"}},
			IsError: true,
		}, nil
	}

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
	comments := f.parseHabrComments(request.URL())

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
	}, nil
}

func (f HabrFetcher) parseHabrComments(articleURL string) string {
	// https://habr.com/ru/articles/717912/
	re := regexp.MustCompile(`\d{6}`)
	articleID := re.FindString(articleURL)
	if articleID == "" {
		return "No valid article ID found in URL"
	}

	request, err := NewRequestPayload("https://habr.com/kek/v2/articles/"+articleID+"/comments/", nil, nil)
	if err != nil {
		return fmt.Sprintf("Incorrect request: %v", request)
	}
	resp, body, err := f.fetch(request)
	if err != nil {
		return fmt.Sprintf("Get comments error: %v", err.Error())
	}
	defer resp.Body.Close()

	if len(body) > 0 {
		var data struct {
			Comments map[string]struct {
				ID       string `json:"id"`
				ParentID string `json:"parentId,omitempty"`
				Level    int    `json:"level"`
				Author   struct {
					Alias string `json:"alias"`
				} `json:"author"`
				Message string `json:"message"`
				Score   int    `json:"score"`
			} `json:"comments"`
		}
		err := json.Unmarshal([]byte(body), &data)
		if err != nil {
			return "Unmarshal error " + err.Error()
		}

		var result strings.Builder
		i := 0

		commentIDs := make([]string, 0, len(data.Comments))
		for id := range data.Comments {
			commentIDs = append(commentIDs, id)
		}

		sort.Strings(commentIDs)

		for _, commentID := range commentIDs {
			if i >= 50 {
				break
			}

			comment := data.Comments[commentID]
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
			fmt.Fprintf(
				&result,
				"- id: %s;%s score: %d; author: %s; text: %s\n",
				comment.ID,
				parentIDStr,
				comment.Score,
				comment.Author.Alias,
				message,
			)
			i++
			if i > 50 {
				break
			}
		}
		return result.String()
	}

	return "No comments found"
}
