package install

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBitbucketWorkspaceValidatorSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/workspaces/ws", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing auth header")
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	validator := BitbucketWorkspaceValidator{BaseURL: srv.URL, HTTPClient: srv.Client()}
	if err := validator.ValidateWorkspace(context.Background(), Credentials{Workspace: "ws", Email: "me@example.com", Token: "secret-token"}); err != nil {
		t.Fatalf("ValidateWorkspace: %v", err)
	}
}

func TestBitbucketWorkspaceValidatorAuthFailureDoesNotLeakToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/workspaces/ws", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	validator := BitbucketWorkspaceValidator{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := validator.ValidateWorkspace(context.Background(), Credentials{Workspace: "ws", Email: "me@example.com", Token: "secret-token"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid Bitbucket credentials") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestBitbucketWorkspaceValidatorWorkspaceNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/workspaces/ws", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	validator := BitbucketWorkspaceValidator{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := validator.ValidateWorkspace(context.Background(), Credentials{Workspace: "ws", Email: "me@example.com", Token: "secret-token"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "workspace not found") {
		t.Fatalf("error = %v", err)
	}
}
