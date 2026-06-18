package mcp

import (
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kidboy-man/bitbucket-mcp/internal/review"
)

// NewServer builds and returns an MCP server with all tools registered.
func NewServer(svc *review.Service) *sdkmcp.Server {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "bitbucket-mcp",
		Version: "1.0.0",
	}, nil)

	registerLegacyTools(server, svc)
	registerReviewTools(server, svc)

	return server
}

func boolPtr(b bool) *bool { return &b }

func errResult(msg string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: msg},
		},
		IsError: true,
	}
}
