package main

import (
	"context"
	"fmt"
	"io"
	"os"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kidboy-man/bitbucket-mcp/internal/bitbucket"
	"github.com/kidboy-man/bitbucket-mcp/internal/config"
	"github.com/kidboy-man/bitbucket-mcp/internal/install"
	mcpserver "github.com/kidboy-man/bitbucket-mcp/internal/mcp"
	"github.com/kidboy-man/bitbucket-mcp/internal/review"
)

type runOptions struct {
	stdout        io.Writer
	stderr        io.Writer
	runServer     func(context.Context) error
	installRunner func(context.Context, []string) int
	doctorRunner  func(context.Context, []string) int
}

func main() {
	os.Exit(run(context.Background(), os.Args, runOptions{
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		runServer: runServer,
		installRunner: func(ctx context.Context, args []string) int {
			return install.Run(ctx, args, install.Options{Stdout: os.Stdout, Stderr: os.Stderr})
		},
		doctorRunner: func(ctx context.Context, args []string) int {
			return install.RunDoctor(ctx, args, install.Options{Stdout: os.Stdout, Stderr: os.Stderr})
		},
	}))
}

func run(ctx context.Context, args []string, opts runOptions) int {
	if len(args) > 1 {
		switch args[1] {
		case "install":
			return opts.installRunner(ctx, args[2:])
		case "doctor":
			return opts.doctorRunner(ctx, args[2:])
		default:
			fmt.Fprintf(opts.stderr, "unknown command %q\n", args[1])
			return 2
		}
	}
	if err := opts.runServer(ctx); err != nil {
		fmt.Fprintln(opts.stderr, err)
		return 1
	}
	return 0
}

func runServer(ctx context.Context) error {
	cfg, warnings, err := config.Load()
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, w)
	}

	bb := bitbucket.NewClient(cfg.Workspace, cfg.Username, cfg.Token)
	svc := review.NewService(bb)
	server := mcpserver.NewServer(svc)

	return server.Run(ctx, &sdkmcp.StdioTransport{})
}
