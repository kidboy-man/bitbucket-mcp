package review

import (
	"context"

	"github.com/kidboy-man/bitbucket-mcp/internal/bitbucket"
)

// PullRequestStore is the port the review service depends on for Bitbucket operations.
// The bitbucket.Client satisfies this interface.
type PullRequestStore interface {
	GetPullRequest(ctx context.Context, prURL string) (*bitbucket.PullRequest, error)
	GetPullRequestDiff(ctx context.Context, prURL string) (string, error)
	ListPullRequestCommits(ctx context.Context, prURL string) ([]bitbucket.Commit, error)
	ListPullRequestComments(ctx context.Context, prURL string) ([]bitbucket.Comment, error)
	CreatePullRequestComment(ctx context.Context, prURL string, input bitbucket.CreateCommentInput) (*bitbucket.Comment, error)
	ListPullRequestTasks(ctx context.Context, prURL string) ([]bitbucket.Task, error)
	CreatePullRequestTask(ctx context.Context, prURL string, input bitbucket.CreateTaskInput) (*bitbucket.Task, error)
	UpdatePullRequestTask(ctx context.Context, prURL string, taskID int, input bitbucket.UpdateTaskInput) (*bitbucket.Task, error)
	ListPullRequestStatuses(ctx context.Context, prURL string) ([]bitbucket.Status, error)
	ApprovePullRequest(ctx context.Context, prURL string) error
	RequestChanges(ctx context.Context, prURL string) error
	ResolveCommentThread(ctx context.Context, prURL string, commentID int) error
	ReopenCommentThread(ctx context.Context, prURL string, commentID int) error
}
