package fetcher

import (
	"net/http"
	"testing"
	"time"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/service/youtube"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewYoutubeRequest(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		headers := map[string]string{"User-Agent": "test"}
		fetchFlags := youtube.FetchTranscript | youtube.FetchComments
		maxComments := 50

		req, err := NewYoutubeRequest(
			"https://youtube.com/watch?v=test123",
			headers,
			fetchFlags,
			maxComments,
		)

		require.NoError(t, err)
		assert.Equal(t, "https://youtube.com/watch?v=test123", req.URL())
		assert.Equal(t, headers, req.Headers())
		assert.Equal(t, fetchFlags, req.fetchFlags)
		assert.Equal(t, maxComments, req.maxComments)
		assert.Equal(t, map[string]any{
			OptYtFetchFlags:  fetchFlags,
			OptYtMaxComments: maxComments,
		}, req.Options())
	})

	t.Run("invalid URL", func(t *testing.T) {
		req, err := NewYoutubeRequest(
			"not-a-valid-url",
			nil,
			youtube.FetchTranscript,
			0,
		)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCreateYoutubeRequest)
		assert.Equal(t, YoutubeRequest{}, req)
	})
}

func TestYoutubeFetcher_Handle_Success(t *testing.T) {
	l := logger.NewTestLogger()
	mockService := newMockYoutubeService(t)

	uploadTime := time.Date(2026, 1, 6, 15, 30, 0, 0, time.UTC)
	likeCount := float64(1500)
	viewCount := float64(50000)
	commentCount := float64(300)

	expectedData := &youtube.YoutubeData{
		Title:        "Test Video Title",
		Transcript:   "This is a test video transcript.\nContains multiple lines of text.",
		UploadedAt:   &uploadTime,
		LikeCount:    &likeCount,
		ViewCount:    &viewCount,
		CommentCount: &commentCount,
		Comments:     "- user1: Great video!\n- user2: Thanks for the information",
	}

	mockService.EXPECT().
		FetchYoutubeData(
			"https://youtube.com/watch?v=test123",
			youtube.FetchTranscript|youtube.FetchComments,
			10,
		).
		Return(expectedData, nil)

	fetcher := NewYoutubeFetcher(l, &http.Client{}, mockService)

	request, err := NewYoutubeRequest(
		"https://youtube.com/watch?v=test123",
		nil,
		youtube.FetchTranscript|youtube.FetchComments,
		10,
	)
	require.NoError(t, err)

	response, err := fetcher.Handle(request)
	require.NoError(t, err)
	assert.False(t, response.IsError)
	assert.Len(t, response.Content, 1)
	assert.Equal(t, ContentTypeText, response.Content[0].Type)

	expectedText := `Title: Test Video Title
Uploaded at: 2026-01-06 15:30:00 | Views: 50K | Likes: 1.5K | Comments count: 300
This is a test video transcript. Contains multiple lines of text.
Popular comments:
- user1: Great video!
- user2: Thanks for the information`

	assert.Equal(t, expectedText, response.Content[0].Text)
}

func TestYoutubeFetcher_Handle_Success_NoComments(t *testing.T) {
	l := logger.NewTestLogger()
	mockService := newMockYoutubeService(t)

	uploadTime := time.Date(2026, 1, 6, 15, 30, 0, 0, time.UTC)

	expectedData := &youtube.YoutubeData{
		Title:      "Video Without Comments",
		Transcript: "Transcript without comments.",
		UploadedAt: &uploadTime,
		// Other fields nil
	}

	mockService.EXPECT().
		FetchYoutubeData(
			"https://youtu.be/short",
			youtube.FetchTranscript,
			0,
		).
		Return(expectedData, nil)

	fetcher := NewYoutubeFetcher(l, &http.Client{}, mockService)

	request, err := NewYoutubeRequest(
		"https://youtu.be/short",
		nil,
		youtube.FetchTranscript,
		0,
	)
	require.NoError(t, err)

	response, err := fetcher.Handle(request)
	require.NoError(t, err)
	assert.False(t, response.IsError)

	expectedText := `Title: Video Without Comments
Uploaded at: 2026-01-06 15:30:00
Transcript without comments.`
	assert.Equal(t, expectedText, response.Content[0].Text)
}

func TestYoutubeFetcher_Handle_Success_OnlyTranscript(t *testing.T) {
	l := logger.NewTestLogger()
	mockService := newMockYoutubeService(t)

	expectedData := &youtube.YoutubeData{
		Title:      "Video With Only Transcript",
		Transcript: "Simple transcript.",
		// All metadata nil
	}

	mockService.EXPECT().
		FetchYoutubeData(
			"https://youtube.com/watch?v=simple",
			youtube.FetchTranscript,
			0,
		).
		Return(expectedData, nil)

	fetcher := NewYoutubeFetcher(l, &http.Client{}, mockService)

	request, err := NewYoutubeRequest(
		"https://youtube.com/watch?v=simple",
		nil,
		youtube.FetchTranscript,
		0,
	)
	require.NoError(t, err)

	response, err := fetcher.Handle(request)
	require.NoError(t, err)
	assert.False(t, response.IsError)

	expectedText := `Title: Video With Only Transcript

Simple transcript.`
	assert.Equal(t, expectedText, response.Content[0].Text)
}

