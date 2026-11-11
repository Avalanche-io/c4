# The Validator reads through the decoder (2026-11-11)

## The fault, measured

`c4m.Validator` splits each line into fields with its own parser
(`strings.Fields`, a first-`/` rule for names, its own link-operator
scan). It shares no code with `c4m.Decoder`, which is the reader every
verb, every port and every shared vector is held to. Run on every file in
`c4m/testdata/*-vector/` (31 files), the Validator's verdict differs from
the decoder's on 18:

| What the Validator does                                                             | Files                                                                              |
| ----------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Rejects every patch chain: a bare C4 ID line is "insufficient fields"               | chain `vector`, deepen `deepen` and `remove`, every `patch.c4m`, `empty-dir-patch` |
| Reads a link target as part of the name, then refuses the `/` in it                 | link-target `canonical`, `pretty`; both ten-dash legacy files                      |
| Rejects the pretty form's timestamps ("must end with 'Z'")                          | link-target `pretty`                                                               |
| Calls the library's own canonical order unsorted (strict: byte order of full paths) | backslash-name `canonical`, `pretty`; zero-null `canonical`, `pretty`              |
| Accepts a damaged mode letter (`-rr-r--r--`, `-rwtr--r--`)                          | damaged-line `mode-wrong-letter`, `mode-sticky-letter-misplaced`                   |
| Accepts a torn C4 ID (89 or 91 characters) in lax mode, as a warning                | damaged-line `id-89`, `id-91`                                                      |

So it refuses valid c4m files that every implementation reads, and accepts
damaged ones every implementation refuses. Two readers of one format is the
duplicate-implementation fault the design philosophy names ("one
implementation per idea").

The Validator has no caller outside its own package's tests. It is public
API, so on master (1.1, the breaking tier) its verdicts may change.

## The rule

There is one reader. The Validator decodes with `c4m.Decoder`. Whatever
the decoder refuses, the Validator reports as one fatal error naming the
decoder's line; whatever it reads, the Validator does not re-parse. So every
format rule (fields, link targets, escapes, pretty form, patch chains and
their checkpoints, damaged fields, depth jumps) has exactly one
implementation, already pinned by the shared vectors.

To see entries as written (the decoder sorts its result), the decoder gains
an unexported hook: as it parses each entry line it hands the Validator the
entry, its line number and its patch section. Nothing about decoding
changes. The Validator keeps only what the decoder does not do:

- **Duplicate paths** within one section: an error naming both lines. The
  spec says parsers MUST reject them ("Parsers MUST: … Reject duplicate
  paths within the same scope"). The decoder does not today, and that
  question is escalated (below). The Validator reports them now, as it
  always has.
- **Order, strict only:** entries out of canonical order (files before
  directories, natural sort among siblings, as `SortEntries` orders them)
  within a section. The spec lets a parser accept non-sorted input ("Parsers
  MAY: Accept non-sorted entries"), so lax mode does not report it.
- **Stats** from decoded entries: types from the mode, sizes, times, depth.

Retired with the old parser: the ergonomic-format detector (it guessed the
form from month names, a form no encoder writes) and the flow-label
pattern (the decoder's `parseFlowTarget` owns flow targets). The Validator's
messages for format faults become the decoder's.

## The test

Every shared vector: the Validator, strict and lax, refuses exactly the
files the decoder refuses, naming the same line. The canonical files the
library writes are valid in strict mode. A duplicate path and an
out-of-order pair are reported with their lines.

## Escalated, not decided here

**The decoder accepts duplicate paths**, contrary to the spec's MUST,
measured: two `a.txt` lines decode and get a C4 ID. It does so by
discarding: `SortEntries` deduplicates siblings by name and keeps the last
("most recent wins", `sortSiblingsHierarchically`), with no message, so the
first entry's size, time and C4 ID vanish. Making the decoder
refuse them changes what every verb reads, and every port's decoder, so it
is Joshua's call. Until then the Validator is where a duplicate is caught.
