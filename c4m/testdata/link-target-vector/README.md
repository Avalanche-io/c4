# Cross-implementation vector: link targets that look like the C4 ID field (2026-11-10)

The shared conformance fixture for reading a link line back exactly as it
was written (SPECIFICATION.md entry format: the C4 ID is always the last
field; "Symlinks": a target escapes its spaces, so it never holds an
unescaped one). Every c4m implementation MUST pass it.

## Files

| File            | Made by                                                                  |
| --------------- | ------------------------------------------------------------------------ |
| `canonical.c4m` | the Go encoder, canonical form (`TestLinkTargetVector`)                  |
| `pretty.c4m`    | the Go encoder, pretty form, timestamps rendered in UTC                  |
| `patch.c4m`     | `EncodePatch` over the empty tree: the empty tree's ID, then every entry |
| `root-id.txt`   | `ComputeC4ID` of the manifest all three files describe                   |

No line was written by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestLinkTargetVector` rewrites the files from the library; the test then
fails if the files differ from what the library writes.

## What a conforming implementation MUST do

Decode `canonical.c4m` and `pretty.c4m` to one manifest whose root ID is
`root-id.txt`, and decode `patch.c4m` to one section over the empty
tree's ID holding the same entries, with these targets and links:

| Entry            | Mode, time, size                    | Target                    | Link           | Own C4 ID |
| ---------------- | ----------------------------------- | ------------------------- | -------------- | --------- |
| `id-null`        | null                                | a C4 ID's text            | symlink        | null      |
| `id-set`         | `lrwxrwxrwx`, known                 | a C4 ID's text            | symlink        | present   |
| `id-exr-null`    | null                                | a C4 ID's text and `.exr` | symlink        | null      |
| `id-exr-set`     | `lrwxrwxrwx`, known                 | a C4 ID's text and `.exr` | symlink        | present   |
| `c4-prefix-null` | null                                | `c4studio`                | symlink        | null      |
| `objects-null`   | null                                | `../objects/` and an ID   | symlink        | null      |
| `hard-null-id`   | null                                | none                      | ungrouped hard | present   |
| `hard-null-nil`  | null                                | none                      | ungrouped hard | null      |
| `epoch-link-nil` | `lrwxr-xr-x`, null time, size known | `missing.txt`             | symlink        | null      |
| `epoch-link-id`  | `lrwxr-xr-x`, null time, size known | `real.txt`                | symlink        | present   |

## The rules under test

- **After a link operator, a single field is the C4 ID field; two fields
  are a target and the C4 ID field.** Under a mode that does not say
  symlink (here the null mode views carry), `-> X -` is a symlink to `X`
  even when `X` starts with `c4` or is a C4 ID's text; only `-> <C4 ID>`
  or `-> -` alone is an ungrouped hard link. The Go decoder through
  4b1fe0a read any `c4`-prefixed target there as a hard link and the
  rest of the line as its ID, and refused the line.
- **The pretty form's padding before the C4 ID field is not part of the
  target.** A pretty line with a null timestamp keeps its alignment
  padding, and the Go decoder through 4b1fe0a (and 1.0.24) took that
  padding into the target of a link whose own C4 ID is null or present,
  escaped it on the next write, and so changed the tree's ID without an
  error. `c4 id -e` reaches this on any tree holding a symlink whose own
  mtime is the epoch; the two `epoch-link` entries are exactly what it
  writes for one, dangling and live.

## A flow-shaped target under a null mode

A symlink whose target has the shape of a flow target (`host:path`,
`C:foo`) cannot be told from a flow link when the mode is null. Ruled
2026-11-13 (decided by the release lead; reversible): under a null mode the
line is a flow link, and a writer keeps a symlink's type by writing its mode
`lrwxrwxrwx` even when the rest of the mode is null (the content
projection does). `TestAFlowShapedTargetIsALinkOnlyUnderASymlinkMode` pins
both readings.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `ID in` a file: a chain; the resolved state has that C4 ID.
- `refused`: decoding fails.
- `unpaired`: it decodes; no file holds its canonical form, for the reason given.

| File            | Re-encodes to   |
| --------------- | --------------- |
| `canonical.c4m` | itself          |
| `patch.c4m`     | itself          |
| `pretty.c4m`    | `canonical.c4m` |
