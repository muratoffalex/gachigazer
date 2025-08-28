package tools

import (
	"fmt"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/service"
)

func (t Tools) Search_images(
	query string,
	maxResults int,
	timeLimit string,
) (string, []string, error) {
	ddg := service.NewDuckDuckGoSearch(t.httpClient, 0)
	images, err := ddg.Images(query, "", "off", timeLimit, nil, nil, nil, nil, nil, &maxResults)
	if err != nil {
		return fmt.Sprintf("Error when search images: %v", err), nil, err
	}

	if len(images) == 0 {
		return "Images for query not found", nil, nil
	}

	imagesList := []string{}
	imagesTextBuilder := []string{}
	for i, image := range images {
		imagesList = append(imagesList, image.Image)
		imagesTextBuilder = append(imagesTextBuilder, fmt.Sprintf("#%d | Title: %s | Image URL:%s", i+1, image.Title, image.URL))
	}

	imagesText := strings.Join(imagesTextBuilder, "\n")

	return fmt.Sprintf("Images found count: %d\n%s", len(images), imagesText), imagesList, nil
}
