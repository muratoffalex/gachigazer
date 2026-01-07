package fetcher

import (
	"fmt"
	"regexp"
)

var urlRegex = regexp.MustCompile(`https?://[a-zA-Z0-9\p{L}\p{N}\-._~:/?#\[\]@!$&'()*+,;=%]+[a-zA-Z0-9\p{L}\p{N}\-._~:/?#\[\]@!$&'()*+,;=%]`)

func ExtractStrictURLs(text string) []string {
	return urlRegex.FindAllString(text, -1)
}

func IsURL(text string) bool {
	urls := ExtractStrictURLs(text)
	return len(urls) == 1 && urls[0] == text
}

func FormatCount(count float64) string {
	switch {
	case count < 1000:
		return fmt.Sprintf("%.0f", count)
	case count < 10000:
		return fmt.Sprintf("%.1fK", count/1000)
	case count < 1000000:
		return fmt.Sprintf("%.0fK", count/1000)
	case count < 10000000:
		return fmt.Sprintf("%.1fM", count/1000000)
	default:
		return fmt.Sprintf("%.0fM", count/1000000)
	}
}
