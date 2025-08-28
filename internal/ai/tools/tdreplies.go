package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/service"
)

func (t Tools) Fetch_tg_post_comments(
	channelInput string,
	postID int,
	limit int,
) (string, error) {
	td := service.GetTD()
	if limit < 0 {
		limit = 0
	}

	posts, err := td.GetPostComments(context.Background(), channelInput, postID, limit)
	if err != nil {
		return "Error", err
	}

	if len(posts) == 0 {
		return "Not found", nil
	}
	slices.Reverse(posts)

	return fmt.Sprintf("Found results: %d\n%s", len(posts), strings.Join(posts, "\n---\n")), nil
}
