package bitbucket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.bitbucket.org/2.0"

// Client is an authenticated Bitbucket Cloud REST API client.
type Client struct {
	workspace string
	email     string
	token     string
	http      *http.Client
}

// New creates a new Client. workspace is your Bitbucket workspace slug.
// email is your Atlassian account email, token is your Bitbucket API token.
func New(workspace, email, token string) *Client {
	return &Client{
		workspace: workspace,
		email:     email,
		token:     token,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// --- PR types ---------------------------------------------------------------

type PR struct {
	ID          int
	Title       string
	Description string
	Author      string
	SourceBranch string
	DestBranch   string
	State       string
	CreatedOn   string
	Links       PRLinks
	RawDiff     string
	Commits     []Commit
}

type PRLinks struct {
	Self string
	HTML string
}

type Commit struct {
	Hash    string
	Message string
	Author  string
}

// --- URL parsing ------------------------------------------------------------

// ParseURL extracts workspace, repoSlug, and prID from a Bitbucket PR URL.
// Supported format: https://bitbucket.org/{workspace}/{repo}/pull-requests/{id}
func ParseURL(rawURL string) (workspace, repoSlug string, prID int, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Expected: [workspace, repo, pull-requests, id]
	if len(parts) < 4 || parts[2] != "pull-requests" {
		return "", "", 0, fmt.Errorf(
			"URL does not match https://bitbucket.org/{workspace}/{repo}/pull-requests/{id}: %q",
			rawURL,
		)
	}

	prID, err = strconv.Atoi(parts[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("PR ID %q is not an integer", parts[3])
	}

	return parts[0], parts[1], prID, nil
}

// --- GetPR ------------------------------------------------------------------

// GetPR fetches PR metadata, diff, and commits in three parallel-friendly
// sequential calls and assembles a single PR struct.
func (c *Client) GetPR(prURL string) (*PR, error) {
	_, repo, prID, err := ParseURL(prURL)
	if err != nil {
		return nil, err
	}

	base := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", c.workspace, repo, prID)

	// Metadata
	metaBytes, err := c.get(base)
	if err != nil {
		return nil, fmt.Errorf("fetching PR metadata: %w", err)
	}

	pr, err := parsePRMeta(metaBytes)
	if err != nil {
		return nil, err
	}

	// Diff
	diffBytes, err := c.get(base + "/diff")
	if err != nil {
		return nil, fmt.Errorf("fetching PR diff: %w", err)
	}
	pr.RawDiff = string(diffBytes)

	// Commits
	commitsBytes, err := c.get(base + "/commits")
	if err != nil {
		return nil, fmt.Errorf("fetching PR commits: %w", err)
	}
	pr.Commits, err = parseCommits(commitsBytes)
	if err != nil {
		return nil, err
	}

	return pr, nil
}

// --- GetComments ------------------------------------------------------------

type InlineComment struct {
	ID       int
	Body     string
	File     string
	Line     int
	Author   string
	CreatedOn string
}

// GetComments returns all existing inline comments on a PR.
func (c *Client) GetComments(prURL string) ([]InlineComment, error) {
	_, repo, prID, err := ParseURL(prURL)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments", c.workspace, repo, prID)
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}

	return parseComments(data)
}

// --- PostInlineComment ------------------------------------------------------

// PostInlineComment posts an inline comment anchored to a specific file and
// diff position. diffPosition must be the DiffPosition from ParsedDiff
// (not the file's absolute line number).
func (c *Client) PostInlineComment(prURL, filePath string, diffPosition int, body string) error {
	_, repo, prID, err := ParseURL(prURL)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"content": map[string]string{"raw": body},
		"inline": map[string]any{
			"to":   diffPosition,
			"path": filePath,
		},
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/comments",
		baseURL, c.workspace, repo, prID)

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Bitbucket API error %d: %s", resp.StatusCode, body)
	}

	return nil
}

// --- internal HTTP helpers --------------------------------------------------

func (c *Client) get(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Bitbucket API error %d on %s: %s", resp.StatusCode, path, b)
	}

	return io.ReadAll(resp.Body)
}

// --- JSON parsers -----------------------------------------------------------

func parsePRMeta(data []byte) (*PR, error) {
	var v struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		CreatedOn   string `json:"created_on"`
		Author      struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
		Source struct {
			Branch struct{ Name string } `json:"branch"`
		} `json:"source"`
		Destination struct {
			Branch struct{ Name string } `json:"branch"`
		} `json:"destination"`
		Links struct {
			Self struct{ Href string } `json:"self"`
			HTML struct{ Href string } `json:"html"`
		} `json:"links"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing PR metadata: %w", err)
	}

	pr := &PR{
		ID:           v.ID,
		Title:        v.Title,
		Description:  v.Description,
		State:        v.State,
		CreatedOn:    v.CreatedOn,
		Author:       v.Author.DisplayName,
		SourceBranch: v.Source.Branch.Name,
		DestBranch:   v.Destination.Branch.Name,
	}
	pr.Links.Self = v.Links.Self.Href
	pr.Links.HTML = v.Links.HTML.Href
	return pr, nil
}

func parseCommits(data []byte) ([]Commit, error) {
	var v struct {
		Values []struct {
			Hash    string `json:"hash"`
			Message string `json:"message"`
			Author  struct {
				Raw string `json:"raw"`
			} `json:"author"`
		} `json:"values"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing commits: %w", err)
	}

	commits := make([]Commit, 0, len(v.Values))
	for _, c := range v.Values {
		commits = append(commits, Commit{
			Hash:    c.Hash[:min(7, len(c.Hash))],
			Message: strings.SplitN(c.Message, "\n", 2)[0], // subject line only
			Author:  c.Author.Raw,
		})
	}
	return commits, nil
}

func parseComments(data []byte) ([]InlineComment, error) {
	var v struct {
		Values []struct {
			ID      int `json:"id"`
			Content struct {
				Raw string `json:"raw"`
			} `json:"content"`
			Inline *struct {
				Path string `json:"path"`
				To   int    `json:"to"`
			} `json:"inline"`
			Author struct {
				DisplayName string `json:"display_name"`
			} `json:"author"`
			CreatedOn string `json:"created_on"`
		} `json:"values"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing comments: %w", err)
	}

	var result []InlineComment
	for _, c := range v.Values {
		if c.Inline == nil {
			continue // skip PR-level comments, only collect inline ones
		}
		result = append(result, InlineComment{
			ID:        c.ID,
			Body:      c.Content.Raw,
			File:      c.Inline.Path,
			Line:      c.Inline.To,
			Author:    c.Author.DisplayName,
			CreatedOn: c.CreatedOn,
		})
	}
	return result, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
