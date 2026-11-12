# A c4m file never ends with its own ID

Status: ruled by Joshua 2026-11-08; implemented on branch
`no-trailing-id`, not landed. Companion to `no-self-id.md` (2026-11-03),
which removed the trailing root-ID line from `c4 id`'s own output and
stated the principle; this note turns the principle into a format rule
and migrates the files already written.

## The ruling

Joshua, 2026-11-08: "A c4 id at the end of a file is no different than
a generic data file that happens to contain c4 ids. The system should
not interact with this. If we are talking about a c4id at the end of a
c4m file, then this is an error in format."

So: a stream ends on an entry line. A bare C4 ID appears in exactly two
places, the first-line external base reference and a checkpoint before a
patch section. A bare C4 ID at end of input is `ErrTrailingID`.

## What this corrects

The 2026-07-13 erratum (snapshot-loop round 3) made a bare ID at EOF the
"closing validator": legal, and verified against the resolved manifest.
It was written because shipped tools emitted that shape. The tool that
emitted it was `c4 diff`: from 1.0.13 through 1.0.26 its output was
`<old ID>`, the entries, `<new ID>`, and the restore dry run printed the
same. Appending a diff to a file (`c4 diff FILE.c4m DIR >> FILE.c4m`) is
the documented way to grow a chain, so every chain grown that way ends
with the ID of the state it resolves to.

Two things were wrong with the erratum, measured on 2026-11-08:

- **It never shipped as a working rule.** The 1.0.24 decoder rejects a
  trailing bare ID as `empty patch section (at end of input)`, so 1.0.24
  cannot read the chains its own `c4 diff >>` writes. Only master (1.1,
  unreleased) accepted them. The released line has always refused this
  shape; the change makes the refusal say why and how to fix it.
- **It contradicts the byte identity of a c4m file.** A c4m file's ID is
  the ID of its bytes, and that ID must summon exactly that file. A file
  whose last line is the ID of its resolved state is a different file
  from the one that state's ID names, and nothing can ever close that
  gap (`no-self-id.md`). My earlier claim in the handoff that "no verb
  emits a closing validator" was wrong: `c4 diff` and the restore dry run
  did, and this note corrects that record.

## The change

Producers:

- `c4 diff` prints the old state's ID and the entries, nothing after.
  The new state's ID is `c4 id -q` of the result, and was never needed
  inside the file: the chain resolves to it.
- The restore dry run prints the destination's current ID and the plan
  entries, nothing after. The plan is a valid chain that resolves to the
  target (pinned by `TestRestoreRecoverFromStoreAlone`).
- `c4m.WriteChainBody` is the one stream writer; `WriteChainStream`,
  which appended the root ID, is deleted (unreleased API, no callers).

Readers:

- The resolving `Decoder` returns `ErrTrailingID` at EOF after a bare ID
  that is not the first-line base reference. The checkpoint is still
  verified first, so a *wrong* trailing ID is `ErrPatchIDMismatch`, as
  the `bad-validator.c4m` vector pins.
- `DecodePatchChain`, the shape-level reader behind `patch`, `log`,
  `split` and `explain`, returns the same error. It used to drop the
  line silently.
- The error text names the fix: delete the line, or rewrite with
  `c4 id FILE.c4m > fixed.c4m`.

Migration:

- `c4 id FILE.c4m` (the normaliser) is the migration, via
  `Decoder.AllowTrailingID()`: it verifies the trailing ID as a
  checkpoint, drops it, reports `trailing root ID removed` on stderr and
  writes the valid form. `DroppedTrailingID()` reports what happened.
  `c4 id -l` lists such a file as not normal (exit 1) and `-d` shows the
  deletion, so a hook finds them.
- `c4m/testdata/legacy/diff-chain-1.0.24.c4m` is a real file written by
  the installed 1.0.24 (`c4 id t > chain.c4m; c4 diff chain.c4m t/ >>
  chain.c4m`), byte-pinned, and the CLI tests prove it is refused with
  the hint, listed by `-l`, and migrated by the normaliser to a file
  that resolves to the ID the dropped line named.

Vectors and text:

- `chain-vector/vector.c4m`, `chain-vector/bad-checkpoint.c4m`,
  `deepen-vector/deepen.c4m` and `deepen-vector/remove.c4m` lose their
  final line. Their `*-root-id.txt` values are unchanged, because the
  line never contributed to the state. `bad-validator.c4m` keeps its
  wrong trailing ID and now pins both rejections. The READMEs record
  the edit as the one exception to "never regenerate".
- SPECIFICATION.md "The Closing Validator" becomes "No Closing Validator";
  C4M-STANDARD.md §10.5 and §10.7 are amended, the §10.7 NOTE that
  RECOMMENDED a closing ID now forbids it, Appendix C.1 is rewritten,
  and `ErrTrailingID` joins both error tables.

## What else breaks (the cost)

Nothing in c4 core's released line changes behaviour for a valid file.
The cost is in the suite and in files already written:

1. **Every other implementation** (c4py, c4ts, c4m-swift, libc4, c4sh's
   vendored decoder) carries a verifying closing-validator rule and a
   copy of the four vectors with the trailing line. Each needs: the
   vector edit, `ErrTrailingID`, the test inversion, and a normaliser
   path if it has one. All five already failed the deepen vector on
   2026-11-03 and were being ported; this rides the same wave.
2. **Chains written by 1.0.13 through 1.0.26 with `c4 diff >>`.** They
   were never readable by the released decoder (it said "empty patch
   section"), so no working flow depends on them; but any that exist on
   disk now get a precise error and a one-command fix instead of a
   puzzling one.
3. **Scripts that read the last line of `c4 diff` output** as the new
   ID. The replacement is `c4 id -q` of the new side.
4. **Docs and site.** `README.md`, `docs/getting-started.md`,
   `docs/cli-reference.md` said "the final line is the ID" (already
   stale since `no-self-id.md`); fixed here. The Avalanche website and
   the c4-operator docs may carry the same sentence; not checked from
   this branch.
5. **`stripValidator` in `cmd/c4/treehelpers_test.go`** is now the
   identity on every stream and could be deleted; left in place to
   keep older tests readable.
6. **1.0.x.** The release line's `c4 diff` still prints the trailing ID
   and its decoder still refuses it. Backporting only the producer
   change (stop printing it) is safe and makes 1.0.x's `diff >>` chains
   readable by 1.0.x for the first time; the decoder error text can
   follow. Not done on this branch.

## What this makes possible

With the rule in place, "the last line of a c4m file is an entry" is a
property every tool can rely on: `tail -1` of a chain is the newest
restated entry, concatenating two chains never produces consecutive
boundaries by accident, and a c4m file's byte ID and its description ID
are the two IDs of `two-ids.md` with no third shape in between.
