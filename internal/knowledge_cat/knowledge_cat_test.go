package knowledge_cat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// Block parsing tests
// ============================================================================

func TestParseBlocks(t *testing.T) {
	body := `Preamble text before any heading.

# First Heading
Content under first heading.

## Subheading
Sub-content here.

# Second Heading
Final content.`

	blocks := ParseBlocks(body)
	// ParseBlocks extracts only TOP-LEVEL blocks: ## subheadings are
	// folded into their parent # block (stopping at next heading of
	// equal or higher level). So ## Subheading is inside first-heading.
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks (preamble + 2 top-level headings), got %d", len(blocks))
	}

	// Preamble block.
	if blocks[0].ID != "top" {
		t.Errorf("preamble ID = %q, want 'top'", blocks[0].ID)
	}
	if blocks[0].Level != 0 {
		t.Errorf("preamble level = %d, want 0", blocks[0].Level)
	}

	// First heading.
	if blocks[1].Heading != "First Heading" {
		t.Errorf("heading = %q, want 'First Heading'", blocks[1].Heading)
	}
	if blocks[1].ID != "first-heading" {
		t.Errorf("block ID = %q, want 'first-heading'", blocks[1].ID)
	}
	if !strings.Contains(blocks[1].Content, "## Subheading") {
		t.Error("first-heading content should include ## Subheading")
	}

	// Second heading.
	if blocks[2].ID != "second-heading" {
		t.Errorf("last block ID = %q, want 'second-heading'", blocks[2].ID)
	}

	// Empty body.
	if ParseBlocks("") != nil {
		t.Error("ParseBlocks('') should return nil")
	}
	if ParseBlocks("   \n  \n") != nil {
		t.Error("ParseBlocks(whitespace) should return nil")
	}
}

func TestGetBlock(t *testing.T) {
	body := `# Schema
Schema content here.

# Examples
Example content.`

	b := GetBlock(body, "schema")
	if b == nil {
		t.Fatal("GetBlock('schema') returned nil")
	}
	if !strings.Contains(b.Content, "Schema content") {
		t.Errorf("block content = %q, want 'Schema content'", b.Content)
	}

	b = GetBlock(body, "nonexistent")
	if b != nil {
		t.Error("GetBlock('nonexistent') should return nil")
	}
}

func TestParseConceptRef(t *testing.T) {
	concept, block := ParseConceptRef("tables/badges#schema")
	if concept != "tables/badges" {
		t.Errorf("concept = %q, want 'tables/badges'", concept)
	}
	if block != "schema" {
		t.Errorf("block = %q, want 'schema'", block)
	}

	concept, block = ParseConceptRef("tables/badges")
	if concept != "tables/badges" || block != "" {
		t.Errorf("no-block: concept=%q block=%q, want concept='tables/badges' block=''", concept, block)
	}

	concept, block = ParseConceptRef("#schema")
	if concept != "" || block != "schema" {
		t.Errorf("fragment-only: concept=%q block=%q, want concept='' block='schema'", concept, block)
	}
}

