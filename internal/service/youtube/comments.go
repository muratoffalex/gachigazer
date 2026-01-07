package youtube

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lrstanley/go-ytdlp"
)

type CommentProcessor struct {
	maxComments int
}

func (cp *CommentProcessor) processComments(comments []*ytdlp.ExtractedVideoComment, maxComments int) string {
	if maxComments == 0 {
		maxComments = 10
	}

	var validComments []*ytdlp.ExtractedVideoComment
	for _, comment := range comments {
		if comment.Text != nil && *comment.Text != "" {
			validComments = append(validComments, comment)
		}
	}

	if len(validComments) == 0 {
		return "Not found"
	}

	// Sort by like count if available
	if validComments[0].LikeCount != nil {
		sort.Slice(validComments, func(i, j int) bool {
			return *validComments[i].LikeCount > *validComments[j].LikeCount
		})
	}

	var result strings.Builder
	commentCount := 0

	for _, comment := range validComments {
		if commentCount >= maxComments {
			break
		}

		var lineBuilder strings.Builder

		if comment.Author != nil && *comment.Author != "" {
			fmt.Fprintf(&lineBuilder, "Author: %s", *comment.Author)
		}

		if comment.LikeCount != nil {
			if lineBuilder.Len() > 0 {
				lineBuilder.WriteString(" | ")
			}
			fmt.Fprintf(&lineBuilder, "Likes: %s", FormatCount(*comment.LikeCount))
		}

		if comment.Timestamp != nil {
			if lineBuilder.Len() > 0 {
				lineBuilder.WriteString(" | ")
			}
			timestamp := time.Unix(int64(*comment.Timestamp), 0)
			fmt.Fprintf(&lineBuilder, "Date: %s", timestamp.UTC().Format(time.DateTime))
		}

		if lineBuilder.Len() > 0 {
			fmt.Fprintf(&result, "%s\n", lineBuilder.String())
		}

		if comment.Text != nil && *comment.Text != "" {
			fmt.Fprintf(&result, "Text: %s", *comment.Text)
		}

		if commentCount < len(validComments)-1 {
			fmt.Fprintln(&result)
		}

		commentCount++
	}

	return result.String()
}
