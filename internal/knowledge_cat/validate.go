package knowledge_cat

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationResult holds the results of validating an OKF bundle against
// the v0.1 spec (Section 9 — Conformance).
type ValidationResult struct {
	// Valid is true if no conformance errors were found.
	Valid bool `json:"valid"`

	// Errors are spec violations that prevent conformance.
	Errors []ValidationIssue `json:"errors"`

	// Warnings are soft-guidance violations — recommended but not required.
	Warnings []ValidationIssue `json:"warnings"`

	// Info provides informational notes about the bundle.
	Info []ValidationIssue `json:"info"`

	// BrokenLinks lists all broken cross-concept links found during validation.
	BrokenLinks []BrokenLink `json:"broken_links,omitempty"`
}

// ValidationIssue represents a single validation finding.
type ValidationIssue struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// logDatePattern matches "## YYYY-MM-DD" headings in log.md.
var logDatePattern = regexp.MustCompile(`^## \d{4}-\d{2}-\d{2}$`)


// Validate checks an OKF bundle for conformance with the v0.1 spec.
//
// Conformance requirements (§9):
//  1. Every non-reserved .md file must have a parseable YAML frontmatter block.
//  2. Every frontmatter block must contain a non-empty `type` field.
//  3. Reserved filenames (index.md, log.md) must follow their respective
//     structures (§6, §7) when present.
//
// Soft checks (warnings):
//   - Missing recommended frontmatter fields (title, description).
//   - index.md entries without descriptions.
//   - log.md entries with non-standard date headings.
func Validate(path string) (*ValidationResult, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("validate: %s is not a directory", absPath)
	}

	result := &ValidationResult{Valid: true}

	// Go to lenient mode first: open the bundle to get parsed concepts.
	// Suppress stderr during Open() since we report warnings ourselves below.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	// Drain the pipe in background so writers don't block.
	go func() {
		var buf [4096]byte
		for {
			if _, readErr := r.Read(buf[:]); readErr != nil {
				return
			}
		}
	}()
	b, err := Open(absPath)
	w.Close()
	os.Stderr = origStderr
	if err != nil {
		return nil, fmt.Errorf("validate: open bundle: %w", err)
	}

	// Check 1 & 2: Every non-reserved .md file must have parseable
	// frontmatter with a non-empty type. We walk the tree ourselves
	// because Open() silently skips malformed concepts.
	err = filepath.WalkDir(absPath, func(filePath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		relPath, relErr := filepath.Rel(absPath, filePath)
		if relErr != nil {
			return fmt.Errorf("relative path for %s: %w", filePath, relErr)
		}

		// Check reserved files separately.
		if isReserved(d.Name()) {
			if d.Name() == "index.md" {
				validateIndexFile(absPath, relPath, result)
			}
			if d.Name() == "log.md" {
				validateLogFile(absPath, relPath, result)
			}
			return nil
		}

		conceptID := strings.TrimSuffix(relPath, ".md")

		// Read file content.
		content, readErr := os.ReadFile(filePath)
		if readErr != nil {
			result.Errors = append(result.Errors, ValidationIssue{
				File:    relPath,
				Message: fmt.Sprintf("cannot read file: %v", readErr),
			})
			result.Valid = false
			return nil
		}

		// Check that frontmatter is parseable.
		fm, _, fmErr := splitFrontmatter(string(content))
		if fmErr != nil {
			result.Errors = append(result.Errors, ValidationIssue{
				File:    relPath,
				Message: fmt.Sprintf("missing or unparseable YAML frontmatter: %v", fmErr),
			})
			result.Valid = false
			return nil
		}

		// Parse YAML to check required fields.
		var fields frontmatterFields
		if yamlErr := yaml.Unmarshal([]byte(fm), &fields); yamlErr != nil {
			result.Errors = append(result.Errors, ValidationIssue{
				File:    relPath,
				Message: fmt.Sprintf("invalid YAML frontmatter: %v", yamlErr),
			})
			result.Valid = false
			return nil
		}

		// Check 2: non-empty type.
		if strings.TrimSpace(fields.Type) == "" {
			result.Errors = append(result.Errors, ValidationIssue{
				File:    relPath,
				Message: fmt.Sprintf("missing required 'type' field (concept: %s)", conceptID),
			})
			result.Valid = false
		}

		// Soft checks on the parsed concept.
		c, ok := b.Concepts[conceptID]
		if !ok {
			// Concept didn't parse — already reported above, but add a note.
			result.Info = append(result.Info, ValidationIssue{
				File:    relPath,
				Message: fmt.Sprintf("concept %s was not loaded by lenient parser", conceptID),
			})
			return nil
		}

		// Warning: missing title.
		if c.Title == "" {
			result.Warnings = append(result.Warnings, ValidationIssue{
				File:    relPath,
				Message: fmt.Sprintf("missing recommended 'title' field (concept: %s)", conceptID),
			})
		}

		// Warning: missing description.
		if c.Description == "" {
			result.Warnings = append(result.Warnings, ValidationIssue{
				File:    relPath,
				Message: fmt.Sprintf("missing recommended 'description' field (concept: %s)", conceptID),
			})
		}

		// Warning: missing tags.
		if len(c.Tags) == 0 {
			result.Warnings = append(result.Warnings, ValidationIssue{
				File:    relPath,
				Message: fmt.Sprintf("missing recommended 'tags' field (concept: %s)", conceptID),
			})
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("validate: walk %s: %w", absPath, err)
	}

	// Info: directories without index.md.
	checkMissingIndexes(absPath, result)

	// Warning: broken cross-links between concepts.
	checkCrossLinks(absPath, b, result)

	return result, nil
}

// validateIndexFile checks an index.md file against the §6 structure.
// Index files must have no frontmatter, section headings, and properly
// formatted list entries.
func validateIndexFile(bundleRoot, relPath string, result *ValidationResult) {
	fullPath := filepath.Join(bundleRoot, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		result.Errors = append(result.Errors, ValidationIssue{
			File:    relPath,
			Message: fmt.Sprintf("cannot read index file: %v", err),
		})
		result.Valid = false
		return
	}

	text := string(content)

	// Check: index.md should not have YAML frontmatter.
	if strings.HasPrefix(strings.TrimSpace(text), "---") {
		matches := frontmatterPattern.FindStringSubmatch(text)
		if matches != nil {
			result.Warnings = append(result.Warnings, ValidationIssue{
				File:    relPath,
				Message: "index.md contains YAML frontmatter (only permitted for okf_version declaration at bundle root)",
			})
		}
	}

	// Check for at least one section heading.
	hasHeading := false
	hasEntry := false
	lines := strings.Split(text, "\n")
	currentHeading := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Track section headings.
		if blockHeadingPattern.MatchString(trimmed) {
			hasHeading = true
			matches := blockHeadingPattern.FindStringSubmatch(trimmed)
			if len(matches) >= 3 {
				currentHeading = strings.TrimSpace(matches[2])
			}
			continue
		}

		// Check for properly formatted entries.
		if indexEntryPattern.MatchString(trimmed) {
			hasEntry = true
			// Check entry has a non-empty description.
			entryMatches := indexEntryPattern.FindStringSubmatch(trimmed)
			if len(entryMatches) >= 4 && strings.TrimSpace(entryMatches[3]) == "" {
				result.Warnings = append(result.Warnings, ValidationIssue{
					File:    relPath,
					Line:    i + 1,
					Message: fmt.Sprintf("index entry missing description under %q", currentHeading),
				})
			}
		} else if strings.HasPrefix(trimmed, "*") {
			// Looks like an entry but doesn't match format.
			result.Warnings = append(result.Warnings, ValidationIssue{
				File:    relPath,
				Line:    i + 1,
				Message: "index entry does not match expected format '* [Title](url) - description'",
			})
		}
	}

	if !hasHeading {
		// Missing headings is info — empty index files are OK.
		result.Info = append(result.Info, ValidationIssue{
			File:    relPath,
			Message: "index.md has no section headings",
		})
	}

	if !hasEntry {
		result.Info = append(result.Info, ValidationIssue{
			File:    relPath,
			Message: "index.md has no list entries",
		})
	}
}