func TestFindBlockForLine(t *testing.T) {
	body := "Preamble\n\n# First\nLine 1\nLine 2\nLine 3\n\n# Second\nLine 4"

	// Line 1: "Preamble" — in top block.
	b := FindBlockForLine(body, 1)
	if b == nil || b.ID != "top" {
		t.Errorf("line 1: got %v, want 'top'", b)
	}

	// Line 2: blank — in top block.
	b = FindBlockForLine(body, 2)
	if b == nil || b.ID != "top" {
		t.Errorf("line 2 (blank): got %v, want 'top'", b)
	}

	// Line 3: "# First" — heading line belongs to first block.
	b = FindBlockForLine(body, 3)
	if b == nil || b.ID != "first" {
		t.Errorf("line 3 (heading): got %v, want 'first'", b)
	}

	// Line 4: "Line 1" — content of first block.
	b = FindBlockForLine(body, 4)
	if b == nil || b.ID != "first" {
		t.Errorf("line 4: got %v, want 'first'", b)
	}

	// Line 9: "Line 4" — content of second block.
	b = FindBlockForLine(body, 9)
	if b == nil || b.ID != "second" {
		t.Errorf("line 9: got %v, want 'second'", b)
	}

	// Past end.
	b = FindBlockForLine(body, 100)
	if b != nil {
		t.Error("line 100 should return nil")
	}

	// Empty body.
	if FindBlockForLine("", 1) != nil {
		t.Error("empty body should return nil")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Schema", "schema"},
		{"Common query patterns", "common-query-patterns"},
		{"Hello_World", "hello-world"},
		{"A/B Test", "a-b-test"},
		{"  Leading and trailing  ", "leading-and-trailing"},
		{"Multiple---dashes", "multiple-dashes"},
		{"Special!@#Chars", "specialchars"},
	}
	for _, tc := range tests {
		got := slugify(tc.in)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ============================================================================
// YAML unmarshaler tests
// ============================================================================

func TestTagListUnmarshalYAML(t *testing.T) {
	// Comma-separated string (scalar).
	content := []byte("---\ntype: Test\ntags: a, b, c\n---\nbody")
	c, err := ParseConcept("test", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tags) != 3 || c.Tags[0] != "a" || c.Tags[1] != "b" || c.Tags[2] != "c" {
		t.Errorf("comma tags = %v, want [a b c]", c.Tags)
	}

	// YAML list.
	content = []byte("---\ntype: Test\ntags:\n  - x\n  - y\n---\nbody")
	c, err = ParseConcept("test", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tags) != 2 || c.Tags[0] != "x" || c.Tags[1] != "y" {
		t.Errorf("list tags = %v, want [x y]", c.Tags)
	}

	// Comma-separated with spaces.
	content = []byte("---\ntype: Test\ntags: \"one, two, three\"\n---\nbody")
	c, err = ParseConcept("test", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tags) != 3 || c.Tags[0] != "one" {
		t.Errorf("spaced comma tags = %v", c.Tags)
	}
}

func TestTagTimeUnmarshalYAML(t *testing.T) {
	tests := []string{
		"2026-06-22T12:00:00Z",
		"2026-06-22T12:00:00.123456789Z",
		"2026-05-28T23:32:46+00:00",
		"2026-06-22",
	}
	for _, ts := range tests {
		content := []byte(fmt.Sprintf("---\ntype: Test\ntimestamp: %s\n---\nbody", ts))
		c, err := ParseConcept("test", content)
		if err != nil {
			t.Errorf("timestamp %q: ParseConcept failed: %v", ts, err)
			continue
		}
		if c.Timestamp.IsZero() {
			t.Errorf("timestamp %q: parsed to zero time", ts)
		}
	}

	// Invalid timestamp.
	content := []byte("---\ntype: Test\ntimestamp: not-a-date\n---\nbody")
	_, err := ParseConcept("test", content)
	if err == nil {
		t.Error("expected error for invalid timestamp")
	}
}

func TestParseConceptErrors(t *testing.T) {
	// No frontmatter.
	_, err := ParseConcept("test", []byte("just body"))
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}

	// Missing type.
	_, err = ParseConcept("test", []byte("---\ntitle: NoType\n---\nbody"))
	if err == nil {
		t.Error("expected error for missing type")
	}
}

// ============================================================================
// Filesystem tests (temp dirs)
// ============================================================================

func makeTestBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Write two concepts.
	concept1 := `---
type: Test Type
title: Concept One
description: First test concept.
tags:
  - tag1
  - tag2
timestamp: 2026-06-22T12:00:00Z
---

# Heading 1
Body of concept one.
Link to [concept two](concept-two.md).
`
	concept2 := `---
type: Other Type
title: Concept Two
description: Second test concept.
---
# Schema
Schema content for concept two.
`
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "concept-one.md"), []byte(concept1), 0o644)
	os.WriteFile(filepath.Join(dir, "concept-two.md"), []byte(concept2), 0o644)
	return dir
}

