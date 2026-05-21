package install

import (
	"encoding/json"
	"errors"
)

const (
	EnvModeReferences = "references"
	EnvModePlaintext  = "plaintext"

	ScopeUser    = "user"
	ScopeProject = "project"

	DefaultServerName = "bitbucket"
)

type Credentials struct {
	Workspace string
	Email     string
	Token     string
}

type ServerEntryInput struct {
	ServerName    string
	Command       string
	Args          []string
	EnvMode       string
	Scope         string
	IncludeSecret bool
	Credentials   Credentials
}

type ServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func BuildServerEntry(input ServerEntryInput) (ServerEntry, error) {
	if input.Command == "" {
		return ServerEntry{}, errors.New("command is required")
	}
	envMode := input.EnvMode
	if envMode == "" {
		envMode = EnvModeReferences
	}
	scope := input.Scope
	if scope == "" {
		scope = ScopeUser
	}

	env := envReferences()
	if envMode == EnvModePlaintext {
		if !input.IncludeSecret {
			return ServerEntry{}, errors.New("plaintext credentials require --include-secrets")
		}
		if scope == ScopeProject {
			return ServerEntry{}, errors.New("plaintext credentials are not allowed for project scope")
		}
		env = map[string]string{
			"BITBUCKET_WORKSPACE": input.Credentials.Workspace,
			"BITBUCKET_EMAIL":     input.Credentials.Email,
			"BITBUCKET_API_TOKEN": input.Credentials.Token,
		}
	}

	return ServerEntry{
		Command: input.Command,
		Args:    append([]string(nil), input.Args...),
		Env:     env,
	}, nil
}

func GenericConfigJSON(input ServerEntryInput) ([]byte, error) {
	serverName := input.ServerName
	if serverName == "" {
		serverName = DefaultServerName
	}
	entryInput := input
	entryInput.IncludeSecret = input.IncludeSecret
	if input.EnvMode == EnvModePlaintext && !input.IncludeSecret {
		entryInput.IncludeSecret = true
	}
	entry, err := BuildServerEntry(entryInput)
	if err != nil {
		return nil, err
	}
	if input.EnvMode == EnvModePlaintext && !input.IncludeSecret {
		entry.Env["BITBUCKET_API_TOKEN"] = "[REDACTED]"
	}
	out := struct {
		MCPServers map[string]ServerEntry `json:"mcpServers"`
	}{
		MCPServers: map[string]ServerEntry{serverName: entry},
	}
	return json.MarshalIndent(out, "", "  ")
}

func envReferences() map[string]string {
	return map[string]string{
		"BITBUCKET_WORKSPACE": "${BITBUCKET_WORKSPACE}",
		"BITBUCKET_EMAIL":     "${BITBUCKET_EMAIL}",
		"BITBUCKET_API_TOKEN": "${BITBUCKET_API_TOKEN}",
	}
}
