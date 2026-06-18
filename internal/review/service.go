package review

import (
	"context"
	"fmt"
	"strings"

	"github.com/kidboy-man/bitbucket-mcp/internal/bitbucket"
	"github.com/kidboy-man/bitbucket-mcp/internal/diff"
)

var validSeverities = map[string]bool{"HIGH": true, "MEDIUM": true, "LOW": true}
var validVerdicts = map[string]bool{"PASS": true, "COMMENT": true, "BLOCK": true}

// Service is the application/usecase layer for PR review automation.
type Service struct {
	store PullRequestStore
}

// NewService constructs a Service.
func NewService(store PullRequestStore) *Service {
	return &Service{store: store}
}

// GetPRContext returns a review-focused aggregate: PR metadata, commits, comments,
// tasks, statuses, and the first diff page.
func (s *Service) GetPRContext(ctx context.Context, input GetPRContextInput) (*PRContext, error) {
	pr, err := s.store.GetPullRequest(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("get PR: %w", err)
	}

	commits, err := s.store.ListPullRequestCommits(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("list commits: %w", err)
	}

	comments, err := s.store.ListPullRequestComments(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}

	tasks, err := s.store.ListPullRequestTasks(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	statuses, err := s.store.ListPullRequestStatuses(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("list statuses: %w", err)
	}

	rawDiff, err := s.store.GetPullRequestDiff(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("get diff: %w", err)
	}

	pd, err := diff.Parse(rawDiff)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	maxLines := input.MaxDiffLines
	if maxLines == 0 {
		maxLines = 500
	}

	diffPage, err := pd.GetPage(diff.PageInput{
		PRURL:    input.PRURL,
		Cursor:   input.DiffCursor,
		MaxLines: maxLines,
	})
	if err != nil {
		return nil, fmt.Errorf("paginate diff: %w", err)
	}

	summary := buildStatusesSummary(statuses)

	return &PRContext{
		PR:              pr,
		Commits:         commits,
		Comments:        comments,
		Tasks:           tasks,
		StatusesSummary: summary,
		Diff:            diffPage,
	}, nil
}

// GetPRDiff returns a paginated structured diff page.
func (s *Service) GetPRDiff(ctx context.Context, input GetPRDiffInput) (*diff.DiffPage, error) {
	rawDiff, err := s.store.GetPullRequestDiff(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("get diff: %w", err)
	}

	pd, err := diff.Parse(rawDiff)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	maxLines := input.MaxLines
	if maxLines == 0 {
		maxLines = 500
	}

	page, err := pd.GetPage(diff.PageInput{
		PRURL:      input.PRURL,
		FileFilter: input.File,
		Cursor:     input.Cursor,
		MaxLines:   maxLines,
	})
	if err != nil {
		return nil, fmt.Errorf("paginate diff: %w", err)
	}
	return page, nil
}

// DraftReviewComments validates findings against the diff and existing comments,
// producing a stateless ReviewDraft. No writes are performed.
func (s *Service) DraftReviewComments(ctx context.Context, input DraftReviewInput) (*ReviewDraft, error) {
	// Validate enum inputs first.
	if input.SummaryVerdict != "" && !validVerdicts[input.SummaryVerdict] {
		return nil, fmt.Errorf("invalid summary_verdict %q: must be PASS, COMMENT, or BLOCK", input.SummaryVerdict)
	}
	for i, f := range input.Findings {
		if f.Severity != "" && !validSeverities[f.Severity] {
			return nil, fmt.Errorf("finding[%d]: invalid severity %q: must be HIGH, MEDIUM, or LOW", i, f.Severity)
		}
	}

	// Fetch PR to get source commit hash.
	pr, err := s.store.GetPullRequest(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("get PR: %w", err)
	}

	// Fetch and parse diff.
	rawDiff, err := s.store.GetPullRequestDiff(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("get diff: %w", err)
	}
	pd, err := diff.Parse(rawDiff)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	// Fetch existing comments for deduplication.
	existing, err := s.store.ListPullRequestComments(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}

	draft := &ReviewDraft{
		PRURL:                   input.PRURL,
		SourceCommitHash:        pr.SourceCommitHash,
		ExistingCommentsChecked: true,
	}

	if input.SummaryBody != "" {
		verdict := input.SummaryVerdict
		if verdict == "" {
			verdict = "COMMENT"
		}
		draft.SummaryComment = &DraftSummary{
			Body:    input.SummaryBody,
			Verdict: verdict,
		}
	}

	for i, f := range input.Findings {
		// Skip empty body.
		if strings.TrimSpace(f.Body) == "" {
			draft.Skipped = append(draft.Skipped, SkippedFinding{
				Index:   i,
				Reason:  "empty_body",
				Finding: f,
			})
			continue
		}

		// Validate anchor: file must be in diff.
		_, anchorErr := pd.FindDiffPosition(f.File, f.NewLineNo)
		if anchorErr != nil {
			draft.Skipped = append(draft.Skipped, SkippedFinding{
				Index:   i,
				Reason:  "line_not_in_diff",
				Finding: f,
			})
			continue
		}

		// Check that the line is not a removed line (cannot anchor inline.to there).
		if isRemovedLine(pd, f.File, f.NewLineNo) {
			draft.Skipped = append(draft.Skipped, SkippedFinding{
				Index:   i,
				Reason:  "removed_line_cannot_be_anchor",
				Finding: f,
			})
			continue
		}

		// Deduplication: same file + same line + normalized body.
		normalizedBody := normalizeBody(f.Body)
		if isDuplicate(existing, f.File, f.NewLineNo, normalizedBody) {
			draft.Duplicates = append(draft.Duplicates, DuplicateFinding{
				Index:   i,
				Reason:  "same_file_line_body",
				Finding: f,
			})
			continue
		}

		idx := len(draft.CommentsToPost)
		draft.CommentsToPost = append(draft.CommentsToPost, DraftComment{
			File:      f.File,
			NewLineNo: f.NewLineNo,
			Body:      f.Body,
			Severity:  f.Severity,
		})

		if f.CreateTask {
			draft.TasksToCreate = append(draft.TasksToCreate, DraftTask{
				CommentIndex: idx,
				Body:         f.Body,
			})
		}
	}

	return draft, nil
}

// PostReviewComments posts the approved draft. Performs stale-commit guard first.
// On partial failure, continues posting and returns all results.
func (s *Service) PostReviewComments(ctx context.Context, draft ReviewDraft) (*PostReviewResult, error) {
	// Stale-commit guard.
	pr, err := s.store.GetPullRequest(ctx, draft.PRURL)
	if err != nil {
		return nil, fmt.Errorf("get PR for stale check: %w", err)
	}
	if pr.SourceCommitHash != draft.SourceCommitHash {
		return nil, fmt.Errorf("source commit changed: expected %s, got %s",
			draft.SourceCommitHash, pr.SourceCommitHash)
	}

	result := &PostReviewResult{}

	// Map comment index → created comment ID (for task linking).
	commentIDs := make(map[int]int)

	for i, dc := range draft.CommentsToPost {
		cm, err := s.store.CreatePullRequestComment(ctx, draft.PRURL, bitbucket.CreateCommentInput{
			Body:     dc.Body,
			FilePath: dc.File,
			NewLine:  dc.NewLineNo,
		})
		if err != nil {
			result.Failed = append(result.Failed, PostFailure{
				Kind:  "comment",
				Index: i,
				Error: err.Error(),
			})
			continue
		}
		commentIDs[i] = cm.ID
		result.PostedComments = append(result.PostedComments, *cm)
	}

	for _, dt := range draft.TasksToCreate {
		input := bitbucket.CreateTaskInput{Body: dt.Body}
		if id, ok := commentIDs[dt.CommentIndex]; ok {
			input.CommentID = id
		}
		task, err := s.store.CreatePullRequestTask(ctx, draft.PRURL, input)
		if err != nil {
			result.Failed = append(result.Failed, PostFailure{
				Kind:  "task",
				Index: dt.CommentIndex,
				Error: err.Error(),
			})
			continue
		}
		result.CreatedTasks = append(result.CreatedTasks, *task)
	}

	if draft.SummaryComment != nil {
		cm, err := s.store.CreatePullRequestComment(ctx, draft.PRURL, bitbucket.CreateCommentInput{
			Body: draft.SummaryComment.Body,
		})
		if err != nil {
			result.Failed = append(result.Failed, PostFailure{
				Kind:  "summary",
				Index: 0,
				Error: err.Error(),
			})
		} else {
			result.PostedSummary = cm
		}
	}

	return result, nil
}

// ApprovePR approves a PR after confirming the source commit hash.
func (s *Service) ApprovePR(ctx context.Context, input ApprovePRInput) (*ApprovePRResult, error) {
	pr, err := s.store.GetPullRequest(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("get PR: %w", err)
	}
	if pr.SourceCommitHash != input.ExpectedSourceCommitHash {
		return nil, fmt.Errorf("source commit changed: expected %s, got %s",
			input.ExpectedSourceCommitHash, pr.SourceCommitHash)
	}
	if err := s.store.ApprovePullRequest(ctx, input.PRURL); err != nil {
		return nil, fmt.Errorf("approve PR: %w", err)
	}
	return &ApprovePRResult{Approved: true, SourceCommitHash: pr.SourceCommitHash}, nil
}

// RequestChangesPR requests changes on a PR after confirming the source commit hash.
func (s *Service) RequestChangesPR(ctx context.Context, input RequestChangesInput) (*RequestChangesResult, error) {
	pr, err := s.store.GetPullRequest(ctx, input.PRURL)
	if err != nil {
		return nil, fmt.Errorf("get PR: %w", err)
	}
	if pr.SourceCommitHash != input.ExpectedSourceCommitHash {
		return nil, fmt.Errorf("source commit changed: expected %s, got %s",
			input.ExpectedSourceCommitHash, pr.SourceCommitHash)
	}
	if err := s.store.RequestChanges(ctx, input.PRURL); err != nil {
		return nil, fmt.Errorf("request changes: %w", err)
	}
	return &RequestChangesResult{RequestedChanges: true, SourceCommitHash: pr.SourceCommitHash}, nil
}

// ResolveTask resolves a PR task.
func (s *Service) ResolveTask(ctx context.Context, input ResolveTaskInput) (*TaskResult, error) {
	task, err := s.store.UpdatePullRequestTask(ctx, input.PRURL, input.TaskID,
		bitbucket.UpdateTaskInput{State: "RESOLVED"})
	if err != nil {
		return nil, fmt.Errorf("resolve task: %w", err)
	}
	return &TaskResult{TaskID: task.ID, State: task.State}, nil
}

// ReopenTask reopens a PR task.
func (s *Service) ReopenTask(ctx context.Context, input ReopenTaskInput) (*TaskResult, error) {
	task, err := s.store.UpdatePullRequestTask(ctx, input.PRURL, input.TaskID,
		bitbucket.UpdateTaskInput{State: "UNRESOLVED"})
	if err != nil {
		return nil, fmt.Errorf("reopen task: %w", err)
	}
	return &TaskResult{TaskID: task.ID, State: task.State}, nil
}

// ResolveCommentThread resolves a PR comment thread.
func (s *Service) ResolveCommentThread(ctx context.Context, input ResolveCommentThreadInput) error {
	return s.store.ResolveCommentThread(ctx, input.PRURL, input.CommentID)
}

// ReopenCommentThread reopens a PR comment thread.
func (s *Service) ReopenCommentThread(ctx context.Context, input ReopenCommentThreadInput) error {
	return s.store.ReopenCommentThread(ctx, input.PRURL, input.CommentID)
}

// --- helpers ----------------------------------------------------------------

func buildStatusesSummary(statuses []bitbucket.Status) bitbucket.StatusesSummary {
	// For duplicate keys, keep latest by UpdatedOn.
	latest := map[string]bitbucket.Status{}
	for _, s := range statuses {
		key := s.Key
		if prev, ok := latest[key]; !ok || s.UpdatedOn > prev.UpdatedOn {
			latest[key] = s
		}
	}

	summary := bitbucket.StatusesSummary{Total: len(latest)}
	for _, s := range latest {
		switch s.State {
		case "SUCCESSFUL":
			summary.Successful++
		case "FAILED", "STOPPED":
			summary.Failed++
			summary.FailedStatuses = append(summary.FailedStatuses, s)
		case "INPROGRESS":
			summary.InProgress++
		}
	}
	return summary
}

func normalizeBody(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func isDuplicate(existing []bitbucket.Comment, file string, line int, normalizedBody string) bool {
	for _, c := range existing {
		if c.File == file && c.Line == line && normalizeBody(c.Body) == normalizedBody {
			return true
		}
	}
	return false
}

func isRemovedLine(pd *diff.ParsedDiff, file string, newLineNo int) bool {
	for _, f := range pd.Files {
		if f.Path != file {
			continue
		}
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if l.NewLineNo == newLineNo && l.Type == diff.LineRemoved {
					return true
				}
			}
		}
	}
	return false
}
