package mcp

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kidboy-man/bitbucket-mcp/internal/diff"
	"github.com/kidboy-man/bitbucket-mcp/internal/review"
)

// registerLegacyTools registers the original four tools.
// They are kept for backwards compatibility.
// Deprecated: prefer get_pr_context, get_pr_diff, and draft_review_comments for new workflows.
func registerLegacyTools(server *sdkmcp.Server, svc *review.Service) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_pr",
		Description: "Deprecated: prefer get_pr_context for new workflows. Fetch a Bitbucket PR's metadata, structured diff (with DiffPosition per line), and commits.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input getPRInput) (*sdkmcp.CallToolResult, *getPROutput, error) {
		return handleGetPR(ctx, svc, input)
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "list_pr_comments",
		Description: "Deprecated: prefer get_pr_context for new workflows. List existing inline comments on a PR.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input prURLInput) (*sdkmcp.CallToolResult, *listCommentsOutput, error) {
		return handleListComments(ctx, svc, input)
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "post_inline_comment",
		Description: "Deprecated: prefer draft_review_comments + post_review_comments for new workflows. Post an inline comment to Bitbucket. Use new_line_no from get_pr output to anchor the comment.",
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
		},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input postCommentInput) (*sdkmcp.CallToolResult, *postCommentOutput, error) {
		return handlePostComment(ctx, svc, input)
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_pr_review_comments_with_context",
		Description: "Deprecated: prefer get_pr_context for new workflows. Fetch all inline review comments on a PR, each enriched with ±5 surrounding diff lines.",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input prURLInput) (*sdkmcp.CallToolResult, *commentsWithContextOutput, error) {
		return handleCommentsWithContext(ctx, svc, input)
	})
}

// --- input/output types -----------------------------------------------------

type getPRInput struct {
	PRURL string `json:"pr_url" jsonschema:"description=Full Bitbucket PR URL"`
}

type getPROutput struct {
	PR   prMeta      `json:"pr"`
	Diff []fileDiff  `json:"diff"`
	Note string      `json:"note"`
}

type prMeta struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Author       string `json:"author"`
	SourceBranch string `json:"source_branch"`
	DestBranch   string `json:"dest_branch"`
	State        string `json:"state"`
}

type fileDiff struct {
	Path      string      `json:"path"`
	OldPath   string      `json:"old_path,omitempty"`
	IsNew     bool        `json:"is_new,omitempty"`
	IsDeleted bool        `json:"is_deleted,omitempty"`
	IsRenamed bool        `json:"is_renamed,omitempty"`
	Hunks     []hunkDiff  `json:"hunks"`
}

type hunkDiff struct {
	Header string     `json:"header"`
	Lines  []lineDiff `json:"lines"`
}

type lineDiff struct {
	DiffPosition int    `json:"diff_position"`
	OldLineNo    int    `json:"old_line_no,omitempty"`
	NewLineNo    int    `json:"new_line_no,omitempty"`
	Type         string `json:"type"`
	Content      string `json:"content"`
}

type prURLInput struct {
	PRURL string `json:"pr_url" jsonschema:"description=Full Bitbucket PR URL"`
}

type listCommentsOutput struct {
	Comments []commentOut `json:"comments"`
}

