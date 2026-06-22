package knowledge_cat

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// frontmatterPattern matches a YAML frontmatter block delimited by --- lines.
var frontmatterPattern = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n?(.*)$`)

// markdownLinkPattern matches markdown links: [text](target)
var markdownLinkPattern = regexp.MustCompile(`\[.*?\]\(([^)]+)\)`)

// Reserved filenames that are NOT concept documents.
var reservedFiles = map[string]bool{
	"index.md": true,
	"log.md":   true,
}

// frontmatterFields is the structured representation of the YAML frontmatter.
// Tags uses a custom type to handle both YAML lists and plain comma-separated strings.
type frontmatterFields struct {
	Type        string     `yaml:"type"`
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	Resource    string     `yaml:"resource"`
	Tags        tagList    `yaml:"tags"`
	Timestamp   tagTime    `yaml:"timestamp"`
}

// tagList handles tags that may be a YAML list or a single comma-separated string.
type tagList []string

func (t *tagList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		// Standard YAML list: [tag1, tag2] or block list.
		var s []string
		if err := value.Decode(&s); err != nil {
			return err
		}
		*t = s
	case yaml.ScalarNode:
		// Single string: "tag1, tag2, tag3" or just "tag1".
		parts := strings.Split(value.Value, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		*t = result
	default:
		return fmt.Errorf("tags: expected list or string, got %v", value.Kind)
	}
	return nil
}

// tagTime handles timestamps that may be ISO 8601 strings or already parsed.
type tagTime struct {
	time.Time
}

func (t *tagTime) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("timestamp: expected string, got %v", value.Kind)
	}
	// Try multiple time formats.
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05+00:00", // ISO 8601 with +00:00 offset
		"2006-01-02",
	}
	for _, f := range formats {
		parsed, err := time.Parse(f, value.Value)
		if err == nil {
			t.Time = parsed
			return nil
		}
	}
	return fmt.Errorf("timestamp: cannot parse %q", value.Value)
}

// ParseConcept parses a single OKF concept from raw markdown content.
// It returns an error if the required type field is missing or empty.
func ParseConcept(id string, content []byte) (*Concept, error) {
	c := &Concept{ID: id}

	// Extract frontmatter and body.
	fm, body, err := splitFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse concept %s: %w", id, err)
	}

	// Parse YAML frontmatter.
	var fields frontmatterFields
	if err := yaml.Unmarshal([]byte(fm), &fields); err != nil {
		return nil, fmt.Errorf("parse concept %s: invalid YAML frontmatter: %w", id, err)
	}

	if fields.Type == "" {
		return nil, fmt.Errorf("parse concept %s: missing required 'type' field", id)
	}

	c.Type = fields.Type
	c.Title = fields.Title
	c.Description = fields.Description
	c.Resource = fields.Resource
	c.Tags = []string(fields.Tags)
	c.Timestamp = fields.Timestamp.Time
	c.Body = strings.TrimSpace(body)

	// Extract markdown links from body.
	c.Links = extractLinks(c.Body)

	return c, nil
}

// WriteConcept serializes a Concept back to its markdown representation
// and writes it to the appropriate file within the bundle.
func WriteConcept(bundlePath string, c *Concept) error {
	targetPath := filepath.Join(bundlePath, c.ID+".md")

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("write concept %s: %w", c.ID, err)
	}

	content := c.Marshal()
	return os.WriteFile(targetPath, content, 0o644)
}

// Marshal serializes a Concept to its markdown representation with
// YAML frontmatter.
func (c *Concept) Marshal() []byte {
	var b strings.Builder

	b.WriteString("---\n")

	// Required field.
	fmt.Fprintf(&b, "type: %s\n", c.Type)

	// Optional fields — only write if set.
	if c.Title != "" {
		fmt.Fprintf(&b, "title: %s\n", c.Title)
	}
	if c.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", c.Description)
	}
	if c.Resource != "" {
		fmt.Fprintf(&b, "resource: %s\n", c.Resource)
	}
	if len(c.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range c.Tags {
			fmt.Fprintf(&b, "  - %s\n", tag)
		}
	}
	if !c.Timestamp.IsZero() {
		fmt.Fprintf(&b, "timestamp: %s\n", c.Timestamp.Format(time.RFC3339))
	}

	b.WriteString("---\n")
	if c.Body != "" {
		b.WriteString("\n")
		b.WriteString(c.Body)
		b.WriteString("\n")
	}

	return []byte(b.String())
}

// splitFrontmatter splits a markdown document into its YAML frontmatter
// and body. Returns an error if the document doesn't begin with ---.
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	matches := frontmatterPattern.FindStringSubmatch(content)
	if matches == nil {
		// No frontmatter block found.
		return "", "", fmt.Errorf("no YAML frontmatter block found (must start with ---)")
	}
	return matches[1], matches[2], nil
}

// extractLinks parses all markdown link targets from body text.
func extractLinks(body string) []string {
	matches := markdownLinkPattern.FindAllStringSubmatch(body, -1)
	links := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, m := range matches {
		target := strings.TrimSpace(m[1])
		if !seen[target] {
			seen[target] = true
			links = append(links, target)
		}
	}
	return links
}

// EditConcept applies a text replacement to a concept's body.
// It finds oldText in the body, replaces it with newText, updates the
// concept's timestamp, writes it back to disk, and appends an entry to
// the bundle root log.md.
//
// description is optional — if empty, a default description is generated.
// Returns the updated concept and any error.
func EditConcept(bundlePath, conceptID, oldText, newText, description string) (*Concept, error) {
	// Open the bundle to find the concept.
	b, err := Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("edit concept %s: %w", conceptID, err)
	}

	c := b.GetConcept(conceptID)
	if c == nil {
		return nil, fmt.Errorf("edit concept: %s not found in bundle", conceptID)
	}

	// Find and replace the text in the body.
	if !strings.Contains(c.Body, oldText) {
		return nil, fmt.Errorf("edit concept %s: existing text not found in body", conceptID)
	}

	c.Body = strings.Replace(c.Body, oldText, newText, 1)

	// Update timestamp.
	c.Timestamp = time.Now()

	// Re-extract links from updated body.
	c.Links = extractLinks(c.Body)

	// Write the concept back to disk.
	if err := WriteConcept(bundlePath, c); err != nil {
		return nil, fmt.Errorf("edit concept %s: %w", conceptID, err)
	}

	// Append to log.md.
	if description == "" {
		description = fmt.Sprintf("Edited concept [%s](/%s.md).", c.Title, conceptID)
	}

	logEntry := LogEntry{
		Date:        time.Now(),
		Action:      "Edit",
		Description: description,
	}
	if err := AppendLog(bundlePath, logEntry); err != nil {
		return c, fmt.Errorf("edit concept %s: saved but failed to update log: %w", conceptID, err)
	}

	return c, nil
}

// CreateConcept creates a new concept document in the bundle.
// The conceptID is the path within the bundle (without .md suffix, e.g. "tables/orders").
// conceptType is required; all other fields are optional.
// The concept is written to disk, logged, and the bundle is re-opened to
// return the parsed result. Returns an error if a concept already exists at
// the given path or if the conceptID would collide with a reserved filename.
func CreateConcept(bundlePath, conceptID, conceptType, title, description, resource string, tags []string, body string) (*Concept, error) {
	// Validate concept ID.
	if conceptID == "" {
		return nil, fmt.Errorf("create concept: concept ID is required")
	}
	if conceptType == "" {
		return nil, fmt.Errorf("create concept %s: type is required", conceptID)
	}

	// Check reserved filenames.
	filename := filepath.Base(conceptID) + ".md"
	if isReserved(filename) {
		return nil, fmt.Errorf("create concept: %q is a reserved filename (index.md, log.md)", filename)
	}

	// Check it doesn't already exist.
	targetPath := filepath.Join(bundlePath, conceptID+".md")
	if _, err := os.Stat(targetPath); err == nil {
		return nil, fmt.Errorf("create concept: %s already exists (use edit to modify)", conceptID)
	}

	c := &Concept{
		ID:          conceptID,
		Type:        conceptType,
		Title:       title,
		Description: description,
		Resource:    resource,
		Tags:        tags,
		Timestamp:   time.Now(),
		Body:        strings.TrimSpace(body),
		Links:       extractLinks(body),
	}

	if err := WriteConcept(bundlePath, c); err != nil {
		return nil, fmt.Errorf("create concept %s: %w", conceptID, err)
	}

	// Log the creation.
	logDesc := description
	if logDesc == "" {
		if title != "" {
			logDesc = fmt.Sprintf("Created [%s](/%s.md) (%s).", title, conceptID, conceptType)
		} else {
			logDesc = fmt.Sprintf("Created [%s](/%s.md) (%s).", conceptID, conceptID, conceptType)
		}
	}
	logEntry := LogEntry{
		Date:        time.Now(),
		Action:      "Creation",
		Description: logDesc,
	}
	if err := AppendLog(bundlePath, logEntry); err != nil {
		return c, fmt.Errorf("create concept %s: saved but failed to update log: %w", conceptID, err)
	}

	return c, nil
}

// isReserved returns true if filename is a reserved file (index.md, log.md).
func isReserved(filename string) bool {
	return reservedFiles[filename]
}
