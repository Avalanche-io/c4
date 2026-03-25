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

c4m detection runs in every identification and storage path:
- `c4 <file>` (bare identification)
- `c4 id <file>`
- `c4 id -s <dir>` (for each file in the directory)
- `store.put()` in every language
- `identify_file()`, `identifyBytes()`, `C4ID.identify()` etc.

## When NOT to Apply

- Piped stdin (`echo "data" | c4`) — stdin is identified as raw bytes unless the content passes the heuristic
- Content already known to be non-c4m by context (e.g., content retrieved from store by a known non-c4m ID)
- Actually: stdin SHOULD also be checked. Any content entering the C4 pipeline gets the detection treatment.

The rule is universal: if C4 is computing an ID, it checks for c4m first.
