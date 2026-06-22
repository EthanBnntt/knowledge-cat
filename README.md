# knowledge cat(ologue)

```
  /\_/\
 ( o.o )  ~meow~
  > ^ <
```

**`know`** — a CLI tool and [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server
for reading, searching, and managing [Open Knowledge Format (OKF)](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md) bundles.

OKF is an open specification (v0.1) for representing organizational knowledge as
directories of markdown files with YAML frontmatter — tables, metrics, APIs, playbooks, and more.

## Quick Start

```bash
# Clone a sample OKF bundle
git clone https://github.com/GoogleCloudPlatform/knowledge-catalog.git /tmp/kc

# List concepts
know list -b /tmp/kc/okf/bundles/stackoverflow

# Read a concept
know read tables/badges -b /tmp/kc/okf/bundles/stackoverflow

# Read just a block within a concept
know read tables/badges#schema -b /tmp/kc/okf/bundles/stackoverflow

# Full-text search
know grep "transaction" -b /tmp/kc/okf/bundles/crypto_bitcoin

# Find concepts by index entries and headings
know find "metric" -b /tmp/kc/okf/bundles/ga4

# Validate bundle conformance
know validate -b /tmp/kc/okf/bundles/stackoverflow

# Generate index.md files
know generate-index --overwrite -b /tmp/kc/okf/bundles/stackoverflow

# Start as an MCP server (for AI agents)
know serve -b /tmp/kc/okf/bundles/stackoverflow
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `know list` | List concepts, optionally filtered by type/tags |
| `know read <id>` | Read a concept or block (`concept#block-id`) |
| `know grep <query>` | Full-text search within concepts (returns block context) |
| `know find <query>` | Find concepts by index entries and headings |
| `know edit <id>` | Edit a concept's body (find-and-replace, auto-logged) |
| `know types` | List all concept types and tags in the bundle |
| `know validate` | Check bundle conformance with the OKF v0.1 spec |
| `know generate-index` | Generate/regenerate index.md files for directories |
| `know serve` | Start as an MCP server over stdio |

All commands accept `-b/--bundle <path>` (defaults to current directory).

## MCP Tools (for AI Agents)

When running as an MCP server (`know serve`), the following tools are exposed:

| Tool | Description |
|------|-------------|
| `know_list_concepts` | List concepts with optional type/tag filters |
| `know_read_concept` | Read a concept or block (`id#block-id`) |
| `know_edit_concept` | Edit a concept's body (auto-logged to log.md) |
| `know_grep` | Full-text search within concept bodies |
| `know_find_concepts` | Find concepts by index.md entries and headings |
| `know_list_types` | List all concept types and tags |
| `know_validate` | Validate bundle conformance |
| `know_generate_index` | Generate index.md files |

**Resources:** Concepts are also exposed as MCP resources under the `know://{+conceptID}` URI scheme.

### Configuring MCP in Claude

Add to your Claude Desktop or Code config:

```json
{
  "mcpServers": {
    "know": {
      "command": "know",
      "args": ["serve", "-b", "/path/to/your/bundle"]
    }
  }
}
```

Or set the `OKF_BUNDLE` environment variable:

```json
{
  "mcpServers": {
    "know": {
      "command": "know",
      "args": ["serve"],
      "env": { "OKF_BUNDLE": "/path/to/your/bundle" }
    }
  }
}
```

## Features

- **Block-level addressing** — `tables/badges#schema` reads just the Schema section
- **Lenient parsing** — Handles comma-separated `tags`, multiple ISO timestamp formats
- **Automatic logging** — Every `know edit` appends to the bundle's `log.md`
- **Cross-link validation** — `know validate` checks markdown links between concepts
- **Index generation** — Auto-generate `index.md` files grouped by concept type
- **Resource URIs** — Concepts are addressable as `know://tables/badges` MCP resources

## Requirements

- Go 1.26.4+
- Compatible with any OKF v0.1 bundle

## Tested Bundles

- Stack Overflow ([GoogleCloudPlatform/knowledge-catalog](https://github.com/GoogleCloudPlatform/knowledge-catalog))
- GA4 E-Commerce
- Crypto Bitcoin

## Building from Source

```bash
go build -o know ./cmd/know
# Optionally install to GOPATH
go install ./cmd/know
```

## License

MIT
