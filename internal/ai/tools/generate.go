package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
)

const (
	modelsAPIURL = "https://ir-api.myqa.cc/v1/openai/images/models"
	generateURL  = "https://ir-api.myqa.cc/v1/openai/images/generations"
)

func (t Tools) Generate_image(prompt, model, apiKey string) (string, string, string, error) {
	baseError := "Image not generated"
	var err error
	if model == "" {
		model, err = t.getRandomFreeModel()
		if err != nil {
			return baseError, "", "", err
		}
	}
	url := generateURL

	if model == "" {
		model = "test/test"
	}

	requestBody := map[string]any{
		"prompt":          prompt,
		"model":           model,
		"quality":         "auto",
		"response_format": "b64_json",
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return baseError, "", "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return baseError, "", "", err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return baseError, "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return "limit is reached", "", "", errors.New("limit is reached")
	}

	var response ImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return baseError, "", "", err
	}

	if len(response.Data) > 0 {
		return "Image generated", response.Data[0].B64JSON, model, nil
	}

	return baseError, "", "", errors.New("image not generated")
}

type ModelInfo struct {
	Providers []struct {
		Pricing struct {
			Type  string  `json:"type"`
			Value float64 `json:"value"`
		} `json:"pricing"`
	} `json:"providers"`
}

type ImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL           string `json:"url"`
		B64JSON       string `json:"b64_json"`
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"data"`
	Latency int `json:"latency"`
	Cost    int `json:"cost"`
}

func (t Tools) getFreeModels() ([]string, error) {
	resp, err := t.httpClient.Get(modelsAPIURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var models map[string]ModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, err
	}

	var freeModels []string
	for name, model := range models {
		if len(model.Providers) > 0 &&
			model.Providers[0].Pricing.Type == "fixed" &&
			model.Providers[0].Pricing.Value == 0 &&
			name != "test/test" {
			freeModels = append(freeModels, name)
		}
	}

	return freeModels, nil
}

func (t Tools) getRandomFreeModel() (string, error) {
	freeModels, err := t.getFreeModels()
	if err != nil {
		return "", err
	}

	if len(freeModels) == 0 {
		fmt.Println("0 free models from image router")
		return "test/test", nil
	}

	return freeModels[rand.Intn(len(freeModels))], nil
}
