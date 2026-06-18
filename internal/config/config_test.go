package config

import (
	"strings"
	"testing"
)

func TestLoadUsesCanonicalEnvironment(t *testing.T) {
	t.Setenv("BITBUCKET_WORKSPACE", "team")
	t.Setenv("BITBUCKET_EMAIL", "user@example.com")
	t.Setenv("BITBUCKET_API_TOKEN", "secret-token")

	cfg, warnings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Workspace != "team" {
		t.Fatalf("Workspace = %q, want team", cfg.Workspace)
	}
	if cfg.Username != "user@example.com" {
		t.Fatalf("Username = %q, want user@example.com", cfg.Username)
	}
	if cfg.Token != "secret-token" {
		t.Fatalf("Token = %q, want secret-token", cfg.Token)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestLoadUsesAliasesWhenCanonicalMissing(t *testing.T) {
	t.Setenv("BITBUCKET_WORKSPACE", "team")
	t.Setenv("BITBUCKET_USERNAME", "legacy-user")
	t.Setenv("BITBUCKET_APP_PASSWORD", "legacy-secret")

	cfg, warnings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Username != "legacy-user" {
		t.Fatalf("Username = %q, want legacy-user", cfg.Username)
	}
	if cfg.Token != "legacy-secret" {
		t.Fatalf("Token = %q, want legacy-secret", cfg.Token)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings len = %d, want 2: %v", len(warnings), warnings)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "BITBUCKET_USERNAME") || !strings.Contains(joined, "BITBUCKET_APP_PASSWORD") {
		t.Fatalf("warnings = %q, want alias names", joined)
	}
	if strings.Contains(joined, "legacy-secret") {
		t.Fatalf("warnings leaked token: %q", joined)
	}
}

func TestLoadPrefersCanonicalOverAliases(t *testing.T) {
	t.Setenv("BITBUCKET_WORKSPACE", "team")
	t.Setenv("BITBUCKET_EMAIL", "canonical-user")
	t.Setenv("BITBUCKET_API_TOKEN", "canonical-secret")
	t.Setenv("BITBUCKET_USERNAME", "legacy-user")
	t.Setenv("BITBUCKET_APP_PASSWORD", "legacy-secret")

	cfg, warnings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Username != "canonical-user" {
		t.Fatalf("Username = %q, want canonical-user", cfg.Username)
	}
	if cfg.Token != "canonical-secret" {
		t.Fatalf("Token = %q, want canonical-secret", cfg.Token)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestLoadReportsMissingRequiredEnvironment(t *testing.T) {
	t.Setenv("BITBUCKET_WORKSPACE", "team")
	t.Setenv("BITBUCKET_API_TOKEN", "secret-token")

	_, _, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing username error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "BITBUCKET_EMAIL") || !strings.Contains(msg, "BITBUCKET_USERNAME") {
		t.Fatalf("error = %q, want canonical and alias names", msg)
	}
	if strings.Contains(msg, "secret-token") {
		t.Fatalf("error leaked token: %q", msg)
	}
}
