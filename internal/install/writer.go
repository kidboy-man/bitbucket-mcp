package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type WriteOptions struct {
	DryRun      bool
	PrivateFile bool
	Now         func() time.Time
}

type WriteResult struct {
	Path            string
	BackupPath      string
	ProposedContent string
	Wrote           bool
}

func WriteMCPServerEntry(path, serverName string, entry ServerEntry, opts WriteOptions) (*WriteResult, error) {
	if serverName == "" {
		serverName = DefaultServerName
	}
	root, exists, err := readConfigObject(path)
	if err != nil {
		return nil, err
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
		root["mcpServers"] = servers
	}
	servers[serverName] = entry

	content, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding config: %w", err)
	}
	content = append(content, '\n')
	if !json.Valid(content) {
		return nil, fmt.Errorf("encoding config produced invalid JSON")
	}
	result := &WriteResult{Path: path, ProposedContent: string(content)}
	if opts.DryRun {
		return result, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating config directory: %w", err)
	}
	if exists {
		backupPath := backupName(path, opts.Now)
		original, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config for backup: %w", err)
		}
		if err := os.WriteFile(backupPath, original, 0o600); err != nil {
			return nil, fmt.Errorf("writing config backup: %w", err)
		}
		result.BackupPath = backupPath
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-")
	if err != nil {
		return nil, fmt.Errorf("creating temp config: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("writing temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("closing temp config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("replacing config: %w", err)
	}
	if opts.PrivateFile {
		_ = os.Chmod(path, 0o600)
	}
	result.Wrote = true
	return result, nil
}

func readConfigObject(path string) (map[string]any, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, false, nil
		}
		return nil, false, fmt.Errorf("reading config: %w", err)
	}
	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, true, fmt.Errorf("parsing config JSON: %w", err)
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, true, nil
}

func backupName(path string, now func() time.Time) string {
	if now == nil {
		now = time.Now
	}
	return fmt.Sprintf("%s.bak-%s", path, now().Format("20060102150405"))
}
