# Cross-implementation epoch-second vector (2026-11-16)

The shared conformance row for a time inside the epoch second. A c4m line
carries whole seconds, the second a time falls in (SPECIFICATION.md,
"Timestamp": `YYYY-MM-DDTHH:MM:SSZ`), and the epoch is null (`-` or `0`,
"internally represented as Unix epoch"). So a time that floors to the
epoch second, such as a file's mtime of 0.5 s, is null, and `-` is its only
stable spelling. Every implementation that scans a tree, builds entries or
writes c4m text (Go `c4m` and `scan`, c4py, c4ts, c4m-swift, libc4) MUST
pass it.

## Files

| File          | What it is                                                                  |
| ------------- | --------------------------------------------------------------------------- |
| `tree.txt`    | the tree to build: one `<octal mode> <mtime ns> <path>` per line            |
| `saved.c4m`   | `c4 id tree/ \| c4 patch -` of that tree, by Go master with this fix        |
| `root-id.txt` | the description ID of the tree, of `saved.c4m`, and of the scan's own c4m   |

Build the tree as the sub-second vector's README says: make every path (a
path ending in `/` is a directory; a file's bytes are its base name and a
newline), then set each mode and time, files first and then directories
deepest first.

## A conforming implementation MUST

1. **Test null at the format's resolution.** A time is null when the
   second it falls in is the epoch: `exact.txt` (0), `half.txt` (0.5 s)
   and `sub/in.txt` (0.25 s) are null, and their lines read `-`. `one.txt`
   (1 s) is `1970-01-01T00:00:01Z` and `pre.txt` (−0.5 s) is
   `1969-12-31T23:59:59Z`, neither null. An entry a caller builds in memory
   with such a time is null too, not only a scanned one.
2. **Converge on one description ID.** The scan's description ID, the ID
   of the c4m the scan writes (as a chain or flattened), and `saved.c4m`'s
   are all `root-id.txt`, on a platform with POSIX modes. On any platform
   the first two agree.
3. **Read its own scan.** A scan written as a chain resolves without a
   checkpoint mismatch.

Measured before the fix, on Go master 2f1380f: half.txt and in.txt were
written `1970-01-01T00:00:00Z`, which reads back as null; `c4 id dir/`
named a boundary its own reader rejected ("patch ID does not match prior
content"); and the description ID of a flat tree differed from its c4m's.
The same order (null test before the floor) was found in c4py, c4ts and
c4m-swift for entries built in memory.

Go pins it in `scan/epoch_second_vector_test.go`,
`cmd/c4/epoch_second_vector_test.go` and `c4m/timestamp_null_test.go`. The
fixture is the contract: regenerating it is never necessary and never
permitted.

## Round trip

| File        | Re-encodes to |
| ----------- | ------------- |
| `saved.c4m` | itself        |
