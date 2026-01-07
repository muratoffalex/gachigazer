package youtube

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lrstanley/go-ytdlp"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSubtitleFetcher_fetchSubtitles(t *testing.T) {
	tests := []struct {
		name           string
		srtContent     string
		expectedText   string
		expectedError  bool
		serverResponse int
	}{
		{
			name: "successful SRT to text conversion",
			srtContent: `1
00:00:00,000 --> 00:00:02,000
Hello world!

2
00:00:02,000 --> 00:00:04,000
This is a test.

3
00:00:04,000 --> 00:00:06,000
Multiple lines
in one subtitle.`,
			expectedText:   "Hello world! This is a test. Multiple lines in one subtitle.",
			expectedError:  false,
			serverResponse: http.StatusOK,
		},
		{
			name:           "empty SRT file",
			srtContent:     "",
			expectedText:   "",
			expectedError:  false,
			serverResponse: http.StatusOK,
		},
		{
			name:           "SRT with only timestamps and numbers",
			srtContent:     "1\n00:00:00,000 --> 00:00:02,000\n\n2\n00:00:02,000 --> 00:00:04,000",
			expectedText:   "",
			expectedError:  false,
			serverResponse: http.StatusOK,
		},
		{
			name:           "HTTP error response",
			srtContent:     "",
			expectedText:   "",
			expectedError:  true,
			serverResponse: http.StatusNotFound,
		},
		{
			name: "SRT with extra whitespace",
			srtContent: `1
00:00:00,000 --> 00:00:02,000
  Hello world!  

2
00:00:02,000 --> 00:00:04,000
    Test with spaces  `,
			expectedText:   "Hello world! Test with spaces",
			expectedError:  false,
			serverResponse: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.serverResponse != http.StatusOK {
					w.WriteHeader(tt.serverResponse)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.srtContent))
			}))
			defer server.Close()

			// Create SubtitleFetcher with test server client
			testLogger := logger.NewTestLogger()
			sf := &SubtitleFetcher{
				httpClient: server.Client(),
				logger:     testLogger,
			}

			// Call fetchSubtitles
			result, err := sf.fetchSubtitles(server.URL)

			// Assert results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedText, result)
			}
		})
	}
}

func TestSubtitleFetcher_fetchSubtitles_NetworkError(t *testing.T) {
	testLogger := logger.NewTestLogger()

	// Create a client that will fail
	sf := &SubtitleFetcher{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
		logger: testLogger,
	}

	// Use an invalid URL that will cause network error
	_, err := sf.fetchSubtitles("http://localhost:99999/invalid")
	assert.Error(t, err)
}

