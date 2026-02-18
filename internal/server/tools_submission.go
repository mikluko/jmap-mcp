package server

import (
	"context"
	"fmt"

	"github.com/mikluko/jmap"
	"github.com/mikluko/jmap/mail"
	"github.com/mikluko/jmap/mail/emailsubmission"
	"github.com/mikluko/jmap/mail/identity"
	"github.com/mikluko/jmap/mail/mailbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- email_submission_set ---

type EmailSubmissionSetInput struct {
	EmailID    string `json:"email_id" jsonschema:"ID of the email to submit for delivery"`
	IdentityID string `json:"identity_id,omitempty" jsonschema:"Sender identity ID (auto-detected if omitted)"`
}

var emailSubmissionSetTool = &mcp.Tool{
	Name:        "email_submission_set",
	Description: "Submit an email for delivery and move it from Drafts to Sent (requires -enable-send flag) (maps to JMAP EmailSubmission/set)",
}

func (s *Server) handleEmailSubmissionSet(ctx context.Context, _ *mcp.CallToolRequest, in EmailSubmissionSetInput) (*mcp.CallToolResult, any, error) {
	if in.EmailID == "" {
		return errorResult(fmt.Errorf("email_id is required")), nil, nil
	}

	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	// Discovery request: fetch mailboxes (for Drafts + Sent) and identities.
	discoverReq := &jmap.Request{Context: ctx}
	discoverReq.Invoke(&mailbox.Get{Account: accountID})
	discoverReq.Invoke(&identity.Get{Account: accountID})

	discoverResp, err := client.Do(discoverReq)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(discoverResp.Responses) < 2 {
		return errorResult(fmt.Errorf("expected 2 discovery responses, got %d", len(discoverResp.Responses))), nil, nil
	}

	// Find Drafts and Sent mailbox IDs.
	var draftsID, sentID jmap.ID
	switch args := discoverResp.Responses[0].Args.(type) {
	case *mailbox.GetResponse:
		for _, mb := range args.List {
			switch mb.Role {
			case mailbox.RoleDrafts:
				draftsID = mb.ID
			case mailbox.RoleSent:
				sentID = mb.ID
			}
		}
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected mailbox response type: %T", args)), nil, nil
	}

	if draftsID == "" {
		return errorResult(fmt.Errorf("no Drafts mailbox found")), nil, nil
	}
	if sentID == "" {
		return errorResult(fmt.Errorf("no Sent mailbox found")), nil, nil
	}

	// Resolve sender identity.
	identityID := jmap.ID(in.IdentityID)
	switch args := discoverResp.Responses[1].Args.(type) {
	case *identity.GetResponse:
		if identityID == "" {
			if len(args.List) == 0 {
				return errorResult(fmt.Errorf("no sender identities available")), nil, nil
			}
			identityID = args.List[0].ID
		}
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected identity response type: %T", args)), nil, nil
	}

	// Submit the email for delivery.
	submitReq := &jmap.Request{Context: ctx}
	submitReq.Invoke(&emailsubmission.Set{
		Account: accountID,
		Create: map[jmap.ID]*emailsubmission.EmailSubmission{
			"send": {
				IdentityID: identityID,
				EmailID:    jmap.ID(in.EmailID),
			},
		},
		OnSuccessUpdateEmail: map[jmap.ID]jmap.Patch{
			"#send": {
				"mailboxIds/" + string(draftsID): nil,
				"mailboxIds/" + string(sentID):   true,
				"keywords/$draft":                nil,
			},
		},
	})

	submitResp, err := client.Do(submitReq)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(submitResp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for EmailSubmission/set")), nil, nil
	}

	switch args := submitResp.Responses[0].Args.(type) {
	case *emailsubmission.SetResponse:
		if se, ok := args.NotCreated["send"]; ok {
			return errorResult(fmt.Errorf("submission failed: %s", se.Type)), nil, nil
		}
		return textResult(fmt.Sprintf("Email %s submitted for delivery", in.EmailID)), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}
