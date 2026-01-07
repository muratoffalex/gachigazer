package fetcher

import (
	"errors"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

const fragranticaRegexp = `^https?://(?:www\.)?fragrantica\.[a-z]{2,3}(?:\.[a-z]{2})?/(?:perfume|Parfum|parfum|parfem|perfumes|perfumy)/(?<designer>[^/]+)/(?<perfumeName>[^/]+-\d+)\.html$`

const (
	OptFragranticaMaxReviews = "max_reviews"
)

type FragranticaRequest struct {
	RequestPayload
	// maxReviews limits the number of reviews to be loaded.
	maxReviews int
}

func NewFragranticaRequest(
	url string,
	maxReviews int,
) (FragranticaRequest, error) {
	payload, err := NewRequestPayload(url, nil, nil)
	if err != nil {
		return FragranticaRequest{}, errors.Join(ErrCreateYoutubeRequest, err)
	}
	return FragranticaRequest{
		RequestPayload: payload,
		maxReviews:     maxReviews,
	}, nil
}

func (r FragranticaRequest) Options() map[string]any {
	return map[string]any{
		OptFragranticaMaxReviews: r.maxReviews,
	}
}

// FragranticaFetcher Extracts all possible data about a fragrance in a compact form.
// What data is available:
// - name
// - description
// - rating
// - accords
// - pyramid
// - reviews
// What data is unavailable and why:
// - season and time of day rating
// - voting results for GENDER, PRICE/QUALITY, LONGEVITY, SILLAGE
// All of them are loaded via JS and cannot be obtained without a headless browser.
type FragranticaFetcher struct {
	BaseFetcher
}

func NewFragranticaFetcher(l logger.Logger, httpClient HTTPClient) FragranticaFetcher {
	return FragranticaFetcher{
		BaseFetcher: NewBaseFetcher(
			FetcherNameFragrantica,
			fragranticaRegexp,
			httpClient,
			l,
		),
	}
}

type fragranticaRating struct {
	current    string
	best       string
	votesCount string
}

type fragranticaNote struct {
	name  string
	image string
	link  string
}

type fragranticaPyramid struct {
	topNotes    []fragranticaNote
	middleNotes []fragranticaNote
	baseNotes   []fragranticaNote
}

type fragranticaReview struct {
	id     string
	author string
	date   string
	text   string
}

type fragranticaSimilar struct {
	name string
	url  string
}

type fragranticaResponse struct {
	title       string
	description string
	accords     []string
	rating      fragranticaRating
	pyramid     fragranticaPyramid
	similar     []fragranticaSimilar
	reviews     []fragranticaReview
}

func (r fragranticaResponse) format() string {
	var sb strings.Builder

	// Title
	fmt.Fprintf(&sb, "Title: %s\n", r.title)

	// Description
	if r.description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", r.description)
	}

	// Rating
	fmt.Fprintf(&sb, "Rating: %s/%s (votes: %s)\n",
		r.rating.current, r.rating.best, r.rating.votesCount)

	// Accords
	if len(r.accords) > 0 {
		sb.WriteString("Accords: ")
		for i, accord := range r.accords {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(accord)
		}
		sb.WriteString("\n")
	}

	// Notes pyramid
	hasNotes := len(r.pyramid.topNotes) > 0 || len(r.pyramid.middleNotes) > 0 || len(r.pyramid.baseNotes) > 0
	if hasNotes {
		sb.WriteString("Notes: ")

		// Top notes
		if len(r.pyramid.topNotes) > 0 {
			sb.WriteString("Top: ")
			for i, note := range r.pyramid.topNotes {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(note.name)
			}
			sb.WriteString("; ")
		}

		// Middle notes
		if len(r.pyramid.middleNotes) > 0 {
			sb.WriteString("Middle: ")
			for i, note := range r.pyramid.middleNotes {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(note.name)
			}
			sb.WriteString("; ")
		}

		// Base notes
		if len(r.pyramid.baseNotes) > 0 {
			sb.WriteString("Base: ")
			for i, note := range r.pyramid.baseNotes {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(note.name)
			}
		}
		sb.WriteString("\n")
	}

	// Similar perfumes
	if len(r.similar) > 0 {
		sb.WriteString("Similar: ")
		for i, similar := range r.similar {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(similar.name)
		}
		sb.WriteString("\n")
	}

	// Reviews summary
	if len(r.reviews) > 0 {
		sb.WriteString("Reviews:\n")
		for _, review := range r.reviews {
			reviewText := strings.TrimSpace(review.text)

			fmt.Fprintf(&sb, "- %s (%s): %s\n",
				review.author,
				review.date,
				reviewText)
		}
	}

	return sb.String()
}

