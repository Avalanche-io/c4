# One command: is everything a claim names in the store?

Status: design, 2026-11-10, for Joshua. Nothing built. From the storage
project's review (item 12): "a one-command check that everything a claim
names is in the store, content as well as listings; `cat -r` covers
listings."

## What exists

- `c4 cat -r <ID>` expands every directory listing through the store,
  verifies each listing's bytes against its ID, names every listing the
  store lacks on stderr as `partial: <path> listing <ID> <why>`, and exits
  2 (master since #71). It says nothing about file content: a file entry's
  C4 ID is printed, never looked up.
- `c4 restore <ID> <dir>` would discover missing content, but only by
  trying to materialise, and `restore` is a tree writer.
- `c4 explain` narrates plans; it does not probe the store.
- `store.Has(id)` is the presence test the planner already uses.

So a claim whose listings are all present but whose content is partly
missing passes `cat -r` with exit 0 today. That is the gap the storage
project hit: a head could name content that was never stored, and nothing
said so short of a restore.

## The shape

No new verb. `c4 cat -r` already walks exactly the set a claim names,
already distinguishes listings present from listings missing, and already
has the partial-scan exit code. One flag extends the same walk to content:

```
c4 cat -r --content <ID>
```

- stdout: unchanged, the expanded tree (data).
- stderr: one line per file entry whose C4 ID the store does not hold,
  `partial: <path> content <ID> not in store`, in tree order, after any
  listing lines.
- exit: 0 when every listing and every file ID is present; 2 when any is
  missing; 1 on a verification failure as today.
- Presence is `store.Has` (name present), not a rehash of every object: a
  completeness check asks "is it here", and rehashing a terabyte to answer
  it is a different command (`c4 cat <ID> >/dev/null` per object, or the
  gc walk's verified read). The flag name says what it checks and the help
  text says it does not verify bytes.
- Null-ID entries (unreadable at scan time) are reported as they are
  today by the partial rule: `partial: <path> recorded with nulls`; they
  are not "missing content", they are content nobody claimed.

Script form: `c4 cat -r --content "$ID" >/dev/null; echo $?`.

## Why this and not a verb

Twelve verbs is the budget (`ARCHITECTURAL_CATALOG.md`). A `c4 verify`
verb would have to decide what verification means (presence, bytes,
reachability, signatures) and would duplicate `cat -r`'s walk. The flag
adds one predicate to a walk that exists, and the output grammar
(`partial:` lines, exit 2) is the one scripts already parse from `c4 id`
and `cat -r`. Cost: one flag in Go; the ports' `cat` equivalents, where
they exist, can add the same flag against the same vector.

## Open for Joshua

- The flag name: `--content`, or `--complete` (checks listings and content
  both; listings are always checked so the shorter name says less).
- Whether `--content` should also accept a path argument (`c4 cat -r
  --content ID/sub`) to check a subtree; the walk already descends paths.
- Whether presence should be checked across every configured store
  (`C4_STORE` lists several; `Has` on a MultiStore already checks all in
  order) or only the first. Recommendation: all, since that is where a
  pull would look.
