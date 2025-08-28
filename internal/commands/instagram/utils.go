package instagram

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	InstagramURLPattern = `https?://(?:www\.)?instagram\.com(?:/[A-Za-z0-9_.-]+)?(?:/p|/reel|/reels|/share/reel|/share/p|/stories/[^/]+)/[A-Za-z0-9_-]+`

	// Separate pattern for stories, as they have a different ID format
	InstagramStoryURLPattern = `instagram\.com/stories/[^/]+/([0-9]+)`
	// Pattern for regular posts
	InstagramPostURLPattern = `instagram\.com(?:/[A-Za-z0-9_.-]+)?(?:/p|/reel|/reels|/share/reel|/share/p)/([A-Za-z0-9_-]+)`
)

var (
	instagramRegex = regexp.MustCompile(InstagramURLPattern)
	storyRegex     = regexp.MustCompile(InstagramStoryURLPattern)
	postRegex      = regexp.MustCompile(InstagramPostURLPattern)
)

func ExtractShortcode(url string) string {
	// First, try as a story URL
	if strings.Contains(url, "/stories/") {
		matches := storyRegex.FindStringSubmatch(url)
		if len(matches) >= 2 {
			if idx := strings.Index(matches[1], "?"); idx != -1 {
				return matches[1][:idx]
			}
			return matches[1]
		}
	}

	// If not a story, try as a regular post
	matches := postRegex.FindStringSubmatch(url)
	if len(matches) >= 2 {
		shortcode := matches[1]
		// Remove GET parameters if any
		if idx := strings.Index(shortcode, "?"); idx != -1 {
			shortcode = shortcode[:idx]
		}

		// If shortcode is already in numeric format, return it as is
		if _, err := strconv.ParseInt(shortcode, 10, 64); err == nil {
			return shortcode
		}

		// Convert alphanumeric shortcode to numeric format
		alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
		mediaID := int64(0)

		for _, c := range shortcode {
			mediaID = mediaID*64 + int64(strings.IndexByte(alphabet, byte(c)))
		}

		return fmt.Sprintf("%d", mediaID)
	}

	return ""
}

func ContainsInstagramURL(text string) bool {
	return instagramRegex.MatchString(text)
}

func ExtractInstagramURL(text string) string {
	matches := instagramRegex.FindString(text)
	return matches
}

func ExtractUsernameFromStoryURL(url string) string {
	re := regexp.MustCompile(`instagram\.com/stories/([^/]+)/`)
	matches := re.FindStringSubmatch(url)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}
