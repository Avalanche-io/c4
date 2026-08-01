# Chain resolution — three defects and their requirements

Status: requirements (2026-07-31). Raised while designing a background
snapshot service for a 12.46 TB / 10,436,110-entry external volume, whose
scan must span days, reboots, and unplug events. Joshua's framing:

> The c4 philosophy is that partial information is valid. So the question
> of crash recovery should really be a question of how much information /
> time should pass before we commit a block of updated information. […]
> C4 has a base reference block plus streaming updates design that we
> should leverage for exactly this optimal, safe, scanning pattern.

**The premise holds.** The grammar already expresses the pattern and needs
no change. Draft v9 §4 ratified all four boundary rules, and a partial
scan output is already a valid reuse guide (verified below). What is
missing is *resolution*: two of the ratified rules are unimplemented or
mis-implemented, and the resolver's cost model makes the pattern
unaffordable at the scale that motivates it.

These three defects stand on their own merits, independent of any
consumer. Per the review method they are a subtraction and ship
separately from the addition that surfaced them.

---

## 1. What was verified working

Reported as carefully as the defects, so none of it is churned:

- **A truncated c4m is a valid reuse guide.** Cutting a scan's streamed
  output to three lines and re-running with `-c partial.c4m` reused what
  the fragment recorded and re-hashed the rest (`reuse: 2 reused, 1
  rehashed`). Crash resumption needs no new format, no new flag, and no
  sidecar. This is the single most load-bearing property of the whole
  design and it already works.
- **Store-backed directory records already give breadth-first
  navigation.** `c4 cat <id>` returns one level; `c4 cat -r <id>`
  backfills the tree from records. Top-down navigability is not a feature
  to add — it is a consequence of directory identity being the ID of the
  one-level canonical listing.
- **The journal is already the handle.** `c4 log | tail -1 | awk '{print
  $NF}'` yields the latest claim ID, fsynced before the claim prints.
  Nothing further is needed to name "the current root".
- **`WithReuseGuide`'s racy rule is sound** and needs no change here.

---

## 2. Defect A — an external base reference is silently dropped