// validateLogFile checks a log.md file against the §7 structure.
// Log files must use date-grouped entries with ISO 8601 YYYY-MM-DD headings.
func validateLogFile(bundleRoot, relPath string, result *ValidationResult) {
	fullPath := filepath.Join(bundleRoot, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		result.Errors = append(result.Errors, ValidationIssue{
			File:    relPath,
			Message: fmt.Sprintf("cannot read log file: %v", err),
		})
		result.Valid = false
		return
	}

	text := string(content)
	lines := strings.Split(text, "\n")

	if strings.TrimSpace(text) == "" {
		result.Info = append(result.Info, ValidationIssue{
			File:    relPath,
			Message: "log.md is empty",
		})
		return
	}

	// Must have "# Directory Update Log" header.
	hasHeader := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "# Directory Update Log" {
			hasHeader = true
			break
		}
	}
	if !hasHeader {
		result.Warnings = append(result.Warnings, ValidationIssue{
			File:    relPath,
			Message: `log.md missing standard header "# Directory Update Log"`,
		})
	}

	// Check date headings follow YYYY-MM-DD format.
	currentDate := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "## ") {
			if !logDatePattern.MatchString(trimmed) {
				result.Warnings = append(result.Warnings, ValidationIssue{
					File:    relPath,
					Line:    i + 1,
					Message: fmt.Sprintf("log date heading %q does not use YYYY-MM-DD format", trimmed),
				})
			}
			currentDate = strings.TrimPrefix(trimmed, "## ")
			continue
		}

		// Check entries under a date heading have bold action.
		if strings.HasPrefix(trimmed, "* ") && currentDate != "" {
			if !strings.Contains(trimmed, "**") {
				result.Warnings = append(result.Warnings, ValidationIssue{
					File:    relPath,
					Line:    i + 1,
					Message: "log entry missing bold action word (convention: **Action**: description)",
				})
			}
		}
	}
}

