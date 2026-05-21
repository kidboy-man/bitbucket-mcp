package install

import (
	"fmt"
	"os"
	"strings"
)

func LoadCredentialsFromEnv() (Credentials, []string) {
	var warnings []string
	creds := Credentials{
		Workspace: os.Getenv("BITBUCKET_WORKSPACE"),
		Email:     os.Getenv("BITBUCKET_EMAIL"),
		Token:     os.Getenv("BITBUCKET_API_TOKEN"),
	}
	if creds.Email == "" {
		if value := os.Getenv("BITBUCKET_USERNAME"); value != "" {
			creds.Email = value
			warnings = append(warnings, "BITBUCKET_USERNAME is deprecated; use BITBUCKET_EMAIL")
		}
	}
	if creds.Token == "" {
		if value := os.Getenv("BITBUCKET_APP_PASSWORD"); value != "" {
			creds.Token = value
			warnings = append(warnings, "BITBUCKET_APP_PASSWORD is deprecated; use BITBUCKET_API_TOKEN")
		}
	}
	return creds, warnings
}

func (c Credentials) ValidateRequired() error {
	var missing []string
	if c.Workspace == "" {
		missing = append(missing, "BITBUCKET_WORKSPACE")
	}
	if c.Email == "" {
		missing = append(missing, "BITBUCKET_EMAIL")
	}
	if c.Token == "" {
		missing = append(missing, "BITBUCKET_API_TOKEN")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return nil
}
