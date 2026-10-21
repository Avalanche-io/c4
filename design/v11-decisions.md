# v1.1 decisions still needing Joshua — the context for each

Written 2026-10-16 after a transcript review (three agents over every c4
session since 2026-07-25, the design directory, and memory). Everything
marked "today" was run on the binaries this morning: released c4 1.0.18
and a build of master `d0eab97`. Nothing here is decided; each section
ends with a recommendation and what the choice costs.

What is already settled and therefore not in this document: the
directory-ID question (resolved 2026-07-25, see `c4m-canonical-storage.md`
and `v11-surface-review.md`), the format-count question (every record
format leaves the core, so the core has one format, c4m), the `-s` claim
line (ruled 2026-10-03), the eight rulings in `ash-brief.md` section 3.

---

## 1. Store bootstrap: what happens the first time someone stores

**Plain words.** A fresh machine has no `~/.c4/store`. The first
`c4 id -s` (or `c4 <path>`) has to put bytes somewhere. Does it create the
store on its own, ask, or refuse until the user sets one up?

**Today (run).** Both 1.0.18 and master print
`No content store configured. Create ~/.c4/store? [Y/n]` and, when stdin
is not a terminal, proceed as if the answer were Y: the store, a
`~/.c4/config`, and (on master) a `journal` appear, and the ID prints. So
the current behavior is a prompt that scripts never see and cannot
decline. In a pipeline that is a silent create with a stray prompt line
on stderr.

**What the philosophy says.** "No interactive prompts in composable
verbs." "Safe by default, fast by flag, destructive by long-form-explicit."
Creating an empty directory under the user's home is not destructive.

**Options.**
- A. Create silently, print one line to stderr the first time
  ("created ~/.c4/store"). Scripts work; nothing is hidden.
- B. Keep the prompt for terminals, create silently otherwise (today's
  behavior, made deliberate).
- C. Refuse until `c4 store init` or `C4_STORE` is set. Teaching error,
  one more verb, every first run fails.

**Recommendation: A.** It removes a concept (the prompt) and matches the
rule that composable verbs never ask. Cost: one stderr line; a user who
did not want a store under their home gets an empty directory they can
delete.

**Costs of the others.** B keeps a prompt in a composable verb and two
behaviors for one command. C adds a verb and a failure to every first
contact with c4, which is the opposite of "discoverability and
usability".

---

## 2. Record headers: what, if anything, about records freezes at v1.1.0

**Plain words.** Besides c4m, master has one more line-oriented text
format: the journal, whose first line is `@c4 journal 1` and whose body
is `<RFC3339> <ID>` per line. The July designs sketched more kinds
(receipt, process, testimony) under the same `@c4 <kind> <version>`
header shape. The re-scope moves all of these out of the core. The
question that remains is whether v1.1.0 promises anything about that
header convention, because a promise is what other implementations and
future readers build against.

**Today (run).** `grep` of master finds exactly one kind in code,
`@c4 journal 1`, in six places. No receipt, process or testimony code
exists. Issue #44 asked what a reader does with an unknown version; no
rule is implemented.

