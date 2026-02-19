package server

import (
	"context"
	"net/http"
	"strings"
)

type contextKey struct{ name string }

var jmapTokenKey = contextKey{"jmap-token"}

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
