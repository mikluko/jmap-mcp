package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/mikluko/jmap"
	"github.com/mikluko/jmap/mail"
	"github.com/mikluko/jmap/mail/mailbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- mailbox_get ---

type MailboxGetInput struct {
	IDs []string `json:"ids,omitempty" jsonschema:"Mailbox IDs to retrieve (omit to get all mailboxes)"`
}

var mailboxGetTool = &mcp.Tool{
	Name:        "mailbox_get",
	Description: "Get mailboxes by ID, or list all mailboxes with names, roles, and email counts. Use this first to discover mailbox IDs for other tools.",
	Annotations: readOnlyAnnotations,
}

func (s *Server) handleMailboxGet(ctx context.Context, _ *mcp.CallToolRequest, in MailboxGetInput) (*mcp.CallToolResult, any, error) {
	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	get := &mailbox.Get{Account: accountID}
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
		return errorResult(fmt.Errorf("empty response for Mailbox/get")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *mailbox.GetResponse:
		if len(args.NotFound) > 0 {
			return errorResult(fmt.Errorf("mailboxes not found: %v", args.NotFound)), nil, nil
		}
		var sb strings.Builder
		for _, mb := range args.List {
			role := string(mb.Role)
			if role == "" {
				role = "folder"
			}
			fmt.Fprintf(&sb, "%s (%s) — %d emails, %d unread [id: %s]\n",
				mb.Name, role, mb.TotalEmails, mb.UnreadEmails, mb.ID)
		}
		return textResult(sb.String()), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}

// --- mailbox_set ---

type MailboxSetCreate struct {
	Name     string `json:"name" jsonschema:"Mailbox name"`
	ParentID string `json:"parent_id,omitempty" jsonschema:"Parent mailbox ID (omit for top-level)"`
}

type MailboxSetUpdate struct {
	Name     string  `json:"name,omitempty" jsonschema:"New name"`
	ParentID *string `json:"parent_id,omitempty" jsonschema:"New parent mailbox ID (null to move to top-level)"`
}

type MailboxSetInput struct {
	Create                map[string]MailboxSetCreate `json:"create,omitempty" jsonschema:"Mailboxes to create keyed by creation ID"`
	Update                map[string]MailboxSetUpdate `json:"update,omitempty" jsonschema:"Mailboxes to update keyed by mailbox ID"`
	Destroy               []string                   `json:"destroy,omitempty" jsonschema:"Mailbox IDs to destroy"`
	OnDestroyRemoveEmails bool                        `json:"on_destroy_remove_emails,omitempty" jsonschema:"Also destroy emails that are only in destroyed mailboxes"`
}

var mailboxSetTool = &mcp.Tool{
	Name:        "mailbox_set",
	Description: "Create, update, or destroy mailboxes. Supports batch operations: create new folders, rename or reparent existing ones, or destroy by ID.",
	Annotations: destructiveAnnotations,
}

func (s *Server) handleMailboxSet(ctx context.Context, _ *mcp.CallToolRequest, in MailboxSetInput) (*mcp.CallToolResult, any, error) {
	if len(in.Create) == 0 && len(in.Update) == 0 && len(in.Destroy) == 0 {
		return errorResult(fmt.Errorf("at least one of create, update, or destroy must be provided")), nil, nil
	}

	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	set := &mailbox.Set{
		Account:               accountID,
		OnDestroyRemoveEmails: in.OnDestroyRemoveEmails,
	}

	if len(in.Create) > 0 {
		set.Create = make(map[jmap.ID]*mailbox.Mailbox, len(in.Create))
		for cid, c := range in.Create {
			mb := &mailbox.Mailbox{Name: c.Name}
			if c.ParentID != "" {
				mb.ParentID = jmap.ID(c.ParentID)
			}
			set.Create[jmap.ID(cid)] = mb
		}
	}

	if len(in.Update) > 0 {
		set.Update = make(map[jmap.ID]jmap.Patch, len(in.Update))
		for id, u := range in.Update {
			patch := jmap.Patch{}
			if u.Name != "" {
				patch["name"] = u.Name
			}
			if u.ParentID != nil {
				if *u.ParentID == "" {
					patch["parentId"] = nil
				} else {
					patch["parentId"] = *u.ParentID
				}
			}
			if len(patch) == 0 {
				continue
			}
			set.Update[jmap.ID(id)] = patch
		}
	}

	if len(in.Destroy) > 0 {
		set.Destroy = toJMAPIDSlice(in.Destroy)
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(set)

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for Mailbox/set")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *mailbox.SetResponse:
		var sb strings.Builder
		var errors []string

		for cid, mb := range args.Created {
			fmt.Fprintf(&sb, "Created mailbox %s [id: %s]\n", cid, mb.ID)
		}
		for cid, se := range args.NotCreated {
			errors = append(errors, fmt.Sprintf("create %s: %s", cid, se.Type))
		}
		for id := range args.Updated {
			fmt.Fprintf(&sb, "Updated mailbox %s\n", id)
		}
		for id, se := range args.NotUpdated {
			errors = append(errors, fmt.Sprintf("update %s: %s", id, se.Type))
		}
		for _, id := range args.Destroyed {
			fmt.Fprintf(&sb, "Destroyed mailbox %s\n", id)
		}
		for id, se := range args.NotDestroyed {
			errors = append(errors, fmt.Sprintf("destroy %s: %s", id, se.Type))
		}

		if len(errors) > 0 {
			fmt.Fprintf(&sb, "Errors: %s\n", strings.Join(errors, "; "))
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
			}, nil, nil
		}
		return textResult(sb.String()), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}