func TestWriteConcept(t *testing.T) {
	dir := t.TempDir()
	c := &Concept{
		ID:          "test/concept",
		Type:        "Test Type",
		Title:       "Test Title",
		Description: "A description.",
		Tags:        []string{"a", "b"},
		Body:        "# Body\nContent.",
	}
	if err := WriteConcept(dir, c); err != nil {
		t.Fatal(err)
	}

	// Read back and verify via ParseConcept.
	content, err := os.ReadFile(filepath.Join(dir, "test/concept.md"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseConcept("test/concept", content)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Type != "Test Type" {
		t.Errorf("type = %q", parsed.Type)
	}
	if len(parsed.Tags) != 2 {
		t.Errorf("tags = %v", parsed.Tags)
	}
}

func TestOpenBundle(t *testing.T) {
	dir := makeTestBundle(t)
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Concepts) != 2 {
		t.Fatalf("expected 2 concepts, got %d", len(b.Concepts))
	}

	c := b.GetConcept("concept-one")
	if c == nil {
		t.Fatal("concept-one not found")
	}
	if c.Type != "Test Type" || c.Title != "Concept One" {
		t.Errorf("concept-one: type=%q title=%q", c.Type, c.Title)
	}

	c = b.GetConcept("concept-two")
	if c == nil {
		t.Fatal("concept-two not found")
	}

	// GetConcept for missing.
	if b.GetConcept("nonexistent") != nil {
		t.Error("GetConcept(nonexistent) should return nil")
	}
}

func TestOpenErrors(t *testing.T) {
	_, err := Open("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}

	f := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(f, []byte("not a dir"), 0o644)
	_, err = Open(f)
	if err == nil {
		t.Error("expected error for non-directory")
	}
}

func TestList(t *testing.T) {
	dir := makeTestBundle(t)
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	// No filter.
	all := b.List(nil)
	if len(all) != 2 {
		t.Errorf("List(nil) = %d, want 2", len(all))
	}

	// Type filter.
	filtered := b.List(&ListFilter{Type: "Test Type"})
	if len(filtered) != 1 || filtered[0].ID != "concept-one" {
		t.Errorf("List(type=Test Type) = %d concepts, want 1", len(filtered))
	}

	// Tag filter.
	filtered = b.List(&ListFilter{Tags: []string{"tag1"}})
	if len(filtered) != 1 || filtered[0].ID != "concept-one" {
		t.Errorf("List(tags=[tag1]) = %d concepts, want 1", len(filtered))
	}

	// Tag filter with no match.
	filtered = b.List(&ListFilter{Tags: []string{"nonexistent"}})
	if len(filtered) != 0 {
		t.Errorf("List(tags=[nonexistent]) = %d, want 0", len(filtered))
	}
}

func TestListTypes(t *testing.T) {
	dir := makeTestBundle(t)
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	types, tags := b.ListTypes()
	if len(types) != 2 {
		t.Errorf("expected 2 types, got %v", types)
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %v", tags)
	}
}

func TestEditConcept(t *testing.T) {
	dir := makeTestBundle(t)
	c, err := EditConcept(dir, "concept-one", "Body of concept one.", "Edited body content.", "test edit")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Body, "Edited body content.") {
		t.Errorf("body = %q, want 'Edited body content.'", c.Body)
	}
	if c.Timestamp.IsZero() {
		t.Error("timestamp not updated")
	}

	// Verify log.md was created.
	logContent, err := os.ReadFile(filepath.Join(dir, "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logContent), "test edit") {
		t.Errorf("log.md missing edit description: %s", string(logContent))
	}

	// Edit with missing concept.
	_, err = EditConcept(dir, "nonexistent", "old", "new", "")
	if err == nil {
		t.Error("expected error for nonexistent concept")
	}

	// Edit with text not found.
	_, err = EditConcept(dir, "concept-one", "text that doesn't exist", "new", "")
	if err == nil {
		t.Error("expected error for text not found")
	}

	// Edit with empty description (tests default-description branch).
	c, err = EditConcept(dir, "concept-one", "Edited body content.", "Final edit.", "")
	if err != nil {
		t.Fatal(err)
	}
	logContent, _ = os.ReadFile(filepath.Join(dir, "log.md"))
	if !strings.Contains(string(logContent), "Edited concept") {
		t.Error("log.md missing auto-generated description")
	}
	_ = c
}

func TestAppendLog(t *testing.T) {
	dir := t.TempDir()

	// First entry creates log.md.
	err := AppendLog(dir, LogEntry{Action: "Creation", Description: "Initial concept."})
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "log.md"))
	text := string(content)
	if !strings.Contains(text, "# Directory Update Log") {
		t.Error("missing header")
	}
	if !strings.Contains(text, "**Creation**: Initial concept.") {
		t.Error("missing entry")
	}

	// Second entry appends under same date.
	err = AppendLog(dir, LogEntry{Action: "Update", Description: "Updated schema."})
	if err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(filepath.Join(dir, "log.md"))
	text = string(content)
	if strings.Count(text, "**Creation**") != 1 {
		t.Error("Creation entry should appear once")
	}
	if !strings.Contains(text, "**Update**") {
		t.Error("missing Update entry")
	}
}

