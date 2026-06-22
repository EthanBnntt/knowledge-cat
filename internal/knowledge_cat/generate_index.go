package knowledge_cat

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GenerateIndex generates or regenerates index.md files for directories in
// the OKF bundle. For each directory, it scans concept documents and
// subdirectories, groups them by their `type` field, and writes an index.md
// following the OKF spec §6 structure.
//
// If overwrite is false, existing index.md files are skipped.
// If dir is non-empty, only that directory (relative to bundle root) is processed.
//
// Returns the paths (relative to bundle root) of index.md files written.
func GenerateIndex(bundlePath string, overwrite bool, dir string) ([]string, error) {
	absPath, err := filepath.Abs(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("generate-index: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("generate-index: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("generate-index: %s is not a directory", absPath)
	}

	// Open the bundle to get parsed concepts.
	b, err := Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("generate-index: open bundle: %w", err)
	}

	var written []string

	// Walk directories.
	walkRoot := absPath
	if dir != "" {
		walkRoot = filepath.Join(absPath, dir)
		if stat, statErr := os.Stat(walkRoot); statErr != nil || !stat.IsDir() {
			return nil, fmt.Errorf("generate-index: directory not found: %s", dir)
		}
	}

	err = filepath.WalkDir(walkRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}

		// Determine if this directory has an existing index.md.
		indexPath := filepath.Join(path, "index.md")
		indexExists := false
		if _, statErr := os.Stat(indexPath); statErr == nil {
			indexExists = true
		}
		if indexExists && !overwrite {
			return nil
		}

		// Collect entries for this directory.
		entries := collectDirEntries(path, absPath, b)

		if len(entries) == 0 {
			return nil
		}

		// Generate index content.
		content := buildIndexContent(entries)

		// Write the file.
		if writeErr := os.WriteFile(indexPath, []byte(content), 0o644); writeErr != nil {
			return fmt.Errorf("write %s: %w", indexPath, writeErr)
		}

		relPath, _ := filepath.Rel(absPath, indexPath)
		written = append(written, relPath)

		return nil
	})

	if err != nil {
		return written, fmt.Errorf("generate-index: %w", err)
	}

	return written, nil
}

// dirEntry represents a single index.md entry for a concept or subdirectory.
type dirEntry struct {
	Title       string
	URL         string
	Description string
	Type        string
	IsDirectory bool
}

// collectDirEntries scans a directory for concepts and subdirectories to
// include in its index.md.
func collectDirEntries(dirPath, bundleRoot string, b *Bundle) []dirEntry {
	var entries []dirEntry

	dirContent, err := os.ReadDir(dirPath)
	if err != nil {
		return nil
	}

	for _, entry := range dirContent {
		name := entry.Name()

		if name == "index.md" || name == "log.md" {
			continue
		}

		relPath, _ := filepath.Rel(bundleRoot, filepath.Join(dirPath, entry.Name()))

		if entry.IsDir() {
			// Subdirectory entry.
			title := name
			entries = append(entries, dirEntry{
				Title:       title,
				URL:         name + "/",
				Description: "",
				Type:        "Directory",
				IsDirectory: true,
			})
			continue
		}

		if !strings.HasSuffix(name, ".md") {
			continue
		}

		conceptID := strings.TrimSuffix(relPath, ".md")

		// Derive title from concept or filename.
		title := strings.TrimSuffix(name, ".md")
		description := ""
		conceptType := ""

		if c, ok := b.Concepts[conceptID]; ok {
			if c.Title != "" {
				title = c.Title
			}
			if c.Description != "" {
				description = c.Description
			}
			conceptType = c.Type
		}

		entries = append(entries, dirEntry{
			Title:       title,
			URL:         name,
			Description: description,
			Type:        conceptType,
		})
	}

	// Sort: directories first, then concepts grouped by type, then alphabetically.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDirectory != entries[j].IsDirectory {
			return entries[i].IsDirectory
		}
		if entries[i].Type != entries[j].Type {
			return entries[i].Type < entries[j].Type
		}
		return entries[i].Title < entries[j].Title
	})

	return entries
}

// buildIndexContent generates the markdown content for an index.md file
// from a list of entries, grouped by type.
func buildIndexContent(entries []dirEntry) string {
	var b strings.Builder

	// Group entries by type.
	groups := make(map[string][]dirEntry)
	var groupOrder []string

	for _, e := range entries {
		typeLabel := e.Type
		if typeLabel == "" {
			if e.IsDirectory {
				typeLabel = "Subdirectories"
			} else {
				typeLabel = "Concepts"
			}
		}
		if _, ok := groups[typeLabel]; !ok {
			groupOrder = append(groupOrder, typeLabel)
		}
		groups[typeLabel] = append(groups[typeLabel], e)
	}

	for i, typeLabel := range groupOrder {
		group := groups[typeLabel]

		if i > 0 {
			b.WriteString("\n")
		}

		b.WriteString(fmt.Sprintf("# %s\n", typeLabel))

		for _, e := range group {
			if e.Description != "" {
				b.WriteString(fmt.Sprintf("* [%s](%s) - %s\n", e.Title, e.URL, e.Description))
			} else {
				b.WriteString(fmt.Sprintf("* [%s](%s)\n", e.Title, e.URL))
			}
		}
	}

	return b.String()
}
