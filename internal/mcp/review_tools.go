package mcp

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kidboy-man/bitbucket-mcp/internal/diff"
	"github.com/kidboy-man/bitbucket-mcp/internal/review"
)

func registerReviewTools(server *sdkmcp.Server, svc *review.Service) {
	// --- read tools ---

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_pr_context",
		Description: "Return review-focused aggregate context: PR metadata, commits, comments, tasks, build statuses summary, and first diff page.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input getPRContextInput) (*sdkmcp.CallToolResult, *review.PRContext, error) {
		if input.PRURL == "" {
			return errResult("pr_url is required"), nil, nil
		}
		result, err := svc.GetPRContext(ctx, review.GetPRContextInput{
			PRURL:        input.PRURL,
			MaxDiffLines: input.MaxDiffLines,
			MaxFiles:     input.MaxFiles,
			DiffCursor:   input.DiffCursor,
		})
		if err != nil {
			return errResult(fmt.Sprintf("get_pr_context failed: %v", err)), nil, nil
		}
		return nil, result, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_pr_diff",
		Description: "Return a paginated structured diff. Use cursor for subsequent pages. Filter by file when needed.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input getPRDiffInput) (*sdkmcp.CallToolResult, *diff.DiffPage, error) {
		if input.PRURL == "" {
			return errResult("pr_url is required"), nil, nil
		}
		result, err := svc.GetPRDiff(ctx, review.GetPRDiffInput{
			PRURL:    input.PRURL,
			File:     input.File,
			Cursor:   input.Cursor,
			MaxLines: input.MaxLines,
		})
		if err != nil {
			return errResult(fmt.Sprintf("get_pr_diff failed: %v", err)), nil, nil
		}
		return nil, result, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "list_pr_tasks",
		Description: "List tasks on a pull request.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input prURLInput) (*sdkmcp.CallToolResult, *listTasksOutput, error) {
		if input.PRURL == "" {
			return errResult("pr_url is required"), nil, nil
		}
		prCtx, err := svc.GetPRContext(ctx, review.GetPRContextInput{PRURL: input.PRURL, MaxDiffLines: 1})
		if err != nil {
			return errResult(fmt.Sprintf("list_pr_tasks failed: %v", err)), nil, nil
		}
		out := &listTasksOutput{}
		for _, t := range prCtx.Tasks {
			out.Tasks = append(out.Tasks, taskOut{
				ID:         t.ID,
				State:      t.State,
				Body:       t.Body,
				Author:     t.Author,
				CreatedOn:  t.CreatedOn,
				ResolvedOn: t.ResolvedOn,
				ResolvedBy: t.ResolvedBy,
			})
		}
		return nil, out, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "list_pr_statuses",
		Description: "List build/pipeline statuses for a pull request's source commit.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input prURLInput) (*sdkmcp.CallToolResult, *listStatusesOutput, error) {
		if input.PRURL == "" {
			return errResult("pr_url is required"), nil, nil
		}
		prCtx, err := svc.GetPRContext(ctx, review.GetPRContextInput{PRURL: input.PRURL, MaxDiffLines: 1})
		if err != nil {
			return errResult(fmt.Sprintf("list_pr_statuses failed: %v", err)), nil, nil
		}
		return nil, &listStatusesOutput{Summary: prCtx.StatusesSummary}, nil
	})

	// --- draft/post workflow tools ---

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "draft_review_comments",
		Description: "Validate review findings and produce a stateless posting plan. No writes are performed. " +
			"Pass the returned draft to post_review_comments to post.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input draftReviewInput) (*sdkmcp.CallToolResult, *review.ReviewDraft, error) {
		if input.PRURL == "" {
			return errResult("pr_url is required"), nil, nil
		}
		findings := make([]review.ReviewFinding, 0, len(input.Findings))
		for _, f := range input.Findings {
			findings = append(findings, review.ReviewFinding{
				File:       f.File,
				NewLineNo:  f.NewLineNo,
				Body:       f.Body,
				Severity:   f.Severity,
				CreateTask: f.CreateTask,
			})
		}
		draft, err := svc.DraftReviewComments(ctx, review.DraftReviewInput{
			PRURL:          input.PRURL,
			Findings:       findings,
			SummaryBody:    input.SummaryBody,
			SummaryVerdict: input.SummaryVerdict,
		})
		if err != nil {
			return errResult(fmt.Sprintf("draft_review_comments failed: %v", err)), nil, nil
		}
		return nil, draft, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "post_review_comments",
		Description: "Post an approved ReviewDraft produced by draft_review_comments. " +
			"Performs stale-commit guard before writing anything. Returns partial success results.",
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
		},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input postReviewInput) (*sdkmcp.CallToolResult, *review.PostReviewResult, error) {
		result, err := svc.PostReviewComments(ctx, input.Draft)
		if err != nil {
			return errResult(fmt.Sprintf("post_review_comments failed: %v", err)), nil, nil
		}
		return nil, result, nil
	})

	// --- explicit state tools ---

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "approve_pr",
		Description: "Approve a pull request. Requires expected_source_commit_hash to guard against stale commits.",
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
		},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input approvePRInput) (*sdkmcp.CallToolResult, *review.ApprovePRResult, error) {
		if input.PRURL == "" || input.ExpectedSourceCommitHash == "" {
			return errResult("pr_url and expected_source_commit_hash are required"), nil, nil
		}
		result, err := svc.ApprovePR(ctx, review.ApprovePRInput{
			PRURL:                    input.PRURL,
			ExpectedSourceCommitHash: input.ExpectedSourceCommitHash,
		})
		if err != nil {
			return errResult(fmt.Sprintf("approve_pr failed: %v", err)), nil, nil
		}
		return nil, result, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "request_changes_pr",
		Description: "Request changes on a pull request. Requires expected_source_commit_hash to guard against stale commits.",
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
		},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input requestChangesInput) (*sdkmcp.CallToolResult, *review.RequestChangesResult, error) {
		if input.PRURL == "" || input.ExpectedSourceCommitHash == "" {
			return errResult("pr_url and expected_source_commit_hash are required"), nil, nil
		}
		result, err := svc.RequestChangesPR(ctx, review.RequestChangesInput{
			PRURL:                    input.PRURL,
			ExpectedSourceCommitHash: input.ExpectedSourceCommitHash,
		})
		if err != nil {
			return errResult(fmt.Sprintf("request_changes_pr failed: %v", err)), nil, nil
		}
		return nil, result, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "resolve_task",
		Description: "Mark a PR task as resolved.",
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
		},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input taskActionInput) (*sdkmcp.CallToolResult, *review.TaskResult, error) {
		if input.PRURL == "" || input.TaskID == 0 {
			return errResult("pr_url and task_id are required"), nil, nil
		}
		result, err := svc.ResolveTask(ctx, review.ResolveTaskInput{PRURL: input.PRURL, TaskID: input.TaskID})
		if err != nil {
			return errResult(fmt.Sprintf("resolve_task failed: %v", err)), nil, nil
		}
		return nil, result, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "reopen_task",
		Description: "Reopen a resolved PR task.",
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
		},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input taskActionInput) (*sdkmcp.CallToolResult, *review.TaskResult, error) {
		if input.PRURL == "" || input.TaskID == 0 {
			return errResult("pr_url and task_id are required"), nil, nil
		}
		result, err := svc.ReopenTask(ctx, review.ReopenTaskInput{PRURL: input.PRURL, TaskID: input.TaskID})
		if err != nil {
			return errResult(fmt.Sprintf("reopen_task failed: %v", err)), nil, nil
		}
		return nil, result, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "resolve_comment_thread",
		Description: "Mark a PR comment thread as resolved.",
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
		},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input commentActionInput) (*sdkmcp.CallToolResult, *actionResult, error) {
		if input.PRURL == "" || input.CommentID == 0 {
			return errResult("pr_url and comment_id are required"), nil, nil
		}
		if err := svc.ResolveCommentThread(ctx, review.ResolveCommentThreadInput{
			PRURL:     input.PRURL,
			CommentID: input.CommentID,
		}); err != nil {
			return errResult(fmt.Sprintf("resolve_comment_thread failed: %v", err)), nil, nil
		}
		return nil, &actionResult{Success: true}, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "reopen_comment_thread",
		Description: "Reopen a resolved PR comment thread.",
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
		},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input commentActionInput) (*sdkmcp.CallToolResult, *actionResult, error) {
		if input.PRURL == "" || input.CommentID == 0 {
			return errResult("pr_url and comment_id are required"), nil, nil
		}
		if err := svc.ReopenCommentThread(ctx, review.ReopenCommentThreadInput{
			PRURL:     input.PRURL,
			CommentID: input.CommentID,
		}); err != nil {
			return errResult(fmt.Sprintf("reopen_comment_thread failed: %v", err)), nil, nil
		}
		return nil, &actionResult{Success: true}, nil
	})
}

