package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kidboy-man/bitbucket-mcp/bitbucket"
	"github.com/kidboy-man/bitbucket-mcp/reviewer"
)

// --- JSON-RPC types ---------------------------------------------------------

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- Tool schema types ------------------------------------------------------

type JSONSchema struct {
	Type        string                `json:"type"`
	Description string                `json:"description,omitempty"`
	Properties  map[string]JSONSchema `json:"properties,omitempty"`
	Required    []string              `json:"required,omitempty"`
}

type ToolDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema JSONSchema `json:"inputSchema"`
}

// --- Server -----------------------------------------------------------------

type Server struct {
	bb *bitbucket.Client
}

func New(bb *bitbucket.Client) *Server {
	return &Server{bb: bb}
}

// Run reads JSON-RPC requests from stdin and writes responses to stdout.
// This is the MCP transport layer (stdio mode).
func (s *Server) Run() {
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			enc.Encode(errorResp(0, -32700, "parse error"))
			continue
		}
		enc.Encode(s.handle(req))
	}
}

func (s *Server) handle(req Request) Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolCall(req)
	default:
		return errorResp(req.ID, -32601, "method not found: "+req.Method)
	}
}

// --- initialize -------------------------------------------------------------

func (s *Server) handleInitialize(req Request) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]string{
				"name":    "bitbucket-mcp",
				"version": "1.0.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		},
	}
}

// --- tools/list -------------------------------------------------------------

var toolDefs = []ToolDef{
	{
		Name:        "get_pr",
		Description: "Fetch a Bitbucket PR's metadata, structured diff (with DiffPosition per line), and commits. Returns everything Claude needs to review the PR.",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]JSONSchema{
				"pr_url": {
					Type:        "string",
					Description: "Full Bitbucket PR URL, e.g. https://bitbucket.org/workspace/repo/pull-requests/42",
				},
			},
			Required: []string{"pr_url"},
		},
	},
	{
		Name:        "list_pr_comments",
		Description: "List existing inline comments on a PR so Claude can avoid duplicating them.",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]JSONSchema{
				"pr_url": {Type: "string"},
			},
			Required: []string{"pr_url"},
		},
	},
	{
		Name:        "post_inline_comment",
		Description: "Post an approved inline comment to Bitbucket. Use the DiffPosition value from get_pr output — NOT the file's absolute line number.",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]JSONSchema{
				"pr_url":        {Type: "string"},
				"file":          {Type: "string", Description: "Relative file path, e.g. internal/handler/user.go"},
				"diff_position": {Type: "integer", Description: "DiffPosition from get_pr output"},
				"body":          {Type: "string", Description: "Markdown comment body"},
			},
			Required: []string{"pr_url", "file", "diff_position", "body"},
		},
	},
}

func (s *Server) handleToolsList(req Request) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"tools": toolDefs},
	}
}

// --- tools/call -------------------------------------------------------------

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolCall(req Request) Response {
	var p toolCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResp(req.ID, -32602, "invalid params")
	}

	var result any
	var err error

	switch p.Name {
	case "get_pr":
		result, err = s.toolGetPR(p.Arguments)
	case "list_pr_comments":
		result, err = s.toolListComments(p.Arguments)
	case "post_inline_comment":
		result, err = s.toolPostComment(p.Arguments)
	default:
		return errorResp(req.ID, -32601, "unknown tool: "+p.Name)
	}

	if err != nil {
		return errorResp(req.ID, -32603, err.Error())
	}

	return Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("%v", result)},
			},
		},
	}
}

// --- tool implementations ---------------------------------------------------

func (s *Server) toolGetPR(args json.RawMessage) (any, error) {
	var a struct {
		PRURL string `json:"pr_url"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}

	pr, err := s.bb.GetPR(a.PRURL)
	if err != nil {
		return nil, err
	}

	pd, err := reviewer.Parse(pr.RawDiff)
	if err != nil {
		return nil, fmt.Errorf("parsing diff: %w", err)
	}

	// Build a structured response Claude can reason about directly.
	type lineResult struct {
		DiffPosition int    `json:"diff_position"`
		OldLineNo    int    `json:"old_line_no,omitempty"`
		NewLineNo    int    `json:"new_line_no,omitempty"`
		Type         string `json:"type"` // "added" | "removed" | "context"
		Content      string `json:"content"`
	}
	type hunkResult struct {
		Header string       `json:"header"`
		Lines  []lineResult `json:"lines"`
	}
	type fileResult struct {
		Path      string       `json:"path"`
		OldPath   string       `json:"old_path,omitempty"`
		IsNew     bool         `json:"is_new,omitempty"`
		IsDeleted bool         `json:"is_deleted,omitempty"`
		IsRenamed bool         `json:"is_renamed,omitempty"`
		Hunks     []hunkResult `json:"hunks"`
	}

	files := make([]fileResult, 0, len(pd.Files))
	for _, f := range pd.Files {
		fr := fileResult{
			Path:      f.Path,
			OldPath:   f.OldPath,
			IsNew:     f.IsNew,
			IsDeleted: f.IsDeleted,
			IsRenamed: f.IsRenamed,
		}
		for _, h := range f.Hunks {
			hr := hunkResult{Header: h.Header}
			for _, l := range h.Lines {
				t := "context"
				switch l.Type {
				case reviewer.LineAdded:
					t = "added"
				case reviewer.LineRemoved:
					t = "removed"
				}
				hr.Lines = append(hr.Lines, lineResult{
					DiffPosition: l.DiffPosition,
					OldLineNo:    l.OldLineNo,
					NewLineNo:    l.NewLineNo,
					Type:         t,
					Content:      l.Content,
				})
			}
			fr.Hunks = append(fr.Hunks, hr)
		}
		files = append(files, fr)
	}

	out := map[string]any{
		"pr": map[string]any{
			"id":            pr.ID,
			"title":         pr.Title,
			"description":   pr.Description,
			"author":        pr.Author,
			"source_branch": pr.SourceBranch,
			"dest_branch":   pr.DestBranch,
			"state":         pr.State,
			"commits":       pr.Commits,
		},
		"diff": files,
		"note": "Use diff_position (not new_line_no) when calling post_inline_comment.",
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *Server) toolListComments(args json.RawMessage) (any, error) {
	var a struct {
		PRURL string `json:"pr_url"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}

	comments, err := s.bb.GetComments(a.PRURL)
	if err != nil {
		return nil, err
	}

	b, err := json.MarshalIndent(comments, "", "  ")
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *Server) toolPostComment(args json.RawMessage) (any, error) {
	var a struct {
		PRURL        string `json:"pr_url"`
		File         string `json:"file"`
		DiffPosition int    `json:"diff_position"`
		Body         string `json:"body"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}

	if err := s.bb.PostInlineComment(a.PRURL, a.File, a.DiffPosition, a.Body); err != nil {
		return nil, err
	}
	return fmt.Sprintf("comment posted on %s:%d", a.File, a.DiffPosition), nil
}

// --- helpers ----------------------------------------------------------------

func errorResp(id, code int, msg string) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: code, Message: msg},
	}
}
