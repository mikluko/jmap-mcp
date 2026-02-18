package server

import (
	"context"
	"fmt"

	"github.com/mikluko/jmap"
	"github.com/mikluko/jmap/mail/mailbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverInstructions = `JMAP MCP Server — email and Sieve script management via JMAP protocol.

## Key concepts

- **Mailbox**: a folder (Inbox, Drafts, Sent, Trash, etc.). Each has an ID and a role.
- **Email**: belongs to one or more mailboxes, identified by an opaque ID.
- **Identity**: a sender address the user may send from.
- **Sieve script**: server-side email filtering rules (auto-filing, vacation replies, etc.).

## Common workflows

**Reading email**: call mailbox_get to discover mailbox IDs and roles, then email_query with filters to get matching email IDs, then email_get with those IDs to retrieve full content.

**Sending email**: call email_create to compose a draft (saved in Drafts), then email_submission_set with the draft ID to submit for delivery (automatically moves from Drafts to Sent).

**Managing email**: use email_move to move between mailboxes, email_flag to mark as read/flagged/answered, email_delete to trash or permanently destroy.

**Managing mailboxes**: use mailbox_set to create, rename, reparent, or destroy mailboxes.

**Sieve scripts**: use sieve_get to list or read scripts, sieve_set to create/update/destroy, sieve_validate to check syntax without saving.

## Important notes

- All tool inputs use opaque string IDs. Get IDs from other tools first (mailbox_get, email_query, identity_get, sieve_get).
- email_query returns only IDs and total count; always follow up with email_get for content.
- email_submission_set may not be available — it requires the server to be started with -enable-send flag.
`

// annotation helpers
func boolPtr(v bool) *bool { return &v }

var (
	readOnlyAnnotations = &mcp.ToolAnnotations{
		ReadOnlyHint: true,
	}
	idempotentAnnotations = &mcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
		IdempotentHint:  true,
	}
	destructiveAnnotations = &mcp.ToolAnnotations{
		DestructiveHint: boolPtr(true),
	}
	mutatingAnnotations = &mcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
	}
)

// registerTools registers all JMAP tools with the MCP server.
func (s *Server) registerTools() {
	// Mailbox tools (Mailbox/get, Mailbox/set)
	mcp.AddTool(s.mcp, mailboxGetTool, s.handleMailboxGet)
	mcp.AddTool(s.mcp, mailboxSetTool, s.handleMailboxSet)

	// Email tools (Email/query, Email/get, Email/set convenience wrappers)
	mcp.AddTool(s.mcp, emailQueryTool, s.handleEmailQuery)
	mcp.AddTool(s.mcp, emailGetTool, s.handleEmailGet)
	mcp.AddTool(s.mcp, emailCreateTool, s.handleEmailCreate)
	mcp.AddTool(s.mcp, emailMoveTool, s.handleEmailMove)
	mcp.AddTool(s.mcp, emailFlagTool, s.handleEmailFlag)
	mcp.AddTool(s.mcp, emailDeleteTool, s.handleEmailDelete)

	// Identity tools (Identity/get)
	mcp.AddTool(s.mcp, identityGetTool, s.handleIdentityGet)

	// Feature-gated: email_submission_set requires -enable-send flag
	if s.enableSend {
		mcp.AddTool(s.mcp, emailSubmissionSetTool, s.handleEmailSubmissionSet)
	}

	// Sieve tools (SieveScript/get, SieveScript/set, SieveScript/validate)
	mcp.AddTool(s.mcp, sieveGetTool, s.handleSieveGet)
	mcp.AddTool(s.mcp, sieveSetTool, s.handleSieveSet)
	mcp.AddTool(s.mcp, sieveValidateTool, s.handleSieveValidate)
}

// --- shared helpers ---

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

func toJMAPIDSlice(ids []string) []jmap.ID {
	result := make([]jmap.ID, len(ids))
	for i, id := range ids {
		result[i] = jmap.ID(id)
	}
	return result
}

// findMailboxByRole fetches all mailboxes and returns the ID of the one matching the given role.
func (s *Server) findMailboxByRole(ctx context.Context, client *jmap.Client, accountID jmap.ID, role mailbox.Role) (jmap.ID, error) {
	req := &jmap.Request{Context: ctx}
	req.Invoke(&mailbox.Get{Account: accountID})

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("mailbox lookup: %w", err)
	}

	if len(resp.Responses) == 0 {
		return "", fmt.Errorf("empty response for Mailbox/get")
	}

	switch args := resp.Responses[0].Args.(type) {
	case *mailbox.GetResponse:
		for _, mb := range args.List {
			if mb.Role == role {
				return mb.ID, nil
			}
		}
		return "", fmt.Errorf("no mailbox with role %q found", role)
	case *jmap.MethodError:
		return "", args
	default:
		return "", fmt.Errorf("unexpected response type: %T", args)
	}
}
