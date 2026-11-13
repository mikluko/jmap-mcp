package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"sarak.sh/mikluko/jmap-mcp/internal/config"
	"sarak.sh/mikluko/jmap-mcp/internal/server"
)

const unlinked = "0.0.0"

var version = unlinked

// reportedVersion returns linked when a release build set it, and otherwise
// the main module's version from info, without its leading "v".
// Where info records no module version it returns linked.
func reportedVersion(linked string, info *debug.BuildInfo) string {
	if linked != unlinked || info == nil || info.Main.Version == "" {
		return linked
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	var opts []server.Option
	if cfg.AuthToken != "" {
		opts = append(opts, server.WithToken(cfg.AuthToken))
	}
	if cfg.EnableEmailSubmission {
		opts = append(opts, server.WithEmailSubmission())
	}
	if cfg.EnableSieve {
		opts = append(opts, server.WithSieve())
	}
	if cfg.Mode == "http" {
		opts = append(opts, server.WithAttachmentURL(cfg.AttachmentURLSecret, cfg.ExternalURL))
	}
	info, _ := debug.ReadBuildInfo()
	srv := server.NewServer(reportedVersion(version, info), cfg.SessionURL, opts...)

	switch cfg.Mode {
	case "stdio":
		runStdio(srv)
	case "http":
		runHTTP(srv, cfg.ListenAddr)
	}
}

func runStdio(srv *server.Server) {
	if err := srv.MCP().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func runHTTP(srv *server.Server, addr string) {
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv.MCP() },
		nil,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/attachments/", srv.AttachmentHandler())
	mux.Handle("/", server.BaseURLMiddleware(server.TokenMiddleware(mcpHandler)))

	log.Printf("Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}
