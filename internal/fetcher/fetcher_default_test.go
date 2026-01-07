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

func TestDefaultFetcher_Handle_HTMLContent(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	htmlContent := `<!DOCTYPE html>
<html>
<head>
	<title>Test Page</title>
	<style>.hidden { display: none; }</style>
	<script>alert('test');</script>
</head>
<body>
	<h1>Main Title</h1>
	<div class="hidden">This should be removed</div>
	<p>First paragraph.</p>
	<p>Second paragraph with <strong>bold</strong> text.</p>
	<!-- Comment should be removed -->
	<nav>Navigation</nav>
	<footer>Footer content</footer>
</body>
</html>`

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(htmlContent))),
			Header: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
		}, nil)

	fetcher := NewDefaultFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://example.com/test",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Len(t, response.Content, 1, "Should have exactly one content item")
	assert.Equal(t, ContentTypeText, response.Content[0].Type, "Content type should be text")

	expectedText := `Test Page Main Title First paragraph. Second paragraph with bold text.`

	assert.Equal(t, expectedText, response.Content[0].Text, "Should extract and clean HTML text")
}

func TestDefaultFetcher_Handle_PlainText(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	plainText := `This is plain text content.
It has multiple lines.
And some special characters: !@#$%^&*()`

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(plainText))),
			Header: http.Header{
				"Content-Type": []string{"text/plain; charset=utf-8"},
			},
		}, nil)

	fetcher := NewDefaultFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://example.com/text.txt",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Len(t, response.Content, 1, "Should have exactly one content item")
	assert.Equal(t, ContentTypeText, response.Content[0].Type, "Content type should be text")

	// For plain text, it should return the original text unchanged
	assert.Equal(t, plainText, response.Content[0].Text, "Should return plain text as-is")
}

func TestDefaultFetcher_Handle_JSONContent(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	jsonContent := `{
		"name": "Test",
		"value": 123,
		"items": ["a", "b", "c"]
	}`

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(jsonContent))),
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
		}, nil)

	fetcher := NewDefaultFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://api.example.com/data",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")

	// JSON is not HTML, so it should return raw text
	assert.Equal(t, jsonContent, response.Content[0].Text, "Should return JSON as plain text")
}

func TestDefaultFetcher_Handle_InvalidHTML(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	invalidHTML := `<html><body><p>Unclosed tag`

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(invalidHTML))),
			Header: http.Header{
				"Content-Type": []string{"text/html"},
			},
		}, nil)

	fetcher := NewDefaultFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://example.com/broken",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	// goquery usually parses even invalid HTML
	require.NoError(t, err, "Handle should not return error for invalid HTML")
	assert.False(t, response.IsError, "Response should not be an error")

	assert.Contains(t, response.Content[0].Text, "Unclosed tag", "Should extract text from invalid HTML")
}

func TestDefaultFetcher_Handle_HTTPError(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(nil, assert.AnError)

	fetcher := NewDefaultFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://example.com/error",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	assert.Error(t, err, "Should return error for HTTP failure")
	assert.True(t, response.IsError, "Response should be an error")
}

func TestDefaultFetcher_Handle_EmptyResponse(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(""))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewDefaultFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://example.com/empty",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error for empty response")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Equal(t, "", response.Content[0].Text, "Should return empty string for empty response")
}

func TestDefaultFetcher_Handle_WithImagesInHTML(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	htmlWithImages := `<!DOCTYPE html>
<html>
<body>
	<h1>Page with Images</h1>
	<p>Text before image.</p>
	<img src="https://example.com/image1.jpg" alt="Image 1">
	<p>Text between images.</p>
	<img src="https://example.com/image2.png" alt="Image 2">
	<p>Text after image.</p>
</body>
</html>`

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(htmlWithImages))),
			Header: http.Header{
				"Content-Type": []string{"text/html"},
			},
		}, nil)

	fetcher := NewDefaultFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://example.com/images",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")

	expectedText := `Page with Images Text before image. Text between images. Text after image.`

	assert.Equal(t, expectedText, response.Content[0].Text, "Should remove image tags and keep only text")
}
