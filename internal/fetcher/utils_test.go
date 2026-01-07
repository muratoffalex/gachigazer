package fetcher

import (
	"testing"
)

func TestExtractStrictURLs(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "simple HTTP URL",
			text: "Check out http://example.com for more info",
			want: []string{"http://example.com"},
		},
		{
			name: "simple HTTPS URL",
			text: "Visit https://example.com",
			want: []string{"https://example.com"},
		},
		{
			name: "URL with Cyrillic characters",
			text: "Reddit post: https://www.reddit.com/r/Escapism_is_my_realm/comments/1pxxd29/%D1%82%D0%B8%D1%82%D0%BB%D0%B5_%D0%BC%D0%B0%D0%BC%D0%BC%D0%B8_%D0%B8%D1%88%D1%83%D1%81/",
			want: []string{"https://www.reddit.com/r/Escapism_is_my_realm/comments/1pxxd29/%D1%82%D0%B8%D1%82%D0%BB%D0%B5_%D0%BC%D0%B0%D0%BC%D0%BC%D0%B8_%D0%B8%D1%88%D1%83%D1%81/"},
		},
		{
			name: "multiple URLs",
			text: "Visit https://example.com and http://test.org",
			want: []string{"https://example.com", "http://test.org"},
		},
		{
			name: "URL with query parameters",
			text: "Search: https://example.com/search?q=test&lang=en",
			want: []string{"https://example.com/search?q=test&lang=en"},
		},
		{
			name: "URL with fragment",
			text: "See https://example.com/page#section",
			want: []string{"https://example.com/page#section"},
		},
		{
			name: "URL with port",
			text: "Local server: http://localhost:8080/api",
			want: []string{"http://localhost:8080/api"},
		},
		{
			name: "no URLs",
			text: "Just some plain text without links",
			want: nil,
		},
		{
			name: "URL with special characters",
			text: "API: https://api.example.com/v1/users?id=123&name=John+Doe",
			want: []string{"https://api.example.com/v1/users?id=123&name=John+Doe"},
		},
		{
			name: "URL with Unicode domain",
			text: "Check https://пример.рф/страница",
			want: []string{"https://пример.рф/страница"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractStrictURLs(tt.text)
			if !equalStringSlices(got, tt.want) {
				t.Errorf("ExtractStrictURLs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "valid HTTP URL",
			text: "http://example.com",
			want: true,
		},
		{
			name: "valid HTTPS URL",
			text: "https://example.com",
			want: true,
		},
		{
			name: "valid URL with Cyrillic characters",
			text: "https://www.reddit.com/r/Escapism_is_my_realm/comments/1pxxd29/%D1%82%D0%B8%D1%82%D0%BB%D0%B5_%D0%BC%D0%B0%D0%BC%D0%BC%D0%B8_%D0%B8%D1%88%D1%83%D1%81/",
			want: true,
		},
		{
			name: "valid URL with path",
			text: "https://example.com/path/to/resource",
			want: true,
		},
		{
			name: "valid URL with query",
			text: "https://example.com/search?q=test",
			want: true,
		},
		{
			name: "valid URL with fragment",
			text: "https://example.com/page#section",
			want: true,
		},
		{
			name: "valid URL with port",
			text: "http://localhost:8080",
			want: true,
		},
		{
			name: "valid URL with Unicode domain",
			text: "https://пример.рф",
			want: true,
		},
		{
			name: "not a URL - plain text",
			text: "just some text",
			want: false,
		},
		{
			name: "not a URL - missing protocol",
			text: "example.com",
			want: false,
		},
		{
			name: "not a URL - wrong protocol",
			text: "ftp://example.com",
			want: false,
		},
		{
			name: "not a URL - URL with space",
			text: "https://example.com with space",
			want: false,
		},
		{
			name: "not a URL - empty string",
			text: "",
			want: false,
		},
		{
			name: "not a URL - text before URL",
			text: "Check https://example.com",
			want: false,
		},
		{
			name: "not a URL - text after URL",
			text: "https://example.com here",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsURL(tt.text)
			if got != tt.want {
				t.Errorf("IsURL(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

// Benchmarks
func BenchmarkExtractStrictURLs(b *testing.B) {
	text := "Visit https://example.com and check https://www.reddit.com/r/test/comments/123/%D1%82%D0%B5%D1%81%D1%82/"

	for b.Loop() {
		ExtractStrictURLs(text)
	}
}

func BenchmarkIsURL(b *testing.B) {
	url := "https://www.reddit.com/r/Escapism_is_my_realm/comments/1pxxd29/%D1%82%D0%B8%D1%82%D0%BB%D0%B5_%D0%BC%D0%B0%D0%BC%D0%BC%D0%B8_%D0%B8%D1%88%D1%83%D1%81/"

	for b.Loop() {
		IsURL(url)
	}
}

func BenchmarkExtractStrictURLs_NoMatch(b *testing.B) {
	text := "Just some plain text without any URLs at all"

	for b.Loop() {
		ExtractStrictURLs(text)
	}
}

// Helpers
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
