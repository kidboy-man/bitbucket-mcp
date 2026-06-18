package review

import (
	"context"
	"errors"
	"testing"

	"github.com/kidboy-man/bitbucket-mcp/internal/bitbucket"
)

// --- fake store -------------------------------------------------------------

type fakeStore struct {
	pr                *bitbucket.PullRequest
	rawDiff           string
	commits           []bitbucket.Commit
	comments          []bitbucket.Comment
	tasks             []bitbucket.Task
	statuses          []bitbucket.Status
	createdComments   []*bitbucket.Comment
	createdTasks      []*bitbucket.Task
	createdTaskInput  []bitbucket.CreateTaskInput
	commentErr        error
	taskErr           error
	approveErr        error
	requestChangesErr error
}

func (f *fakeStore) GetPullRequest(_ context.Context, _ string) (*bitbucket.PullRequest, error) {
	return f.pr, nil
}
func (f *fakeStore) GetPullRequestDiff(_ context.Context, _ string) (string, error) {
	return f.rawDiff, nil
}
func (f *fakeStore) ListPullRequestCommits(_ context.Context, _ string) ([]bitbucket.Commit, error) {
	return f.commits, nil
}
func (f *fakeStore) ListPullRequestComments(_ context.Context, _ string) ([]bitbucket.Comment, error) {
	return f.comments, nil
}
func (f *fakeStore) CreatePullRequestComment(_ context.Context, _ string, input bitbucket.CreateCommentInput) (*bitbucket.Comment, error) {
	if f.commentErr != nil {
		return nil, f.commentErr
	}
	idx := len(f.createdComments)
	cm := &bitbucket.Comment{ID: idx + 1, Body: input.Body, File: input.FilePath, Line: input.NewLine}
	f.createdComments = append(f.createdComments, cm)
	return cm, nil
}
func (f *fakeStore) ListPullRequestTasks(_ context.Context, _ string) ([]bitbucket.Task, error) {
	return f.tasks, nil
}
func (f *fakeStore) CreatePullRequestTask(_ context.Context, _ string, input bitbucket.CreateTaskInput) (*bitbucket.Task, error) {
	if f.taskErr != nil {
		return nil, f.taskErr
	}
	f.createdTaskInput = append(f.createdTaskInput, input)
	task := &bitbucket.Task{ID: 100, State: "UNRESOLVED", Body: input.Body}
	f.createdTasks = append(f.createdTasks, task)
	return task, nil
}
func (f *fakeStore) UpdatePullRequestTask(_ context.Context, _ string, taskID int, input bitbucket.UpdateTaskInput) (*bitbucket.Task, error) {
	return &bitbucket.Task{ID: taskID, State: input.State}, nil
}
func (f *fakeStore) ListPullRequestStatuses(_ context.Context, _ string) ([]bitbucket.Status, error) {
	return f.statuses, nil
}
func (f *fakeStore) ApprovePullRequest(_ context.Context, _ string) error {
	return f.approveErr
}
func (f *fakeStore) RequestChanges(_ context.Context, _ string) error {
	return f.requestChangesErr
}
func (f *fakeStore) ResolveCommentThread(_ context.Context, _ string, _ int) error { return nil }
func (f *fakeStore) ReopenCommentThread(_ context.Context, _ string, _ int) error  { return nil }

// --- sample diff ------------------------------------------------------------

const sampleDiff = `diff --git a/internal/handler/user.go b/internal/handler/user.go
index abc1234..def5678 100644
--- a/internal/handler/user.go
+++ b/internal/handler/user.go
@@ -10,7 +10,9 @@ import (
 func GetUser(ctx context.Context, id string) (*User, error) {
 	if id == "" {
 		return nil, errors.New("id is required")
+	}
+	if len(id) > 64 {
+		return nil, errors.New("id too long")
 	}
 	return db.Query(ctx, id)
 }
@@ -25,6 +27,5 @@ func DeleteUser(ctx context.Context, id string) error {
 	if err != nil {
 		return err
 	}
-	log.Printf("deleted user %s", id)
 	return nil
 }`

func newFake(commitHash string) *fakeStore {
	return &fakeStore{
		pr: &bitbucket.PullRequest{
			ID:               1,
			Title:            "Test PR",
			SourceCommitHash: commitHash,
		},
		rawDiff: sampleDiff,
	}
}

