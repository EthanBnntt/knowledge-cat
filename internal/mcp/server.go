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

// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	// BundlePath is the path to the OKF bundle to serve.
	// If empty, defaults to the OKF_BUNDLE environment variable.
	BundlePath string
}

// Run starts the MCP server over stdin/stdout using the official go-sdk.
// It blocks until the client disconnects.
func Run(cfg ServerConfig) error {
	bundlePath := cfg.BundlePath
	if bundlePath == "" {
		bundlePath = os.Getenv("OKF_BUNDLE")
	}
	if bundlePath == "" {
		// Last resort: try current directory.
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("no bundle path specified and cannot get CWD: %w", err)
		}
		bundlePath = cwd
	}

	b, err := knowledge_cat.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open OKF bundle at %s: %w", bundlePath, err)
	}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "know",
			Version: "0.1.0",
		},
		&mcp.ServerOptions{
			Instructions: fmt.Sprintf(
				"If you are unfamiliar with the Open Knowledge Format (OKF), call know_view_spec first to read the full specification. Serving OKF bundle at %s with %d concepts. "+
					"Use know_list_concepts for an overview, "+
					"know_read_concept to read a concept by ID (supports #block-id, e.g. 'tables/badges#schema'), "+
					"know_grep for full-text search within concepts (returns block context), "+
					"know_find_concepts to find concepts by index/headings, and "+
					"know_list_types to see available types and tags.",
				bundlePath, len(b.Concepts),
			),
		},
	)

	// Register tools.
	registerTools(server, b)

	// Register resources.
	registerResources(server, b)

	// Run over stdio.
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

// registerTools adds all OKF tools to the server.
func registerTools(server *mcp.Server, b *knowledge_cat.Bundle) {
	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_list_concepts",
			Description: "List concepts in the OKF bundle, optionally filtered by type and/or tags. Returns concept ID, type, title, and description — use know_read_concept to get the full body.",
		},
		makeListConceptsHandler(b),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_read_concept",
			Description: "Read a concept from the OKF bundle by its ID. Supports block addressing: use 'tables/badges#schema' to read just the Schema block. Returns frontmatter fields and markdown body (or block content if a #block-id is specified).",
		},
		makeReadConceptHandler(b),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_edit_concept",
			Description: "Edit a concept's body by replacing existing text with new text. The edit is logged to the bundle's log.md with the optional description. Works like a find-and-replace: provide the exact text to find and its replacement.",
		},
		makeEditConceptHandler(b),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_find_concepts",
			Description: "Find concepts by searching the bundle's index.md entries and concept markdown headings. Returns matches grouped by concept — including which index entries and which section headings matched. Use this to discover concepts by their structural metadata: the categories they're listed under or section headings like 'Schema' or 'Examples'. For full-text search within concept bodies, use know_grep instead.",
		},
		makeFindConceptsHandler(b),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_grep",
			Description: "Full-text search within concept bodies. Returns matches with block context (block_id field) for heading-delimited sections. Performs case-insensitive text search on titles, descriptions, and body content. Optionally filter by concept types. For structured concept discovery by index entries and headings, use know_find_concepts instead.",
		},
		makeGrepHandler(b),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_list_types",
			Description: "List all unique concept types and tags used in the OKF bundle. Useful for discovery — see what kinds of concepts are available.",
		},
		makeListTypesHandler(b),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_validate",
			Description: "Validate the OKF bundle against the v0.1 spec. Checks that every non-reserved .md file has parseable YAML frontmatter with a non-empty 'type' field, and that index.md and log.md files follow their respective structures (§6, §7). Returns conformance errors, warnings, and informational notes.",
		},
		makeValidateHandler(b),
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
			Name:        "know_generate_index",
			Description: "Generate or regenerate index.md files for directories in the OKF bundle. Scans concept documents and subdirectories, groups them by type, and writes index.md files following the OKF spec §6 format. By default, existing index.md files are NOT overwritten.",
		},
		makeGenerateIndexHandler(b),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "know_read_log",
			Description: "Read the bundle's log.md file — returns the chronological history of edits, creations, and other changes. Each entry includes a date, action label, and description.",
		},
		makeReadLogHandler(b),
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

