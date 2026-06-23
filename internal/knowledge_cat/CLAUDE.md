# internal/knowledge_cat — Core OKF Domain Package

## Responsibility
Leaf package — the entire business logic for Open Knowledge Format bundles.
Reads, parses, queries, mutates, and validates OKF directory trees. Exports all
domain types (`Concept`, `Bundle`, `Block`, search/validation result types) and
operations consumed by both `cmd/know` (CLI) and `internal/mcp` (MCP server).

## Dependencies
- **`gopkg.in/yaml.v3`** — YAML frontmatter parsing with custom `UnmarshalYAML` hooks
- Standard library only otherwise (`os`, `path/filepath`, `strings`, `regexp`, `time`, `sort`)

## Consumers
- **`main.go`** — All Cobra commands call into package functions and `*Bundle` methods
- **`internal/mcp/server.go`** — MCP tool handlers delegate to the same API surface

## Module Structure
| File(s) | Role |
|---------|------|
| `knowledge_cat.go` | Domain types: `Concept`, `Bundle`, `ListFilter`, `ResolveBundlePath()` |
| `bundle.go` | Constructor `Open()`, queries `List()`, `GetConcept()`, `ListTypes()` |
| `concept.go` | Parsing, serialization, and mutation: `ParseConcept()`, `Marshal()`, `EditConcept()`, custom YAML unmarshalers for `tags` and `timestamp` |
| `block.go` | Block addressing: `ParseBlocks()`, `GetBlock()`, `ParseConceptRef()` — `conceptID#blockID` |
| `search.go`, `index_search.go` | Search: full-text grep (`Search()`) + structured index/heading search (`FindConcepts()`) |
| `validate.go` | Spec conformance: `Validate()` — Errors/Warnings/Info triad, cross-link checking |
| `log.go`, `generate_index.go` | Supporting ops: `appendLog()`, `GenerateIndex()` |

## Lenient YAML Parsing (Custom `UnmarshalYAML` Types)
Tags can be a YAML list or comma-separated string; timestamps use multiple ISO formats.
Private deserialization struct isolates YAML shape from the public `Concept` type.

```go
// Private struct mirrors YAML fields with custom types for lenient fields.
type frontmatterFields struct {
    Type      string  `yaml:"type"`
    Tags      tagList `yaml:"tags"`
    Timestamp tagTime `yaml:"timestamp"`
}

// tagList handles both YAML sequences and "tag1, tag2" scalar strings.
type tagList []string
func (t *tagList) UnmarshalYAML(value *yaml.Node) error {
    switch value.Kind {
    case yaml.SequenceNode:
        var s []string
        if err := value.Decode(&s); err != nil { return err }
        *t = s
    case yaml.ScalarNode:
        for _, p := range strings.Split(value.Value, ",") {
            if p = strings.TrimSpace(p); p != "" { *t = append(*t, p) }
        }
    }
    return nil
}
```

## Block-Level Addressing (`conceptID#blockID`)
Headings delimit addressable sections. `ParseConceptRef("tables/badges#schema")` →
`("tables/badges", "schema")`. Preamble before first heading gets block ID `"top"`.
Block IDs are slugified: `"Common patterns"` → `"common-patterns"`.

```go
blocks := knowledge_cat.ParseBlocks(c.Body)                       // split at #{1,6} headings
block  := knowledge_cat.GetBlock(c.Body, "schema")                // or nil
conceptID, blockID := knowledge_cat.ParseConceptRef("path/to/concept#block")
```

## Edit → Write → Log Workflow
Edits re-open the bundle from disk (no stale pointer). Text replace is
first-occurrence-only. Timestamp auto-updated. Log append is best-effort
(edit is already saved if logging fails).

```go
c, err := knowledge_cat.EditConcept(bundlePath, conceptID, oldText, newText, description)
// Re-opens bundle → finds concept → strings.Replace(old, new, 1) → WriteConcept → AppendLog
```

## Phased Search → Merge-by-Map
`FindConcepts()` searches index.md entries and concept headings in two independent
phases, merging results into a shared `map[string]*FindResult`. Output is sorted
by concept ID.

```go
resultMap := make(map[string]*FindResult)
b.searchIndexFiles(query, resultMap)      // Phase 1: walk index.md files
b.searchConceptHeadings(query, resultMap) // Phase 2: scan concept headings
// Convert map → sorted slice
```

## Architectural Boundaries
- **NO internal imports** from `know` — leaf package, only stdlib + `yaml.v3`
- **NO concurrent safety** — `Bundle` is an immutable snapshot; edits re-open from disk
- Lenient `Open()` skips malformed concepts with stderr warnings; use `Validate()` for strict checks
- Reserved filenames (`index.md`, `log.md`) are never parsed as concepts

<important if="you are adding a new OKF operation to this package">
## Adding a New OKF Operation
1. **Choose the shape**: method on `*Bundle` (operates on loaded concepts) or standalone function (filesystem operation like `Validate`/`GenerateIndex`)
2. **Define result types** if returning structured data — follow `Result`/`Output`/`Match` suffix convention
3. **Normalize inputs early** — lowercase search queries, `filepath.Abs` paths, trim strings
4. **Wrap errors**: `fmt.Errorf("operation: %w", err)` with consistent prefix
5. **Follow naming**: `VerbNoun` for exported; lowercase `verbNoun` for helpers; pre-compile regexes at package level as `var`
6. **Add to `main.go`** as a new `cobra.Command` (see root CLAUDE.md) and/or `internal/mcp/server.go` as a new `mcp.ToolHandlerFor[I,O]`
</important>

<important if="you are adding a new validation check">
## Adding a New Validation Check
1. **Choose tier**: `result.Errors` (spec violation), `result.Warnings` (recommendation), or `result.Info` (advisory)
2. **Write a `checkXxx` function** taking `(bundleRoot, bundle, result)` and appending `ValidationIssue{File, Line, Message}` — keep checks independent
3. **Call it from `Validate()`** after the main walk loop
4. **Set `result.Valid = false`** when appending to Errors
</important>
