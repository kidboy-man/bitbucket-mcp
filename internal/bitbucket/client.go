package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.bitbucket.org/2.0"
const maxBodyRead = 64 * 1024 // 64 KB error body guard
const maxPaginationPages = 100
const maxJSONBodyRead = 8 * 1024 * 1024
const maxPaginationBodyRead = 32 * 1024 * 1024
const maxDiffBodyRead = 16 * 1024 * 1024

// Client is an authenticated Bitbucket Cloud REST API client.
type Client struct {
	workspace string
	username  string
	token     string
	baseURL   string
	http      *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.http = c }
}

// WithBaseURL overrides the Bitbucket API base URL (required for httptest).
func WithBaseURL(u string) Option {
	return func(cl *Client) { cl.baseURL = strings.TrimRight(u, "/") }
}

// NewClient constructs a Client. workspace is your Bitbucket workspace slug.
// username is the Bitbucket account username (or email). token is your app password or API token.
func NewClient(workspace, username, token string, opts ...Option) *Client {
	c := &Client{
		workspace: workspace,
		username:  username,
		token:     token,
		baseURL:   defaultBaseURL,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	// Do not change the caller's client or weaken its redirect policy.
	httpClient := *c.http
	checkRedirect := httpClient.CheckRedirect
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := c.validateDestination(req.URL); err != nil {
			return err
		}
		if checkRedirect != nil {
			if err := checkRedirect(req, via); err != nil {
				return err
			}
		} else if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return c.validateDestination(req.URL)
	}
	c.http = &httpClient
	return c
}

// --- URL parsing ------------------------------------------------------------

// ParseURL extracts workspace, repoSlug, and prID from a Bitbucket PR URL.
// Supported format: https://bitbucket.org/{workspace}/{repo}/pull-requests/{id}
func ParseURL(rawURL string) (PullRequestRef, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return PullRequestRef{}, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Host != "bitbucket.org" {
		return PullRequestRef{}, fmt.Errorf("URL host must be bitbucket.org, got %q", u.Host)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Expected: [workspace, repo, pull-requests, id]
	if len(parts) < 4 || parts[2] != "pull-requests" {
		return PullRequestRef{}, fmt.Errorf(
			"URL does not match https://bitbucket.org/{workspace}/{repo}/pull-requests/{id}: %q",
			rawURL,
		)
	}

	id, err := strconv.Atoi(parts[3])
	if err != nil {
		return PullRequestRef{}, fmt.Errorf("PR ID %q is not an integer", parts[3])
	}
	if parts[1] == "" {
		return PullRequestRef{}, fmt.Errorf("repo slug must not be empty in URL %q", rawURL)
	}

	return PullRequestRef{
		Workspace: parts[0],
		RepoSlug:  parts[1],
		ID:        id,
	}, nil
}

// validateRef parses the URL and confirms the workspace matches.
func (c *Client) validateRef(prURL string) (PullRequestRef, error) {
	ref, err := ParseURL(prURL)
	if err != nil {
		return PullRequestRef{}, err
	}
	if ref.Workspace != c.workspace {
		return PullRequestRef{}, fmt.Errorf(
			"PR workspace %q does not match configured workspace %q",
			ref.Workspace, c.workspace,
		)
	}
	return ref, nil
}

func (c *Client) prBase(ref PullRequestRef) string {
	return fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", c.workspace, ref.RepoSlug, ref.ID)
}

// --- Core HTTP helper -------------------------------------------------------

func (c *Client) validateDestination(destination *url.URL) error {
	base, err := url.Parse(c.baseURL)
	if err != nil || base.Hostname() == "" || (base.Scheme != "https" && base.Scheme != "http") || base.User != nil || base.Fragment != "" {
		return fmt.Errorf("invalid API base URL")
	}
	if destination.User != nil || destination.Fragment != "" || destination.Opaque != "" ||
		destination.Scheme != base.Scheme || !strings.EqualFold(destination.Hostname(), base.Hostname()) ||
		effectivePort(destination) != effectivePort(base) {
		return fmt.Errorf("request destination must match configured API origin without userinfo or fragment")
	}
	return nil
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	u, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("invalid request destination")
	}
	reqURL := path
	if !u.IsAbs() && u.Host == "" {
		reqURL = c.baseURL + path
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if err := c.validateDestination(req.URL); err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.token)
	return req, nil
}

