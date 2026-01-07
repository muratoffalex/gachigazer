package fetcher

import (
	"errors"
	"fmt"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/logger"
)

type Manager struct {
	fetchers       []Fetcher
	fetcherMap     map[string]Fetcher
	defaultFetcher Fetcher
	logger         logger.Logger
}

func NewManager(logger logger.Logger) *Manager {
	return &Manager{
		fetchers:       make([]Fetcher, 0),
		fetcherMap:     make(map[string]Fetcher),
		defaultFetcher: nil,
		logger:         logger,
	}
}

func (f *Manager) Fetch(request Request) (Response, error) {
	URL := request.URL()
	for _, item := range f.fetchers {
		log := f.logger.WithField("fetcher", item.GetName())
		if !item.CanHandle(URL) {
			continue
		}

		log.Debug("Matched fetcher")
		resp, err := item.Handle(request)
		if err == nil {
			return resp, nil
		}

		if errors.Is(err, ErrNotHandle) {
			log.WithError(err).Warn("fetcher not handling url, skip")
			continue
		}

		return resp, err
	}

	if f.defaultFetcher != nil {
		f.logger.Info("Using default fetcher")
		return f.defaultFetcher.Handle(request)
	}

	return Response{}, fmt.Errorf("no fetcher found for URL: %s", URL)
}

func (f *Manager) SetDefaultFetcher(fetcher Fetcher) {
	f.defaultFetcher = fetcher
}

func (f *Manager) RegisterFetcher(fetcher Fetcher) {
	if f.ContainsFetcher(fetcher) {
		return
	}
	f.fetchers = append(f.fetchers, fetcher)
	f.fetcherMap[fetcher.GetName()] = fetcher
}

func (f *Manager) ContainsFetcher(fetcher Fetcher) bool {
	_, exists := f.fetcherMap[fetcher.GetName()]
	return exists
}

// --- Response ---

type Response struct {
	Content []Content
	IsError bool
}

func (r Response) GetURLs() []string {
	return r.getByType(ContentTypeURL)
}

func (r Response) GetImages() []string {
	return r.getByType(ContentTypeImage)
}

func (r Response) getByType(contentType ContentType) []string {
	result := []string{}
	for _, item := range r.Content {
		if item.Type == contentType {
			result = append(result, item.Text)
		}
	}
	return result
}

func (r Response) GetText() string {
	var textBuilder strings.Builder
	for _, item := range r.Content {
		if item.Type == ContentTypeText {
			textBuilder.WriteString(item.Text)
			textBuilder.WriteString("\n")
		}
	}
	return strings.TrimSpace(textBuilder.String())
}
