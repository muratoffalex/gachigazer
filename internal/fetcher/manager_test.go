package fetcher

import (
	"errors"
	"testing"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	log := logger.NewTestLogger()
	manager := NewManager(log)

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.fetchers)
	assert.Empty(t, manager.fetchers)
	assert.Nil(t, manager.defaultFetcher)
	assert.Equal(t, log, manager.logger)
}

func TestManager_RegisterFetcher(t *testing.T) {
	log := logger.NewTestLogger()
	manager := NewManager(log)

	t.Run("register new fetcher", func(t *testing.T) {
		mockFetcher := NewMockFetcher(t)
		mockFetcher.EXPECT().GetName().Return("test-fetcher")

		manager.RegisterFetcher(mockFetcher)

		assert.True(t, manager.ContainsFetcher(mockFetcher))
		assert.Len(t, manager.fetchers, 1)
		mockFetcher.AssertExpectations(t)
	})

	t.Run("duplicate registration ignored", func(t *testing.T) {
		mockFetcher := NewMockFetcher(t)
		mockFetcher.EXPECT().GetName().Return("test-fetcher-2").Times(4)

		manager.RegisterFetcher(mockFetcher)
		manager.RegisterFetcher(mockFetcher)

		assert.True(t, manager.ContainsFetcher(mockFetcher))
		assert.Len(t, manager.fetchers, 2)
		mockFetcher.AssertExpectations(t)
	})
}

func TestManager_ContainsFetcher(t *testing.T) {
	log := logger.NewTestLogger()
	manager := NewManager(log)

	t.Run("fetcher not found before registration", func(t *testing.T) {
		mockFetcher := NewMockFetcher(t)
		mockFetcher.EXPECT().GetName().Return("test-fetcher")

		assert.False(t, manager.ContainsFetcher(mockFetcher))
		mockFetcher.AssertExpectations(t)
	})

	t.Run("fetcher found after registration", func(t *testing.T) {
		mockFetcher := NewMockFetcher(t)
		mockFetcher.EXPECT().GetName().Return("test-fetcher")

		manager.RegisterFetcher(mockFetcher)
		assert.True(t, manager.ContainsFetcher(mockFetcher))
		mockFetcher.AssertExpectations(t)
	})
}

func TestManager_SetDefaultFetcher(t *testing.T) {
	log := logger.NewTestLogger()
	manager := NewManager(log)

	t.Run("set default fetcher", func(t *testing.T) {
		mockFetcher := NewMockFetcher(t)
		manager.SetDefaultFetcher(mockFetcher)
		assert.Equal(t, mockFetcher, manager.defaultFetcher)
	})
}

