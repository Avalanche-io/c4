# Store layout — lowercased shard directories

Status: **drafted rule, attacked, not decided (2026-10-14).** Study runs:
grounding (compatibility, portability) 2026-10-03; scale, draft and three
adversaries 2026-10-14, all five completed. Structured results:
`store-layout-partial-results.json` (grounding) and
`store-layout-finish-results.json` (scale, draft, attacks). Everything
marked "run" was executed on this Mac (case-insensitive APFS, c4 1.0.18
and a master build); case-sensitive behavior was emulated, never run on a
real Linux filesystem. All three adversaries returned **needs-changes**,
none **refuted**; the required changes are listed below and none of them
alters the rule's two sentences.

## The problem

The 1.0.x store finds an object by walking a trie of 2-character
directories named from successive pairs of ID characters
(`<store>/c4/4E/<ID>`), flat until 4,096 objects, each leaf splitting again
above 4,096. IDs are case-sensitive; default macOS volumes are not. On a
Mac `4e` and `4E` are one directory, named by whichever was created first
(uppercase in practice, because split creates them in sorted order).
Copied to Linux, about 40% of objects were unreachable. v1.0.18 added a
tolerant lookup as a workaround; master does not have it.

## The rule (two format sentences)

Joshua, 2026-10-03: "the rule is probably that a store is toLower(id[0-6])
or something like that, for directory names in the store's shards for all
filesystems regardless." And: "c4 should support any size filesystem that
one might ever create... without making it unruly in the much more common
low end."

1. **Shard directories are named by the ID two characters at a time,
   ASCII-lowercased, at every level** (`c4`, `4e`, `uw`, …); the object
   file keeps its full exact-case 90-character ID as its name.
2. **An object is valid at any level of its own prefix path**
   (`<store>/<ID>`, `<store>/c4/<ID>`, `<store>/c4/4e/<ID>`, …). A store
   with no directories is the flat case.

By hand: from the store root, take the ID two characters at a time,
lowercased, and step into that directory while it exists; the object is
the file named by its full ID in the deepest directory reached, or in any
directory passed on the way. On an object store the key is the exact-case
ID (depth zero). Nothing else is in the format: no marker file, no fixed
depth, no threshold.

`toLower(id[0:6])` read as pairs is exactly the first three levels of this
trie, so Joshua's rule and the existing layout coincide.

## Reference reader and writer (implementation guidance, not format)

- **Lookup, deepest-first:** descend while the lowercase directory exists,
  stat the file there, then climb one level at a time to the root. Measured:
  identical to 1.0.18 on a hit (depth+2 stats), one extra stat per level
  only on a miss (4/6/6.2 at depth 1/2/3), against 11–13 for the 1.0.18
  fold fallback. Top-down order costs 2 extra stats on every hit; do not
  use it.
- **Derived obligations of any-depth** (the real conformance cost): Put
  probes every level before writing; Walk reports each ID once; Remove
  deletes at every level; on a verification failure at the deepest copy,
  keep climbing and return the first copy that verifies (duplicates are
  legal and are not identical by construction).
- **Put:** temp file in the store root (dot-prefixed, shorter than 90
  characters), then lookup; present at any level is success; otherwise
  rename into the deepest existing directory. Any rename failure, including
  a Windows sharing violation, re-runs the lookup.
- **Split** is a writer-local optimization, default 4,096 ID-named files
  per directory in Go; writers that never split (imgbin, Swift, C, a shell
  script) conform. Split moves only names that parse as C4 IDs. A flat root
  may split directly into `c4/xx` (valid under any-depth; avoids the
  measured double move of every object through `c4/`).
- **Walk** enumerates every directory regardless of spelling (see attack
  finding 1) and ignores names that are not 90-character IDs.
- **Non-ID names in the root** (`journal`, `log.c4m`, temp files) are
  reserved, never moved, never returned.
- Readers never infer filesystem case sensitivity (ext4 casefold, ZFS
  mixed, NTFS per-directory make it unsound).
- Fold to lowercase only: base58 has no `I`, so lowercasing is locale-proof;
  uppercasing `i` is not (Turkish, Azeri).

