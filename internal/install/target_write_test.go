package install

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunInstallCursorProjectDryRun(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run(context.Background(), []string{
		"--target", "cursor",
		"--scope", ScopeProject,
		"--dry-run",
		"--command", "/bin/bitbucket-mcp",
	}, Options{Stdout: stdout, Stderr: stderr})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, ".cursor/mcp.json") {
		t.Fatalf("output missing cursor project path: %s", out)
	}
	if !strings.Contains(out, "${BITBUCKET_API_TOKEN}") {
		t.Fatalf("output missing env reference: %s", out)
	}
}

func TestRunInstallRejectsPlaintextProjectScope(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := Run(context.Background(), []string{
		"--target", "cursor",
		"--scope", ScopeProject,
		"--env-mode", EnvModePlaintext,
		"--include-secrets",
		"--dry-run",
		"--command", "/bin/bitbucket-mcp",
	}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(stderr.String(), "plaintext credentials are not allowed for project scope") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
