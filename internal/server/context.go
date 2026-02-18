package server

import (
	"context"
	"net/http"
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

// TokenQueryMiddleware is HTTP middleware that extracts jmap_token from the
// query string and stores it in the request context.
func TokenQueryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := r.URL.Query().Get("jmap_token"); token != "" {
			r = r.WithContext(ContextWithToken(r.Context(), token))
		}
		next.ServeHTTP(w, r)
	})
}
