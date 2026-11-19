# c4m nulls in context: how partial knowledge moves through the system

Status: draft for Joshua, 2026-11-08, at his request via c4-fleet. Not a
decision. Everything measured below was measured on master at 59b7636
with the binary built from it, on 2026-11-08; the spec quoted is
`c4m/SPECIFICATION.md` at the same commit.

## What Joshua asked for

"It is correct in many places for c4 to show null or incomplete
information, and to try to represent it as well as it can in that
incomplete state. So we need to consider how this progressive refinement
of information propagates through the system in various situations. It
wasn't really the intent to be able to materialize progressive
refinement, but it is useful for partial validation."

His examples: a null-filled c4m that skips identifying files for speed,
sent to a remote so it returns only the date and size changes, after which
the client chooses which files to hash. "A file with a name, size and date
with no id is a file that can be any content at all that fits those
parameters", for example during a scan. And on materialising: "an empty
file of the given size could be created, or there may be a similarly
clever way to encode the null id for round trip." Later: nothing may be
out of band or complex, "because every c4 supporting tool is expected to
handle these things correctly." Every rule has to live in the c4m and
the bytes.

## The one sentence

A null is a statement about the describer's knowledge, never about the
file. Every context below either preserves that statement, narrows it, or
has to turn it into bytes; only the last kind needs a rule beyond "carry
the null".

## The contexts

| Context | What a c4m is here | What a null means | What may change a null |
| --- | --- | --- | --- |
| **Description** | a record of what was observed | not observed | nothing: a description is never filled in transit |
| **Refinement** | a layer in a chain | not yet observed | a later layer, by the describer who observed it |
| **Query and sync** | a request | "tell me about this field" | the answer, as a patch from the side that knows |
| **Validation** | an expectation | "don't compare this field" | nothing; the comparison is projected to the expectation's knowledge (spec, "Materializing null metadata", rule 4) |
| **Materialisation** | a target for bytes on a filesystem | a field the filesystem must have a value for anyway | the materialiser, by a rule that round-trips (rules 1 to 3 and 5) |

The first four never invent a value. The fifth cannot avoid choosing one,
because a filesystem has no null, so each field has a rule for what is
written and how it reads back:

| Field | On create | On an existing path | Reads back as | Source |
| --- | --- | --- | --- | --- |
| timestamp | epoch | left alone | null (epoch is the null spelling) | rule 2; measured, round-trips |
| mode | platform default | left alone | the default, a known value | rule 3; a scan at finer knowledge |
| size | (has no value of its own; follows the content) | | | |
| C4 ID | nothing is written; the entry is reported as kept-as-found | left alone | whatever was there | plan.go `cannot create without a C4 ID`; measured below |
| size on a mount | the size the content has, found from any source; `0` if no source has it | | null, because `0` beside a non-empty ID is null (`design/null-size-zero.md`) | rule 5, ruled 2026-11-08 |

## Verbs and surfaces, mapped

| Verb or surface | Context | Nulls today | Notes |
| --- | --- | --- | --- |
| `c4 id DIR` | description (+ refinement: the stream is base, boundary, patch) | written where not observed; `-m s`/`-m m`/`-m c` null fields on purpose | the one producer of nulls by intent |
| `c4 id FILE.c4m` (normaliser) | description | re-derives directory aggregates from listed children; fills a null the children determine; nulls one they cannot support; never touches a leaf's null | correct: it only computes what the file already implies |
| `c4 id -` (stdin) | description | as above | |
| `c4 cat` | description | serves bytes by ID; a null ID has nothing to serve | |
| `c4 diff` | query: "what differs at the knowledge the description has" | a description side sets the level; a dir side is scanned and projected to it; `-c` is the stated trust | dir/dir compares at content level (see the open point below) |
| `c4 patch` | refinement | folds layers; nulls fill only from later layers | text algebra, never touches a filesystem |
| `c4 log`, `c4 paths`, `c4 split`, `c4 explain` | description (readers) | carried | |
| `c4 intersect` | query (set algebra on names) | result entries carry null mode and timestamp on purpose (`intersect.go:183`) | a set, not a description of a tree |
| `c4 merge` | query (three-way on descriptions) | LWW by timestamp; a null timestamp loses to any value; result directories get size -1 for the normaliser (`merge.go:243`) | a null timestamp means "unknown", and "unknown loses" is a choice that should be stated in the merge doc |
| `c4 restore` | materialisation, then validation | create: epoch and default mode; existing: left alone; no C4 ID: kept as found and reported; verification at the description's knowledge (`projectToKnowledge`) | measured below |
| `reconcile` package | materialisation | same as restore | the one implementation |
| store ingest (`c4 id -s`) | description | stores listings as written, nulls included | an ingested description's ID is its own; `-m` projects downward only |
| the mount (not yet built) | materialisation (serving) and description (capture) | serving: every field needs a value, rule 5 for size; capture: writes `-` for what it knows is null | the only surface that must show a null size as a number |

