package fetcher

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
)

const (
	FetcherNameDefault     = "default"
	FetcherNameYoutube     = "youtube"
	FetcherNameGithub      = "github"
	FetcherNameHabr        = "habr"
	FetcherNameOpennet     = "opennet"
	FetcherNameFragrantica = "fragrantica"
	FetcherNameTelegram    = "telegram"
	FetcherNameAvito       = "avito"
	FetcherNameReddit      = "reddit"
)

const (
	ContentTypeText  ContentType = "text"
	ContentTypeURL   ContentType = "url"
	ContentTypeImage ContentType = "image"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var UserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:138.0) Gecko/20100101 Firefox/138.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 YaBrowser/25.4.1.1015 Yowser/2.5 Safari/537.36",
}

func RandomUserAgent() string {
	return UserAgents[rand.Intn(len(UserAgents))]
}

type Fetcher interface {
	Handle(request Request) (Response, error)
	CanHandle(url string) bool
	GetName() string
}

type ContentType string

type fetchHandler func(f BaseFetcher, request Request) (Response, error)

type Request interface {
	URL() string
	Method() string
	Headers() map[string]string
	// Options returns a map of options for a specific fetcher.
	// Keys must be consistent between the fetcher and the request.
	Options() map[string]any
}

type RequestPayload struct {
	url     string
	method  string
	headers map[string]string
	options map[string]any
}

func NewRequestPayload(
	urlString string,
	headers map[string]string,
	options map[string]any,
) (RequestPayload, error) {
	return NewRequestPayloadWithMethod(urlString, http.MethodGet, headers, options)
}

func NewRequestPayloadWithMethod(
	urlString string,
	method string,
	headers map[string]string,
	options map[string]any,
) (RequestPayload, error) {
	if strings.TrimSpace(urlString) == "" {
		return RequestPayload{}, ErrCannotBeEmpty
	}
	parsedURL, err := url.ParseRequestURI(urlString)
	if err != nil {
		return RequestPayload{}, fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme == "" {
		return RequestPayload{}, fmt.Errorf("URL must have a scheme (http:// or https://)")
	}

	if parsedURL.Host == "" {
		return RequestPayload{}, fmt.Errorf("URL must have a host")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return RequestPayload{}, fmt.Errorf("unsupported URL scheme: %s", parsedURL.Scheme)
	}
	return RequestPayload{
		url:     urlString,
		method:  method,
		headers: headers,
		options: options,
	}, nil
}

func MustNewRequestPayload(
	urlString string,
	headers map[string]string,
	options map[string]any,
) RequestPayload {
	return RequestPayload{
		url:     urlString,
		headers: headers,
		options: options,
	}
}

func (r RequestPayload) URL() string {
	return r.url
}

func (r RequestPayload) Method() string {
	return r.method
}

func (r RequestPayload) Headers() map[string]string {
	return r.headers
}

func (r RequestPayload) Options() map[string]any {
	return r.options
}

type Content struct {
	Type ContentType
	Text string
}
