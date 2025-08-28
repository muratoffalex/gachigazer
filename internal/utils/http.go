package utils

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"golang.org/x/net/proxy"
)

func SetupHTTPClient(proxyURL string, logger logger.Logger) *http.Client {
	dialContext, err := CreateProxyDialer(proxyURL, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to create dialer")
	}

	return &http.Client{
		Transport: &http.Transport{
			DialContext:           dialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: 3 * time.Minute,
	}
}

func CreateProxyDialer(proxyURL string, logger logger.Logger) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	if proxyURL != "" {
		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
		}

		if parsedURL.Scheme == "socks5" {
			proxyDialer, err := proxy.FromURL(parsedURL, dialer)
			if err != nil {
				return nil, fmt.Errorf("failed to create proxy dialer: %w", err)
			}
			logger.Info("Proxy configured: " + parsedURL.Redacted())
			return func(ctx context.Context, network, addr string) (net.Conn, error) {
				return proxyDialer.Dial(network, addr)
			}, nil
		}
	}

	logger.Info("Proxy not configured, using direct connection")
	return dialer.DialContext, nil
}
