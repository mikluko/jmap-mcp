package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mikluko/jmap"
	"github.com/mikluko/jmap/mail"
	"github.com/mikluko/jmap/mail/email"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- email_attachment_url ---

type EmailAttachmentURLInput struct {
	EmailID string `json:"email_id" jsonschema:"ID of the email containing the attachment"`
	BlobID  string `json:"blob_id,omitempty" jsonschema:"Blob ID of the attachment to download. Optional when the email has exactly one attachment. Blob IDs are listed by email_get."`
}

var emailAttachmentURLTool = &mcp.Tool{
	Name:        "email_attachment_url",
	Description: "Return a signed URL that streams an email attachment directly from the mail server. The URL requires no authentication and EXPIRES 30 SECONDS after issuance — fetch it immediately (e.g. with curl or a download tool), do not store it for later. Use email_get to list attachments and their blob IDs.",
	Annotations: readOnlyAnnotations,
}

func (s *Server) handleEmailAttachmentURL(ctx context.Context, _ *mcp.CallToolRequest, in EmailAttachmentURLInput) (*mcp.CallToolResult, any, error) {
	_, accountID, part, err := s.fetchAttachmentPart(ctx, in.EmailID, in.BlobID)
	if err != nil {
		return errorResult(err), nil, nil
	}

	base := s.externalURL
	if base == "" {
		base = BaseURLFromContext(ctx)
	}
	if base == "" {
		return errorResult(fmt.Errorf("no external base URL available; signed attachment URLs require http mode")), nil, nil
	}

	token, err := s.resolveToken(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	expiresAt := time.Now().Add(attachmentURLTTL).UTC()
	opaque, err := s.attachmentURL.seal(&attachmentClaims{
		Token:   token,
		Account: string(accountID),
		Blob:    string(part.BlobID),
		Name:    part.Name,
		Type:    part.Type,
		Exp:     expiresAt.Unix(),
	})
	if err != nil {
		return errorResult(fmt.Errorf("seal attachment URL: %w", err)), nil, nil
	}

	name := part.Name
	if name == "" {
		name = "(unnamed)"
	}
	return textResult(fmt.Sprintf(
		"Attachment %s (%s, %d bytes) available for the next %d seconds at:\n%s/attachments/%s\nFetch it now; the link expires at %s.",
		name, part.Type, part.Size, int(attachmentURLTTL.Seconds()),
		strings.TrimSuffix(base, "/"), opaque,
		expiresAt.Format(time.RFC3339),
	)), nil, nil
}

// --- shared attachment helpers ---

// fetchAttachmentPart resolves an email's attachment part by blob ID (or the
// sole attachment), returning the authenticated client and account for the
// subsequent blob download.
func (s *Server) fetchAttachmentPart(ctx context.Context, emailID, blobID string) (*jmap.Client, jmap.ID, *email.BodyPart, error) {
	if emailID == "" {
		return nil, "", nil, fmt.Errorf("email_id is required")
	}

	client, err := s.jmapClient(ctx)
	if err != nil {
		return nil, "", nil, err
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return nil, "", nil, fmt.Errorf("no primary mail account")
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(&email.Get{
		Account:    accountID,
		IDs:        []jmap.ID{jmap.ID(emailID)},
		Properties: []string{"id", "attachments"},
	})

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", nil, err
	}

	if len(resp.Responses) == 0 {
		return nil, "", nil, fmt.Errorf("empty response for Email/get")
	}

	var attachments []*email.BodyPart
	switch args := resp.Responses[0].Args.(type) {
	case *email.GetResponse:
		if len(args.NotFound) > 0 || len(args.List) == 0 {
			return nil, "", nil, fmt.Errorf("email not found: %s", emailID)
		}
		attachments = args.List[0].Attachments
	case *jmap.MethodError:
		return nil, "", nil, args
	default:
		return nil, "", nil, fmt.Errorf("unexpected response type: %T", args)
	}

	part, err := selectAttachment(attachments, blobID)
	if err != nil {
		return nil, "", nil, err
	}
	return client, accountID, part, nil
}

// selectAttachment picks the attachment matching blobID, or the sole
// attachment when blobID is empty.
func selectAttachment(attachments []*email.BodyPart, blobID string) (*email.BodyPart, error) {
	if len(attachments) == 0 {
		return nil, fmt.Errorf("email has no attachments")
	}
	if blobID == "" {
		if len(attachments) == 1 {
			return attachments[0], nil
		}
		return nil, fmt.Errorf("email has %d attachments, blob_id is required:\n%s", len(attachments), formatAttachmentList(attachments, ""))
	}
	for _, part := range attachments {
		if string(part.BlobID) == blobID {
			return part, nil
		}
	}
	return nil, fmt.Errorf("no attachment with blob ID %s, available:\n%s", blobID, formatAttachmentList(attachments, ""))
}

// formatAttachmentList renders one line per attachment, prefixed with indent.
func formatAttachmentList(attachments []*email.BodyPart, indent string) string {
	var sb strings.Builder
	for i, part := range attachments {
		if i > 0 {
			sb.WriteByte('\n')
		}
		name := part.Name
		if name == "" {
			name = "(unnamed)"
		}
		fmt.Fprintf(&sb, "%s%s (%s, %d bytes) [blob: %s]", indent, name, part.Type, part.Size, part.BlobID)
	}
	return sb.String()
}