func readBoundedBody(body io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("response exceeds %d byte limit", limit)
	}
	return b, nil
}

// do executes an authenticated HTTP request. body may be nil for GET/DELETE.
// out receives JSON-decoded response body on 2xx; may be nil to discard.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	remaining := int64(maxJSONBodyRead)
	return c.doWithBudget(ctx, method, path, body, out, &remaining)
}

func (c *Client) doWithBudget(ctx context.Context, method, path string, body any, out any, remaining *int64) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := c.newRequest(ctx, method, path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyRead))
		return &APIError{
			StatusCode: resp.StatusCode,
			Path:       path,
			Body:       string(b),
		}
	}

	b, err := readBoundedBody(resp.Body, min(int64(maxJSONBodyRead), *remaining))
	if err != nil {
		return err
	}
	*remaining -= int64(len(b))
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("decoding response from %s: %w", path, err)
		}
	}
	return nil
}

// --- Pagination helper -------------------------------------------------------

type page[T any] struct {
	Values []T    `json:"values"`
	Next   string `json:"next"`
}

func getAllPagesTyped[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var result []T
	seen := map[string]bool{}
	next := path
	pages := 0
	remaining := int64(maxPaginationBodyRead)

	for next != "" {
		if pages >= maxPaginationPages {
			return nil, fmt.Errorf("pagination exceeded %d pages", maxPaginationPages)
		}
		if seen[next] {
			return nil, fmt.Errorf("pagination detected repeated next URL")
		}
		seen[next] = true
		pages++

		var p page[T]
		if err := c.doWithBudget(ctx, http.MethodGet, next, nil, &p, &remaining); err != nil {
			return nil, err
		}
		result = append(result, p.Values...)
		next = p.Next
	}
	return result, nil
}

// --- Pull request methods ----------------------------------------------------

// GetPullRequest fetches PR metadata.
func (c *Client) GetPullRequest(ctx context.Context, prURL string) (*PullRequest, error) {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return nil, err
	}

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
			Commit struct{ Hash string } `json:"commit"`
		} `json:"source"`
		Destination struct {
			Branch struct{ Name string } `json:"branch"`
		} `json:"destination"`
		Links struct {
			Self struct{ Href string } `json:"self"`
			HTML struct{ Href string } `json:"html"`
		} `json:"links"`
	}

	if err := c.do(ctx, http.MethodGet, c.prBase(ref), nil, &v); err != nil {
		return nil, fmt.Errorf("fetching PR metadata: %w", err)
	}

	pr := &PullRequest{
		ID:               v.ID,
		Title:            v.Title,
		Description:      v.Description,
		State:            v.State,
		CreatedOn:        v.CreatedOn,
		Author:           v.Author.DisplayName,
		SourceBranch:     v.Source.Branch.Name,
		DestBranch:       v.Destination.Branch.Name,
		SourceCommitHash: v.Source.Commit.Hash,
	}
	pr.Links.Self = v.Links.Self.Href
	pr.Links.HTML = v.Links.HTML.Href
	return pr, nil
}

// GetPullRequestDiff fetches the raw unified diff for a PR.
func (c *Client) GetPullRequestDiff(ctx context.Context, prURL string) (string, error) {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return "", err
	}

	// Diff endpoint returns text/plain, not JSON — use raw request.
	req, err := c.newRequest(ctx, http.MethodGet, c.prBase(ref)+"/diff", nil)
	if err != nil {
		return "", fmt.Errorf("building diff request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching PR diff: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyRead))
		return "", &APIError{StatusCode: resp.StatusCode, Path: c.prBase(ref) + "/diff", Body: string(b)}
	}

	b, err := readBoundedBody(resp.Body, maxDiffBodyRead)
	if err != nil {
		return "", fmt.Errorf("reading diff response: %w", err)
	}
	return string(b), nil
}

