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
- **Names that are paths:** checked here at first, by the library's
  `isPathName`. Since the name rule (`c4m/testdata/name-rule-vector/`, the
  same day) the decoder refuses them itself, so the Validator reports the
  decoder's error and keeps no check of its own: one implementation.
- **An entry deeper than its parent directory allows.**
- **Flow targets**, checked on the decoded value: `<location>:<path>`, the
  location matching `[a-zA-Z][a-zA-Z0-9_-]*` (spec, "Flow Links"), the path
  neither absolute nor holding `..`.
- **Stats** from decoded entries: types from the mode, sizes, times, depth.

Retired with the old parser: the ergonomic-format detector (it guessed the
form from month names, a form no encoder writes). The Validator's messages
for format faults become the decoder's.

> **Corrected 2026-11-11, same day.** This note first said the flow-label
> pattern was retired too, because "the decoder's `parseFlowTarget` owns
> flow targets". That was an untested premise, and it was wrong: measured,
> `parseFlowTarget` only splits the target off its line, and the decoder
> accepts `nas:../escape/` and `nas:/absolute`. The check stays, on the
> decoded `FlowTarget`. That is a check of a value, not a second parse.

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

## What reading through the decoder found it cannot see

Some lines the Validator must refuse arrive from the decoder already read as
something else, so no check of decoded values can catch them. They are held
in `TestValidatorHeldForDecoderRules`. Each case there is the correct
expectation, and it skips with the reason printed.

- **A name holding `/` is cut at its first slash** and the rest of the line,
  C4 ID included, is dropped: `sub/file.txt` on a file line decodes as a
  directory `sub/`, with exit 0. Measured on Go (master and 1.0.25), c4py,
  c4ts and libc4 (main and release/1.0.x); c4m-swift refuses. The Go
  encoder writes such a name without error, so the library breaks its own
  round trip. Decided (2026-11-11): every decoder refuses such a line and
  every encoder refuses to write such a name, pinned by a shared vector,
  in 1.0.27. That change un-skipped the first group (now
  `TestValidatorRefusesNamesHoldingSlash`).
- **Lenient readings of names and indentation.** The decoder accepts an
  unescaped space in a name (so a damaged ID field such as `invalid-c4-id`
  or a 90-character `x4…` token becomes part of the name), a raw double
  quote (reserved, escaped `\"` by the spec), a raw control byte (encoded by
  the spec), and odd indentation (the spec: parsers MAY accept it "with
  warning"). An escaped control byte decodes to the same name as a raw one,
  so only the decoder can tell them apart. Whether the decoder refuses
  these or reports them to the Validator is not yet ruled; that ruling
  un-skips the second group.