**What is at stake.** If nothing is frozen, each future tool invents its
own header and the "known parseable type" idea (Joshua, 2026-07-25:
"everything in c4 is content, but testimonial content is a known
parseable type") has no shared first line to recognize. If the kinds are
frozen, v1.1 commits to formats that are not in the core and not yet
designed.

**Options.**
- A. Freeze only the convention: a record is a text object whose first
  line is `@c4 <kind> <version>`; a reader that does not know the kind or
  version treats the object as opaque bytes (never an error). Freeze no
  specific kind. The journal keeps its header as an instance of the
  convention, documented in the toolbox layer, not the standard.
- B. Freeze the convention and the journal format.
- C. Freeze nothing; records are a v1.2 matter.

**Recommendation: A.** It is the smallest promise that keeps
recognizability, and the unknown-kind rule is the forward-compatibility
gate (#44) in one sentence. Cost: one paragraph in the spec and a
conformance vector per language ("unknown kind is opaque"). B freezes a
format whose home (toolbox or product) the re-scope has not fixed. C
leaves every later tool free to collide.

---

## 3. The help front door

**Plain words.** `c4 help` on master prints 797 words before a reader
gets to a command. The July review said this is too much to be a front
door and that parts of it repeat each other, but that the SCRIPTING
section (stdout is data, stderr is narration, exit codes, the claim line)
is a contract scripts rely on and must stay.

**Today (run).** master: 797 words. 1.0.18 prints its help to stderr and
is shorter.

**Options.**
- A. Cut duplicated prose, keep SCRIPTING verbatim, target the length of
  `git help` (under 300 words), move the removed material to `c4 explain`
  or per-command help.
- B. Leave it; fix after the re-scope changes the verb set anyway.

**Recommendation: B, with A as a task inside the re-scope.** The verb
set is what the re-scope decides; rewriting the front door before it is
wasted work. Cost of waiting: nothing ships until v1.1 anyway. This
question needs only a yes that it is sequenced that way.

---

## 4. `c4 id` carries many concepts

**Plain words.** On master `c4 id` does: describe a tree; snapshot and
write it (`-s`); stream entries; read a c4m from stdin (`-`); guided
rescan against a prior c4m (`-c`); audit (`--verify`); four detail
levels (`-m s|m|f|c`: structure, metadata, full, content IDs only);
sequence folding (`-S`); exclusions; quiet (`-q`), whose meaning changes
with multiple paths. The July review asked whether one short flag (`-s`)
turning a read verb into a write verb is discoverable, and whether four
detail levels are all load-bearing.

**What was ruled.** Only that every `-s` ends with a claim line (keeps
`-s` under `c4 id`). Your July messages treat the detail levels and
folding as load-bearing ("it is valid to have two different c4m files
for the same frame sequence"), and the canonical-storage design indexes
all three levels. No ruling on splitting the verb.

**Today (run).** 1.0.18 `c4 id -s <file>` prints `stored: <ID>` and the
entry line; master prints only the entry line, so the claim-line ruling
is not yet implemented on master.

**Options.**
- A. Keep `c4 id` as is for v1.1; the re-scope records the verb's
  composition (each flag documented as what it composes) and defers any
  split.
- B. Split writing out: `c4 id` never writes; `c4 store <path>` (or the
  existing bare-path form `c4 <path>`) is the only way bytes enter a
  store. `-s` goes away.
- C. Split by concept: `c4 id`, `c4 scan` (guided rescan), `c4 verify`.

**Recommendation: decide this inside the re-scope document, not now,
but with a lean toward A.** The thing the philosophy objects to is a
read verb that writes; B fixes exactly that and nothing else, and the
bare-path form already exists as the write verb. The cost of B is every
script and doc that uses `-s`, in six languages' worth of README, plus
the claim-line ruling becomes a ruling about `c4 <path>` instead. The
cost of A is living with the objection.

---

## 5. A bare ID handed to someone else does not carry its scan settings

**Plain words.** The July resolution gave each store a map from a tree's
ID to the detail level and folding it was scanned with, so that inside
your own store "do I already have this tree" is answerable at any level.
That map does not travel. If you email someone a directory ID, they
cannot tell from the ID whether it was a full scan, a content-only scan,
or folded, so "compare like with like" depends on both sides having
scanned alike. You parked a "virtual parent" idea for this on
2026-07-25: a c4m one level above the captured tree that records the scan
parameters (and exclusions) as content, so the parent's ID carries them.
You also said the general form is to index every directory at every
level, which makes the virtual root "just another entity ID".

**Options.**
- A. Accept for v1.1: the ID answers "what is this listing", and the
  listing itself shows its level (a content-only listing has no
  metadata; a folded one has range lines). Recipients compare listings,
  not bare IDs, when the level matters. Document it.
- B. Specify the virtual parent in v1.1: a c4m whose entries are the
  captured root plus a scan-parameters line, so one ID carries both.
  This is new grammar or a new record kind.
- C. Defer B to the listings design (v1.2).

**Recommendation: A now, C for the rest.** The listing already carries
its own level, so the residue is only "a 90-character string alone
cannot say how it was made", which is true of every hash and is why the
listing exists. B adds grammar to a frozen format. Cost of A: a
documented limitation; none in code.

---

## 6. Symlinks inside a c4m file

**Plain words.** On 2026-07-29 you said c4m needs "a way within the
standard c4m file to intentionally record a symbolic link", that "both
having an ID and not having an ID should be supported as well as fully
collapsing the symlink", and that "these are issues that need to be
resolved with priority". This is in no design document.

**Today.** The brief's rule for v1.1 is only the symlink-ID rule: a
link's ID is null unless its target is an in-tree regular file (1.0.18
hashes out-of-tree targets, which leaks content from outside the tree
into the ID). Symlink timestamps are not restored (x/sys, approved).
There is no way to say in a c4m "this entry is a link to X".

**The constraint.** The c4m grammar is frozen. Whatever records the link
must already be expressible: an entry's mode field (`lrwxr-xr-x` is a
valid mode string today), a null ID, and the name. The target path is
the only thing with no column.

**Options.**
- A. Mode says link; ID is the target's ID when the target is an
  in-tree regular file, else null; the target path is not recorded. A
  restore recreates a link only when the ID resolves to a path inside the
  same c4m (one ID, one path: unambiguous; several: null link, reported).
  No grammar change.
- B. As A, plus the target path recorded as the entry's name suffix
  (`name -> target`, the `ls -l` form). Readable, but the name column
  gains a sub-grammar, and names containing ` -> ` become ambiguous.
- C. Collapse by default (scan through links, record the target's bytes
  as if copied), with A available by flag. This is what 1.0.18 does for
  files, minus the out-of-tree leak.
- D. Record the link target as a sidecar record about the entry (record
  layer), leaving the c4m untouched.

**Recommendation: A for the format, with "collapse" as a scan option
and D as the home for the target path when the record layer exists.** A
needs no new grammar and keeps "the ID means these bytes". Cost: a
relative link whose target is outside the tree or is a directory restores
as a null link and the restore reports it; that is partial knowledge,
which is allowed. B is the only option that restores every link, and it
costs the grammar.

---

## 7. The 1.0.19 decisions from the VM agent's report

**Suite 1.0.19 shipped 2026-10-21** with 7e, 7h and 7j and the scanner
cancel fix. 7b, 7c, 7d (the store-layout and gc branches) are still
open and go into a later patch.

These are patch-release calls; the agent's full reasoning is in
`/home/ash/ws/c4/STATUS-1.0.19.md` on ash-cloud, copied to the Mac on
2026-10-19 as `~/ws/active/software/c4/cloud-1.0.19/STATUS-1.0.19.md`.

**7a. Ship without `c4 store normalize`.** The verb needs an `--apply`
flag, and patches add no flags; in 1.0.x `c4 store` already means
"identify and store the path named store"; and renaming directories on a
Mac makes replica spellings diverge, which is the rsync shard-wipe state.
With the case-agnostic reader every existing store is valid as it is.
Recommendation: yes, ship without it; normalize is a v1.1 verb.
**RULED 2026-10-19 (Joshua, relayed by the home-6e session): no
normalize command at all, in 1.0.19 or v1.1.** His words: "No normalize
command at all. I don't think that C4 should interact with this. The
prescription right now is that C4 understands how to interact with any
version of that folder system. And it has a recommendation for how it
will build one if it needs to build one. So there's nothing to do. The
user doesn't need to interact with it at all." So the recommendation's
second half was wrong: the verb is dropped from the plan, not deferred.
The reader stays case-agnostic and any-depth, new splits follow the
layout rule, and the opportunistic rename-only migration (ash-brief §4
item 3) is dropped with it. 7b, 7c and 7d remain open.

**7b. Lowercase split directories are invisible to older readers on
Linux shared stores.** New directories are created only by splits. A
reader that builds `c4/<id[2:4]>/` with the ID's own case (c4 1.0.17 and
earlier, c4py 1.0.18, imgbin, hand-written scripts) does not find an
object a 1.0.19 split placed under a lowercase name when the ID has an
uppercase letter there. Measured: splitting a flat root of 4,098 objects
hides 1,594 from 1.0.17; c4 1.0.18 sees them all. The reviewer called
this a blocker unless accepted. Recommendation: accept, release the
readers in lockstep (c4, c4git, c4sh pins, c4py, c4ts) and tell imgbin's
owner first. Alternative: exact-case split names in 1.0.19 and lowercase
from v1.1, which postpones the rule you ruled.

