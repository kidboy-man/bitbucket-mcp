# bitbucket-mcp

A Go MCP (Model Context Protocol) server that exposes Bitbucket Cloud pull request operations as tools to Claude. It allows Claude to fetch PR diffs, read existing comments, and post inline review comments.

## How it works

The server communicates over **stdio using JSON-RPC 2.0**, the standard MCP transport. Claude Desktop spawns the binary and communicates via stdin/stdout.

## Prerequisites

- Go 1.22+
- A Bitbucket Cloud account
- A [Bitbucket API Token](https://id.atlassian.com/manage-profile/security/api-tokens) with scopes:
  - `pullrequest` (read)
  - `pullrequest:write` (post comments)

## Installation

```bash
git clone https://github.com/kidboy-man/bitbucket-mcp.git
cd bitbucket-mcp
make build
```

This produces a `bitbucket-mcp` binary in the project root.

## Claude Desktop Setup

Add the following to `~/.claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "bitbucket": {
      "command": "/absolute/path/to/bitbucket-mcp",
      "env": {
        "BITBUCKET_WORKSPACE": "your-workspace-slug",
        "BITBUCKET_EMAIL": "your-atlassian-email",
        "BITBUCKET_API_TOKEN": "your-api-token"
      }
    }
  }
}
```

Restart Claude Desktop after editing. The four tools will appear automatically in every conversation.

## Available Tools

### `get_pr`

Fetches a PR's metadata, structured diff, and commits.

```json
{ "pr_url": "https://bitbucket.org/workspace/repo/pull-requests/42" }
```

Returns:
- `pr` — id, title, description, author, branches, state, commits
- `diff` — files with hunks; each line includes `diff_position`, `old_line_no`, `new_line_no`, `type`, and `content`
- `note` — reminder to use `diff_position` (not `new_line_no`) when posting comments

### `list_pr_comments`

Lists existing inline comments on a PR to avoid duplicates.

```json
{ "pr_url": "https://bitbucket.org/workspace/repo/pull-requests/42" }
```

Returns an array of comments with id, body, file, line, author, and created_on.

### `post_inline_comment`

Posts an inline review comment anchored to a specific diff position.

```json
{
  "pr_url":        "https://bitbucket.org/workspace/repo/pull-requests/42",
  "file":          "internal/handler/user.go",
  "diff_position": 4,
  "body":          "Consider using `fmt.Errorf` instead of `errors.New(fmt.Sprintf(...))`"
}
```

> **Important:** `diff_position` must be the value from `get_pr` output — not the file's absolute line number. Passing the wrong value will result in a 422 error from the Bitbucket API.

### `get_pr_review_comments_with_context`

Fetches all inline review comments on a PR, each enriched with ±5 surrounding diff lines so Claude can understand the code being discussed and propose targeted fixes.

```json
{ "pr_url": "https://bitbucket.org/workspace/repo/pull-requests/42" }
```

Returns an array of enriched comments. Each entry includes:
- `id`, `author`, `created_on`, `body`, `file`, `line` — the comment itself
- `diff_context` — up to 11 surrounding diff lines (±5 from the anchored line), each with `diff_position`, `old_line_no`, `new_line_no`, `type`, and `content`. Empty array when the line is not in the diff.

## Review Workflow

### Posting new comments (code review)

1. Paste a PR URL in Claude.
2. Claude calls `get_pr` → receives structured diff and metadata.
3. Claude calls `list_pr_comments` → sees existing comments to avoid duplication.
4. Claude proposes inline comments grouped by severity, each with its `new_line_no`.
5. You approve, edit, or drop individual comments.
6. Claude calls `post_inline_comment` once per approved comment.

### Addressing existing feedback (propose fixes)

1. Paste a PR URL in Claude.
2. Claude calls `get_pr_review_comments_with_context` → receives every review comment with the surrounding code snippet.
3. Claude analyzes each comment against its code context and proposes a concrete fix in the conversation.
4. You review the proposed fixes, approve or adjust them.
5. Claude applies the changes to the local source files (or posts a reply comment via `post_inline_comment`).

## Development

```bash
make test    # run tests
make build   # compile
make clean   # remove binary
```

No external dependencies — stdlib only.
