package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"slices"
)

const (
	modelsAPIURL = "https://api.imagerouter.io/v1/models"
	generateURL  = "https://api.imagerouter.io/v1/openai/images/generations"
)

func (t Tools) Generate_image(prompt, model, apiKey string) (string, string, string, error) {
	var err error
	if model == "" {
		model, err = t.getRandomFreeModel()
		if err != nil {
			return "", "", "", t.formatError(err.Error())
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

	t.logger.WithField("request", requestBody).Debug("Image generator request")

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", "", t.formatError(err.Error())
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", "", "", t.formatError(err.Error())
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", "", "", t.formatError(err.Error())
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errorMessage := t.extractErrorMessage(bodyBytes, resp.StatusCode, resp.Status)
		return "", "", "", t.formatError(errorMessage)
	}

	var response ImageResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return "", "", "", t.formatError(err.Error())
	}

	if len(response.Data) > 0 {
		return "Image generated and sent in chat", response.Data[0].B64JSON, model, nil
	}

	return "", "", "", t.formatError("")
}

type ModelInfo struct {
	Providers []struct {
		Pricing struct {
			Type  string  `json:"type"`
			Value float64 `json:"value"`
		} `json:"pricing"`
	} `json:"providers"`
	Output []string `json:"output"`
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
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models API returned status %d: %s", resp.StatusCode, resp.Status)
	}

	var models map[string]ModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, err
	}

	var freeModels []string
	for name, model := range models {
		if len(model.Providers) > 0 &&
			model.Providers[0].Pricing.Type == "fixed" &&
			model.Providers[0].Pricing.Value == 0 &&
			name != "test/test" &&
			slices.Contains(model.Output, "image") {
			freeModels = append(freeModels, name)
		}
	}

	return freeModels, nil
}

func (t Tools) getRandomFreeModel() (string, error) {
	freeModels, err := t.getFreeModels()
	if err != nil {
		return "", fmt.Errorf("failed to get free models: %w", err)
	}

	if len(freeModels) == 0 {
		fmt.Println("0 free models from image router")
		return "test/test", nil
	}

	return freeModels[rand.Intn(len(freeModels))], nil
}

func (t Tools) formatError(message string) error {
	return errors.New(message)
}

func (t Tools) extractErrorMessage(bodyBytes []byte, statusCode int, statusText string) string {
	var errorResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &errorResp); err == nil && errorResp.Error.Message != "" {
		return fmt.Sprintf("status %d: %s", statusCode, errorResp.Error.Message)
	}

	var simpleError struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(bodyBytes, &simpleError); err == nil && simpleError.Message != "" {
		return fmt.Sprintf("status %d: %s", statusCode, simpleError.Message)
	}

	if len(bodyBytes) == 0 {
		return fmt.Sprintf("status %d: %s", statusCode, statusText)
	}

	errorBody := string(bodyBytes)
	if len(errorBody) > 200 {
		errorBody = errorBody[:200] + "..."
	}
	return fmt.Sprintf("status %d: %s", statusCode, errorBody)
}
