package fetch

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type AvitoRatingResponse struct {
	Entries []struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	} `json:"entries"`
}

type ScoreEntry struct {
	ReviewCount int     `json:"reviewCount"`
	Score       int     `json:"score"`
	ScoreFloat  float64 `json:"scoreFloat"`
	Title       string  `json:"title"`
}

type RatingEntry struct {
	DeliveryTitle string        `json:"deliveryTitle"`
	ItemTitle     string        `json:"itemTitle"`
	Score         int           `json:"score"`
	Rated         string        `json:"rated"`
	StageTitle    string        `json:"stageTitle"`
	Title         string        `json:"title"`
	TextSections  []TextSection `json:"textSections"`
}

type TextSection struct {
	Text string `json:"text"`
}

func (f *Fetcher) parseAvito(doc *goquery.Document) Response {
	title := doc.Find("[data-marker='item-view/title-info']").First()
	price := doc.Find("[data-marker='item-view/item-price']").First()
	priceCurrency := doc.Find("[itemprop='priceCurrency']").First()
	params := doc.Find("[data-marker='item-view/item-params'] ul").First()
	description := doc.Find("[data-marker='item-view/item-description']").First()
	date := doc.Find("[data-marker='item-view/item-date']").First()
	views := doc.Find("[data-marker='item-view/total-views']").First()
	sellerName := doc.Find("[data-marker='seller-info/name']").First()
	sellerType := doc.Find("[data-marker='seller-info/label']").First()
	address := doc.Find("[itemprop='address']").First()
	mainImage := doc.Find("[data-marker='image-frame/image-wrapper']").First()

	result := []string{}
	if title != nil && title.Text() != "" {
		result = append(result, "Title: "+title.Text())
	}
	if price != nil && price.Text() != "" {
		currency := " ₽"
		if priceCurrency != nil {
			currency = priceCurrency.Text()
		}
		result = append(result, "Price: "+price.Text()+currency)
	}
	if description != nil && description.Text() != "" {
		result = append(result, "Description: "+description.Text())
	}
	if params != nil && params.Text() != "" {
		result = append(result, "Parameters: "+params.Text())
	}
	if date != nil && date.Text() != "" {
		result = append(result, "Date: "+date.Text())
	}
	if views != nil && views.Text() != "" {
		result = append(result, "Views count: "+views.Text())
	}
	if address != nil && address.Text() != "" {
		result = append(result, "Address: "+address.Text())
	}
	sellerInfo := []string{}
	if sellerName != nil && sellerName.Text() != "" {
		sellerInfo = append(sellerInfo, "Seller name: "+sellerName.Text())
	}
	if sellerType != nil && sellerType.Text() != "" {
		sellerInfo = append(sellerInfo, "Seller type: "+sellerType.Text())
	}

	sellerID := f.getSellerID(doc)

	if sellerID != "" {
		sellerInfo = append(sellerInfo, f.parseAvitoSellerInfo(sellerID))
	}

	text := strings.Join(result, "\n")
	if text != "" {
		if len(sellerInfo) > 0 {
			sellerInfo = slices.Concat([]string{"Seller information:"}, sellerInfo)
			text += "\n" + strings.Join(sellerInfo, "\n")
		} else {
			text += "\n" + "No seller information."
		}
	}

	if text != "" {
		response := Response{
			Content: []Content{},
			IsError: false,
		}
		response.Content = append(response.Content, Content{
			Type: ContentTypeText,
			Text: text,
		})
		if mainImage != nil {
			if url, exists := mainImage.Attr("data-url"); exists {
				response.Content = append(response.Content, Content{
					Type: ContentTypeImage,
					Text: url,
				})
			}
		}

		return response
	} else {
		return f.parseDefault(doc)
	}
}

func (f *Fetcher) getSellerID(doc *goquery.Document) string {
	var sellerID string
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		if strings.Contains(text, "__initialData__") {
			re := regexp.MustCompile(`window\.__initialData__\s*=\s*"([^"]+)"`)
			matches := re.FindStringSubmatch(text)
			if len(matches) < 2 {
				return
			}

			decoded, err := url.QueryUnescape(matches[1])
			if err != nil {
				log.Printf("Failed to decode initialData: %v", err)
				return
			}

			var data map[string]any
			if err := json.Unmarshal([]byte(decoded), &data); err != nil {
				log.Printf("Failed to parse initialData JSON: %v", err)
				return
			}

			for key := range data {
				if strings.HasPrefix(key, "@avito/bx-item-view:") {
					if bxItem, ok := data[key].(map[string]any); ok {
						if buyerItem, ok := bxItem["buyerItem"].(map[string]any); ok {
							if rating, ok := buyerItem["rating"].(map[string]any); ok {
								if userKey, ok := rating["userKey"].(string); ok {
									sellerID = userKey
									break
								}
							}
						}
					}
				}
			}
		}
	})
	return sellerID
}

func (f *Fetcher) parseAvitoSellerInfo(ID string) string {
	resp, body, err := f.fetch(RequestPayload{
		URL: fmt.Sprintf("https://www.avito.ru/web/6/user/%s/ratings?summary_redesign=1", ID),
	})
	if err != nil {
		return fmt.Sprintf("Get avito seller info error: %v", err.Error())
	}
	defer resp.Body.Close()

	if len(body) > 0 {
		var response AvitoRatingResponse
		if err := json.Unmarshal([]byte(body), &response); err != nil {
			return "Unmarshal error: " + err.Error()
		}

		var result strings.Builder

		titleExist := false
		for _, entry := range response.Entries {
			switch entry.Type {
			case "score":
				var score ScoreEntry
				if err := json.Unmarshal(entry.Value, &score); err == nil {
					result.WriteString(fmt.Sprintf(
						"Overall rating: %s\nRatings count: %d\nAverage score: %.1f\n\n",
						score.Title,
						score.ReviewCount,
						score.ScoreFloat,
					))
				}

			case "rating":
				if !titleExist {
					result.WriteString("Seller reviews:\n")
					titleExist = true
				}
				var rating RatingEntry
				if err := json.Unmarshal(entry.Value, &rating); err == nil {
					texts := make([]string, 0, len(rating.TextSections))
					for _, section := range rating.TextSections {
						texts = append(texts, section.Text)
					}

					if rating.DeliveryTitle == "" {
						rating.DeliveryTitle = "In-person meeting"
					}

					result.WriteString(fmt.Sprintf(
						"▻ %s\nItem: %s\nRating: %d/5\nDate: %s\nStage: %s\nDeal type: %s\nReview:\n%s\n\n",
						rating.Title,
						rating.ItemTitle,
						rating.Score,
						rating.Rated,
						rating.StageTitle,
						rating.DeliveryTitle,
						strings.Join(texts, "\n"),
					))
				}
			}
		}

		return result.String()
	}
	return "No seller info found"
}
