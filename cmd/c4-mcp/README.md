# c4-mcp

MCP (Model Context Protocol) server that exposes c4 operations as tools for AI assistants.

## Build

```bash
go build -o c4-mcp ./cmd/c4-mcp/
```

## Configure

Add to your Claude Code MCP settings (`~/.claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "c4": {
      "command": "/path/to/c4-mcp"
    }
  }
}
```

## Tools

| Tool | Description |
|------|-------------|
| `c4_id` | Compute the C4 ID of a file |
| `c4_scan` | Scan a directory, return c4m capsule data |
| `c4_ls` | List capsule contents (colon syntax supported) |
| `c4_diff` | Compare two capsules or directories |
| `c4_search` | Find files by glob pattern in a capsule |
| `c4_mk` | Establish a capsule for writing |
| `c4_mkdir` | Create a directory in a capsule |
| `c4_cp` | Copy between local filesystem and capsules |
| `c4_validate` | Validate a capsule for spec compliance |

## Protocol

JSON-RPC 2.0 over stdio, newline-delimited. Compatible with MCP 2024-11-05.
