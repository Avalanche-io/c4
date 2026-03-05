# C4M Language Server Design

## Overview

A Language Server Protocol (LSP) implementation for c4m files, enabling rich editor support across VS Code, Neovim, Emacs, and any LSP-compatible editor. The server parses c4m's indentation-based hierarchy to provide navigation, diagnostics, hover info, and intelligent completions.

## Starting Point

The existing `vscode-c4m` extension provides:
- TextMate grammar for syntax highlighting
- Folding ranges based on directory indentation
- Smart paste with auto-indentation
- Folding analysis diagnostic command

These features are implemented directly in VS Code's extension API (TypeScript). The language server would replace the programmatic features (folding, diagnostics) with LSP equivalents, while the TextMate grammar continues to handle syntax highlighting.

## Architecture Decision: Go vs TypeScript

### Option A: Go Language Server (Recommended)

**Rationale:**
- The canonical c4m parser exists in Go (`c4m` package) — reuse real parsing logic
- Go LSP servers are proven (gopls serves the entire Go ecosystem)
- Single binary deployment — no Node.js dependency
- Natural fit for the C4 ecosystem (c4, c4d, c4-mcp all in Go)
- Can link directly to `c4.ID` for validation and computation
- Better performance for large c4m files (45MB+, 150K+ lines)

**Libraries:**
- `golang.org/x/tools/gopls/internal/protocol` — LSP protocol types (or generate from spec)
- `github.com/tliron/glsp` — lightweight Go LSP framework
- Alternative: raw `jsonrpc2` from `golang.org/x/tools/internal/jsonrpc2`

**Binary:** `c4m-lsp` — standalone, ships alongside `c4` and `c4-mcp`

### Option B: TypeScript Language Server

**Rationale:**
- VS Code has first-class TypeScript LSP support via `vscode-languageserver`
- Existing extension is TypeScript — no language boundary
- `vscode-languageclient` handles VS Code integration automatically

**Drawbacks:**
- Would need to reimplement c4m parsing in TypeScript (no existing parser)
- Two parsers to maintain (Go canonical + TypeScript LSP)
- Node.js dependency for non-VS-Code editors
- Slower for large files than Go

### Recommendation: Go

A Go language server reuses the battle-tested `c4m` package parser, requires zero reimplementation of format knowledge, and runs as a single binary. The vscode-c4m extension becomes a thin client that launches the Go server.

## C4M Format Properties That Affect LSP Design

### Indentation-Based Hierarchy
- 2-space indent per depth level (auto-detected by parser)
- Directory entries (`d` mode or trailing `/`) define scope
- Children are indented under parent directories
- This is the primary structure the LSP must model

### Entry Structure
```
[indent]MODE TIMESTAMP SIZE NAME [-> TARGET] [C4ID]
```
- MODE: 10-char Unix permissions or `-` (null)
- TIMESTAMP: RFC3339 UTC or ergonomic local time
- SIZE: integer bytes, comma-separated display, or `-` (null)
- NAME: unquoted, quoted (`"..."`), or sequence pattern (`frame.[0001-0100].exr`)
- C4ID: `c4` + 88 base62 chars, or absent

### Directives
- `@c4m 1.0` — header (required)
- `@layer`, `@remove`, `@end` — changesets
- `@base C4ID` — base manifest reference
- `@data C4ID` — embedded data block
- `@intent` — intent manifest marker
- `@by`, `@time`, `@note` — layer metadata

### Comments
- `#` at start of line (after optional whitespace)

### Scale
- Files can exceed 150K lines and 45MB
- VS Code disables features above 20MB by default
- Incremental parsing is critical for usability

## LSP Capabilities

### Phase 1: Core Navigation (Priority)

#### Document Symbols (`textDocument/documentSymbol`)
Every directory becomes a `DocumentSymbol` with kind `Namespace` or `Module`. Files become children with kind `File`. This powers:
- **VS Code Outline view** — collapsible directory tree in sidebar
- **Breadcrumb navigation** — path hierarchy in breadcrumb bar
- **Go to Symbol** (`Cmd+Shift+O`) — type to jump to any directory or file

```
Symbol tree example:
  project/          (Namespace)
    README.md       (File)
    src/            (Namespace)
      main.go       (File)
      utils/        (Namespace)
        helper.go   (File)
```

