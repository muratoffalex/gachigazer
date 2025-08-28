package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/muratoffalex/gachigazer/internal/fetch"
)

type CurrencyService struct {
	httpClient  *http.Client
	lastUpdated time.Time
	currentRate float64
	rateMutex   sync.Mutex
}

var (
	currencyServiceInstance *CurrencyService
	currencyServiceOnce     sync.Once
)

func GetCurrencyService() *CurrencyService {
	currencyServiceOnce.Do(func() {
		currencyServiceInstance = &CurrencyService{
			httpClient: &http.Client{
				Timeout: 5 * time.Second,
			},
		}
	})
	return currencyServiceInstance
}

type currencyAPIResponse struct {
	Date string             `json:"date"`
	USD  map[string]float64 `json:"usd"`
}

func (s *CurrencyService) GetUSDRate(ctx context.Context, currencyCode string) (float64, error) {
	s.rateMutex.Lock()
	defer s.rateMutex.Unlock()

	if time.Since(s.lastUpdated) < 24*time.Hour {
		return s.currentRate, nil
	}

	currencyCode = strings.ToLower(currencyCode)
	urls := []string{
		"https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@latest/v1/currencies/usd.min.json",
		"https://latest.currency-api.pages.dev/v1/currencies/usd.min.json",
	}

	var lastErr error
	for _, url := range urls {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		req.Header.Set("User-Agent", fetch.RandomUserAgent())

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to get currency rates from %s: %w", url, err)
			continue
		}

		var data currencyAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("failed to decode currency response from %s: %w", url, err)
			continue
		}

		resp.Body.Close()

		rate, ok := data.USD[currencyCode]
		if !ok {
			lastErr = fmt.Errorf("%s rate not found in response from %s", currencyCode, url)
			continue
		}

		s.currentRate = rate
		s.lastUpdated = time.Now()
		return s.currentRate, nil
	}

	return s.currentRate, fmt.Errorf("all API endpoints failed, last error: %w", lastErr)
}
