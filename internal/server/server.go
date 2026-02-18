package server

import (
	"context"
	"fmt"

	"github.com/mikluko/jmap"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	// Register JMAP method response types for deserialization.
	_ "github.com/mikluko/jmap/mail/email"
	_ "github.com/mikluko/jmap/mail/emailsubmission"
	_ "github.com/mikluko/jmap/mail/identity"
	_ "github.com/mikluko/jmap/mail/mailbox"
	_ "github.com/mikluko/jmap/sieve/sievescript"
)

// Option configures optional Server behavior.
type Option func(*Server)

// WithToken sets a static JMAP auth token (used in stdio mode).
func WithToken(token string) Option {
	return func(s *Server) { s.token = token }
}

// WithEmailSubmission enables the email_submission_set tool.
func WithEmailSubmission() Option {
	return func(s *Server) { s.enableEmailSubmission = true }
}

// WithSieve enables the sieve_get, sieve_set, and sieve_validate tools.
func WithSieve() Option {
	return func(s *Server) { s.enableSieve = true }
}

// Server wraps the MCP server and JMAP client.
type Server struct {
	mcp                   *mcp.Server
	sessionURL            string
	token                 string // static token for stdio mode; empty in HTTP-only mode
	enableEmailSubmission bool
	enableSieve           bool
}

// NewServer creates a new MCP server with JMAP tools.
func NewServer(version, sessionURL string, opts ...Option) *Server {
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "jmap-mcp",
		Version: version,
	}, &mcp.ServerOptions{
		Instructions: serverInstructions,
	})

	s := &Server{
		mcp:        mcpServer,
		sessionURL: sessionURL,
	}
	for _, opt := range opts {
		opt(s)
	}

	s.registerTools()

	return s
}

// MCP returns the underlying MCP server instance.
func (s *Server) MCP() *mcp.Server {
	return s.mcp
}

// resolveToken returns the JMAP auth token, preferring context (HTTP mode)
// over the static token (stdio mode).
func (s *Server) resolveToken(ctx context.Context) (string, error) {
	if t := TokenFromContext(ctx); t != "" {
		return t, nil
	}
	if s.token != "" {
		return s.token, nil
	}
	return "", fmt.Errorf("no JMAP auth token available")
}

// jmapClient creates a JMAP client using the resolved token, authenticates
// the session, and returns the ready client.
func (s *Server) jmapClient(ctx context.Context) (*jmap.Client, error) {
	token, err := s.resolveToken(ctx)
	if err != nil {
		return nil, err
	}
	client := (&jmap.Client{SessionEndpoint: s.sessionURL}).WithAccessToken(token)
	if err := client.Authenticate(); err != nil {
		return nil, fmt.Errorf("jmap session: %w", err)
	}
	return client, nil
}
