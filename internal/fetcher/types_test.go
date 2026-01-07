package fetcher

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequestPayloadWithMethod(t *testing.T) {
	t.Run("valid URL without headers or options", func(t *testing.T) {
		url := "https://example.com"
		req, err := NewRequestPayload(url, nil, nil)

		require.NoError(t, err)
		assert.Equal(t, url, req.URL())
		assert.Nil(t, req.Headers())
		assert.Nil(t, req.Options())
	})

	t.Run("valid URL with headers and options", func(t *testing.T) {
		URL := "https://example.com/path?query=1"
		headers := map[string]string{"User-Agent": "test"}
		options := map[string]any{"timeout": 30}

		req, err := NewRequestPayload(URL, headers, options)

		require.NoError(t, err)
		assert.Equal(t, URL, req.URL())
		assert.Equal(t, headers, req.Headers())
		assert.Equal(t, options, req.Options())

		parsedURL, err := url.Parse(URL)
		require.NoError(t, err)
		assert.NotNil(t, parsedURL)
	})

	t.Run("empty URL returns error", func(t *testing.T) {
		req, err := NewRequestPayload("", nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "URL cannot be empty")
		assert.Equal(t, RequestPayload{}, req)
	})

	t.Run("URL with only whitespace returns error", func(t *testing.T) {
		req, err := NewRequestPayload("   ", nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "URL cannot be empty")
		assert.Equal(t, RequestPayload{}, req)
	})

	t.Run("invalid URL returns error", func(t *testing.T) {
		req, err := NewRequestPayload("://invalid", nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid URL")
		assert.Equal(t, RequestPayload{}, req)
	})
}

func TestRequestPayload_Methods(t *testing.T) {
	t.Run("accessor methods return correct values", func(t *testing.T) {
		url := "https://example.com"
		headers := map[string]string{
			"User-Agent": "test-agent",
			"Accept":     "application/json",
		}
		options := map[string]any{
			"timeout": 30,
			"retry":   true,
		}

		req, err := NewRequestPayload(url, headers, options)
		require.NoError(t, err)

		assert.Equal(t, url, req.URL())
		assert.Equal(t, headers, req.Headers())
		assert.Equal(t, options, req.Options())
	})

	t.Run("nil headers and options are handled", func(t *testing.T) {
		url := "https://example.com"
		req, err := NewRequestPayload(url, nil, nil)
		require.NoError(t, err)

		assert.Equal(t, url, req.URL())
		assert.Nil(t, req.Headers())
		assert.Nil(t, req.Options())
	})
}
