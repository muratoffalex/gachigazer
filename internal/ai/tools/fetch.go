package tools

import (
	"errors"

	"github.com/muratoffalex/gachigazer/internal/fetch"
)

func (t Tools) Fetch(url string) (string, error) {
	content := t.fetcher.Txt(fetch.RequestPayload{
		URL: url,
	})
	if content.IsError {
		return "Error", errors.New("error")
	}
	return content.GetText(), nil
}