// checkCrossLinks validates markdown links between concepts.
// Broken links are reported as warnings (spec §9 says consumers MUST tolerate them)
// and also collected in the structured BrokenLinks field.
func checkCrossLinks(bundleRoot string, b *Bundle, result *ValidationResult) {
	for id := range b.Concepts {
		for _, bl := range findBrokenLinks(b, id) {
			// Also check disk — concept might exist but not have been parsed.
			targetPath := filepath.Join(bundleRoot, bl.ResolvedID+".md")
			if _, statErr := os.Stat(targetPath); os.IsNotExist(statErr) {
				result.Warnings = append(result.Warnings, ValidationIssue{
					File:    bl.SourceID + ".md",
					Message: fmt.Sprintf("broken link to %q (target %q not found in bundle)", bl.LinkTarget, bl.ResolvedID),
				})
				result.BrokenLinks = append(result.BrokenLinks, bl)
			}
		}
	}
}

// checkMissingIndexes reports directories that don't have an index.md.
func checkMissingIndexes(bundleRoot string, result *ValidationResult) {
	err := filepath.WalkDir(bundleRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}

		// Check if this directory has an index.md.
		indexPath := filepath.Join(path, "index.md")
		if _, statErr := os.Stat(indexPath); os.IsNotExist(statErr) {
			relPath, _ := filepath.Rel(bundleRoot, path)
			if relPath == "." {
				relPath = "(bundle root)"
			}
			result.Info = append(result.Info, ValidationIssue{
				File:    filepath.Join(relPath, "index.md"),
				Message: "directory has no index.md (recommended for progressive disclosure)",
			})
		}
		return nil
	})
	if err != nil {
		// Non-fatal.
		result.Warnings = append(result.Warnings, ValidationIssue{
			File:    "",
			Message: fmt.Sprintf("error scanning for missing index files: %v", err),
		})
	}
}
