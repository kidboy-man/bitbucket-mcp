package install

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

type Options struct {
	Stdout io.Writer
	Stderr io.Writer
}

func Run(_ context.Context, args []string, opts Options) int {
	stdout, stderr := writers(opts)
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "", "target MCP client")
	scope := fs.String("scope", ScopeUser, "config scope")
	envMode := fs.String("env-mode", EnvModeReferences, "credential env mode")
	includeSecrets := fs.Bool("include-secrets", false, "include literal secret values")
	dryRun := fs.Bool("dry-run", false, "show changes without writing")
	printConfig := fs.Bool("print-config", false, "print config")
	command := fs.String("command", "", "server command path")
	serverName := fs.String("server-name", DefaultServerName, "server name")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *target == "" {
		fmt.Fprintln(stderr, "--target is required")
		return 2
	}
	cmd := *command
	if cmd == "" {
		path, err := os.Executable()
		if err != nil {
			fmt.Fprintf(stderr, "resolving executable path: %v\n", err)
			return 1
		}
		cmd = path
	}
	entryInput := ServerEntryInput{ServerName: *serverName, Command: cmd, EnvMode: *envMode, Scope: *scope, IncludeSecret: *includeSecrets}
	if *target == "generic" {
		out, err := GenericConfigJSON(entryInput)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if *printConfig || *target == "generic" {
			fmt.Fprintln(stdout, string(out))
			return 0
		}
		return 0
	}
	entry, err := BuildServerEntry(entryInput)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	loc, err := locationForTarget(*target, *scope)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if !*dryRun {
		fmt.Fprintln(stderr, "direct config writes require --dry-run in this build")
		return 2
	}
	result, err := WriteMCPServerEntry(loc.Path, *serverName, entry, WriteOptions{DryRun: true})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "target: %s\npath: %s\n%s\n", loc.Target, loc.Path, result.ProposedContent)
	return 0
}

type DoctorReport struct {
	Target string   `json:"target"`
	Issues []string `json:"issues,omitempty"`
}

func RunDoctor(_ context.Context, args []string, opts Options) int {
	stdout, stderr := writers(opts)
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "", "target MCP client")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *target == "" {
		fmt.Fprintln(stderr, "--target is required")
		return 2
	}
	report := DoctorReport{Target: *target}
	if *jsonOutput {
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(out))
		return 0
	}
	fmt.Fprintf(stdout, "%s: ok\n", *target)
	return 0
}

func writers(opts Options) (io.Writer, io.Writer) {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return stdout, stderr
}
