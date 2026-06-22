// know is a CLI tool and MCP server for interacting with Open Knowledge Format (OKF) bundles.
//
// OKF is an open specification for representing organizational knowledge as
// a directory of markdown files with YAML frontmatter. See:
// https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
//
// Usage:
//
//	# Start as an MCP server (for AI agents)
//	know serve --bundle /path/to/bundle
//	OKF_BUNDLE=/path/to/bundle know serve
//
//	# CLI commands (run directly from terminal)
//	know list --bundle /path/to/bundle
//	know read tables/orders --bundle /path/to/bundle
//	know search "revenue"
//	know types
//
// If --bundle is not specified, defaults to the current directory.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/EthanBnntt/knowledge-cat/internal/mcp"
	"github.com/EthanBnntt/knowledge-cat/internal/knowledge_cat"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "know: %v\n", err)
		os.Exit(1)
	}
}

var (
	bundlePath string
)

var rootCmd = &cobra.Command{
	Use:   "know",
	Short: "CLI and MCP server for Open Knowledge Format (OKF) bundles",
	Long: `know is a tool for reading, searching, and managing OKF knowledge bundles.
It can be used directly from the terminal or as an MCP server for AI agents.

OKF bundles are directories of markdown files with YAML frontmatter
that represent organizational knowledge — tables, metrics, APIs, playbooks,
and more.`,
	SilenceUsage: true,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start as an MCP server over stdio",
	Long: `Start know as a Model Context Protocol (MCP) server over stdin/stdout.

AI agents can connect to this server to read, list, search, and interact
with the OKF bundle. Tools exposed:
  - know_list_concepts  : list concepts with optional type/tag filters
  - know_read_concept   : read a concept by ID
  - know_search         : full-text search across concepts
  - know_list_types     : list available types and tags

Set the OKF_BUNDLE environment variable or use --bundle to specify the bundle path.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcp.Run(mcp.ServerConfig{BundlePath: bundlePath})
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List concepts in the bundle",
	Long:  "List all concepts in the OKF bundle, optionally filtered by type and/or tags.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := openBundle()
		if err != nil {
			return err
		}

		concepts := b.List(nil)
		for _, c := range concepts {
			fmt.Printf("%-40s  %-20s  %s\n", c.ID, c.Type, c.Title)
		}
		return nil
	},
}

var readCmd = &cobra.Command{
	Use:   "read <concept-id | concept-id#block-id>",
	Short: "Read a concept or a block within a concept",
	Long: `Read and display a full concept from the bundle, or a specific block within it.

Supports block addressing:
  know read tables/badges          # reads the entire concept
  know read tables/badges#schema   # reads just the Schema block
  know read tables/badges#citations # reads just the Citations block`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := openBundle()
		if err != nil {
			return err
		}

		conceptID, blockID := knowledge_cat.ParseConceptRef(args[0])

		c := b.GetConcept(conceptID)
		if c == nil {
			return fmt.Errorf("concept not found: %s", conceptID)
		}

		if blockID != "" {
			// Read just the block.
			block := knowledge_cat.GetBlock(c.Body, blockID)
			if block == nil {
				// List available blocks.
				blocks := knowledge_cat.ParseBlocks(c.Body)
				ids := make([]string, len(blocks))
				for i, bl := range blocks {
					ids[i] = bl.ID
				}
				return fmt.Errorf("block %q not found in %s. Available blocks: %s",
					blockID, conceptID, strings.Join(ids, ", "))
			}
			fmt.Printf("%s#%s\n", conceptID, blockID)
			fmt.Println(strings.Repeat("-", 40))
			fmt.Println(block.Content)
			return nil
		}

		os.Stdout.Write(c.Marshal())
		return nil
	},
}

var grepCmd = &cobra.Command{
	Use:   "grep <query>",
	Short: "Search within concept bodies",
	Long:  "Perform a case-insensitive full-text search within concept titles, descriptions, and body content. Use 'know find' to search for concepts by their structured metadata (index entries, headings, type, tags).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := openBundle()
		if err != nil {
			return err
		}

		results := b.Search(args[0], nil)
		for _, r := range results {
			blockInfo := ""
			if r.BlockID != "" {
				blockInfo = fmt.Sprintf("#%s", r.BlockID)
			}
			fmt.Printf("%s%s  [%s/%s]  %s\n",
				r.Concept.ID, blockInfo, r.Concept.Type, r.Field, knowledge_cat.TruncateString(r.Line, 80))
		}
		return nil
	},
}

