package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/k3a/html2text"
	"github.com/mikluko/jmap"
	"github.com/mikluko/jmap/mail"
	"github.com/mikluko/jmap/mail/email"
	"github.com/mikluko/jmap/mail/mailbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- email_query ---

type EmailQueryInput struct {
	MailboxID     string   `json:"mailbox_id,omitempty" jsonschema:"ID of the mailbox to search in"`
	Query         string   `json:"query,omitempty" jsonschema:"Full-text search query"`
	From          string   `json:"from,omitempty" jsonschema:"Filter by sender address"`
	To            string   `json:"to,omitempty" jsonschema:"Filter by recipient address"`
	Subject       string   `json:"subject,omitempty" jsonschema:"Filter by subject text"`
	Before        string   `json:"before,omitempty" jsonschema:"Emails before this date (RFC 3339 or YYYY-MM-DD)"`
	After         string   `json:"after,omitempty" jsonschema:"Emails after this date (RFC 3339 or YYYY-MM-DD)"`
	HasAttachment *bool    `json:"has_attachment,omitempty" jsonschema:"Filter by attachment presence"`
	Limit         int      `json:"limit,omitempty" jsonschema:"Maximum number of results (default 20)"`
	Headers       []string `json:"headers,omitempty" jsonschema:"Header names to include in results (e.g. List-Id, Message-ID)"`
}

var emailQueryTool = &mcp.Tool{
	Name:        "email_query",
	Description: "Search emails with filters. Returns ID, subject, sender, date, and size (bytes) for each match. Optionally include specific headers (e.g. List-Id, Message-ID) via the headers parameter. Use email_get to retrieve full content. Sorted by date descending.",
	Annotations: readOnlyAnnotations,
}

func (s *Server) handleEmailQuery(ctx context.Context, _ *mcp.CallToolRequest, in EmailQueryInput) (*mcp.CallToolResult, any, error) {
	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	filter := &email.FilterCondition{
		InMailbox: jmap.ID(in.MailboxID),
		Text:      in.Query,
		From:      in.From,
		To:        in.To,
		Subject:   in.Subject,
	}
	if in.HasAttachment != nil && *in.HasAttachment {
		filter.HasAttachment = true
	}
	if in.Before != "" {
		t, err := parseDate(in.Before, "T23:59:59Z")
		if err != nil {
			return errorResult(err), nil, nil
		}
		filter.Before = t
	}
	if in.After != "" {
		t, err := parseDate(in.After, "T00:00:00Z")
		if err != nil {
			return errorResult(err), nil, nil
		}
		filter.After = t
	}

	limit := uint64(in.Limit)
	if limit == 0 {
		limit = 20
	}

	req := &jmap.Request{Context: ctx}
	queryCallID := req.Invoke(&email.Query{
		Account:        accountID,
		Filter:         filter,
		Sort:           []*email.SortComparator{{Property: "receivedAt", IsAscending: false}},
		Limit:          limit,
		CalculateTotal: true,
	})

	// Chain Email/get via back-reference to fetch summary fields in one round-trip.
	properties := []string{"id", "subject", "from", "receivedAt", "size"}
	if len(in.Headers) > 0 {
		properties = append(properties, "headers")
	}
	req.Invoke(&email.Get{
		Account: accountID,
		ReferenceIDs: &jmap.ResultReference{
			ResultOf: queryCallID,
			Name:     "Email/query",
			Path:     "/ids",
		},
		Properties: properties,
	})

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for Email/query")), nil, nil
	}

	// First response: Email/query
	var total uint64
	switch args := resp.Responses[0].Args.(type) {
	case *email.QueryResponse:
		total = args.Total
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}

	// Second response: Email/get with summary properties
	if len(resp.Responses) < 2 {
		return errorResult(fmt.Errorf("missing Email/get response in query chain")), nil, nil
	}

	switch args := resp.Responses[1].Args.(type) {
	case *email.GetResponse:
		var sb strings.Builder
		fmt.Fprintf(&sb, "Total: %d (returning %d)\n\n", total, len(args.List))
		for _, e := range args.List {
			from := ""
			if len(e.From) > 0 {
				from = formatAddresses(e.From)
			}
			date := ""
			if e.ReceivedAt != nil {
				date = e.ReceivedAt.Format("2006-01-02 15:04")
			}
			fmt.Fprintf(&sb, "%s  %s  %s  [%d bytes]  %s\n", e.ID, date, from, e.Size, e.Subject)
			for _, h := range e.Headers {
				for _, want := range in.Headers {
					if strings.EqualFold(h.Name, want) {
						fmt.Fprintf(&sb, "  %s: %s\n", h.Name, strings.TrimSpace(h.Value))
						break
					}
				}
			}
		}
		return textResult(sb.String()), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}

