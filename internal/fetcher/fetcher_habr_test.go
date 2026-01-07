package fetcher

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHabrFetcher_Handle_Success(t *testing.T) {
	l := logger.NewTestLogger()

	mockClient := NewMockHTTPClient(t)

	htmlContent, err := os.ReadFile("testdata/habr_success.html")
	require.NoError(t, err, "Failed to read test HTML file")

	commentsJSON, err := os.ReadFile("testdata/habr_comments.json")
	require.NoError(t, err, "Failed to read test comments JSON file")

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://habr.com/ru/articles/123456/"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(htmlContent)),
			Header:     make(http.Header),
		}, nil)

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://habr.com/kek/v2/articles/123456/comments/"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(commentsJSON)),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewHabrFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://habr.com/ru/articles/123456/",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Len(t, response.Content, 1, "Should have exactly one content item")
	assert.Equal(t, ContentTypeText, response.Content[0].Type, "Content type should be text")

	expectedText := `Title: Тестовая статья на Habr
Date: 5 января 2026
Rating: +42
Text:
Это тестовый текст статьи для проверки парсера Habr.

Images:
Image: https://habr.com/images/test1.jpg
Image: https://habr.com/images/test2.png

Comments:
- id: 29346598; score: 2; author: gun_dose; text: <blockquote><p>Новая цель - сверхразум: Система, которая справляется с ролью генерального директора крупной корпорации</p></blockquote><p>А что, в корпорациях на должность генерального выбирают самого умного?</p><p><span class="habrahidden">А ещё этот абзац в тексте задвоился</span>
- id: 29346670; parentId: 29346598; score: 0; author: vital_peresvet; text: <strong>Владелец компании</strong> желает получить самого умного на должность генерального  директора крупной корпорации. Надо быть <strong>умным владельцем</strong>!   
- id: 29346846; score: 1; author: onets; text: Че-то вспомнилось - не рой яму другому, сам в нее попадешь /s
- id: 29346944; score: 0; author: LinkToOS; text: <blockquote><p>Новая цель - сверхразум: Система, которая справляется с ролью  генерального директора крупной корпорации или главы научной лаборатории  лучше любого человека.</p></blockquote><p>И срок за человека отсидит?
- id: 29346966; parentId: 29346944; score: 0; author: inkelyad; text: <blockquote><p>И срок за человека отсидит?  </p></blockquote><p>Срок существует не просто так, а чтобы этот(и другие) товарищи так (больше) не делали.  Электронной системе мозги можно вправлять по другому.
`

	assert.Equal(t, expectedText, response.Content[0].Text, "Response text should match expected")
}

func TestHabrFetcher_Handle_NoContent(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte("<html></html>"))),
			Header:     make(http.Header),
		}, nil).Once()

	fetcher := NewHabrFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://habr.com/ru/articles/999999/",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.True(t, response.IsError, "Response should be an error for no content")
	assert.Equal(t, "No Habr content found", response.Content[0].Text, "Should return no content message")
}

func TestHabrFetcher_Handle_HTTPError(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(nil, assert.AnError)

	fetcher := NewHabrFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://habr.com/ru/articles/123456/",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	assert.Error(t, err, "Should return error for HTTP failure")
	assert.True(t, response.IsError, "Response should be an error")
}

func TestHabrFetcher_parseHabrComments(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	// Тестовые данные для комментариев
	commentsJSON := `{
		"comments": {
			"123": {
				"id": "123",
				"parentId": "",
				"author": {"alias": "user1"},
				"message": "<p>Первый комментарий</p>",
				"score": 10
			},
			"124": {
				"id": "124",
				"parentId": "123",
				"author": {"alias": "user2"},
				"message": "<div xmlns=\"http://www.w3.org/1999/xhtml\"><p>Ответ на комментарий</p></div>",
				"score": 5
			},
			"125": {
				"id": "125",
				"parentId": "",
				"author": {"alias": "user3"},
				"message": "UFO just landed and posted this here",
				"score": 0
			}
		}
	}`

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(commentsJSON))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewHabrFetcher(l, mockClient)

	// Test successful parsing
	result := fetcher.parseHabrComments("https://habr.com/ru/articles/123456/")
	expected := `- id: 123; score: 10; author: user1; text: Первый комментарий
- id: 124; parentId: 123; score: 5; author: user2; text: Ответ на комментарий
`
	assert.Equal(t, expected, result, "Should parse comments correctly")

	// Testing URL without ID
	result = fetcher.parseHabrComments("https://habr.com/ru/articles/")
	assert.Equal(t, "No valid article ID found in URL", result, "Should return error for invalid URL")

	// Test HTTP error
	mockClient2 := NewMockHTTPClient(t)
	mockClient2.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(nil, assert.AnError)

	fetcher2 := NewHabrFetcher(l, mockClient2)
	result = fetcher2.parseHabrComments("https://habr.com/ru/articles/789012/")
	assert.Contains(t, result, "Get comments error:", "Should return error message for HTTP failure")
}
