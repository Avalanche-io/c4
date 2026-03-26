# The C4M Identity Paradox

## One Directory, One Identity

C4 makes a commitment: the same directory always has the same C4 ID. This
is the foundation of everything — diffing, syncing, deduplication, indelible
metadata. Without it, two people looking at the same files could compute
different IDs, and the entire system breaks.

To keep this promise, C4 normalizes c4m content before computing its ID.
When you identify a file that happens to be a c4m file, C4 parses it,
produces the canonical form, and hashes that. Not the bytes on disk. The
canonical bytes.

This is a simple rule with strange consequences.

## What "Canonical" Means

A canonical c4m file has:
- Entries sorted: files before directories, natural sort within each group
- No extra whitespace, no column alignment, no pretty-printing
- Timestamps in UTC with `Z` suffix
- Null fields as single `-`
- Single space between fields
- One trailing newline per entry

Two c4m files that describe the same filesystem — same files, same names,
same IDs, same metadata — produce identical canonical text. Different
formatting, different sort order, different indentation width — all
normalize to the same canonical form, and therefore the same C4 ID.

## The Strange Part

### Your c4m file's ID is not the hash of its bytes

If you pretty-print a c4m file with `c4 cat -e project.c4m > pretty.c4m`,
then identify it with `c4 pretty.c4m`, you'll get the same C4 ID as the
canonical version. But if you run `sha512sum pretty.c4m`, you'll get a
different hash — because `sha512sum` hashes the raw bytes, and the raw
bytes include the pretty formatting.

This is the one case where `c4 <file>` and `sha512sum <file>` diverge.

C4 isn't lying. It's telling you the identity of the *content* — the
filesystem description. The formatting is presentation, not content.
But if you're expecting byte-for-byte hash equivalence with other tools,
this will surprise you.

### Non-canonical c4m files are ephemeral

If you write a c4m file by hand, add some comments between entries for
your own notes, indent with tabs instead of spaces, or sort entries
alphabetically instead of files-before-directories — all of that
disappears the moment the file passes through any C4 tool.

`c4 cat hand-edited.c4m` outputs the canonical form. If you store it
using C4's built-in content store, the canonical form is what's stored.
The hand-edited version is an ephemeral view that C4's tools are blind to.

This is how C4's built-in tools behave today. The C4 ID is always
computed from the canonical form, and content stored under a C4 ID
always matches that ID byte-for-byte — that's inviolable. A store
that kept non-canonical bytes under a canonical ID would be broken.
What may vary between implementations is *when* normalization happens
and whether non-canonical views are preserved separately for other
purposes. Even C4's own approach here may evolve.

### c4 cat shows different depths depending on input

```
# A c4m file with inlined nesting (created by c4 <dir>)
$ c4 cat -e project.c4m
...shows files AND nested subdirectory contents...

# The same directory's ID, fetched from the store
$ c4 cat -e <root-dir-id>
...shows only direct children...
```

Same logical content. Different display. The inlined file has everything
because the scanner put it there. The bare ID in the store only has direct
children — that's how directory IDs are computed (one level).

Use `-r` to explicitly request recursive expansion from the store:
```
$ c4 cat -er <root-dir-id>
...shows the full tree, fetched level by level from the store...
```

This inconsistency exists because a c4m file on disk is a convenience
format — it's allowed to contain more than what the canonical form
strictly requires. The canonical form of a directory is one level. The
inlined form is a human convenience. Both are valid, both have the same
C4 ID.

### A pretty-printed c4m might "change" after round-trip

```
$ c4 cat -e project.c4m > pretty.c4m
$ c4 cat pretty.c4m > canonical.c4m
$ diff pretty.c4m canonical.c4m
# ...differences in formatting...
```

The content is identical. The C4 ID is identical. But the bytes on disk
are different. This is the same thing that happens when you `go fmt` a
Go file or `prettier` a JavaScript file — the meaning is preserved, the
formatting is normalized.

If your workflow depends on byte-stable c4m files, always work with the
canonical form. Use `c4 cat file.c4m > file.c4m` to normalize.

### An editor might reload after another tool canonicalizes

