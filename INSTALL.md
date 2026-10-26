# Installing bitbucket-mcp

`bitbucket-mcp` can generate MCP stdio config for Claude Desktop, Claude Code, Cursor, Codex, or any compatible client.

## Build

Use Go 1.26.10 or newer on a supported, security-patched release line.

```bash
make build
go version -m ./bitbucket-mcp
```

This creates `./bitbucket-mcp`. Confirm its embedded Go and MCP SDK versions after
updating dependencies, then restart the MCP client to load the rebuilt binary.

## Quick install

Print generic MCP config:

```bash
./bitbucket-mcp install --target generic --print-config
```

Generate Cursor project config as dry-run:

```bash
./bitbucket-mcp install --target cursor --scope project --dry-run
```

Run diagnostics:

```bash
./bitbucket-mcp doctor --target generic
```

## Environment variables

Default generated config references env vars instead of storing secrets:

```bash
export BITBUCKET_WORKSPACE="your-workspace"
export BITBUCKET_EMAIL="you@example.com"
export BITBUCKET_API_TOKEN="your-token"
```

`BITBUCKET_API_TOKEN` should have Bitbucket Cloud repository read and pull request read/write permissions.

## Security note

Many MCP clients store server `env` values in plaintext config files. `bitbucket-mcp install` defaults to env references like `${BITBUCKET_API_TOKEN}` to avoid writing token literals. Do not commit project MCP config files containing plaintext credentials.

## Targets

### Generic

```bash
./bitbucket-mcp install --target generic --print-config
```

Paste output into any MCP-compatible stdio client.

### Cursor

```bash
./bitbucket-mcp install --target cursor --scope user --dry-run
./bitbucket-mcp install --target cursor --scope project --dry-run
```

Project scope writes `.cursor/mcp.json` when full writes are enabled.

### Claude Desktop

Use generic output or target-specific dry-run:

```bash
./bitbucket-mcp install --target claude-desktop --dry-run
```

Restart Claude Desktop after config changes.

### Claude Code

```bash
./bitbucket-mcp install --target claude-code --scope user --dry-run
./bitbucket-mcp install --target claude-code --scope project --dry-run
```

Project scope must use env references.

### Codex

```bash
./bitbucket-mcp install --target codex --scope user --dry-run
```

Codex write support depends on config format verification.
