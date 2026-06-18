package install

import "path/filepath"

type DetectEnvironment struct {
	HomeDir       string
	UserConfigDir string
	WorkingDir    string
	GOOS          string
}

type InstallLocation struct {
	Target string
	Scope  string
	Path   string
}

func DetectTargets(env DetectEnvironment) []InstallLocation {
	return []InstallLocation{
		{Target: "claude-desktop", Scope: ScopeUser, Path: claudeDesktopPath(env)},
		{Target: "claude-code", Scope: ScopeUser, Path: filepath.Join(env.HomeDir, ".claude", ".mcp.json")},
		{Target: "claude-code", Scope: ScopeProject, Path: filepath.Join(env.WorkingDir, ".claude", "mcp.json")},
		{Target: "cursor", Scope: ScopeUser, Path: filepath.Join(env.HomeDir, ".cursor", "mcp.json")},
		{Target: "cursor", Scope: ScopeProject, Path: filepath.Join(env.WorkingDir, ".cursor", "mcp.json")},
		{Target: "codex", Scope: ScopeUser, Path: filepath.Join(env.HomeDir, ".codex", "config.toml")},
		{Target: "generic"},
	}
}

func claudeDesktopPath(env DetectEnvironment) string {
	switch env.GOOS {
	case "darwin":
		base := env.UserConfigDir
		if base == "" {
			base = filepath.Join(env.HomeDir, "Library", "Application Support")
		}
		return filepath.Join(base, "Claude", "claude_desktop_config.json")
	case "windows":
		base := env.UserConfigDir
		if base == "" {
			base = filepath.Join(env.HomeDir, "AppData", "Roaming")
		}
		return filepath.Join(base, "Claude", "claude_desktop_config.json")
	default:
		base := env.UserConfigDir
		if base == "" {
			base = filepath.Join(env.HomeDir, ".config")
		}
		return filepath.Join(base, "Claude", "claude_desktop_config.json")
	}
}
