# C4 CLI Reference

## Synopsis

```
c4 [path...]                     # Identify files/directories (output is c4m)
c4 ls <location>:                # List contents of a c4m file, location, or managed dir
c4 cat <location>:<path>         # Output file content bytes to stdout
c4 cp <src> <dst>                # Copy content between local, c4m, and remote locations
c4 mv <src> <dst>                # Move/rename entries
c4 ln <src> <dst>                # Create hard links or tags
c4 mkdir <location>:<path>/      # Create directory in a c4m file
c4 mk <name>.c4m:                # Establish a c4m file or location for writing
c4 mk <name>: <host:port>        # Establish remote location
c4 rm <location>:<path>          # Remove entries or endpoints
c4 diff <source> <target>        # Compare two sources (output is a c4m patch)
c4 patch <target> <input>        # Apply a c4m patch or converge to target state
c4 undo :                        # Revert last operation on managed directory
c4 redo :                        # Re-apply undone operation
c4 unrm :                        # List or recover removed items
c4 version                       # Show version and mesh nodes
```

## Scanning and Identification

```bash
# Identify a single file (outputs a c4m entry)
c4 photo.jpg

# Identify from stdin (bare C4 ID, no metadata)
echo "hello" | c4

# Identify a directory (full recursive c4m listing)
c4 my-project/

# Just the C4 ID
c4 -i my-project/

# Pretty-print with aligned columns
c4 -p my-project/

# Save c4m to file
c4 my-project/ > project.c4m
```

## Global Flags

| Flag | Long | Description |
|------|------|-------------|
| `-i` | `--id` | Output bare C4 ID(s) instead of c4m |
| `-p` | `--pretty` | Pretty-print (aligned columns, local time, comma sizes) |

## Comparing and Patching

Compare two sources:

```bash
c4 diff old.c4m: new.c4m:
c4 diff :~1 :                    # what changed in last operation
c4 diff :~release-v1 :           # changes since tagged state
```

Apply changes:

```bash
c4 patch : changes.c4m           # apply delta (tracked, undoable)
c4 patch : desired.c4m           # converge to target state
```

## c4m File Endpoints

Establish a c4m file for writing:

```bash
c4 mk project.c4m:
```

Establish a location backed by c4d:

```bash
c4 mk store: localhost:17433
```

Create a directory inside a c4m file:

```bash
c4 mkdir project.c4m:src/
```

Copy content into a c4m file:

```bash
c4 cp ./src/ project.c4m:src/
```

Copy content from a c4m file to disk:

```bash
c4 cp project.c4m:src/ ./restored/
```

Remove an endpoint:

```bash
c4 rm project.c4m:
```

## Colon Syntax

The colon (`:`) is the portal between local paths, c4m files, and remote locations:

```
<endpoint>:<subpath>
```

- `project.c4m:` — the c4m file root
- `project.c4m:src/main.go` — a file inside the c4m file
- `store:assets/` — a directory in a location
- `:` — the managed current directory
- `:~1` — one snapshot ago
- `:~release-v1` — tagged snapshot

Trailing colon means "look inside" — it's the boundary between the endpoint and its contents.
