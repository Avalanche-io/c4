# Cross-implementation vector: a name is never a path (2026-11-11)

The shared conformance fixture for entry names. Every c4m implementation
MUST pass it. The rule under test: only `/` separates paths in c4m, and a
name may end in `/` only as a directory's marker. A name holding any other
`/`, and the names `.`, `..`, `./`, `../` and `/`, are path components, not
entry names (SPECIFICATION.md, "Invalid names"; "Parsers MUST … Reject path
traversal attempts").

## Files

`ok-file.c4m` and `ok-dir.c4m` are one line each, written by the Go library
(`TestNameRuleVector`). Each other file is one of those lines with only its
name changed, made by the same test; no line was written by hand.
`C4M_UPDATE_VECTORS=1 go test ./c4m -run TestNameRuleVector` rewrites them,
and the test fails if a file differs from what the library and the damage
write.

## What a conforming implementation MUST do

**Reader.** Decode `ok-file.c4m` and `ok-dir.c4m`. Refuse every other file
with an error that names line 1.

**Writer.** Refuse to write an entry with any of these names, before writing
anything. Still write a directory's name ending in `/`.

| File                     | Name            | Through 1.0.26 (Go, c4py, c4ts, libc4)      |
| ------------------------ | --------------- | ------------------------------------------- |
| `file-sub.c4m`           | `sub/real.txt`  | read as a directory `sub/`, C4 ID dropped   |
| `file-leading-slash.c4m` | `/real.txt`     | read as a directory `/`, C4 ID dropped      |
| `file-trailing-path.c4m` | `real.txt/x`    | read as a directory `real.txt/`, ID dropped |
| `file-dotdot-inside.c4m` | `a/../real.txt` | read as a directory `a/`, C4 ID dropped     |
| `file-dot.c4m`           | `.`             | read as a file named `.`                    |
| `file-dotdot.c4m`        | `..`            | read as a file named `..`                   |
| `dir-nested.c4m`         | `a/empty/`      | read as a directory `a/`, C4 ID dropped     |
| `dir-dot.c4m`            | `./`            | read as a directory named `./`              |
| `dir-dotdot.c4m`         | `../`           | read as a directory named `../`             |
| `dir-root.c4m`           | `/`             | read as a directory named `/`               |
| `dir-double-slash.c4m`   | `empty//`       | read as a directory `empty/`, C4 ID dropped |

Measured 2026-11-11 on main and release/1.0.x of every implementation:
- **Go** (master and 1.0.25), **c4py**, **c4ts** and **libc4** read all
  eleven files as shown, with no error.
- **c4m-swift** refused the six files holding a `/` only incidentally: what
  followed the cut failed as a C4 ID field. It read the five path
  components (`.`, `..`, `./`, `../`, `/`) as names.
- **Encoders:** the Go encoder wrote all eleven names raw. The c4py, c4ts,
  c4m-swift and libc4 encoders wrote raw each name measured on them:
  `sub/real.txt`, `/real.txt`, `.`, `..`, `./`, `../`, `/` and `a/b/`.
- **Restore:** restoring `file-sub.c4m` with c4 master made a directory
  `sub` with the file's mode and failed its own verification.
