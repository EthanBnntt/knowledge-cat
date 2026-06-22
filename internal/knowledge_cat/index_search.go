package knowledge_cat

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// indexEntry represents a single entry in an index.md file.
// Format: * [Title](url) - description
type indexEntry struct {
	Section     string // the # heading this entry falls under
	Title       string // the linked text
	URL         string // the link target (e.g., "badges.md" or "subdir/index.md")
	Description string // text after the " - "
}

// HeadingMatch represents a match in a concept's markdown headings.
type HeadingMatch struct {
	Heading  string // e.g., "Schema", "Common query patterns"
	Level    int    // heading level (1 = #, 2 = ##, etc.)
	Snippet  string // first ~200 chars of content under the heading
}

// IndexMatch represents a match from an index.md entry.
type IndexMatch struct {
	IndexPath   string // e.g., "tables/index.md"
	Section     string // e.g., "BigQuery Table"
	ConceptID   string // derived from the URL, e.g., "tables/badges"
	ConceptTitle string // the link text
	Description string // the index entry description
}

// FindResult bundles all matches for a single concept from a find search.
type FindResult struct {
	ConceptID    string         `json:"concept_id"`
	ConceptTitle string         `json:"concept_title"`
	ConceptType  string         `json:"concept_type"`
	IndexMatches []IndexMatch   `json:"index_matches,omitempty"`
	Headings     []HeadingMatch `json:"heading_matches,omitempty"`
}

// FindOutput wraps the results of a find search.
type FindOutput struct {
	Query   string       `json:"query"`
	Results []FindResult `json:"results"`
}

// indexEntryPattern matches markdown list entries with links and descriptions.
// Matches: * [Title](url) - description
var indexEntryPattern = regexp.MustCompile(`^\*\s+\[([^\]]+)\]\(([^)]+)\)\s*-\s*(.*)$`)



// FindConcepts searches the OKF bundle for concepts matching the given query,
// searching:
//  1. All index.md files — matches against entry titles and descriptions.
//  2. All concept headings (e.g., "# Schema", "## Common patterns") —
//     matches against heading text and the content immediately below.
//
// Results are grouped by concept. If a concept appears in an index.md entry
// and also has matching headings, both are included under the same result.
func (b *Bundle) FindConcepts(query string) FindOutput {
	query = strings.ToLower(query)
	output := FindOutput{Query: query}

	// Map from concept ID to result (for merging index + heading matches).
	resultMap := make(map[string]*FindResult)

	// Phase 1: Search index.md files.
	b.searchIndexFiles(query, resultMap)

	// Phase 2: Search concept headings.
	b.searchConceptHeadings(query, resultMap)

	// Convert map to sorted slice.
	for _, r := range resultMap {
		output.Results = append(output.Results, *r)
	}
	sort.Slice(output.Results, func(i, j int) bool {
		return output.Results[i].ConceptID < output.Results[j].ConceptID
	})

	return output
}

// searchIndexFiles walks all index.md files and matches entries against the query.
func (b *Bundle) searchIndexFiles(query string, resultMap map[string]*FindResult) {
	filepath.WalkDir(b.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "index.md" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Compute the index path relative to the bundle root.
		indexRel, err := filepath.Rel(b.Path, path)
		if err != nil {
			return nil
		}

		entries := parseIndexFile(string(content))
		for _, entry := range entries {
			// Match against title and description.
			if !strings.Contains(strings.ToLower(entry.Title), query) &&
				!strings.Contains(strings.ToLower(entry.Description), query) {
				continue
			}

			// Derive concept ID from the URL.
			conceptID := deriveConceptID(entry.URL, filepath.Dir(indexRel))

			im := IndexMatch{
				IndexPath:    indexRel,
				Section:      entry.Section,
				ConceptID:    conceptID,
				ConceptTitle: entry.Title,
				Description:  entry.Description,
			}

			// Get or create the result entry.
			r, ok := resultMap[conceptID]
			if !ok {
				r = &FindResult{
					ConceptID:    conceptID,
					ConceptTitle: entry.Title,
				}
				// Try to get the type from the actual concept if loaded.
				if c := b.GetConcept(conceptID); c != nil {
					r.ConceptType = c.Type
				}
				resultMap[conceptID] = r
			}
			r.IndexMatches = append(r.IndexMatches, im)
		}

		return nil
	})
}

