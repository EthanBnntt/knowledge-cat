// Package mcp provides the Model Context Protocol server for the know CLI.
// It exposes tools for AI agents to read, list, search, and interact with
// Open Knowledge Format (OKF) bundles.
package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/EthanBnntt/knowledge-cat/internal/knowledge_cat"
)

// bundleHandle holds the currently active bundle, safe for concurrent swaps.
type bundleHandle struct {
	b    *knowledge_cat.Bundle
	path string
}

func (bh *bundleHandle) Bundle() *knowledge_cat.Bundle { return bh.b }
func (bh *bundleHandle) Path() string                   { return bh.path }

// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	// BundlePath is the path to the OKF bundle to serve.
	// If empty, defaults to the OKF_BUNDLE environment variable.
	BundlePath string
}

// Run starts the MCP server over stdin/stdout using the official go-sdk.
// It blocks until the client disconnects.
func Run(cfg ServerConfig) error {
	bundlePath, err := knowledge_cat.ResolveBundlePath(cfg.BundlePath, os.Getenv("OKF_BUNDLE"))
	if err != nil {
		return fmt.Errorf("resolve bundle path: %w", err)
	}

	b, err := knowledge_cat.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open OKF bundle at %s: %w", bundlePath, err)
	}
	bh := &bundleHandle{b: b, path: bundlePath}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "know",
			Version: "0.1.0",
		},
		&mcp.ServerOptions{
			Instructions: fmt.Sprintf(
				"If you are unfamiliar with the Open Knowledge Format (OKF), call know_view_spec first to read the full specification. Serving OKF bundle at %s with %d concepts. To switch to a different bundle, use know_switch_bundle. "+
					"Use know_list_concepts for an overview, "+
					"know_read_concept to read a concept by ID (supports #block-id, e.g. 'tables/badges#schema'), "+
					"know_grep for full-text search within concepts (returns block context), "+
					"know_find_concepts to find concepts by index/headings, and "+
					"know_list_types to see available types and tags, "+
					"know_links to traverse concept link graphs (outgoing links + backlinks), and "+
				"know_follow_link to follow a raw markdown link target from a concept to the linked concept.",
				bundlePath, len(bh.Bundle().Concepts),
			),
		},
	)

	// Register tools.
	registerTools(server, bh)

	// Register resources.
	registerResources(server, bh)

	// Run over stdio.
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