**7c. Lookup order.** The draft said lowercase first; 1.0.19 probes the
ID's own spelling first because 1.0.18-written Linux stores have both
`c4/4E` and `c4/4e`, and lowercase-first put 40% of new writes where
older readers cannot see them. Recommendation: exact-case first in
1.0.19; v1.1 may switch once stores are normalized. Note: this order is
also what causes the macOS double-count bug found yesterday (the split
counter is keyed by spelling); the fix is to key the counter by the
directory's on-disk spelling, which does not change the order.

**7d. c4ts timestamp bug.** Pretty-printed c4m on a UTC machine prints
the zone as `CUT`, which the parser rejects; three tests fail under
`TZ=UTC` on 1.0.18 and the branch alike. Recommendation: fix in 1.0.19;
it is a correctness bug with no surface change.

**7e. S3 README.** c4-containers' README says `S3_ENDPOINT`; c4 reads
`C4_S3_ENDPOINT`. Docs only. Recommendation: fix in 1.0.19. **Done
2026-10-17** on c4-containers `main` (1df1f15) and `release/1.0.x`
(cee8aab): README corrected; the s3worker script exports
`C4_S3_ENDPOINT` from `S3_ENDPOINT`.

**7f. c4git gc with `C4_STORE=s3://…`.** The filters fall back to the
repo store, so gc follows them and collects it. Keep that, or refuse
whenever any `C4_STORE` is set? Recommendation: keep; gc follows the
filters by definition, and the repo store is private.