// searchConceptHeadings scans all loaded concepts for heading matches.
func (b *Bundle) searchConceptHeadings(query string, resultMap map[string]*FindResult) {
	for _, c := range b.Concepts {
		headings := extractHeadingMatches(c.Body, query)
		if len(headings) == 0 {
			continue
		}

		r, ok := resultMap[c.ID]
		if !ok {
			r = &FindResult{
				ConceptID:    c.ID,
				ConceptTitle: c.Title,
				ConceptType:  c.Type,
			}
			resultMap[c.ID] = r
		}
		r.Headings = append(r.Headings, headings...)
	}
}

// parseIndexFile parses an index.md file into structured entries.
// It tracks the current section heading and extracts list entries.
func parseIndexFile(content string) []indexEntry {
	var entries []indexEntry
	var currentSection string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Track section headings.
		if h := blockHeadingPattern.FindStringSubmatch(line); h != nil {
			// Only consider # and ## as section markers (index.md uses # for sections).
			if len(h[1]) <= 2 {
				currentSection = strings.TrimSpace(h[2])
			}
			continue
		}

		// Match list entries.
		m := indexEntryPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		entries = append(entries, indexEntry{
			Section:     currentSection,
			Title:       strings.TrimSpace(m[1]),
			URL:         strings.TrimSpace(m[2]),
			Description: strings.TrimSpace(m[3]),
		})
	}

	return entries
}

// extractHeadingMatches finds headings in a concept body that match the query,
// and returns each with a snippet of the content below.
func extractHeadingMatches(body, query string) []HeadingMatch {
	var matches []HeadingMatch
	lines := strings.Split(body, "\n")

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

		// Check if heading matches query.
		if !strings.Contains(strings.ToLower(heading), query) {
			i++
			continue
		}

		// Collect snippet: lines after the heading until next heading of same or higher level.
		var snippetLines []string
		j := i + 1
		for j < len(lines) {
			nextLine := strings.TrimSpace(lines[j])
			nh := blockHeadingPattern.FindStringSubmatch(nextLine)
			if nh != nil && len(nh[1]) <= level {
				break // hit a same-or-higher-level heading
			}
			if nextLine != "" {
				snippetLines = append(snippetLines, nextLine)
			}
			if len(snippetLines) >= 5 {
				break
			}
			j++
		}

		snippet := strings.Join(snippetLines, " ")
		if len(snippet) > 200 {
			snippet = snippet[:197] + "..."
		}

		matches = append(matches, HeadingMatch{
			Heading: heading,
			Level:   level,
			Snippet: snippet,
		})

		i = j
	}

	return matches
}

// deriveConceptID converts an index entry URL to a concept ID relative to the bundle root.
// For example, with indexDir = "tables" and url = "badges.md" → "tables/badges".
// With url = "../other/foo.md" → resolves relative path.
// With url = "subdir/" → strips trailing slash and appends "/index".
func deriveConceptID(url, indexDir string) string {
	// Remove .md suffix if present.
	url = strings.TrimSuffix(url, ".md")

	// If URL is a directory reference (ends with /), strip the slash.
	url = strings.TrimSuffix(url, "/")

	// If the URL starts with /, it's absolute from bundle root.
	if strings.HasPrefix(url, "/") {
		return strings.TrimPrefix(url, "/")
	}

	// Resolve relative to the index file's directory.
	if indexDir == "." {
		return url
	}

	resolved := filepath.Join(indexDir, url)
	// Clean up any ../ navigation.
	resolved = filepath.Clean(resolved)
	return resolved
}

