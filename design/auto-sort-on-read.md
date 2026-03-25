# Auto-Sort on Read — Tolerant C4M Input

## Summary

Every tool that reads a c4m file should sort entries on load. Malformed ordering is never an error — it's unsorted input that gets fixed silently (or with a warning).

## Motivation

c4m files are hand-editable text. Users will:
- Append entries at the end (`echo "- - - newfile.txt -" >> project.c4m`)
- Pipe unsorted path lists through `c4 paths`
- Copy/paste entries between files
- Edit with `vi` and not think about sort order

If tools reject or silently misinterpret out-of-order input, hand-editing becomes fragile. The tools should be tolerant.

## Design

- The decoder calls `SortEntries()` after parsing, before returning the manifest
- `c4 paths` sorts output regardless of input order
- Optionally emit a warning to stderr when input was reordered: `c4m: warning: entries were not in canonical order (reordered)`
- Never error on sort order
- The canonical sort rules (files before directories, natural sort) are the only valid ordering — tools always produce this

## Impact

This makes the demo script simpler:
```bash
# Just append. The next tool that reads it fixes the order.
$ echo '- - - .blenderrc c43Q4j81...' >> template.c4m
```

Instead of:
```bash
# Prepend to maintain sort order manually.
$ echo '...' | cat - template.c4m > tmp && mv tmp template.c4m
```

## Status

Pending implementation. Not blocking for announcement — the demo can use append and note that tools auto-sort.