**7g. Push identity.** The VM's git identity is yours, so its commits are
authored "Joshua Kolden". Push as is, or reauthor as a bot identity?
Recommendation: push as is; they are your project's commits on your VM.

**7h. c4git smudge truncates pass-through content (new, verified
2026-10-16).** `filter/filter.go` `Smudge` reads at most 256 bytes to
sniff for a C4 ID and, when the input is not an ID, writes only those
256 bytes and never the rest of the stream. Run on c4git 1.0.18 in an
isolated store: a 1,000-byte and a 300,000-byte non-pointer file both
come out as 256 bytes; 100 bytes passes intact. Git calls smudge on
checkout for every blob matching the filter attribute, so any file
committed before c4git was configured (or whose clean step was skipped)
is truncated in the working tree on checkout. Data loss, no surface
change to fix. Recommendation: 1.0.19 blocker; the fix is to copy the
remainder of the reader after the sniffed bytes. Reported by the
c4-operator session. **Fixed 2026-10-17**: c4git commit d3b4965 on
`release/1.0.x` and `main` (main fast-forwarded to the release line);
new test fails on 1.0.18 and passes with the fix; suite green with race
on macOS. Not yet in any tag: it ships with 1.0.19.

**7i. A pre-release tag on master.** The operator now depends on master
by Go pseudo-version `v1.0.17-0.20261016134402-db4c7bec2376`. That
version sorts below v1.0.18, so any module that also requires c4
v1.0.18 resolves to v1.0.18 and loses the three master-only APIs. Only
a tag that sorts above v1.0.18 fixes that, e.g. `v1.1.0-alpha.1` on
master. Against it: master's surface is what the re-scope changes, and
the versioning policy says minor is the breaking tier; a tagged alpha is
a promise that this surface exists under the v1.1 name. For it: an
alpha tag is explicitly not a release, Go treats it as a pre-release,
and it unblocks a public project. Recommendation: no tag until the
re-scope document is signed; if the operator must ship before then, it
vendors or keeps the `replace`. Your call, and the operator session
will raise it with you directly.

