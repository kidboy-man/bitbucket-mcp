# bitbucket-mcp

A Go MCP (Model Context Protocol) server that exposes Bitbucket Cloud pull request review operations as tools via the official MCP Go SDK over stdio.

## Repository layout

```
bitbucket-mcp/
├── cmd/bitbucket-mcp/      # entrypoint: loads config, wires deps, starts MCP stdio server
├── internal/config/        # env var loading and alias warnings
├── internal/bitbucket/     # typed Bitbucket Cloud REST API v2 adapter
├── internal/diff/          # unified diff parser and cursor pagination
├── internal/review/        # review workflow service and store port
└── internal/mcp/           # MCP SDK server and tool registration
```

## Package responsibilities

### `cmd/bitbucket-mcp`
Loads config with `config.Load()`, constructs `bitbucket.Client`, wraps it in `review.Service`, registers MCP tools, then runs `sdkmcp.StdioTransport`.

### `internal/config`
Reads canonical env vars and supports deprecated aliases. Canonical vars win when both are set. Warnings never include token values.

### `internal/bitbucket`
Typed Bitbucket Cloud REST client with context-aware requests, Basic Auth, bounded error bodies, testable base URL/client options, PR URL validation, and pagination loop guards.

Important rules:
- Only Bitbucket Cloud URLs are supported: `https://bitbucket.org/{workspace}/{repo}/pull-requests/{id}`.
- PR URL workspace must match configured workspace.
- Inline comments use `inline.to` with the new-file line number.
- `diff_position` is metadata only; never use it as Bitbucket comment anchor.

### `internal/diff`
Parses unified diffs into files, hunks, and line metadata. `DiffPosition` is a 1-based position in the parsed file diff. `NewLineNo` is the head-file line number used for inline comment anchors.

`ParsedDiff.GetPage()` returns paginated structured diff pages. Cursors are opaque base64 JSON and are locked to request parameters with a filter hash.

### `internal/review`
Application service for review workflows. MCP handlers should stay thin: validate input, call service, translate result.

Key invariants:
- `DraftReviewComments` validates anchors and deduplicates, but performs no writes.
- `PostReviewComments` requires a full stateless draft and checks `source_commit_hash` before any write.
- `ApprovePR` and `RequestChangesPR` require `expected_source_commit_hash` before writing.
- Posting comments can partially succeed; results include posted, failed, skipped, and created task arrays.
- No merge, decline, auto-approval, or auto-request-changes behavior exists.

### `internal/mcp`
Registers legacy and new MCP SDK tools with read/write annotations. Legacy tools remain available with deprecation notes.

## MCP tools

Legacy tools:
- `get_pr` — deprecated aggregate PR metadata + full structured diff.
- `list_pr_comments` — deprecated inline comment list.
- `post_inline_comment` — deprecated single inline comment post.
- `get_pr_review_comments_with_context` — deprecated inline comments enriched with diff context.

Current read tools:
- `get_pr_context` — PR metadata, commits, comments, tasks, statuses summary, first diff page.
- `get_pr_diff` — paginated structured diff; supports cursor and file filter.
- `list_pr_tasks` — PR tasks.
- `list_pr_statuses` — build/pipeline status summary.
- `draft_review_comments` — validate findings and produce stateless posting plan; no writes.

Current write tools:
- `post_review_comments` — post approved draft after stale-commit guard.
- `approve_pr` — approve after expected source commit hash guard.
- `request_changes_pr` — request changes after expected source commit hash guard.
- `resolve_task` / `reopen_task` — update PR task state.
- `resolve_comment_thread` / `reopen_comment_thread` — update comment thread resolution.

## Auth & configuration

| Env var | Description |
|---|---|
| `BITBUCKET_WORKSPACE` | Workspace slug |
| `BITBUCKET_EMAIL` | Bitbucket account email or username for Basic Auth |
| `BITBUCKET_API_TOKEN` | Bitbucket API token/app password with `Repositories: Read`, `Pull requests: Read+Write` |

Aliases are supported for existing configs: `BITBUCKET_USERNAME` falls back to `BITBUCKET_EMAIL`, and `BITBUCKET_APP_PASSWORD` falls back to `BITBUCKET_API_TOKEN`.

## Commands

```bash
go test ./...
go test ./internal/review -run TestDraftReviewComments
go test ./internal/bitbucket -run TestPagination
go build ./cmd/bitbucket-mcp
make build
make test
```

## Claude Desktop integration

`~/.claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "bitbucket": {
      "command": "/absolute/path/to/bitbucket-mcp",
      "env": {
        "BITBUCKET_WORKSPACE": "your-workspace",
        "BITBUCKET_EMAIL": "your-email@example.com",
        "BITBUCKET_API_TOKEN": "your-api-token"
      }
    }
  }
}
```

## Known gaps

- `get_pr` legacy output can still be large for very large PRs.
- Success-path diff response reads are unbounded before parsing.
- `ParseURL` assumes Bitbucket Cloud, not self-hosted Bitbucket Server.
