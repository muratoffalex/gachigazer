package fetcher

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestContent(t *testing.T) {
	content := Content{Type: ContentTypeText, Text: "test"}
	assert.Equal(t, ContentTypeText, content.Type)
	assert.Equal(t, "test", content.Text)
}

func TestNewBaseFetcher(t *testing.T) {
	t.Run("create with valid args", func(t *testing.T) {
		l := logger.NewTestLogger()
		client := &http.Client{}
		NewBaseFetcher("test", `^https://example\.com`, client, l)

		assert.Empty(t, l.GetEntries())
	})

	t.Run("create with invalid pattern", func(t *testing.T) {
		l := logger.NewTestLogger()
		client := &http.Client{}
		NewBaseFetcher("test", `[invalid`, client, l)

		assert.True(t, l.HasEntry("error", "Pattern is invalid"))
	})
}

func TestBaseFetcher_CanHandle(t *testing.T) {
	l := logger.NewTestLogger()
	client := &http.Client{}
	fetcher := NewBaseFetcher("test", `^https://example\.com`, client, l)

	t.Run("valid pattern matching", func(t *testing.T) {
		tests := []struct {
			url     string
			matches bool
		}{
			{"https://example.com/page", true},
			{"http://example.com/page", false},
			{"https://domain.com", false},
		}

		for _, tt := range tests {
			t.Run(tt.url, func(t *testing.T) {
				result := fetcher.CanHandle(tt.url)
				assert.Equal(t, tt.matches, result, "CanHandle(%q)", tt.url)
			})
		}
	})

	t.Run("empty pattern", func(t *testing.T) {
		fetcherEmpty := NewBaseFetcher("test", "", client, l)

		result := fetcherEmpty.CanHandle("https://example.com")

		assert.True(t, result, "empty pattern should match everything")
	})
}

func TestBaseFetcher_GetName(t *testing.T) {
	l := logger.NewTestLogger()
	client := &http.Client{}
	fetcher := NewBaseFetcher("test-fetcher", `.*`, client, l)

	assert.Equal(t, "test-fetcher", fetcher.GetName())
}

func TestFuncFetcher_Handle(t *testing.T) {
	l := logger.NewTestLogger()
	client := &http.Client{}
	called := false

	mockRequest := NewMockRequest(t)

	handler := func(base BaseFetcher, req Request) (Response, error) {
		called = true
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: "test response"}},
			IsError: false,
		}, nil
	}

	fetcher := NewFuncFetcher("func", `.*`, client, l, handler)
	resp, err := fetcher.Handle(mockRequest)

	assert.NoError(t, err)
	assert.True(t, called, "handler should be called")
	assert.False(t, resp.IsError)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, "test response", resp.Content[0].Text)
}

