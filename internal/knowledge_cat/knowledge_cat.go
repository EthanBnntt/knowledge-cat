// Package okf provides a library for reading, writing, and manipulating
// Open Knowledge Format (OKF) bundles.
//
// OKF is an open specification (v0.1) for representing organizational knowledge
// as a directory of markdown files with YAML frontmatter. See:
// https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
package knowledge_cat

import (
	"fmt"
	"os"
	"time"
)

// Concept represents a single unit of knowledge within an OKF bundle.
// Each concept is backed by one markdown file with YAML frontmatter.
type Concept struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Resource    string    `json:"resource,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Timestamp   time.Time `json:"timestamp,omitempty"`
	Body        string    `json:"body,omitempty"`
	Links       []string  `json:"links,omitempty"`
}

// TimestampString returns the timestamp formatted as ISO 8601, or an empty
// string if the timestamp is zero.
func (c *Concept) TimestampString() string {
	if c.Timestamp.IsZero() {
		return ""
	}
	return c.Timestamp.Format("2006-01-02T15:04:05Z")
}

// ListFilter controls which concepts are returned by Bundle.List.
// All fields are optional — nil/zero values mean "no filter".
type ListFilter struct {
	// Type filters concepts to those with the given type. Case-sensitive.
	Type string
	// Tags filters concepts to those containing at least one of the given tags.
	Tags []string
}

// Bundle represents a self-contained OKF knowledge bundle — a directory tree
// of markdown files.
type Bundle struct {
	// Path is the absolute filesystem path to the bundle root.
	Path string

	// Concepts maps concept IDs to their parsed Concept structs.
	Concepts map[string]*Concept
}

// ResolveBundlePath returns the first non-empty candidate as the bundle path,
// falling back to the current working directory. Used by both the CLI and MCP
// server to resolve bundle paths with consistent fallback behavior.
func ResolveBundlePath(candidates ...string) (string, error) {
	for _, c := range candidates {
		if c != "" {
			return c, nil
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}
	return cwd, nil
}
