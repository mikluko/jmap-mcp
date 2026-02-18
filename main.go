package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mikluko/jmap-mcp/internal/config"
	"github.com/mikluko/jmap-mcp/internal/server"
)

var version = "0.0.0"

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	srv := server.NewServer(version, cfg.SessionURL, cfg.AuthToken, cfg.EnableSend)

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
	mux.Handle("/", server.TokenQueryMiddleware(mcpHandler))

	log.Printf("Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}