// termSearchResult is the JSON representation of a single term search result.
type termSearchResult struct {
	ConceptID    string             `json:"concept_id"`
	ConceptTitle string             `json:"concept_title"`
	ConceptType  string             `json:"concept_type"`
	IndexMatches []termIndexMatch   `json:"index_matches,omitempty"`
	Headings     []termHeadingMatch `json:"heading_matches,omitempty"`
}

type termIndexMatch struct {
	IndexPath    string `json:"index_path"`
	Section      string `json:"section"`
	ConceptTitle string `json:"concept_title"`
	Description  string `json:"description"`
}

type termHeadingMatch struct {
	Heading string `json:"heading"`
	Level   int    `json:"level"`
	Snippet string `json:"snippet"`
}

// findConceptsOutput wraps the results of a term search.
type findConceptsOutput struct {
	Query   string             `json:"query"`
	Results []termSearchResult `json:"results"`
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

// conceptSummary is a lightweight view of a concept (no body).
type conceptSummary struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Resource    string   `json:"resource,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// listConceptsOutput wraps a list of concept summaries.
type listConceptsOutput struct {
	Concepts []conceptSummary `json:"concepts"`
}

// conceptFull is the complete concept including body and links.
type conceptFull struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Resource    string   `json:"resource,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Timestamp   string   `json:"timestamp,omitempty"`
	Body        string   `json:"body"`
	Links       []string `json:"links,omitempty"`
}

// searchOutput wraps search results.
type searchOutput struct {
	Query   string      `json:"query"`
	Results []searchHit `json:"results"`
}

type searchHit struct {
	ConceptID    string `json:"concept_id"`
	ConceptType  string `json:"concept_type"`
	ConceptTitle string `json:"concept_title"`
	Field        string `json:"field"`
	Match        string `json:"match"`
	BlockID      string `json:"block_id,omitempty"`
}

// listTypesOutput wraps the list of types and tags.
type listTypesOutput struct {
	Types []string `json:"types"`
	Tags  []string `json:"tags"`
}

// --- Tool handler constructors ---

func makeListConceptsHandler(b *knowledge_cat.Bundle) mcp.ToolHandlerFor[listConceptsInput, listConceptsOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input listConceptsInput) (*mcp.CallToolResult, listConceptsOutput, error) {
		filter := &knowledge_cat.ListFilter{
			Type: input.Type,
			Tags: input.Tags,
		}
		concepts := b.List(filter)
		summaries := make([]conceptSummary, len(concepts))
		for i, c := range concepts {
			summaries[i] = conceptSummary{
				ID:          c.ID,
				Type:        c.Type,
				Title:       c.Title,
				Description: c.Description,
				Resource:    c.Resource,
				Tags:        c.Tags,
			}
		}
		return nil, listConceptsOutput{Concepts: summaries}, nil
	}
}

