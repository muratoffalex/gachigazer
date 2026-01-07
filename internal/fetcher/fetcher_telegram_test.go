package fetcher

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTelegramFetcher_Handle_Success(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	htmlContent := `<!DOCTYPE html>
<html>
<body>
    <div data-post="channelname/123">
        <div class="tgme_widget_message_text">Тестовый пост в Telegram канале</div>
        <div class="tgme_widget_message_photo_wrap" style="background-image: url('https://cdn5.telesco.pe/file/test_image.jpg')"></div>
    </div>
</body>
</html>`

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://t.me/channelname/123"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(htmlContent))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewTelegramFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://t.me/channelname/123",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Len(t, response.Content, 1, "Should have exactly one content item")
	assert.Equal(t, ContentTypeText, response.Content[0].Type, "Content type should be text")

	expectedText := `Post text: Тестовый пост в Telegram канале
Image: https://cdn5.telesco.pe/file/test_image.jpg`
	assert.Equal(t, expectedText, response.Content[0].Text, "Response text should match expected")
}

func TestTelegramFetcher_Handle_SuccessWithCaption(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	htmlContent := `<!DOCTYPE html>
<html>
<body>
    <div data-post="channelname/456">
        <div class="tgme_widget_message_caption">Подпись к медиафайлу</div>
        <div class="tgme_widget_message_photo_wrap" style="background-image: url('https://cdn5.telesco.pe/file/media_image.jpg')"></div>
    </div>
</body>
</html>`

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://t.me/channelname/456"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(htmlContent))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewTelegramFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://t.me/channelname/456",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Len(t, response.Content, 1, "Should have exactly one content item")
	assert.Equal(t, ContentTypeText, response.Content[0].Type, "Content type should be text")

	expectedText := `Post text: Подпись к медиафайлу
Image: https://cdn5.telesco.pe/file/media_image.jpg`
	assert.Equal(t, expectedText, response.Content[0].Text, "Response text should match expected")
}

func TestTelegramFetcher_Handle_PostNotFound(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	htmlContent := `<!DOCTYPE html>
<html>
<body>
    <div data-post="otherchannel/111">
        <div class="tgme_widget_message_text">Другой пост</div>
    </div>
</body>
</html>`

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://t.me/channelname/999"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(htmlContent))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewTelegramFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://t.me/channelname/999",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.True(t, response.IsError, "Response should be an error for post not found")
	assert.Equal(t, "Failed to find a post channelname/999", response.Content[0].Text, "Should return post not found message")
}

func TestTelegramFetcher_Handle_EmptyMessage(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	htmlContent := `<!DOCTYPE html>
<html>
<body>
    <div data-post="channelname/789">
        <div class="tgme_widget_message_text"></div>
        <div class="tgme_widget_message_caption"></div>
    </div>
</body>
</html>`

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://t.me/channelname/789"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(htmlContent))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewTelegramFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://t.me/channelname/789",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.True(t, response.IsError, "Response should be an error for empty message")
	assert.Equal(t, "Empty message or failed to extract text", response.Content[0].Text, "Should return empty message error")
}

func TestTelegramFetcher_Handle_HTTPError(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(nil, assert.AnError)

	fetcher := NewTelegramFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://t.me/channelname/123",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	assert.Error(t, err, "Should return error for HTTP failure")
	assert.True(t, response.IsError, "Response should be an error")
}

func TestTelegramFetcher_Handle_ShortLinkFormat(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	htmlContent := `<!DOCTYPE html>
<html>
<body>
    <div data-post="channelname/123">
        <div class="tgme_widget_message_text">Пост из короткой ссылки</div>
    </div>
</body>
</html>`

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://t.me/s/channelname/123"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(htmlContent))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewTelegramFetcher(l, mockClient)

	// Test with short link format (t.me/s/)
	request, err := NewRequestPayload(
		"https://t.me/s/channelname/123",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Len(t, response.Content, 1, "Should have exactly one content item")
	assert.Equal(t, "Post text: Пост из короткой ссылки", response.Content[0].Text, "Response text should match expected")
}

func TestTelegramFetcher_Handle_MultipleImages(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	htmlContent := `<!DOCTYPE html>
<html>
<body>
    <div data-post="channelname/555">
        <div class="tgme_widget_message_text">Пост с несколькими изображениями</div>
        <div class="tgme_widget_message_photo_wrap" style="background-image: url('https://cdn5.telesco.pe/file/image1.jpg')"></div>
        <div class="tgme_widget_message_photo_wrap" style="background-image: url('https://cdn5.telesco.pe/file/image2.jpg')"></div>
        <div class="tgme_widget_message_photo_wrap" style="background-image: url('https://cdn5.telesco.pe/file/image3.jpg')"></div>
    </div>
</body>
</html>`

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://t.me/channelname/555"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(htmlContent))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewTelegramFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://t.me/channelname/555",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")

	expectedText := `Post text: Пост с несколькими изображениями
Image: https://cdn5.telesco.pe/file/image1.jpg
Image: https://cdn5.telesco.pe/file/image2.jpg
Image: https://cdn5.telesco.pe/file/image3.jpg`
	assert.Equal(t, expectedText, response.Content[0].Text, "Should extract all images")
}
