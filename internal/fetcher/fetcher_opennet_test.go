package fetcher

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestOpennetFetcher_Handle(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)

		htmlContent, err := os.ReadFile("testdata/opennet_success.html")
		require.NoError(t, err, "Failed to read test HTML file")

		mockClient.EXPECT().
			Do(mock.MatchedBy(func(req *http.Request) bool {
				return req.URL.String() == "https://www.opennet.ru/opennews/art.shtml?num=12345"
			})).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(htmlContent)),
				Header:     make(http.Header),
			}, nil)

		fetcher := NewOpennetFetcher(l, mockClient)

		request, err := NewRequestPayload(
			"https://www.opennet.ru/opennews/art.shtml?num=12345",
			nil,
			nil,
		)
		require.NoError(t, err, "Failed to create request")

		response, err := fetcher.Handle(request)
		require.NoError(t, err, "Handle should not return error")
		assert.False(t, response.IsError, "Response should not be an error")
		assert.Len(t, response.Content, 1, "Should have exactly one content item")
		assert.Equal(t, ContentTypeText, response.Content[0].Type, "Content type should be text")

		expectedText := `TITLE: Test Article Title on Opennet

CONTENT:
This is test content of the article for Opennet parser. It contains multiple lines and some to other articles. Here is another paragraph with more information about the topic.

COMMENTS:
——
#1, TestUser1, 14:30, 05/01/2026 | +5
First comment text with some and formatting. This is a test comment for the parser.
——
#2, TestUser2, 15:45, 05/01/2026
Second comment without rating. Just plain text here.
——
#3, TestUser3, 16:00, 05/01/2026 | -2
Third comment with negative rating. Some information here.
——
#4, LongUsernameExample, 17:15, 05/01/2026 | +15
Comment with high rating and longer text to test parsing of various content types. Multiple lines and special characters: !@#$%^&*().
——
#5, 18:30, 05/01/2026 | 0
Anonymous comment without author name.`

		assert.Equal(t, expectedText, response.Content[0].Text, "Response text should match expected")
	})

	t.Run("NoContent", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)

		mockClient.EXPECT().
			Do(mock.AnythingOfType("*http.Request")).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte("<html><body></body></html>"))),
				Header:     make(http.Header),
			}, nil)

		fetcher := NewOpennetFetcher(l, mockClient)

		request, err := NewRequestPayload(
			"https://www.opennet.ru/opennews/art.shtml?num=99999",
			nil,
			nil,
		)
		require.NoError(t, err, "Failed to create request")

		response, err := fetcher.Handle(request)
		require.Error(t, err, "Handle should not return error")
		assert.True(t, response.IsError, "Response should be an error for no content")
		assert.Equal(t, "no opennet content found", response.Content[0].Text, "Should return no content message")
	})

	t.Run("HTTPError", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)

		mockClient.EXPECT().
			Do(mock.AnythingOfType("*http.Request")).
			Return(nil, assert.AnError)

		fetcher := NewOpennetFetcher(l, mockClient)

		request, err := NewRequestPayload(
			"https://www.opennet.ru/opennews/art.shtml?num=12345",
			nil,
			nil,
		)
		require.NoError(t, err, "Failed to create request")

		response, err := fetcher.Handle(request)
		assert.Error(t, err, "Should return error for HTTP failure")
		assert.True(t, response.IsError, "Response should be an error")
	})

	t.Run("CommentsLimit", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)

		var htmlContent strings.Builder
		htmlContent.WriteString(`
		<html>
		<body>
			<table class="thdr2"><tr><td>Test Title</td></tr></table>
			<td class="chtext">Test content</td>
		`)

		for i := 1; i <= 60; i++ {
			htmlContent.WriteString(`
			<table class="cblk">
				<tr>
					<td class="chdr">
						<a href="/openforum/"><font>#` + string(rune('0'+i)) + `</font></a>
						<a class="nick">User` + string(rune('0'+i)) + `</a>
						14:30, 05/01/2026
					</td>
				</tr>
				<tr>
					<td class="ctxt">Comment ` + string(rune('0'+i)) + `</td>
				</tr>
			</table>`)
		}
		htmlContent.WriteString(`</body></html>`)

		mockClient.EXPECT().
			Do(mock.AnythingOfType("*http.Request")).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(htmlContent.String()))),
				Header:     make(http.Header),
			}, nil)

		fetcher := NewOpennetFetcher(l, mockClient)

		request, err := NewRequestPayload(
			"https://www.opennet.ru/opennews/art.shtml?num=12345",
			nil,
			nil,
		)
		require.NoError(t, err, "Failed to create request")

		response, err := fetcher.Handle(request)
		require.NoError(t, err, "Handle should not return error")
		assert.False(t, response.IsError, "Response should not be an error")

		content := response.Content[0].Text
		commentCount := strings.Count(content, "——")
		assert.LessOrEqual(t, commentCount, 50, "Should limit comments to 50")
	})
}
