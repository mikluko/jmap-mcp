package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var remoteAddrKey = contextKey{"remote-addr"}

// RemoteAddrMiddleware is HTTP middleware that stores the direct peer IP in
// the request context. The access log prefers X-Forwarded-For (set by the
// ingress) and falls back to this address for direct connections.
func RemoteAddrMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), remoteAddrKey, host)))
	})
}

// AccessLogMiddleware returns MCP middleware that writes one access log entry
// per incoming request: JSON-RPC method, tool name for tool calls, client IP,
// user agent, session ID, duration, and error.
func AccessLogMiddleware(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			start := time.Now()
			res, err := next(ctx, method, req)

			attrs := []slog.Attr{
				slog.String("method", method),
				slog.Duration("duration", time.Since(start)),
			}
			if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
				attrs = append(attrs, slog.String("tool", p.Name))
			}
			if ip := clientIP(ctx, req); ip != "" {
				attrs = append(attrs, slog.String("client_ip", ip))
			}
			if extra := req.GetExtra(); extra != nil && extra.Header != nil {
				if ua := extra.Header.Get("User-Agent"); ua != "" {
					attrs = append(attrs, slog.String("user_agent", ua))
				}
			}
			if s := req.GetSession(); s != nil {
				if id := s.ID(); id != "" {
					attrs = append(attrs, slog.String("session", id))
				}
			}
			if r, ok := res.(*mcp.CallToolResult); ok && r.IsError {
				attrs = append(attrs, slog.Bool("tool_error", true))
			}
			if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))
			}
			logger.LogAttrs(ctx, slog.LevelInfo, "access", attrs...)

			return res, err
		}
	}
}

// clientIP resolves the originating client IP: the first X-Forwarded-For hop
// when the request came through the ingress, otherwise the direct peer
// address stored by RemoteAddrMiddleware.
func clientIP(ctx context.Context, req mcp.Request) string {
	if extra := req.GetExtra(); extra != nil && extra.Header != nil {
		if xff := extra.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(first)
		}
	}
	addr, _ := ctx.Value(remoteAddrKey).(string)
	return addr
}
