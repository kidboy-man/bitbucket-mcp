package main

import (
	"fmt"
	"os"

	"github.com/yourorg/bitbucket-mcp/bitbucket"
	"github.com/yourorg/bitbucket-mcp/mcp"
)

func main() {
	cfg := loadConfig()

	bb := bitbucket.New(cfg.workspace, cfg.username, cfg.password)
	srv := mcp.New(bb)
	srv.Run()
}

type config struct {
	workspace string
	username  string
	password  string
}

func loadConfig() config {
	cfg := config{
		workspace: os.Getenv("BITBUCKET_WORKSPACE"),
		username:  os.Getenv("BITBUCKET_USERNAME"),
		password:  os.Getenv("BITBUCKET_APP_PASSWORD"),
	}

	var missing []string
	if cfg.workspace == "" {
		missing = append(missing, "BITBUCKET_WORKSPACE")
	}
	if cfg.username == "" {
		missing = append(missing, "BITBUCKET_USERNAME")
	}
	if cfg.password == "" {
		missing = append(missing, "BITBUCKET_APP_PASSWORD")
	}

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "missing required env vars: %v\n", missing)
		os.Exit(1)
	}

	return cfg
}