func makeReadConceptHandler(b *knowledge_cat.Bundle) mcp.ToolHandlerFor[readConceptInput, conceptFull] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input readConceptInput) (*mcp.CallToolResult, conceptFull, error) {
		conceptID, blockID := knowledge_cat.ParseConceptRef(input.ID)

		c := b.GetConcept(conceptID)
		if c == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf("Concept not found: %s. Use know_list_concepts to see available concepts.", conceptID),
					},
				},
				IsError: true,
			}, conceptFull{}, nil
		}

		// If a block ID is specified, return just that block's content.
		if blockID != "" {
			block := knowledge_cat.GetBlock(c.Body, blockID)
			if block == nil {
				blocks := knowledge_cat.ParseBlocks(c.Body)
				ids := make([]string, len(blocks))
				for i, bl := range blocks {
					ids[i] = bl.ID
				}
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{
							Text: fmt.Sprintf("Block %q not found in %s. Available blocks: %s",
								blockID, conceptID, strings.Join(ids, ", ")),
						},
					},
					IsError: true,
				}, conceptFull{}, nil
			}

			// Return the concept's frontmatter with just the block content in the body.
			ts := ""
			if !c.Timestamp.IsZero() {
				ts = c.Timestamp.Format("2006-01-02T15:04:05Z")
			}
			return nil, conceptFull{
				ID:          c.ID,
				Type:        c.Type,
				Title:       c.Title,
				Description: c.Description,
				Resource:    c.Resource,
				Tags:        c.Tags,
				Timestamp:   ts,
				Body:        block.Content,
				Links:       c.Links,
			}, nil
		}

		ts := ""
		if !c.Timestamp.IsZero() {
			ts = c.Timestamp.Format("2006-01-02T15:04:05Z")
		}

		return nil, conceptFull{
			ID:          c.ID,
			Type:        c.Type,
			Title:       c.Title,
			Description: c.Description,
			Resource:    c.Resource,
			Tags:        c.Tags,
			Timestamp:   ts,
			Body:        c.Body,
			Links:       c.Links,
		}, nil
	}
}

func makeEditConceptHandler(b *knowledge_cat.Bundle) mcp.ToolHandlerFor[editConceptInput, editConceptOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input editConceptInput) (*mcp.CallToolResult, editConceptOutput, error) {
		c, err := knowledge_cat.EditConcept(b.Path, input.ID, input.OldText, input.NewText, input.Description)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Edit failed: %v", err)},
				},
				IsError: true,
			}, editConceptOutput{}, nil
		}

		return nil, editConceptOutput{
			ID:      c.ID,
			Title:   c.Title,
			Message: fmt.Sprintf("Concept %q edited successfully. Change logged to log.md.", c.ID),
		}, nil
	}
}

func makeFindConceptsHandler(b *knowledge_cat.Bundle) mcp.ToolHandlerFor[findConceptsInput, findConceptsOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input findConceptsInput) (*mcp.CallToolResult, findConceptsOutput, error) {
		results := b.FindConcepts(input.Query)

		out := findConceptsOutput{
			Query:   results.Query,
			Results: make([]termSearchResult, len(results.Results)),
		}

		for i, r := range results.Results {
			tr := termSearchResult{
				ConceptID:    r.ConceptID,
				ConceptTitle: r.ConceptTitle,
				ConceptType:  r.ConceptType,
			}

			for _, im := range r.IndexMatches {
				tr.IndexMatches = append(tr.IndexMatches, termIndexMatch{
					IndexPath:    im.IndexPath,
					Section:      im.Section,
					ConceptTitle: im.ConceptTitle,
					Description:  im.Description,
				})
			}

			for _, h := range r.Headings {
				tr.Headings = append(tr.Headings, termHeadingMatch{
					Heading: h.Heading,
					Level:   h.Level,
					Snippet: h.Snippet,
				})
			}

			out.Results[i] = tr
		}

		return nil, out, nil
	}
}

func makeGrepHandler(b *knowledge_cat.Bundle) mcp.ToolHandlerFor[searchInput, searchOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, searchOutput, error) {
		results := b.Search(input.Query, input.Types)
		hits := make([]searchHit, len(results))
		for i, r := range results {
			hits[i] = searchHit{
				ConceptID:    r.Concept.ID,
				ConceptType:  r.Concept.Type,
				ConceptTitle: r.Concept.Title,
				Field:        r.Field,
				Match:        knowledge_cat.TruncateString(r.Line, 200),
				BlockID:      r.BlockID,
			}
		}
		return nil, searchOutput{
			Query:   input.Query,
			Results: hits,
		}, nil
	}
}

