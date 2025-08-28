package fetch

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

func (f *Fetcher) parseGithubRepo(URL string) Response {
	u, err := url.Parse(URL)
	if err != nil {
		return f.errorResponse(fmt.Errorf("invalid URL: %v", err))
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) < 2 {
		return f.errorResponse(fmt.Errorf("invalid GitHub URL"))
	}

	owner := pathParts[0]
	repo := pathParts[1]

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
		}
	}

	// Fetch repo info
	repoInfo, err := f.getRepoInfo(owner, repo)
	if err != nil {
		return f.errorResponse(fmt.Errorf("repo info: %v", err))
	}

	// TODO: add commit count for last 30 days, add count closed issues
	// Build response text (LLM-optimized format)
	text := fmt.Sprintf(
		"GitHub Repository: %s/%s\n"+
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
		time.Since(repoInfo.PushedAt).Round(time.Hour*24),
	)

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
		text += fmt.Sprintf(
			"Owner: %s (%s)\n"+
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
		text += "README:\n" + readmeContent + "\n\n"
	}

	// Add PR summary
	prs, _ := f.getPullRequests(owner, repo)
	if len(prs) > 0 {
		text += "Recent Pull Requests:\n"
		for i, pr := range prs {
			text += fmt.Sprintf(
				"- [%s] %s by %s\n  %s\n",
				pr.State,
				pr.Title,
				pr.User.Login,
				pr.HTMLURL,
			)
			if i >= 2 {
				break // Limit to 3 PRs
			}
		}
	}

	// Create response
	return Response{
		Content: []Content{
			{Type: ContentTypeText, Text: strings.TrimSpace(text)},
		},
		IsError: false,
	}
}

// Helper to summarize long text for LLM context
func (f *Fetcher) summarizeText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "... [truncated]"
}

func (f *Fetcher) getRepoInfo(owner, repo string) (*GitHubRepoInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	resp, body, err := f.fetch(RequestPayload{URL: apiURL})
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

func (f *Fetcher) getUserInfo(login string) (*GitHubUserInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/users/%s", login)
	resp, body, err := f.fetch(RequestPayload{URL: apiURL})
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

func (f *Fetcher) getReadme(owner, repo string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/readme", owner, repo)
	resp, body, err := f.fetch(RequestPayload{URL: apiURL})
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

func (f *Fetcher) getPullRequests(owner, repo string) ([]GitHubPullRequest, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=all&per_page=3", owner, repo)
	resp, body, err := f.fetch(RequestPayload{URL: apiURL})
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

func (f *Fetcher) getUserTotalStars(login string) (int, error) {
	apiURL := fmt.Sprintf("https://api.github.com/users/%s/repos?per_page=100", login)
	resp, body, err := f.fetch(RequestPayload{URL: apiURL})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Repos API error: %s", resp.Status)
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

// for last 30 days
func (f *Fetcher) getCommitCount(owner, repo string) (int, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?since=%s",
		owner,
		repo,
		time.Now().AddDate(0, -1, 0).Format(time.RFC3339))

	resp, body, err := f.fetch(RequestPayload{URL: apiURL})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	var commits []any
	if err := json.Unmarshal([]byte(body), &commits); err != nil {
		return 0, err
	}
	return len(commits), nil
}

func (f *Fetcher) getClosedIssuesCount(owner, repo string) (int, error) {
	apiURL := fmt.Sprintf("https://api.github.com/search/issues?q=repo:%s/%s+type:issue+state:closed", owner, repo)
	resp, body, err := f.fetch(RequestPayload{URL: apiURL})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		TotalCount int `json:"total_count"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return 0, err
	}
	return result.TotalCount, nil
}

func (f *Fetcher) getFileContent(owner, repo, branch, path string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, branch)
	resp, body, err := f.fetch(RequestPayload{URL: apiURL})
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

func (f *Fetcher) isBinary(data []byte) bool {
	if len(data) > 1024 {
		data = data[:1024] // Check first 1KB
	}
	return slices.Contains(data, 0)
}