func (f FragranticaFetcher) Handle(request Request) (Response, error) {
	comRequest, err := f.getComRequest(request)
	if err != nil {
		return Response{}, err
	}

	maxReviews := 30
	if opts := request.Options(); opts != nil {
		if v, ok := opts[OptFragranticaMaxReviews].(int); ok {
			maxReviews = v
		}
	}

	doc, err := f.getHTML(comRequest)
	if err != nil {
		return f.errorResponse(err)
	}
	response := fragranticaResponse{}
	title, _ := doc.Find("meta[property='og:title']").First().Attr("content")
	response.title = title

	body := doc.Find("div#toptop").Next()

	// accords
	var accords []string
	body.Find("div.accord-box").Each(func(i int, s *goquery.Selection) {
		accordText := s.Find("div.accord-bar").Text()
		if accordText != "" {
			accords = append(accords, strings.TrimSpace(accordText))
		}
	})
	response.accords = accords

	// rating
	rating := fragranticaRating{}
	ratingContainer := body.Find("p.info-note").First()
	rating.current = ratingContainer.Find("span[itemprop='ratingValue']").First().Text()
	rating.best = ratingContainer.Find("span[itemprop='bestRating']").First().Text()
	rating.votesCount = ratingContainer.Find("span[itemprop='ratingCount']").First().Text()
	response.rating = rating

	// description
	response.description = strings.TrimSpace(body.Find("div[itemprop='description'] p").First().Text())

	// Pyramid
	pyramid := fragranticaPyramid{}
	pyramidContainer := doc.Find("div#pyramid").First()

	// Top Notes
	topNotesSection := pyramidContainer.Find("h4:contains('Top Notes')").Next()
	pyramid.topNotes = f.parsePyramidNotes(topNotesSection)

	// Middle Notes
	middleNotesSection := pyramidContainer.Find("h4:contains('Middle Notes')").Next()
	pyramid.middleNotes = f.parsePyramidNotes(middleNotesSection)

	// Base Notes
	baseNotesSection := pyramidContainer.Find("h4:contains('Base Notes')").Next()
	pyramid.baseNotes = f.parsePyramidNotes(baseNotesSection)

	response.pyramid = pyramid

	// Reviews
	doc.Find("div.fragrance-review-box").Each(func(i int, s *goquery.Selection) {
		if len(response.reviews) >= maxReviews {
			return
		}

		review := fragranticaReview{}
		review.id, _ = s.Attr("id")
		review.author = s.Find("b.idLinkify").First().Text()
		review.date = s.Find("span[itemprop='datePublished']").First().Text()
		review.text = s.Find("div[itemprop='reviewBody'] p").First().Text()

		response.reviews = append(response.reviews, review)
	})

	// Similar perfumes
	similar := []fragranticaSimilar{}
	similarContainer := doc.Find("div.carousel").First()
	similarContainer.Find("div.carousel-cell").Each(func(i int, s *goquery.Selection) {
		if len(similar) >= 5 {
			return
		}
		similarItem := fragranticaSimilar{}
		similarItem.name, _ = s.Find("img").First().Attr("alt")
		similarItem.url, _ = s.Find("a").First().Attr("href")
		similar = append(similar, similarItem)
	})
	response.similar = similar

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: response.format()}},
		IsError: false,
	}, nil
}

func (f FragranticaFetcher) parsePyramidNotes(s *goquery.Selection) []fragranticaNote {
	var notes []fragranticaNote
	s.Children().Find("div").Each(func(i int, noteDiv *goquery.Selection) {
		note := fragranticaNote{}
		note.name = strings.TrimSpace(noteDiv.Find("div").Text())
		note.link, _ = noteDiv.Find("a").First().Attr("href")
		note.image, _ = noteDiv.Find("img").First().Attr("src")
		if note.name != "" {
			notes = append(notes, note)
		}
	})
	return notes
}

func (f FragranticaFetcher) getComRequest(request Request) (Request, error) {
	re := f.pattern
	if !re.MatchString(request.URL()) {
		return nil, fmt.Errorf("URL does not match pattern: %s", request.URL())
	}

	matches := re.FindStringSubmatch(request.URL())
	if len(matches) == 0 {
		return nil, fmt.Errorf("no matches found in URL: %s", request.URL())
	}

	designerIndex := re.SubexpIndex("designer")
	perfumeNameIndex := re.SubexpIndex("perfumeName")

	if designerIndex < 0 || perfumeNameIndex < 0 {
		return nil, fmt.Errorf("required named groups not found in pattern")
	}

	designer := matches[designerIndex]
	perfumeName := matches[perfumeNameIndex]

	if designer == "" || perfumeName == "" {
		return nil, fmt.Errorf("empty designer or perfume name extracted")
	}

	newURL := fmt.Sprintf(
		"https://www.fragrantica.com/perfume/%s/%s.html",
		designer,
		perfumeName,
	)

	newRequest, err := NewRequestPayload(newURL, request.Headers(), request.Options())
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	return newRequest, nil
}