#### Folding Ranges (`textDocument/foldingRange`)
Replaces the current TypeScript folding provider. Directory entries fold to hide their children. Also fold:
- `@layer`...`@end` blocks
- `@data` blocks
- Comment blocks (consecutive `#` lines)

#### Hover (`textDocument/hover`)
Show structured info on hover over any entry:
```
src/main.go
  Permissions: -rw-r--r-- (644)
  Modified: 2025-01-24 13:29:55 UTC
  Size: 14,754 bytes (14.4 KB)
  Depth: 2
  C4 ID: c452SQNpb9hVSiR6...
  Full path: project/src/main.go
```

For directories, also show entry count and total size of children.

For C4 IDs specifically, show the full ID (they're long) and duplicate count if the same ID appears elsewhere in the file (identical content).

### Phase 2: Diagnostics and Validation

#### Diagnostics (`textDocument/publishDiagnostics`)
Real-time validation using the `c4m` package's validator:

**Errors (red squiggles):**
- Invalid mode string (not 10 chars, invalid type character)
- Malformed timestamp (not parseable by any supported format)
- Malformed C4 ID (wrong prefix, wrong length, invalid base62)
- Unterminated quoted name
- Missing `@c4m` header

**Warnings (yellow squiggles):**
- Odd indentation (not a multiple of indent width)
- File indented deeper than any parent directory
- Duplicate paths at the same directory level
- Directory with `d` mode but missing trailing `/`
- Null metadata fields (`-` for mode, timestamp, or size)

**Info (blue):**
- Non-canonical timestamp format (local timezone)
- Display-format sizes (commas)
- Sequences detected

#### Code Actions (`textDocument/codeAction`)
Quick fixes for common issues:
- Convert non-canonical timestamp to UTC
- Fix indentation to nearest valid level
- Add missing trailing `/` to directory entries
- Remove duplicate entries

### Phase 3: Intelligent Editing

#### Completion (`textDocument/completion`)
- **Path completion within the document:** When typing a name that matches an existing path prefix, suggest completions from the file's own directory tree
- **C4 ID completion:** After typing `c4`, suggest C4 IDs that appear elsewhere in the document (for deduplication)
- **Directive completion:** After typing `@`, suggest valid directives

#### Go-to-Definition (`textDocument/definition`)
- Clicking a directory name jumps to where that directory's entries begin
- Clicking a C4 ID jumps to the first occurrence of that ID in the document
- Clicking a `@base` C4 ID could open the referenced manifest (if resolvable)

#### Find References (`textDocument/references`)
- On a C4 ID: find all entries with the same ID (files with identical content)
- On a directory name: find all entries within that directory

#### Rename (`textDocument/rename`)
- Rename a directory: updates all child indentation context (the directory name itself, not children names)

### Phase 4: Advanced Features

#### Workspace Symbols (`workspace/symbol`)
Search across all `.c4m` files in the workspace for paths or C4 IDs.

#### Semantic Tokens (`textDocument/semanticTokens`)
Richer highlighting than TextMate grammar:
- Color directories by depth
- Dim null values (`-`)
- Highlight duplicate C4 IDs
- Distinguish canonical vs ergonomic format fields

#### Format Conversion
Custom commands (not standard LSP, exposed via `workspace/executeCommand`):
- `c4m.convertToCanonical` — convert timestamps, sizes to canonical form
- `c4m.convertToErgonomic` — convert to human-readable local time, comma sizes
- `c4m.sortEntries` — sort entries within each directory level

#### Document Links (`textDocument/documentLink`)
Make C4 IDs clickable — open in browser (if a web viewer exists) or copy to clipboard.

## Internal Architecture

### Document Model

```go
// Document represents a parsed c4m file with position tracking
type Document struct {
    URI     string
    Version int
    Lines   []string       // raw line content
    Tree    *DirectoryTree // parsed hierarchy with spans
    Diags   []Diagnostic   // current diagnostics
}

// DirectoryTree maps the indentation hierarchy
type DirectoryTree struct {
    Root     *Node
    ByLine   map[int]*Node   // line number -> node
    ByPath   map[string]*Node // full path -> node
    ByC4ID   map[string][]*Node // c4id -> nodes (for duplicates)
}

// Node is a file or directory entry with source positions
type Node struct {
    Entry    *c4m.Entry
    Line     int          // 0-based line number
    Span     Range        // full line range
    Children []*Node
    Parent   *Node
    FullPath string       // computed path from root
}
```

### Incremental Parsing Strategy

For large files, full reparse on every keystroke is unacceptable. Strategy:

1. **On open:** Full parse, build document model + tree
2. **On edit:** Determine affected line range from `textDocument/didChange`
   - If edit is within a single line: reparse that line, update its node
   - If edit adds/removes lines: reparse from the edited line to the next line at the same or lesser indent depth
   - Rebuild tree indices for the affected region only
3. **Debounce:** Diagnostics run on a 300ms debounce after last edit

The `c4m.Decoder` is line-based and stateless per-entry (context comes from indentation only), making partial reparse straightforward — each line can be parsed independently given the indent width.

### Performance Targets

| File Size | Open Time | Edit Response | Symbol Query |
|-----------|-----------|---------------|-------------|
| < 1K lines | < 50ms | < 10ms | < 5ms |
| 10K lines | < 200ms | < 20ms | < 10ms |
| 150K lines | < 2s | < 50ms | < 50ms |

## VS Code Extension Changes

The `vscode-c4m` extension becomes a language client:

```json
{
  "activationEvents": ["onLanguage:c4m"],
  "main": "./out/extension.js",
  "contributes": {
    "languages": [{ "id": "c4m", "extensions": [".c4m"] }],
    "grammars": [{ "language": "c4m", "scopeName": "source.c4m", "path": "./syntaxes/c4m.tmLanguage.json" }],
    "configuration": {
      "properties": {
        "c4m.languageServer.path": {
          "type": "string",
          "description": "Path to c4m-lsp binary"
        }
      }
    }
  }
}
```

Extension TypeScript reduces to:
1. Find or download `c4m-lsp` binary
2. Start language client pointing at the binary
3. Register any custom commands that invoke `workspace/executeCommand`
4. Keep TextMate grammar for syntax highlighting (LSP semantic tokens supplement, not replace)

All current programmatic features (folding provider, smart paste, analyze command) move to the LSP server.

## Binary Distribution

`c4m-lsp` is built with the same goreleaser pipeline as `c4` and `c4-mcp`:
- Cross-compiled for darwin/linux/windows on amd64/arm64
- Installed via `go install`, Homebrew, or direct download
- VS Code extension can auto-download the correct binary on first activation

## Feature Priority vs Effort

| Feature | LSP Capability | Priority | Effort |
|---------|---------------|----------|--------|
| Document symbols / outline | `documentSymbol` | P0 | Low |
| Breadcrumb navigation | `documentSymbol` | P0 | Free (from symbols) |
| Folding ranges | `foldingRange` | P0 | Low |
| Hover info | `hover` | P0 | Low |
| Go-to-path | `definition` | P1 | Low |
| Path completion | `completion` | P1 | Medium |
| Diagnostics | `publishDiagnostics` | P1 | Medium |
| Find duplicate C4 IDs | `references` | P2 | Low |
| Quick fixes | `codeAction` | P2 | Medium |
| Format conversion | `executeCommand` | P2 | Medium |
| Semantic tokens | `semanticTokens` | P3 | Medium |
| Workspace symbols | `workspace/symbol` | P3 | Medium |

## Go Module Structure

```
cmd/c4m-lsp/
    main.go          // LSP server entry point, stdio transport
internal/lsp/
    server.go        // LSP method dispatch
    document.go      // Document model + incremental parsing
    symbols.go       // documentSymbol provider
    folding.go       // foldingRange provider
    hover.go         // hover provider
    diagnostics.go   // diagnostic engine
    completion.go    // completion provider
    definition.go    // go-to-definition
    references.go    // find references
```

The `internal/lsp` package imports `c4m` for parsing and `c4` for ID validation. No circular dependencies.

## Open Questions

1. **Smart paste:** Should smart paste move to the LSP (`textDocument/onTypeFormatting`) or remain a VS Code command? The LSP approach is more portable but paste interception is tricky in LSP.

2. **Bundled binary:** Should the VS Code extension bundle the `c4m-lsp` binary, or require separate installation? Bundling is more user-friendly but increases extension size (~5MB per platform).

3. **Streaming for huge files:** For files > 50MB, should the LSP lazily parse (parse visible region + surroundings) rather than full upfront parse?

4. **Multi-file features:** Should the LSP track cross-file C4 ID references (workspace-wide deduplication view)?
