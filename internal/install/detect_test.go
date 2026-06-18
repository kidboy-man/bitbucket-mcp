package install

import (
	"path/filepath"
	"testing"
)

func TestDetectTargetsDarwinPaths(t *testing.T) {
	env := DetectEnvironment{
		HomeDir:       "/Users/alice",
		UserConfigDir: "/Users/alice/Library/Application Support",
		WorkingDir:    "/repo",
		GOOS:          "darwin",
	}
	locations := DetectTargets(env)

	assertLocation(t, locations, "claude-desktop", ScopeUser, filepath.Join("/Users/alice/Library/Application Support", "Claude", "claude_desktop_config.json"))
	assertLocation(t, locations, "claude-code", ScopeUser, filepath.Join("/Users/alice", ".claude", ".mcp.json"))
	assertLocation(t, locations, "claude-code", ScopeProject, filepath.Join("/repo", ".claude", "mcp.json"))
	assertLocation(t, locations, "cursor", ScopeUser, filepath.Join("/Users/alice", ".cursor", "mcp.json"))
	assertLocation(t, locations, "cursor", ScopeProject, filepath.Join("/repo", ".cursor", "mcp.json"))
	assertLocation(t, locations, "codex", ScopeUser, filepath.Join("/Users/alice", ".codex", "config.toml"))
	assertLocation(t, locations, "generic", "", "")
}

func TestDetectTargetsLinuxClaudeDesktopPath(t *testing.T) {
	env := DetectEnvironment{
		HomeDir:       "/home/alice",
		UserConfigDir: "/home/alice/.config",
		WorkingDir:    "/repo",
		GOOS:          "linux",
	}
	locations := DetectTargets(env)
	assertLocation(t, locations, "claude-desktop", ScopeUser, filepath.Join("/home/alice", ".config", "Claude", "claude_desktop_config.json"))
}

func assertLocation(t *testing.T, locations []InstallLocation, target, scope, path string) {
	t.Helper()
	for _, loc := range locations {
		if loc.Target == target && loc.Scope == scope && loc.Path == path {
			return
		}
	}
	t.Fatalf("missing location target=%q scope=%q path=%q in %#v", target, scope, path, locations)
}
