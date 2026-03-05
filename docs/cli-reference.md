# C4 CLI Reference

## Synopsis

```
c4 [options] [path...]           # Generate C4 IDs or capsules
c4 fmt [options] <file.c4m>      # Format capsule (canonical or ergonomic)
c4 diff <source> <target>        # Compare capsules or paths
c4 union <inputs...>             # Combine capsules
c4 intersect <inputs...>         # Find common elements
c4 subtract <from> <remove>      # Set subtraction
c4 validate <file|bundle>        # Validate capsule or bundle
c4 extract <bundle> [output]     # Extract bundle to single capsule
c4 mk <name>.c4m:                # Establish capsule endpoint
c4 mk <name>: <host:port>        # Establish location endpoint
c4 rm <name>:                    # Remove endpoint
c4 mkdir <name>.c4m:<path>/      # Create directory in capsule
c4 cp [-r] <src> <dst>           # Copy content between endpoints
```

## Scanning and Identification

```bash
# Identify a single file
c4 photo.jpg

# Identify from stdin
echo "hello" | c4

# Identify a directory (single C4 ID)
c4 my-project/

# One-level capsule
c4 -m my-project/

# Full recursive capsule
c4 -mr my-project/

# Save capsule to file
c4 -mr my-project/ > project.c4m

# Pretty-print with aligned columns
c4 -m --pretty my-project/

# Progressive scan (interruptible)
c4 --progressive --bundle large-dir/

# Resume interrupted scan
c4 --bundle --resume large-dir.c4m_bundle
```

## Options

| Flag | Long | Description |
|------|------|-------------|
| `-a` | `--absolute` | Use absolute paths |
| | `--bundle` | Create/use bundle for unbounded scans |
| `-d` | `--depth N` | Max depth for recursive processing (default: unlimited) |
| | `--empty` | Exit 0 if empty, 1 if content |
| `-L` | `--follow` | Follow symbolic links |
| | `--format` | Output format: `c4m`, `paths`, `json` |
| `-m` | `--manifest` | Output capsule format |
| `-n` | `--no-ids` | Skip C4 ID computation (faster) |
| `-p` | `--paths` | Output paths only |
| | `--pretty` | Pretty-print with aligned columns and formatted sizes |
| | `--progressive` | Progressive scan with interrupt support |
| `-q` | `--quiet` | Quiet mode |
| `-r` | `--recursive` | Process recursively |
| | `--resume` | Resume incomplete bundle scan |
| `-v` | `--verbose` | Verbose output |
| | `--version` | Show version |

## Set Operations

Compare two capsules:

```bash
c4 diff old.c4m new.c4m
```

Find files present in both:

```bash
c4 intersect a.c4m b.c4m
```

Find files in `needed.c4m` but not in local directory:

```bash
c4 subtract needed.c4m ./local/ > missing.c4m
```

Merge capsules:

```bash
c4 union part1.c4m part2.c4m > combined.c4m
```

## Capsule Endpoints

Establish a capsule for writing:

```bash
c4 mk project.c4m:
```

Establish a location backed by c4d:

```bash
c4 mk store: localhost:17433
```

Create a directory inside a capsule:

```bash
c4 mkdir project.c4m:src/
```

Copy content into a capsule:

```bash
c4 cp -r ./src/ project.c4m:src/
```

Copy content from a capsule to disk:

```bash
c4 cp project.c4m:src/ ./restored/
```

Remove an endpoint:

```bash
c4 rm project.c4m:
```

## Colon Syntax

The colon (`:`) separates an endpoint name from a subpath within it:

```
<endpoint>:<subpath>
```

- `project.c4m:` — the capsule root
- `project.c4m:src/main.go` — a file inside the capsule
- `store:assets/` — a directory in a location

Trailing colon means "look inside" — it's the boundary between the endpoint and its contents.

## Formatting

Reformat a capsule to canonical form:

```bash
c4 fmt capsule.c4m
```

Pretty-print an existing capsule:

```bash
c4 fmt --pretty capsule.c4m
```