func TestSubtitleFetcher_getSubtitleURL(t *testing.T) {
	tests := []struct {
		name           string
		info           *ytdlp.ExtractedInfo
		language       string
		expectedURL    string
		expectedError  string
		expectLogging  bool
		expectedFields map[string]any
	}{
		{
			name: "successful with exact language match",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("en"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"en": {
						{
							Name: stringPtr("English"),
							URL:  "http://example.com/subtitle.srt?fmt=srt",
						},
					},
				},
			},
			language:      "en",
			expectedURL:   "http://example.com/subtitle.srt?fmt=srt",
			expectedError: "",
			expectLogging: true,
			expectedFields: map[string]any{
				"language":           "en",
				"available_captions": 1,
			},
		},
		{
			name: "successful with base language match (en-US -> en)",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("en"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"en": {
						{
							Name: stringPtr("English"),
							URL:  "http://example.com/subtitle.srt?fmt=srt",
						},
					},
				},
			},
			language:      "en-US",
			expectedURL:   "http://example.com/subtitle.srt?fmt=srt",
			expectedError: "",
			expectLogging: true,
			expectedFields: map[string]any{
				"language":           "en-US",
				"available_captions": 1,
			},
		},
		{
			name: "no captions available - nil Language",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: nil,
				},
				AutomaticCaptions: nil,
			},
			language:      "en",
			expectedURL:   "",
			expectedError: "no captions available for this video",
			expectLogging: false,
		},
		{
			name: "no captions for requested language",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("en"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"es": {
						{
							Name: stringPtr("Spanish"),
							URL:  "http://example.com/subtitle.srt?fmt=srt",
						},
					},
				},
			},
			language:      "en",
			expectedURL:   "",
			expectedError: "no captions available for language: en",
			expectLogging: false,
		},
		{
			name: "no SRT format available",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("en"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"en": {
						{
							Name: stringPtr("English"),
							URL:  "http://example.com/subtitle.vtt",
						},
					},
				},
			},
			language:      "en",
			expectedURL:   "",
			expectedError: "no subtitle URL found",
			expectLogging: true,
			expectedFields: map[string]any{
				"language":           "en",
				"available_captions": 1,
			},
		},
		{
			name: "empty URL in caption",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("en"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"en": {
						{
							Name: stringPtr("English"),
							URL:  "",
						},
					},
				},
			},
			language:      "en",
			expectedURL:   "",
			expectedError: "no subtitle URL found",
			expectLogging: true,
			expectedFields: map[string]any{
				"language":           "en",
				"available_captions": 1,
			},
		},
		{
			name: "multiple captions, select first SRT",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("en"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"en": {
						{
							Name: stringPtr("English VTT"),
							URL:  "http://example.com/subtitle.vtt",
						},
						{
							Name: stringPtr("English SRT"),
							URL:  "http://example.com/subtitle.srt?fmt=srt",
						},
						{
							Name: stringPtr("Another SRT"),
							URL:  "http://example.com/another.srt?fmt=srt",
						},
					},
				},
			},
			language:      "en",
			expectedURL:   "http://example.com/subtitle.srt?fmt=srt",
			expectedError: "",
			expectLogging: true,
			expectedFields: map[string]any{
				"language":           "en",
				"available_captions": 3,
			},
		},
		{
			name: "case insensitive SRT check",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("en"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"en": {
						{
							Name: stringPtr("English"),
							URL:  "http://example.com/subtitle.srt?FMT=SRT",
						},
					},
				},
			},
			language:      "en",
			expectedURL:   "http://example.com/subtitle.srt?FMT=SRT",
			expectedError: "",
			expectLogging: true,
			expectedFields: map[string]any{
				"language":           "en",
				"available_captions": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testLogger := logger.NewTestLogger()

			sf := &SubtitleFetcher{
				httpClient: &http.Client{},
				logger:     testLogger,
			}

			result, err := sf.getSubtitleURL(tt.info, tt.language)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, result)
			}

			// Check logging
			if tt.expectLogging {
				assert.True(t, testLogger.CountEntries() > 0, "Should have log entries")
				entries := testLogger.GetEntries()
				assert.Equal(t, "debug", entries[0].Level, "Log level should be debug")
				assert.Equal(t, "Available captions", entries[0].Message, "Log message should match")

				// Check fields if provided
				if tt.expectedFields != nil {
					for key, expectedValue := range tt.expectedFields {
						assert.Equal(t, expectedValue, entries[0].Fields[key], "Field %s should match", key)
					}
				}
			} else {
				assert.Equal(t, 0, testLogger.CountEntries(), "Should not have any log entries")
			}
		})
	}
}

