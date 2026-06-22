# know — Knowledge Cat(ologue) CLI + MCP Server

## Overview
Go CLI tool and MCP server for reading, searching, editing, and validating
[Open Knowledge Format](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md) bundles.
Built on **Cobra** (CLI) and the **MCP Go SDK** (modelcontextprotocol/go-sdk v1.6.1).

## Architecture
Single-module Go project with idiomatic `cmd/` + `internal/` layout. Two consumer
layers (CLI and MCP) share one domain package.

```
cmd/know/main.go          ── CLI entry (Cobra commands)
internal/mcp/server.go   ── MCP server (tools + resources)
         │         └──────────────────┐
         ▼                            ▼
     internal/knowledge_cat/   ←── domain logic (leaf package, no internal deps)
```

- **`internal/knowledge_cat/`** — Core domain (see its CLAUDE.md for patterns and workflows)
- **`cmd/know/main.go`** — All Cobra command definitions in one file. Each command calls `openBundle()` or `resolveBundlePath()` then delegates to `internal/knowledge_cat`
- **`internal/mcp/server.go`** — MCP server setup, tool registration, resource templates. Tool handlers are factory functions returning `mcp.ToolHandlerFor[Input, Output]`

## Commands
| Purpose | Command |
|---------|---------|
| Build | `go build -o know ./cmd/know` |
| Test | `go test ./...` |
| Run CLI | `go run ./cmd/know <command> -b /path/to/bundle` |
| Lint/format | `goimports`, `gofumpt` (Neovim gopls LSP) |

<important if="you are adding a new CLI command">
## Adding a New CLI Command
1. Define a `var myCmd = &cobra.Command{...}` with `RunE` calling `openBundle()` or `resolveBundlePath()`
2. If the command needs flags, declare package-level `var myFlag` and register in `init()` with `myCmd.Flags().StringVar(...)`
3. Register in `init()`: `rootCmd.AddCommand(myCmd)`
4. Commands that only read use `openBundle()`; commands that write or validate use `resolveBundlePath()` + direct library call
5. Keep error formatting in the CLI layer — return errors from `RunE`, cobra prints them to stderr
</important>

<important if="you are adding a new MCP tool">
## Adding a New MCP Tool
1. Define input struct with `json` + `jsonschema` tags (see existing tools in `server.go`)
2. Define output struct — **must be an object (not a slice)** — the go-sdk requires output schema type `"object"`, so wrap slices
3. Create factory function `func makeXxxHandler(b *okf.Bundle) mcp.ToolHandlerFor[Input, Output]` — captures the bundle in closure
4. Register in `registerTools()`: `mcp.AddTool(server, &mcp.Tool{Name: "know_xxx", ...}, makeXxxHandler(b))`
5. For user-facing errors, return `&mcp.CallToolResult{IsError: true, Content: [...]}` + zero-value output + `nil` Go error. Reserve Go errors for truly unexpected failures
6. No-input tools use `struct{}` as input type (see `makeListTypesHandler`)
</important>

<important if="you are writing tests in this project">
## Testing Conventions
- Tests live in the package they test (same-package `_test.go`)
- Integration test `TestOpenSampleBundle` requires a real OKF bundle on disk; uses `/tmp/knowledge-catalog/okf/bundles/stackoverflow` or `OKF_TEST_BUNDLE` env var
- `go test ./...` runs all tests (only `internal/knowledge_cat` has tests currently)
</important>
