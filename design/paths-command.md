# c4 paths — Bidirectional Path/C4M Converter

## Summary

`c4 paths` is a Unix filter that converts between c4m format and plain paths.

- c4m input → path output
- path input → c4m structure output (null metadata)

It detects the input format automatically.

## Usage

```
c4 paths <file.c4m>          # c4m to paths
c4 paths -                    # read c4m from stdin
cat file.c4m | c4 paths       # pipe
find . -type f | c4 paths     # paths to c4m
echo "src/main.go" | c4 paths # single path to c4m
```

## Examples

### c4m to paths

```
$ c4 paths project.c4m
.blenderrc
scenes/monkey.blend
renders/render_001.png
assets/
```

One path per line. Directories end with `/`. Full paths reconstructed from c4m depth/indentation.

### paths to c4m

```
$ echo -e "scenes/monkey.blend\nrenders/render_001.png" | c4 paths
drwxr-xr-x - - renders/
  - - - render_001.png -
drwxr-xr-x - - scenes/
  - - - monkey.blend -
```

Parent directories are created automatically. All metadata fields are null (`-`). This produces a structure-only c4m that can be filled in with `c4 id -c`.

### Composition

```
# List all .blend files in a project
c4 paths project.c4m | grep '\.blend$'

# Create a c4m from a find command
find /farm/output -name '*.exr' | c4 paths > renders.c4m

# Fill in metadata
find . -type f | c4 paths | c4 id -c - .
```

## Detection

Input is c4m if any line starts with a file mode character (`-`, `d`, `l`, `p`, `s`, `b`, `c`) or whitespace (indented child entry). Otherwise input is paths.

## Output

- c4m → paths: one path per line, no trailing newline on last line
- paths → c4m: canonical c4m format, sorted, null metadata