// registerTools adds all OKF tools to the server.
func registerTools(server *mcp.Server, bh *bundleHandle) {
	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_list_concepts",
			Description: "List concepts in the OKF bundle, optionally filtered by type and/or tags. Returns concept ID, type, title, and description — use know_read_concept to get the full body.",
		},
		makeListConceptsHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_read_concept",
			Description: "Read a concept from the OKF bundle by its ID. Supports block addressing: use 'tables/badges#schema' to read just the Schema block. Returns frontmatter fields and markdown body (or block content if a #block-id is specified).",
		},
		makeReadConceptHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_edit_concept",
			Description: "Edit a concept's body by replacing existing text with new text. The edit is logged to the bundle's log.md with the optional description. Works like a find-and-replace: provide the exact text to find and its replacement.",
		},
		makeEditConceptHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_find_concepts",
			Description: "Find concepts by searching the bundle's index.md entries and concept markdown headings. Returns matches grouped by concept — including which index entries and which section headings matched. Use this to discover concepts by their structural metadata: the categories they're listed under or section headings like 'Schema' or 'Examples'. For full-text search within concept bodies, use know_grep instead.",
		},
		makeFindConceptsHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_grep",
			Description: "Full-text search within concept bodies. Returns matches with block context (block_id field) for heading-delimited sections. Performs case-insensitive text search on titles, descriptions, and body content. Optionally filter by concept types. For structured concept discovery by index entries and headings, use know_find_concepts instead.",
		},
		makeGrepHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_list_types",
			Description: "List all unique concept types and tags used in the OKF bundle. Useful for discovery — see what kinds of concepts are available.",
		},
		makeListTypesHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_validate",
			Description: "Validate the OKF bundle against the v0.1 spec. Checks that every non-reserved .md file has parseable YAML frontmatter with a non-empty 'type' field, and that index.md and log.md files follow their respective structures (§6, §7). Returns conformance errors, warnings, and informational notes.",
		},
		makeValidateHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_view_spec",
			Description: "View the full Open Knowledge Format (OKF) v0.1 specification. Returns the complete spec — use this to understand the bundle format, frontmatter fields, concept structure, validation rules, and conventions before working with the bundle.",
		},
		makeViewSpecHandler(),
	)
	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_switch_bundle",
			Description: "Switch the active bundle to a different directory. Provide the path to an OKF bundle directory. After switching, all other tools (list, read, grep, find, etc.) will operate on the new bundle. Use this to work with multiple bundles without restarting the server.",
		},
		makeSwitchBundleHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_create_concept",
			Description: "Create a new concept document in the bundle. Provide the concept ID (path within the bundle, e.g. 'tables/orders'), a type (required), and optional title, description, tags, and markdown body. The concept is written to disk and logged. Use know_edit_concept to modify an existing concept.",
		},
		makeCreateConceptHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_generate_index",
			Description: "Generate or regenerate index.md files for directories in the OKF bundle. Scans concept documents and subdirectories, groups them by type, and writes index.md files following the OKF spec §6 format. By default, existing index.md files are NOT overwritten.",
		},
		makeGenerateIndexHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_read_log",
			Description: "Read the bundle's log.md file — returns the chronological history of edits, creations, and other changes. Each entry includes a date, action label, and description.",
		},
		makeReadLogHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_follow_link",
			Description: "Follow a markdown link from a concept to the linked concept. Given a source concept ID and the raw link target (as it appears in the markdown body, e.g. 'concept-two.md' or '/tables/orders.md#schema'), resolves the link and returns the full linked concept. For fragment-only links (#block-id), returns the source concept. For external URLs, returns the URL in the response.",
		},
		makeFollowLinkHandler(bh),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_links",
			Description: "Show outgoing and incoming links for a concept. Resolves markdown links to enriched concept references with type and title. Returns outgoing links (what this concept links to) and incoming links/backlinks (what concepts link to this one). Broken links and external URLs are excluded.",
		},
		makeLinksHandler(bh),
	)
}

// --- Tool input types ---

// listConceptsInput is the input for know_list_concepts.
type listConceptsInput struct {
	// Type filters concepts by their type field (e.g., "BigQuery Table").
	Type string `json:"type,omitempty" jsonschema:"filter by concept type (e.g. 'BigQuery Table')"`
	// Tags filters concepts that have at least one of the given tags.
	Tags []string `json:"tags,omitempty" jsonschema:"filter by tags (concept must have at least one)"`
}

// readConceptInput is the input for know_read_concept.
type readConceptInput struct {
	// ID is the concept identifier, optionally with a block ID.
	// E.g., "tables/orders" or "tables/orders#schema".
	ID string `json:"id" jsonschema:"required, the concept ID (path sans .md). Supports #block-id suffix, e.g. 'tables/orders#schema'"`
}

// editConceptInput is the input for know_edit_concept.
type editConceptInput struct {
	// ID is the concept identifier (path without .md, e.g. "tables/orders").
	ID string `json:"id" jsonschema:"required, the concept ID to edit (path sans .md)"`
	// OldText is the exact text to find and replace in the concept body.
	OldText string `json:"old_text" jsonschema:"required, the exact existing text to replace"`
	// NewText is the replacement text.
	NewText string `json:"new_text" jsonschema:"required, the new text to insert"`
	// Description is an optional description of the edit for the log.
	Description string `json:"description,omitempty" jsonschema:"optional description of this edit (logged to log.md)"`
}

