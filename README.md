# jmap-mcp

MCP server exposing JMAP email and Sieve script operations as tools.

Built with the Go MCP SDK ([go-sdk](https://github.com/modelcontextprotocol/go-sdk))
and [go-jmap](https://github.com/mikluko/jmap) library.

## Tools

Tools map closely to JMAP methods. Email mutation tools provide structured convenience wrappers over `Email/set` patches.

### Mailbox (RFC 8621)

| Tool           | JMAP Method    | Description                                         |
|----------------|----------------|-----------------------------------------------------|
| `mailbox_get`  | `Mailbox/get`  | Get mailboxes by ID, or list all                    |
| `mailbox_set`  | `Mailbox/set`  | Create, update, or destroy mailboxes                |

### Email (RFC 8621)

| Tool           | JMAP Method  | Description                                                    |
|----------------|--------------|----------------------------------------------------------------|
| `email_query`  | `Email/query`| Search emails with filters, returns IDs and total count        |
| `email_get`    | `Email/get`  | Get full content of emails by ID                               |
| `email_create` | `Email/set`  | Create a new email draft in the Drafts mailbox                 |
| `email_move`   | `Email/set`  | Move emails to a different mailbox                             |
| `email_flag`   | `Email/set`  | Set or remove flags (seen, flagged, answered, draft)           |
| `email_delete` | `Email/set`  | Delete emails (move to Trash or permanently destroy)           |

### Identity

| Tool           | JMAP Method    | Description                                       |
|----------------|----------------|---------------------------------------------------|
| `identity_get` | `Identity/get` | List sender identities (email addresses)          |

### Submission (feature-gated)

| Tool                   | JMAP Method            | Description                                        |
|------------------------|------------------------|----------------------------------------------------|
| `email_submission_set` | `EmailSubmission/set`  | Submit a draft for delivery (requires `-enable-send`) |

### Sieve Scripts (RFC 9425)

| Tool             | JMAP Method            | Description                                          |
|------------------|------------------------|------------------------------------------------------|
| `sieve_get`      | `SieveScript/get`      | List all scripts, or get one with full content       |
| `sieve_set`      | `SieveScript/set`      | Create, update, or destroy Sieve scripts             |
| `sieve_validate` | `SieveScript/validate` | Validate a Sieve script without saving               |

## Configuration

| Env var            | Required   | Description                                                          |
|--------------------|------------|----------------------------------------------------------------------|
| `JMAP_SESSION_URL` | always     | JMAP session endpoint (e.g. `https://api.fastmail.com/jmap/session`) |
| `JMAP_AUTH_TOKEN`  | stdio mode | Bearer token for JMAP authentication                                 |

| Flag             | Default | Description                                    |
|------------------|---------|------------------------------------------------|
| `-mode`          | `stdio` | Server mode: `stdio` or `http`                 |
| `-listen`        | `:8080` | HTTP listen address (http mode only)           |
| `-enable-send`   | `false` | Enable the `email_submission_set` tool (off by default) |

In HTTP mode, the token can come from the `jmap_token` query parameter.

## Installation

### Kubernetes (Helm)

```bash
helm install jmap-mcp oci://ghcr.io/mikluko/helm-charts/jmap-mcp \
  --version 0.1.0 \
  --set jmap.sessionURL="https://api.fastmail.com/jmap/session" \
  --set jmap.authToken="your-token"
```

### Binary

```bash
go install github.com/mikluko/jmap-mcp@latest
```

### From source

```bash
git clone https://github.com/mikluko/jmap-mcp.git
cd jmap-mcp
make install
```

## Usage

```bash
# stdio mode (default)
export JMAP_SESSION_URL=https://api.fastmail.com/jmap/session
export JMAP_AUTH_TOKEN=your-token
./jmap-mcp

# HTTP mode
./jmap-mcp -mode http -listen :8080
```

## Build

```bash
make build # Build and push container image + Helm chart
make image # Build and push container image with ko
make package # Package and push Helm chart to OCI registry
make test # Run tests
make install # Install binary locally
```

## References

- [RFC 8620](https://www.rfc-editor.org/rfc/rfc8620) — JMAP Core
- [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621) — JMAP Mail
- [RFC 9425](https://www.rfc-editor.org/rfc/rfc9425) — JMAP Sieve Scripts
- [MCP Specification](https://modelcontextprotocol.io/specification)
