package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRunDispatchNoArgsRunsServer(t *testing.T) {
	var serverCalls int
	code := run(context.Background(), []string{"bitbucket-mcp"}, runOptions{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		runServer: func(context.Context) error {
			serverCalls++
			return nil
		},
		installRunner: func(context.Context, []string) int {
			t.Fatal("install runner called")
			return 0
		},
		doctorRunner: func(context.Context, []string) int {
			t.Fatal("doctor runner called")
			return 0
		},
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if serverCalls != 1 {
		t.Fatalf("server calls = %d, want 1", serverCalls)
	}
}

func TestRunDispatchInstall(t *testing.T) {
	var gotArgs []string
	code := run(context.Background(), []string{"bitbucket-mcp", "install", "--target", "generic"}, runOptions{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		runServer: func(context.Context) error {
			t.Fatal("server runner called")
			return nil
		},
		installRunner: func(_ context.Context, args []string) int {
			gotArgs = append([]string(nil), args...)
			return 7
		},
		doctorRunner: func(context.Context, []string) int {
			t.Fatal("doctor runner called")
			return 0
		},
	})
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "--target" || gotArgs[1] != "generic" {
		t.Fatalf("install args = %#v", gotArgs)
	}
}

func TestRunDispatchDoctor(t *testing.T) {
	var gotArgs []string
	code := run(context.Background(), []string{"bitbucket-mcp", "doctor", "--json"}, runOptions{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		runServer: func(context.Context) error {
			t.Fatal("server runner called")
			return nil
		},
		installRunner: func(context.Context, []string) int {
			t.Fatal("install runner called")
			return 0
		},
		doctorRunner: func(_ context.Context, args []string) int {
			gotArgs = append([]string(nil), args...)
			return 3
		},
	})
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	if len(gotArgs) != 1 || gotArgs[0] != "--json" {
		t.Fatalf("doctor args = %#v", gotArgs)
	}
}

func TestRunDispatchUnknownCommand(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := run(context.Background(), []string{"bitbucket-mcp", "unknown"}, runOptions{
		stdout: &bytes.Buffer{},
		stderr: stderr,
		runServer: func(context.Context) error {
			t.Fatal("server runner called")
			return nil
		},
		installRunner: func(context.Context, []string) int {
			t.Fatal("install runner called")
			return 0
		},
		doctorRunner: func(context.Context, []string) int {
			t.Fatal("doctor runner called")
			return 0
		},
	})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("unknown command")) {
		t.Fatalf("stderr = %q, want unknown command", stderr.String())
	}
}
