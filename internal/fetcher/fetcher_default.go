package fetcher

import (
	"github.com/muratoffalex/gachigazer/internal/logger"
)

func defaultHandle(f BaseFetcher, request Request) (Response, error) {
	resp, body, err := f.fetch(request)
	if err != nil {
		return f.errorResponse(err)
	}

	if !f.isHTMLContent(resp, body) {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: body}},
		}, nil
	}

	doc, err := f.getGoqueryDoc(body)
	if err != nil {
		return f.errorResponse(err)
	}

	f.cleanDoc(doc)
	text := doc.Text()
	normalizedText := f.cleanText(text)

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: normalizedText}},
	}, nil
}

func NewDefaultFetcher(l logger.Logger, httpClient HTTPClient) FuncFetcher {
	return NewFuncFetcher(FetcherNameDefault, "", httpClient, l, defaultHandle)
}