// --- input/output types -----------------------------------------------------

type getPRContextInput struct {
	PRURL        string `json:"pr_url"`
	MaxDiffLines int    `json:"max_diff_lines,omitempty"`
	MaxFiles     int    `json:"max_files,omitempty"`
	DiffCursor   string `json:"diff_cursor,omitempty"`
}

type getPRDiffInput struct {
	PRURL    string `json:"pr_url"`
	File     string `json:"file,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
	MaxLines int    `json:"max_lines,omitempty"`
}

type listTasksOutput struct {
	Tasks []taskOut `json:"tasks"`
}

type taskOut struct {
	ID         int    `json:"id"`
	State      string `json:"state"`
	Body       string `json:"body"`
	Author     string `json:"author"`
	CreatedOn  string `json:"created_on"`
	ResolvedOn string `json:"resolved_on,omitempty"`
	ResolvedBy string `json:"resolved_by,omitempty"`
}

type listStatusesOutput struct {
	Summary interface{} `json:"summary"`
}

type findingInput struct {
	File       string `json:"file"`
	NewLineNo  int    `json:"new_line_no"`
	Body       string `json:"body"`
	Severity   string `json:"severity,omitempty"`
	CreateTask bool   `json:"create_task,omitempty"`
}

type draftReviewInput struct {
	PRURL          string         `json:"pr_url"`
	Findings       []findingInput `json:"findings"`
	SummaryBody    string         `json:"summary_body,omitempty"`
	SummaryVerdict string         `json:"summary_verdict,omitempty"`
}

type postReviewInput struct {
	Draft review.ReviewDraft `json:"draft"`
}

type approvePRInput struct {
	PRURL                    string `json:"pr_url"`
	ExpectedSourceCommitHash string `json:"expected_source_commit_hash"`
}

type requestChangesInput struct {
	PRURL                    string `json:"pr_url"`
	ExpectedSourceCommitHash string `json:"expected_source_commit_hash"`
}

type taskActionInput struct {
	PRURL  string `json:"pr_url"`
	TaskID int    `json:"task_id"`
}

type commentActionInput struct {
	PRURL     string `json:"pr_url"`
	CommentID int    `json:"comment_id"`
}

type actionResult struct {
	Success bool `json:"success"`
}
