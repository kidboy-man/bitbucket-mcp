# bitbucket-mcp

A Go MCP (Model Context Protocol) server that exposes Bitbucket Cloud pull request
operations as tools to Claude via **stdio / JSON-RPC 2.0**.

## Repository layout

```
bitbucket-mcp/
├── main.go              # entrypoint: loads config, wires deps, starts server
├── go.mod               # module: github.com/kidboy-man/bitbucket-mcp, go 1.22
├── mcp/
│   └── server.go        # JSON-RPC 2.0 handler + tool registry + dispatch
├── bitbucket/
│   └── client.go        # Bitbucket Cloud REST API v2 client
└── reviewer/
    ├── parser.go        # unified diff parser → ParsedDiff
    └── parser_test.go   # 15 tests for parser correctness
```

## Package responsibilities

### `main`
Reads env vars via `loadConfig()`, fails fast on missing values, constructs
`bitbucket.Client` and `mcp.Server`, then calls `srv.Run()`.

### `mcp` (server.go)
`Server.Run()` loops over stdin with `bufio.Scanner`, decodes each line as a
JSON-RPC `Request`, dispatches via `handle()`, encodes the `Response` to stdout.

Handles three methods:
- `initialize` — protocol version + server capabilities
- `tools/list` — returns the four tool definitions
- `tools/call` — dispatches to `toolGetPR`, `toolListComments`, `toolPostComment`, or `toolGetCommentsWithContext`

Tool results are always wrapped as:
```json
{ "content": [{ "type": "text", "text": "..." }] }
```

### `bitbucket` (client.go)
Key types: `Client`, `PR`, `Commit`, `InlineComment`.

- `ParseURL(rawURL)` — parses `https://bitbucket.org/{workspace}/{repo}/pull-requests/{id}`
- `GetPR(prURL)` — three sequential calls: metadata, diff, commits
- `GetComments(prURL)` — returns only inline comments (skips PR-level)
- `PostInlineComment(prURL, filePath, newLineNo, body)` — posts with `inline.to`
  set to `newLineNo` (the new-file line number, not a diff position)

All HTTP calls use Basic Auth, 30s timeout, no external dependencies.

### `reviewer` (parser.go)
Core types:

```go
type LineType int  // LineContext | LineAdded | LineRemoved

type DiffLine struct {
    DiffPosition int    // 1-based position in the file's diff — what Bitbucket API wants
    OldLineNo    int    // base file line number (0 if line doesn't exist there)
    NewLineNo    int    // head file line number (0 if line doesn't exist there)
    Type         LineType
    Content      string
}
```

Key functions:
- `Parse(raw string) (*ParsedDiff, error)` — state-machines through the unified diff.
  `DiffPosition` increments once per `@@` line and once per content line, resets per file.
- `(*ParsedDiff).FindDiffPosition(filePath, newLineNo)` — maps a new-file line number to
  its `DiffPosition` for the Bitbucket API.
- `(*ParsedDiff).GetContextForLine(filePath, newLineNo, contextLines)` — returns the slice
  of `DiffLine`s within ±`contextLines` of the given new-file line number, clamped to hunk
  boundaries. Returns `(nil, false)` when the line is not found in the diff.
- `(*ParsedDiff).Summary()` — compact text block for Claude showing file/hunk/line metadata.

## The four MCP tools

### `get_pr`
Input: `{ "pr_url": "https://bitbucket.org/..." }`

Returns JSON with:
- `pr` — id, title, description, author, branches, state, commits
- `diff` — array of files with hunks; each line has `diff_position`, `old_line_no`,
  `new_line_no`, `type`, `content`
- `note` — reminder to use `new_line_no`, not `diff_position`, when calling `post_inline_comment`

### `list_pr_comments`
Input: `{ "pr_url": "..." }`

Returns existing inline comments (id, body, file, line, author, created_on).

### `post_inline_comment`
Input:
```json
{
  "pr_url":      "https://bitbucket.org/...",
  "file":        "internal/handler/user.go",
  "new_line_no": 45,
  "body":        "Consider using fmt.Errorf instead."
}
```

**Note:** `new_line_no` is the `new_line_no` value from `get_pr` output — the line number
in the new (head) file. This is what `bitbucket.PostInlineComment` passes as `inline.to`.

### `get_pr_review_comments_with_context`
Input: `{ "pr_url": "..." }`

Returns an array of enriched inline comments. Each entry:
- `id`, `author`, `created_on`, `body`, `file`, `line` — the comment itself
- `diff_context` — up to 11 diff lines (±5 from the anchored line) with `diff_position`,
  `old_line_no`, `new_line_no`, `type`, `content`. Empty array when line not in diff.

Use this tool when you need to read existing review feedback and propose fixes: the
`diff_context` field provides the code snippet each reviewer was commenting on so you
can reason about what change is being requested.

## Auth & configuration

| Env var | Description |
|---|---|
| `BITBUCKET_WORKSPACE` | Workspace slug |
| `BITBUCKET_USERNAME` | Bitbucket account username |
| `BITBUCKET_APP_PASSWORD` | App password with `Repositories: Read`, `Pull requests: Read+Write` |

## Build

```bash
go test ./...
go build -o bitbucket-mcp .
```

## Claude Desktop integration

`~/.claude/claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "bitbucket": {
      "command": "/absolute/path/to/bitbucket-mcp",
      "env": {
        "BITBUCKET_WORKSPACE":    "your-workspace",
        "BITBUCKET_USERNAME":     "your-username",
        "BITBUCKET_APP_PASSWORD": "your-app-password"
      }
    }
  }
}
```

## Known gaps

- No pagination on `GetComments` — truncated at Bitbucket's default page size (~100)
- `post_inline_comment` is sequential; large batches should add retry + partial-failure reporting
- No diff size guard — very large PRs will produce a large `get_pr` response
- `ParseURL` assumes Bitbucket Cloud (not self-hosted Bitbucket Server)
- `min()` helper in `client.go` is redundant on Go 1.21+ (has builtin `min`)
