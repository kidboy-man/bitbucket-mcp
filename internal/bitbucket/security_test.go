package bitbucket

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const securityPRURL = "https://bitbucket.org/myworkspace/myrepo/pull-requests/1"

type securityTransport func(*http.Request) (*http.Response, error)

func (f securityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func securityResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r, ContentLength: -1}
}

func TestPaginationRejectsUnsafeDestinations(t *testing.T) {
	for _, next := range []string{
		"https://attacker.invalid/page", "http://api.bitbucket.org/page",
		"https://api.bitbucket.org:444/page", "https://user:password@api.bitbucket.org/page",
		"https://api.bitbucket.org/page#fragment", "//attacker.invalid/page", "ftp://api.bitbucket.org/page",
	} {
		t.Run(next, func(t *testing.T) {
			calls := 0
			client := NewClient("myworkspace", "test-user", "test-token", WithHTTPClient(&http.Client{Transport: securityTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					body, err := json.Marshal(map[string]any{"values": []any{}, "next": next})
					if err != nil {
						return nil, err
					}
					return securityResponse(r, 200, string(body)), nil
				}
				return securityResponse(r, 200, `{"values":[]}`), nil
			})}))
			got, err := client.ListPullRequestCommits(context.Background(), securityPRURL)
			if err == nil || got != nil {
				t.Fatalf("unsafe next accepted: result=%v, err=%v", got, err)
			}
			if calls != 1 {
				t.Fatalf("rejected destination reached transport; calls=%d", calls)
			}
		})
	}
}

func TestPaginationAllowsEquivalentOrigin(t *testing.T) {
	calls := 0
	client := NewClient("myworkspace", "test-user", "test-token", WithHTTPClient(&http.Client{Transport: securityTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if user, token, ok := r.BasicAuth(); !ok || user != "test-user" || token != "test-token" {
			return nil, errors.New("missing authentication")
		}
		if calls == 1 {
			return securityResponse(r, 200, `{"values":[],"next":"https://API.BITBUCKET.ORG:443/2.0/page"}`), nil
		}
		return securityResponse(r, 200, `{"values":[{"hash":"abc1234","message":"allowed"}]}`), nil
	})}))
	got, err := client.ListPullRequestCommits(context.Background(), securityPRURL)
	if err != nil || len(got) != 1 || calls != 2 {
		t.Fatalf("result=%v, calls=%d, err=%v", got, calls, err)
	}
}

func TestRedirectDestinationGuards(t *testing.T) {
	for _, endpoint := range []string{"json", "diff"} {
		for _, target := range []string{"https://attacker.invalid/page", "http://api.bitbucket.org/page", "https://api.bitbucket.org:444/page", "https://user@api.bitbucket.org/page", "https://api.bitbucket.org/page#fragment", "https://api.bitbucket.org/2.0/page"} {
			t.Run(endpoint+"/"+target, func(t *testing.T) {
				calls := 0
				client := NewClient("myworkspace", "test-user", "test-token", WithHTTPClient(&http.Client{Transport: securityTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					if calls == 1 {
						resp := securityResponse(r, 302, "")
						resp.Header.Set("Location", target)
						return resp, nil
					}
					return securityResponse(r, 200, `{}`), nil
				})}))
				var err error
				if endpoint == "diff" {
					_, err = client.GetPullRequestDiff(context.Background(), securityPRURL)
				} else {
					_, err = client.GetPullRequest(context.Background(), securityPRURL)
				}
				allowed := target == "https://api.bitbucket.org/2.0/page"
				if allowed && (err != nil || calls != 2) {
					t.Fatalf("allowed redirect failed: calls=%d err=%v", calls, err)
				}
				if !allowed && (err == nil || calls != 1) {
					t.Fatalf("unsafe redirect reached transport: calls=%d err=%v", calls, err)
				}
			})
		}
	}
}

func TestRedirectPreservesInjectedClient(t *testing.T) {
	sentinel := errors.New("caller redirect policy")
	callbacks := 0
	original := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { callbacks++; return sentinel }, Transport: securityTransport(func(r *http.Request) (*http.Response, error) {
		resp := securityResponse(r, 302, "")
		resp.Header.Set("Location", "https://api.bitbucket.org/2.0/page")
		return resp, nil
	})}
	client := NewClient("myworkspace", "user", "token", WithHTTPClient(original))
	_, err := client.GetPullRequest(context.Background(), securityPRURL)
	if !errors.Is(err, sentinel) || callbacks != 1 {
		t.Fatalf("callback not preserved: %v, calls=%d", err, callbacks)
	}
	if client.http == original || original.Timeout != time.Second || client.http.Timeout != time.Second {
		t.Fatal("injected client was not cloned with settings intact")
	}
	if err := original.CheckRedirect(nil, nil); !errors.Is(err, sentinel) {
		t.Fatal("original redirect policy changed")
	}
}

