package youtube

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/lrstanley/go-ytdlp"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

var (
	ErrNoVideoLanguage = errors.New("video language not available")
	ErrGetSubtitleURL  = errors.New("failed to get subtitle URL")
	ErrFetchTranscript = errors.New("failed to fetch transcript")
)

type SubtitleFetcherer interface {
	Fetch(info *ytdlp.ExtractedInfo) (string, error)
}

type SubtitleFetcher struct {
	httpClient HTTPClient
	logger     logger.Logger
}

func NewSubtitleFetcher(httpClient HTTPClient, logger logger.Logger) *SubtitleFetcher {
	return &SubtitleFetcher{
		httpClient: httpClient,
		logger:     logger,
	}
}

func (sf *SubtitleFetcher) Fetch(info *ytdlp.ExtractedInfo) (string, error) {
	if info.Language == nil {
		return "", ErrNoVideoLanguage
	}
	lang := *info.Language

	subtitleURL, err := sf.getSubtitleURL(info, lang)
	if err != nil {
		return "", errors.Join(ErrGetSubtitleURL, err)
	}

	content, err := sf.fetchSubtitles(subtitleURL)
	if err != nil {
		return "", errors.Join(ErrFetchTranscript, err)
	}

	return content, nil
}

func (sf *SubtitleFetcher) fetchSubtitles(url string) (string, error) {
	resp, err := sf.httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Convert SRT to plain text
	lines := strings.Split(string(body), "\n")
	var textLines []string
	for _, line := range lines {
		// Skip line numbers and timestamps
		if _, err := strconv.Atoi(line); err == nil {
			continue
		}
		if strings.Contains(line, "-->") {
			continue
		}
		if strings.TrimSpace(line) != "" {
			textLines = append(textLines, strings.TrimSpace(line))
		}
	}

	return strings.Join(textLines, " "), nil
}

func (sf *SubtitleFetcher) getSubtitleURL(info *ytdlp.ExtractedInfo, language string) (string, error) {
	if info.Language == nil || info.AutomaticCaptions == nil {
		return "", errors.New("no captions available for this video")
	}

	baseLanguage := strings.Split(language, "-")[0]

	var languageCaptions []*ytdlp.ExtractedSubtitle
	var exists bool

	languageCaptions, exists = info.AutomaticCaptions[language]
	if !exists {
		// Try to find the base language (e.g., en-US -> en)
		languageCaptions, exists = info.AutomaticCaptions[baseLanguage]
	}

	if !exists {
		return "", fmt.Errorf("no captions available for language: %s", language)
	}

	sf.logger.WithFields(logger.Fields{
		"language":           language,
		"available_captions": len(languageCaptions),
	}).Debug("Available captions")

	for _, caption := range languageCaptions {
		if strings.Contains(strings.ToLower(caption.URL), "fmt=srt") && caption.URL != "" {
			return caption.URL, nil
		}
	}

	return "", errors.New("no subtitle URL found")
}