// ListPullRequestCommits lists commits on a PR.
func (c *Client) ListPullRequestCommits(ctx context.Context, prURL string) ([]Commit, error) {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return nil, err
	}

	type raw struct {
		Hash    string `json:"hash"`
		Message string `json:"message"`
		Author  struct {
			Raw string `json:"raw"`
		} `json:"author"`
	}

	items, err := getAllPagesTyped[raw](ctx, c, c.prBase(ref)+"/commits")
	if err != nil {
		return nil, fmt.Errorf("fetching commits: %w", err)
	}

	commits := make([]Commit, 0, len(items))
	for _, r := range items {
		h := r.Hash
		if len(h) > 7 {
			h = h[:7]
		}
		commits = append(commits, Commit{
			Hash:    h,
			Message: strings.SplitN(r.Message, "\n", 2)[0],
			Author:  r.Author.Raw,
		})
	}
	return commits, nil
}

// ListPullRequestComments lists all comments (inline and PR-level).
func (c *Client) ListPullRequestComments(ctx context.Context, prURL string) ([]Comment, error) {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return nil, err
	}

	type raw struct {
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
	}

	items, err := getAllPagesTyped[raw](ctx, c, c.prBase(ref)+"/comments")
	if err != nil {
		return nil, fmt.Errorf("fetching comments: %w", err)
	}

	result := make([]Comment, 0, len(items))
	for _, r := range items {
		cm := Comment{
			ID:        r.ID,
			Body:      r.Content.Raw,
			Author:    r.Author.DisplayName,
			CreatedOn: r.CreatedOn,
		}
		if r.Inline != nil {
			cm.File = r.Inline.Path
			cm.Line = r.Inline.To
		}
		result = append(result, cm)
	}
	return result, nil
}

// CreatePullRequestComment posts a comment to a PR.
// When input.FilePath is non-empty, the comment is posted as an inline comment
// anchored at input.NewLine (new-file line number, passed as inline.to).
func (c *Client) CreatePullRequestComment(ctx context.Context, prURL string, input CreateCommentInput) (*Comment, error) {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"content": map[string]string{"raw": input.Body},
	}
	if input.FilePath != "" {
		payload["inline"] = map[string]any{
			"to":   input.NewLine,
			"path": input.FilePath,
		}
	}

	var v struct {
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
	}

	if err := c.do(ctx, http.MethodPost, c.prBase(ref)+"/comments", payload, &v); err != nil {
		return nil, fmt.Errorf("creating comment: %w", err)
	}

	cm := &Comment{
		ID:        v.ID,
		Body:      v.Content.Raw,
		Author:    v.Author.DisplayName,
		CreatedOn: v.CreatedOn,
	}
	if v.Inline != nil {
		cm.File = v.Inline.Path
		cm.Line = v.Inline.To
	}
	return cm, nil
}

// --- Task methods ------------------------------------------------------------

// ListPullRequestTasks lists tasks on a PR.
func (c *Client) ListPullRequestTasks(ctx context.Context, prURL string) ([]Task, error) {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return nil, err
	}

	type raw struct {
		ID      int    `json:"id"`
		State   string `json:"state"`
		Content struct {
			Raw string `json:"raw"`
		} `json:"content"`
		Creator struct {
			DisplayName string `json:"display_name"`
		} `json:"creator"`
		CreatedOn  string `json:"created_on"`
		ResolvedOn string `json:"resolved_on"`
		Resolver   *struct {
			DisplayName string `json:"display_name"`
		} `json:"resolver"`
	}

	items, err := getAllPagesTyped[raw](ctx, c, c.prBase(ref)+"/tasks")
	if err != nil {
		return nil, fmt.Errorf("fetching tasks: %w", err)
	}

	result := make([]Task, 0, len(items))
	for _, r := range items {
		t := Task{
			ID:        r.ID,
			State:     r.State,
			Body:      r.Content.Raw,
			Author:    r.Creator.DisplayName,
			CreatedOn: r.CreatedOn,
		}
		if r.ResolvedOn != "" {
			t.ResolvedOn = r.ResolvedOn
		}
		if r.Resolver != nil {
			t.ResolvedBy = r.Resolver.DisplayName
		}
		result = append(result, t)
	}
	return result, nil
}

