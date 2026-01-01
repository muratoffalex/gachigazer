package fetch

import "regexp"

var urlRegex = regexp.MustCompile(`https?://[a-zA-Z0-9\p{L}\p{N}\-._~:/?#\[\]@!$&'()*+,;=%]+[a-zA-Z0-9\p{L}\p{N}\-._~:/?#\[\]@!$&'()*+,;=%]`)

func ExtractStrictURLs(text string) []string {
	return urlRegex.FindAllString(text, -1)
}

func IsURL(text string) bool {
	urls := ExtractStrictURLs(text)
	return len(urls) == 1 && urls[0] == text
}