func makeListTypesHandler(b *knowledge_cat.Bundle) mcp.ToolHandlerFor[struct{}, listTypesOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listTypesOutput, error) {
		types, tags := b.ListTypes()
		return nil, listTypesOutput{
			Types: types,
			Tags:  tags,
		}, nil
	}
}

func makeValidateHandler(b *knowledge_cat.Bundle) mcp.ToolHandlerFor[struct{}, validateOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, validateOutput, error) {
		result, err := knowledge_cat.Validate(b.Path)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Validation error: %v", err)},
				},
				IsError: true,
			}, validateOutput{}, nil
		}

		out := validateOutput{
			Valid:    result.Valid,
			Errors:   make([]validateIssue, len(result.Errors)),
			Warnings: make([]validateIssue, len(result.Warnings)),
			Info:     make([]validateInfoIssue, len(result.Info)),
		}

		for i, e := range result.Errors {
			out.Errors[i] = validateIssue{File: e.File, Line: e.Line, Message: e.Message}
		}
		for i, w := range result.Warnings {
			out.Warnings[i] = validateIssue{File: w.File, Line: w.Line, Message: w.Message}
		}
		for i, inf := range result.Info {
			out.Info[i] = validateInfoIssue{File: inf.File, Message: inf.Message}
		}

		return nil, out, nil
	}
}

// validateOutput is the result of know_validate.
type validateOutput struct {
	Valid    bool                `json:"valid"`
	Errors   []validateIssue     `json:"errors"`
	Warnings []validateIssue     `json:"warnings"`
	Info     []validateInfoIssue `json:"info"`
}

type validateIssue struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type validateInfoIssue struct {
	File    string `json:"file"`
	Message string `json:"message"`
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

func makeGenerateIndexHandler(b *knowledge_cat.Bundle) mcp.ToolHandlerFor[generateIndexInput, generateIndexOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input generateIndexInput) (*mcp.CallToolResult, generateIndexOutput, error) {
		written, err := knowledge_cat.GenerateIndex(b.Path, input.Overwrite, input.Directory)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Generate-index error: %v", err)},
				},
				IsError: true,
			}, generateIndexOutput{}, nil
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
func registerResources(server *mcp.Server, b *knowledge_cat.Bundle) {
	server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			URITemplate: "know://{+conceptID}",
			Name:        "OKF Concept",
			Description: "An Open Knowledge Format concept document, identified by its concept ID (path without .md suffix).",
			MIMEType:    "text/markdown",
		},
		makeResourceHandler(b),
	)
}

// makeResourceHandler creates a ResourceHandler that reads a concept by URI.
// The URI scheme is know://<conceptID>, e.g., know://tables/badges
func makeResourceHandler(b *knowledge_cat.Bundle) mcp.ResourceHandler {
	return func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI

		// Parse the URI to extract the concept ID.
		conceptID := strings.TrimPrefix(uri, "know://")
		if conceptID == uri {
			return nil, mcp.ResourceNotFoundError(uri)
		}

		conceptID, blockID := knowledge_cat.ParseConceptRef(conceptID)

		c := b.GetConcept(conceptID)
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
	Entries []logEntryItem `json:"entries"`
}

type logEntryItem struct {
	Date        string `json:"date"`
	Action      string `json:"action"`
	Description string `json:"description"`
}

func makeReadLogHandler(b *knowledge_cat.Bundle) mcp.ToolHandlerFor[struct{}, readLogOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, readLogOutput, error) {
		entries, err := knowledge_cat.ReadLog(b.Path)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Read log error: %v", err)},
				},
				IsError: true,
			}, readLogOutput{}, nil
		}

		items := make([]logEntryItem, len(entries))
		for i, e := range entries {
			items[i] = logEntryItem{
				Date:        e.Date.Format("2006-01-02"),
				Action:      e.Action,
				Description: e.Description,
			}
		}

		return nil, readLogOutput{Entries: items}, nil
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