func TestBaseFetcher_cleanText(t *testing.T) {
	l := logger.NewTestLogger()
	client := &http.Client{}
	fetcher := NewBaseFetcher("test", `.*`, client, l)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple spaces",
			input:    "hello    world",
			expected: "hello world",
		},
		{
			name:     "tabs and newlines",
			input:    "hello\t\nworld",
			expected: "hello world",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  hello world  ",
			expected: "hello world",
		},
		{
			name:     "mixed whitespace",
			input:    "  hello\n\t  world\r\n  ",
			expected: "hello world",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \t\n\r   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fetcher.cleanText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseFetcher_cleanDoc(t *testing.T) {
	l := logger.NewTestLogger()
	client := &http.Client{}
	fetcher := NewBaseFetcher("test", `.*`, client, l)

	html := `
		<html>
			<body>
				<div>Content to keep</div>
				<script>alert('remove me')</script>
				<style>.hidden { display: none; }</style>
				<footer>Footer content</footer>
				<nav>Navigation</nav>
				<aside>Sidebar</aside>
				<div class="cookie-consent">Cookie banner</div>
				<div class="promoted-link">Ad</div>
				<div class="sidebar">More sidebar</div>
				<div class="login-form">Login</div>
				<div class="signup-form">Signup</div>
				<div class="hidden">Hidden content</div>
				<p>More content to keep</p>
			</body>
		</html>
	`

	doc, err := fetcher.getGoqueryDoc(html)
	require.NoError(t, err)

	fetcher.cleanDoc(doc)

	// Проверяем, что удалены ненужные элементы
	assert.Zero(t, doc.Find("script").Length(), "scripts should be removed")
	assert.Zero(t, doc.Find("style").Length(), "styles should be removed")
	assert.Zero(t, doc.Find("footer").Length(), "footer should be removed")
	assert.Zero(t, doc.Find("nav").Length(), "nav should be removed")
	assert.Zero(t, doc.Find("aside").Length(), "aside should be removed")
	assert.Zero(t, doc.Find(".cookie-consent").Length(), "cookie consent should be removed")
	assert.Zero(t, doc.Find(".promoted-link").Length(), "promoted links should be removed")
	assert.Zero(t, doc.Find(".sidebar").Length(), "sidebar should be removed")
	assert.Zero(t, doc.Find(".login-form").Length(), "login form should be removed")
	assert.Zero(t, doc.Find(".signup-form").Length(), "signup form should be removed")
	assert.Zero(t, doc.Find(".hidden").Length(), "hidden elements should be removed")

	// Проверяем, что сохранён нужный контент
	assert.Contains(t, doc.Find("div").First().Text(), "Content to keep")
	assert.Contains(t, doc.Find("p").Text(), "More content to keep")
}

func TestBaseFetcher_getGoqueryDoc(t *testing.T) {
	l := logger.NewTestLogger()
	client := &http.Client{}
	fetcher := NewBaseFetcher("test", `.*`, client, l)

	t.Run("valid HTML", func(t *testing.T) {
		html := "<html><body><h1>Test</h1></body></html>"
		doc, err := fetcher.getGoqueryDoc(html)

		require.NoError(t, err)
		assert.Equal(t, "Test", doc.Find("h1").Text())
	})

	t.Run("malformed HTML", func(t *testing.T) {
		html := "<html><body><h1>Test</body></html>"
		doc, err := fetcher.getGoqueryDoc(html)

		assert.NoError(t, err, "goquery should handle malformed HTML")
		assert.NotNil(t, doc)
	})

	t.Run("empty string", func(t *testing.T) {
		doc, err := fetcher.getGoqueryDoc("")

		assert.NoError(t, err)
		assert.NotNil(t, doc)
	})

	t.Run("complex HTML structure", func(t *testing.T) {
		html := `
			<html>
				<head><title>Page Title</title></head>
				<body>
					<div id="content">
						<p class="text">Paragraph 1</p>
						<p class="text">Paragraph 2</p>
					</div>
				</body>
			</html>
		`
		doc, err := fetcher.getGoqueryDoc(html)

		require.NoError(t, err)
		assert.Equal(t, "Page Title", doc.Find("title").Text())
		assert.Equal(t, 2, doc.Find(".text").Length())
		assert.True(t, doc.Find("#content").Length() > 0)
	})
}

func TestBaseFetcher_errorResponse(t *testing.T) {
	l := logger.NewTestLogger()
	client := &http.Client{}
	fetcher := NewBaseFetcher("test", `.*`, client, l)

	testErr := fmt.Errorf("test error message")
	resp, err := fetcher.errorResponse(testErr)

	assert.Error(t, err)
	assert.Equal(t, testErr, err)
	assert.True(t, resp.IsError)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, ContentTypeText, resp.Content[0].Type)
	assert.Equal(t, "test error message", resp.Content[0].Text)
}

