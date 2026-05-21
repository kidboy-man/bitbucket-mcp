package config

import (
	"fmt"
	"os"
)

type Config struct {
	Workspace string
	Username  string
	Token     string
}

func Load() (Config, []string, error) {
	var warnings []string
	cfg := Config{
		Workspace: os.Getenv("BITBUCKET_WORKSPACE"),
		Username:  os.Getenv("BITBUCKET_EMAIL"),
		Token:     os.Getenv("BITBUCKET_API_TOKEN"),
	}

	if cfg.Username == "" {
		if value := os.Getenv("BITBUCKET_USERNAME"); value != "" {
			cfg.Username = value
			warnings = append(warnings, "BITBUCKET_USERNAME is deprecated; use BITBUCKET_EMAIL")
		}
	}
	if cfg.Token == "" {
		if value := os.Getenv("BITBUCKET_APP_PASSWORD"); value != "" {
			cfg.Token = value
			warnings = append(warnings, "BITBUCKET_APP_PASSWORD is deprecated; use BITBUCKET_API_TOKEN")
		}
	}

	var missing []string
	if cfg.Workspace == "" {
		missing = append(missing, "BITBUCKET_WORKSPACE")
	}
	if cfg.Username == "" {
		missing = append(missing, "BITBUCKET_EMAIL or BITBUCKET_USERNAME")
	}
	if cfg.Token == "" {
		missing = append(missing, "BITBUCKET_API_TOKEN or BITBUCKET_APP_PASSWORD")
	}
	if len(missing) > 0 {
		return Config{}, warnings, fmt.Errorf("missing required env vars: %v", missing)
	}

	return cfg, warnings, nil
}
