package youtube

import "net/http"

type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// Ensure http.Client implements HTTPClient
var _ HTTPClient = (*http.Client)(nil)
