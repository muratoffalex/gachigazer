package fetcher

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/logger"
)

type GitHubRepoInfo struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Stars       int       `json:"stargazers_count"`
	Forks       int       `json:"forks_count"`
	PushedAt    time.Time `json:"pushed_at"`
	OpenIssues  int       `json:"open_issues_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Owner       struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
}

type GitHubUserInfo struct {
	Login       string `json:"login"`
	Name        string `json:"name"`
	Followers   int    `json:"followers"`
	PublicRepos int    `json:"public_repos"`
	Company     string `json:"company"`
	TotalStars  int
}

type GitHubReadme struct {
	Content string `json:"content"`
}

type GitHubPullRequest struct {
	Title string `json:"title"`
	State string `json:"state"`
	User  struct {
		Login string `json:"login"`
	} `json:"user"`
	HTMLURL string `json:"html_url"`
}

type GitHubFileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type GithubFetcher struct {
	BaseFetcher
}

func NewGithubFetcher(l logger.Logger, client HTTPClient) GithubFetcher {
	return GithubFetcher{
		BaseFetcher: NewBaseFetcher(FetcherNameGithub, "github\\.com", client, l),
	}
}

func (f GithubFetcher) Handle(request Request) (Response, error) {
	u, err := url.Parse(request.URL())
	if err != nil {
		return f.errorResponse(fmt.Errorf("invalid URL: %v", err))
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) < 2 {
		return f.errorResponse(fmt.Errorf("invalid GitHub URL"))
	}

	owner := pathParts[0]
	repo := pathParts[1]

	// fetch file content
	if len(pathParts) > 3 && (pathParts[2] == "blob" || !strings.Contains(pathParts[2], "blob")) {
		branch := "main" // default branch
		filePath := ""

		if pathParts[2] == "blob" {
			if len(pathParts) < 5 {
				return f.errorResponse(fmt.Errorf("invalid file URL"))
			}
			branch = pathParts[3]
			filePath = strings.Join(pathParts[4:], "/")
		} else {
			branch = "main"
			filePath = strings.Join(pathParts[2:], "/")
		}

		content, err := f.getFileContent(owner, repo, branch, filePath)
		if err != nil {
			return f.errorResponse(fmt.Errorf("failed to get file content: %v", err))
		}

		return Response{
			Content: []Content{
				{Type: ContentTypeText, Text: fmt.Sprintf("File content from %s/%s@%s:\n\n%s",
					repo, filePath, branch, content)},
			},
			IsError: false,
		}, nil
	}

	// Fetch repo info
	repoInfo, err := f.getRepoInfo(owner, repo)
	if err != nil {
		return f.errorResponse(fmt.Errorf("repo info: %v", err))
	}

	var text strings.Builder
	fmt.Fprintf(&text, "GitHub Repository: %s/%s\n"+
		"Description: %s\n"+
		"Stars: %d | Forks: %d | Issues: %d\n"+
		"Created: %s | Updated: %s | Last push: %s ago\n\n",
		owner,
		repo,
		repoInfo.Description,
		repoInfo.Stars,
		repoInfo.Forks,
		repoInfo.OpenIssues,
		repoInfo.CreatedAt.Format("2006-01-02"),
		repoInfo.UpdatedAt.Format("2006-01-02"),
		time.Since(repoInfo.PushedAt).Round(time.Second))

	// Add owner info
	userInfo, err := f.getUserInfo(owner)
	if err != nil {
		log.Printf("Failed to get user info: %v", err)
	} else {
		// Fetch user's total stars
		totalStars, err := f.getUserTotalStars(owner)
		if err != nil {
			log.Printf("Failed to get user stars: %v", err)
		} else {
			userInfo.TotalStars = totalStars
		}
	}
	if userInfo != nil {
		fmt.Fprintf(&text, "Owner: %s (%s)\n"+
			"Followers: %d | Public repos: %d | Total stars: %d\n\n",
			userInfo.Login,
			userInfo.Name,
			userInfo.Followers,
			userInfo.PublicRepos,
			userInfo.TotalStars,
		)
	}

	// Add concise README preview
	readmeContent, _ := f.getReadme(owner, repo)
	if readmeContent != "" {
		text.WriteString("README:\n" + readmeContent + "\n\n")
	}

	// Add PR summary
	prs, _ := f.getPullRequests(owner, repo)
	if len(prs) > 0 {
		text.WriteString("Recent Pull Requests:\n")
		for i, pr := range prs {
			fmt.Fprintf(&text, "- [%s] %s by %s\n  %s\n",
				pr.State,
				pr.Title,
				pr.User.Login,
				pr.HTMLURL)
			if i >= 2 {
				break // Limit to 3 PRs
			}
		}
	}

	// Create response
	return Response{
		Content: []Content{
			{Type: ContentTypeText, Text: strings.TrimSpace(text.String())},
		},
		IsError: false,
	}, nil
}

func (f GithubFetcher) getRepoInfo(owner, repo string) (*GitHubRepoInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	resp, body, err := f.fetch(MustNewRequestPayload(apiURL, nil, nil))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var repoInfo GitHubRepoInfo
	return &repoInfo, json.Unmarshal([]byte(body), &repoInfo)
}

func (f GithubFetcher) getUserInfo(login string) (*GitHubUserInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/users/%s", login)
	resp, body, err := f.fetch(MustNewRequestPayload(apiURL, nil, nil))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var userInfo GitHubUserInfo
	return &userInfo, json.Unmarshal([]byte(body), &userInfo)
}

func (f GithubFetcher) getReadme(owner, repo string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/readme", owner, repo)
	resp, body, err := f.fetch(MustNewRequestPayload(apiURL, nil, nil))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var readme struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(body), &readme); err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(readme.Content)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func (f GithubFetcher) getPullRequests(owner, repo string) ([]GitHubPullRequest, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=all&per_page=3", owner, repo)
	resp, body, err := f.fetch(MustNewRequestPayload(apiURL, nil, nil))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var prs []GitHubPullRequest
	return prs, json.Unmarshal([]byte(body), &prs)
}

func (f GithubFetcher) getUserTotalStars(login string) (int, error) {
	apiURL := fmt.Sprintf("https://api.github.com/users/%s/repos?per_page=100", login)
	resp, body, err := f.fetch(MustNewRequestPayload(apiURL, nil, nil))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("repos API error: %s", resp.Status)
	}

	var repos []struct {
		Stars int `json:"stargazers_count"`
	}
	if err := json.Unmarshal([]byte(body), &repos); err != nil {
		return 0, err
	}

	totalStars := 0
	for _, repo := range repos {
		totalStars += repo.Stars
	}

	return totalStars, nil
}

func (f GithubFetcher) getFileContent(owner, repo, branch, path string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, branch)
	resp, body, err := f.fetch(MustNewRequestPayload(apiURL, nil, nil))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var fileContent GitHubFileContent
	if err := json.Unmarshal([]byte(body), &fileContent); err != nil {
		return "", err
	}

	if fileContent.Encoding != "base64" {
		return "", fmt.Errorf("unsupported encoding: %s", fileContent.Encoding)
	}

	// Check file size (GitHub API returns size in bytes)
	if len(fileContent.Content) > 1_000_000 { // ~1MB
		return "", fmt.Errorf("file too large (max 1MB allowed)")
	}

	decoded, err := base64.StdEncoding.DecodeString(fileContent.Content)
	if err != nil {
		return "", err
	}

	// Basic binary file detection
	if f.isBinary(decoded) {
		return "", fmt.Errorf("binary file detected")
	}

	return string(decoded), nil
}

func (f GithubFetcher) isBinary(data []byte) bool {
	if len(data) > 1024 {
		data = data[:1024] // Check first 1KB
	}
	return slices.Contains(data, 0)
}