var editCmd = &cobra.Command{
	Use:   "edit <concept-id>",
	Short: "Edit a concept's body by replacing existing text",
	Long: `Edit a concept by replacing existing text in its body with new text.
Works like find-and-replace: provide the exact text to find and its replacement.
The edit is automatically logged to the bundle's log.md.

Examples:
  know edit tables/orders --old "old text" --new "new text"
  know edit tables/orders --old "old text" --new "new text" --description "Updated join paths"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := resolveBundlePath()
		if err != nil {
			return err
		}

		c, err := knowledge_cat.EditConcept(path, args[0], editOld, editNew, editDescription)
		if err != nil {
			return err
		}

		fmt.Printf("Edited concept %q (%s). Change logged to log.md.\n", c.ID, c.Title)
		return nil
	},
}

var (
	editOld         string
	editNew         string
	editDescription string
)

var findCmd = &cobra.Command{
	Use:   "find <query>",
	Short: "Find concepts by index entries and headings",
	Long: `Find concepts by searching the bundle's structured surface:
  - Index.md entries (concept titles and descriptions in directory listings)
  - Concept markdown headings (e.g., "# Schema", "## Common patterns")

Use 'know grep' to search within concept body content instead.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := openBundle()
		if err != nil {
			return err
		}

		results := b.FindConcepts(args[0])
		for _, r := range results.Results {
			fmt.Printf("\n%s  [%s]\n", r.ConceptID, r.ConceptType)

			for _, im := range r.IndexMatches {
				fmt.Printf("  index: %s | %s → %s\n",
					im.IndexPath, im.Section, knowledge_cat.TruncateString(im.Description, 100))
			}
			for _, h := range r.Headings {
				prefix := strings.Repeat("#", h.Level)
				fmt.Printf("  heading: %s %s → %s\n",
					prefix, h.Heading, knowledge_cat.TruncateString(h.Snippet, 100))
			}
		}
		fmt.Printf("\n%d results for %q\n", len(results.Results), args[0])
		return nil
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the OKF bundle against the v0.1 spec",
	Long: `Check the OKF bundle for conformance with the v0.1 spec.

Reports conformance errors (required type field, parseable frontmatter),
warnings (missing recommended fields, index/log format issues), and
informational notes (missing index.md files).

Exit code 0 if no errors, 1 if errors found.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := resolveBundlePath()
		if err != nil {
			return err
		}

		result, err := knowledge_cat.Validate(path)
		if err != nil {
			return err
		}

		// Print errors.
		if len(result.Errors) > 0 {
			fmt.Printf("\n❌ Errors (%d):\n", len(result.Errors))
			for _, e := range result.Errors {
				fmt.Printf("  %s: %s\n", e.File, e.Message)
			}
		}

		// Print warnings.
		if len(result.Warnings) > 0 {
			fmt.Printf("\n⚠️  Warnings (%d):\n", len(result.Warnings))
			for _, w := range result.Warnings {
				fmt.Printf("  %s: %s\n", w.File, w.Message)
			}
		}

		// Print info.
		if len(result.Info) > 0 {
			fmt.Printf("\nℹ️  Info (%d):\n", len(result.Info))
			for _, inf := range result.Info {
				fmt.Printf("  %s: %s\n", inf.File, inf.Message)
			}
		}

		if result.Valid {
			fmt.Println("\n✅ Bundle is conformant with OKF v0.1")
			return nil
		}

		fmt.Println("\n❌ Bundle is not conformant with OKF v0.1")
		return fmt.Errorf("validation failed with %d error(s)", len(result.Errors))
	},
}

var generateIndexCmd = &cobra.Command{
	Use:   "generate-index [directory]",
	Short: "Generate index.md files for bundle directories",
	Long: `Generate or regenerate index.md files for directories in the OKF bundle.

For each directory, scans concept documents and subdirectories, groups them
by their 'type' field, and writes an index.md following the OKF spec §6 format.

By default, existing index.md files are NOT overwritten. Use --overwrite to
force regeneration. Optionally specify a directory to scope the operation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := resolveBundlePath()
		if err != nil {
			return err
		}

		dir := ""
		if len(args) > 0 {
			dir = args[0]
		}

		written, err := knowledge_cat.GenerateIndex(path, genOverwrite, dir)
		if err != nil {
			return err
		}

		if len(written) == 0 {
			fmt.Println("No index.md files generated (all directories already have index.md — use --overwrite to regenerate).")
		} else {
			fmt.Printf("Generated %d index.md file(s):\n", len(written))
			for _, w := range written {
				fmt.Printf("  %s\n", w)
			}
		}
		return nil
	},
}

var genOverwrite bool

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show the bundle's update log (log.md)",
	Long:  "Read and display the bundle's log.md file — a chronological history of edits, creations, and other changes.",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := resolveBundlePath()
		if err != nil {
			return err
		}

		entries, err := knowledge_cat.ReadLog(path)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No log entries found (log.md is empty or doesn't exist).")
			return nil
		}

		for _, e := range entries {
			fmt.Printf("%s  **%s**: %s\n",
				e.Date.Format("2006-01-02"), e.Action, knowledge_cat.TruncateString(e.Description, 100))
		}
		return nil
	},
}

var typesCmd = &cobra.Command{
	Use:   "types",
	Short: "List types and tags",
	Long:  "List all unique concept types and tags used in the bundle.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := openBundle()
		if err != nil {
			return err
		}

		types, tags := b.ListTypes()
		fmt.Println("Types:")
		for _, t := range types {
			fmt.Printf("  %s\n", t)
		}
		fmt.Println("\nTags:")
		for _, t := range tags {
			fmt.Printf("  %s\n", t)
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&bundlePath, "bundle", "b", "", "Path to OKF bundle (defaults to current directory)")
	editCmd.Flags().StringVar(&editOld, "old", "", "Existing text to replace (required)")
	editCmd.Flags().StringVar(&editNew, "new", "", "Replacement text (required)")
	editCmd.Flags().StringVarP(&editDescription, "description", "d", "", "Optional description of the edit (logged to log.md)")
	editCmd.MarkFlagRequired("old")
	editCmd.MarkFlagRequired("new")
	generateIndexCmd.Flags().BoolVar(&genOverwrite, "overwrite", false, "Overwrite existing index.md files")
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(readCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(grepCmd)
	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(generateIndexCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(typesCmd)
}

// openBundle opens the OKF bundle from the configured path.
// If no path is set, defaults to the current working directory.
func openBundle() (*knowledge_cat.Bundle, error) {
	path, err := resolveBundlePath()
	if err != nil {
		return nil, err
	}
	return knowledge_cat.Open(path)
}

// resolveBundlePath returns the bundle path from the --bundle flag or CWD.
func resolveBundlePath() (string, error) {
	if bundlePath != "" {
		return bundlePath, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}
	return cwd, nil
}

