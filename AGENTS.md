# AGENTS.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

JMAP MCP server — exposes JMAP (JSON Meta Application Protocol, RFC 8620/8621) email and Sieve script (RFC 9425) operations as MCP (Model Context Protocol) tools. Built with the official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`) and the `github.com/mikluko/jmap` library for typed JMAP protocol support.

## Build & Test

```bash
go build .                           # build binary
go test ./...                        # all tests
go test -run TestFoo ./internal/...  # single test
go vet ./...
```

Version is injected via ldflags:
```bash
go build -ldflags="-X main.version=$(git describe --tags --always)"
```

## Configuration

| Env var | Required | Description |
|---|---|---|
| `JMAP_SESSION_URL` | always | JMAP session endpoint (e.g. `https://api.fastmail.com/jmap/session`) |
| `JMAP_AUTH_TOKEN` | stdio mode | Bearer token for JMAP authentication |

In HTTP mode, the token can come from the `jmap_token` query parameter instead of the env var.

## Architecture

```
main.go                         # entrypoint: flag parsing, transport selection (stdio/http)
internal/
  config/                       # CLI flags + env vars (JMAP_SESSION_URL, JMAP_AUTH_TOKEN, -enable-send, -enable-sieve)
  server/                       # MCP server wrapper
    server.go                   # Server struct, token resolution, jmap.Client factory, blank imports for type registration
    context.go                  # Token context key, TokenQueryMiddleware for HTTP mode
    tools.go                    # mailbox_get, email_query, email_get, helpers, registerTools()
    tools_email_mutate.go       # Email/set convenience wrappers (email_create, email_move, email_flag, email_delete)
    tools_email_send.go         # identity_get + email_submission_set (feature-gated by -enable-send)
    tools_mailbox_mutate.go     # mailbox_set (create/update/destroy)
    tools_sieve.go              # sieve_get, sieve_set, sieve_validate
```

External dependency `github.com/mikluko/jmap` provides:
- `jmap` — core JMAP client, session, request/response, type registry, `Patch` type
- `jmap/mail` — mail capability URI, `Address` type
- `jmap/mail/mailbox` — `Mailbox`, `Get`/`GetResponse`, `Set`/`SetResponse`, role constants (`RoleTrash`, `RoleDrafts`, `RoleSent`, etc.)
- `jmap/mail/email` — `Email`, `Get`/`GetResponse`, `Set`/`SetResponse`, `Query`/`QueryResponse`, `FilterCondition`
- `jmap/mail/emailsubmission` — `EmailSubmission`, `Set`/`SetResponse` (with `OnSuccessUpdateEmail`)
- `jmap/mail/identity` — `Identity`, `Get`/`GetResponse`
- `jmap/sieve` — sieve capability URI
- `jmap/sieve/sievescript` — `SieveScript`, `Get`/`GetResponse`, `Set`/`SetResponse`, `Validate`/`ValidateResponse`

### Server wrapper pattern

`internal/server.Server` wraps `*mcp.Server` and holds the JMAP session URL + static token. Tools are methods on Server, registered in `registerTools()`. The wrapper exposes `MCP() *mcp.Server` for transport wiring in `main.go`.

Each tool call creates a fresh `*jmap.Client` via `s.jmapClient(ctx)`, which resolves the token (context first, then static fallback) and authenticates the session. Session caching can be added later.

### Transport modes

`-mode stdio` (default) uses `mcp.StdioTransport{}` with `server.Run()`.
`-mode http` uses `mcp.NewStreamableHTTPHandler()` behind `http.ListenAndServe`, with `TokenQueryMiddleware` extracting `jmap_token` from the query string into request context.

### Token resolution

