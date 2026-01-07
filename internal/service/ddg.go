package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/muratoffalex/gachigazer/internal/fetcher"
)

var (
	ErrVQDNotFound    = errors.New("could not extract vqd")
	allowedTimelimits = []string{"d", "w", "m", "y"}
)

type TextResult struct {
	Title string
	Href  string
	Body  string
}

type ImageResult struct {
	Title     string
	Image     string
	Thumbnail string
	URL       string
	Source    string
}

type DuckDuckGoSearch struct {
	client    *http.Client
	rateLimit time.Duration
}

func NewDuckDuckGoSearch(client *http.Client, rateLimit time.Duration) *DuckDuckGoSearch {
	if rateLimit == 0 {
		rateLimit = 1 * time.Second
	}
	return &DuckDuckGoSearch{
		client:    client,
		rateLimit: rateLimit,
	}
}

func (d *DuckDuckGoSearch) getVQD(keywords string) (string, error) {
	time.Sleep(d.rateLimit)

	req, err := http.NewRequest("GET", "https://duckduckgo.com", nil)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	q.Add("q", keywords)
	req.URL.RawQuery = q.Encode()

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("VDQ URL: %s", req.URL.String())
		return "", fmt.Errorf("failed to get VQD: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return d.extractVQD(body, keywords)
}

func (d *DuckDuckGoSearch) getURL(method string, urlStr string, params url.Values) ([]byte, error) {
	time.Sleep(d.rateLimit)

	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = params.Encode()
	req.Header.Set("Referer", "https://duckduckgo.com/")
	req.Header.Set("User-Agent", fetcher.RandomUserAgent())

	resp, err := d.client.Do(req)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "timeout") {
			return nil, fmt.Errorf("%s timeout: %v", urlStr, err)
		}
		return nil, fmt.Errorf("%s request failed: %v", urlStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("%s ratelimit: status %d", urlStr, resp.StatusCode)
		}
		return nil, fmt.Errorf("%s failed: status %d", urlStr, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (d DuckDuckGoSearch) Images(
	keywords string,
	region string,
	safesearch string,
	timeLimit string,
	size *string,
	color *string,
	typeImage *string,
	layout *string,
	licenseImage *string,
	maxResults *int,
) ([]ImageResult, error) {
	if keywords == "" {
		return nil, errors.New("keywords is mandatory")
	}

	vqd, err := d.getVQD(keywords)
	if err != nil {
		return nil, err
	}

	safesearchBase := map[string]string{"on": "1", "moderate": "1", "off": "-1"}
	filters := []string{}
	if timeLimit != "" && slices.Contains(allowedTimelimits, timeLimit) {
		filters = append(filters, fmt.Sprintf("time:%s", timeLimit))
	}
	if size != nil {
		filters = append(filters, fmt.Sprintf("size:%s", *size))
	}
	if color != nil {
		filters = append(filters, fmt.Sprintf("color:%s", *color))
	}
	if typeImage != nil {
		filters = append(filters, fmt.Sprintf("type:%s", *typeImage))
	}
	if layout != nil {
		filters = append(filters, fmt.Sprintf("layout:%s", *layout))
	}
	if licenseImage != nil {
		filters = append(filters, fmt.Sprintf("license:%s", *licenseImage))
	}

	payload := url.Values{
		"l":   []string{region},
		"o":   []string{"json"},
		"q":   []string{keywords},
		"vqd": []string{vqd},
		"f":   []string{strings.Join(filters, ",")},
		"p":   []string{safesearchBase[strings.ToLower(safesearch)]},
	}

	cache := make(map[string]bool)
	var results []ImageResult

	for range 5 {
		resp, err := d.getURL("GET", "https://duckduckgo.com/i.js", payload)
		if err != nil {
			return nil, err
		}

		var respData struct {
			Results []struct {
				Title     string `json:"title"`
				Image     string `json:"image"`
				Thumbnail string `json:"thumbnail"`
				URL       string `json:"url"`
				Height    int    `json:"height"`
				Width     int    `json:"width"`
				Source    string `json:"source"`
			} `json:"results"`
			Next string `json:"next"`
		}

		if err := json.Unmarshal(resp, &respData); err != nil {
			return nil, err
		}

		for _, row := range respData.Results {
			if row.Image != "" && !cache[row.Image] {
				cache[row.Image] = true
				result := ImageResult{
					Title:     row.Title,
					Image:     normalizeURL(row.Image),
					Thumbnail: normalizeURL(row.Thumbnail),
					URL:       normalizeURL(row.URL),
					Source:    row.Source,
				}
				results = append(results, result)
				if maxResults != nil && len(results) >= *maxResults {
					return results, nil
				}
			}
		}

		if respData.Next == "" || maxResults == nil {
			return results, nil
		}
		nextParams := strings.Split(respData.Next, "s=")
		if len(nextParams) < 2 {
			return results, nil
		}
		payload.Set("s", strings.Split(nextParams[1], "&")[0])
	}

	return results, nil
}

func (d DuckDuckGoSearch) extractVQD(htmlBytes []byte, keywords string) (string, error) {
	patterns := []struct {
		prefix []byte
		suffix []byte
		offset int
	}{
		{[]byte(`vqd="`), []byte(`"`), 5},
		{[]byte(`vqd=`), []byte(`&`), 4},
		{[]byte(`vqd='`), []byte(`'`), 5},
	}

	for _, p := range patterns {
		start := bytes.Index(htmlBytes, p.prefix)
		if start == -1 {
			continue
		}
		start += p.offset
		end := bytes.Index(htmlBytes[start:], p.suffix)
		if end == -1 {
			continue
		}
		return string(htmlBytes[start : start+end]), nil
	}

	return "", fmt.Errorf("%w: keywords=%s", ErrVQDNotFound, keywords)
}

func normalizeURL(urlStr string) string {
	if urlStr == "" {
		return ""
	}
	unescaped, err := url.QueryUnescape(urlStr)
	if err != nil {
		return urlStr
	}
	return strings.ReplaceAll(unescaped, " ", "+")
}

func (d *DuckDuckGoSearch) Text(
	keywords string,
	region string,
	timeLimit string,
	maxResults int,
) ([]TextResult, error) {
	if keywords == "" {
		return nil, errors.New("keywords is mandatory")
	}

	if maxResults == 0 {
		maxResults = 3
	}

	payload := url.Values{
		"q":  []string{keywords},
		"b":  []string{""},
		"kl": []string{region},
	}
	if timeLimit == "" || slices.Contains(allowedTimelimits, timeLimit) {
		payload.Set("df", timeLimit)
	}

	cache := make(map[string]bool)
	var results []TextResult

	for range 5 {
		resp, err := d.getURL("POST", "https://html.duckduckgo.com/html", payload)
		if err != nil {
			return nil, err
		}

		if bytes.Contains(resp, []byte("No results.")) {
			return results, nil
		}

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp))
		if err != nil {
			return nil, err
		}

		doc.Find("div.result").Each(func(i int, s *goquery.Selection) {
			if len(results) >= maxResults {
				return
			}
			link := s.Find("a.result__a")
			href, exists := link.Attr("href")
			if !exists {
				return
			}

			if href != "" && !cache[href] &&
				!strings.HasPrefix(href, "http://www.google.com/search?q=") &&
				!strings.HasPrefix(href, "https://duckduckgo.com/y.js?ad_domain") {

				cache[href] = true
				title := link.Text()
				body := s.Find("a.result__snippet").Text()

				results = append(results, TextResult{
					Title: normalize(title),
					Href:  normalizeURL(href),
					Body:  normalize(body),
				})

				if len(results) >= maxResults {
					return
				}
			}
		})

		if len(results) >= maxResults {
			break
		}

		nextPage := doc.Find("div.nav-link").Last()
		if nextPage.Length() == 0 {
			break
		}

		nextPage.Find("input[type=hidden]").Each(func(i int, s *goquery.Selection) {
			name, _ := s.Attr("name")
			value, _ := s.Attr("value")
			if name != "" {
				payload.Set(name, value)
			}
		})
	}

	return results, nil
}

func normalize(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