func TestSubtitleFetcher_getSubtitleURL_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		info          *ytdlp.ExtractedInfo
		language      string
		expectedError string
		expectLogging bool
	}{
		{
			name: "nil caption name",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("en"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"en": {
						{
							Name: nil,
							URL:  "http://example.com/subtitle.srt?fmt=srt",
						},
					},
				},
			},
			language:      "en",
			expectedError: "",
			expectLogging: true,
		},
		{
			name: "language with dash in the middle",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("zh"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"zh-CN": {
						{
							Name: stringPtr("Chinese"),
							URL:  "http://example.com/subtitle.srt?fmt=srt",
						},
					},
				},
			},
			language:      "zh-CN",
			expectedError: "",
			expectLogging: true,
		},
		{
			name: "multiple base language attempts",
			info: &ytdlp.ExtractedInfo{
				ExtractedFormat: &ytdlp.ExtractedFormat{
					Language: stringPtr("en"),
				},
				AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
					"en": {
						{
							Name: stringPtr("English"),
							URL:  "http://example.com/subtitle.srt?fmt=srt",
						},
					},
					"en-US": {
						{
							Name: stringPtr("English US"),
							URL:  "http://example.com/subtitle-us.srt?fmt=srt",
						},
					},
				},
			},
			language:      "en-GB",
			expectedError: "",
			expectLogging: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testLogger := logger.NewTestLogger()
			sf := &SubtitleFetcher{
				httpClient: &http.Client{},
				logger:     testLogger,
			}

			result, err := sf.getSubtitleURL(tt.info, tt.language)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
			}

			// Check logging
			if tt.expectLogging {
				assert.True(t, testLogger.CountEntries() > 0, "Should have log entries")
				entries := testLogger.GetEntries()
				assert.Equal(t, "debug", entries[0].Level, "Log level should be debug")
				assert.Equal(t, "Available captions", entries[0].Message, "Log message should match")
			} else {
				assert.Equal(t, 0, testLogger.CountEntries(), "Should not have any log entries")
			}
		})
	}
}

func TestSubtitleFetcher_Fetch(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		mockHTTP := NewMockHTTPClient(t)
		mockHTTP.EXPECT().Get(mock.Anything).Return(createHTTPResponse(200, `1
00:00:00,000 --> 00:00:02,000
Hello world!`), nil)

		testLogger := logger.NewTestLogger()
		sf := &SubtitleFetcher{
			httpClient: mockHTTP,
			logger:     testLogger,
		}

		info := &ytdlp.ExtractedInfo{
			ExtractedFormat: &ytdlp.ExtractedFormat{
				Language: stringPtr("en"),
			},
			AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
				"en": {
					{
						Name: stringPtr("English"),
						URL:  "http://example.com/subtitle.srt?fmt=srt",
					},
				},
			},
		}

		result, err := sf.Fetch(info)
		assert.NoError(t, err)
		assert.Equal(t, "Hello world!", result)
	})

	t.Run("no video language", func(t *testing.T) {
		testLogger := logger.NewTestLogger()
		sf := &SubtitleFetcher{
			httpClient: &http.Client{},
			logger:     testLogger,
		}

		info := &ytdlp.ExtractedInfo{
			ExtractedFormat: &ytdlp.ExtractedFormat{},
		}

		_, err := sf.Fetch(info)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNoVideoLanguage)
	})

	t.Run("failed to get subtitle URL", func(t *testing.T) {
		testLogger := logger.NewTestLogger()
		sf := &SubtitleFetcher{
			httpClient: &http.Client{},
			logger:     testLogger,
		}

		info := &ytdlp.ExtractedInfo{
			ExtractedFormat: &ytdlp.ExtractedFormat{
				Language: stringPtr("en"),
			},
			AutomaticCaptions: nil,
		}

		_, err := sf.Fetch(info)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrGetSubtitleURL)
	})

	t.Run("failed to fetch transcript", func(t *testing.T) {
		mockHTTP := NewMockHTTPClient(t)
		mockHTTP.EXPECT().Get(mock.Anything).Return(nil, errors.New("err")).Once()

		testLogger := logger.NewTestLogger()
		sf := &SubtitleFetcher{
			httpClient: mockHTTP,
			logger:     testLogger,
		}

		info := &ytdlp.ExtractedInfo{
			ExtractedFormat: &ytdlp.ExtractedFormat{
				Language: stringPtr("en"),
			},
			AutomaticCaptions: map[string][]*ytdlp.ExtractedSubtitle{
				"en": {
					{
						Name: stringPtr("English"),
						URL:  "http://example.com/subtitle.srt?fmt=srt",
					},
				},
			},
		}

		_, err := sf.Fetch(info)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrFetchTranscript)
	})
}