func TestManager_Fetch(t *testing.T) {
	log := logger.NewTestLogger()

	t.Run("success with matching fetcher", func(t *testing.T) {
		manager := NewManager(log)
		mockFetcher := NewMockFetcher(t)
		mockFetcher.EXPECT().GetName().Return("test-fetcher")
		mockFetcher.EXPECT().CanHandle("https://example.com").Return(true)

		request := MustNewRequestPayload("https://example.com", nil, nil)
		expectedResponse := Response{
			Content: []Content{{Type: ContentTypeText, Text: "test content"}},
			IsError: false,
		}

		mockFetcher.EXPECT().Handle(mock.Anything).Return(expectedResponse, nil)
		manager.RegisterFetcher(mockFetcher)

		response, err := manager.Fetch(request)

		require.NoError(t, err)
		assert.Equal(t, expectedResponse, response)
		assert.True(t, log.HasEntry("debug", "Matched fetcher"))
		mockFetcher.AssertExpectations(t)
	})

	t.Run("skip non-matching fetcher", func(t *testing.T) {
		manager := NewManager(log)
		mockFetcher := NewMockFetcher(t)
		mockFetcher.EXPECT().GetName().Return("test-fetcher")
		mockFetcher.EXPECT().CanHandle("https://example.com").Return(false)

		request := MustNewRequestPayload("https://example.com", nil, nil)
		manager.RegisterFetcher(mockFetcher)

		response, err := manager.Fetch(request)

		require.Error(t, err)
		assert.Equal(t, Response{}, response)
		assert.Contains(t, err.Error(), "no fetcher found for URL")
		mockFetcher.AssertExpectations(t)
	})

	t.Run("use default fetcher", func(t *testing.T) {
		manager := NewManager(log)
		mockFetcher := NewMockFetcher(t)
		mockFetcher.EXPECT().GetName().Return("test-fetcher")
		mockFetcher.EXPECT().CanHandle("https://example.com").Return(false)

		mockDefaultFetcher := NewMockFetcher(t)
		request := MustNewRequestPayload("https://example.com", nil, nil)
		expectedResponse := Response{
			Content: []Content{{Type: ContentTypeText, Text: "default content"}},
			IsError: false,
		}

		mockDefaultFetcher.EXPECT().Handle(mock.Anything).Return(expectedResponse, nil)
		manager.RegisterFetcher(mockFetcher)
		manager.SetDefaultFetcher(mockDefaultFetcher)

		response, err := manager.Fetch(request)

		require.NoError(t, err)
		assert.Equal(t, expectedResponse, response)
		assert.True(t, log.HasEntry("info", "Using default fetcher"))
		mockFetcher.AssertExpectations(t)
		mockDefaultFetcher.AssertExpectations(t)
	})

	t.Run("no fetcher found", func(t *testing.T) {
		manager := NewManager(log)
		request := MustNewRequestPayload("https://example.com", nil, nil)

		response, err := manager.Fetch(request)

		require.Error(t, err)
		assert.Equal(t, Response{}, response)
		assert.Contains(t, err.Error(), "no fetcher found for URL")
	})

	t.Run("handle ErrNotHandle", func(t *testing.T) {
		manager := NewManager(log)
		mockFetcher1 := NewMockFetcher(t)
		mockFetcher1.EXPECT().GetName().Return("fetcher-1")
		mockFetcher1.EXPECT().CanHandle("https://example.com").Return(true)
		mockFetcher1.EXPECT().Handle(mock.Anything).Return(Response{}, ErrNotHandle)

		mockFetcher2 := NewMockFetcher(t)
		mockFetcher2.EXPECT().GetName().Return("fetcher-2")
		mockFetcher2.EXPECT().CanHandle("https://example.com").Return(true)

		request := MustNewRequestPayload("https://example.com", nil, nil)
		expectedResponse := Response{
			Content: []Content{{Type: ContentTypeText, Text: "success content"}},
			IsError: false,
		}

		mockFetcher2.EXPECT().Handle(mock.Anything).Return(expectedResponse, nil)
		manager.RegisterFetcher(mockFetcher1)
		manager.RegisterFetcher(mockFetcher2)

		response, err := manager.Fetch(request)

		require.NoError(t, err)
		assert.Equal(t, expectedResponse, response)
		assert.True(t, log.HasEntry("warn", "fetcher not handling url, skip"))
		mockFetcher1.AssertExpectations(t)
		mockFetcher2.AssertExpectations(t)
	})

	t.Run("propagate other errors", func(t *testing.T) {
		manager := NewManager(log)
		mockFetcher := NewMockFetcher(t)
		mockFetcher.EXPECT().GetName().Return("test-fetcher")
		mockFetcher.EXPECT().CanHandle("https://example.com").Return(true)

		request := MustNewRequestPayload("https://example.com", nil, nil)
		expectedError := errors.New("network error")
		mockFetcher.EXPECT().Handle(mock.Anything).Return(Response{}, expectedError)
		manager.RegisterFetcher(mockFetcher)

		response, err := manager.Fetch(request)

		require.Error(t, err)
		assert.Equal(t, Response{}, response)
		assert.Equal(t, expectedError, err)
		mockFetcher.AssertExpectations(t)
	})

	t.Run("fetcher order matters", func(t *testing.T) {
		manager := NewManager(log)
		mockFetcher1 := NewMockFetcher(t)
		mockFetcher1.EXPECT().GetName().Return("fetcher-1")
		mockFetcher1.EXPECT().CanHandle("https://example.com").Return(true)

		mockFetcher2 := NewMockFetcher(t)
		mockFetcher2.EXPECT().GetName().Return("fetcher-2")

		request := MustNewRequestPayload("https://example.com", nil, nil)
		expectedResponse := Response{
			Content: []Content{{Type: ContentTypeText, Text: "from fetcher-1"}},
			IsError: false,
		}

		mockFetcher1.EXPECT().Handle(mock.Anything).Return(expectedResponse, nil)
		manager.RegisterFetcher(mockFetcher1)
		manager.RegisterFetcher(mockFetcher2)

		response, err := manager.Fetch(request)

		require.NoError(t, err)
		assert.Equal(t, expectedResponse, response)
		mockFetcher1.AssertExpectations(t)
		mockFetcher2.AssertNotCalled(t, "Handle", mock.Anything)
	})

	t.Run("with headers and options", func(t *testing.T) {
		manager := NewManager(log)
		mockFetcher := NewMockFetcher(t)
		mockFetcher.EXPECT().GetName().Return("test-fetcher")
		mockFetcher.EXPECT().CanHandle("https://example.com").Return(true)

		headers := map[string]string{
			"Authorization": "Bearer token123",
			"User-Agent":    "TestAgent",
		}
		options := map[string]any{
			"timeout": 30,
			"retries": 3,
		}
		request := MustNewRequestPayload("https://example.com", headers, options)
		expectedResponse := Response{
			Content: []Content{{Type: ContentTypeText, Text: "content with headers"}},
			IsError: false,
		}

		mockFetcher.EXPECT().Handle(mock.Anything).Return(expectedResponse, nil)
		manager.RegisterFetcher(mockFetcher)

		response, err := manager.Fetch(request)

		require.NoError(t, err)
		assert.Equal(t, expectedResponse, response)
		mockFetcher.AssertExpectations(t)
	})

	t.Run("multiple fetchers for same URL", func(t *testing.T) {
		manager := NewManager(log)
		mockFetcher1 := NewMockFetcher(t)
		mockFetcher1.EXPECT().GetName().Return("fetcher-1")
		mockFetcher1.EXPECT().CanHandle("https://example.com").Return(false)

		mockFetcher2 := NewMockFetcher(t)
		mockFetcher2.EXPECT().GetName().Return("fetcher-2")
		mockFetcher2.EXPECT().CanHandle("https://example.com").Return(true)

		mockFetcher3 := NewMockFetcher(t)
		mockFetcher3.EXPECT().GetName().Return("fetcher-3")

		request := MustNewRequestPayload("https://example.com", nil, nil)
		expectedResponse := Response{
			Content: []Content{{Type: ContentTypeText, Text: "from fetcher-2"}},
			IsError: false,
		}

		mockFetcher2.EXPECT().Handle(mock.Anything).Return(expectedResponse, nil)
		manager.RegisterFetcher(mockFetcher1)
		manager.RegisterFetcher(mockFetcher2)
		manager.RegisterFetcher(mockFetcher3)

		response, err := manager.Fetch(request)

		require.NoError(t, err)
		assert.Equal(t, expectedResponse, response)
		mockFetcher1.AssertExpectations(t)
		mockFetcher2.AssertExpectations(t)
		mockFetcher3.AssertNotCalled(t, "CanHandle", mock.Anything)
		mockFetcher3.AssertNotCalled(t, "Handle", mock.Anything)
	})
}