If you have a c4m file open in an editor and another process (a watcher,
a sync tool, c4d) canonicalizes it, the editor may detect the change and
offer to reload. The content hasn't changed — the formatting has. This
is expected.

## Why We Made This Choice

### The alternative is worse

If C4 hashed raw bytes for c4m files, then:

- Two people scanning the same directory on different machines (different
  locale, different timezone display, different default permissions) could
  get different IDs for identical filesystems.

- Storing a c4m file and retrieving it could change its ID if any
  normalization happened in between.

- You couldn't reliably diff c4m files by ID. You'd have to parse and
  compare every time.

- The deduplication promise breaks. The same project description could have
  multiple IDs based on formatting, defeating the point of universal identity.

C4's current behavior — normalize first, then hash — means the ID always
reflects the *meaning*, not the *presentation*. We believe this is the
right tradeoff for an identity system, but we hold it as a strong
convention rather than an immutable law. So far.

### It's what databases do

SQL doesn't preserve your whitespace when you insert a row. JSON parsers
don't preserve key order (in most languages). Protocol Buffers serialize
to a canonical wire format regardless of how you constructed the message.
CSV normalizers strip trailing whitespace.

C4M canonical form is the same concept: a defined serialization that
preserves semantics while normalizing presentation. It only seems odd in our
case because we are exposing the file format directly to users. This in itself
is a powerful feature — you can hand-edit c4m files, and read them as easily as
`ls -l`, but these apparent paradoxes are the price of that power.

### It's what git does (sort of)

Git normalizes line endings (CRLF → LF) based on `.gitattributes`
settings. A file with Windows line endings and the same file with Unix
line endings can have the same blob hash if the clean filter normalizes
them. This surprises people too. But it's the right behavior for a
content-addressed system that operates across platforms.

Git, however, does not normalize formatting within files — and it
shouldn't. Git can't know the semantics of every file format, so it
treats bytes as bytes. The consequence is format-only commit diffs:
someone auto-formats a file and every line shows as changed, even
though nothing meaningful changed. This has real practical impact on
code review, blame, and merge workflows.

C4 can make a different choice for c4m files because we control the
format. We know exactly what's meaningful and what's presentation.
Normalizing before identification eliminates the format-only diff
problem entirely — for the one file format where we have that power.

## What We're Watching

These are areas where the current behavior might evolve based on real
usage:

### Should c4 cat flatten inlined files?

Currently `c4 cat file.c4m` shows whatever nesting is in the file.
The alternative: always flatten to top-level (canonical), require `-r`
to show nesting. Pro: consistent output regardless of input. Con:
removing content from a display command feels wrong.

### Should c4 auto-canonicalize on disk?

Currently c4m files on disk can be in any format. C4's built-in tools
normalize during identification and storage. The alternative: every
tool that writes a c4m file writes canonical form. Pro: eliminates the
ephemeral view concept entirely. Con: breaks hand-editing workflows,
makes c4m files less human-friendly.

### Should there be a "formatted ID" separate from "canonical ID"?

Currently there's one ID per c4m — the canonical one. The alternative:
track both the canonical ID (for content identity) and the raw byte
hash (for file integrity). Pro: no more sha512sum divergence. Con:
two IDs for one file is confusing, and the raw hash has no semantic
meaning.

### Should non-canonical c4m be a warning?

When C4 detects a non-canonical c4m file, should it warn the user?
Currently it silently normalizes on ingest, and silently discards the
ID of a formatted c4m — the raw-byte hash is never computed or shown.
A warning ("note: c4m normalized to canonical form") would make the
behavior more visible. But it would also be noisy for every
pretty-printed c4m file.

## The Bottom Line

C4 promises: same content, same ID. For c4m files, "content" means
the filesystem description, not the byte representation. This is a
deliberate choice that preserves the most important invariant at the
cost of some surprising behavior around formatting.

If this causes problems in your workflow, we want to hear about it.
File an issue at https://github.com/Avalanche-io/c4/issues with a
concrete example of what went wrong. The canonical behavior is
settled, but the edges — display depth, disk formatting, warnings —
are open for refinement.
