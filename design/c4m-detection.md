# C4M Detection — How to Recognize a C4M File

## Purpose

Every C4 implementation must detect c4m content during identification and storage. This document defines the detection heuristic used across all languages.

## The Two-Phase Check

### Phase 1: Cheap Rejection (first 1-2 bytes)

Reject 99.99% of non-c4m files instantly:

1. Skip leading whitespace (spaces only — tabs are not valid c4m indentation)
2. Look at the first non-whitespace byte:
   - `-` → possible c4m (null mode or regular file mode)
   - `d` → possible c4m (directory mode)
   - `l` → possible c4m (symlink mode)
   - `p` → possible c4m (named pipe mode)
   - `s` → possible c4m (socket mode)
   - `b` → possible c4m (block device mode)
   - `c` → possible c4m (character device mode — but also matches C source code)
   - Anything else → NOT c4m

A single byte check eliminates virtually all non-c4m content: binaries (start with magic bytes), JSON (starts with `{` or `[`), XML (starts with `<`), scripts (start with `#!`), images, audio, video — none start with these characters.

The `-` case is the most common and also the most ambiguous (could be a diff file, a markdown list, etc.). Phase 2 resolves this.

### Phase 2: Full Parse Attempt

If Phase 1 passes, attempt to parse the content as c4m using the standard decoder. If the parse succeeds without error and produces at least one valid entry, the content is c4m.

If the parse fails for any reason, fall back to raw byte identification.

### File Extension Shortcut

If the file has a `.c4m` extension, skip Phase 1 and go directly to Phase 2. Trust the extension but verify with parsing.

## Implementation Notes

- Phase 1 is O(1) — just skip whitespace and check one byte
- Phase 2 is O(n) — full parse of the content. Only reached for the tiny fraction that passes Phase 1
- The heuristic is deliberately conservative: false negatives (missing a c4m file) are acceptable, false positives (treating non-c4m as c4m) are not
- If Phase 2 fails on content that passed Phase 1, silently fall back to raw bytes. No error, no warning.
- The `c` byte is the most ambiguous (matches C code, CSV, etc.). In practice, `c` as a mode type (character device) is rare, and C source files won't parse as c4m

## Edge Cases

- Empty files: not c4m (no entries)
- Files containing only whitespace: not c4m
- Files starting with a valid mode string but with garbled remaining fields: Phase 2 rejects them
- c4m files with BOM: rejected (c4m is UTF-8, no BOM)
- Binary files that happen to start with `-` or `d`: Phase 2 rejects them (binary content won't parse as c4m entry fields)

## When to Apply

**Scoped 2026-07-25.** Detection runs on the **argument/description
path only** — where a caller hands C4 something and asks what it *is*.
It never runs on the file-storing path.

Applies to:
- `c4 <file>` (bare identification of an argument)
- `c4 id <file>` (identification of an argument)
- piped stdin being identified as a description

## When NOT to Apply

- **Files inside a scanned tree — never.** `c4 id -s <dir>` stores
  every file's exact bytes, whatever its name and whatever its content
  parses as. This is what `help.go` promises verbatim (*"files inside
  scanned trees are ALWAYS bytes, whatever their name"*) and what
  c4py, c4ts, libc4 and c4-mcp already do.
- `store.put()` — the store has no object types. What an ID names is
  knowledge the caller brings (`snapshot-loop/round-1/draft-v7.md:66`).
- Content already known to be non-c4m by context.

**The rule is scoped, not universal:** if C4 is being asked what an
argument *is*, it checks for c4m. If C4 is storing a file, it stores
bytes.

### Why this changed

The original list included `c4 id -s <dir>` (for each file in the
directory), which authorized storing a transformed copy of any file
that parsed as c4m. That destroyed the original bytes, made a single
entry line record a size and an ID naming different bytes, and broke
whole-tree restore on any tree containing a `.c4m` file
([#43](https://github.com/Avalanche-io/c4/issues/43)). The goal that
motivated it — formatting-independent identity for descriptions — is
preserved by the argument path and by the recorded projection map; see
`c4m-canonical-storage.md`.

### Consequence for the heuristic's tolerance

With detection confined to the argument path, the cost of a false
positive drops sharply: a mis-detected *argument* yields a wrong answer
to a question the caller asked and can re-ask, whereas a mis-detected
*file* silently destroyed data. The conservative bias stated above
remains correct, but it is no longer load-bearing for data safety.
