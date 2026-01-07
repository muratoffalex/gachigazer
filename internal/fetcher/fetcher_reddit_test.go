package fetcher

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRedditFetcher_Handle_Success(t *testing.T) {
	t.Run("successful post with all elements", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)

		// Read test HTML file
		htmlContent, err := os.ReadFile("testdata/reddit_success.html")
		require.NoError(t, err, "Failed to read test HTML file")

		mockClient.EXPECT().
			Do(mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(htmlContent)),
				Header:     make(http.Header),
			}, nil)

		fetcher := NewRedditFetcher(l, mockClient)

		request, err := NewRequestPayload(
			"https://old.reddit.com/r/programming/comments/abc123/test_post/",
			nil,
			nil,
		)
		require.NoError(t, err, "Failed to create request")

		response, err := fetcher.Handle(request)
		require.NoError(t, err, "Handle should not return error")
		assert.False(t, response.IsError, "Response should not be an error")
		assert.Len(t, response.Content, 2, "Should have text and URL content items")

		// Find text content
		var textContent Content
		for _, c := range response.Content {
			if c.Type == ContentTypeText {
				textContent = c
				break
			}
		}

		// TODO: MUST BE ALSO IMAGES BLOCK, but telegram.IsAvailableImageUrl needs refactoring
		// IMAGES:
		// Image: https://i.redd.it/test_image.jpg
		// Video preview: https://i.redd.it/test_video_preview.jpg
		expectedText := `TITLE: Test Programming Post
INFO: Posted by testuser at 2026-01-07T10:30:00Z, +256 points
TEXT:
This is a test post about programming. It contains some example text to test the Reddit fetcher. URL: https://example.com/article
COMMENTS:
👤 commenter1 (+42): This is a great post!
👤 commenter2 (+15): I agree with the points made here.
👤 commenter3: This needs more explanation.
👤 commenter4 (+5): Thanks for sharing this information.
👤 commenter5 (-2): I disagree with this approach.`

		assert.Equal(t, expectedText, textContent.Text, "Response text should match expected")

		// Check URL content
		var urlContent Content
		for _, c := range response.Content {
			if c.Type == ContentTypeURL {
				urlContent = c
				break
			}
		}
		assert.Equal(t, "https://example.com/article", urlContent.Text, "URL content should match")
	})

	t.Run("post with only title and metadata", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)

		htmlContent := `<!DOCTYPE html>
<html>
<head><title>Reddit: Minimal Post</title></head>
<body>
	<div id="siteTable">
		<div class="thing">
			<a class="title" href="/r/test/comments/xyz">Minimal Test Post</a>
			<div class="score unvoted">123</div>
			<p class="tagline">
				<a class="author" href="/user/minimaluser">minimaluser</a>
				<time datetime="2026-01-07T08:15:00Z"></time>
			</p>
		</div>
	</div>
</body>
</html>`

		mockClient.EXPECT().
			Do(mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(htmlContent)),
				Header:     make(http.Header),
			}, nil)

		fetcher := NewRedditFetcher(l, mockClient)

		request, err := NewRequestPayload(
			"https://old.reddit.com/r/test/comments/xyz",
			nil,
			nil,
		)
		require.NoError(t, err, "Failed to create request")

		response, err := fetcher.Handle(request)
		require.NoError(t, err, "Handle should not return error")
		assert.False(t, response.IsError, "Response should not be an error")
		assert.Len(t, response.Content, 1, "Should have only text content")

		expectedText := `TITLE: Minimal Test Post
INFO: Posted by minimaluser at 2026-01-07T08:15:00Z, 123 points`

		assert.Equal(t, expectedText, response.Content[0].Text, "Response text should match expected")
	})
}