const prURL = "https://bitbucket.org/ws/repo/pull-requests/1"

// --- GetPRContext tests -----------------------------------------------------

func TestGetPRContext_Aggregates(t *testing.T) {
	store := newFake("abc123")
	store.commits = []bitbucket.Commit{{Hash: "abc123", Message: "init"}}
	store.tasks = []bitbucket.Task{{ID: 1, State: "UNRESOLVED", Body: "todo"}}
	store.statuses = []bitbucket.Status{{Key: "ci", State: "SUCCESSFUL"}}

	svc := NewService(store)
	ctx := context.Background()
	result, err := svc.GetPRContext(ctx, GetPRContextInput{PRURL: prURL})
	if err != nil {
		t.Fatalf("GetPRContext: %v", err)
	}
	if result.PR == nil {
		t.Error("PR nil")
	}
	if len(result.Commits) != 1 {
		t.Errorf("commits len = %d, want 1", len(result.Commits))
	}
	if len(result.Tasks) != 1 {
		t.Errorf("tasks len = %d, want 1", len(result.Tasks))
	}
	if result.Diff == nil {
		t.Error("diff nil")
	}
	if result.StatusesSummary.Successful != 1 {
		t.Errorf("successful statuses = %d, want 1", result.StatusesSummary.Successful)
	}
}

// --- DraftReviewComments tests ----------------------------------------------

func TestDraftReviewComments_AcceptsValidFinding(t *testing.T) {
	store := newFake("abc")
	svc := NewService(store)
	draft, err := svc.DraftReviewComments(context.Background(), DraftReviewInput{
		PRURL: prURL,
		Findings: []ReviewFinding{
			{File: "internal/handler/user.go", NewLineNo: 13, Body: "Use fmt.Errorf.", Severity: "HIGH"},
		},
	})
	if err != nil {
		t.Fatalf("DraftReviewComments: %v", err)
	}
	if len(draft.CommentsToPost) != 1 {
		t.Errorf("comments_to_post len = %d, want 1", len(draft.CommentsToPost))
	}
	if len(draft.Skipped) != 0 {
		t.Errorf("skipped len = %d, want 0", len(draft.Skipped))
	}
}

func TestDraftReviewComments_SkipsLineNotInDiff(t *testing.T) {
	store := newFake("abc")
	svc := NewService(store)
	draft, err := svc.DraftReviewComments(context.Background(), DraftReviewInput{
		PRURL: prURL,
		Findings: []ReviewFinding{
			{File: "internal/handler/user.go", NewLineNo: 9999, Body: "comment", Severity: "LOW"},
		},
	})
	if err != nil {
		t.Fatalf("DraftReviewComments: %v", err)
	}
	if len(draft.Skipped) != 1 {
		t.Fatalf("skipped len = %d, want 1", len(draft.Skipped))
	}
	if draft.Skipped[0].Reason != "line_not_in_diff" {
		t.Errorf("skip reason = %q, want line_not_in_diff", draft.Skipped[0].Reason)
	}
}

func TestDraftReviewComments_SkipsEmptyBody(t *testing.T) {
	store := newFake("abc")
	svc := NewService(store)
	draft, err := svc.DraftReviewComments(context.Background(), DraftReviewInput{
		PRURL: prURL,
		Findings: []ReviewFinding{
			{File: "internal/handler/user.go", NewLineNo: 13, Body: "   ", Severity: "LOW"},
		},
	})
	if err != nil {
		t.Fatalf("DraftReviewComments: %v", err)
	}
	if len(draft.Skipped) != 1 {
		t.Fatalf("skipped len = %d, want 1", len(draft.Skipped))
	}
	if draft.Skipped[0].Reason != "empty_body" {
		t.Errorf("skip reason = %q, want empty_body", draft.Skipped[0].Reason)
	}
}