// editConceptOutput is the result of an edit.
type editConceptOutput struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// findConceptsInput is the input for know_find_concepts.
type findConceptsInput struct {
	// Query is the search term (case-insensitive).
	Query string `json:"query" jsonschema:"required, the search term (case-insensitive)"`
}

// searchInput is the input for know_search.
type searchInput struct {
	// Query is the search term (case-insensitive).
	Query string `json:"query" jsonschema:"required, the search query (case-insensitive)"`
	// Types optionally restricts search to specific concept types.
	Types []string `json:"types,omitempty" jsonschema:"optional, restrict search to these concept types"`
}

// --- Tool output types ---
// All output types must be structs (objects) for the go-sdk.

// listConceptsOutput wraps a list of concepts.
type listConceptsOutput struct {
	Concepts []*knowledge_cat.Concept `json:"concepts"`
}

// searchOutput wraps search results.
type searchOutput struct {
	Query   string                        `json:"query"`
	Results []knowledge_cat.SearchResult `json:"results"`
}

// listTypesOutput wraps the list of types and tags.
type listTypesOutput struct {
	Types []string `json:"types"`
	Tags  []string `json:"tags"`
}

// --- Tool handler constructors ---

func makeListConceptsHandler(bh *bundleHandle) mcp.ToolHandlerFor[listConceptsInput, listConceptsOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input listConceptsInput) (*mcp.CallToolResult, listConceptsOutput, error) {
		filter := &knowledge_cat.ListFilter{
			Type: input.Type,
			Tags: input.Tags,
		}
		concepts := bh.Bundle().List(filter)
		return nil, listConceptsOutput{Concepts: concepts}, nil
	}
}

func makeReadConceptHandler(bh *bundleHandle) mcp.ToolHandlerFor[readConceptInput, knowledge_cat.Concept] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input readConceptInput) (*mcp.CallToolResult, knowledge_cat.Concept, error) {
		conceptID, blockID := knowledge_cat.ParseConceptRef(input.ID)

		c := bh.Bundle().GetConcept(conceptID)
		if c == nil {
			return mcpErrorf[knowledge_cat.Concept]("Concept not found: %s. Use know_list_concepts to see available concepts.", conceptID)
		}

		// If a block ID is specified, return just that block's content.
		if blockID != "" {
			block := knowledge_cat.GetBlock(c.Body, blockID)
			if block == nil {
				return mcpErrorf[knowledge_cat.Concept]("%s", knowledge_cat.BlockNotFoundMessage(conceptID, blockID, c.Body))
			}

			// Return the concept's frontmatter with just the block content in the body.
			shallow := *c
			shallow.Body = block.Content
			return nil, shallow, nil
		}

		return nil, *c, nil
	}
}

func makeEditConceptHandler(bh *bundleHandle) mcp.ToolHandlerFor[editConceptInput, editConceptOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input editConceptInput) (*mcp.CallToolResult, editConceptOutput, error) {
		c, err := knowledge_cat.EditConcept(bh.Path(), input.ID, input.OldText, input.NewText, input.Description)
		if err != nil {
			return mcpErrorf[editConceptOutput]("Edit failed: %v", err)
		}

		return nil, editConceptOutput{
			ID:      c.ID,
			Title:   c.Title,
			Message: fmt.Sprintf("Concept %q edited successfully. Change logged to log.md.", c.ID),
		}, nil
	}
}

func makeFindConceptsHandler(bh *bundleHandle) mcp.ToolHandlerFor[findConceptsInput, knowledge_cat.FindOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input findConceptsInput) (*mcp.CallToolResult, knowledge_cat.FindOutput, error) {
		return nil, bh.Bundle().FindConcepts(input.Query), nil
	}
}

func makeGrepHandler(bh *bundleHandle) mcp.ToolHandlerFor[searchInput, searchOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, searchOutput, error) {
		results := bh.Bundle().Search(input.Query, input.Types)
		for i := range results {
			results[i].Line = knowledge_cat.TruncateString(results[i].Line, 200)
		}
		return nil, searchOutput{
			Query:   input.Query,
			Results: results,
		}, nil
	}
}