## Why this and not a fixed prefix (measured)

| Layout | Existing readers | 1,000 objects | 10,000 | 10^6 | 10^9 |
|---|---|---|---|---|---|
| Lowercased adaptive trie (this rule) | 1.0.18 everywhere; master, c4py, imgbin on a Mac | flat | 183 dirs | 141k dirs, mean 7 | 75M dirs, mean 14 |
| `toLower(id[0:4])`, one level | none | 182 dirs, 5.6 each | 182 | 182, mean 5,495 | 182, mean 5.5M (past ext4 htree) |
| `toLower(id[0:5])` | none | 914 dirs of 1.1 | 4,745 of 2.1 | 6,370, mean 157 | mean 157k |
| `toLower(id[0:6])` or two fixed levels | none | 997 dirs of 1.0 | 9,721 of 1.0 | 210k, mean 4.8 | 223k, mean 4,485 |

At 50,000 objects every layout looks alike for lookups (3–4 µs hit); the
six-character layouts are 25x slower to enumerate and 1.7x slower to
create. With a fixed layout every existing writer silently writes a second
copy at the store root (run) and migration moves every object; with the
trie a Mac store migrates by 120 case-only directory renames (0.05 s on a
4,201-object copy, all sampled objects readable afterward by 1.0.18 and
master).

**Ceilings.** The trie has none of its own: depth 4–5 at 10^15 objects,
6.5 stats per hit. The filesystem is the ceiling: ext4 and NTFS about
4.29×10^9 files per volume; XFS 2^64; ZFS unlimited; APFS 2^63. Somewhere
between 10^8 and 10^9 objects a single-volume directory store stops being
sensible (file cap, inode budget, tens of millions of directories to walk).
Partitioning then uses the same rule one level up: a routing table from
level-1 (182 bins) or level-2 (222,950 bins) prefixes to volumes, buckets or
stores, computed from the ID alone. That is new code in `MultiStore`
(ordered search today), not a format change.

**The real scale defect is the split shape, not the ceiling.** A 4,096
threshold with a 1,225-way lowercased fan-out drops a leaf to about 3.3
objects per directory the moment it splits: tidy up to about 600k objects,
then 141k–223k directories averaging 7–45 between 10^6 and 10^7, recurring
at every deeper split. One real split of a 4,097-object leaf produced 1,131
directories (run). Under a 2-character step no FAT32-safe threshold
(≤ 8,191) yields more than 6.7 objects per leaf after a deep split; 65,536
gives 53 and keeps 10^6 and up near the sane band, but breaks FAT32. So
FAT32 support and tidy occupancy above ~600k objects are mutually
exclusive under this step; 1-character levels would fix the shape but are a
clean break for every store and reader.

## What the adversaries found (all "needs-changes", none refuted)

1. **A Walk that filters on lowercase names hides 79% of the real store**
   (153,384 of 194,139 objects sit under directories spelled `4E`). Walk
   must enumerate every directory regardless of spelling.
2. **Legacy uppercase directories absorb new writes forever on a Mac**:
   `mkdir c4/4e` succeeds against an existing `4E`, so a conforming writer
   lands objects under the old spelling and transport carries it to Linux
   (run). The writer must confirm the on-disk spelling after mkdir-exists
   and rename a case variant in place, or normalization must precede use.
3. **rsync `--delete-after` / `--delete-delay` between a normalized and a
   not-yet-normalized replica deletes the whole shard, exit 0** (run; plain
   `--delete` is correct). Normalization never propagates by cp, ditto,
   tar or rsync; it must run on every replica, and delete-after is
   forbidden until both sides are normalized.
4. **The drafted Linux normalize, run on a case-insensitive volume, deletes
   the only copy**: `4E/<f>` and `4e/<f>` are the same inode, the
   duplicate check passes, the prune removes the file (run). Normalize must
   test `os.SameFile` before treating anything as a duplicate.
5. **Both shipped writers move reserved root files on a root split**
   (`journal` → `jo/journal`, `log.c4m` → `lo/log.c4m`) and count them
   toward the threshold (run with 1.0.18 and master). Normalize must restore
   any non-ID file found at `<store>/xx/<name>`.
