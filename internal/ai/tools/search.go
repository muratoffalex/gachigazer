package tools

import (
	"fmt"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/service"
)

func (t Tools) Search(
	query string,
	maxResults int,
	timeLimit string,
) (string, []string, error) {
	ddg := service.NewDuckDuckGoSearch(t.httpClient, 0)

	results, err := ddg.Text(query, "", timeLimit, maxResults)
	if err != nil {
		return "Error", nil, err
	}

	if len(results) == 0 {
		return "Not found", nil, nil
	}

	list := []string{}
	links := []string{}
	for i, result := range results {
		links = append(links, result.Href)
		list = append(list, fmt.Sprintf("%d. Title: %s\nDescription: %s\nLink: %s", i+1, result.Title, result.Body, result.Href))
	}

	return fmt.Sprintf("Found %d results. To get detailed content from a specific result, use the fetch_url tool with one of the links below.\n\n%s", len(results), strings.Join(list, "\n\n")), links, nil
}
