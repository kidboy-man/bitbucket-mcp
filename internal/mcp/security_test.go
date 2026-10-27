package mcp

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kidboy-man/bitbucket-mcp/internal/review"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type countingDiffStore struct {
	review.PullRequestStore
	calls atomic.Int32
}

func (s *countingDiffStore) GetPullRequestDiff(context.Context, string) (string, error) {
	s.calls.Add(1)
	return "", nil
}

type protocolWriter struct {
	io.WriteCloser
	malformed atomic.Bool
}

func (w *protocolWriter) Write(p []byte) (int, error) {
	originalSize := len(p)
	if w.malformed.Load() {
		p = bytes.ReplaceAll(p, []byte(`"method":`), []byte(`"Method":`))
	}
	_, err := w.WriteCloser.Write(p)
	if err != nil {
		return 0, err
	}
	return originalSize, nil
}

func TestProtocolCasingCannotDispatchTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store := &countingDiffStore{}
	server := NewServer(review.NewService(store))
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	session, err := server.Connect(ctx, &sdkmcp.IOTransport{Reader: serverConn, Writer: serverConn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	writer := &protocolWriter{WriteCloser: clientConn}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "security-test", Version: "1"}, nil)
	peer, err := client.Connect(ctx, &sdkmcp.IOTransport{Reader: clientConn, Writer: writer}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	params := &sdkmcp.CallToolParams{Name: "get_pr_diff", Arguments: map[string]any{"pr_url": "https://bitbucket.org/ws/repo/pull-requests/1"}}
	result, err := peer.CallTool(ctx, params)
	if err != nil || result.IsError || store.calls.Load() != 1 {
		t.Fatalf("valid tool call failed: result=%v err=%v calls=%d", result, err, store.calls.Load())
	}
	writer.malformed.Store(true)
	_, err = peer.CallTool(ctx, params)
	if err == nil || store.calls.Load() != 1 {
		t.Fatalf("malformed casing dispatched tool: err=%v calls=%d", err, store.calls.Load())
	}
}

type discardWriteCloser struct{ io.Writer }

func (discardWriteCloser) Close() error { return nil }

func TestDefaultInboundFrameLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	frame := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"padding":"` + strings.Repeat("x", 17*1024*1024) + `"}}` + "\n"
	transport := &sdkmcp.IOTransport{Reader: io.NopCloser(strings.NewReader(frame)), Writer: discardWriteCloser{io.Discard}}
	conn, err := transport.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Read(ctx); err == nil {
		t.Fatal("oversized inbound frame accepted")
	}
}
