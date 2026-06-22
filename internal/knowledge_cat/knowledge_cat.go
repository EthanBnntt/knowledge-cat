// Package okf provides a library for reading, writing, and manipulating
// Open Knowledge Format (OKF) bundles.
//
// OKF is an open specification (v0.1) for representing organizational knowledge
// as a directory of markdown files with YAML frontmatter. See:
// https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
package knowledge_cat

import "time"

// Concept represents a single unit of knowledge within an OKF bundle.
// Each concept is backed by one markdown file with YAML frontmatter.
type Concept struct {
	// ID is the path of the concept's file within the bundle, with the .md
	// suffix removed. For example, "tables/orders".
	ID string

	// Type is the required frontmatter field identifying the kind of concept.
	// Examples: "BigQuery Table", "API Endpoint", "Metric", "Playbook".
	Type string

	// Title is the optional human-readable display name.
	Title string

	// Description is an optional one-line summary of the concept.
	Description string

	// Resource is an optional canonical URI for the underlying asset the
	// concept describes.
	Resource string

	// Tags is an optional list of strings for cross-cutting categorization.
	Tags []string

	// Timestamp is the optional ISO 8601 datetime of last meaningful change.
	Timestamp time.Time

	// Body is the markdown content after the YAML frontmatter block.
	Body string

	// Links contains the targets of markdown links found in the body.
	// Both absolute (/path/to/concept.md) and relative (../other.md) links are included.
	Links []string
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
