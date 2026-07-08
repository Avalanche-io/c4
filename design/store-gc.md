# c4 gc — Reclaim Unreferenced Store Content

## Summary

`c4 gc` is mark-and-sweep garbage collection for the local content store.
The keep-set roots are the c4m files named on the command line. It is a
dry run by default — it reports what would be deleted and touches nothing.
Deletion requires an explicit `--force`.

## Problem

`c4 id -s` (and the bare `c4 <path>` shortcut) store content at snapshot
time. Nothing ever deletes: a working store only grows. Workflows that
snapshot every session or every step make unbounded growth a blocker.
GC closes the loop: keep the c4m files you care about, collect the rest.

## Usage

```
c4 gc <file.c4m>...            # dry run: report reachable/garbage, delete nothing
c4 gc -v <file.c4m>...         # dry run, also list each garbage ID
c4 gc --force <file.c4m>...    # delete unreferenced objects
```

There is no default root set. GC never guesses what you meant to keep —
a destructive operation gets explicit roots or nothing.

## Reachability

One rule: **an object is reachable if its ID appears in a kept
description, or in any reachable description.**

- **Roots** are the c4m files named on the command line. Every root must
  parse as a c4m file or patch chain; any parse failure aborts the whole
  run with a non-zero exit before the store is examined.
- **Marking is token-based.** Every C4 ID appearing anywhere in a root —
  entry IDs, bare chain/base ID lines, sequence range-data lists — is
  marked. Over-marking retains at worst a few extra objects; under-marking
  destroys data, so the marker is deliberately the bluntest correct tool.
- **The roots mark themselves.** The canonical (and raw) ID of each named
  file is marked, so a store copy of a kept description survives and
  `c4 cat <manifest-id>` keeps working.
- **Closure.** Any marked object whose stored content is itself a c4m
  description — directory records stored by `c4 id -s`, external base
  manifests, chain blocks — is read from the store and its IDs marked in
  turn, until no new IDs appear. Sniffing is heuristic (first bytes look
  like an entry line or a bare C4 ID); a false positive only causes an
  extra read and, at worst, extra marks. Always safe, never lossy.

Consequences:

- **Directory records** survive whenever any kept manifest lists the
  directory: the directory entry's C4 ID is the record's ID.
- **Patch-chain intermediate states** survive when the chain file is kept.
  The chain file explicitly describes those states (`c4 log`, `c4 split`
  can materialize them), and GC keeps everything a kept description can
  reach. To collect intermediates, `c4 split` the chain and keep only the
  part you want, then gc.

## Sweep

The store is enumerated with `store.Walker` (below). Any object whose ID
is not marked is garbage. The sweep only ever considers file names that
parse as C4 IDs — temp files, config files, and stray files are invisible
to it and can never be deleted.

Dry run prints a summary table:

```
objects       20,000   (1,254,887,424 bytes)
reachable        412   (18,733,056 bytes)
garbage       19,588   (1,236,154,368 bytes)

Dry run — nothing deleted. Re-run with --force to delete.
```

`--force` prints the same table, then deletes and reports:

```
deleted 19,588 objects (1,236,154,368 bytes)
```

## Safety

- **Dry run by default.** Deletion happens only with `--force`, which has
  no single-letter form — you type the word.
- **Empty keep-set refuses.** No file arguments is a usage error. Roots
  that parse but contain zero C4 IDs (e.g. structure-mode manifests) abort
  with an error. There is no code path in which "nothing is reachable"
  proceeds to deletion.
- **Parse errors are fatal.** Exit non-zero before touching the store.
- **Idempotent.** A second `--force` run reports zero garbage.
- **Local stores only.** GC operates on the configured local store
  (`C4_STORE` / `~/.c4/config`). S3 stores are out of scope for this verb.
- **No locking.** Objects stored concurrently while gc runs are not
  protected — the same caveat as `git gc`. Run gc when no snapshot is in
  flight. Failed removals warn and continue; any failure exits non-zero.

## Non-goals

- Pruning empty shard directories after deletion (harmless; the trie
  resolves through them correctly).
- Chain truncation or rewriting — that is `c4 split`'s job.
- Remote/S3 collection.

## Store API

One optional interface, one implementation:

```go
// Walker enumerates every object in a store.
type Walker interface {
	Walk(fn func(id c4.ID, size int64) error) error
}
```

`TreeStore` implements it by walking the shard trie and yielding every
regular file whose name parses as a C4 ID. Nothing else in the store
package changes.
