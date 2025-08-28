package fetch

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lrstanley/go-ytdlp"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

func (f *Fetcher) parseYoutube(url string) Response {
	transcript, err := f.FetchYoutubeData(url, FetchTranscript, 0)
	if err != nil {
		return Response{
			Content: []Content{{Type: ContentTypeText, Text: err.Error()}},
			IsError: true,
		}
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
	responseString += "\n\n" + strings.TrimSpace(normalizedTranscript)
	if transcript.Comments != "" {
		responseString += "\nPopular comments:\n" + transcript.Comments
	}

	return Response{
		Content: []Content{{Type: ContentTypeText, Text: responseString}},
		IsError: false,
	}
}

func (f *Fetcher) fetchYtSubtitles(url string) (string, error) {
	resp, err := f.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Conversion of SRT to plain text
	lines := strings.Split(string(body), "\n")
	var textLines []string
	for _, line := range lines {
		// Skip line numbers and timestamps
		if _, err := strconv.Atoi(line); err == nil {
			continue
		}
		if strings.Contains(line, "-->") {
			continue
		}
		if strings.TrimSpace(line) != "" {
			textLines = append(textLines, strings.TrimSpace(line))
		}
	}

	return strings.Join(textLines, " "), nil
}

const (
	FetchTranscript = 1 << iota
	FetchComments
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

func (f *Fetcher) FetchYoutubeData(url string, flags int, maxComments int) (*YoutubeData, error) {
	dl := ytdlp.New().
		SkipDownload().
		PrintJSON()

	if flags&FetchComments != 0 {
		dl.WriteComments()
	}

	if f.proxy != "" {
		dl.Proxy(f.proxy)
	}

	output, err := dl.Run(context.TODO(), url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch youtube data: %v", err)
	}

	info, err := output.GetExtractedInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to extract video info: %v", err)
	}

	result := &YoutubeData{}
	file := info[0]
	result.LikeCount = file.LikeCount
	result.CommentCount = file.CommentCount
	result.ViewCount = file.ViewCount
	if file.Title != nil {
		result.Title = *file.Title
	}
	if timestamp := file.Timestamp; timestamp != nil {
		val := time.Unix(int64(*timestamp), 0)
		result.UploadedAt = &val
	}

	if flags&FetchTranscript != 0 {
		if len(info) == 0 || info[0] == nil {
			return nil, fmt.Errorf("no video info available")
		}
		file := info[0]

		if file.Language == nil || file.AutomaticCaptions == nil {
			return nil, fmt.Errorf("no captions available for this video")
		}

		languageCaptions, exists := file.AutomaticCaptions[*file.Language]
		if !exists || len(languageCaptions) == 0 {
			return nil, fmt.Errorf("no captions available for language: %s", *file.Language)
		}

		f.logger.WithFields(logger.Fields{
			"language":           *file.Language,
			"available_captions": len(languageCaptions),
		}).Debug("Available captions")

		var subtitleURL string
		for _, caption := range languageCaptions {
			if caption.Name != nil && strings.Contains(strings.ToLower(caption.URL), "fmt=srt") && caption.URL != "" {
				subtitleURL = caption.URL
				break
			}
		}

		if subtitleURL == "" {
			return nil, fmt.Errorf("no subtitle URL found")
		}

		content, err := f.fetchYtSubtitles(subtitleURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch transcript: %v", err)
		}
		result.Transcript = content
	}

	if flags&FetchComments != 0 {
		var comments strings.Builder
		commentCount := 0
		if maxComments == 0 {
			// TODO: add to config
			maxComments = 50
		}

		validComments := make([]*ytdlp.ExtractedVideoComment, 0, len(info[0].Comments))
		for _, comment := range info[0].Comments {
			if comment.Text != nil && *comment.Text != "" {
				validComments = append(validComments, comment)
			}
		}

		if len(validComments) > 0 {
			if validComments[0].LikeCount != nil {
				sort.Slice(validComments, func(i, j int) bool {
					return *validComments[i].LikeCount > *validComments[j].LikeCount
				})
			}

			for _, comment := range validComments {
				if commentCount >= maxComments {
					break
				}

				var timestamp time.Time
				if comment.Timestamp != nil {
					timestamp = time.Unix(int64(*comment.Timestamp), 0)
				}

				comments.WriteString(fmt.Sprintf(
					"Author: %s | Likes: %s | Date: %s\nText: %s\n",
					*comment.Author,
					FormatCount(*comment.LikeCount),
					timestamp.Format(time.DateTime),
					*comment.Text,
				))
				commentCount++
			}
			result.Comments = comments.String()
		} else {
			result.Comments = "Not found"
		}
	}
	return result, nil
}

func FormatCount(count float64) string {
	switch {
	case count < 1000:
		return fmt.Sprintf("%.0f", count)
	case count < 10000:
		return fmt.Sprintf("%.1fK", count/1000)
	case count < 1000000:
		return fmt.Sprintf("%.0fK", count/1000)
	case count < 10000000:
		return fmt.Sprintf("%.1fM", count/1000000)
	default:
		return fmt.Sprintf("%.0fM", count/1000000)
	}
}