### Where today's code mixes contexts

1. **The restore dry run is a query printed in description form**, and
   until PR #67 it ended with the target's ID, which no description may
   do. Fixed on the branch.
2. **`c4 diff a/ b/` chooses a knowledge level the caller did not state.**
   Two directories compare at content level (`smartResolve`: "Both
   directories: compare at content level"), so the boundary ID is the
   content-level ID; appended to a full scan it makes a chain whose
   checkpoint mismatches. The verb mixes "what content differs" (query at
   a level) with "produce a layer for this chain" (refinement at the
   chain's level). Either the dir/dir level follows `c4 id`'s default or
   the docs say a dir/dir diff is a content query and chains grow with
   `c4 diff FILE.c4m DIR`.
3. **`c4 merge` treats a null timestamp as a value that loses**, which is
   a materialisation-style decision inside a query verb. Carrying the
   null forward (unknown stays unknown unless the other side knows) is
   the description-context answer; the current rule is defensible but is
   not written down.
4. **Restore's verification failure message says "re-run the restore"**
   when the failure is a no-ID entry that no re-run can create (measured
   below). The message belongs to the materialisation context; the
   failure belongs to the description's incompleteness.

## Materialising name, size and date with no C4 ID

What restore does today, measured (`c4 id -m m src > m.c4m`, sizes and
dates known, IDs null; restore into an empty directory):

```
incomplete entry kept as found: a.txt
incomplete entry kept as found: b.txt
stored: c459…   (the empty pre-image)
verification failed: as-built c42eub…
re-run the restore; the pre-image c459…
exit 3
```

The directory `d/` was created; no file was. So, as built: **rule 4 does
not make a no-ID entry pass**. The projection nulls the built side's C4 ID
where the target's is null, but a path that was never created is simply
absent from the built tree, so the IDs differ and restore fails, exit 3.
That is truthful (the tree does not match the description) and the
"re-run" hint is wrong for this case.

The question is whether restore should create something instead. Three
options, each with its failure mode.

**A. Refuse and report (today's behaviour, with a better message).**
No bytes are invented. The entry is reported as kept-as-found, the exit
code says the target was not reached, and the message should say that
these entries cannot be created from this description rather than "re-run".
Failure mode: a tree built from a `-m m` description is missing every
file, and a caller who only reads the exit code learns no more than
"failed". Verification could report *which* entries were uncreatable and
exclude them from the ID comparison, so the exit distinguishes "could not
build" from "built wrong"; that is a reporting change, not a filling rule.

**B. Create zeros of the stated size and date (c4-fleet's proposal).**
Verification then passes under rule 4, because the C4 ID is not compared
and size and date match. A later scan without the original c4m truthfully
reports the zero content's ID and size; nothing is encoded in the bytes.
Failure mode: the created file is indistinguishable from a real file of N
zero bytes. A later `c4 diff` against the original description shows no
change (the description has no ID to differ from), so the placeholder is
only discoverable by comparing against a description that *does* know
the content. Joshua's own concern applies in the other direction too: a
real all-zero file of size N (padding, a blank image plane) is a
legitimate file, so no reader may ever infer "placeholder" from zeros.
That is fine as long as nobody tries to: B is honest only if the zeros
are never read back as null. Also: with size null as well (the `-m s`
case) there is nothing to create, so B degrades to A for those entries.

**C. Encode the null in the bytes.** Rejected. Every bytes-only encoding
(a sentinel pattern, a sparse file, a specific length) is a legitimate
file somewhere in the wild, so a reader that recovered the null from the
bytes would destroy real data. The empty-file rule is different in kind:
`0` beside a non-empty ID is a contradiction, not a pattern, which is why
it can carry a null and these cannot.

**D. A marker outside the bytes** (xattr or sidecar: materialise at size,
mark as a C4 placeholder, read null only for a marked unchanged file).
Withdrawn: Joshua ruled out anything out of band or complex, since every
C4-supporting tool must get it right. Recorded here so it is not proposed
again: it also fails on exFAT and on any copy that drops xattrs, and
restore's own temp-plus-rename would have to re-apply it.

Recommendation: **A with the reporting change, and B only behind an
explicit flag** (`--fill-unknown` or similar), never by default. The
default materialiser should not write bytes the description does not
name; a caller who wants a shape to work in (a layout to mount tools
over, a size-accurate stand-in for a transfer plan) can ask for zeros and
knows they asked. The flag's output is an ordinary tree of real files;
the c4m that described it is the only record that they stand in for
unknown content, and that is exactly where Joshua said the record belongs.

## Joshua's two later questions

**Should a non-zero-size empty file on disk encode a null C4 ID?** No. An
all-zero file of size N is a real file; reading it as "no ID" would let a
content-filled file replace one that was known to be empty-of-content,
which is the interchangeability he warned about. The scanner records the
ID of the bytes it read, always. The only null-encoding the format has is
the one contradiction it can rely on, `0` beside a non-empty ID.

**Does `0` with no C4 ID read as null?** No (corrected from my first
answer to c4-fleet). `c4 id -m m` writes `0 name -` for a real empty file,
known from `stat`. It is a known zero.

## Rule 4 across the implementations

Rule 4 (verify at the description's knowledge) is implemented once, in Go,
in `cmd/c4/restore.go` `projectToKnowledge`: the built tree's mode,
timestamp, size and C4 ID are nulled wherever the target's are, directory
aggregates are re-derived by the normaliser, and the IDs are compared.

| Implementation | Materialises? | Null handling on create | Verifies at knowledge (rule 4)? |
| --- | --- | --- | --- |
| Go `reconcile` + `c4 restore` | yes | timestamp → epoch; mode → default; no ID → kept as found, reported | yes, `projectToKnowledge`; but an uncreated entry still fails (measured) |
| c4py `reconcile.py` | yes | timestamp: set only when not null (left alone on create too, so a created file carries the write time, not the epoch); no ID → `full.touch()`: **an empty file is created**, size 0, no metadata applied | no. `verify.py` checks only entries that have a C4 ID and reports missing/corrupt/extra by content; size, date and mode are not compared, and a no-ID entry is never checked |
| c4ts `reconcile.ts` | yes (`workspace.checkout`) | content written only when the entry has an ID; metadata goes through an optional `setMeta`, implemented only by the in-memory filesystem, which applies mode and mtime as given (a null timestamp sets the epoch, right; a null mode sets 0, **wrong: `----------`, not the platform default**); on a real filesystem no metadata is applied at all, so a stated time or mode is dropped; no ID → no file is created, silently | no equivalent of rule 4 found; `verify.ts` not examined in depth |
| c4m-swift | no (decoder only) | n/a | n/a |
| libc4 | no (decoder only) | n/a | n/a |

Two defects worth filing from this table, both in materialisers, neither
blocking the note: c4py invents an empty file for a no-ID entry (it is
option B with size 0 regardless of the stated size, and it is silent);
c4ts's in-memory filesystem materialises a null mode as zero permissions, and its real one applies no metadata. All are the
"distinguish discarding from inventing" failure the primer names.

## The mount, as specified so far

The mount is a materialiser that serves rather than writes. Joshua's
rulings for it (2026-11-07 and -08): never a null size, time or mode is
shown; `----------` serves as 0000; a non-local file refuses to open; a
null size is found from any source and shown as `0` only when none has
it. In context terms: serving is rule 2, 3 and 5 applied live; capture
(write-back) is description, and writes `-` for what it knows is null.
The round trip Joshua named for size holds for the mount's own capture,
which knows the entry's ID; a client scanning the mount cannot recover an
ID for bytes it was refused, and records a read error instead (partial
scan, exit 2), which is the right outcome.

## What this note does not decide

Whether restore gains a fill flag; whether verification should exclude
uncreatable entries from the comparison; whether `c4 diff a/ b/` keeps
the content level; whether `c4 merge`'s "null loses" stands. All four are
Joshua's.
