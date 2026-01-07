package fetcher

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/service/youtube"
)

var ErrCreateYoutubeRequest = errors.New("failed to create youtube request")

const (
	OptYtFetchFlags  = "fetch_flags"
	OptYtMaxComments = "max_comments"
)

type YoutubeData struct {
	LikeCount    *float64
	CommentCount *float64
	ViewCount    *float64
	UploadedAt   *time.Time
	Title        string
	Transcript   string
	Comments     string
}

type YoutubeRequest struct {
	RequestPayload
	// fetchFlags determines what to load (transcription, comments).
	fetchFlags youtube.FetchFlag
	// maxComments limits the number of comments to be loaded.
	maxComments int
}

func NewYoutubeRequest(
	url string,
	headers map[string]string,
	fetchFlags youtube.FetchFlag, maxComments int,
) (YoutubeRequest, error) {
	payload, err := NewRequestPayload(url, headers, nil)
	if err != nil {
		return YoutubeRequest{}, errors.Join(ErrCreateYoutubeRequest, err)
	}
	return YoutubeRequest{
		RequestPayload: payload,
		fetchFlags:     fetchFlags,
		maxComments:    maxComments,
	}, nil
}

func (r YoutubeRequest) Options() map[string]any {
	return map[string]any{
		OptYtFetchFlags:  r.fetchFlags,
		OptYtMaxComments: r.maxComments,
	}
}

type youtubeService interface {
	FetchYoutubeData(url string, flags youtube.FetchFlag, maxComments int) (*youtube.YoutubeData, error)
}

type YoutubeFetcher struct {
	BaseFetcher
	service youtubeService
}

func NewYoutubeFetcher(l logger.Logger, httpClient *http.Client, ytService youtubeService) YoutubeFetcher {
	return YoutubeFetcher{
		BaseFetcher: NewBaseFetcher(FetcherNameYoutube, "youtube\\.com|youtu\\.be", httpClient, l),
		service:     ytService,
	}
}

func (f YoutubeFetcher) Handle(request Request) (Response, error) {
	fetchFlags := youtube.FetchTranscript
	maxComments := 0

	// try to get parameters from Options.
	if opts := request.Options(); opts != nil {
		if flags, ok := opts[OptYtFetchFlags].(youtube.FetchFlag); ok {
			fetchFlags = flags
		}
		if mc, ok := opts[OptYtMaxComments].(int); ok {
			maxComments = mc
		}
	}

	transcript, err := f.service.FetchYoutubeData(request.URL(), fetchFlags, maxComments)
	if err != nil {
		return f.errorResponse(err)
	}
	title := transcript.Title
	normalizedTranscript := f.cleanText(transcript.Transcript)
	metadataItems := []string{}
	if val := transcript.UploadedAt; val != nil {
		metadataItems = append(metadataItems, "Uploaded at: "+val.Format(time.DateTime))
	}
	if val := transcript.ViewCount; val != nil {
		metadataItems = append(metadataItems, "Views: "+FormatCount(*val))
	}
	if val := transcript.LikeCount; val != nil {
		metadataItems = append(metadataItems, "Likes: "+FormatCount(*val))
	}
	if val := transcript.CommentCount; val != nil {
		metadataItems = append(metadataItems, "Comments count: "+FormatCount(*val))
	}
	metadataString := strings.Join(metadataItems, " | ")
	responseString := "Title: " + title + "\n"
	responseString += metadataString
	responseString += "\n" + strings.TrimSpace(normalizedTranscript)
	if transcript.Comments != "" {
		responseString += "\nPopular comments:\n" + transcript.Comments
	}

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: responseString}},
		IsError: false,
	}, nil
}