// --- email_get ---

type EmailGetInput struct {
	EmailIDs    []string `json:"email_ids" jsonschema:"IDs of emails to retrieve"`
	FullHeaders bool     `json:"full_headers,omitempty" jsonschema:"Include all raw email headers"`
	MaxChars    int      `json:"max_chars,omitempty" jsonschema:"Maximum total response size in characters (default 50000). When exceeded, remaining emails are omitted with an advisory to fetch fewer at a time."`
}

const defaultMaxChars = 50000

var emailGetTool = &mcp.Tool{
	Name:        "email_get",
	Description: "Get full content of emails by ID, including headers, body text, flags, and mailbox membership. Use email_query first to obtain IDs. Response is capped at max_chars (default 50000); excess emails are omitted with an advisory — reduce batch size if truncated.",
	Annotations: readOnlyAnnotations,
}

func (s *Server) handleEmailGet(ctx context.Context, _ *mcp.CallToolRequest, in EmailGetInput) (*mcp.CallToolResult, any, error) {
	if len(in.EmailIDs) == 0 {
		return errorResult(fmt.Errorf("email_ids is required")), nil, nil
	}

	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	properties := []string{
		"id", "subject", "from", "to", "cc", "bcc", "replyTo",
		"receivedAt", "sentAt", "preview", "hasAttachment", "keywords",
		"mailboxIds", "size", "bodyValues", "textBody", "htmlBody",
	}
	if in.FullHeaders {
		properties = append(properties, "headers")
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(&email.Get{
		Account:            accountID,
		IDs:                toJMAPIDSlice(in.EmailIDs),
		Properties:         properties,
		FetchAllBodyValues: true,
	})

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for Email/get")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *email.GetResponse:
		if len(args.NotFound) > 0 {
			return errorResult(fmt.Errorf("emails not found: %v", args.NotFound)), nil, nil
		}
		if len(args.List) == 0 {
			return errorResult(fmt.Errorf("no emails found")), nil, nil
		}

		maxChars := in.MaxChars
		if maxChars <= 0 {
			maxChars = defaultMaxChars
		}

		var sb strings.Builder
		included := 0
		for i, e := range args.List {
			// Render headers into a temporary buffer.
			var hdr strings.Builder
			if i > 0 {
				fmt.Fprintf(&hdr, "\n---\n\n")
			}
			if in.FullHeaders && len(e.Headers) > 0 {
				for _, h := range e.Headers {
					fmt.Fprintf(&hdr, "%s: %s\n", h.Name, strings.TrimSpace(h.Value))
				}
			} else {
				fmt.Fprintf(&hdr, "ID: %s\n", e.ID)
				fmt.Fprintf(&hdr, "Subject: %s\n", e.Subject)
				if len(e.From) > 0 {
					fmt.Fprintf(&hdr, "From: %s\n", formatAddresses(e.From))
				}
				if len(e.To) > 0 {
					fmt.Fprintf(&hdr, "To: %s\n", formatAddresses(e.To))
				}
				if len(e.CC) > 0 {
					fmt.Fprintf(&hdr, "CC: %s\n", formatAddresses(e.CC))
				}
				if e.ReceivedAt != nil {
					fmt.Fprintf(&hdr, "Date: %s\n", e.ReceivedAt.Format(time.RFC3339))
				}
			}
			fmt.Fprintln(&hdr)

			body := extractBody(e)
			if body == "" {
				body = "(no body content)"
			}

			// Check if appending this email would exceed the limit.
			remaining := maxChars - sb.Len() - hdr.Len()
			if remaining <= 0 {
				omitted := len(args.List) - included
				fmt.Fprintf(&sb, "\n\n--- TRUNCATED: %d of %d emails omitted (response would exceed %d chars). Fetch fewer emails per call. ---\n", omitted, len(args.List), maxChars)
				break
			}

			sb.WriteString(hdr.String())
			sb.WriteString(TruncateBody(body, remaining))
			included++
		}

		return textResult(sb.String()), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}

// --- email_create ---

type EmailCreateInput struct {
	To      []string `json:"to,omitempty" jsonschema:"Recipient email addresses"`
	CC      []string `json:"cc,omitempty" jsonschema:"CC email addresses"`
	BCC     []string `json:"bcc,omitempty" jsonschema:"BCC email addresses"`
	Subject string   `json:"subject" jsonschema:"Email subject"`
	Body    string   `json:"body" jsonschema:"Plain text email body"`
}

var emailCreateTool = &mcp.Tool{
	Name:        "email_create",
	Description: "Create a new email draft in the Drafts mailbox. Returns the draft ID, which can be passed to email_submission_set to send it.",
	Annotations: mutatingAnnotations,
}

func (s *Server) handleEmailCreate(ctx context.Context, _ *mcp.CallToolRequest, in EmailCreateInput) (*mcp.CallToolResult, any, error) {
	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	draftsID, err := s.findMailboxByRole(ctx, client, accountID, mailbox.RoleDrafts)
	if err != nil {
		return errorResult(err), nil, nil
	}

	draft := &email.Email{
		MailboxIDs: map[jmap.ID]bool{draftsID: true},
		Keywords:   map[string]bool{"$draft": true},
		To:         toMailAddresses(in.To),
		CC:         toMailAddresses(in.CC),
		BCC:        toMailAddresses(in.BCC),
		Subject:    in.Subject,
		BodyValues: map[string]*email.BodyValue{
			"body": {Value: in.Body},
		},
		TextBody: []*email.BodyPart{
			{PartID: "body", Type: "text/plain"},
		},
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(&email.Set{
		Account: accountID,
		Create:  map[jmap.ID]*email.Email{"draft": draft},
	})

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for Email/set")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *email.SetResponse:
		if se, ok := args.NotCreated["draft"]; ok {
			return errorResult(fmt.Errorf("draft creation failed: %s", se.Type)), nil, nil
		}
		if created, ok := args.Created["draft"]; ok {
			return textResult(fmt.Sprintf("Created draft [id: %s]", created.ID)), nil, nil
		}
		return textResult("Created draft"), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}

// --- email_move ---

type EmailMoveInput struct {
	EmailIDs  []string `json:"email_ids" jsonschema:"IDs of emails to move"`
	MailboxID string   `json:"mailbox_id" jsonschema:"Destination mailbox ID"`
}

var emailMoveTool = &mcp.Tool{
	Name:        "email_move",
	Description: "Move emails to a different mailbox by ID. Replaces all current mailbox memberships. Use mailbox_get to find the destination mailbox ID.",
	Annotations: idempotentAnnotations,
}

func (s *Server) handleEmailMove(ctx context.Context, _ *mcp.CallToolRequest, in EmailMoveInput) (*mcp.CallToolResult, any, error) {
	if len(in.EmailIDs) == 0 {
		return errorResult(fmt.Errorf("email_ids is required")), nil, nil
	}

	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	updates := make(map[jmap.ID]jmap.Patch, len(in.EmailIDs))
	for _, id := range in.EmailIDs {
		updates[jmap.ID(id)] = jmap.Patch{
			"mailboxIds": map[string]bool{in.MailboxID: true},
		}
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(&email.Set{
		Account: accountID,
		Update:  updates,
	})

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for Email/set")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *email.SetResponse:
		var errors []string
		for id, se := range args.NotUpdated {
			errors = append(errors, fmt.Sprintf("%s: %s", id, se.Type))
		}
		if len(errors) > 0 {
			return errorResult(fmt.Errorf("move failed: %s", strings.Join(errors, "; "))), nil, nil
		}
		return textResult(fmt.Sprintf("Moved %d email(s) to mailbox %s", len(in.EmailIDs), in.MailboxID)), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}

// --- email_flag ---

type EmailFlagInput struct {
	EmailIDs []string `json:"email_ids" jsonschema:"IDs of emails to update"`
	Seen     *bool    `json:"seen,omitempty" jsonschema:"Mark as seen (true) or unseen (false)"`
	Flagged  *bool    `json:"flagged,omitempty" jsonschema:"Mark as flagged/starred (true) or unflagged (false)"`
	Answered *bool    `json:"answered,omitempty" jsonschema:"Mark as answered (true) or unanswered (false)"`
	Draft    *bool    `json:"draft,omitempty" jsonschema:"Mark as draft (true) or not-draft (false)"`
}

var emailFlagTool = &mcp.Tool{
	Name:        "email_flag",
	Description: "Set or remove flags on emails. Supports: seen (read/unread), flagged (starred), answered, draft. Each flag is independent — omit to leave unchanged.",
	Annotations: idempotentAnnotations,
}

func (s *Server) handleEmailFlag(ctx context.Context, _ *mcp.CallToolRequest, in EmailFlagInput) (*mcp.CallToolResult, any, error) {
	if len(in.EmailIDs) == 0 {
		return errorResult(fmt.Errorf("email_ids is required")), nil, nil
	}

	patch := jmap.Patch{}
	applyKeyword(patch, "keywords/$seen", in.Seen)
	applyKeyword(patch, "keywords/$flagged", in.Flagged)
	applyKeyword(patch, "keywords/$answered", in.Answered)
	applyKeyword(patch, "keywords/$draft", in.Draft)

	if len(patch) == 0 {
		return errorResult(fmt.Errorf("at least one flag must be provided")), nil, nil
	}

	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	updates := make(map[jmap.ID]jmap.Patch, len(in.EmailIDs))
	for _, id := range in.EmailIDs {
		updates[jmap.ID(id)] = patch
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(&email.Set{
		Account: accountID,
		Update:  updates,
	})

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for Email/set")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *email.SetResponse:
		var errors []string
		for id, se := range args.NotUpdated {
			errors = append(errors, fmt.Sprintf("%s: %s", id, se.Type))
		}
		if len(errors) > 0 {
			return errorResult(fmt.Errorf("flag update failed: %s", strings.Join(errors, "; "))), nil, nil
		}
		return textResult(fmt.Sprintf("Updated flags on %d email(s)", len(in.EmailIDs))), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}

// --- email_delete ---

type EmailDeleteInput struct {
	EmailIDs  []string `json:"email_ids" jsonschema:"IDs of emails to delete"`
	Permanent bool     `json:"permanent,omitempty" jsonschema:"Permanently destroy emails instead of moving to Trash (default false)"`
}

var emailDeleteTool = &mcp.Tool{
	Name:        "email_delete",
	Description: "Delete emails by moving to Trash (default), or permanently destroy them (permanent=true). Permanent destruction cannot be undone.",
	Annotations: destructiveAnnotations,
}

func (s *Server) handleEmailDelete(ctx context.Context, _ *mcp.CallToolRequest, in EmailDeleteInput) (*mcp.CallToolResult, any, error) {
	if len(in.EmailIDs) == 0 {
		return errorResult(fmt.Errorf("email_ids is required")), nil, nil
	}

	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID := client.Session.PrimaryAccounts[mail.URI]
	if accountID == "" {
		return errorResult(fmt.Errorf("no primary mail account")), nil, nil
	}

	if in.Permanent {
		ids := make([]jmap.ID, len(in.EmailIDs))
		for i, id := range in.EmailIDs {
			ids[i] = jmap.ID(id)
		}

		req := &jmap.Request{Context: ctx}
		req.Invoke(&email.Set{
			Account: accountID,
			Destroy: ids,
		})

		resp, err := client.Do(req)
		if err != nil {
			return errorResult(err), nil, nil
		}

		if len(resp.Responses) == 0 {
			return errorResult(fmt.Errorf("empty response for Email/set")), nil, nil
		}

		switch args := resp.Responses[0].Args.(type) {
		case *email.SetResponse:
			var errors []string
			for id, se := range args.NotDestroyed {
				errors = append(errors, fmt.Sprintf("%s: %s", id, se.Type))
			}
			if len(errors) > 0 {
				return errorResult(fmt.Errorf("destroy failed: %s", strings.Join(errors, "; "))), nil, nil
			}
			return textResult(fmt.Sprintf("Permanently destroyed %d email(s)", len(in.EmailIDs))), nil, nil
		case *jmap.MethodError:
			return errorResult(args), nil, nil
		default:
			return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
		}
	}

	// Soft delete: find Trash mailbox, then move emails there.
	trashID, err := s.findMailboxByRole(ctx, client, accountID, mailbox.RoleTrash)
	if err != nil {
		return errorResult(err), nil, nil
	}

	updates := make(map[jmap.ID]jmap.Patch, len(in.EmailIDs))
	for _, id := range in.EmailIDs {
		updates[jmap.ID(id)] = jmap.Patch{
			"mailboxIds": map[string]bool{string(trashID): true},
		}
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(&email.Set{
		Account: accountID,
		Update:  updates,
	})

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for Email/set")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *email.SetResponse:
		var errors []string
		for id, se := range args.NotUpdated {
			errors = append(errors, fmt.Sprintf("%s: %s", id, se.Type))
		}
		if len(errors) > 0 {
			return errorResult(fmt.Errorf("trash failed: %s", strings.Join(errors, "; "))), nil, nil
		}
		return textResult(fmt.Sprintf("Moved %d email(s) to Trash", len(in.EmailIDs))), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}

// --- email helpers ---

func formatAddresses(addrs []*mail.Address) string {
	parts := make([]string, len(addrs))
	for i, a := range addrs {
		parts[i] = a.String()
	}
	return strings.Join(parts, ", ")
}

func extractBody(e *email.Email) string {
	for _, part := range e.TextBody {
		if bv, ok := e.BodyValues[part.PartID]; ok {
			return PrepareBody(bv.Value, 0)
		}
	}
	for _, part := range e.HTMLBody {
		if bv, ok := e.BodyValues[part.PartID]; ok {
			return PrepareBody(html2text.HTML2Text(StripBlockquotes(bv.Value)), 0)
		}
	}
	return ""
}

// parseDate parses a date string as RFC 3339, normalizing bare dates (YYYY-MM-DD)
// by appending the given time suffix first.
func parseDate(s, timeSuffix string) (*time.Time, error) {
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		s = s + timeSuffix
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, fmt.Errorf("invalid date format %q: expected YYYY-MM-DD or RFC 3339", s)
	}
	return &t, nil
}

// toMailAddresses converts a slice of email strings to JMAP Address objects.
func toMailAddresses(addrs []string) []*mail.Address {
	if len(addrs) == 0 {
		return nil
	}
	result := make([]*mail.Address, len(addrs))
	for i, a := range addrs {
		result[i] = &mail.Address{Email: a}
	}
	return result
}

// applyKeyword sets a JMAP keyword patch entry. true adds the keyword, false removes it.
func applyKeyword(patch jmap.Patch, key string, val *bool) {
	if val == nil {
		return
	}
	if *val {
		patch[key] = true
	} else {
		patch[key] = nil
	}
}