// ============================================================================
// Search tests
// ============================================================================

func TestSearch(t *testing.T) {
	dir := makeTestBundle(t)
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Search title.
	results := b.Search("Concept One", nil)
	if len(results) == 0 {
		t.Error("no results for title search 'Concept One'")
	}

	// Search body.
	results = b.Search("Body of concept", nil)
	if len(results) == 0 {
		t.Error("no results for body search")
	}

	// Search description.
	results = b.Search("Second test concept", nil)
	if len(results) == 0 {
		t.Error("no results for description search")
	}

	// Filter by type.
	results = b.Search("concept", []string{"Test Type"})
	allTestType := true
	for _, r := range results {
		if r.Concept.Type != "Test Type" {
			allTestType = false
		}
	}
	if !allTestType {
		t.Error("type filter not applied")
	}

	// No match.
	results = b.Search("zzznonexistentzzz", nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestHasAnyTag(t *testing.T) {
	if !hasAnyTag([]string{"a", "b"}, []string{"a"}) {
		t.Error("should match")
	}
	if hasAnyTag([]string{"a", "b"}, []string{"c"}) {
		t.Error("should not match")
	}
	if hasAnyTag([]string{}, []string{"a"}) {
		t.Error("empty tags should not match")
	}
}

func TestContainsType(t *testing.T) {
	if !containsType([]string{"a", "b"}, "a") {
		t.Error("should contain")
	}
	if containsType([]string{"a", "b"}, "c") {
		t.Error("should not contain")
	}
}

func TestTruncateString(t *testing.T) {
	s := TruncateString("short", 100)
	if s != "short" {
		t.Errorf("short string: %q", s)
	}

	s = TruncateString("this is a long string that should be truncated", 20)
	if len(s) > 20 {
		t.Errorf("truncated length %d > 20: %q", len(s), s)
	}
	if !strings.HasSuffix(s, "...") {
		t.Errorf("truncated string should end with '...': %q", s)
	}
}

// ============================================================================
// Validation tests
// ============================================================================

func TestValidate(t *testing.T) {
	dir := makeTestBundle(t)
	result, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Errorf("valid bundle marked invalid: errors=%v", result.Errors)
	}

	// Add a broken concept.
	os.WriteFile(filepath.Join(dir, "broken.md"), []byte("no frontmatter"), 0o644)
	result, err = Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Error("bundle with broken concept should be invalid")
	}
}

func TestValidateIndexFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "concept.md"), []byte("---\ntype: Test\n---\n"), 0o644)

	// Valid index.
	os.WriteFile(filepath.Join(dir, "index.md"), []byte("# Section\n* [Concept](concept.md) - A concept.\n"), 0o644)
	result := &ValidationResult{Valid: true}
	validateIndexFile(dir, "index.md", result)
	if !result.Valid {
		t.Error("valid index.md flagged")
	}

	// Index with frontmatter (warning).
	os.WriteFile(filepath.Join(dir, "index.md"), []byte("---\ntype: bad\n---\n# Section\n* [C](c.md) - desc\n"), 0o644)
	result = &ValidationResult{Valid: true}
	validateIndexFile(dir, "index.md", result)
}

func TestValidateLogFile(t *testing.T) {
	dir := t.TempDir()

	// Valid log.
	os.WriteFile(filepath.Join(dir, "log.md"), []byte("# Directory Update Log\n\n## 2026-06-22\n* **Update**: Something.\n"), 0o644)
	result := &ValidationResult{Valid: true}
	validateLogFile(dir, "log.md", result)

	// Bad date format.
	os.WriteFile(filepath.Join(dir, "log.md"), []byte("# Directory Update Log\n\n## not-a-date\n* entry\n"), 0o644)
	result = &ValidationResult{Valid: true}
	validateLogFile(dir, "log.md", result)
	if len(result.Warnings) == 0 {
		t.Error("expected warning for bad date format")
	}
}

func TestResolveLinkTarget(t *testing.T) {
	// External URL.
	if resolveLinkTarget("https://example.com", "source") != "" {
		t.Error("external URL should return empty")
	}
	if resolveLinkTarget("http://example.com", "source") != "" {
		t.Error("http URL should return empty")
	}

	// Fragment only.
	if resolveLinkTarget("#schema", "source") != "" {
		t.Error("fragment-only should return empty (same concept)")
	}

	// Absolute.
	if got := resolveLinkTarget("/tables/orders.md", "source"); got != "tables/orders" {
		t.Errorf("absolute link = %q, want 'tables/orders'", got)
	}
	if got := resolveLinkTarget("/tables/orders", "source"); got != "tables/orders" {
		t.Errorf("absolute link no ext = %q, want 'tables/orders'", got)
	}

	// Relative.
	if got := resolveLinkTarget("other.md", "dir/source"); got != "dir/other" {
		t.Errorf("relative link = %q, want 'dir/other'", got)
	}
	if got := resolveLinkTarget("../sibling.md", "dir/sub/source"); got != "dir/sibling" {
		t.Errorf("parent relative = %q, want 'dir/sibling'", got)
	}

	// With fragment — .md stripped first, then #block removed.
	if got := resolveLinkTarget("target.md#block", "source"); got != "target" {
		t.Errorf("with fragment = %q, want 'target'", got)
	}

	// Fragment-only in same concept.
	if got := resolveLinkTarget("concept.md#section", "concept"); got != "concept" {
		t.Errorf("same-concept fragment = %q, want 'concept'", got)
	}
}

func TestCheckCrossLinks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "source.md"), []byte("---\ntype: Test\ntitle: Source\n---\n[valid](target.md) [broken](missing.md) [external](https://example.com)\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "target.md"), []byte("---\ntype: Test\ntitle: Target\n---\nBody\n"), 0o644)

	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	result := &ValidationResult{Valid: true}
	checkCrossLinks(dir, b, result)

	brokenFound := false
	for _, w := range result.Warnings {
		if strings.Contains(w.Message, "missing") {
			brokenFound = true
		}
	}
	if !brokenFound {
		t.Error("expected warning for broken link to 'missing'")
	}
}

func TestCheckMissingIndexes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "concept.md"), []byte("---\ntype: T\n---\n"), 0o644)

	result := &ValidationResult{Valid: true}
	checkMissingIndexes(dir, result)
	if len(result.Info) == 0 {
		t.Error("expected info about missing index.md")
	}
}

// ============================================================================
// Generate index tests
// ============================================================================

