package fetcher

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

type OpennetFetcher struct {
	BaseFetcher
}

func NewOpennetFetcher(l logger.Logger, httpClient HTTPClient) OpennetFetcher {
	return OpennetFetcher{
		BaseFetcher: NewBaseFetcher(FetcherNameOpennet, "opennet\\.ru", httpClient, l),
	}
}

func (f OpennetFetcher) Handle(request Request) (Response, error) {
	doc, err := f.getHTML(request)
	if err != nil {
		return f.errorResponse(err)
	}
	clean := func(text string) string {
		text = strings.ReplaceAll(text, "\n", " ")
		// remove square brackets and their contents
		re := regexp.MustCompile(`\[.*?\]`)
		text = re.ReplaceAllString(text, "")
		// remove extra spaces
		text = strings.Join(strings.Fields(text), " ")
		return strings.TrimSpace(text)
	}
	// Extract header
	title := doc.Find("table.thdr2").First().Text()
	title = clean(title)

	// Main content
	content := doc.Find("td.chtext").First().Text()
	content = clean(content)

	if title == "" && content == "" {
		return f.errorResponse(errors.New("no opennet content found"))
	}

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
		commentText = clean(commentText)

		// Format the output
		if commentInfo.Len() > 0 || commentText != "" {
			fmt.Fprintf(&comments, "——\n%s\n%s\n",
				commentInfo.String(),
				commentText,
			)
		}
	})

	// Collect the result
	fullText := strings.TrimSpace(
		"TITLE: " + title +
			"\n\nCONTENT:\n" + content +
			"\n\nCOMMENTS:\n" + comments.String(),
	)

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: fullText}},
		IsError: false,
	}, nil
}
