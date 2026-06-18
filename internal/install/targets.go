package install

import (
	"fmt"
	"os"
	"runtime"
)

func locationForTarget(target, scope string) (InstallLocation, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return InstallLocation{}, fmt.Errorf("resolving home directory: %w", err)
	}
	configDir, _ := os.UserConfigDir()
	cwd, err := os.Getwd()
	if err != nil {
		return InstallLocation{}, fmt.Errorf("resolving working directory: %w", err)
	}
	locations := DetectTargets(DetectEnvironment{
		HomeDir:       home,
		UserConfigDir: configDir,
		WorkingDir:    cwd,
		GOOS:          runtime.GOOS,
	})
	for _, loc := range locations {
		if loc.Target == target && loc.Scope == scope {
			return loc, nil
		}
	}
	if target == "generic" {
		return InstallLocation{Target: "generic"}, nil
	}
	return InstallLocation{}, fmt.Errorf("unsupported target/scope: %s/%s", target, scope)
}

