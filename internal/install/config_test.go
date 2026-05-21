package install

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestServerEntryDefaultsToEnvReferences(t *testing.T) {
	entry, err := BuildServerEntry(ServerEntryInput{
		Command: "/usr/local/bin/bitbucket-mcp",
	})
	if err != nil {
		t.Fatalf("BuildServerEntry: %v", err)
	}
	if entry.Command != "/usr/local/bin/bitbucket-mcp" {
		t.Fatalf("command = %q", entry.Command)
	}
	want := map[string]string{
		"BITBUCKET_WORKSPACE": "${BITBUCKET_WORKSPACE}",
		"BITBUCKET_EMAIL":     "${BITBUCKET_EMAIL}",
		"BITBUCKET_API_TOKEN": "${BITBUCKET_API_TOKEN}",
	}
	for key, value := range want {
		if entry.Env[key] != value {
			t.Fatalf("env[%s] = %q, want %q", key, entry.Env[key], value)
		}
	}
}

func TestServerEntryRejectsPlaintextSecretsWithoutOptIn(t *testing.T) {
	_, err := BuildServerEntry(ServerEntryInput{
		Command:       "/bin/bitbucket-mcp",
		EnvMode:       EnvModePlaintext,
		IncludeSecret: false,
		Credentials: Credentials{
			Workspace: "ws",
			Email:     "me@example.com",
			Token:     "secret-token",
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestServerEntryPlaintextAllowedOnlyForUserScope(t *testing.T) {
	_, err := BuildServerEntry(ServerEntryInput{
		Command:       "/bin/bitbucket-mcp",
		EnvMode:       EnvModePlaintext,
		IncludeSecret: true,
		Scope:         ScopeProject,
		Credentials: Credentials{
			Workspace: "ws",
			Email:     "me@example.com",
			Token:     "secret-token",
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestGenericConfigRedactsTokenByDefault(t *testing.T) {
	out, err := GenericConfigJSON(ServerEntryInput{
		ServerName: "bitbucket",
		Command:    "/bin/bitbucket-mcp",
		EnvMode:    EnvModePlaintext,
		Scope:      ScopeUser,
		Credentials: Credentials{
			Workspace: "ws",
			Email:     "me@example.com",
			Token:     "secret-token",
		},
	})
	if err != nil {
		t.Fatalf("GenericConfigJSON: %v", err)
	}
	if strings.Contains(string(out), "secret-token") {
		t.Fatalf("output leaked token: %s", out)
	}
	if !strings.Contains(string(out), "[REDACTED]") {
		t.Fatalf("output missing redaction: %s", out)
	}
}

func TestGenericConfigShape(t *testing.T) {
	out, err := GenericConfigJSON(ServerEntryInput{
		ServerName: "bitbucket",
		Command:    "/bin/bitbucket-mcp",
	})
	if err != nil {
		t.Fatalf("GenericConfigJSON: %v", err)
	}
	var decoded struct {
		MCPServers map[string]ServerEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	entry := decoded.MCPServers["bitbucket"]
	if entry.Command != "/bin/bitbucket-mcp" {
		t.Fatalf("command = %q", entry.Command)
	}
	if entry.Env["BITBUCKET_API_TOKEN"] != "${BITBUCKET_API_TOKEN}" {
		t.Fatalf("token env = %q", entry.Env["BITBUCKET_API_TOKEN"])
	}
}
