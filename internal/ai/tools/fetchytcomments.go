package tools

import (
	"fmt"

	"github.com/muratoffalex/gachigazer/internal/fetch"
)

func (t Tools) Fetch_yt_comments(url string, max int) (string, error) {
	content, err := t.fetcher.FetchYoutubeData(url, fetch.FetchComments, max)
	if err != nil {
		return fmt.Sprintf("Error: %v", err.Error()), err
	}
	return fmt.Sprintf("Fetched comments:\n%s", content.Comments), nil
}
