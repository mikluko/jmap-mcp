package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAccessLogMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "test_tool", Description: "test"},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	srv.AddReceivingMiddleware(AccessLogMiddleware(logger))

	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "test_tool"}); err != nil {
		t.Fatal(err)
	}

	entry := findLogEntry(t, &buf, "tools/call")
	if entry["tool"] != "test_tool" {
		t.Errorf("tool = %v, want test_tool", entry["tool"])
	}
	if _, ok := entry["duration"]; !ok {
		t.Error("duration attr missing")
	}
}

func TestClientIP(t *testing.T) {
	reqWithHeader := func(h http.Header) mcp.Request {
		return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
			Params: &mcp.CallToolParamsRaw{Name: "x"},
			Extra:  &mcp.RequestExtra{Header: h},
		}
	}

	t.Run("forwarded single", func(t *testing.T) {
		req := reqWithHeader(http.Header{"X-Forwarded-For": {"203.0.113.7"}})
		if got := clientIP(context.Background(), req); got != "203.0.113.7" {
			t.Errorf("clientIP = %q, want 203.0.113.7", got)
		}
	})

	t.Run("forwarded chain takes first hop", func(t *testing.T) {
		req := reqWithHeader(http.Header{"X-Forwarded-For": {"203.0.113.7, 10.0.0.1"}})
		if got := clientIP(context.Background(), req); got != "203.0.113.7" {
			t.Errorf("clientIP = %q, want 203.0.113.7", got)
		}
	})

	t.Run("falls back to remote addr", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), remoteAddrKey, "192.0.2.1")
		req := reqWithHeader(http.Header{})
		if got := clientIP(ctx, req); got != "192.0.2.1" {
			t.Errorf("clientIP = %q, want 192.0.2.1", got)
		}
	})

	t.Run("nothing available", func(t *testing.T) {
		req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: "x"}}
		if got := clientIP(context.Background(), req); got != "" {
			t.Errorf("clientIP = %q, want empty", got)
		}
	})
}

func TestRemoteAddrMiddleware(t *testing.T) {
	var got string
	h := RemoteAddrMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = r.Context().Value(remoteAddrKey).(string)
	}))

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = "192.0.2.1:54321"
	h.ServeHTTP(httptest.NewRecorder(), r)

	if got != "192.0.2.1" {
		t.Errorf("remote addr = %q, want 192.0.2.1", got)
	}
}

// findLogEntry scans JSON log lines for the first entry with the given method.
func findLogEntry(t *testing.T, buf *bytes.Buffer, method string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(buf)
	for {
		var entry map[string]any
		if err := dec.Decode(&entry); err != nil {
			t.Fatalf("no log entry with method %q", method)
		}
		if entry["method"] == method {
			return entry
		}
	}
}
