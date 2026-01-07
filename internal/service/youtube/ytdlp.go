package youtube

import (
	"context"

	"github.com/lrstanley/go-ytdlp"
)

type FetchOptions struct {
	SkipDownload  bool
	PrintJSON     bool
	WriteComments bool
	Proxy         string
}

type YtdlpContentExtractor struct{}

func NewYTDLPFetcher() ContentExtractor {
	return &YtdlpContentExtractor{}
}

func (f *YtdlpContentExtractor) Extract(
	ctx context.Context,
	url string,
	options FetchOptions,
) (*ytdlp.Result, error) {
	dl := ytdlp.New()

	if options.SkipDownload {
		dl = dl.SkipDownload()
	}

	if options.PrintJSON {
		dl = dl.PrintJSON()
	}

	if options.WriteComments {
		dl = dl.WriteComments()
	}

	if options.Proxy != "" {
		dl = dl.Proxy(options.Proxy)
	}

	return dl.Run(ctx, url)
}