func TestManager_Integration(t *testing.T) {
	log := logger.NewTestLogger()

	t.Run("full scenario with multiple fetchers", func(t *testing.T) {
		manager := NewManager(log)

		mockFetcher1 := NewMockFetcher(t)
		mockFetcher1.EXPECT().GetName().Return("fetcher-1")
		mockFetcher1.EXPECT().CanHandle("https://site1.com").Return(false)
		mockFetcher1.EXPECT().CanHandle("https://site2.com").Return(true)
		mockFetcher1.EXPECT().CanHandle("https://site3.com").Return(false)

		mockFetcher2 := NewMockFetcher(t)
		mockFetcher2.EXPECT().GetName().Return("fetcher-2")
		mockFetcher2.EXPECT().CanHandle("https://site1.com").Return(true)
		mockFetcher2.EXPECT().CanHandle("https://site3.com").Return(false)

		mockDefaultFetcher := NewMockFetcher(t)
		mockDefaultFetcher.EXPECT().Handle(mock.Anything).Return(Response{
			Content: []Content{{Type: ContentTypeText, Text: "default response"}},
		}, nil)

		manager.RegisterFetcher(mockFetcher1)
		manager.RegisterFetcher(mockFetcher2)
		manager.SetDefaultFetcher(mockDefaultFetcher)

		mockFetcher2.EXPECT().Handle(mock.Anything).Return(Response{
			Content: []Content{{Type: ContentTypeText, Text: "from fetcher-2"}},
		}, nil)

		request1 := MustNewRequestPayload("https://site1.com", nil, nil)
		response1, err1 := manager.Fetch(request1)
		require.NoError(t, err1)
		assert.Equal(t, "from fetcher-2", response1.GetText())

		mockFetcher1.EXPECT().Handle(mock.Anything).Return(Response{
			Content: []Content{{Type: ContentTypeText, Text: "from fetcher-1"}},
		}, nil)

		request2 := MustNewRequestPayload("https://site2.com", nil, nil)
		response2, err2 := manager.Fetch(request2)
		require.NoError(t, err2)
		assert.Equal(t, "from fetcher-1", response2.GetText())

		request3 := MustNewRequestPayload("https://site3.com", nil, nil)
		response3, err3 := manager.Fetch(request3)
		require.NoError(t, err3)
		assert.Equal(t, "default response", response3.GetText())

		mockFetcher1.AssertExpectations(t)
		mockFetcher2.AssertExpectations(t)
		mockDefaultFetcher.AssertExpectations(t)
	})
}