func TestRedirectDefaultLimit(t *testing.T) {
	calls := 0
	client := NewClient("myworkspace", "user", "token", WithHTTPClient(&http.Client{Transport: securityTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		resp := securityResponse(r, 302, "")
		resp.Header.Set("Location", fmt.Sprintf("https://api.bitbucket.org/page?%d", calls))
		return resp, nil
	})}))
	_, err := client.GetPullRequest(context.Background(), securityPRURL)
	if err == nil || calls != 10 {
		t.Fatalf("redirect limit lost: calls=%d err=%v", calls, err)
	}
}

func TestSuccessResponseSizeLimits(t *testing.T) {
	for _, endpoint := range []string{"json", "diff"} {
		limit := 8 * 1024 * 1024
		if endpoint == "diff" {
			limit = 16 * 1024 * 1024
		}
		for _, extra := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/extra=%d", endpoint, extra), func(t *testing.T) {
				body := `{}` + strings.Repeat(" ", limit-2+extra)
				client := NewClient("myworkspace", "user", "token", WithHTTPClient(&http.Client{Transport: securityTransport(func(r *http.Request) (*http.Response, error) { return securityResponse(r, 200, body), nil })}))
				var err error
				if endpoint == "diff" {
					var got string
					got, err = client.GetPullRequestDiff(context.Background(), securityPRURL)
					if extra == 1 && got != "" {
						t.Fatal("oversized diff returned partial data")
					}
				} else {
					var got *PullRequest
					got, err = client.GetPullRequest(context.Background(), securityPRURL)
					if extra == 1 && got != nil {
						t.Fatal("oversized JSON returned partial data")
					}
				}
				if (err != nil) != (extra == 1) {
					t.Fatalf("extra=%d err=%v", extra, err)
				}
			})
		}
	}
}

func TestPaginationCumulativeSizeLimit(t *testing.T) {
	for _, count := range []int{4, 5} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			calls := 0
			client := NewClient("myworkspace", "user", "token", WithHTTPClient(&http.Client{Transport: securityTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body := `{"values":[{"hash":"abc1234","message":"page"}]}`
				if calls < count {
					body = fmt.Sprintf(`{"values":[{"hash":"abc1234","message":"page"}],"next":"https://api.bitbucket.org/page?%d"}`, calls)
				}
				body += strings.Repeat(" ", 8*1024*1024-len(body))
				return securityResponse(r, 200, body), nil
			})}))
			got, err := client.ListPullRequestCommits(context.Background(), securityPRURL)
			if count == 4 && (err != nil || len(got) != 4) {
				t.Fatalf("exact budget rejected: result=%v err=%v", got, err)
			}
			if count == 5 && (err == nil || got != nil) {
				t.Fatalf("budget exceeded without error: result=%v err=%v", got, err)
			}
		})
	}
}

func TestCompressedChunkedResponsesAreBoundedAfterDecompression(t *testing.T) {
	for _, endpoint := range []string{"json", "diff"} {
		t.Run(endpoint, func(t *testing.T) {
			limit := 8 * 1024 * 1024
			if endpoint == "diff" {
				limit = 16 * 1024 * 1024
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", "gzip")
				w.(http.Flusher).Flush()
				compressed := gzip.NewWriter(w)
				if _, err := io.WriteString(compressed, `{}`+strings.Repeat(" ", limit-1)); err != nil {
					t.Errorf("writing compressed response: %v", err)
				}
				if err := compressed.Close(); err != nil {
					t.Errorf("closing compressed response: %v", err)
				}
			}))
			defer srv.Close()
			client := NewClient("myworkspace", "user", "token", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
			if endpoint == "diff" {
				got, err := client.GetPullRequestDiff(context.Background(), securityPRURL)
				if err == nil || got != "" {
					t.Fatalf("oversized decompressed diff accepted: err=%v", err)
				}
			} else {
				got, err := client.GetPullRequest(context.Background(), securityPRURL)
				if err == nil || got != nil {
					t.Fatalf("oversized decompressed JSON accepted: err=%v", err)
				}
			}
		})
	}
}

type countingBody struct {
	io.Reader
	read int
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func TestOversizedResponseReadsOnlyLimitPlusOne(t *testing.T) {
	body := &countingBody{Reader: strings.NewReader(strings.Repeat("x", 1024))}
	got, err := readBoundedBody(body, 16)
	if err == nil || got != nil || body.read != 17 {
		t.Fatalf("result=%v err=%v bytes read=%d", got, err, body.read)
	}
}
