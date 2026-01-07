package youtube

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/lrstanley/go-ytdlp"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_FetchYoutubeData(t *testing.T) {
	t.Run("successful fetch with transcript and comments", func(t *testing.T) {
		testLogger := logger.NewTestLogger()

		mockExtractor := NewMockContentExtractor(t)
		mockExtractor.EXPECT().Extract(
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return(createMockResult(), nil).Once()
		mockHTTP := NewMockHTTPClient(t)
		mockHTTP.EXPECT().Get(mock.Anything).
			Return(createHTTPResponse(200, "Test subtitle text"), nil).Once()

		service := Service{
			config:           Config{},
			logger:           testLogger,
			contentExtractor: mockExtractor,
			subtitleFetcher:  NewSubtitleFetcher(mockHTTP, testLogger),
			commentProcessor: &CommentProcessor{maxComments: 30},
		}

		result, err := service.FetchYoutubeData(
			"https://youtube.com/watch?v=test",
			FetchTranscript|FetchComments,
			10,
		)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "Test Video", result.Title)
		assert.Contains(t, result.Transcript, "Test subtitle text")
		assert.NotEmpty(t, result.Comments)
	})

	t.Run("fetch without transcript", func(t *testing.T) {
		testLogger := logger.NewTestLogger()

		mockExtractor := NewMockContentExtractor(t)
		mockExtractor.EXPECT().Extract(
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return(createMockResult(), nil).Once()

		service := Service{
			config:           Config{},
			logger:           testLogger,
			contentExtractor: mockExtractor,
			commentProcessor: &CommentProcessor{maxComments: 30},
		}

		result, err := service.FetchYoutubeData(
			"https://youtube.com/watch?v=test",
			FetchComments,
			10,
		)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Transcript)
		assert.NotEmpty(t, result.Comments)
	})

	t.Run("extraction error", func(t *testing.T) {
		testLogger := logger.NewTestLogger()

		mockExtractor := NewMockContentExtractor(t)
		mockExtractor.EXPECT().Extract(
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return(nil, errors.New("err")).Once()

		service := Service{
			config:           Config{},
			logger:           testLogger,
			contentExtractor: mockExtractor,
		}

		result, err := service.FetchYoutubeData("https://youtube.com/watch?v=test", 0, 0)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrExtractYoutubeData)
	})

	t.Run("no video info available", func(t *testing.T) {
		testLogger := logger.NewTestLogger()

		mockExtractor := NewMockContentExtractor(t)
		mockExtractor.EXPECT().Extract(
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return(createMockResultWithoutInfo(), nil).Once()

		service := Service{
			config:           Config{},
			logger:           testLogger,
			contentExtractor: mockExtractor,
		}

		result, err := service.FetchYoutubeData("https://youtube.com/watch?v=test", 0, 0)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrNoVideoInfo)
	})
}

func TestService_extractVideoInfo(t *testing.T) {
	t.Run("all fields present", func(t *testing.T) {
		service := Service{}

		title := "Test Title"
		likeCount := 1000.0
		viewCount := 50000.0
		commentCount := 200.0
		timestamp := float64(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix())

		info := &ytdlp.ExtractedInfo{
			Title:        &title,
			LikeCount:    &likeCount,
			ViewCount:    &viewCount,
			CommentCount: &commentCount,
			Timestamp:    &timestamp,
		}

		result := service.extractVideoInfo(info)

		assert.Equal(t, "Test Title", result.Title)
		assert.Equal(t, &likeCount, result.LikeCount)
		assert.Equal(t, &viewCount, result.ViewCount)
		assert.Equal(t, &commentCount, result.CommentCount)
		assert.NotNil(t, result.UploadedAt)
	})

	t.Run("minimal info", func(t *testing.T) {
		service := Service{}
		info := &ytdlp.ExtractedInfo{}

		result := service.extractVideoInfo(info)

		assert.Empty(t, result.Title)
		assert.Nil(t, result.LikeCount)
		assert.Nil(t, result.UploadedAt)
	})
}

func TestFormatCount(t *testing.T) {
	tests := []struct {
		count    float64
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0K"},
		{9999, "10.0K"},
		{10000, "10K"},
		{999999, "1000K"},
		{1000000, "1.0M"},
		{9999999, "10.0M"},
		{10000000, "10M"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatCount(tt.count)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions

func createMockResult() *ytdlp.Result {
	title := "Test Video"
	language := "en"
	likeCount := 100.0
	viewCount := 1000.0
	commentCount := 50.0
	timestamp := float64(time.Now().Unix())

	info := &ytdlp.ExtractedInfo{
		Title:        &title,
		LikeCount:    &likeCount,
		ViewCount:    &viewCount,
		CommentCount: &commentCount,
		Timestamp:    &timestamp,
		ExtractedFormat: &ytdlp.ExtractedFormat{
			Language: &language,
		},
		Type: ytdlp.ExtractedTypeAny,
		AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
			"en": {
				{
					Name: stringPtr("English"),
					URL:  "http://example.com/subtitle.srt?fmt=srt",
				},
			},
		},
		Comments: []*ytdlp.ExtractedVideoComment{
			{
				Text:      stringPtr("Great video!"),
				Author:    stringPtr("User1"),
				LikeCount: float64Ptr(10.0),
				Timestamp: &timestamp,
			},
		},
	}

	rawJSON, _ := json.Marshal(info)
	jsonMsg := json.RawMessage(rawJSON)

	result := &ytdlp.Result{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
		OutputLogs: []*ytdlp.ResultLog{
			{
				Timestamp: time.Now(),
				Line:      string(rawJSON),
				JSON:      &jsonMsg,
				Pipe:      "stdout",
			},
		},
	}

	return result
}

func createMockResultWithoutInfo() *ytdlp.Result {
	result := &ytdlp.Result{
		ExitCode:   0,
		Stdout:     "",
		Stderr:     "",
		OutputLogs: nil,
	}

	return result
}

func createHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		Header:     make(http.Header),
	}
}
