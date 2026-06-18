package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultBitbucketAPIBaseURL = "https://api.bitbucket.org/2.0"

type WorkspaceValidator interface {
	ValidateWorkspace(context.Context, Credentials) error
}

type BitbucketWorkspaceValidator struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (v BitbucketWorkspaceValidator) ValidateWorkspace(ctx context.Context, creds Credentials) error {
	if err := creds.ValidateRequired(); err != nil {
		return err
	}
	baseURL := v.BaseURL
	if baseURL == "" {
		baseURL = defaultBitbucketAPIBaseURL
	}
	client := v.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	url := strings.TrimRight(baseURL, "/") + "/workspaces/" + creds.Workspace
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating Bitbucket validation request: %w", err)
	}
	req.SetBasicAuth(creds.Email, creds.Token)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("validating Bitbucket workspace: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body = bytes.TrimSpace(body)
	status := resp.StatusCode
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("invalid Bitbucket credentials (status %d)", status)
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("workspace not found: %s", creds.Workspace)
	}
	if len(body) > 0 {
		return fmt.Errorf("Bitbucket workspace validation failed with status %d: %s", status, body)
	}
	return fmt.Errorf("Bitbucket workspace validation failed with status %d", status)
}