func makeListTypesHandler(bh *bundleHandle) mcp.ToolHandlerFor[struct{}, listTypesOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listTypesOutput, error) {
		types, tags := bh.Bundle().ListTypes()
		return nil, listTypesOutput{
			Types: types,
			Tags:  tags,
		}, nil
	}
}

func makeValidateHandler(bh *bundleHandle) mcp.ToolHandlerFor[struct{}, knowledge_cat.ValidationResult] {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, knowledge_cat.ValidationResult, error) {
		result, err := knowledge_cat.Validate(bh.Path())
		if err != nil {
			return mcpErrorf[knowledge_cat.ValidationResult]("Validation error: %v", err)
		}
		return nil, *result, nil
	}
}

// generateIndexInput is the input for know_generate_index.
type generateIndexInput struct {
	Overwrite bool   `json:"overwrite,omitempty" jsonschema:"overwrite existing index.md files"`
	Directory string `json:"directory,omitempty" jsonschema:"scope to a specific directory (relative to bundle root)"`
}

// generateIndexOutput is the result of know_generate_index.
type generateIndexOutput struct {
	Written []string `json:"written"`
	Message string   `json:"message"`
}

func makeGenerateIndexHandler(bh *bundleHandle) mcp.ToolHandlerFor[generateIndexInput, generateIndexOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input generateIndexInput) (*mcp.CallToolResult, generateIndexOutput, error) {
		written, err := knowledge_cat.GenerateIndex(bh.Path(), input.Overwrite, input.Directory)
		if err != nil {
			return mcpErrorf[generateIndexOutput]("Generate-index error: %v", err)
		}

		msg := fmt.Sprintf("No index.md files generated (use overwrite=true to regenerate existing ones).")
		if len(written) > 0 {
			msg = fmt.Sprintf("Generated %d index.md file(s).", len(written))
		}

		return nil, generateIndexOutput{
			Written: written,
			Message: msg,
		}, nil
	}
}

// registerResources adds OKF concept resources to the server.
// Concepts are exposed as resources under the know:// URI scheme with a
// URI template: know://{+conceptID}
func registerResources(server *mcp.Server, bh *bundleHandle) {
	server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			URITemplate: "know://{+conceptID}",
			Name:        "OKF Concept",
			Description: "An Open Knowledge Format concept document, identified by its concept ID (path without .md suffix).",
			MIMEType:    "text/markdown",
		},
		makeResourceHandler(bh),
	)
}

// makeResourceHandler creates a ResourceHandler that reads a concept by URI.
// The URI scheme is know://<conceptID>, e.g., know://tables/badges
func makeResourceHandler(bh *bundleHandle) mcp.ResourceHandler {
	return func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI

		// Parse the URI to extract the concept ID.
		conceptID := strings.TrimPrefix(uri, "know://")
		if conceptID == uri {
			return nil, mcp.ResourceNotFoundError(uri)
		}

		conceptID, blockID := knowledge_cat.ParseConceptRef(conceptID)

		c := bh.Bundle().GetConcept(conceptID)
		if c == nil {
			return nil, mcp.ResourceNotFoundError(uri)
		}

		text := string(c.Marshal())
		if blockID != "" {
			block := knowledge_cat.GetBlock(c.Body, blockID)
			if block == nil {
				return nil, mcp.ResourceNotFoundError(uri)
			}
			text = block.Content
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      uri,
					MIMEType: "text/markdown",
					Text:     text,
				},
			},
		}, nil
	}
}

// readLogOutput is the result of know_read_log.
type readLogOutput struct {
	Entries []knowledge_cat.LogEntry `json:"entries"`
}

func makeReadLogHandler(bh *bundleHandle) mcp.ToolHandlerFor[struct{}, readLogOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, readLogOutput, error) {
		entries, err := knowledge_cat.ReadLog(bh.Path())
		if err != nil {
			return mcpErrorf[readLogOutput]("Read log error: %v", err)
		}

		return nil, readLogOutput{Entries: entries}, nil
	}
}