func TestDraftReviewComments_ReportsDuplicateExistingComment(t *testing.T) {
	store := newFake("abc")
	store.comments = []bitbucket.Comment{
		{File: "internal/handler/user.go", Line: 13, Body: "Use fmt.Errorf."},
	}
	svc := NewService(store)
	draft, err := svc.DraftReviewComments(context.Background(), DraftReviewInput{
		PRURL: prURL,
		Findings: []ReviewFinding{
			{File: "internal/handler/user.go", NewLineNo: 13, Body: "Use fmt.Errorf.", Severity: "MEDIUM"},
		},
	})
	if err != nil {
		t.Fatalf("DraftReviewComments: %v", err)
	}
	if len(draft.Duplicates) != 1 {
		t.Fatalf("duplicates len = %d, want 1", len(draft.Duplicates))
	}
	if draft.Duplicates[0].Reason != "same_file_line_body" {
		t.Errorf("duplicate reason = %q, want same_file_line_body", draft.Duplicates[0].Reason)
	}
}

func TestDraftReviewComments_RejectsInvalidSeverity(t *testing.T) {
	store := newFake("abc")
	svc := NewService(store)
	_, err := svc.DraftReviewComments(context.Background(), DraftReviewInput{
		PRURL: prURL,
		Findings: []ReviewFinding{
			{File: "internal/handler/user.go", NewLineNo: 13, Body: "comment", Severity: "CRITICAL"},
		},
	})
	if err == nil {
		t.Error("expected error for invalid severity")
	}
}

func TestDraftReviewComments_RejectsInvalidSummaryVerdict(t *testing.T) {
	store := newFake("abc")
	svc := NewService(store)
	_, err := svc.DraftReviewComments(context.Background(), DraftReviewInput{
		PRURL:          prURL,
		SummaryVerdict: "REJECT",
	})
	if err == nil {
		t.Error("expected error for invalid summary verdict")
	}
}

// --- PostReviewComments tests -----------------------------------------------

func TestPostReviewComments_RejectsStaleSourceCommit(t *testing.T) {
	store := newFake("current-hash")
	svc := NewService(store)

	draft := ReviewDraft{
		PRURL:            prURL,
		SourceCommitHash: "old-hash", // mismatches store's "current-hash"
		CommentsToPost: []DraftComment{
			{File: "internal/handler/user.go", NewLineNo: 13, Body: "fix it"},
		},
	}
	_, err := svc.PostReviewComments(context.Background(), draft)
	if err == nil {
		t.Fatal("expected stale commit error")
	}
	// Confirm no comments were created.
	if len(store.createdComments) != 0 {
		t.Errorf("expected 0 comments posted before rejection, got %d", len(store.createdComments))
	}
}

func TestPostReviewComments_PostsSequentially(t *testing.T) {
	store := newFake("abc")
	svc := NewService(store)

	draft := ReviewDraft{
		PRURL:            prURL,
		SourceCommitHash: "abc",
		CommentsToPost: []DraftComment{
			{File: "internal/handler/user.go", NewLineNo: 13, Body: "first"},
			{File: "internal/handler/user.go", NewLineNo: 14, Body: "second"},
		},
	}
	result, err := svc.PostReviewComments(context.Background(), draft)
	if err != nil {
		t.Fatalf("PostReviewComments: %v", err)
	}
	if len(result.PostedComments) != 2 {
		t.Errorf("posted comments = %d, want 2", len(result.PostedComments))
	}
}

