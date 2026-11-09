package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sarak.sh/mikluko/jmap"
	"sarak.sh/mikluko/jmap/mail"
	"sarak.sh/mikluko/jmap/mail/identity"
)

// --- identity_get ---

type IdentityGetInput struct {
	IDs []string `json:"ids,omitempty" jsonschema:"Identity IDs to retrieve (omit to get all)"`
}

var identityGetTool = &mcp.Tool{
	Name:        "identity_get",
	Description: "Get sender identities — the email addresses the user may send from, with the text and HTML signatures email_create signs with. Useful before email_create to choose or verify the sender identity.",
	Annotations: readOnlyAnnotations,
}

// formatIdentity renders ident as a line, followed by a quoted line per
// signature it has.
func formatIdentity(ident *identity.Identity) string {
	name := ident.Name
	if name == "" {
		name = "(unnamed)"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s <%s> [id: %s]\n", name, ident.Email, ident.ID)
	if ident.TextSignature != "" {
		fmt.Fprintf(&sb, "  textSignature: %q\n", ident.TextSignature)
	}
	if ident.HTMLSignature != "" {
		fmt.Fprintf(&sb, "  htmlSignature: %q\n", ident.HTMLSignature)
	}
	return sb.String()
}

func (s *Server) handleIdentityGet(ctx context.Context, _ *mcp.CallToolRequest, in IdentityGetInput) (*mcp.CallToolResult, any, error) {
	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	get := &identity.Get{Account: accountID}
	if len(in.IDs) > 0 {
		get.IDs = toJMAPIDSlice(in.IDs)
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(get)

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for Identity/get")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *identity.GetResponse:
		if len(args.NotFound) > 0 {
			return errorResult(fmt.Errorf("identities not found: %v", args.NotFound)), nil, nil
		}
		var sb strings.Builder
		for _, id := range args.List {
			sb.WriteString(formatIdentity(id))
		}
		if len(args.List) == 0 {
			sb.WriteString("No sender identities found.\n")
		}
		return textResult(sb.String()), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}