// followLinkInput is the input for know_follow_link.
type followLinkInput struct {
	// SourceID is the concept ID that contains the link.
	SourceID string `json:"source_id" jsonschema:"required, the concept ID containing the link (e.g. 'tables/orders')"`
	// LinkTarget is the raw markdown link target as it appears in the body.
	// E.g., "concept-two.md", "/tables/orders.md#schema", "#block-id".
	LinkTarget string `json:"link_target" jsonschema:"required, the raw link target from the markdown body (e.g. 'concept-two.md', '/tables/orders.md#schema')"`
}

// followLinkOutput is the result of know_follow_link.
type followLinkOutput struct {
	SourceID    string `json:"source_id"`
	LinkTarget  string `json:"link_target"`
	ResolvedID  string `json:"resolved_id"`
	SameConcept bool   `json:"same_concept"`
	External    bool   `json:"external"`
	ExternalURL string `json:"external_url,omitempty"`
	Broken      bool   `json:"broken"`
	Fragment    string `json:"fragment,omitempty"`
	// Target fields — populated when a valid concept link is followed.
	TargetID          string   `json:"target_id,omitempty"`
	TargetType        string   `json:"target_type,omitempty"`
	TargetTitle       string   `json:"target_title,omitempty"`
	TargetDescription string   `json:"target_description,omitempty"`
	TargetResource    string   `json:"target_resource,omitempty"`
	TargetTags        []string `json:"target_tags,omitempty"`
	TargetTimestamp   string   `json:"target_timestamp,omitempty"`
	TargetBody        string   `json:"target_body,omitempty"`
	TargetLinks       []string `json:"target_links,omitempty"`
}

func makeFollowLinkHandler(bh *bundleHandle) mcp.ToolHandlerFor[followLinkInput, followLinkOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input followLinkInput) (*mcp.CallToolResult, followLinkOutput, error) {
		result, err := bh.Bundle().FollowLink(input.SourceID, input.LinkTarget)
		if err != nil {
			// Broken link — return the partial result as a user error.
			if result != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{
							Text: fmt.Sprintf("Broken link: target %q (resolved to %q) not found in bundle.",
								result.LinkTarget, result.ResolvedID),
						},
					},
					IsError: true,
				}, followLinkOutput{
					SourceID:   result.SourceID,
					LinkTarget: result.LinkTarget,
					ResolvedID: result.ResolvedID,
					Broken:     true,
					Fragment:   result.Fragment,
				}, nil
			}
			return mcpErrorf[followLinkOutput]("Follow-link error: %v", err)
		}

		out := followLinkOutput{
			SourceID:    result.SourceID,
			LinkTarget:  result.LinkTarget,
			ResolvedID:  result.ResolvedID,
			SameConcept: result.SameConcept,
			External:    result.External,
			ExternalURL: result.ExternalURL,
			Broken:      result.Broken,
			Fragment:    result.Fragment,
		}

		if result.Target != nil {
			out.TargetID = result.Target.ID
			out.TargetType = result.Target.Type
			out.TargetTitle = result.Target.Title
			out.TargetDescription = result.Target.Description
			out.TargetResource = result.Target.Resource
			out.TargetTags = result.Target.Tags
			out.TargetTimestamp = result.Target.TimestampString()
			out.TargetBody = result.Target.Body
			out.TargetLinks = result.Target.Links
		}

		return nil, out, nil
	}
}

// linksInput is the input for know_links.
type linksInput struct {
	// ID is the concept identifier (path sans .md) whose links to trace.
	ID string `json:"id" jsonschema:"required, concept ID whose link graph to trace (e.g. 'tables/orders')"`
}

// linksOutput is the result of know_links.
func makeLinksHandler(bh *bundleHandle) mcp.ToolHandlerFor[linksInput, knowledge_cat.LinksResult] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input linksInput) (*mcp.CallToolResult, knowledge_cat.LinksResult, error) {
		result, err := bh.Bundle().LinksOf(input.ID)
		if err != nil {
			return mcpErrorf[knowledge_cat.LinksResult]("Links error: %v", err)
		}
		return nil, *result, nil
	}
}