`resolveToken(ctx)` checks context first (populated by HTTP middleware from `?jmap_token=`), then falls back to the static env var token. This supports both:
- **stdio**: static `JMAP_AUTH_TOKEN` env var
- **http**: per-request `?jmap_token=` query parameter (for Claude Web MCP integrations that can't set headers)

### Tool pattern

Each tool follows this structure:
1. Input struct with `json` and `jsonschema` tags
2. Package-level `var xxxTool = &mcp.Tool{...}` definition
3. Handler method on Server: `func (s *Server) handleXxx(ctx, req, input) (*mcp.CallToolResult, any, error)`
4. Registration in `registerTools()` via `mcp.AddTool(s.mcp, xxxTool, s.handleXxx)`

Tools return `any` as output type (no typed output schema) and build `*mcp.CallToolResult` manually with `TextContent`. Errors are returned as tool errors (via `errorResult`), not protocol errors.

### JMAP request pattern

All handlers use the go-jmap typed request/response pattern:
1. `req := &jmap.Request{Context: ctx}`
2. `callID := req.Invoke(&method{Account: accountID, ...})` — capabilities are auto-registered from method's `Requires()`
3. `resp, err := client.Do(req)` — sends the request
4. Type-switch `resp.Responses[i].Args` for typed response (`*mailbox.GetResponse`, `*email.GetResponse`, etc.) or `*jmap.MethodError`

Account IDs are resolved from `client.Session.PrimaryAccounts[mail.URI]` (mail tools) or `client.Session.PrimaryAccounts[sieve.URI]` (sieve tools).

### Tool list

Tools map closely to JMAP methods. Email mutation tools (`email_move`, `email_flag`, `email_delete`, `email_create`) are convenience wrappers that translate structured input into `Email/set` patches.

| Tool | JMAP Method | File |
|---|---|---|
| `mailbox_get` | `Mailbox/get` | tools.go |
| `mailbox_set` | `Mailbox/set` (create/update/destroy) | tools_mailbox_mutate.go |
| `email_query` | `Email/query` | tools.go |
| `email_get` | `Email/get` (multi-ID) | tools.go |
| `email_create` | `Mailbox/get` + `Email/set` (create draft) | tools_email_mutate.go |
| `email_move` | `Email/set` (update mailboxIds) | tools_email_mutate.go |
| `email_flag` | `Email/set` (update keywords) | tools_email_mutate.go |
| `email_delete` | `Mailbox/get` + `Email/set` (trash or destroy) | tools_email_mutate.go |
| `identity_get` | `Identity/get` | tools_email_send.go |
| `email_submission_set` | `Mailbox/get` + `Identity/get` + `EmailSubmission/set` | tools_email_send.go |
| `sieve_get` | `SieveScript/get` (+ blob download when ID given) | tools_sieve.go |
| `sieve_set` | blob upload + `SieveScript/set` (create/update/destroy) | tools_sieve.go |
| `sieve_validate` | blob upload + `SieveScript/validate` | tools_sieve.go |

`email_submission_set` is feature-gated behind the `-enable-send` CLI flag (default `false`). Without this flag, the tool is not registered and not visible to MCP clients.

`sieve_get`, `sieve_set`, `sieve_validate` are feature-gated behind the `-enable-sieve` CLI flag (default `false`). Not all JMAP servers support Sieve (e.g. Fastmail does not advertise `urn:ietf:params:jmap:sieve`).

### Tool naming

MCP tool names allow `[a-zA-Z0-9_\-.]`, max 128 chars. Use underscore-separated: `mailbox_get`, `email_query`, `sieve_get`.

## Go MCP SDK Reference (go-sdk v1.3.x)

Import: `github.com/modelcontextprotocol/go-sdk/mcp`

### Key APIs

- `mcp.NewServer(impl, opts)` — create server
- `mcp.AddTool(server, tool, handler)` — generic typed tool registration (derives JSON schema from Go types)
- `server.AddPrompt(prompt, handler)` — register prompt
- `server.AddResource(resource, handler)` / `server.AddResourceTemplate(template, handler)`
- `mcp.StdioTransport{}` / `mcp.NewStreamableHTTPHandler()`
- Handler signature: `func(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)`
- Struct tags: `json:"name"` for field names, `jsonschema:"description"` for schema descriptions

## JMAP Concepts

- **Session**: `GET` session URL with bearer token → returns account IDs, capabilities, API URL
- **Method calls**: batched JSON `POST` to API URL (`Email/query`, `Email/get`, `Mailbox/get`, `SieveScript/get`, etc.)
- **Result references**: back-references within a batch (`#ref`) to chain method calls without round-trips
- **Account ID**: scopes all JMAP operations; resolved from `session.PrimaryAccounts[capability.URI]`
- **Blob upload/download**: binary data exchange for script content; upload returns `BlobID` for use in method calls
