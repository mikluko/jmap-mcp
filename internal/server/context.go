package server

import (
	"context"
	"net/http"
	"strings"
)

type contextKey struct{ name string }

var (
	jmapTokenKey = contextKey{"jmap-token"}
	baseURLKey   = contextKey{"base-url"}
)

// ContextWithToken returns a new context with the JMAP auth token stored.
func ContextWithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, jmapTokenKey, token)
}

// TokenFromContext extracts the JMAP auth token from the context, or returns empty string.
func TokenFromContext(ctx context.Context) string {
	v, _ := ctx.Value(jmapTokenKey).(string)
	return v
}

// TokenMiddleware is HTTP middleware that extracts the JMAP auth token from
// the request and stores it in the request context. It checks, in order:
//  1. jmap_token query parameter
//  2. Authorization: Bearer <token> header
func TokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string
		if token = r.URL.Query().Get("jmap_token"); token == "" {
			if v := r.Header.Get("Authorization"); v != "" {
				token, _ = strings.CutPrefix(v, "Bearer ")
			}
		}
		if token != "" {
			r = r.WithContext(ContextWithToken(r.Context(), token))
		}
		next.ServeHTTP(w, r)
	})
}

// ContextWithBaseURL returns a new context carrying the external base URL of
// the server (scheme://host, no trailing slash).
func ContextWithBaseURL(ctx context.Context, base string) context.Context {
	return context.WithValue(ctx, baseURLKey, base)
}

// BaseURLFromContext extracts the external base URL from the context, or
// returns empty string.
func BaseURLFromContext(ctx context.Context) string {
	v, _ := ctx.Value(baseURLKey).(string)
	return v
}

// BaseURLMiddleware is HTTP middleware that derives the server's external
// base URL from the request (honoring X-Forwarded-Proto and X-Forwarded-Host)
// and stores it in the request context for tool handlers to build absolute URLs.
func BaseURLMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme := r.Header.Get("X-Forwarded-Proto")
		if scheme == "" {
			if r.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		}
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
		if host != "" {
			r = r.WithContext(ContextWithBaseURL(r.Context(), scheme+"://"+host))
		}
		next.ServeHTTP(w, r)
	})
}