// viewSpecOutput is the result of know_view_spec.
type viewSpecOutput struct {
	Spec string `json:"spec"`
}

func makeViewSpecHandler() mcp.ToolHandlerFor[struct{}, viewSpecOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, viewSpecOutput, error) {
		return nil, viewSpecOutput{Spec: knowledge_cat.Spec}, nil
	}
}

// switchBundleInput is the input for know_switch_bundle.
type switchBundleInput struct {
	// Path is the absolute or relative path to the OKF bundle directory.
	Path string `json:"path" jsonschema:"required, path to the OKF bundle directory"`
}

// switchBundleOutput is the result of know_switch_bundle.
type switchBundleOutput struct {
	Path     string `json:"path"`
	Concepts int    `json:"concepts"`
	Message  string `json:"message"`
}

func makeSwitchBundleHandler(bh *bundleHandle) mcp.ToolHandlerFor[switchBundleInput, switchBundleOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input switchBundleInput) (*mcp.CallToolResult, switchBundleOutput, error) {
		b, err := knowledge_cat.Open(input.Path)
		if err != nil {
			return mcpErrorf[switchBundleOutput]("Failed to open bundle at %s: %v", input.Path, err)
		}
		bh.b = b
		bh.path = input.Path
		return nil, switchBundleOutput{
			Path:     input.Path,
			Concepts: len(b.Concepts),
			Message:  fmt.Sprintf("Switched to bundle at %s with %d concepts.", input.Path, len(b.Concepts)),
		}, nil
	}
}

// createConceptInput is the input for know_create_concept.
type createConceptInput struct {
	// ID is the concept identifier (path within the bundle, e.g. "tables/orders").
	ID string `json:"id" jsonschema:"required, path of the new concept within the bundle (e.g. 'tables/orders')"`
	// Type is the concept type (required, e.g. "package", "architecture", "metric").
	Type string `json:"type" jsonschema:"required, the concept type (e.g. 'package', 'architecture', 'metric')"`
	// Title is the optional display name for the concept.
	Title string `json:"title,omitempty" jsonschema:"optional display title"`
	// Description is an optional one-line summary.
	Description string `json:"description,omitempty" jsonschema:"optional one-line description"`
	// Resource is an optional canonical URI for the underlying asset.
	Resource string `json:"resource,omitempty" jsonschema:"optional canonical URI"`
	// Tags are optional categorization tags.
	Tags []string `json:"tags,omitempty" jsonschema:"optional list of tags"`
	// Body is the markdown body content.
	Body string `json:"body,omitempty" jsonschema:"markdown body content for the concept"`
}

// createConceptOutput is the result of know_create_concept.
type createConceptOutput struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

func makeCreateConceptHandler(bh *bundleHandle) mcp.ToolHandlerFor[createConceptInput, createConceptOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input createConceptInput) (*mcp.CallToolResult, createConceptOutput, error) {
		c, err := knowledge_cat.CreateConcept(bh.Path(), input.ID, input.Type, input.Title, input.Description, input.Resource, input.Tags, input.Body)
		if err != nil {
			return mcpErrorf[createConceptOutput]("Create failed: %v", err)
		}
		return nil, createConceptOutput{
			ID:      c.ID,
			Type:    c.Type,
			Title:   c.Title,
			Message: fmt.Sprintf("Created concept %q (%s). Logged to log.md.", c.ID, c.Type),
		}, nil
	}
}

// mcpErrorf returns a CallToolResult with IsError=true and a formatted text
// message, along with a zero value of the output type T. Use in MCP handlers
// to report user-facing errors without a Go error.
func mcpErrorf[T any](format string, args ...any) (*mcp.CallToolResult, T, error) {
	var zero T
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(format, args...)},
		},
		IsError: true,
	}, zero, nil
}

