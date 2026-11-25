package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/muratoffalex/gachigazer/internal/logger"
)

func TestAskStreamCompletion(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  string
	}{
		{name: "unexpected EOF"},
		{name: "finished", end: "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if _, err := fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"+tc.end); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()

			client := NewOpenAICompatibleClient("test", server.URL, "", "", "", logger.NewTestLogger(), false, nil, server.Client())
			stream, _, err := client.AskStream(context.Background(), CompletionRequest{Model: "test"}, nil)
			require.NoError(t, err)
			var streamErr error
			var content string
			for chunk := range stream {
				content += chunk.Content
				if chunk.Error != nil {
					streamErr = chunk.Error
				}
			}
			require.Equal(t, "hello", content)
			if tc.end == "" {
				require.ErrorIs(t, streamErr, ErrIncompleteStream)
			} else {
				require.NoError(t, streamErr)
			}
		})
	}
}
