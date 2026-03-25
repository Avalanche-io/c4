# c4m Canonical Storage — c4m Files Are Always Canonicalized

## Rule

When C4 encounters content that parses as a valid c4m file — in any context (identification, storage, or retrieval) — it MUST canonicalize the content before computing the ID and before storing.

This is the one exception to the general rule that C4 identifies raw bytes.

## Why

Two c4m files that describe the same filesystem should have the same C4 ID, regardless of formatting. A pretty-printed c4m and its canonical equivalent describe the same thing. They should have the same identity.

Without this rule, `c4 id pretty.c4m` and `c4 id canonical.c4m` would produce different IDs for files with identical logical content. This defeats the purpose of content identity.

## Behavior

### Identification

```
$ c4 pretty-project.c4m
# Parses as c4m → canonicalize → hash canonical form → output canonical c4m entry
# The ID is the ID of the canonical form, not the raw bytes
```

### Storage

```
$ c4 pretty-project.c4m    # bare c4 stores by default
# Content stored is the canonical form
# The pretty formatting is lost in the store
```

### Retrieval

```
$ c4 cat <c4m-id>
# Returns canonical c4m (what was stored)
# Use -e for pretty-print, -r for recursive expansion
```

### The sha512sum divergence

```
$ sha512sum project.c4m     # hash of raw bytes on disk
$ c4 project.c4m            # hash of canonical form
# These will differ if project.c4m is not already canonical
```

This is documented and expected. c4m files are the one case where C4 deliberately diverges from raw byte hashing.

## Non-canonical c4m as ephemeral view

A c4m file on disk may be in any format — pretty-printed, hand-edited, unsorted, with comments between sections. These are all valid inputs. But they are ephemeral views. The durable, storable, identifiable form is always canonical.

Users may observe:
- A pretty-printed c4m file "changes" after store/restore (formatting normalized)
- An editor reloads a c4m file after another tool canonicalized it
- `diff` between a local c4m and a restored one shows formatting changes but no content changes

All of these are expected.

## Detection

C4 attempts to parse any file as c4m before falling back to raw byte identification. Detection heuristic: if the first non-blank line starts with a valid mode string (`-`, `d`, `l`, or a 10-char Unix permission string), attempt full parse. If parse succeeds, treat as c4m. If parse fails, fall back to raw bytes.

## Impact

This must be implemented in ALL identification and storage code paths across every language:
- Go: `c4 id`, `c4 cat`, `store.Put()`
- Python: `c4py.identify_file()`, `store.put()`
- TypeScript: `identifyBytes()`, `Store.put()`
- Swift: `C4ID.identify(url:)`, store operations
- C: `c4_identify()`, store operations

## Status

Design complete. Implementation pending — this is a significant change that touches the core identification path in every implementation.
