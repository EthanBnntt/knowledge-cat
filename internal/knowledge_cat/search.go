package knowledge_cat

import "strings"

// SearchResult represents a single match from a search query.
type SearchResult struct {
	Concept *Concept
	// Field is where the match was found: "title", "description", "body".
	Field string
	// Line is the matching line (or portion of body around the match).
	Line string
	// BlockID is the block containing this match, if the match is in the body
	// and the body has heading-delimited blocks. E.g., "schema", "citations".
	BlockID string
}

// Search performs a case-insensitive text search across all concepts in the
// bundle. It searches title, description, and body fields.
//
// If types is non-empty, only concepts of those types are searched.
func (b *Bundle) Search(query string, types []string) []SearchResult {
	query = strings.ToLower(query)
	var results []SearchResult

	for _, c := range b.Concepts {
		if len(types) > 0 {
			if !containsType(types, c.Type) {
				continue
			}
		}

		// Search title.
		if matchLine(c.Title, query) {
			results = append(results, SearchResult{
				Concept: c,
				Field:   "title",
				Line:    c.Title,
			})
		}

		// Search description.
		if matchLine(c.Description, query) {
			results = append(results, SearchResult{
				Concept: c,
				Field:   "description",
				Line:    c.Description,
			})
		}

		// Search body — return matching lines with block context.
		bodyLines := strings.Split(c.Body, "\n")
		for lineNum, line := range bodyLines {
			if matchLine(line, query) {
				blockID := ""
				if b := FindBlockForLine(c.Body, lineNum+1); b != nil {
					blockID = b.ID
				}
				results = append(results, SearchResult{
					Concept: c,
					Field:   "body",
					Line:    strings.TrimSpace(line),
					BlockID: blockID,
				})
			}
		}
	}

	return results
}

// matchLine returns true if the line contains the query (case-insensitive).
func matchLine(line, query string) bool {
	return strings.Contains(strings.ToLower(line), query)
}

// containsType returns true if the slice contains the given type.
func containsType(types []string, t string) bool {
	for _, tt := range types {
		if tt == t {
			return true
		}
	}
	return false
}

// TruncateString truncates s to maxLen characters, appending "..." if truncated.
func TruncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
