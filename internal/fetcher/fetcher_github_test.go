package fetcher

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGithubFetcher_Handle_RepoInfoSuccess(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	// Mock repo info response
	repoInfoJSON := `{
		"name": "test-repo",
		"description": "A test repository",
		"stargazers_count": 100,
		"forks_count": 20,
		"open_issues_count": 5,
		"created_at": "2023-01-01T00:00:00Z",
		"updated_at": "2023-12-01T00:00:00Z",
		"pushed_at": "2023-12-01T00:00:00Z",
		"owner": {
			"login": "testuser",
			"avatar_url": "https://avatar.url"
		}
	}`

	// Mock user info response
	userInfoJSON := `{
		"login": "testuser",
		"name": "Test User",
		"followers": 50,
		"public_repos": 10,
		"company": "Test Corp"
	}`

	// Mock repos for stars calculation
	reposJSON := `[
		{"stargazers_count": 100},
		{"stargazers_count": 50}
	]`

	// Mock readme response (empty for simplicity)
	readmeJSON := `{"content": ""}`

	// Mock PRs response (empty)
	prsJSON := `[]`

	// Setup expectations
	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://api.github.com/repos/testuser/test-repo"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(repoInfoJSON))),
			Header:     make(http.Header),
		}, nil)

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://api.github.com/users/testuser"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(userInfoJSON))),
			Header:     make(http.Header),
		}, nil)

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://api.github.com/users/testuser/repos?per_page=100"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(reposJSON))),
			Header:     make(http.Header),
		}, nil)

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://api.github.com/repos/testuser/test-repo/readme"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(readmeJSON))),
			Header:     make(http.Header),
		}, nil)

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://api.github.com/repos/testuser/test-repo/pulls?state=all&per_page=3"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(prsJSON))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewGithubFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://github.com/testuser/test-repo",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Len(t, response.Content, 1, "Should have exactly one content item")
	assert.Equal(t, ContentTypeText, response.Content[0].Type, "Content type should be text")
	assert.Contains(t, response.Content[0].Text, "GitHub Repository: testuser/test-repo", "Should contain repo info")
	assert.Contains(t, response.Content[0].Text, "Description: A test repository", "Should contain description")
	assert.Contains(t, response.Content[0].Text, "Stars: 100", "Should contain stars count")
	assert.Contains(t, response.Content[0].Text, "Owner: testuser (Test User)", "Should contain owner info")
}

func TestGithubFetcher_Handle_FileContentSuccess(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	// Mock file content response (base64 encoded "Hello, World!")
	fileContentJSON := `{
		"content": "SGVsbG8sIFdvcmxkIQ==",
		"encoding": "base64"
	}`

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://api.github.com/repos/testuser/test-repo/contents/test.txt?ref=main"
		})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(fileContentJSON))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewGithubFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://github.com/testuser/test-repo/blob/main/test.txt",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Len(t, response.Content, 1, "Should have exactly one content item")
	assert.Equal(t, ContentTypeText, response.Content[0].Type, "Content type should be text")
	assert.Contains(t, response.Content[0].Text, "File content from test-repo/test.txt@main:", "Should contain file info")
	assert.Contains(t, response.Content[0].Text, "Hello, World!", "Should contain decoded file content")
}

func TestGithubFetcher_Handle_HTTPError(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(nil, assert.AnError)

	fetcher := NewGithubFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://github.com/testuser/test-repo",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	assert.Error(t, err, "Should return error for HTTP failure")
	assert.True(t, response.IsError, "Response should be an error")
}

func TestGithubFetcher_Handle_InvalidURL(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	fetcher := NewGithubFetcher(l, mockClient)

	// Test with invalid GitHub URL (no repo name)
	request, err := NewRequestPayload(
		"https://github.com/testuser",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	assert.Error(t, err, "Should return error for invalid URL")
	assert.True(t, response.IsError, "Response should be an error")
}

func TestGithubFetcher_Handle_FileNotFound(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)

	mockClient.EXPECT().
		Do(mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://api.github.com/repos/testuser/test-repo/contents/nonexistent.txt?ref=main"
		})).
		Return(&http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"message": "Not Found"}`))),
			Header:     make(http.Header),
		}, nil)

	fetcher := NewGithubFetcher(l, mockClient)

	request, err := NewRequestPayload(
		"https://github.com/testuser/test-repo/blob/main/nonexistent.txt",
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	assert.Error(t, err, "Should return error for file not found")
	assert.True(t, response.IsError, "Response should be an error")
}
