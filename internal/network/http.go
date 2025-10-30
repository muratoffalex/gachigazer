package network

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

const LogProxyNotConfigured = "Proxy not configured, using direct connection"

type HTTPClientConfig struct {
	ProxyURL              string
	Timeout               time.Duration
	DisableKeepAlives     bool
	MaxIdleConns          int
	IdleConnTimeout       time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration
	ForceAttemptHTTP2     bool
}

func NewDefaultHTTPClientConfig(proxy string) HTTPClientConfig {
	return HTTPClientConfig{
		ProxyURL:              proxy,
		Timeout:               3 * time.Minute,
		MaxIdleConns:          100,
		DisableKeepAlives:     false,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

func SetupHTTPClient(cfg HTTPClientConfig, logger logger.Logger) *http.Client {
	transport := &http.Transport{
		ForceAttemptHTTP2:     cfg.ForceAttemptHTTP2,
		MaxIdleConns:          cfg.MaxIdleConns,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		DisableKeepAlives:     cfg.DisableKeepAlives,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ExpectContinueTimeout: cfg.ExpectContinueTimeout,
	}

	if cfg.ProxyURL != "" {
		if err := configureProxy(transport, cfg.ProxyURL, logger); err != nil {
			logger.WithError(err).Fatal("failed to configure proxy")
		}
	} else {
		logger.Info(LogProxyNotConfigured)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}
}

func configureProxy(transport *http.Transport, proxyURL string, logger logger.Logger) error {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("failed to parse proxy URL: %w", err)
	}

	switch parsedURL.Scheme {
	case "socks5":
		dialContext, err := createSOCKS5ProxyDialer(parsedURL, logger)
		if err != nil {
			return fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}
		transport.DialContext = dialContext
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedURL)
		transport.DialContext = createSimpleDialer().DialContext
		logger.Info("Proxy configured: " + parsedURL.Redacted())
	default:
		return fmt.Errorf("unsupported proxy scheme: %s", parsedURL.Scheme)
	}

	return nil
}

func createSimpleDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
}

func createSOCKS5ProxyDialer(proxyURL *url.URL, logger logger.Logger) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	dialer := createSimpleDialer()

	proxyDialer, err := proxy.FromURL(proxyURL, dialer)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy dialer: %w", err)
	}
	logger.Info("Proxy configured: " + proxyURL.Redacted())
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return proxyDialer.Dial(network, addr)
	}, nil
}