func TestGenerateIndex(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("---\ntype: TypeA\ntitle: Alpha\ndescription: First.\n---\nBody\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "beta.md"), []byte("---\ntype: TypeB\ntitle: Beta\ndescription: Second.\n---\nBody\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "gamma.md"), []byte("---\ntype: TypeA\ntitle: Gamma\n---\nBody\n"), 0o644)

	written, err := GenerateIndex(dir, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) < 1 {
		t.Fatal("expected at least 1 index.md written")
	}

	// Read back and verify.
	content, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "TypeA") || !strings.Contains(text, "TypeB") {
		t.Errorf("index content missing type groups: %s", text)
	}
	if !strings.Contains(text, "[Alpha](alpha.md)") {
		t.Error("index missing Alpha entry")
	}
	if !strings.Contains(text, "[Beta](beta.md)") {
		t.Error("index missing Beta entry")
	}
	if !strings.Contains(text, "- First") {
		t.Error("index missing description for Alpha")
	}
	if !strings.Contains(text, "[Gamma](gamma.md)") {
		t.Error("index missing Gamma entry")
	}

	// Overwrite=false should not regenerate.
	written2, err := GenerateIndex(dir, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(written2) != 0 {
		t.Errorf("expected no overwrites, got %v", written2)
	}

	// Scoped to subdir (empty — no index generated).
	written3, err := GenerateIndex(dir, true, "subdir")
	if err != nil {
		t.Fatal(err)
	}
	if len(written3) != 0 {
		t.Errorf("empty subdir should not generate index, got %v", written3)
	}
}

func TestBuildIndexContent(t *testing.T) {
	entries := []dirEntry{
		{Title: "Sub", URL: "sub/", IsDirectory: true},
		{Title: "Alpha", URL: "alpha.md", Description: "First concept.", Type: "TypeA"},
		{Title: "Beta", URL: "beta.md", Type: "TypeA"},
	}
	content := buildIndexContent(entries)
	if !strings.Contains(content, "[Sub](sub/)") {
		t.Error("missing Sub entry")
	}
	if !strings.Contains(content, "[Alpha](alpha.md) - First concept.") {
		t.Error("missing Alpha with description")
	}
	if !strings.Contains(content, "[Beta](beta.md)") {
		t.Error("missing Beta")
	}
}

// ============================================================================
// Split frontmatter tests
// ============================================================================

func TestSplitFrontmatter(t *testing.T) {
	fm, body, err := splitFrontmatter("---\ntype: T\n---\nBody content.")
	if err != nil {
		t.Fatal(err)
	}
	if fm != "type: T" {
		t.Errorf("frontmatter = %q", fm)
	}
	if body != "Body content." {
		t.Errorf("body = %q", body)
	}

	_, _, err = splitFrontmatter("no frontmatter")
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

// ============================================================================
// FindConcepts tests
// ============================================================================

func TestFindConcepts(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "tables"), 0o755)

	// Create concepts with headings.
	os.WriteFile(filepath.Join(dir, "tables", "alpha.md"), []byte("---\ntype: TestType\ntitle: Alpha\n---\n# Schema\nSchema for alpha.\n# Examples\nExample usage.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "tables", "beta.md"), []byte("---\ntype: TestType\ntitle: Beta\n---\n# Joins\nJoin logic here.\n"), 0o644)

	// Create index.md.
	os.WriteFile(filepath.Join(dir, "tables", "index.md"), []byte("# TestType\n* [Alpha](alpha.md) - First concept.\n* [Beta](beta.md) - Second concept with joins.\n"), 0o644)

	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Search by index entry.
	result := b.FindConcepts("Second concept")
	if len(result.Results) == 0 {
		t.Error("expected results for index search 'Second concept'")
	} else {
		found := false
		for _, r := range result.Results {
			if r.ConceptID == "tables/beta" {
				found = true
				if len(r.IndexMatches) == 0 {
					t.Error("expected IndexMatches for Beta")
				}
			}
		}
		if !found {
			t.Error("Beta not found in results")
		}
	}

	// Search by heading.
	result = b.FindConcepts("Schema")
	if len(result.Results) == 0 {
		t.Error("expected results for heading search 'Schema'")
	}

	// No match.
	result = b.FindConcepts("zzznonexistentzzz")
	if len(result.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(result.Results))
	}
}