func TestRedditFetcher_Handle_NoContent(t *testing.T) {
	t.Run("empty HTML response", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)

		mockClient.EXPECT().
			Do(mock.AnythingOfType("*http.Request")).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte("<html><body></body></html>"))),
				Header:     make(http.Header),
			}, nil)

		fetcher := NewRedditFetcher(l, mockClient)

		request, err := NewRequestPayload(
			"https://old.reddit.com/r/empty/comments/999",
			nil,
			nil,
		)
		require.NoError(t, err, "Failed to create request")

		response, err := fetcher.Handle(request)
		require.NoError(t, err, "Handle should not return error")
		assert.True(t, response.IsError, "Response should be an error for no content")
		assert.Equal(t, "No old.reddit content found", response.Content[0].Text, "Should return no content message")
	})

	t.Run("missing siteTable element", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)

		htmlContent := []byte(`
			<html>
				<body>
					<div id="otherDiv">
						Some other content
					</div>
				</body>
			</html>`)

		mockClient.EXPECT().
			Do(mock.AnythingOfType("*http.Request")).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(htmlContent)),
				Header:     make(http.Header),
			}, nil)

		fetcher := NewRedditFetcher(l, mockClient)

		request, err := NewRequestPayload(
			"https://old.reddit.com/r/missing/comments/777",
			nil,
			nil,
		)
		require.NoError(t, err, "Failed to create request")

		response, err := fetcher.Handle(request)
		require.NoError(t, err, "Handle should not return error")
		assert.True(t, response.IsError, "Response should be an error for no content")
		assert.Equal(t, "No old.reddit content found", response.Content[0].Text, "Should return no content message")
	})
}

func TestRedditFetcher_Handle_HTTPError(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(nil, assert.AnError)

	fetcher := NewRedditFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://old.reddit.com/r/error/comments/123",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	assert.Error(t, err, "Should return error for HTTP failure")
	assert.True(t, response.IsError, "Response should be an error")
}

func TestRedditFetcher_extractMetadata(t *testing.T) {
	t.Run("full metadata extraction", func(t *testing.T) {
		l := logger.NewTestLogger()
		fetcher := NewRedditFetcher(l, &http.Client{})

		html := `
			<div id="siteTable">
				<div class="score unvoted">+999</div>
				<p class="tagline">
					<a class="author" href="/user/testauthor">testauthor</a>
					<time datetime="2026-01-07T12:00:00Z">2 hours ago</time>
				</p>
			</div>`

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
		require.NoError(t, err, "Failed to parse HTML")

		siteTable := doc.Find("div#siteTable")
		metadata := fetcher.extractMetadata(siteTable)

		assert.Equal(t, "+999", metadata.score, "Score should match")
		assert.Len(t, metadata.metadataParts, 2, "Should have 2 metadata parts")
		assert.Contains(t, metadata.metadataParts, "Posted by testauthor", "Should contain author")
		assert.Contains(t, metadata.metadataParts, "at 2026-01-07T12:00:00Z", "Should contain timestamp")
	})

	t.Run("metadata without timestamp", func(t *testing.T) {
		l := logger.NewTestLogger()
		fetcher := NewRedditFetcher(l, &http.Client{})

		html := `
			<div id="siteTable">
				<div class="score unvoted">42</div>
				<p class="tagline">
					<a class="author" href="/user/authoronly">authoronly</a>
				</p>
			</div>`

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
		require.NoError(t, err, "Failed to parse HTML")

		siteTable := doc.Find("div#siteTable")
		metadata := fetcher.extractMetadata(siteTable)

		assert.Equal(t, "42", metadata.score, "Score should match")
		assert.Len(t, metadata.metadataParts, 1, "Should have 1 metadata part")
		assert.Equal(t, "Posted by authoronly", metadata.metadataParts[0], "Should contain only author")
	})

	t.Run("empty metadata", func(t *testing.T) {
		l := logger.NewTestLogger()
		fetcher := NewRedditFetcher(l, &http.Client{})

		html := `<div id="siteTable"></div>`

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
		require.NoError(t, err, "Failed to parse HTML")

		siteTable := doc.Find("div#siteTable")
		metadata := fetcher.extractMetadata(siteTable)

		assert.Equal(t, "", metadata.score, "Score should be empty")
		assert.Empty(t, metadata.metadataParts, "Metadata parts should be empty")
	})
}

