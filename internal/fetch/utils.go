package fetch

import "regexp"

func ExtractStrictURLs(text string) []string {
	urlRegex := regexp.MustCompile(`https?://[a-zA-Z0-9\p{L}\p{N}\-._~:/?#\[\]@!$&'()*+,;=%]+[a-zA-Z0-9\p{L}\p{N}\-._~:/?#\[\]@!$&'()*+,;=%]`)
	return urlRegex.FindAllString(text, -1)
}

func IsURL(text string) bool {
	urlRegex := regexp.MustCompile(`^https?://[a-zA-Z0-9\p{L}\p{N}\-._~:/?#\[\]@!$&'()*+,;=%]+[a-zA-Z0-9\p{L}\p{N}\-._~:/?#\[\]@!$&'()*+,;=%]$`)
	return urlRegex.MatchString(text)
}
