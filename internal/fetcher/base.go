package fetcher

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"golang.org/x/net/html/charset"
)

// --- BaseFetcher ---

type BaseFetcher struct {
	name    string
	client  HTTPClient
	pattern *regexp.Regexp
	logger  logger.Logger
}

func NewBaseFetcher(name string, pattern string, httpClient HTTPClient, l logger.Logger) BaseFetcher {
	l = l.WithField("fetcher", name)
	// compile all patterns at startup to immediately reject invalid ones + improve performance
	compiledPattern, err := regexp.Compile(pattern)
	if err != nil {
		l.WithFields(logger.Fields{
			"pattern": pattern,
		}).Error("Pattern is invalid")
	}
	return BaseFetcher{
		name,
		httpClient,
		compiledPattern,
		l,
	}
}

func (f BaseFetcher) CanHandle(url string) bool {
	return f.pattern.MatchString(url)
}

func (f BaseFetcher) GetName() string {
	return f.name
}

func (f BaseFetcher) cleanDoc(doc *goquery.Document) {
	doc.Find("script, style, footer, nav, aside, .cookie-consent, .promoted-link, .sidebar, .login-form, .signup-form, .hidden").Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})
}

func (f BaseFetcher) cleanText(text string) string {
	normalizedText := regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	normalizedText = strings.TrimSpace(normalizedText)
	return normalizedText
}

func (f BaseFetcher) fetch(payload Request) (*http.Response, string, error) {
	if _, err := url.ParseRequestURI(payload.URL()); err != nil {
		return nil, "", fmt.Errorf("invalid URL: %w", err)
	}
	req, err := http.NewRequest(payload.Method(), payload.URL(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", RandomUserAgent())
	for k, v := range payload.Headers() {
		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, "", fmt.Errorf("connection reset by peer (EOF) - possible server issue with %s", payload.URL())
		}
		return nil, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusTooManyRequests: // 429
		return nil, "", fmt.Errorf("rate limit exceeded (429) for %s", payload.URL())
	case http.StatusForbidden: // 403
		return nil, "", fmt.Errorf("access forbidden (403) for %s", payload.URL())
	case http.StatusInternalServerError: // 500
		return nil, "", fmt.Errorf("server error (500) for %s", payload.URL())
	case http.StatusBadGateway: // 502
		return nil, "", fmt.Errorf("bad gateway (502) for %s", payload.URL())
	case http.StatusServiceUnavailable: // 503
		return nil, "", fmt.Errorf("service unavailable (503) for %s", payload.URL())
	case http.StatusGatewayTimeout: // 504
		return nil, "", fmt.Errorf("gateway timeout (504) for %s", payload.URL())
	}

	utf8Reader, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return resp, "", nil
		}
		return nil, "", fmt.Errorf("charset detection failed: %w", err)
	}

	bodyBytes, err := io.ReadAll(utf8Reader)
	if err != nil {
		return nil, "", fmt.Errorf("reading body failed: %w", err)
	}

	return resp, string(bodyBytes), nil
}

func (f BaseFetcher) getHTML(request Request) (*goquery.Document, error) {
	_, body, err := f.fetch(request)
	if err != nil {
		return nil, fmt.Errorf("failed to load page: %v", err)
	}
	doc, err := f.getGoqueryDoc(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %v", err)
	}

	return doc, nil
}

func (f BaseFetcher) getGoqueryDoc(body string) (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return doc, nil
}

func (f BaseFetcher) errorResponse(err error) (Response, error) {
	return Response{
		Content: []Content{{Type: ContentTypeText, Text: err.Error()}},
		IsError: true,
	}, err
}

func (f BaseFetcher) isHTMLContent(resp *http.Response, body string) bool {
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml+xml") {
		return true
	}

	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "<!DOCTYPE") ||
		strings.HasPrefix(trimmed, "<html") ||
		strings.Contains(trimmed, "<head>") ||
		strings.Contains(trimmed, "<body>") {
		return true
	}

	return false
}

// --- FuncFetcher ---

type FuncFetcher struct {
	BaseFetcher
	handler fetchHandler
}

func NewFuncFetcher(
	name string,
	pattern string,
	httpClient HTTPClient,
	l logger.Logger,
	handler fetchHandler,
) FuncFetcher {
	return FuncFetcher{
		BaseFetcher: NewBaseFetcher(name, pattern, httpClient, l),
		handler:     handler,
	}
}

func (f FuncFetcher) Handle(request Request) (Response, error) {
	return f.handler(f.BaseFetcher, request)
}
