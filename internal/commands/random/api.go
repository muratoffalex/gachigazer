package random

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/app/di"
	"github.com/muratoffalex/gachigazer/internal/cache"
	"github.com/muratoffalex/gachigazer/internal/logger"
)

var ErrorAuth = errors.New("authentication error")

type api struct {
	baseURL string
	apiKey  string
	userID  string
	logger  logger.Logger
	cache   cache.Cache
	client  *http.Client
}

func newAPI(di *di.Container) *api {
	cfg := di.Cfg.GetRCommandConfig()
	return &api{
		baseURL: cfg.APIURL,
		apiKey:  cfg.APIKey,
		userID:  cfg.APIUserID,
		logger:  di.Logger,
		cache:   di.Cache,
		client:  di.HttpClient,
	}
}

func (a *api) doRequest(URL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if strings.Contains(string(body), "Missing authentication") {
		a.logger.WithError(fmt.Errorf("authentication error: %s", string(body))).Error("authentication error")
		return nil, ErrorAuth
	}

	// Create a new response body since we've already read the original one
	resp.Body = io.NopCloser(bytes.NewReader(body))

	return resp, nil
}

func (a *api) buildURL(params url.Values) string {
	if a.apiKey != "" {
		params.Add("api_key", a.apiKey)
	}
	if a.userID != "" {
		params.Add("user_id", a.userID)
	}
	return fmt.Sprintf("%s?%s", a.baseURL, params.Encode())
}

func (a *api) getPosts(tags []string) ([]Post, error) {
	key := fmt.Sprintf("mem:posts:%s", strings.Join(tags, "_"))

	if data, found := a.cache.Get(key); found {
		var posts []Post
		if err := json.Unmarshal(data, &posts); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cached posts: %w", err)
		}
		a.logger.WithFields(logger.Fields{
			"tags": tags,
		}).Info("Retrieved posts from cache")
		return posts, nil
	}

	params := url.Values{}
	params.Add("page", "dapi")
	params.Add("s", "post")
	params.Add("q", "index")
	params.Add("limit", "1000")
	params.Add("json", "1")
	params.Add("tags", strings.Join(tags, " "))

	fullURL := a.buildURL(params)
	a.logger.WithFields(logger.Fields{
		"url": fullURL,
	}).Info("Fetching posts from API")

	resp, err := a.doRequest(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch posts: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if len(body) == 0 || string(body) == "[]" {
		a.logger.WithFields(logger.Fields{
			"tags": tags,
		}).Debug("No posts found for given tags")
		return nil, nil
	}

	var posts []Post
	if err := json.Unmarshal(body, &posts); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(posts) == 0 {
		return nil, nil
	}

	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Score > posts[j].Score
	})

	data, err := json.Marshal(posts)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal posts for cache: %w", err)
	}

	if err := a.cache.Set(key, data, 24*time.Hour); err != nil {
		a.logger.WithError(err).Error("Failed to cache posts")
	}

	go func(posts []Post) {
		if err := a.addTagsFromPosts(posts); err != nil {
			a.logger.WithError(err).Error("Failed to cache tags from posts asynchronously")
		}
	}(posts)

	return posts, nil
}

