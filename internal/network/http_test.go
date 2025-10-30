package network

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/muratoffalex/gachigazer/internal/logger"
)

func TestCreateProxyDialer(t *testing.T) {
	t.Run("with socks5 proxy", func(t *testing.T) {
		testLogger := logger.NewTestLogger()
		proxyURL, _ := url.Parse("socks5://127.0.0.1:1080")

		dialFunc, err := createSOCKS5ProxyDialer(proxyURL, testLogger)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if dialFunc == nil {
			t.Fatal("expected dialFunc to be non-nil")
		}
		if !testLogger.HasEntry("info", "Proxy configured: socks5://127.0.0.1:1080") {
			t.Error("expected log entry about proxy configuration")
		}
		if testLogger.HasEntry("info", LogProxyNotConfigured) {
			t.Error("should not log direct connection when proxy is configured")
		}
	})

	t.Run("with socks5 proxy and auth", func(t *testing.T) {
		testLogger := logger.NewTestLogger()
		proxyURL, _ := url.Parse("socks5://user:pass@127.0.0.1:1080")

		dialFunc, err := createSOCKS5ProxyDialer(proxyURL, testLogger)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if dialFunc == nil {
			t.Fatal("expected dialFunc to be non-nil")
		}
		if !testLogger.HasEntry("info", "Proxy configured: socks5://user:xxxxx@127.0.0.1:1080") {
			t.Error("expected log entry with redacted password")
		}
		if testLogger.HasEntry("info", LogProxyNotConfigured) {
			t.Error("should not log direct connection when proxy is configured")
		}
	})
}

func TestSetupHTTPClient(t *testing.T) {
	t.Run("without proxy", func(t *testing.T) {
		testLogger := logger.NewTestLogger()

		client := SetupHTTPClient(NewDefaultHTTPClientConfig(""), testLogger)

		if client == nil {
			t.Fatal("expected client to be non-nil")
		}
		if client.Transport == nil {
			t.Fatal("expected transport to be non-nil")
		}
		if !testLogger.HasEntry("info", LogProxyNotConfigured) {
			t.Error("expected log entry about direct connection")
		}
	})

	t.Run("with socks5 proxy", func(t *testing.T) {
		testLogger := logger.NewTestLogger()
		proxyURL := "socks5://127.0.0.1:1080"

		client := SetupHTTPClient(NewDefaultHTTPClientConfig(proxyURL), testLogger)

		if client == nil {
			t.Fatal("expected client to be non-nil")
		}
		transport, ok := client.Transport.(*http.Transport)
		if !ok {
			t.Fatal("expected transport to be *http.Transport")
		}
		if transport.DialContext == nil {
			t.Error("expected DialContext to be set for SOCKS5 proxy")
		}
		if !testLogger.HasEntry("info", "Proxy configured: socks5://127.0.0.1:1080") {
			t.Error("expected log entry about proxy configuration")
		}
	})

	t.Run("with http proxy", func(t *testing.T) {
		testLogger := logger.NewTestLogger()
		proxyURL := "http://127.0.0.1:8080"

		client := SetupHTTPClient(NewDefaultHTTPClientConfig(proxyURL), testLogger)

		if client == nil {
			t.Fatal("expected client to be non-nil")
		}
		transport, ok := client.Transport.(*http.Transport)
		if !ok {
			t.Fatal("expected transport to be *http.Transport")
		}
		if transport.Proxy == nil {
			t.Error("expected Proxy to be set for HTTP proxy")
		}
		if !testLogger.HasEntry("info", "Proxy configured: http://127.0.0.1:8080") {
			t.Error("expected log entry about proxy configuration")
		}
	})

	t.Run("with https proxy", func(t *testing.T) {
		testLogger := logger.NewTestLogger()
		proxyURL := "https://127.0.0.1:8080"

		client := SetupHTTPClient(NewDefaultHTTPClientConfig(proxyURL), testLogger)

		if client == nil {
			t.Fatal("expected client to be non-nil")
		}
		transport, ok := client.Transport.(*http.Transport)
		if !ok {
			t.Fatal("expected transport to be *http.Transport")
		}
		if transport.Proxy == nil {
			t.Error("expected Proxy to be set for HTTPS proxy")
		}
		if !testLogger.HasEntry("info", "Proxy configured: https://127.0.0.1:8080") {
			t.Error("expected log entry about proxy configuration")
		}
	})

	t.Run("with http proxy and auth", func(t *testing.T) {
		testLogger := logger.NewTestLogger()
		proxyURL := "http://user:pass@127.0.0.1:8080"

		client := SetupHTTPClient(NewDefaultHTTPClientConfig(proxyURL), testLogger)

		if client == nil {
			t.Fatal("expected client to be non-nil")
		}
		transport, ok := client.Transport.(*http.Transport)
		if !ok {
			t.Fatal("expected transport to be *http.Transport")
		}
		if transport.Proxy == nil {
			t.Error("expected Proxy to be set for HTTP proxy")
		}
		if !testLogger.HasEntry("info", "Proxy configured: http://user:xxxxx@127.0.0.1:8080") {
			t.Error("expected log entry with redacted password")
		}
	})
}