6. **Any pre-rule writer that triggers a split in a lowercased store
   recreates exact-case names one level down** (run: 932 of 1,131 new
   directories non-lowercase). Linux stores must not be normalized until
   every writer that touches them lowercases: a 1.0.19 for the Go line, and
   pins bumped in c4git (pins v1.0.16) and c4sh.
7. **Mixed-depth merges hide objects from every shipped reader** and every
   re-store duplicates one level deeper (run). The rule fixes readers; old
   readers remain exposed until retired.
8. **There is no cross-process lock**: `TreeStore` uses an in-process
   mutex only, so a concurrent split and Put can leave an object one level
   above a directory created concurrently. Under any-depth that state is
   valid; the writer text must not claim a cross-process lock.
9. **The `c4/` level double-moves every object at the first split** (run:
   4,096 objects into `c4/`, then all of them again into 182 leaves on the
   next Put). Free fix under any-depth: split a flat root straight into
   `c4/xx`.
10. Smaller: the fold fallback must be Go-1.0.x-only, not part of the shared
    reader, and retired by a verifiable condition (normalize reports zero
    non-lowercase directories), not a time box; case-sensitive migration
    must recurse; Walk trusts names, so only re-hashing is authoritative
    against name-folding transports (Samba `preserve case = no`).

**Survived every attack:** 2-character levels at every depth (no Windows
reserved name is 2 characters); ASCII lowercase directory names with
exact-case file names; the `c4/` level kept; any-depth validity with a
deepest-first reader; threshold outside the format; no marker file; flat
exact-case S3 keys; rejection of fixed-prefix and 1-character layouts.

## Migration (rename-only, never copies bytes)

- One verb, `c4 store normalize` (dry run by default, `--apply` to act,
  stderr narration, stdout data): Mac mixed-case trie → case-only
  directory renames; Linux exact-case trie → recursive merge of `4E/` into
  `4e/` with the SameFile guard; c4ts (`c4/<88 chars>`) → rename to
  `c4/c4<88>`; c4d (`<root>/<id[3:5]>/<ID>`) → rename to `<root>/<ID>`;
  moved root files (`jo/journal`, `lo/log.c4m`) restored. Mixed-depth
  stores need no migration at all.
- Never required: re-hashing, copying bytes, a format marker, splitting,
  changing S3 keys, taking the store offline.
- Rollout order is a requirement: (1) a 1.0.19 whose writer lowercases and
  whose reader is deepest-first any-depth; (2) pins bumped in c4git, c4sh,
  c4-mcp, c4snap, and ports in c4py and imgbin; (3) only then `normalize
  --apply` on case-sensitive volumes. Mac stores can be normalized at (1).

## Cost

About two lines per trie implementation to lowercase (Go `path()` and
`split()`; c4py `store.py`; imgbin `Store.swift`), plus the any-depth loop
in each reader, plus one shared set of ID-to-path conformance vectors
(13 drafted, including IDs differing only by case in the shard characters,
digit-only and uppercase-only segments, the rare third character `6`, an
object at a shallower level, duplicates at two levels, non-ID root names).
c4ts needs a rewrite regardless; `store/sharded.go` and the flat `Folder`
type are deleted (`Folder` is depth zero of the one rule).

## Decisions for Joshua

1. Adopt the two-sentence rule (lowercased adaptive 2-character trie,
   any-depth validity), with the nine required changes above folded into
   the reference implementation.
2. Which release carries it: v1.1 only, or also a 1.0.19 so that Linux
   stores can be normalized while 1.0.x tools still write to them.
3. Whether normalizing is an explicit `normalize --apply` (recommended) or
   automatic on first write by a new tool.
4. Whether FAT32/exFAT remain targets. If not, the Go default threshold can
   rise (65,536 gives a far better shape above 10^6) with no format
   consequence.
5. Whether the 10^6–10^7 band (141k–223k directories averaging 7–45;
   lookups unchanged, enumeration ~25x slower) is acceptable, or whether
   1-character levels deserve a study as a future writer option.
6. Walk semantics for duplicates: report each ID once (proposed) versus
   exposing duplicates to a tidy verb.
