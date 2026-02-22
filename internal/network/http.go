package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"golang.org/x/net/proxy"
)

const LogProxyNotConfigured = "Proxy not configured, using direct connection"

type HTTPClientConfig struct {
	ProxyURL              string
	NoProxy               []string
	Timeout               time.Duration
	DisableKeepAlives     bool
	MaxIdleConns          int
	IdleConnTimeout       time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration
	ForceAttemptHTTP2     bool
	DisableCompression    bool
}

func NewDefaultHTTPClientConfig(cfg config.HTTPConfig) HTTPClientConfig {
	return HTTPClientConfig{
		ProxyURL:              cfg.GetProxy(),
		NoProxy:               cfg.GetNoProxy(),
		Timeout:               3 * time.Minute,
		MaxIdleConns:          100,
		DisableKeepAlives:     false,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		DisableCompression:    false,
	}
}

func NewStreamingHTTPClientConfig(cfg config.HTTPConfig) HTTPClientConfig {
	return HTTPClientConfig{
		ProxyURL:              cfg.GetProxy(),
		NoProxy:               cfg.GetNoProxy(),
		Timeout:               0,
		MaxIdleConns:          100,
		DisableKeepAlives:     true,
		IdleConnTimeout:       0,
		TLSHandshakeTimeout:   30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
	}
}

func NewHTTPClientConfigForFetcher(cfg config.HTTPConfig) HTTPClientConfig {
	conf := NewDefaultHTTPClientConfig(cfg)
	conf.Timeout = 30 * time.Second
	conf.MaxIdleConns = 10
	conf.IdleConnTimeout = 10 * time.Second
	conf.DisableKeepAlives = true
	return conf
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
		if err := configureProxy(transport, cfg.ProxyURL, cfg.NoProxy, logger); err != nil {
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

func configureProxy(transport *http.Transport, proxyURL string, noProxy []string, logger logger.Logger) error {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("failed to parse proxy URL: %w", err)
	}

	switch parsedURL.Scheme {
	case "socks5":
		dialContext, err := createSOCKS5ProxyDialer(parsedURL, noProxy, logger)
		if err != nil {
			return fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}
		transport.DialContext = dialContext
	case "http", "https":
		transport.Proxy = createProxyFunc(proxyURL, noProxy)
		// transport.DialContext = createSimpleDialer().DialContext
		logger.Info(fmt.Sprintf("Proxy configured: %s, no_proxy: %v", parsedURL.Redacted(), noProxy))
	default:
		return fmt.Errorf("unsupported proxy scheme: %s", parsedURL.Scheme)
	}

	return nil
}

func createProxyFunc(proxyURL string, noProxy []string) func(*http.Request) (*url.URL, error) {
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		return nil
	}

	return func(req *http.Request) (*url.URL, error) {
		host := req.URL.Hostname()
		for _, exclusion := range noProxy {
			if matchHost(host, exclusion) {
				return nil, nil
			}
		}
		return proxy, nil
	}
}

func matchHost(host, pattern string) bool {
	if strings.Contains(pattern, "*") {
		pattern = strings.ReplaceAll(pattern, "*", ".*")
		matched, _ := regexp.MatchString("^"+pattern+"$", host)
		return matched
	}
	return host == pattern
}

func createSimpleDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
}

func createSOCKS5ProxyDialer(proxyURL *url.URL, noProxy []string, logger logger.Logger) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	directDialer := createSimpleDialer()

	proxyDialer, err := proxy.FromURL(proxyURL, directDialer)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy dialer: %w", err)
	}
	logger.Info(fmt.Sprintf("Proxy configured: %s, no_proxy: %v", proxyURL.Redacted(), noProxy))
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		for _, exclusion := range noProxy {
			if matchHost(host, exclusion) {
				return directDialer.DialContext(ctx, network, addr)
			}
		}
		return proxyDialer.Dial(network, addr)
	}, nil
}
