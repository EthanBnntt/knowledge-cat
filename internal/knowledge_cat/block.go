package knowledge_cat

import (
	"fmt"
	"regexp"
	"strings"
)

// Block represents a heading-delimited section of a concept body.
// Each block starts at a markdown heading and includes all content
// until the next heading of equal or higher level.
//
// Blocks can be referenced using the syntax conceptID#blockID,
// e.g., "tables/badges#schema" or "tables/badges#common-query-patterns".
type Block struct {
	// ID is the slugified heading text used for addressing.
	// Derived by lowercasing, replacing spaces/punctuation with hyphens,
	// and collapsing runs. E.g., "Common query patterns" → "common-query-patterns".
	ID string `json:"id"`

	// Heading is the original heading text without the # prefix.
	Heading string `json:"heading"`

	// Level is the heading level (1 for #, 2 for ##, etc.).
	Level int `json:"level"`

	// Content is the markdown content under this heading, including
	// sub-headings and their content, until the next heading at this
	// level or higher.
	Content string `json:"content"`
}

// headingLinePattern matches a markdown ATX heading line: # Heading
var blockHeadingPattern = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

// ParseBlocks splits a concept body into blocks delimited by headings.
// If the body has content before the first heading, it is treated as an
// untitled preamble block with ID "top".
//
// Block IDs are derived from heading text by slugifying:
//   - Lowercased
//   - Spaces and punctuation replaced with hyphens
//   - Multiple hyphens collapsed
//   - Leading/trailing hyphens stripped
func ParseBlocks(body string) []Block {
	if strings.TrimSpace(body) == "" {
		return nil
	}

	lines := strings.Split(body, "\n")
	var blocks []Block

	// Collect any content before the first heading as the "top" preamble.
	preamble := collectPreamble(lines)
	if preamble != "" {
		blocks = append(blocks, Block{
			ID:      "top",
			Heading: "",
			Level:   0,
			Content: preamble,
		})
	}

	// Find headings and their content blocks.
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		h := blockHeadingPattern.FindStringSubmatch(line)
		if h == nil {
			i++
			continue
		}

		level := len(h[1])
		heading := strings.TrimSpace(h[2])
		blockID := slugify(heading)

		// Collect content until next heading of equal or higher level.
		contentLines := []string{}
		j := i + 1
		for j < len(lines) {
			nextLine := strings.TrimSpace(lines[j])
			nh := blockHeadingPattern.FindStringSubmatch(nextLine)
			if nh != nil && len(nh[1]) <= level {
				break
			}
			contentLines = append(contentLines, lines[j])
			j++
		}

		content := strings.Join(contentLines, "\n")

		blocks = append(blocks, Block{
			ID:      blockID,
			Heading: heading,
			Level:   level,
			Content: content,
		})

		i = j
	}

	return blocks
}

// GetBlock returns the block with the given ID from a concept body.
// Returns nil if no block with that ID exists.
func GetBlock(body, blockID string) *Block {
	blocks := ParseBlocks(body)
	for i := range blocks {
		if blocks[i].ID == blockID {
			return &blocks[i]
		}
	}
	return nil
}

// findBlockForLine returns the block that contains the given line number.
// lineNumber is 1-indexed. Returns nil if no block contains that line.
func findBlockForLine(body string, lineNumber int) *Block {
	blocks := ParseBlocks(body)
	if len(blocks) == 0 {
		return nil
	}

	currentLine := 1
	for _, block := range blocks {
		blockContentLines := strings.Count(block.Content, "\n") + 1
		if strings.TrimSpace(block.Content) == "" {
			blockContentLines = 0
		}

		headingLine := 0
		if block.Level > 0 {
			headingLine = 1
		}

		blockStart := currentLine
		blockEnd := blockStart + headingLine + blockContentLines - 1

		if lineNumber >= blockStart && lineNumber <= blockEnd {
			return &block
		}
		currentLine = blockEnd + 1
	}

	return nil
}

// collectPreamble collects lines before the first heading.
func collectPreamble(lines []string) string {
	var preamble []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if blockHeadingPattern.MatchString(trimmed) {
			break
		}
		preamble = append(preamble, line)
	}
	return strings.Join(preamble, "\n")
}


// slugify converts a heading string into a block ID.
// "Common query patterns" → "common-query-patterns"
// "Schema" → "schema"
func slugify(s string) string {
	s = strings.ToLower(s)
	// Replace spaces, underscores, and common punctuation with hyphens.
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, "/", "-")
	// Collapse runs of hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	// Strip non-alphanumeric, non-hyphen characters.
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s = b.String()
	// Strip leading/trailing hyphens.
	s = strings.Trim(s, "-")
	return s
}

// ParseConceptRef splits a concept reference into concept ID and optional block ID.
// "tables/badges" → ("tables/badges", "")
// "tables/badges#schema" → ("tables/badges", "schema")
// "#schema" → ("", "schema")
func ParseConceptRef(ref string) (conceptID, blockID string) {
	if idx := strings.LastIndex(ref, "#"); idx >= 0 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
}

// BlockNotFoundMessage returns a message describing a missing block, listing
// the available block IDs.
func BlockNotFoundMessage(conceptID, blockID, body string) string {
	blocks := ParseBlocks(body)
	ids := make([]string, len(blocks))
	for i, bl := range blocks {
		ids[i] = bl.ID
	}
	return fmt.Sprintf("block %q not found in %s. Available blocks: %s",
		blockID, conceptID, strings.Join(ids, ", "))
}
