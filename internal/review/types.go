package review

import (
	"github.com/kidboy-man/bitbucket-mcp/internal/bitbucket"
	"github.com/kidboy-man/bitbucket-mcp/internal/diff"
)

// --- Input types ------------------------------------------------------------

// GetPRContextInput is the input for GetPRContext.
type GetPRContextInput struct {
	PRURL       string
	MaxDiffLines int
	MaxFiles    int
	DiffCursor  string
}

// GetPRDiffInput is the input for GetPRDiff.
type GetPRDiffInput struct {
	PRURL      string
	File       string
	Cursor     string
	MaxLines   int
}

// DraftReviewInput is the input for DraftReviewComments.
type DraftReviewInput struct {
	PRURL          string
	Findings       []ReviewFinding
	SummaryBody    string
	SummaryVerdict string // "PASS" | "COMMENT" | "BLOCK"
}

// ApprovePRInput is the input for ApprovePR.
type ApprovePRInput struct {
	PRURL                    string
	ExpectedSourceCommitHash string
}

// RequestChangesInput is the input for RequestChangesPR.
type RequestChangesInput struct {
	PRURL                    string
	ExpectedSourceCommitHash string
}

// ResolveTaskInput is the input for ResolveTask.
type ResolveTaskInput struct {
	PRURL  string
	TaskID int
}

// ReopenTaskInput is the input for ReopenTask.
type ReopenTaskInput struct {
	PRURL  string
	TaskID int
}

// ResolveCommentThreadInput is the input for ResolveCommentThread.
type ResolveCommentThreadInput struct {
	PRURL     string
	CommentID int
}

// ReopenCommentThreadInput is the input for ReopenCommentThread.
type ReopenCommentThreadInput struct {
	PRURL     string
	CommentID int
}

// --- Domain types -----------------------------------------------------------

// ReviewFinding is a single review comment to be posted.
type ReviewFinding struct {
	File       string
	NewLineNo  int
	Body       string
	Severity   string // "HIGH" | "MEDIUM" | "LOW"
	CreateTask bool
}

// ReviewDraft is a validated, stateless posting plan produced by DraftReviewComments.
type ReviewDraft struct {
	PRURL                  string                 `json:"pr_url"`
	SourceCommitHash       string                 `json:"source_commit_hash"`
	CommentsToPost         []DraftComment         `json:"comments_to_post"`
	TasksToCreate          []DraftTask            `json:"tasks_to_create"`
	SummaryComment         *DraftSummary          `json:"summary_comment,omitempty"`
	Skipped                []SkippedFinding       `json:"skipped,omitempty"`
	Duplicates             []DuplicateFinding     `json:"duplicates,omitempty"`
	ExistingCommentsChecked bool                  `json:"existing_comments_checked"`
}

// DraftComment is one planned inline comment.
type DraftComment struct {
	File      string `json:"file"`
	NewLineNo int    `json:"new_line_no"`
	Body      string `json:"body"`
	Severity  string `json:"severity"`
}

// DraftTask is one planned task linked to a comment by index.
type DraftTask struct {
	CommentIndex int    `json:"comment_index"` // index into CommentsToPost
	Body         string `json:"body"`
}

// DraftSummary is the planned PR-level summary comment.
type DraftSummary struct {
	Body    string `json:"body"`
	Verdict string `json:"verdict"`
}

// SkippedFinding records a finding that was excluded from the draft.
type SkippedFinding struct {
	Index   int           `json:"index"`
	Reason  string        `json:"reason"`
	Finding ReviewFinding `json:"finding"`
}

// DuplicateFinding records a finding that duplicates an existing comment.
type DuplicateFinding struct {
	Index   int           `json:"index"`
	Reason  string        `json:"reason"`
	Finding ReviewFinding `json:"finding"`
}

// --- Output types -----------------------------------------------------------

// PRContext is the aggregate returned by GetPRContext.
type PRContext struct {
	PR             *bitbucket.PullRequest  `json:"pr"`
	Commits        []bitbucket.Commit      `json:"commits"`
	Comments       []bitbucket.Comment     `json:"comments"`
	Tasks          []bitbucket.Task        `json:"tasks"`
	StatusesSummary bitbucket.StatusesSummary `json:"statuses_summary"`
	Diff           *diff.DiffPage          `json:"diff"`
}

// PostReviewResult is the result of PostReviewComments.
type PostReviewResult struct {
	PostedComments []bitbucket.Comment   `json:"posted_comments"`
	CreatedTasks   []bitbucket.Task      `json:"created_tasks"`
	PostedSummary  *bitbucket.Comment    `json:"posted_summary,omitempty"`
	Failed         []PostFailure         `json:"failed,omitempty"`
}

// PostFailure records one item that failed to post.
type PostFailure struct {
	Kind  string `json:"kind"` // "comment" | "task" | "summary"
	Index int    `json:"index"`
	Error string `json:"error"`
}

// ApprovePRResult is the result of ApprovePR.
type ApprovePRResult struct {
	Approved         bool   `json:"approved"`
	SourceCommitHash string `json:"source_commit_hash"`
}

// RequestChangesResult is the result of RequestChangesPR.
type RequestChangesResult struct {
	RequestedChanges bool   `json:"requested_changes"`
	SourceCommitHash string `json:"source_commit_hash"`
}

// TaskResult is the result of ResolveTask / ReopenTask.
type TaskResult struct {
	TaskID int    `json:"task_id"`
	State  string `json:"state"`
}
