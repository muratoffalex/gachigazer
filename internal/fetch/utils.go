package fetch

import "regexp"

func ExtractStrictURLs(text string) []string {
	urlRegex := regexp.MustCompile(`https?://[\w\-._~:/?#\[\]@!$&'()*+,;=]+[\w\-._~:/?#\[\]@!$&'()*+,;=]`)
	return urlRegex.FindAllString(text, -1)
}

func IsURL(text string) bool {
	urlRegex := regexp.MustCompile(`^https?://[\w\-._~:/?#\[\]@!$&'()*+,;=]+[\w\-._~:/?#\[\]@!$&'()*+,;=]$`)
	return urlRegex.MatchString(text)
}
