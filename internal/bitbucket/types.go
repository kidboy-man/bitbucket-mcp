package bitbucket

// PullRequestRef holds parsed fields from a Bitbucket PR URL.
type PullRequestRef struct {
	Workspace string
	RepoSlug  string
	ID        int
}

// PullRequest is the domain representation of a Bitbucket pull request.
type PullRequest struct {
	ID               int
	Title            string
	Description      string
	Author           string
	SourceBranch     string
	DestBranch       string
	State            string
	CreatedOn        string
	SourceCommitHash string
	Links            PRLinks
}

// PRLinks holds URL links for a pull request.
type PRLinks struct {
	Self string
	HTML string
}

// Commit is a condensed PR commit.
type Commit struct {
	Hash    string
	Message string
	Author  string
}

// Comment is a PR comment (inline or PR-level).
type Comment struct {
	ID        int
	Body      string
	File      string // empty for PR-level comments
	Line      int    // inline.to; 0 for PR-level comments
	Author    string
	CreatedOn string
}

// Task is a Bitbucket PR task.
type Task struct {
	ID         int
	State      string // "UNRESOLVED" | "RESOLVED"
	Body       string
	Author     string
	CreatedOn  string
	ResolvedOn string
	ResolvedBy string
}

// Status is a Bitbucket build/pipeline status attached to a commit.
type Status struct {
	Key       string
	Name      string
	State     string // "SUCCESSFUL" | "FAILED" | "INPROGRESS" | "STOPPED"
	URL       string
	CreatedOn string
	UpdatedOn string
}

// StatusesSummary aggregates status counts.
type StatusesSummary struct {
	Total          int      `json:"total"`
	Successful     int      `json:"successful"`
	Failed         int      `json:"failed"`
	InProgress     int      `json:"in_progress"`
	FailedStatuses []Status `json:"failed_statuses,omitempty"`
}

// APIError is a typed error returned when Bitbucket responds with a non-2xx status.
type APIError struct {
	StatusCode int
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	return "bitbucket API error " + itoa(e.StatusCode) + " on " + e.Path + ": " + e.Body
}

// CreateCommentInput is the payload for creating a PR comment.
type CreateCommentInput struct {
	Body     string
	FilePath string // empty → PR-level comment
	NewLine  int    // inline.to; ignored when FilePath is empty
}

// CreateTaskInput is the payload for creating a PR task.
type CreateTaskInput struct {
	Body      string
	CommentID int // optional: link to a comment
}

// UpdateTaskInput is the payload for updating a PR task state.
type UpdateTaskInput struct {
	State string // "RESOLVED" | "UNRESOLVED"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