func TestPostReviewComments_ContinuesAfterOneFailure(t *testing.T) {
	store := newFake("abc")
	callCount := 0
	store.commentErr = nil
	// Override CreatePullRequestComment to fail on first call only.
	failOnce := &failOnceStore{fakeStore: store}
	svc := NewService(failOnce)

	draft := ReviewDraft{
		PRURL:            prURL,
		SourceCommitHash: "abc",
		CommentsToPost: []DraftComment{
			{File: "internal/handler/user.go", NewLineNo: 13, Body: "fail"},
			{File: "internal/handler/user.go", NewLineNo: 14, Body: "ok"},
		},
	}
	result, err := svc.PostReviewComments(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = callCount
	if len(result.Failed) != 1 {
		t.Errorf("failed = %d, want 1", len(result.Failed))
	}
	if len(result.PostedComments) != 1 {
		t.Errorf("posted = %d, want 1", len(result.PostedComments))
	}
}

func TestPostReviewComments_CreatesTaskOnlyWhenRequested(t *testing.T) {
	store := newFake("abc")
	svc := NewService(store)

	draft := ReviewDraft{
		PRURL:            prURL,
		SourceCommitHash: "abc",
		CommentsToPost: []DraftComment{
			{File: "internal/handler/user.go", NewLineNo: 13, Body: "fix it"},
		},
		TasksToCreate: []DraftTask{
			{CommentIndex: 0, Body: "fix it"},
		},
	}
	result, err := svc.PostReviewComments(context.Background(), draft)
	if err != nil {
		t.Fatalf("PostReviewComments: %v", err)
	}
	if len(result.CreatedTasks) != 1 {
		t.Errorf("created tasks = %d, want 1", len(result.CreatedTasks))
	}
	if len(store.createdTaskInput) != 1 {
		t.Fatalf("created task inputs = %d, want 1", len(store.createdTaskInput))
	}
	if store.createdTaskInput[0].CommentID != result.PostedComments[0].ID {
		t.Errorf("task comment ID = %d, want %d", store.createdTaskInput[0].CommentID, result.PostedComments[0].ID)
	}
}

// --- ApprovePR tests --------------------------------------------------------

func TestApprovePR_RejectsStaleCommit(t *testing.T) {
	store := newFake("current")
	svc := NewService(store)
	_, err := svc.ApprovePR(context.Background(), ApprovePRInput{
		PRURL:                    prURL,
		ExpectedSourceCommitHash: "old",
	})
	if err == nil {
		t.Error("expected stale commit error")
	}
}

func TestApprovePR_CallsStoreApproveOnMatch(t *testing.T) {
	store := newFake("abc")
	approved := false
	store.approveErr = nil
	svc := NewService(&approveCapture{fakeStore: store, onApprove: func() { approved = true }})
	res, err := svc.ApprovePR(context.Background(), ApprovePRInput{
		PRURL:                    prURL,
		ExpectedSourceCommitHash: "abc",
	})
	if err != nil {
		t.Fatalf("ApprovePR: %v", err)
	}
	if !res.Approved {
		t.Error("expected Approved=true")
	}
	if !approved {
		t.Error("expected store.ApprovePullRequest called")
	}
}

func TestRequestChangesPR_RejectsStaleCommit(t *testing.T) {
	store := newFake("current")
	svc := NewService(store)
	_, err := svc.RequestChangesPR(context.Background(), RequestChangesInput{
		PRURL:                    prURL,
		ExpectedSourceCommitHash: "old",
	})
	if err == nil {
		t.Error("expected stale commit error")
	}
}

// --- Task tests -------------------------------------------------------------

func TestResolveTask_UpdatesToResolved(t *testing.T) {
	store := newFake("abc")
	svc := NewService(store)
	result, err := svc.ResolveTask(context.Background(), ResolveTaskInput{PRURL: prURL, TaskID: 5})
	if err != nil {
		t.Fatalf("ResolveTask: %v", err)
	}
	if result.State != "RESOLVED" {
		t.Errorf("state = %q, want RESOLVED", result.State)
	}
	if result.TaskID != 5 {
		t.Errorf("task ID = %d, want 5", result.TaskID)
	}
}

func TestReopenTask_UpdatesToUnresolved(t *testing.T) {
	store := newFake("abc")
	svc := NewService(store)
	result, err := svc.ReopenTask(context.Background(), ReopenTaskInput{PRURL: prURL, TaskID: 5})
	if err != nil {
		t.Fatalf("ReopenTask: %v", err)
	}
	if result.State != "UNRESOLVED" {
		t.Errorf("state = %q, want UNRESOLVED", result.State)
	}
}

// --- helper fake types for partial-failure test ---

type failOnceStore struct {
	*fakeStore
	calls int
}

func (f *failOnceStore) CreatePullRequestComment(ctx context.Context, prURL string, input bitbucket.CreateCommentInput) (*bitbucket.Comment, error) {
	f.calls++
	if f.calls == 1 {
		return nil, errors.New("simulated failure")
	}
	return f.fakeStore.CreatePullRequestComment(ctx, prURL, input)
}

type approveCapture struct {
	*fakeStore
	onApprove func()
}

func (a *approveCapture) ApprovePullRequest(_ context.Context, _ string) error {
	a.onApprove()
	return nil
}
