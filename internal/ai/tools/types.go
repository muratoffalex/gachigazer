package tools

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/ai"
	"github.com/muratoffalex/gachigazer/internal/fetcher"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/service/youtube"
)

const (
	ToolWeather             = "weather"
	ToolSearch              = "search"
	ToolFetchURL            = "fetch_url"
	ToolGenerateImage       = "generate_image"
	ToolSearchImages        = "search_images"
	ToolFetchTgPosts        = "fetch_tg_posts"
	ToolFetchTgPostComments = "fetch_tg_post_comments"
	ToolFetchYtComments     = "fetch_yt_comments"
)

func NewTools(
	httpClient *http.Client,
	fetcher *fetcher.Manager,
	ytService *youtube.Service,
	logger logger.Logger,
) *Tools {
	return &Tools{
		httpClient: httpClient,
		fetcher:    fetcher,
		logger:     logger,
		ytService:  ytService,
	}
}

type Tools struct {
	httpClient *http.Client
	fetcher    *fetcher.Manager
	logger     logger.Logger
	ytService  *youtube.Service
}

func ToolNames(allowedTools []string, excludedTools []string) []string {
	availableTools := AvailableTools(allowedTools, excludedTools)
	names := make([]string, 0, len(availableTools))
	for k := range availableTools {
		names = append(names, k)
	}
	return names
}

func AvailableToolsText(allowedTools []string, excludedTools []string) string {
	var sb strings.Builder
	tools := AvailableTools(allowedTools, excludedTools)

	for name, tool := range tools {
		sb.WriteString(fmt.Sprintf("• %s: %s\n", name, tool.Function.Description))

		if len(tool.Function.Parameters.Properties) > 0 {
			sb.WriteString("  Parameters:\n")
			for paramName, param := range tool.Function.Parameters.Properties {
				sb.WriteString(fmt.Sprintf("  - %s (%s): %s\n",
					paramName,
					param.Type,
					param.Description))
			}
		}
	}
	return sb.String()
}

func AvailableTools(allowedTools []string, excludedTools []string) map[string]ai.Tool {
	result := make(map[string]ai.Tool)

	if len(allowedTools) > 0 {
		// If there are allowedTools, use only them
		for _, name := range allowedTools {
			if tool, exists := AllTools[name]; exists {
				result[name] = tool
			}
		}
	} else if len(excludedTools) > 0 {
		// If there are only excludedTools, exclude them
		for name, tool := range AllTools {
			if !slices.Contains(excludedTools, name) {
				result[name] = tool
			}
		}
	} else {
		// If both lists are empty, return all instruments
		maps.Copy(result, AllTools)
	}

	return result
}

