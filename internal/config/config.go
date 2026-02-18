package config

import (
	"flag"
	"fmt"
	"os"
)

// Config holds the application configuration.
type Config struct {
	Mode       string // "stdio" or "http"
	ListenAddr string // for HTTP mode
	SessionURL string // JMAP session URL
	AuthToken  string // JMAP bearer token (optional in http mode)
	EnableSend bool   // enable email_send tool
}

// LoadConfig parses command-line flags and environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.Mode, "mode", "stdio", "Server mode: stdio or http")
	flag.StringVar(&cfg.ListenAddr, "listen", ":8080", "HTTP listen address (http mode only)")
	flag.BoolVar(&cfg.EnableSend, "enable-send", false, "Enable email_send tool (disabled by default for safety)")
	flag.Parse()

	cfg.SessionURL = os.Getenv("JMAP_SESSION_URL")
	if cfg.SessionURL == "" {
		return nil, fmt.Errorf("JMAP_SESSION_URL environment variable is required")
	}

	cfg.AuthToken = os.Getenv("JMAP_AUTH_TOKEN")

	if cfg.Mode == "stdio" && cfg.AuthToken == "" {
		return nil, fmt.Errorf("JMAP_AUTH_TOKEN environment variable is required in stdio mode")
	}

	if cfg.Mode != "stdio" && cfg.Mode != "http" {
		return nil, fmt.Errorf("mode must be 'stdio' or 'http', got: %s", cfg.Mode)
	}

	return cfg, nil
}
