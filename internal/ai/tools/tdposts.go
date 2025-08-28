package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/service"
)

const TG_MAX_DURATION = 720 // 1 month

func (t Tools) Fetch_tg_posts(
	channelName string,
	duration string,
	limit int,
) (string, error) {
	td := service.GetTD()
	var err error
	d := time.Duration(0)
	sinceDate := time.Time{}
	if duration != "" {
		d, err = time.ParseDuration(duration)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), err
		}
		if d.Hours() > TG_MAX_DURATION {
			d = TG_MAX_DURATION * time.Hour
		}
		sinceDate = time.Now().Add(-d)
	}
	if limit < 0 {
		limit = 0
	}

	posts, err := td.GetChannelPosts(context.Background(), channelName, sinceDate, limit, 0)
	if err != nil {
		return "Error", err
	}

	if len(posts) == 0 {
		return "Not found", nil
	}

	return fmt.Sprintf("Found results: %d\n%s", len(posts), strings.Join(posts, "\n---\n")), nil
}
