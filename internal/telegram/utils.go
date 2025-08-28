package telegram

import (
	"net/http"
	"strings"
)

func IsImageURL(url string) bool {
	imageExts := []string{".jpg", ".jpeg", ".png", ".webp"}
	for _, ext := range imageExts {
		if strings.Contains(strings.ToLower(url), ext) {
			return true
		}
	}

	return false
}

func IsFileURL(url string) bool {
	exts := []string{".pdf"}
	for _, ext := range exts {
		if strings.Contains(strings.ToLower(url), ext) {
			return true
		}
	}

	return false
}

func IsAvailableFileURL(url string) (bool, error) {
	if IsFileURL(url) {
		return IsURLAvailable(url)
	}
	return false, nil
}

func IsAvailableImageURL(url string) (bool, error) {
	if IsImageURL(url) {
		return IsURLAvailable(url)
	}
	return false, nil
}

func IsURLAvailable(url string) (bool, error) {
	resp, err := http.Head(url)
	if err == nil && resp.StatusCode == http.StatusOK {
		return true, nil
	}

	return false, err
}