func TestParseIndexFile(t *testing.T) {
	content := "# Section\n* [Concept One](one.md) - Description one.\n* [Concept Two](two.md) - Description two.\n\n# Another Section\n* [Subdir](sub/) - A subdirectory.\n"
	entries := parseIndexFile(content)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Section != "Section" || entries[0].Title != "Concept One" {
		t.Errorf("entry 0: section=%q title=%q", entries[0].Section, entries[0].Title)
	}
	if entries[2].Section != "Another Section" || entries[2].Title != "Subdir" {
		t.Errorf("entry 2: section=%q title=%q", entries[2].Section, entries[2].Title)
	}
}

func TestExtractHeadingMatches(t *testing.T) {
	body := "# Schema\nSchema content here.\n\n# Examples\nExample one.\nExample two.\n\n## Nested\nNested content.\n"
	matches := extractHeadingMatches(body, "schema")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Heading != "Schema" {
		t.Errorf("heading = %q", matches[0].Heading)
	}

	matches = extractHeadingMatches(body, "nonexistent")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestDeriveConceptID(t *testing.T) {
	// Relative within same dir.
	if got := deriveConceptID("badges.md", "tables"); got != "tables/badges" {
		t.Errorf("relative = %q, want 'tables/badges'", got)
	}

	// Absolute.
	if got := deriveConceptID("/tables/badges.md", "anything"); got != "tables/badges" {
		t.Errorf("absolute = %q, want 'tables/badges'", got)
	}

	// Root.
	if got := deriveConceptID("concept.md", "."); got != "concept" {
		t.Errorf("root = %q, want 'concept'", got)
	}

	// Parent dir reference.
	if got := deriveConceptID("../other.md", "tables/sub"); got != "tables/other" {
		t.Errorf("parent = %q, want 'tables/other'", got)
	}
}

// ============================================================================
// Error path tests
// ============================================================================

func TestValidateErrors(t *testing.T) {
	// Non-existent path.
	_, err := Validate("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}

	// Non-directory.
	f := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(f, []byte("text"), 0o644)
	_, err = Validate(f)
	if err == nil {
		t.Error("expected error for non-directory")
	}
}

func TestGenerateIndexErrors(t *testing.T) {
	// Non-existent path.
	_, err := GenerateIndex("/nonexistent", true, "")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}

	// Bad scoped directory.
	dir := t.TempDir()
	_, err = GenerateIndex(dir, true, "nonexistent-subdir")
	if err == nil {
		t.Error("expected error for nonexistent subdir")
	}
}

func TestWriteConceptErrors(t *testing.T) {
	// Read-only directory (write should fail).
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o555) // read-only subdir
	c := &Concept{ID: "sub/nope", Type: "T", Body: "x"}
	err := WriteConcept(dir, c)
	if err == nil {
		t.Error("expected error writing to read-only directory")
	}
}

func TestAppendLogErrors(t *testing.T) {
	// Write to read-only directory.
	dir := t.TempDir()
	os.Chmod(dir, 0o555)
	err := AppendLog(dir, LogEntry{Action: "Test"})
	if err == nil {
		t.Error("expected error writing log to read-only dir")
	}
}

func TestMarshalConcept(t *testing.T) {
	c := &Concept{
		ID:          "test",
		Type:        "TestType",
		Title:       "TT",
		Description: "A desc.",
		Resource:    "res://x",
		Tags:        []string{"a", "b"},
		Body:        "body text",
	}
	content := c.Marshal()
	parsed, err := ParseConcept("test", content)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Type != c.Type {
		t.Errorf("type = %q", parsed.Type)
	}
	if parsed.Title != c.Title {
		t.Errorf("title = %q", parsed.Title)
	}
	if len(parsed.Tags) != len(c.Tags) {
		t.Errorf("tags = %v", parsed.Tags)
	}
	if parsed.Body != c.Body {
		t.Errorf("body = %q", parsed.Body)
	}
}
