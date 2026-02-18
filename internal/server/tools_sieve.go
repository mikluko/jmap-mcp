package server

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mikluko/jmap"
	"github.com/mikluko/jmap/sieve"
	"github.com/mikluko/jmap/sieve/sievescript"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sieveAccountID returns the primary account ID for the Sieve capability,
// or an error if the server does not advertise it.
func sieveAccountID(client *jmap.Client) (jmap.ID, error) {
	id := client.Session.PrimaryAccounts[sieve.URI]
	if id == "" {
		return "", fmt.Errorf("Sieve capability not available: server does not advertise %s", sieve.URI)
	}
	return id, nil
}

// --- sieve_get ---

type SieveGetInput struct {
	ID string `json:"id,omitempty" jsonschema:"Script ID to retrieve with content (omit to list all scripts)"`
}

var sieveGetTool = &mcp.Tool{
	Name:        "sieve_get",
	Description: "Get Sieve scripts: list all (no ID) or get full content of one (with ID) (maps to JMAP SieveScript/get)",
}

func (s *Server) handleSieveGet(ctx context.Context, _ *mcp.CallToolRequest, in SieveGetInput) (*mcp.CallToolResult, any, error) {
	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID, err := sieveAccountID(client)
	if err != nil {
		return errorResult(err), nil, nil
	}

	get := &sievescript.Get{Account: accountID}
	if in.ID != "" {
		get.IDs = []jmap.ID{jmap.ID(in.ID)}
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(get)

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for SieveScript/get")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *sievescript.GetResponse:
		if len(args.NotFound) > 0 {
			return errorResult(fmt.Errorf("sieve scripts not found: %v", args.NotFound)), nil, nil
		}

		// Single script with ID: return metadata + blob content.
		if in.ID != "" {
			if len(args.List) == 0 {
				return errorResult(fmt.Errorf("sieve script %s not found", in.ID)), nil, nil
			}
			script := args.List[0]
			reader, err := client.DownloadWithContext(ctx, accountID, script.BlobID)
			if err != nil {
				return errorResult(fmt.Errorf("download sieve script: %w", err)), nil, nil
			}
			defer reader.Close()

			content, err := io.ReadAll(reader)
			if err != nil {
				return errorResult(fmt.Errorf("read sieve script: %w", err)), nil, nil
			}

			var sb strings.Builder
			name := "(unnamed)"
			if script.Name != nil {
				name = *script.Name
			}
			fmt.Fprintf(&sb, "Name: %s\n", name)
			fmt.Fprintf(&sb, "Active: %v\n", script.IsActive)
			fmt.Fprintf(&sb, "ID: %s\n\n", script.ID)
			sb.Write(content)

			return textResult(sb.String()), nil, nil
		}

		// No ID: list all scripts metadata.
		var sb strings.Builder
		for _, script := range args.List {
			name := "(unnamed)"
			if script.Name != nil {
				name = *script.Name
			}
			active := ""
			if script.IsActive {
				active = " [ACTIVE]"
			}
			fmt.Fprintf(&sb, "%s%s [id: %s]\n", name, active, script.ID)
		}
		if len(args.List) == 0 {
			sb.WriteString("No Sieve scripts found.\n")
		}
		return textResult(sb.String()), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}

// --- sieve_set ---

type SieveSetInput struct {
	Name     string `json:"name,omitempty" jsonschema:"Name for the Sieve script (required for create)"`
	Content  string `json:"content,omitempty" jsonschema:"Sieve script source code (required for create, optional for update)"`
	ID       string `json:"id,omitempty" jsonschema:"ID of existing script to update"`
	Activate *bool  `json:"activate,omitempty" jsonschema:"Activate script on successful create/update"`
	Destroy  []string `json:"destroy,omitempty" jsonschema:"Script IDs to destroy"`
}

var sieveSetTool = &mcp.Tool{
	Name:        "sieve_set",
	Description: "Create, update, or destroy Sieve scripts (maps to JMAP SieveScript/set)",
}

func (s *Server) handleSieveSet(ctx context.Context, _ *mcp.CallToolRequest, in SieveSetInput) (*mcp.CallToolResult, any, error) {
	isCreate := in.ID == "" && (in.Name != "" || in.Content != "")
	isUpdate := in.ID != ""
	isDestroy := len(in.Destroy) > 0

	if !isCreate && !isUpdate && !isDestroy {
		return errorResult(fmt.Errorf("provide name+content to create, id to update, or destroy list")), nil, nil
	}

	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID, err := sieveAccountID(client)
	if err != nil {
		return errorResult(err), nil, nil
	}

	set := &sievescript.Set{Account: accountID}

	// Upload blob if content is provided (for create or update).
	var blobID jmap.ID
	if in.Content != "" {
		uploadResp, err := client.UploadWithContext(ctx, accountID, strings.NewReader(in.Content))
		if err != nil {
			return errorResult(fmt.Errorf("upload sieve script: %w", err)), nil, nil
		}
		blobID = uploadResp.ID
	}

	if isCreate {
		if in.Content == "" {
			return errorResult(fmt.Errorf("content is required for create")), nil, nil
		}
		set.Create = map[jmap.ID]*sievescript.SieveScript{
			"new": {Name: &in.Name, BlobID: blobID},
		}
		if in.Activate != nil && *in.Activate {
			id := jmap.ID("#new")
			set.OnSuccessActivateScript = &id
		}
	}

	if isUpdate {
		patch := jmap.Patch{}
		if blobID != "" {
			patch["blobId"] = blobID
		}
		if in.Name != "" {
			patch["name"] = in.Name
		}
		if len(patch) > 0 {
			set.Update = map[jmap.ID]jmap.Patch{
				jmap.ID(in.ID): patch,
			}
		}
		if in.Activate != nil && *in.Activate {
			id := jmap.ID(in.ID)
			set.OnSuccessActivateScript = &id
		}
	}

	if isDestroy {
		set.Destroy = toJMAPIDSlice(in.Destroy)
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(set)

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for SieveScript/set")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *sievescript.SetResponse:
		var sb strings.Builder
		var errors []string

		for cid, script := range args.Created {
			fmt.Fprintf(&sb, "Created sieve script %s [id: %s]\n", cid, script.ID)
		}
		for cid, se := range args.NotCreated {
			errors = append(errors, fmt.Sprintf("create %s: %s", cid, se.Type))
		}
		for id := range args.Updated {
			fmt.Fprintf(&sb, "Updated sieve script %s\n", id)
		}
		for id, se := range args.NotUpdated {
			errors = append(errors, fmt.Sprintf("update %s: %s", id, se.Type))
		}
		for _, id := range args.Destroyed {
			fmt.Fprintf(&sb, "Destroyed sieve script %s\n", id)
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

// --- sieve_validate ---

type SieveValidateInput struct {
	Content string `json:"content" jsonschema:"Sieve script source code to validate"`
}

var sieveValidateTool = &mcp.Tool{
	Name:        "sieve_validate",
	Description: "Validate a Sieve script without saving it (maps to JMAP SieveScript/validate)",
}

func (s *Server) handleSieveValidate(ctx context.Context, _ *mcp.CallToolRequest, in SieveValidateInput) (*mcp.CallToolResult, any, error) {
	client, err := s.jmapClient(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	accountID, err := sieveAccountID(client)
	if err != nil {
		return errorResult(err), nil, nil
	}

	uploadResp, err := client.UploadWithContext(ctx, accountID, strings.NewReader(in.Content))
	if err != nil {
		return errorResult(fmt.Errorf("upload sieve script: %w", err)), nil, nil
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(&sievescript.Validate{
		Account: accountID,
		BlobID:  uploadResp.ID,
	})

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if len(resp.Responses) == 0 {
		return errorResult(fmt.Errorf("empty response for SieveScript/validate")), nil, nil
	}

	switch args := resp.Responses[0].Args.(type) {
	case *sievescript.ValidateResponse:
		if args.Error != nil {
			desc := args.Error.Type
			if args.Error.Description != nil {
				desc += ": " + *args.Error.Description
			}
			return textResult(fmt.Sprintf("Validation failed: %s", desc)), nil, nil
		}
		return textResult("Sieve script is valid."), nil, nil
	case *jmap.MethodError:
		return errorResult(args), nil, nil
	default:
		return errorResult(fmt.Errorf("unexpected response type: %T", args)), nil, nil
	}
}