func TestBaseFetcher_isHTMLContent(t *testing.T) {
	l := logger.NewTestLogger()
	client := &http.Client{}
	fetcher := NewBaseFetcher("test", `.*`, client, l)

	tests := []struct {
		name        string
		contentType string
		body        string
		expected    bool
	}{
		{
			name:        "text/html content type",
			contentType: "text/html; charset=utf-8",
			body:        "",
			expected:    true,
		},
		{
			name:        "application/xhtml+xml content type",
			contentType: "application/xhtml+xml",
			body:        "",
			expected:    true,
		},
		{
			name:        "DOCTYPE declaration",
			contentType: "text/plain",
			body:        "<!DOCTYPE html><html><body>content</body></html>",
			expected:    true,
		},
		{
			name:        "html tag",
			contentType: "text/plain",
			body:        "<html><body>content</body></html>",
			expected:    true,
		},
		{
			name:        "head tag",
			contentType: "text/plain",
			body:        "<head><title>Test</title></head><body>content</body>",
			expected:    true,
		},
		{
			name:        "body tag",
			contentType: "text/plain",
			body:        "<body>content</body>",
			expected:    true,
		},
		{
			name:        "JSON content",
			contentType: "application/json",
			body:        `{"key": "value"}`,
			expected:    false,
		},
		{
			name:        "plain text",
			contentType: "text/plain",
			body:        "Just some plain text content",
			expected:    false,
		},
		{
			name:        "empty body",
			contentType: "text/plain",
			body:        "",
			expected:    false,
		},
		{
			name:        "XML content",
			contentType: "application/xml",
			body:        "<?xml version=\"1.0\"?><root></root>",
			expected:    false,
		},
		{
			name:        "whitespace with DOCTYPE",
			contentType: "",
			body:        "   <!DOCTYPE html>\n<html><body>test</body></html>",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: http.Header{
					"Content-Type": []string{tt.contentType},
				},
			}

			result := fetcher.isHTMLContent(resp, tt.body)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseFetcher_fetch(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockResponse := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("<html><body>Test</body></html>")),
			Header: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		resp, body, err := fetcher.fetch(mockRequest)

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, body, "<html>")
		assert.Contains(t, body, "Test")
	})

	t.Run("invalid URL", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("://invalid-url")

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid URL")
	})

	t.Run("request creation failed", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com")
		mockRequest.EXPECT().Method().Return("INVALID\nMETHOD")

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create request")
	})

	t.Run("HTTP client error - EOF", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("unexpected EOF"))

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "EOF")
	})

	t.Run("HTTP client error - other", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("connection timeout"))

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request failed")
		assert.Contains(t, err.Error(), "connection timeout")
	})

	t.Run("status code 429 - rate limit", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockResponse := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rate limit exceeded (429)")
	})

	t.Run("status code 403 - forbidden", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockResponse := &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access forbidden (403)")
	})

	t.Run("status code 500 - internal server error", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockResponse := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "server error (500)")
	})

	t.Run("status code 502 - bad gateway", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockResponse := &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "bad gateway (502)")
	})

	t.Run("status code 503 - service unavailable", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockResponse := &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "service unavailable (503)")
	})

	t.Run("status code 504 - gateway timeout", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockResponse := &http.Response{
			StatusCode: http.StatusGatewayTimeout,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		_, _, err := fetcher.fetch(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "gateway timeout (504)")
	})

	t.Run("custom headers", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "value",
		})

		var capturedReq *http.Request
		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Run(func(req *http.Request) {
			capturedReq = req
		}).Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("<html></html>")),
			Header:     http.Header{"Content-Type": []string{"text/html"}},
		}, nil)

		_, _, err := fetcher.fetch(mockRequest)

		require.NoError(t, err)
		assert.NotNil(t, capturedReq)
		assert.Equal(t, "Bearer token123", capturedReq.Header.Get("Authorization"))
		assert.Equal(t, "value", capturedReq.Header.Get("X-Custom"))
		assert.NotEmpty(t, capturedReq.Header.Get("User-Agent"))
	})

	t.Run("charset conversion - EOF error", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockResponse := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{"Content-Type": []string{"text/html"}},
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		resp, body, err := fetcher.fetch(mockRequest)

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Empty(t, body)
	})
}

func TestBaseFetcher_getHTML(t *testing.T) {
	t.Run("successful HTML fetch and parse", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		htmlContent := `
			<html>
				<head><title>Test Page</title></head>
				<body>
					<h1>Header</h1>
					<p>Content</p>
				</body>
			</html>
		`

		mockResponse := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(htmlContent)),
			Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		doc, err := fetcher.getHTML(mockRequest)

		require.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, "Test Page", doc.Find("title").Text())
		assert.Equal(t, "Header", doc.Find("h1").Text())
		assert.Contains(t, doc.Find("p").Text(), "Content")
	})

	t.Run("fetch failed", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("network error"))

		_, err := fetcher.getHTML(mockRequest)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load page")
	})

	t.Run("invalid HTML", func(t *testing.T) {
		l := logger.NewTestLogger()
		mockClient := NewMockHTTPClient(t)
		fetcher := NewBaseFetcher("test", `.*`, mockClient, l)

		mockRequest := NewMockRequest(t)
		mockRequest.EXPECT().URL().Return("https://example.com/page")
		mockRequest.EXPECT().Method().Return("GET")
		mockRequest.EXPECT().Headers().Return(map[string]string{})

		// goquery usually handles even invalid HTML, so this test checks a boundary case
		mockResponse := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("<html><body><h1>Test")),
			Header:     http.Header{"Content-Type": []string{"text/html"}},
		}

		mockClient.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(mockResponse, nil)

		doc, err := fetcher.getHTML(mockRequest)

		// goquery should handle even invalid HTML
		require.NoError(t, err)
		assert.NotNil(t, doc)
	})
}
