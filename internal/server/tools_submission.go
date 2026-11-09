package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sarak.sh/mikluko/jmap"
	"sarak.sh/mikluko/jmap/mail"
	"sarak.sh/mikluko/jmap/mail/email"
	"sarak.sh/mikluko/jmap/mail/emailsubmission"
	"sarak.sh/mikluko/jmap/mail/identity"
	"sarak.sh/mikluko/jmap/mail/mailbox"
)

// --- email_submission_set ---

type EmailSubmissionSetInput struct {
	EmailID    string `json:"email_id" jsonschema:"ID of the email to submit for delivery"`
	IdentityID string `json:"identity_id,omitempty" jsonschema:"Sender identity ID (the one whose address is the draft's From if omitted)"`
}

var emailSubmissionSetTool = &mcp.Tool{
	Name:        "email_submission_set",
	Description: "Submit a draft email for delivery. Automatically moves it from Drafts to Sent and removes the draft flag. Create the draft first with email_create; a draft without From is refused. Identity defaults to the one whose address is the draft's From.",
	Annotations: mutatingAnnotations,
}

// submissionIdentity returns the identity to submit a draft with: the one named
// by id, else the one whose address is the draft's first From, else the first.
// A draft without From is an error: servers send it without a From header.
func submissionIdentity(list []*identity.Identity, id jmap.ID, from []*mail.Address) (*identity.Identity, error) {
	if len(from) == 0 {
		return nil, fmt.Errorf("draft has no From; create it with email_create")
	}
	if id == "" {
		for _, ident := range list {
			if strings.EqualFold(ident.Email, from[0].Email) {
				return ident, nil
			}
		}
	}
	return senderIdentity(list, id)
}

type submissionDiscovery struct {
	draftsID, sentID jmap.ID
	identities       []*identity.Identity
	from             []*mail.Address
}

// discoverSubmission fetches, in one request, the Drafts and Sent mailboxes,
// the sender identities and the From of the draft emailID.
func (s *Server) discoverSubmission(ctx context.Context, client *jmap.Client, accountID, emailID jmap.ID) (*submissionDiscovery, error) {
	req := &jmap.Request{Context: ctx}
	req.Invoke(&mailbox.Get{Account: accountID})
	req.Invoke(&identity.Get{Account: accountID})
	req.Invoke(&email.Get{Account: accountID, IDs: []jmap.ID{emailID}, Properties: []string{"from"}})

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if len(resp.Responses) < 3 {
		return nil, fmt.Errorf("expected 3 discovery responses, got %d", len(resp.Responses))
	}

	d := &submissionDiscovery{}
	switch args := resp.Responses[0].Args.(type) {
	case *mailbox.GetResponse:
		for _, mb := range args.List {
			switch mb.Role {
			case mailbox.RoleDrafts:
				d.draftsID = mb.ID
			case mailbox.RoleSent:
				d.sentID = mb.ID
			}
		}
	case *jmap.MethodError:
		return nil, args
	default:
		return nil, fmt.Errorf("unexpected mailbox response type: %T", args)
	}
	if d.draftsID == "" {
		return nil, fmt.Errorf("no Drafts mailbox found")
	}
	if d.sentID == "" {
		return nil, fmt.Errorf("no Sent mailbox found")
	}

	switch args := resp.Responses[1].Args.(type) {
	case *identity.GetResponse:
		d.identities = args.List
	case *jmap.MethodError:
		return nil, args
	default:
		return nil, fmt.Errorf("unexpected identity response type: %T", args)
	}

	switch args := resp.Responses[2].Args.(type) {
	case *email.GetResponse:
		if len(args.List) == 0 {
			return nil, fmt.Errorf("no email %s", emailID)
		}
		d.from = args.List[0].From
	case *jmap.MethodError:
		return nil, args
	default:
		return nil, fmt.Errorf("unexpected email response type: %T", args)
	}
	return d, nil
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

	d, err := s.discoverSubmission(ctx, client, accountID, jmap.ID(in.EmailID))
	if err != nil {
		return errorResult(err), nil, nil
	}
	draftsID, sentID := d.draftsID, d.sentID
	sender, err := submissionIdentity(d.identities, jmap.ID(in.IdentityID), d.from)
	if err != nil {
		return errorResult(err), nil, nil
	}
	identityID := sender.ID

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