func (a *api) getTags() ([]Tag, error) {
	key := "db:tags"

	if data, found := a.cache.Get(key); found {
		var tags []Tag
		if err := json.Unmarshal(data, &tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cached tags: %w", err)
		}
		return tags, nil
	}

	params := url.Values{}
	params.Add("page", "dapi")
	params.Add("s", "tag")
	params.Add("q", "index")
	params.Add("limit", "1000")

	fullURL := a.buildURL(params)
	a.logger.WithFields(logger.Fields{
		"url": fullURL,
	}).Debug("Fetching tags from API")

	resp, err := a.doRequest(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch posts: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var tags Tags
	if err := xml.Unmarshal(body, &tags); err != nil {
		return nil, fmt.Errorf("failed to parse XML response: %w", err)
	}

	data, err := json.Marshal(tags.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags for cache: %w", err)
	}

	if err := a.cache.Set(key, data, time.Hour); err != nil {
		a.logger.WithError(err).Error("Failed to cache tags")
	}

	return tags.Tags, nil
}

func (a *api) getPostsWithScore(posts []Post, minScore int) []Post {
	if minScore <= 0 {
		return posts
	}

	var filteredPosts []Post
	for _, post := range posts {
		if post.Score >= minScore {
			filteredPosts = append(filteredPosts, post)
		}
	}

	sort.Slice(filteredPosts, func(i, j int) bool {
		return filteredPosts[i].Score > filteredPosts[j].Score
	})

	return filteredPosts
}

func (a *api) addTagsFromPosts(posts []Post) error {
	tags, err := a.getTags()
	if err != nil {
		return err
	}

	existingTags := make(map[string]bool)
	for _, tag := range tags {
		existingTags[tag.Name] = true
	}

	newTags := make(map[string]Tag)
	for _, post := range posts {
		postTags := strings.FieldsSeq(strings.TrimSpace(post.Tags))
		for tagName := range postTags {
			if !existingTags[tagName] {
				newTags[tagName] = Tag{
					Name:  tagName,
					Count: 1,
				}
			}
		}
	}

	if len(newTags) > 0 {
		var tagNames []string
		for tagName := range newTags {
			tagNames = append(tagNames, tagName)
		}
		sort.Strings(tagNames)

		a.logger.WithFields(logger.Fields{
			"new_tags_count": len(newTags),
			"new_tags":       strings.Join(tagNames, ", "),
		}).Debug("Adding new tags to cache")

		for _, tag := range newTags {
			tags = append(tags, tag)
		}

		data, err := json.Marshal(tags)
		if err != nil {
			return fmt.Errorf("failed to marshal updated tags: %w", err)
		}

		if err := a.cache.Set("db:tags", data, time.Hour); err != nil {
			return fmt.Errorf("failed to update tags cache: %w", err)
		}
	} else {
		a.logger.Debug("No new tags found in posts")
	}

	return nil
}

func (a *api) findSimilarTags(searchTag string) []string {
	tags, err := a.getTags()
	if err != nil {
		a.logger.WithError(err).Error("Failed to get tags for similarity search")
		return nil
	}

	searchTag = strings.ToLower(searchTag)
	tagScores := make(map[string]int)

	searchParts := strings.Split(searchTag, "_")

	for _, tag := range tags {
		tagName := strings.ToLower(tag.Name)

		if tagName == searchTag {
			tagScores[tag.Name] = 10
			continue
		}

		if strings.Contains(tagName, searchTag) {
			tagScores[tag.Name] = 5
			continue
		}

		// Only if simple checks did not work, we make complex
		score := 0

		tagParts := strings.Split(tagName, "_")

		partMatches := 0
		for _, searchPart := range searchParts {
			if slices.Contains(tagParts, searchPart) {
				partMatches++
				score += 3
			}
		}

		// We calculate the distance of Levenstein only if there are partial coincidences
		if partMatches > 0 {
			distance := levenshteinDistance(searchTag, tagName)
			if distance <= 2 {
				score += (3 - distance)
			}

			if score > 0 {
				tagScores[tag.Name] = score
			}
		}
	}

	type tagScore struct {
		tag   string
		score int
	}
	var scores []tagScore
	for tag, score := range tagScores {
		if score > 0 {
			scores = append(scores, tagScore{tag, score})
		}
	}

	// Sort by score
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	var result []string
	for i := 0; i < len(scores) && i < 6; i++ {
		result = append(result, scores[i].tag)
	}

	a.logger.WithFields(logger.Fields{
		"search_tag": searchTag,
		"found_tags": result,
		"scores":     scores,
	}).Debug("Found similar tags")

	return result
}
