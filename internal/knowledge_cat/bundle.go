package knowledge_cat

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Open opens an OKF bundle at the given path. It walks the directory tree,
// parses all concept documents (.md files except index.md and log.md),
// and returns a Bundle.
//
// Open is lenient: non-concept .md files without valid frontmatter are
// skipped with a warning rather than causing an error, matching OKF's
// permissive consumption model.
func Open(path string) (*Bundle, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("open bundle: %s is not a directory", absPath)
	}

	b := &Bundle{
		Path:     absPath,
		Concepts: make(map[string]*Concept),
	}

	var warnings []string

	err = filepath.WalkDir(absPath, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-markdown files.
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		// Skip reserved files (index.md, log.md).
		if isReserved(d.Name()) {
			return nil
		}

		// Compute concept ID relative to bundle root.
		relPath, err := filepath.Rel(absPath, filePath)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", filePath, err)
		}
		conceptID := strings.TrimSuffix(relPath, ".md")

		// Read and parse the concept.
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read %s: %w", filePath, err)
		}

		concept, err := ParseConcept(conceptID, content)
		if err != nil {
			// Lenient: warn and skip malformed concepts.
			warnings = append(warnings, fmt.Sprintf("skipping %s: %v", relPath, err))
			return nil
		}

		b.Concepts[conceptID] = concept
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("open bundle: walk %s: %w", absPath, err)
	}

	// Print warnings to stderr if any.
	if len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "know: warning: %s\n", w)
		}
	}

	return b, nil
}

// GetConcept returns the concept with the given ID, or nil if not found.
func (b *Bundle) GetConcept(id string) *Concept {
	return b.Concepts[id]
}

// List returns concepts matching the optional filter. If filter is nil,
// all concepts are returned. Concepts are sorted by ID.
func (b *Bundle) List(filter *ListFilter) []*Concept {
	result := make([]*Concept, 0, len(b.Concepts))

	for _, c := range b.Concepts {
		if filter != nil {
			if filter.Type != "" && c.Type != filter.Type {
				continue
			}
			if len(filter.Tags) > 0 && !hasAnyTag(c.Tags, filter.Tags) {
				continue
			}
		}
		result = append(result, c)
	}

	// Sort by ID for deterministic output.
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// ListTypes returns all unique type values and tag values used in the bundle.
// Useful for discoverability — letting users/agents see what's available.
func (b *Bundle) ListTypes() (types []string, tags []string) {
	typeSet := make(map[string]bool)
	tagSet := make(map[string]bool)

	for _, c := range b.Concepts {
		typeSet[c.Type] = true
		for _, t := range c.Tags {
			tagSet[t] = true
		}
	}

	for t := range typeSet {
		types = append(types, t)
	}
	for t := range tagSet {
		tags = append(tags, t)
	}

	sort.Strings(types)
	sort.Strings(tags)
	return types, tags
}

// hasAnyTag returns true if the concept tags include at least one of the
// filter tags.
func hasAnyTag(conceptTags, filterTags []string) bool {
	for _, ft := range filterTags {
		for _, ct := range conceptTags {
			if ct == ft {
				return true
			}
		}
	}
	return false
}


