# Cross-implementation sub-second time vector (2026-11-16)

The shared conformance row for a scan's sub-second times. A c4m line
carries whole seconds (SPECIFICATION.md, "Timestamp":
`YYYY-MM-DDTHH:MM:SSZ`), so two scans of an
unchanged file differ only below the second, and a text round trip drops
that difference. Every implementation that scans a tree and diffs
c4m files (Go `c4m` and `scan`, c4py, c4ts, c4m-swift, libc4) MUST pass
it.

## Files

| File          | What it is                                                         |
| ------------- | ------------------------------------------------------------------ |
| `tree.txt`    | the tree to build: one `<octal mode> <mtime ns> <path>` per line   |
| `saved.c4m`   | Go's `c4 id tree/` of that tree on release/1.0.27, as written      |
| `root-id.txt` | the description ID `saved.c4m` resolves to                         |

To build the tree, make every path in `tree.txt` (a path ending in `/` is
a directory; a file's bytes are its base name and a newline), then set
each mode and time, files first and then directories deepest first, so a
child's time never moves its parent's.

## A conforming implementation MUST

1. **Scan at the format's resolution.** Scanning the tree gives the
   times `saved.c4m` shows: each mtime floored to its whole second.
   `b.txt`'s mtime is `…:40.9999997`; it is `15:06:40Z`, never `:41`. A
   scanner that turns nanoseconds into a float of seconds and rounds to
   microseconds records `:41`, and its C4 IDs differ from every other
   implementation's for the same tree.
2. **Diff at the format's resolution.** A diff of the scan against its
   own saved c4m, and against `saved.c4m` (on a platform with POSIX
   modes), is an empty patch: no entry lines. Comparing below the second
   writes every unchanged entry into the patch as a "modification" whose
   line, once encoded, is byte-identical to the base's, and `c4 patch`
   rightly reads an identical line as a removal. That deleted unchanged
   files in a documented c4py workflow on 1.0.26.
3. Resolve `saved.c4m` to the ID in `root-id.txt`.

Go pins it in `scan/subsecond_vector_test.go`. The times are fixed
constants. NTFS keeps 100 ns, which moves none of them across a second,
so rules 1 and 2 hold on Windows too. The fixture is the contract:
regenerating it is never necessary and never permitted.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes (`itself`: the output is the file,
byte for byte).

| File        | Re-encodes to |
| ----------- | ------------- |
| `saved.c4m` | itself        |
