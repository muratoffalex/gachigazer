package youtube

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lrstanley/go-ytdlp"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

type FetchFlag uint16

const (
	FetchTranscript FetchFlag = 1 << iota
	FetchComments
)

var (
	ErrExtractYoutubeData = errors.New("failed to extract youtube data")
	ErrExtractVideoInfo   = errors.New("failed to extract video info")
	ErrNoVideoInfo        = errors.New("no video info available")
)

type ContentExtractor interface {
	Extract(ctx context.Context, url string, options FetchOptions) (*ytdlp.Result, error)
}

type Config struct {
	Proxy       string
	MaxComments int
}

type Service struct {
	config           Config
	httpClient       HTTPClient
	logger           logger.Logger
	contentExtractor ContentExtractor
	subtitleFetcher  *SubtitleFetcher
	commentProcessor *CommentProcessor
}

func NewService(l logger.Logger, httpClient HTTPClient, config Config) Service {
	return Service{
		config:           config,
		logger:           l,
		httpClient:       httpClient,
		contentExtractor: &YtdlpContentExtractor{},
		subtitleFetcher:  NewSubtitleFetcher(httpClient, l),
		commentProcessor: &CommentProcessor{
			maxComments: config.MaxComments,
		},
	}
}

type YoutubeData struct {
	LikeCount    *float64
	CommentCount *float64
	ViewCount    *float64
	UploadedAt   *time.Time
	Title        string
	Transcript   string
	Comments     string
}

func (f *Service) FetchYoutubeData(url string, flags FetchFlag, maxComments int) (*YoutubeData, error) {
	output, err := f.contentExtractor.Extract(context.Background(), url, FetchOptions{
		SkipDownload:  true,
		PrintJSON:     true,
		WriteComments: flags&FetchComments != 0,
		Proxy:         f.config.Proxy,
	})
	if err != nil {
		return nil, errors.Join(ErrExtractYoutubeData, err)
	}

	info, err := output.GetExtractedInfo()
	if err != nil {
		return nil, errors.Join(ErrExtractVideoInfo, err)
	}

	if len(info) == 0 || info[0] == nil {
		return nil, ErrNoVideoInfo
	}

	file := info[0]
	result := f.extractVideoInfo(file)

	if flags&FetchTranscript != 0 {
		content, err := f.subtitleFetcher.Fetch(file)
		if err != nil {
			return nil, err
		}
		result.Transcript = content
	}

	if flags&FetchComments != 0 {
		result.Comments = f.commentProcessor.processComments(file.Comments, maxComments)
	}

	return result, nil
}

func (f *Service) extractVideoInfo(info *ytdlp.ExtractedInfo) *YoutubeData {
	result := &YoutubeData{
		LikeCount:    info.LikeCount,
		CommentCount: info.CommentCount,
		ViewCount:    info.ViewCount,
	}

	if info.Title != nil {
		result.Title = *info.Title
	}
	if timestamp := info.Timestamp; timestamp != nil {
		val := time.Unix(int64(*timestamp), 0)
		result.UploadedAt = &val
	}

	return result
}

func FormatCount(count float64) string {
	switch {
	case count < 1000:
		return fmt.Sprintf("%.0f", count)
	case count < 10000:
		return fmt.Sprintf("%.1fK", count/1000)
	case count < 1000000:
		return fmt.Sprintf("%.0fK", count/1000)
	case count < 10000000:
		return fmt.Sprintf("%.1fM", count/1000000)
	default:
		return fmt.Sprintf("%.0fM", count/1000000)
	}
}
