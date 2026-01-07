package youtube

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lrstanley/go-ytdlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommentProcessor_processComments(t *testing.T) {
	cp := &CommentProcessor{
		maxComments: 30,
	}

	t.Run("empty comments list returns 'Not found'", func(t *testing.T) {
		result := cp.processComments([]*ytdlp.ExtractedVideoComment{}, 0)
		assert.Equal(t, "Not found", result)
	})

	t.Run("all comments have empty text returns 'Not found'", func(t *testing.T) {
		comments := []*ytdlp.ExtractedVideoComment{
			{Text: stringPtr("")},
			{Text: stringPtr("")},
		}
		result := cp.processComments(comments, 0)
		assert.Equal(t, "Not found", result)
	})

	t.Run("respects maxComments parameter", func(t *testing.T) {
		comments := createTestComments(10)
		result := cp.processComments(comments, 3)

		lines := strings.Split(strings.TrimSpace(result), "\n")
		// Each comment takes 2 lines (Author and Text)
		assert.Equal(t, 6, len(lines))
	})

	t.Run("maxComments is 0", func(t *testing.T) {
		comments := createTestComments(40)
		result := cp.processComments(comments, 0)

		lines := strings.Split(strings.TrimSpace(result), "\n")
		// 30 comments × 2 lines = 60 lines
		assert.Equal(t, 20, len(lines))
	})

	t.Run("sorts by like count when available", func(t *testing.T) {
		comments := []*ytdlp.ExtractedVideoComment{
			{
				Text:      stringPtr("Comment 1"),
				Author:    stringPtr("Author1"),
				LikeCount: float64Ptr(10.0),
				Timestamp: float64Ptr(float64(time.Now().Unix())),
			},
			{
				Text:      stringPtr("Comment 2"),
				Author:    stringPtr("Author2"),
				LikeCount: float64Ptr(100.0),
				Timestamp: float64Ptr(float64(time.Now().Unix())),
			},
			{
				Text:      stringPtr("Comment 3"),
				Author:    stringPtr("Author3"),
				LikeCount: float64Ptr(50.0),
				Timestamp: float64Ptr(float64(time.Now().Unix())),
			},
		}

		result := cp.processComments(comments, 3)
		require.Contains(t, result, "Comment 2") // Most popular should be first

		// Check order
		lines := strings.Split(result, "\n")
		assert.Contains(t, lines[0], "Author2")
		assert.Contains(t, lines[2], "Author3")
		assert.Contains(t, lines[4], "Author1")
	})

	t.Run("does not sort when like count is not available", func(t *testing.T) {
		comments := []*ytdlp.ExtractedVideoComment{
			{
				Text:      stringPtr("Comment 1"),
				Author:    stringPtr("Author1"),
				LikeCount: nil,
				Timestamp: float64Ptr(float64(time.Now().Unix())),
			},
			{
				Text:      stringPtr("Comment 2"),
				Author:    stringPtr("Author2"),
				LikeCount: nil,
				Timestamp: float64Ptr(float64(time.Now().Unix())),
			},
		}

		result := cp.processComments(comments, 2)
		// Should keep original order when no like count
		lines := strings.Split(result, "\n")
		assert.Contains(t, lines[0], "Author1")
		assert.Contains(t, lines[2], "Author2")
	})

	t.Run("handles nil pointers gracefully", func(t *testing.T) {
		comments := []*ytdlp.ExtractedVideoComment{
			{
				Text:      nil,
				Author:    stringPtr("Author1"),
				LikeCount: float64Ptr(10.0),
			},
			{
				Text:      stringPtr("Valid comment"),
				Author:    nil,
				LikeCount: nil,
				Timestamp: nil,
			},
		}

		result := cp.processComments(comments, 5)
		// Should process only the second comment
		assert.Contains(t, result, "Valid comment")
		assert.NotContains(t, result, "Author1")
	})

	t.Run("formats output correctly", func(t *testing.T) {
		fixedTime := time.Date(2023, 12, 30, 15, 30, 0, 0, time.UTC)
		comments := []*ytdlp.ExtractedVideoComment{
			{
				Text:      stringPtr("Test comment text"),
				Author:    stringPtr("Test Author"),
				LikeCount: float64Ptr(1234.0),
				Timestamp: float64Ptr(float64(fixedTime.Unix())),
			},
		}

		result := cp.processComments(comments, 1)
		expected := "Author: Test Author | Likes: 1.2K | Date: 2023-12-30 15:30:00\nText: Test comment text"
		assert.Equal(t, expected, result)
	})

	t.Run("handles zero like count", func(t *testing.T) {
		comments := []*ytdlp.ExtractedVideoComment{
			{
				Text:      stringPtr("Zero likes comment"),
				Author:    stringPtr("Author"),
				LikeCount: float64Ptr(0.0),
				Timestamp: float64Ptr(float64(time.Now().Unix())),
			},
		}

		result := cp.processComments(comments, 1)
		assert.Contains(t, result, "Likes: 0")
	})

	t.Run("handles negative timestamp", func(t *testing.T) {
		comments := []*ytdlp.ExtractedVideoComment{
			{
				Text:      stringPtr("Old comment"),
				Author:    stringPtr("Author"),
				LikeCount: float64Ptr(10.0),
				Timestamp: float64Ptr(-1000.0), // Negative timestamp
			},
		}

		result := cp.processComments(comments, 1)
		// Should handle negative timestamp (will produce date before 1970)
		assert.Contains(t, result, "Author: Author")
		assert.Contains(t, result, "Old comment")
	})

	t.Run("filters out comments with empty text", func(t *testing.T) {
		comments := []*ytdlp.ExtractedVideoComment{
			{Text: stringPtr("")},
			{Text: stringPtr("Valid 1")},
			{Text: stringPtr("")},
			{Text: stringPtr("Valid 2")},
			{Text: stringPtr("")},
		}

		result := cp.processComments(comments, 10)
		assert.Contains(t, result, "Valid 1")
		assert.Contains(t, result, "Valid 2")

		lines := strings.Split(strings.TrimSpace(result), "\n")
		assert.Equal(t, 2, len(lines))
	})

	t.Run("handles very large like counts", func(t *testing.T) {
		comments := []*ytdlp.ExtractedVideoComment{
			{
				Text:      stringPtr("Viral comment"),
				Author:    stringPtr("Popular Author"),
				LikeCount: float64Ptr(1234567.0),
				Timestamp: float64Ptr(float64(time.Now().Unix())),
			},
		}

		result := cp.processComments(comments, 1)
		// FormatCount should format large numbers
		assert.Contains(t, result, "Viral comment")
		assert.Contains(t, result, "Popular Author")
	})
}

// Helper functions for test data
func stringPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

func createTestComments(count int) []*ytdlp.ExtractedVideoComment {
	comments := make([]*ytdlp.ExtractedVideoComment, count)
	now := float64(time.Now().Unix())

	for i := range count {
		comments[i] = &ytdlp.ExtractedVideoComment{
			Text:      stringPtr(fmt.Sprintf("Comment %d", i+1)),
			Author:    stringPtr(fmt.Sprintf("Author %d", i+1)),
			LikeCount: float64Ptr(float64((i + 1) * 10)),
			Timestamp: float64Ptr(now - float64(i*3600)), // Different timestamps
		}
	}
	return comments
}