func TestRedditFetcher_extractPostTextAndURL(t *testing.T) {
	t.Run("post with text and external URL", func(t *testing.T) {
		l := logger.NewTestLogger()
		fetcher := NewRedditFetcher(l, &http.Client{})

		html := `
			<div id="siteTable">
				<div class="usertext-body">
					<p>This is the post text content.</p>
					<p>It has multiple paragraphs.</p>
				</div>
				<a data-event-action="title" data-href-url="https://external.com/article">
					Post Title
				</a>
			</div>`

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
		require.NoError(t, err, "Failed to parse HTML")

		siteTable := doc.Find("div#siteTable")
		postText, urlContent := fetcher.extractPostTextAndURL(siteTable)

		expectedText := "This is the post text content.\n\t\t\t\t\tIt has multiple paragraphs. URL: https://external.com/article"
		assert.Equal(t, expectedText, postText, "Post text should match")
		require.NotNil(t, urlContent, "URL content should not be nil")
		assert.Equal(t, ContentTypeURL, urlContent.Type, "Content type should be URL")
		assert.Equal(t, "https://external.com/article", urlContent.Text, "URL should match")
	})

	t.Run("post with text only (no URL)", func(t *testing.T) {
		l := logger.NewTestLogger()
		fetcher := NewRedditFetcher(l, &http.Client{})

		html := `
			<div id="siteTable">
				<div class="usertext-body">
					Text-only post
				</div>
			</div>`

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
		require.NoError(t, err, "Failed to parse HTML")

		siteTable := doc.Find("div#siteTable")
		postText, urlContent := fetcher.extractPostTextAndURL(siteTable)

		assert.Equal(t, "Text-only post", postText, "Post text should match")
		assert.Nil(t, urlContent, "URL content should be nil for text-only posts")
	})

	t.Run("post with image URL (should be excluded)", func(t *testing.T) {
		l := logger.NewTestLogger()
		fetcher := NewRedditFetcher(l, &http.Client{})

		html := `
			<div id="siteTable">
				<a data-event-action="title" data-href-url="https://i.redd.it/image.jpg">
					Image Post
				</a>
			</div>`

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
		require.NoError(t, err, "Failed to parse HTML")

		siteTable := doc.Find("div#siteTable")
		postText, urlContent := fetcher.extractPostTextAndURL(siteTable)

		assert.Equal(t, "", postText, "Post text should be empty")
		assert.Nil(t, urlContent, "URL content should be nil for image URLs")
	})
}

func TestRedditFetcher_extractComments(t *testing.T) {
	t.Run("extract multiple comments", func(t *testing.T) {
		l := logger.NewTestLogger()
		fetcher := NewRedditFetcher(l, &http.Client{})

		html := `
			<div class="commentarea">
				<div class="sitetable">
					<div class="comment">
						<a class="author" href="/user/user1">user1</a>
						<div class="usertext-body">First comment text</div>
						<span class="score unvoted">+100</span>
					</div>
					<div class="comment">
						<a class="author" href="/user/user2">user2</a>
						<div class="usertext-body">Second comment</div>
						<span class="score unvoted">-5</span>
					</div>
					<div class="comment">
						<div class="usertext-body">Anonymous comment</div>
					</div>
				</div>
			</div>`

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
		require.NoError(t, err, "Failed to parse HTML")

		comments := fetcher.extractComments(doc)

		expected := `👤 user1 (+100): First comment text
👤 user2 (-5): Second comment
👤 Anonymous: Anonymous comment
`
		assert.Equal(t, expected, comments, "Comments should be extracted correctly")
	})

	t.Run("limit to 50 comments", func(t *testing.T) {
		l := logger.NewTestLogger()
		fetcher := NewRedditFetcher(l, &http.Client{})

		var htmlBuilder strings.Builder
		htmlBuilder.WriteString(`<div class="commentarea"><div class="sitetable">`)
		for i := 1; i <= 60; i++ {
			fmt.Fprintf(&htmlBuilder, `
				<div class="comment">
					<a class="author" href="/user/user%d">user%d</a>
					<div class="usertext-body">Comment %d</div>
					<span class="score unvoted">+%d</span>
				</div>`, i, i, i, i)
		}
		htmlBuilder.WriteString(`</div></div>`)

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBuilder.String()))
		require.NoError(t, err, "Failed to parse HTML")

		comments := fetcher.extractComments(doc)

		// Count lines in comments
		lines := strings.Count(comments, "\n")
		assert.Equal(t, 50, lines, "Should extract exactly 50 comments")
	})

	t.Run("empty comments section", func(t *testing.T) {
		l := logger.NewTestLogger()
		fetcher := NewRedditFetcher(l, &http.Client{})

		html := `<div class="commentarea"></div>`

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
		require.NoError(t, err, "Failed to parse HTML")

		comments := fetcher.extractComments(doc)

		assert.Equal(t, "", comments, "Comments should be empty")
	})
}

