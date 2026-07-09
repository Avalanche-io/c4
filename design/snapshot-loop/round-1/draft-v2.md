# c4 snapshot–store–restore — Draft v2 (round-1 fold)

## Round-1 fold summary

Four blind personas (cold developer, 15-year git user, autonomous agent, skeptical HN skimmer) ran the five success tasks cold: **20/20 succeeded**. Zero round-1 prosecutor blocking counts remain unresolved. Nine distinct novel friction events surfaced; every one is folded below or accepted into the ledger. Two are design changes; the rest are exactness fixes:

1. **ID/path store addressing** (design change; friction: "no find-by-path primitive; grep-and-hope lookup"). Store-reading verbs accept `<ID>/relative/path`, descending stored listings by exact entry name, one hash-verified level at a time. Zero new flags, zero new verbs — thin sugar over the documented listing walk.
2. **Journal-line durability equivalence + pinned repair** (design change; friction: "'trust me, it self-heals' with zero specifics"). A complete journal line found after a crash names a fully durable snapshot, because everything it references was barriered before the line was appended; a torn tail is the residue of an ingest that never claimed success; repair = truncate the trailing partial section under the append lock. Worst case stated.
3. Exactness fixes: `id -s` side effects enumerated (store only, nothing else, ever); `-q` framed as output form, never identity selection; restore verification defined per knowledge level and "platform-default metadata" defined; the timing model reconciled against its own transcript (durability overhead vs hash-bound total); the SHA-512 axiom stated once; progress narration on stderr; "the journal is the reflog" stated where a git user looks for refs.

## Verdict shape

Two properties carry the design, the first now readable from both sides:

1. **The print barrier.** A snapshot ID reaches stdout only after everything ingest stored — file bytes, listings, and the journal entry — is on stable media. Printed means durable; unprinted means nothing was claimed: the source tree was never written to, the store's prior contents are intact, and rerun is idempotent (dedup skips existing objects). New in v2: the claim is verifiable from the store side too — **any complete journal line found after a crash names a fully durable snapshot** (its objects were barriered before the append began); a torn tail can only belong to an ingest that never printed.
2. **The journal.** The store names its own roots. Every `-s` ingest (snapshots, stdin blobs, every `restore --force` pre-image, including empty ones) appends one section to `<store>/log.c4m`, an ordinary c4m patch chain. `c4 log` with no arguments lists it. This rescues the complaint-#2 user whose scrollback is gone and closes complaint #1's deepest form ("the ID itself is also losable"). There is deliberately **no ref namespace**: handles are IDs; the journal is the reflog.

## 1. Identity — one mechanism, two named knowledge levels

There is exactly one naming rule (frozen): the C4 ID of canonical bytes; for a directory, the bytes are the one-level canonical listing of its direct children. The design pins which observation fields populate the listing — a semantics choice the frozen grammar's first-class nulls already permit — and names the two levels users need:

- **content ID** — listing with mode and timestamp null; names, sizes, symlink targets, and child *content* IDs real. Equal iff contents are byte-identical, on any machine, clock, or umask. **The default level for `c4 id`.** Answers T3 by string equality.
- **snapshot ID** — listing with everything observed; directory entries carry child *full-listing* IDs, so every ID in a full listing is line-locally recomputable from the object it names. Printed by `c4 id -s`; the recovery handle.

