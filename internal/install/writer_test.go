package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteMCPServerEntryPreservesUnrelatedEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := `{
  "theme": "dark",
  "mcpServers": {
    "existing": {
      "command": "/bin/existing"
    },
    "bitbucket": {
      "command": "/old"
    }
  }
}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	entry := ServerEntry{Command: "/new", Env: envReferences()}
	result, err := WriteMCPServerEntry(path, "bitbucket", entry, WriteOptions{})
	if err != nil {
		t.Fatalf("WriteMCPServerEntry: %v", err)
	}
	if result.BackupPath == "" {
		t.Fatal("expected backup path")
	}

	written := readJSONMap(t, path)
	if written["theme"] != "dark" {
		t.Fatalf("theme = %v", written["theme"])
	}
	servers := written["mcpServers"].(map[string]any)
	if servers["existing"].(map[string]any)["command"] != "/bin/existing" {
		t.Fatalf("existing server not preserved: %#v", servers["existing"])
	}
	if servers["bitbucket"].(map[string]any)["command"] != "/new" {
		t.Fatalf("bitbucket command not replaced: %#v", servers["bitbucket"])
	}
}

func TestWriteMCPServerEntryDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := `{"mcpServers":{"old":{"command":"/old"}}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := WriteMCPServerEntry(path, "bitbucket", ServerEntry{Command: "/new"}, WriteOptions{DryRun: true})
	if err != nil {
		t.Fatalf("WriteMCPServerEntry: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("file changed during dry-run: %s", content)
	}
	if !strings.Contains(result.ProposedContent, "/new") {
		t.Fatalf("dry-run missing proposed content: %q", result.ProposedContent)
	}
}

func TestWriteMCPServerEntryInvalidJSONDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := `{invalid json`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := WriteMCPServerEntry(path, "bitbucket", ServerEntry{Command: "/new"}, WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("invalid json overwritten: %s", content)
	}
}

func TestWriteMCPServerEntryCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	_, err := WriteMCPServerEntry(path, "bitbucket", ServerEntry{Command: "/new"}, WriteOptions{})
	if err != nil {
		t.Fatalf("WriteMCPServerEntry: %v", err)
	}
	written := readJSONMap(t, path)
	servers := written["mcpServers"].(map[string]any)
	if servers["bitbucket"].(map[string]any)["command"] != "/new" {
		t.Fatalf("bitbucket entry missing: %#v", servers)
	}
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(content, &out); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, content)
	}
	return out
}
