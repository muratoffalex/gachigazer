package tools

import (
	"errors"

	fetch "github.com/muratoffalex/gachigazer/internal/fetcher"
)

func (t Tools) Fetch_url(url string) (string, error) {
	req, err := fetch.NewRequestPayload(url, nil, nil)
	if err != nil {
		return "Error", err
	}
	content, _ := t.fetcher.Fetch(req)
	if content.IsError {
		return "Error", errors.New("error")
	}
	return content.GetText(), nil
}