**7j. Natural order of long digit runs (new 2026-10-20, from the
operator team).** `c4m.NaturalLess` parsed digit runs into an int64, so
a 20-digit run wrapped: two different runs could compare equal, and a
tree with such names got a different ID per run (a reviewer saw two
IDs in six runs). Fixed on master (b7f9e63; c4py 2cec4de; c4ts 1bc9723)
by comparing runs as integers of any length with no parse; shared
vectors in `c4m/testdata/natural_sort_vectors.json`; the c4m spec says
"of any length" now. Names with runs of 18 digits or fewer sort exactly
as before, so their IDs do not change; longer runs get the one ID the
spec defined, which c4py already produced. **Joshua, 2026-10-20: "We
must fix any c4m errors in sorting asap."** Done the same day on every
release line: c4 release/1.0.x e9f77e7, c4py 56f50b7, c4ts 1a6659a,
libc4 release/1.0.x (text segments now compare whole: a1 < ab, which
its character walk had backwards), c4-swift main ae2a696 (its Int64
parse trapped at 20 digits, a crash, not only a wrong order). The
1.0.19 suite entry is in c4toolkit releases.json, waiting on the
release go. Earlier text, kept for the record: Question: cherry-pick
into 1.0.19? Recommendation: yes. It removes nondeterminism, adds no API and
no flag, and the only IDs that move were never stable. The patch policy
("safe to upgrade blind") is met for every tree that had one ID.

**7k. Backport the `c4 farm` dispatch to a 1.0.20 patch? (new
2026-10-21, from the c4-operator release).** Joshua's gate for c4-hud
going public is `brew install avalanche-io/tap/c4` then `c4 farm
agents` on a clean machine. The dispatch (`design/external-tools.md`)
is on master only, and v1.1 waits on the re-scope. The patch policy
allows strictly additive changes that break no script. The dispatch in
its additive form qualifies: `c4 <name>` runs `c4-<name>` only when no
file or directory of that name exists, so every invocation that works
on 1.0.19 means the same on 1.0.20; `c4 help` lists the tools. The
reserved names (`farm`, `agent` winning over a same-named directory)
change what `c4 farm` does for a directory named `farm`, so they stay
in v1.1. Recommendation: yes, 1.0.20 with the additive rule, cut right
after c4-operator v0.1.0 publishes its formulas, carrying c4-farm,
c4-agent and c4-unit from those release archives in the suite bundles
and `c4.rb` depending on the two formulas in `Avalanche-io/homebrew-tap`.
Alternative: wait for v1.1.0 (no date). Your call. **RULED 2026-10-21
(Joshua): "I agree with building 1.0.20."** The additive dispatch is on
release/1.0.x; the suite cut waits for c4-operator v0.1.0's formulas and
archives and the new tap.

**7l. c4-operator in the version group (Joshua, 2026-10-21: "we should
be looking towards adding c4-operator to the version group").** What
joining means: a `releases.json` component with the suite's version,
tagged `vX.Y.Z` with the rest and validated together, the way
c4-containers already is. Two facts set the timing. (1) c4-operator
needs library API that exists on master only (`c4m.IsJournal`,
`scan.ModeContent`, the closing-checkpoint parse), and a patch release
adds no exported API, so an operator pinned to c4 1.0.x cannot build:
in the 1.0.x suites it can only be a dependency at its own version
(0.1.0), which is what 1.0.20 does. (2) At v1.1.0 the APIs it needs are
released, so it can pin `c4 v1.1.0` and take the suite version. The
fork in the road is the first operator tag: `v0.1.0` now (its own
cadence until v1.1) or no tag until v1.1.0, when its first release is
`v1.1.0` beside the rest. Recommendation: tag v0.1.0 now, so 1.0.20 and
the c4-hud launch do not wait on the re-scope, and make v1.1.0 the
version at which c4-operator joins the group (its RELEASING.md,
install.yaml URLs and goreleaser need nothing but the tag name). Also
c4-hud, once it has a remote: same question, same answer.

---

## 8. One consistency check: attic versus "no replicated attic"

In the narrator session (2026-10-03 and 04) you asked for the c4 attic
to allow time-machine-like restore and eventual safe removal, with a
purge that is symbolic and a removal system that is modular, and two
deletion classes (obviously safe, needs human review). The brief's
ruling 5 says no replicated attic or trash folders. These read as
consistent: the attic is the local decision zone and never replicates;
removal is the durability ladder. If that is not what you meant, this is
the place to say so.
