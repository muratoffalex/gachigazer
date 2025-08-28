package markdown

import (
	"strings"
	"testing"
)

func BenchmarkTelegramifyMarkdown(b *testing.B) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "small",
			data: "**bold** _italic_ `code`",
		},
		{
			name: "medium",
			data: strings.Repeat("**bold** _italic_ `code` ", 100),
		},
		{
			name: "large",
			data: strings.Repeat("**bold** _italic_ `code` ", 1000),
		},
	}

	tm := &MarkdownProcessor{}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for b.Loop() {
				_, _ = tm.Convert(tt.data)
			}
		})
	}
}
