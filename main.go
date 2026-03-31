package main

import (
	"fmt"
	"os"

	"github.com/kidboy-man/bitbucket-mcp/bitbucket"
	"github.com/kidboy-man/bitbucket-mcp/mcp"
)

func main() {
	cfg := loadConfig()

	bb := bitbucket.New(cfg.workspace, cfg.email, cfg.token)
	srv := mcp.New(bb)
	srv.Run()
}

type config struct {
	workspace string
	email     string
	token     string
}

func loadConfig() config {
	cfg := config{
		workspace: os.Getenv("BITBUCKET_WORKSPACE"),
		email:     os.Getenv("BITBUCKET_EMAIL"),
		token:     os.Getenv("BITBUCKET_API_TOKEN"),
	}

	var missing []string
	if cfg.workspace == "" {
		missing = append(missing, "BITBUCKET_WORKSPACE")
	}
	if cfg.email == "" {
		missing = append(missing, "BITBUCKET_EMAIL")
	}
	if cfg.token == "" {
		missing = append(missing, "BITBUCKET_API_TOKEN")
	}

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "missing required env vars: %v\n", missing)
		os.Exit(1)
	}

	return cfg
}