Directory size field = total bytes of contained files in both forms (deterministic; never st_size). A single file's snapshot ID and content ID coincide (its bytes). Flatten rule: a description's own canonical-text ID equals its root ID at its own knowledge level — so the pipe law (`c4 id DIR | c4 id -q -` prints DIR's content ID) and snapshot-ID recomputability (`c4 id -q saved.c4m`; `c4 cat -r SNAP | c4 id -q -`) are the *same* rule.

v2 pins from sim friction:

- **`-q` is output form, never identity selection.** What is identified is chosen by `-m` (default: content). `c4 id -q dir` is machine-independent because content is the default level, not because of `-q`; `-q` only suppresses the c4m text in favor of the bare ID. Stated in help and man page — kills the git-user inversion ("I must opt into the stable hash via a flag").
- **The hash axiom, stated once.** ID equality is SHA-512 equality: distinct byte streams share an ID only by a SHA-512 collision — the axiom every content-addressed claim in c4 rests on, stated once rather than hedged per line. Reads never extend that trust: every object fetched from the store is rehashed against the requested ID. (Ledger-accepted tradeoff.)

Dispatch pinned, no sniffing (unchanged from v1): a top-level argument ending in `.c4m`, and `-` on `id`, parse as descriptions (unparseable `.c4m` = error, never silently hashed); every other file argument is bytes; **files inside a scanned tree are always bytes** — never parsed, canonicalized, or re-ID'd. Bare `c4` with piped stdin identifies raw bytes. Patch chains resolve to their final state before identification (`patch -n` reaches earlier states).

## 2. Self-description — the store alone recovers everything, and is addressable

`c4 id -s DIR` is a total ingest: file bytes; per-directory full-form listings (the snapshot graph, root included); per-directory content-form listings (so content IDs also resolve via `cat`/`restore`); one journal section. Listings are tiny text objects; both graphs dedup across snapshots (unchanged subtrees reuse objects — O(changes) history with no history feature). There is no separate description blob (rejected in v1: second object kind, key-collision hazard; under the flatten unification the description *is* the root listing). Ingesting a description (`c4 id -s x.c4m`, `c4 patch chain | c4 id -s -`) stores its listings; referenced content is not thereby stored — partial knowledge is first-class, and `restore` pre-flights the closure.

v2 pins from sim friction:

- **Side effects enumerated.** `id -s` writes only inside the store directory: content objects, listing objects, one journal section. It creates no files anywhere else — no `.c4m` file, nothing in the scanned tree, nothing in the working directory, ever. stdout is the ID; stderr is narration. (Two personas' worst confusion was doubt about exactly this; one sentence on the surface ends it.)
- **ID/path addressing.** Anywhere a store-resolved ID is accepted (`cat`, `restore` targets, `diff` sides), the ID may be followed by a slash-separated relative path: `c4 cat SNAP/src/parser.go`. Each component descends one level by exact entry-name match in the stored listing; every fetched object is verified by rehash; the final component may name a file (bytes) or a directory (its listing). Works at either knowledge level. This is sugar over the documented walk (cat the listing → read the entry's ID → cat that ID) and replaces the fragile `cat -r | grep ' name '` idiom as the documented lookup (the raw composition remains valid). Unknown component ⇒ exit 1, missing name on stderr. Not a third identity concept: it is an addressing form that resolves to an entry's existing ID. Edge semantics (symlink components, trailing slash, `.c4m`-file targets) are round-2 items.
- **Progress narration.** stderr carries progress: a live file/byte count on a terminal, a single summary line otherwise. stdout stays byte-pure (the ID and nothing else).

Journal semantics: **history only** (catalog resolution stays rejected — basename collisions handed users the wrong pre-image). Entry format unchanged: `- <UTC time> <listing size> <name>.c4m <- <host:abs-path> <snapshot ID>` plus the chain boundary. Discovery is `c4 log` (no-arg default), which lists every section, evicting nothing.

## 3. Durability — printed means durable; journaled means durable; measured, not aspirational

Protocol (unchanged): objects land temp-file + fsync(2) + atomic rename (~126 µs/file; presence under a final name implies integrity, keeping dedup's Has() valid after a crash); one device flush barrier; journal append + fsync; second barrier; then the ID prints and exit 0. Per-file F_FULLFSYNC stays rejected (measured ~6.4 ms/file, ~3 min per 20k files, buys nothing over the claim-point barrier). `--no-fsync` stays deleted everywhere.

v2 pins from sim friction:

- **Arithmetic reconciled on the surface.** "Overhead" is durability overhead *beyond reading and hashing the data, which dominate*: ~130 µs/file plus two ~5 ms barriers. The transcript's run reconciles explicitly: 20,312 files / 4.8 GB ⇒ ~3.5 s to read and hash + ~2.6 s durability overhead ≈ 6.1 s total. One model, one transcript, consistent — the HN sim's "numbers contradict by 2×" dissolves because the man page now states what the constant excludes and shows the sum.
- **Constants are measurements, the contract is ordering.** Figures are from one machine (Apple-silicon APFS SSD); other filesystems change the constants, never the ordering contract: synced before renamed; barriered before journaled; journaled before printed. Linux supplies the equivalent (fsync of files and containing directories plus a device flush).
- **Journal-line equivalence.** If power fails between the journal barrier and the print, whatever `c4 log` shows after reboot is real: a complete journal line names a fully durable snapshot, verifiable in one pipe (`c4 cat -r ID | c4 id -q -`); the command merely never claimed it.
- **Torn-tail repair pinned.** Writers hold the advisory lock and truncate any trailing partial section before appending; readers parse complete sections and warn once on stderr about a tail. Worst case: if no append ever follows, the tail persists harmlessly and every complete section stays readable — a torn tail can only be the residue of an ingest that never printed, so nothing claimed is ever lost. Objects staged by an unclaimed ingest are unrooted: `gc` dry-run lists them; rerunning the ingest claims them.

Materialization keeps the strong promise: restored files are written temp+fsync+rename; the target ID prints only after the tree is verified by recomputation at the target's knowledge level and one barrier completes. Between restore's two output lines, the tree may hold a mix of old and new whole files — both endpoints are already durable in the store; re-run to finish or restore line 1 to go back.

## 4. Destructive reconcile — one tree-writer, plan by default, pre-image before destruction

`c4 restore <target> <dir>` is the only verb that writes working trees. `patch` is pure text algebra and errors on directory arguments with a message that teaches `restore`. Unchanged from v1: dry-run default with pinned plan output (boundary IDs always present; equal, with no entries, when already matching); `--force` = pre-flight the closure (missing content ⇒ stderr list, exit 1, touch nothing) → full-fidelity pre-image through the total-ingest protocol, durable and journaled **before the first destructive operation**, printed as stdout line 1 → reconcile → verify → target ID as line 2; completeness gate (refuse, listing offenders, when `<dir>` holds entries the pre-image cannot fully capture); no pre-image opt-out (`rm -rf` remains the honest capture-free spelling); fidelity boundary stated on the surface (records names/sizes/modes/mtimes/symlinks/hardlinks/sequences/content IDs — not ownership/xattrs/ACLs/special files); empty-description constant as pre-image of absent/empty dirs (restore never deletes `<dir>` itself); targets are descriptions only (ID — now optionally ID/path — `.c4m`, or `-`); mirroring = `restore --force $(c4 id -s A) B` with the missing-content trap named; `--source` stays rejected (one flag of headroom deliberately preserved); concurrent restores of one directory uncoordinated — store safe, tree outcome undefined (ledger-accepted tradeoff; re-flagged by a sim, stands).

v2 pins from sim friction:

- **Verification defined per knowledge level.** Snapshot-ID target: recorded modes and mtimes are applied explicitly (creation umask is irrelevant), then the full-form listing graph is recomputed from the resulting tree and must reproduce the snapshot ID. Content-ID target: the content-form graph is recomputed and must reproduce the content ID — names, sizes, symlink targets, and bytes are verified; modes and times are **platform-default metadata**, defined as whatever file creation under your umask and clock produced — not recorded by the target, not claimed, not verified. Line 2 prints only if recomputation reproduces the target ID exactly.
- **No ref namespace, said where git users look for it.** The undo handle is the pre-image ID itself: stdout line 1, echoed in the stderr undo hint as a complete command, and recorded permanently in the journal — `c4 log` is the reflog. Nothing to update, nothing to lose.

**GC coherence** (unchanged): default roots = every ID appearing in any journal section — full history, both listing graphs, and their closures; explicit c4m arguments add roots, never replace. Nothing journaled is ever collectable until the journal is explicitly truncated (`split`, then `gc --force`). v2 adds: unjournaled objects from an unclaimed ingest are unrooted by construction — the dry run names them. Retention policy remains a non-goal.

## Surface accounting (honest)

Within budget: **2 identity concepts** (content ID, snapshot ID); **1 new verb** (`restore`); **1 new flag** (`--force` on restore; 1 of 2 spent). Ledgered non-flag surface from v1: mode value `c` on existing `-m` (new default); `id -q`/`id -s` output redefinitions (breaking, pre-freeze, deliberate); bare `c4 <path>` read-only; stdin no longer auto-stores; one well-known file `<store>/log.c4m`; no-arg default on `log`. **New ledgered non-flag surface in v2:** ID/path addressing on store-resolved IDs (an addressing form, not an identity concept); stderr progress narration (non-contractual). Deletions unchanged: `id -x`; `patch -s/-r/-q/--dry-run/--source/--no-store/--no-fsync/-m` and all directory forms; `diff -r/-s/-q` — net flag count still falls by roughly nine. 0 grammar changes; 0 dependencies; stdout byte-pure everywhere.

## Migration notes (design-level, unchanged)

Legacy stat-bearing listings remain valid full-form listing objects that verify as the hash of their own text; nothing dangles. Content IDs re-derive in one command (`c4 id -q`). Mixed-version fleets answer T3 only when both ends run the suite that ships content-default IDs — governed by the coordinated suite release (RELEASE.md); this identity default is the launch gate.

## Task closure (v2)

- T1: `SNAP=$(c4 id -s proj/)` — writes nothing outside the store, so "files produced outside the store" is the empty set *by construction, now stated on the surface*; `rm -rf proj/`; a lost ID is re-supplied by `c4 log`; `c4 restore --force SNAP proj/` rebuilds byte-exact with metadata, verified before line 2 prints.
- T2: the accident's own stdout line 1 (or `c4 log`) names the victim's pre-image; one `restore --force` returns it byte-exact; the undo prints its own line 1, so it is itself undoable.
- T3: `c4 id -q dir` on each machine; string-equal iff byte-identical (content is the default level; `-q` merely quiets output).
- T4: ~6 s for 20k files / 4.8 GB (~3.5 s read+hash + ~2.6 s durability). Printed ⇒ snapshot, content, and journal entry survive power loss. Journal line present but unprinted ⇒ snapshot durable and discoverable via `c4 log`. Otherwise ⇒ nothing claimed, tree untouched, store consistent, rerun resumes.
- T5: `c4 cat SNAP` lists one level; `c4 cat SNAP/src/parser.go > parser.go` extracts by name (verified at every level); `cat -r | grep` remains the raw composition; `c4 cat -r SNAP | c4 id -q -` falsifies or confirms in one pipe.

## Open (for round 2)

1. ID/path edge semantics: a component that names a symlink or file mid-path; trailing-slash form; whether `.c4m`-file targets may take `/path`.
2. Journal entry naming for stdin blobs (`| c4 -s`): what fills the `<name>.c4m` and origin fields.
3. stderr progress line format and TTY detection heuristics (cosmetic; narration is non-contractual).
4. Exact `c4 log` column layout (cosmetic; survived sims unchanged).
5. Journal semantics for non-local (s3/remote) stores — networking is a non-goal; this surface claims local stores only.
6. Retention recipe concurrency (stop writers during `split`; gc holds the append lock while rooting) — implementation detail under a declared non-goal.
7. Fidelity sidecar for xattrs/ownership/ACLs — future external structure per the sidecar doctrine.