func TestRedditFetcher_buildFullText(t *testing.T) {
	l := logger.NewTestLogger()
	fetcher := NewRedditFetcher(l, &http.Client{})

	t.Run("complete post with all sections", func(t *testing.T) {
		metadata := postMetadata{
			score:         "+256",
			metadataParts: []string{"Posted by testuser", "at 2026-01-07T10:30:00Z"},
		}

		result := fetcher.buildFullText(
			"Test Title",
			metadata,
			"This is the post text.",
			"Image: https://example.com/image.jpg\nImage: https://example.com/image2.png",
			"👤 user1 (+10): First comment\n👤 user2: Second comment",
		)

		expected := `TITLE: Test Title
INFO: Posted by testuser at 2026-01-07T10:30:00Z, +256 points
TEXT:
This is the post text.
IMAGES:
Image: https://example.com/image.jpg
Image: https://example.com/image2.png
COMMENTS:
👤 user1 (+10): First comment
👤 user2: Second comment`

		assert.Equal(t, expected, result, "Full text should be built correctly")
	})

	t.Run("post with only title and metadata", func(t *testing.T) {
		metadata := postMetadata{
			score:         "42",
			metadataParts: []string{"Posted by simpleuser"},
		}

		result := fetcher.buildFullText(
			"Simple Post",
			metadata,
			"",
			"",
			"",
		)

		expected := `TITLE: Simple Post
INFO: Posted by simpleuser, 42 points`

		assert.Equal(t, expected, result, "Should handle minimal post correctly")
	})

	t.Run("post with empty title", func(t *testing.T) {
		metadata := postMetadata{
			score:         "0",
			metadataParts: []string{"Posted by anon"},
		}

		result := fetcher.buildFullText(
			"",
			metadata,
			"Some text content",
			"",
			"",
		)

		expected := `INFO: Posted by anon, 0 points
TEXT:
Some text content`

		assert.Equal(t, expected, result, "Should handle empty title correctly")
	})
}

func TestRedditFetcher_getOldRedditURL(t *testing.T) {
	l := logger.NewTestLogger()

	t.Run("convert new reddit to old reddit", func(t *testing.T) {
		mockClient := NewMockHTTPClient(t)
		fetcher := NewRedditFetcher(l, mockClient)

		// Mock the parseRedditURL call
		mockClient.EXPECT().
			Do(mock.AnythingOfType("*http.Request")).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`<div id="canonical-url-updater" value="https://www.reddit.com/r/test/comments/abc123/post_title/"></div>`))),
				Header:     make(http.Header),
			}, nil)

		result := fetcher.getOldRedditURL("https://www.reddit.com/r/test/comments/abc123/post_title/")

		expected := "https://old.reddit.com/r/test/comments/abc123/post_title/"
		assert.Equal(t, expected, result, "Should convert to old.reddit.com")
	})

	t.Run("already old reddit URL", func(t *testing.T) {
		fetcher := NewRedditFetcher(l, &http.Client{})

		result := fetcher.getOldRedditURL("https://old.reddit.com/r/test/comments/xyz")

		assert.Equal(t, "https://old.reddit.com/r/test/comments/xyz", result, "Should return same URL for old.reddit.com")
	})

	t.Run("non-reddit URL", func(t *testing.T) {
		fetcher := NewRedditFetcher(l, &http.Client{})

		result := fetcher.getOldRedditURL("https://example.com/page")

		assert.Equal(t, "https://example.com/page", result, "Should return same URL for non-reddit domains")
	})
}