func TestYoutubeFetcher_Handle_ServiceError(t *testing.T) {
	l := logger.NewTestLogger()
	mockService := newMockYoutubeService(t)

	mockService.EXPECT().
		FetchYoutubeData(
			"https://youtube.com/watch?v=error",
			youtube.FetchTranscript,
			0,
		).
		Return(nil, assert.AnError)

	fetcher := NewYoutubeFetcher(l, &http.Client{}, mockService)

	request, err := NewYoutubeRequest(
		"https://youtube.com/watch?v=error",
		nil,
		youtube.FetchTranscript,
		0,
	)
	require.NoError(t, err)

	response, err := fetcher.Handle(request)
	assert.Error(t, err)
	assert.True(t, response.IsError)
}

func TestYoutubeFetcher_Handle_CleanText(t *testing.T) {
	l := logger.NewTestLogger()
	mockService := newMockYoutubeService(t)

	expectedData := &youtube.YoutubeData{
		Title:      "Video With Dirty Text",
		Transcript: "  Text with  extra   spaces  \n\nAnd empty lines  \n\n\nAnd tabs\t",
	}

	mockService.EXPECT().
		FetchYoutubeData(
			mock.AnythingOfType("string"),
			mock.AnythingOfType("youtube.FetchFlag"),
			mock.AnythingOfType("int"),
		).
		Return(expectedData, nil)

	fetcher := NewYoutubeFetcher(l, &http.Client{}, mockService)

	request, err := NewYoutubeRequest(
		"https://youtube.com/watch?v=clean",
		nil,
		youtube.FetchTranscript,
		0,
	)
	require.NoError(t, err)

	response, err := fetcher.Handle(request)
	require.NoError(t, err)

	// Check that text is cleaned
	assert.NotContains(t, response.Content[0].Text, "  ")     // Double spaces
	assert.NotContains(t, response.Content[0].Text, "\n\n\n") // Triple newlines
}

func TestYoutubeFetcher_Handle_WithCommentsOnly(t *testing.T) {
	l := logger.NewTestLogger()
	mockService := newMockYoutubeService(t)

	expectedData := &youtube.YoutubeData{
		Title:    "Video With Comments Only",
		Comments: "- user1: First comment\n- user2: Second comment\n- user3: Third comment",
	}

	mockService.EXPECT().
		FetchYoutubeData(
			"https://youtube.com/watch?v=comments",
			youtube.FetchComments,
			20,
		).
		Return(expectedData, nil)

	fetcher := NewYoutubeFetcher(l, &http.Client{}, mockService)

	request, err := NewYoutubeRequest(
		"https://youtube.com/watch?v=comments",
		nil,
		youtube.FetchComments,
		20,
	)
	require.NoError(t, err)

	response, err := fetcher.Handle(request)
	require.NoError(t, err)
	assert.False(t, response.IsError)

	expectedText := `Title: Video With Comments Only


Popular comments:
- user1: First comment
- user2: Second comment
- user3: Third comment`
	assert.Equal(t, expectedText, response.Content[0].Text)
}

func TestYoutubeFetcher_Handle_EmptyTranscript(t *testing.T) {
	l := logger.NewTestLogger()
	mockService := newMockYoutubeService(t)

	uploadTime := time.Date(2026, 1, 6, 15, 30, 0, 0, time.UTC)

	expectedData := &youtube.YoutubeData{
		Title:      "Video With Empty Transcript",
		Transcript: "", // Empty transcript
		UploadedAt: &uploadTime,
	}

	mockService.EXPECT().
		FetchYoutubeData(
			"https://youtube.com/watch?v=empty",
			youtube.FetchTranscript,
			0,
		).
		Return(expectedData, nil)

	fetcher := NewYoutubeFetcher(l, &http.Client{}, mockService)

	request, err := NewYoutubeRequest(
		"https://youtube.com/watch?v=empty",
		nil,
		youtube.FetchTranscript,
		0,
	)
	require.NoError(t, err)

	response, err := fetcher.Handle(request)
	require.NoError(t, err)
	assert.False(t, response.IsError)

	expectedText := `Title: Video With Empty Transcript
Uploaded at: 2026-01-06 15:30:00
`
	assert.Equal(t, expectedText, response.Content[0].Text)
}