var AllTools = map[string]ai.Tool{
	ToolWeather: {
		Type: "function",
		Function: ai.ToolFunction{
			Name:        ToolWeather,
			Description: `Fetches comprehensive weather forecasts`,
			Parameters: ai.Parameters{
				Type: "object",
				Properties: map[string]ai.Property{
					"location": {Type: "string", Description: "City name in English (e.g., `London`, `New+York`)"},
					"days":     {Type: "integer", Description: "Number of forecast days (1-3). 1 - Today, 2 - Today and tomorrow, etc."},
				},
				Required: []string{"location", "days"},
			},
		},
	},
	ToolSearch: {
		Type: "function",
		Function: ai.ToolFunction{
			Name:        ToolSearch,
			Description: "Search with duckduckgo, use when need more relevant information.",
			Parameters: ai.Parameters{
				Type: "object",
				Properties: map[string]ai.Property{
					"query":       {Type: "string", Description: "Search query"},
					"max_results": {Type: "integer", Description: "Max search results. Min: 3, max: 10"},
					"time_limit":  {Type: "string", Enum: []string{"", "d", "w", "m", "y"}, Description: "Time range for search results: 'd' (last 24h), 'w' (last week), 'm' (last month), 'y' (last year). Leave empty for all time."},
				},
				Required: []string{"query", "max_results"},
			},
		},
	},
	ToolFetchURL: {
		Type: "function",
		Function: ai.ToolFunction{
			Name:        ToolFetchURL,
			Description: `Fetch full content from URL. Use when you need more info from URL (e.g. after search) or if user asks.`,
			Parameters: ai.Parameters{
				Type: "object",
				Properties: map[string]ai.Property{
					"url": {Type: "string"},
				},
				Required: []string{"url"},
			},
		},
	},
	ToolGenerateImage: {
		Type: "function",
		Function: ai.ToolFunction{
			Name:        ToolGenerateImage,
			Description: `Generate image with prompt`,
			Parameters: ai.Parameters{
				Type: "object",
				Properties: map[string]ai.Property{
					"prompt": {Type: "string", Description: "Detailed prompt in English"},
				},
				Required: []string{"prompt"},
			},
		},
	},
	ToolSearchImages: {
		Type: "function",
		Function: ai.ToolFunction{
			Name:        ToolSearchImages,
			Description: `Search images in internet`,
			Parameters: ai.Parameters{
				Type: "object",
				Properties: map[string]ai.Property{
					"keywords":    {Type: "string", Description: "Search keywords"},
					"max_results": {Type: "integer", Description: "Limit images in result. Min 1, max 5"},
					"time_limit":  {Type: "string", Enum: []string{"", "d", "w", "m", "y"}, Description: "Time range for search results: 'd' (last 24h), 'w' (last week), 'm' (last month), 'y' (last year). Default: empty. Leave empty for all time."},
				},
				Required: []string{"keywords", "max_results"},
			},
		},
	},
	ToolFetchYtComments: ToolFetchYtCommentsSpec,
}

var ToolFetchTgPostsSpec = ai.Tool{
	Type: "function",
	Function: ai.ToolFunction{
		Name:        ToolFetchTgPosts,
		Description: `Fetch posts from telegram channel. By default uses limit=10. Use duration for time period (e.g. "last 24h") OR limit for exact count. Can use both only if explicitly requested (e.g. "last 5 posts from 24h")`,
		Parameters: ai.Parameters{
			Type: "object",
			Properties: map[string]ai.Property{
				"channel_name": {Type: "string", Description: "Channel username"},
				"duration":     {Type: "string", Description: "Only use when time period is specified (e.g. 'posts from last 24h'). Must end with 'h'. Max: " + fmt.Sprint(TG_MAX_DURATION) + "h"},
				"limit":        {Type: "integer", Description: "Use when post count is specified (e.g. '5 posts'). Max: 100"},
			},
			Required: []string{"channel_name"},
		},
	},
}

var ToolFetchTgPostCommentsSpec = ai.Tool{
	Type: "function",
	Function: ai.ToolFunction{
		Name:        ToolFetchTgPostComments,
		Description: `Fetch comments from telegram post. Accepts either channel_name with post_id or automatically extracts them from telegram URL (e.g. https://t.me/channel_name/post_id)`,
		Parameters: ai.Parameters{
			Type: "object",
			Properties: map[string]ai.Property{
				"channel_name": {Type: "string", Description: "Channel username (can be extracted from https://t.me/channel_name/post_id)"},
				"post_id":      {Type: "integer", Description: "Post ID (can be extracted from https://t.me/channel_name/post_id)"},
			},
			Required: []string{"channel_name", "post_id"},
		},
	},
}

var ToolFetchYtCommentsSpec = ai.Tool{
	Type: "function",
	Function: ai.ToolFunction{
		Name:        ToolFetchYtComments,
		Description: `Fetch comments from YouTube video`,
		Parameters: ai.Parameters{
			Type: "object",
			Properties: map[string]ai.Property{
				"url": {
					Type:        "string",
					Description: "YouTube video URL",
				},
				"max": {
					Type:        "integer",
					Description: "Maximum number of comments to fetch (default: 50, max: 100)",
				},
			},
			Required: []string{"url"},
		},
	},
}
