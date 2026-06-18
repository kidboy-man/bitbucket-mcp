package bitbucket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, mux *http.ServeMux) *Client {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewClient("myworkspace", "user@example.com", "secret",
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
}

func TestParseURL_Valid(t *testing.T) {
	ref, err := ParseURL("https://bitbucket.org/acme/myrepo/pull-requests/42")
	if err != nil {
		t.Fatalf("ParseURL error: %v", err)
	}
	if ref.Workspace != "acme" || ref.RepoSlug != "myrepo" || ref.ID != 42 {
		t.Errorf("unexpected ref: %+v", ref)
	}
}

func TestParseURL_NonBitbucketHost(t *testing.T) {
	_, err := ParseURL("https://github.com/acme/repo/pull/1")
	if err == nil {
		t.Error("expected error for non-bitbucket host")
	}
}

func TestParseURL_MalformedPath(t *testing.T) {
	_, err := ParseURL("https://bitbucket.org/acme/repo/pulls/1")
	if err == nil {
		t.Error("expected error for malformed path")
	}
}

func TestParseURL_NonIntegerID(t *testing.T) {
	_, err := ParseURL("https://bitbucket.org/acme/repo/pull-requests/abc")
	if err == nil {
		t.Error("expected error for non-integer PR ID")
	}
}

func TestValidateRef_WorkspaceMismatch(t *testing.T) {
	mux := http.NewServeMux()
	c := newTestClient(t, mux)
	_, err := c.validateRef("https://bitbucket.org/otherworkspace/repo/pull-requests/1")
	if err == nil {
		t.Error("expected workspace mismatch error")
	}
}

func TestGetPullRequest_SetsBasicAuth(t *testing.T) {
	mux := http.NewServeMux()
	var gotAuth string
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "title": "Test PR", "state": "OPEN",
			"author":      map[string]string{"display_name": "Alice"},
			"source":      map[string]any{"branch": map[string]string{"name": "feature"}, "commit": map[string]string{"hash": "abc123"}},
			"destination": map[string]any{"branch": map[string]string{"name": "main"}},
			"links":       map[string]any{"self": map[string]string{"href": ""}, "html": map[string]string{"href": ""}},
		})
	})
	c := newTestClient(t, mux)
	_, err := c.GetPullRequest(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1")
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("expected Basic auth, got: %q", gotAuth)
	}
}

func TestGetPullRequest_SourceCommitHash(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "title": "T", "state": "OPEN",
			"author":      map[string]string{"display_name": "Bob"},
			"source":      map[string]any{"branch": map[string]string{"name": "f"}, "commit": map[string]string{"hash": "deadbeef"}},
			"destination": map[string]any{"branch": map[string]string{"name": "main"}},
			"links":       map[string]any{"self": map[string]string{"href": ""}, "html": map[string]string{"href": ""}},
		})
	})
	c := newTestClient(t, mux)
	pr, err := c.GetPullRequest(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1")
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if pr.SourceCommitHash != "deadbeef" {
		t.Errorf("SourceCommitHash = %q, want deadbeef", pr.SourceCommitHash)
	}
}