// CreatePullRequestTask creates a task on a PR.
func (c *Client) CreatePullRequestTask(ctx context.Context, prURL string, input CreateTaskInput) (*Task, error) {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"content": map[string]string{"raw": input.Body},
	}
	if input.CommentID != 0 {
		payload["comment"] = map[string]any{"id": input.CommentID}
	}

	var v struct {
		ID      int    `json:"id"`
		State   string `json:"state"`
		Content struct {
			Raw string `json:"raw"`
		} `json:"content"`
		Creator struct {
			DisplayName string `json:"display_name"`
		} `json:"creator"`
		CreatedOn string `json:"created_on"`
	}

	if err := c.do(ctx, http.MethodPost, c.prBase(ref)+"/tasks", payload, &v); err != nil {
		return nil, fmt.Errorf("creating task: %w", err)
	}

	return &Task{
		ID:        v.ID,
		State:     v.State,
		Body:      v.Content.Raw,
		Author:    v.Creator.DisplayName,
		CreatedOn: v.CreatedOn,
	}, nil
}

// UpdatePullRequestTask updates the state of a PR task.
func (c *Client) UpdatePullRequestTask(ctx context.Context, prURL string, taskID int, input UpdateTaskInput) (*Task, error) {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/tasks/%d", c.prBase(ref), taskID)
	payload := map[string]any{
		"state": input.State,
	}

	var v struct {
		ID      int    `json:"id"`
		State   string `json:"state"`
		Content struct {
			Raw string `json:"raw"`
		} `json:"content"`
		Creator struct {
			DisplayName string `json:"display_name"`
		} `json:"creator"`
		CreatedOn string `json:"created_on"`
	}

	if err := c.do(ctx, http.MethodPut, path, payload, &v); err != nil {
		return nil, fmt.Errorf("updating task: %w", err)
	}

	return &Task{
		ID:        v.ID,
		State:     v.State,
		Body:      v.Content.Raw,
		Author:    v.Creator.DisplayName,
		CreatedOn: v.CreatedOn,
	}, nil
}

// --- Status methods ----------------------------------------------------------

// ListPullRequestStatuses lists commit statuses for the PR's source commit.
func (c *Client) ListPullRequestStatuses(ctx context.Context, prURL string) ([]Status, error) {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return nil, err
	}

	type raw struct {
		Key   string `json:"key"`
		Name  string `json:"name"`
		State string `json:"state"`
		URL   string `json:"url"`
		Links struct {
			Commit struct {
				Href string `json:"href"`
			} `json:"commit"`
		} `json:"links"`
		CreatedOn string `json:"created_on"`
		UpdatedOn string `json:"updated_on"`
	}

	items, err := getAllPagesTyped[raw](ctx, c, c.prBase(ref)+"/statuses")
	if err != nil {
		return nil, fmt.Errorf("fetching statuses: %w", err)
	}

	result := make([]Status, 0, len(items))
	for _, r := range items {
		result = append(result, Status{
			Key:       r.Key,
			Name:      r.Name,
			State:     r.State,
			URL:       r.URL,
			CreatedOn: r.CreatedOn,
			UpdatedOn: r.UpdatedOn,
		})
	}
	return result, nil
}

// --- Approval / request-changes methods -------------------------------------

// ApprovePullRequest posts an approval for the PR.
func (c *Client) ApprovePullRequest(ctx context.Context, prURL string) error {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, c.prBase(ref)+"/approve", nil, nil)
}

// RequestChanges posts a request-changes review state.
func (c *Client) RequestChanges(ctx context.Context, prURL string) error {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, c.prBase(ref)+"/request-changes", nil, nil)
}

// --- Comment thread resolve/reopen ------------------------------------------

// ResolveCommentThread marks a comment thread as resolved.
func (c *Client) ResolveCommentThread(ctx context.Context, prURL string, commentID int) error {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s/comments/%d/resolve", c.prBase(ref), commentID)
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

// ReopenCommentThread reopens a resolved comment thread.
func (c *Client) ReopenCommentThread(ctx context.Context, prURL string, commentID int) error {
	ref, err := c.validateRef(prURL)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s/comments/%d/resolve", c.prBase(ref), commentID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