`c4m/chain.go:59-61` parses a first-line bare ID into
`PatchSection.BaseID`, honoring draft v9 §4 rule 1 ("FIRST LINE bare ID:
external base reference"). Nothing then consumes it.
`ResolvePatchChain` (`c4m/chain.go:114`) builds the base from
`sections[0].Entries` and never reads `sections[0].BaseID`. The only
other reference in the tree is `cmd/c4/split.go:79`, which copies it
through.

Reproduction — `block1.c4m` stored, `block2.c4m` naming it as base:

```
block2.c4m:  c44Cz1ki…                       ← block 1, present in the store
             -rw-r--r-- … g5 c42yB5s4…

$ c4 patch chain/block2.c4m
-rw-r--r-- … g5 c42yB5s4…                    ← block 1's `x/` directory is gone
$ echo $?
0
```

The base's contents vanish with no diagnostic and a success exit. This is
the silent-false-miss class: a consumer cannot distinguish "the base was
empty" from "the base was never fetched".

### Requirements

- **A1.** A chain whose first section carries a `BaseID` MUST resolve
  that base before applying the section, or fail loudly. Silent omission
  is not an acceptable outcome under any configuration.
- **A2.** Resolution is by content lookup. Because `id` never reads the
  store, base fetching MUST NOT be wired into `c4 id`; it belongs to the
  verbs that already read the store (`cat`, `patch`, `restore`) and to an
  explicit library entry point that takes a source.
- **A3.** When no source is available, or the base ID is absent from it,
  the result MUST be an error naming the missing ID — never a partial
  manifest presented as complete.
- **A4.** Chains nest: a fetched base may itself carry a `BaseID`.
  Resolution MUST backfill transitively and MUST detect cycles rather
  than recursing without bound.
- **A5.** Partial knowledge remains first-class, but "I could not read
  the base" is ignorance about the *description*, not a null field within
  it. It is an error, not a null.

---

## 3. Defect B — "deepen a directory" is read as "delete it"

`c4m/operations.go:257`, in `applyPatchTree`:

```go
// Check if exact match (removal)
if entriesIdentical(bNode.entry, pNode.entry) {
    delete(base.children, name)
    continue
}
```

The removal rule (draft v9 §2 rule 3, "restated-identical means removal")
fires without consulting `pNode.children`. A patch section that restates
a directory line unchanged *while declaring children under it* therefore
deletes the directory and the children it just declared.

Reproduction:

```
base:   drwxr-xr-x - - a/ -
        -rw-r--r-- - - f3 -

patch:  drwxr-xr-x - - a/ -        ← restated identically…
          -rw-r--r-- - - f1 -      ← …while declaring a child

$ c4 patch p3.c4m
-rw-r--r-- - - f3 -                ← a/ AND f1 both gone, exit 0
```

Changing any field of the parent line makes it behave correctly, which is
why the bug has stayed hidden: every patch produced by `PatchDiff` mutates
the lines it restates.

**Why this is in the critical path.** Breadth-first structure discovery
learns a directory's *children* before it learns anything about the
directory *itself*. Its natural emission is exactly the shape above:
unchanged parent line, new children. The most obvious way to write a
progressive scanner is the data-losing way, silently, at exit 0.

### Requirements

- **B1.** A restated entry that carries children in the same section is
  NOT a removal. The removal rule MUST NOT fire when `len(pNode.children)
  > 0`.
- **B2.** Removal semantics for a leaf, and for a directory restated with
  no children, are unchanged. This is a narrowing of an over-broad rule,
  not a redefinition.
- **B3.** A directory restated identically *with* children MUST merge
  those children into the existing node, preserving the base's other
  children.
- **B4.** The rule as narrowed MUST be stated in `C4M-STANDARD.md` and
  `SPECIFICATION.md`, and MUST be covered by a shared cross-implementation
  test vector — the removal rule is chain grammar, so c4py, c4ts,
  c4m-swift and libc4 each carry the same latent defect until verified.

---

## 4. Defect C — fold cost is O(base size) per section

`ApplyPatch` (`c4m/operations.go:190`) calls `buildPatchTree(base)`,
allocating a `patchNode` and a `map[string]*patchNode` for **every entry
in the entire base**, then flattens and re-sorts the whole result — once
per patch, regardless of patch size. `ResolvePatchChain`
(`c4m/chain.go:117-120`) calls it in a loop, so resolving K sections over
N entries is O(K·N).

Measured with `chainbench`, synthetic walk-ordered trees, one-entry
patches versus full-restatement patches:

| Base entries | Fold of a 1-entry patch | Fold of a full-restatement patch | µs per base entry |
|-------------:|------------------------:|---------------------------------:|------------------:|
|       45,253 |                 38.2 ms |                            45 ms |              0.84 |
|      174,317 |                146.8 ms |                           167 ms |              0.84 |
|      713,459 |                615.4 ms |                           698 ms |              0.86 |

Linear over a 16× range at ~0.85 µs per base entry per fold, and **a
one-entry patch costs essentially what a patch restating every entry
costs**. Patch content is nearly free; base size is everything.

Extrapolated to the 10,436,110-entry volume that raised this: 8.9 s per
fold. A scan committing a block every few minutes across an ~86-hour pass
produces ~1000 sections, so reading the current state back costs ~2.5
hours. At 10⁹ entries it is ~14 minutes per fold and ~236 hours per
resolution.

This is the same algorithmic shape as the `PropagateMetadata` quadratic
recorded in `scan-memory-and-throughput.md` — per-item work that rescans
the whole collection — in a different function.

### Requirements

- **C1.** `ResolvePatchChain` MUST build the base tree once, apply every
  section into it, and flatten and sort once. Target complexity O(N +
  ΣPᵢ) rather than O(K·N).
- **C2.** Resolution MUST remain byte-identical to the current
  implementation for every existing chain. The intermediate
  flatten/sort/re-parse between sections is an implementation artifact;
  if any test depends on it observably, that dependence is the finding
  and must be recorded, not silently preserved.
- **C3.** `ApplyPatch(base, patch)` keeps its current signature and
  behavior as the two-manifest primitive. C1 is a change to how the chain
  resolver composes folds, not to the meaning of a fold.
- **C4.** `stopAt` semantics are unchanged.
- **C5.** A scaling benchmark MUST land alongside the fix, asserting that
  per-section cost is independent of base size — the same guard
  `BenchmarkPropagateMetadata_Linear` provides for the earlier quadratic.
  A fix without a scaling test will regress.

---

## 5. What this changes for the consumer

With C1, chain *depth* stops multiplying tree *size* on every read.
Committing frequently becomes cheap to read back (~9 s rather than ~2.5
hours at 10.4M entries), and periodic consolidation into a fresh base
becomes an optimization rather than a load-bearing requirement whose
frequency must be tuned against chain depth forever.

Two measurements from the same investigation are recorded here because
they constrain any future scanner design, and belong with the evidence
rather than in a downstream repo:

**A separate structure LOD costs a full extra cold walk.** Crossover on
cold subtrees of the target volume, 150,000 entries each:

| Tree | Mode | Cache state | Time | Rate |
|------|------|-------------|-----:|-----:|
| Storage01 | readdir | cold | 28.79 s | 5,211/s |
| Storage02 | readdir+stat | cold | 35.67 s | 4,208/s |
| Storage01 | readdir+stat | dirs warm from run 1 | 38.54 s | 3,892/s |
| Storage02 | readdir | inodes warm from run 2 | 1.65 s | 90,769/s |

Run 4 sets the ceiling: a walk over cached data is 55× faster. Run 3
gained nothing from run 1 — **readdir warms directory blocks, stat needs
inode data, and warming one does not warm the other.** On one tree, split
costs 67.3 s against 35.7 s fused. Scaled to the full volume: ~78 minutes
split, ~41 minutes fused, to deliver structure ~8 minutes earlier. The
three-level ladder (`ModeStructure` / `ModeMetadata` / `ModeFull`) is a
useful vocabulary for *what a manifest knows*; it is not a schedule for
*how to acquire it*.

**Per-entry canonical text is ~143 bytes**, so each full restatement
layer costs ~1.5 GB of chain text at 10.4M entries and ~143 GB at 10⁹.
Layers are cheap to fold (see the table above) but not cheap to store.

---

## 6. Scope and non-goals

In scope: the three defects, their spec text, and the shared test
vectors. Out of scope: the snapshot service itself, commit-interval
policy, consolidation policy, and any scan-side change — those live with
the consumer. No grammar change. No new dependency. No new verb or flag.

## 7. Found during implementation

Two constraints the acceptance test surfaced, recorded because they bind
any block-emitting consumer:

**Blocks must be stored verbatim.** A block's ID is the handle its
successor names and the resolver opens, so the stored bytes must be the
block's own bytes. `c4 id -s <block>.c4m` is therefore *not* how to
store one: it canonicalizes c4m content before storing, which would
change the bytes and break the chain — and it cannot resolve the base
anyway, since `id` never reads the store. A block writer uses
`store.Put` on the raw bytes. This is a constraint, not a defect: `id`'s
canonicalization and its store-free pin are both correct.

**Deferred checkpoint verification is not yet performed.** `Decode`
skips checkpoint verification for every boundary of a stream carrying an
external base reference (`decoder.go:111`), deferring it "to the
resolver that fetches the base" — but `Decode` discards the checkpoint
IDs, so `ResolveBase` has nothing to verify against. A block closing
with its own resolved root ID therefore has that validator parsed and
ignored. This is a real gap in "verification over assertion": no
incorrect result is produced, but a claim that could be checked is not.
Closing it requires `Decode` to surface deferred checkpoints, which is a
wider change than these three defects and is deliberately not bundled
here.

**`buildPatchTree` panics on malformed input.** `c4m/operations.go:231`
does `parent := stack[e.Depth]` with no bounds check, so an entry whose
depth jumps past its parent — a corrupt, truncated, or hand-edited c4m,
or a producer bug — panics the process instead of returning an error:

```
panic: runtime error: index out of range [1] with length 1
  c4m.buildPatchTree      operations.go:231
  c4m.ResolvePatchChain   chain.go:136
  c4m.ResolveBaseID       base.go:42
```

Found while building the first block-emitting consumer, by emitting a
block whose entries were not walk-ordered. The producer bug is fixed on
that side, but any consumer parsing c4m it did not write has the same
exposure, and a parser that panics on bad input cannot be used on
untrusted data. This is a defect in its own right and is deliberately
NOT bundled into the three fixes above.

## 8. Open items

- Whether B1's narrowing needs an erratum note in the round-3 chain
  grammar record, or is a plain defect fix against already-ratified
  intent. Leaning defect fix: rule 3 was written to describe removal, not
  to forbid deepening.
- Sibling-implementation ports for B and C (c4py, c4ts, c4m-swift,
  libc4). B is a correctness gate; C is performance and may lag.
- Whether `c4 cat -r`'s existing recursive expansion should share the
  cycle detection A4 requires.