func TestDoNon2xx_ReturnsAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"not found"}}`, http.StatusNotFound)
	})
	c := newTestClient(t, mux)
	_, err := c.GetPullRequest(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !isAPIError(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func TestCreateComment_InlinePayload(t *testing.T) {
	mux := http.NewServeMux()
	var gotBody map[string]any
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/comments", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         99,
			"content":    map[string]string{"raw": "hello"},
			"inline":     map[string]any{"path": "foo.go", "to": float64(42)},
			"author":     map[string]string{"display_name": "Bot"},
			"created_on": "2024-01-01",
		})
	})
	c := newTestClient(t, mux)
	cm, err := c.CreatePullRequestComment(context.Background(),
		"https://bitbucket.org/myworkspace/myrepo/pull-requests/1",
		CreateCommentInput{Body: "hello", FilePath: "foo.go", NewLine: 42},
	)
	if err != nil {
		t.Fatalf("CreatePullRequestComment: %v", err)
	}
	if cm.Line != 42 || cm.File != "foo.go" {
		t.Errorf("comment inline fields wrong: file=%q line=%d", cm.File, cm.Line)
	}
	// Verify inline.to in payload.
	inline, _ := gotBody["inline"].(map[string]any)
	if inline == nil {
		t.Fatal("missing inline in payload")
	}
	if inline["to"] != float64(42) {
		t.Errorf("inline.to = %v, want 42", inline["to"])
	}
	if inline["path"] != "foo.go" {
		t.Errorf("inline.path = %v, want foo.go", inline["path"])
	}
}

func TestCreateComment_PRLevelOmitsInline(t *testing.T) {
	mux := http.NewServeMux()
	var gotBody map[string]any
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/comments", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         100,
			"content":    map[string]string{"raw": "summary"},
			"author":     map[string]string{"display_name": "Bot"},
			"created_on": "2024-01-01",
		})
	})
	c := newTestClient(t, mux)
	_, err := c.CreatePullRequestComment(context.Background(),
		"https://bitbucket.org/myworkspace/myrepo/pull-requests/1",
		CreateCommentInput{Body: "summary"},
	)
	if err != nil {
		t.Fatalf("CreatePullRequestComment: %v", err)
	}
	if _, ok := gotBody["inline"]; ok {
		t.Error("PR-level comment should not have inline field")
	}
}

func TestCreateTask_Payload(t *testing.T) {
	mux := http.NewServeMux()
	var gotBody map[string]any
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/tasks", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         5,
			"state":      "UNRESOLVED",
			"content":    map[string]string{"raw": "fix this"},
			"creator":    map[string]string{"display_name": "Bot"},
			"created_on": "2024-01-01",
		})
	})
	c := newTestClient(t, mux)
	task, err := c.CreatePullRequestTask(context.Background(),
		"https://bitbucket.org/myworkspace/myrepo/pull-requests/1",
		CreateTaskInput{Body: "fix this"},
	)
	if err != nil {
		t.Fatalf("CreatePullRequestTask: %v", err)
	}
	if task.State != "UNRESOLVED" {
		t.Errorf("task.State = %q, want UNRESOLVED", task.State)
	}
	content, _ := gotBody["content"].(map[string]any)
	if content["raw"] != "fix this" {
		t.Errorf("task content.raw = %v, want 'fix this'", content["raw"])
	}
}

func TestUpdateTask_SendsState(t *testing.T) {
	mux := http.NewServeMux()
	var gotBody map[string]any
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/tasks/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         5,
			"state":      "RESOLVED",
			"content":    map[string]string{"raw": "done"},
			"creator":    map[string]string{"display_name": "Bot"},
			"created_on": "2024-01-01",
		})
	})
	c := newTestClient(t, mux)
	task, err := c.UpdatePullRequestTask(context.Background(),
		"https://bitbucket.org/myworkspace/myrepo/pull-requests/1",
		5, UpdateTaskInput{State: "RESOLVED"},
	)
	if err != nil {
		t.Fatalf("UpdatePullRequestTask: %v", err)
	}
	if task.State != "RESOLVED" {
		t.Errorf("task.State = %q, want RESOLVED", task.State)
	}
	if gotBody["state"] != "RESOLVED" {
		t.Errorf("payload state = %v, want RESOLVED", gotBody["state"])
	}
}

func TestApprove_UsesPost(t *testing.T) {
	mux := http.NewServeMux()
	var method string
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/approve", func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{})
	})
	c := newTestClient(t, mux)
	if err := c.ApprovePullRequest(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1"); err != nil {
		t.Fatalf("ApprovePullRequest: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
}

func TestRequestChanges_UsesPost(t *testing.T) {
	mux := http.NewServeMux()
	var method string
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/request-changes", func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{})
	})
	c := newTestClient(t, mux)
	if err := c.RequestChanges(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1"); err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
}

func TestResolveCommentThread_UsesPost(t *testing.T) {
	mux := http.NewServeMux()
	var method string
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/comments/7/resolve", func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusNoContent)
	})
	c := newTestClient(t, mux)
	if err := c.ResolveCommentThread(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1", 7); err != nil {
		t.Fatalf("ResolveCommentThread: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
}

func TestReopenCommentThread_UsesDelete(t *testing.T) {
	mux := http.NewServeMux()
	var method string
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/comments/7/resolve", func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusNoContent)
	})
	c := newTestClient(t, mux)
	if err := c.ReopenCommentThread(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1", 7); err != nil {
		t.Fatalf("ReopenCommentThread: %v", err)
	}
	if method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", method)
	}
}

func TestPagination_FollowsNext(t *testing.T) {
	mux := http.NewServeMux()
	calls := 0
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/commits", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			// First page — next points to page 2.
			nextURL := "http://" + r.Host + "/repositories/myworkspace/myrepo/pullrequests/1/commits?page=2"
			json.NewEncoder(w).Encode(map[string]any{
				"values": []map[string]any{
					{"hash": "aaa1111", "message": "first", "author": map[string]string{"raw": "Alice"}},
				},
				"next": nextURL,
			})
			return
		}
		// Page 2 — no next.
		json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{
				{"hash": "bbb2222", "message": "second", "author": map[string]string{"raw": "Bob"}},
			},
		})
	})
	c := newTestClient(t, mux)
	commits, err := c.ListPullRequestCommits(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1")
	if err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}
	if len(commits) != 2 {
		t.Errorf("len(commits) = %d, want 2", len(commits))
	}
}

func TestPagination_DetectsRepeatedNext(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/commits", func(w http.ResponseWriter, r *http.Request) {
		// Always returns same next URL → infinite loop guard.
		selfURL := "http://" + r.Host + r.URL.Path
		json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{},
			"next":   selfURL,
		})
	})
	c := newTestClient(t, mux)
	_, err := c.ListPullRequestCommits(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1")
	if err == nil {
		t.Error("expected error for repeated next URL")
	}
}

func TestPagination_StopsAtMaxPages(t *testing.T) {
	mux := http.NewServeMux()
	calls := 0
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/commits", func(w http.ResponseWriter, r *http.Request) {
		calls++
		nextURL := "http://" + r.Host + r.URL.Path + "?page=" + itoa(calls+1)
		json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{},
			"next":   nextURL,
		})
	})
	c := newTestClient(t, mux)
	_, err := c.ListPullRequestCommits(context.Background(), "https://bitbucket.org/myworkspace/myrepo/pull-requests/1")
	if err == nil {
		t.Fatal("expected max pages error")
	}
	if calls != maxPaginationPages {
		t.Errorf("calls = %d, want %d", calls, maxPaginationPages)
	}
}

func TestContentType_Post(t *testing.T) {
	mux := http.NewServeMux()
	var ct string
	mux.HandleFunc("/repositories/myworkspace/myrepo/pullrequests/1/comments", func(w http.ResponseWriter, r *http.Request) {
		ct = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         1,
			"content":    map[string]string{"raw": "x"},
			"author":     map[string]string{"display_name": "Bot"},
			"created_on": "2024-01-01",
		})
	})
	c := newTestClient(t, mux)
	c.CreatePullRequestComment(context.Background(),
		"https://bitbucket.org/myworkspace/myrepo/pull-requests/1",
		CreateCommentInput{Body: "x"},
	)
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// isAPIError unwraps err to find *APIError.
func isAPIError(err error, out **APIError) bool {
	for err != nil {
		if e, ok := err.(*APIError); ok {
			*out = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}
