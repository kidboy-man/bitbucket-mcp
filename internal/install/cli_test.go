package install

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunInstallGenericPrintConfig(t *testing.T) {
	stdout := &bytes.Buffer{}
	code := Run(context.Background(), []string{"--target", "generic", "--print-config", "--command", "/bin/bitbucket-mcp"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "\"mcpServers\"") || !strings.Contains(out, "\"bitbucket\"") {
		t.Fatalf("output missing config: %s", out)
	}
	if !strings.Contains(out, "${BITBUCKET_API_TOKEN}") {
		t.Fatalf("output missing env reference: %s", out)
	}
}

func TestRunInstallRequiresTarget(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := Run(context.Background(), []string{"--print-config"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(stderr.String(), "--target is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunDoctorGeneric(t *testing.T) {
	stdout := &bytes.Buffer{}
	code := RunDoctor(context.Background(), []string{"--target", "generic", "--json"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "\"target\": \"generic\"") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}