type commentOut struct {
	ID        int    `json:"id"`
	Body      string `json:"body"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Author    string `json:"author"`
	CreatedOn string `json:"created_on"`
}

type postCommentInput struct {
	PRURL     string `json:"pr_url"`
	File      string `json:"file"`
	NewLineNo int    `json:"new_line_no" jsonschema:"description=new_line_no from get_pr output"`
	Body      string `json:"body"`
}

type postCommentOutput struct {
	Message string `json:"message"`
}

type commentsWithContextOutput struct {
	Comments []commentWithContext `json:"comments"`
}

type commentWithContext struct {
	ID          int        `json:"id"`
	Author      string     `json:"author"`
	CreatedOn   string     `json:"created_on"`
	Body        string     `json:"body"`
	File        string     `json:"file"`
	Line        int        `json:"line"`
	DiffContext []lineDiff `json:"diff_context"`
}

// --- handlers ----------------------------------------------------------------

func handleGetPR(ctx context.Context, svc *review.Service, input getPRInput) (*sdkmcp.CallToolResult, *getPROutput, error) {
	if input.PRURL == "" {
		return errResult("pr_url is required"), nil, nil
	}

	prCtx, err := svc.GetPRContext(ctx, review.GetPRContextInput{
		PRURL:        input.PRURL,
		MaxDiffLines: 0, // no limit for legacy tool
	})
	if err != nil {
		return errResult(fmt.Sprintf("get_pr failed: %v", err)), nil, nil
	}

	out := &getPROutput{
		PR: prMeta{
			ID:           prCtx.PR.ID,
			Title:        prCtx.PR.Title,
			Description:  prCtx.PR.Description,
			Author:       prCtx.PR.Author,
			SourceBranch: prCtx.PR.SourceBranch,
			DestBranch:   prCtx.PR.DestBranch,
			State:        prCtx.PR.State,
		},
		Note: "Use new_line_no (not diff_position) when calling post_inline_comment.",
	}

	// Rebuild fileDiff from DiffPage.
	if prCtx.Diff != nil {
		for _, f := range prCtx.Diff.Files {
			fd := fileDiff{
				Path:      f.Path,
				OldPath:   f.OldPath,
				IsNew:     f.IsNew,
				IsDeleted: f.IsDeleted,
				IsRenamed: f.IsRenamed,
			}
			for _, h := range f.Hunks {
				hd := hunkDiff{Header: h.Header}
				for _, l := range h.Lines {
					hd.Lines = append(hd.Lines, lineDiff{
						DiffPosition: l.DiffPosition,
						OldLineNo:    l.OldLineNo,
						NewLineNo:    l.NewLineNo,
						Type:         l.Type,
						Content:      l.Content,
					})
				}
				fd.Hunks = append(fd.Hunks, hd)
			}
			out.Diff = append(out.Diff, fd)
		}
	}

	return nil, out, nil
}

func handleListComments(ctx context.Context, svc *review.Service, input prURLInput) (*sdkmcp.CallToolResult, *listCommentsOutput, error) {
	if input.PRURL == "" {
		return errResult("pr_url is required"), nil, nil
	}
	prCtx, err := svc.GetPRContext(ctx, review.GetPRContextInput{PRURL: input.PRURL, MaxDiffLines: 1})
	if err != nil {
		return errResult(fmt.Sprintf("list_pr_comments failed: %v", err)), nil, nil
	}

	out := &listCommentsOutput{}
	for _, c := range prCtx.Comments {
		// Legacy tool returns only inline comments.
		if c.File == "" {
			continue
		}
		out.Comments = append(out.Comments, commentOut{
			ID:        c.ID,
			Body:      c.Body,
			File:      c.File,
			Line:      c.Line,
			Author:    c.Author,
			CreatedOn: c.CreatedOn,
		})
	}
	return nil, out, nil
}

func handlePostComment(ctx context.Context, svc *review.Service, input postCommentInput) (*sdkmcp.CallToolResult, *postCommentOutput, error) {
	if input.PRURL == "" || input.File == "" || input.NewLineNo == 0 || input.Body == "" {
		return errResult("pr_url, file, new_line_no, and body are required"), nil, nil
	}

	draft, err := svc.DraftReviewComments(ctx, review.DraftReviewInput{
		PRURL: input.PRURL,
		Findings: []review.ReviewFinding{
			{File: input.File, NewLineNo: input.NewLineNo, Body: input.Body},
		},
	})
	if err != nil {
		return errResult(fmt.Sprintf("draft failed: %v", err)), nil, nil
	}

	if len(draft.CommentsToPost) == 0 {
		reason := "unknown"
		if len(draft.Skipped) > 0 {
			reason = draft.Skipped[0].Reason
		} else if len(draft.Duplicates) > 0 {
			reason = "duplicate: " + draft.Duplicates[0].Reason
		}
		return errResult(fmt.Sprintf("comment not queued: %s", reason)), nil, nil
	}

	result, err := svc.PostReviewComments(ctx, *draft)
	if err != nil {
		return errResult(fmt.Sprintf("post failed: %v", err)), nil, nil
	}

	if len(result.Failed) > 0 {
		return errResult(fmt.Sprintf("post failed: %s", result.Failed[0].Error)), nil, nil
	}

	return nil, &postCommentOutput{
		Message: fmt.Sprintf("comment posted on %s:%d", input.File, input.NewLineNo),
	}, nil
}

func handleCommentsWithContext(ctx context.Context, svc *review.Service, input prURLInput) (*sdkmcp.CallToolResult, *commentsWithContextOutput, error) {
	if input.PRURL == "" {
		return errResult("pr_url is required"), nil, nil
	}

	prCtx, err := svc.GetPRContext(ctx, review.GetPRContextInput{PRURL: input.PRURL, MaxDiffLines: 0})
	if err != nil {
		return errResult(fmt.Sprintf("get_pr_review_comments_with_context failed: %v", err)), nil, nil
	}

	out := &commentsWithContextOutput{}

	for _, c := range prCtx.Comments {
		if c.File == "" {
			continue
		}
		cwc := commentWithContext{
			ID:          c.ID,
			Author:      c.Author,
			CreatedOn:   c.CreatedOn,
			Body:        c.Body,
			File:        c.File,
			Line:        c.Line,
			DiffContext: []lineDiff{},
		}

		// Look up context from diff page.
		if prCtx.Diff != nil {
			ctxLines := getContextFromDiffPage(prCtx.Diff, c.File, c.Line, 5)
			cwc.DiffContext = ctxLines
		}

		out.Comments = append(out.Comments, cwc)
	}
	return nil, out, nil
}

// getContextFromDiffPage extracts ±contextLines around newLineNo from a DiffPage.
func getContextFromDiffPage(page *diff.DiffPage, filePath string, newLineNo, contextLines int) []lineDiff {
	for _, f := range page.Files {
		if f.Path != filePath {
			continue
		}
		for _, h := range f.Hunks {
			anchorIdx := -1
			for i, l := range h.Lines {
				if l.NewLineNo == newLineNo && l.Type != "removed" {
					anchorIdx = i
					break
				}
			}
			if anchorIdx == -1 {
				continue
			}
			start := anchorIdx - contextLines
			if start < 0 {
				start = 0
			}
			end := anchorIdx + contextLines + 1
			if end > len(h.Lines) {
				end = len(h.Lines)
			}
			result := make([]lineDiff, 0, end-start)
			for _, l := range h.Lines[start:end] {
				result = append(result, lineDiff{
					DiffPosition: l.DiffPosition,
					OldLineNo:    l.OldLineNo,
					NewLineNo:    l.NewLineNo,
					Type:         l.Type,
					Content:      l.Content,
				})
			}
			return result
		}
	}
	return []lineDiff{}
}

