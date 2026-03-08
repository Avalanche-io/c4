# Flow Links: CLI Impact Analysis

How flow links and `location:path` interact with each of the 14 c4
verbs. Analysis grounded in the CLI spec (`cli_v1.md`) and the
implemented code in `cmd/c4/`.

## Current State

The CLI implements all 14 verbs. Pathspec parsing (`internal/pathspec/`)
recognizes five target types: `Local`, `C4m`, `Location`, `Container`,
and `Managed`. Location paths are parsed when the left side of a colon
matches a registered location name via `establish.IsLocationEstablished`.
Today, locations require explicit `c4 mk studio: host:port` to register
an address. Location operations (read/write through `location:path`)
are not yet wired to c4d relay — the plumbing exists in pathspec but
the `cp`, `ls`, and `diff` handlers fall through to "not yet supported"
for Location-type targets.

Flow links introduce a new entry type in c4m: directional declarations
(`->`, `<-`, `<>`) binding a local path to a remote location. These are
metadata — they live inside c4m files and managed directory state, not
as filesystem objects.

---

## Verb-by-Verb Analysis

### 1. `c4` (Identify) — No change

Bare `c4 path` scans the filesystem and computes C4 IDs. It produces
c4m output. Flow links are not filesystem entries — they are c4m
metadata declarations. Scanning a directory will never encounter a flow
link because flow links don't exist on disk.

If a c4m file being identified *contains* flow link entries, those
entries contribute to the c4m's C4 ID like any other entry. No special
handling needed.

### 2. `c4 cat` — No change

`cat` outputs file content bytes. Flow links have no content — they are
declarations, not files. `c4 cat` operates on files (by path or by C4
ID). No interaction with flow.

### 3. `c4 ls` — Affected

`c4 ls` reads c4m state and outputs it. Flow-linked entries need to be
visible in listings.

**Display question:** When `c4 ls :` or `c4 ls project.c4m:` shows a
directory that has a flow link, how does it appear?

Proposal: flow links are their own entry type, displayed in-line with
the entries they annotate. They are not file entries — they are
declarations attached to a path. The natural representation:

```
drwxr-xr-x 2026-03-04T14:22:10Z - footage/   c4xyz...
  -> nas:footage/
drwxr-xr-x 2026-03-04T14:22:10Z - renders/   c4abc...
  <> studio:renders/
-rw-r--r-- 2026-03-04T14:22:10Z 108 main.go   c4def...
```

The flow declaration line is indented under the entry it annotates.
It has no mode, timestamp, size, or C4 ID — it is purely structural.
This means `awk '{ print $NF }'` still extracts C4 IDs from entry
lines without being confused by flow lines. Flow lines start with a
direction marker (`->`, `<-`, `<>`) which is unambiguous — no c4m
entry line starts with these tokens.

**Grammar extension:**

```
flow_line ::= indent direction ' ' location_ref
direction ::= '->' | '<-' | '<>'
location_ref ::= name ':' subpath?
```

Flow lines appear immediately after the entry they annotate, at the
same indentation level as the entry's children. They are not entries
themselves — they carry no C4 ID and don't affect parent directory
identity.

**Alternative:** Flow declarations could be entry-like with all-nil
fields (like ignore patterns), e.g.:

```
- - - -> nas:footage/ -
```

This keeps the fixed-field grammar intact. The direction marker is
part of the name field. The all-nil pattern distinguishes them from
real entries. This approach is more consistent with existing c4m
conventions (all-nil entries for exclusion patterns).

**Recommendation:** Use the all-nil entry form. It preserves the
positional grammar that makes c4m awk-native. The direction marker
(`->`, `<-`, `<>`) appears in the name field position, followed by
the location reference. Since real filenames never start with `->`,
`<-`, or `<>`, this is unambiguous.

```
drwxr-xr-x 2026-03-04T14:22:10Z - footage/ c4xyz...
  - - - -> nas:footage/ -
drwxr-xr-x 2026-03-04T14:22:10Z - renders/ c4abc...
  - - - <> studio:renders/ -
```

**Pretty format:** `--pretty` could render these more readably:

```
drwxr-xr-x  Mar 04 14:22:10 2026 CST  footage/     c4xyz...
  -> nas:footage/
drwxr-xr-x  Mar 04 14:22:10 2026 CST  renders/     c4abc...
  <> studio:renders/
```

### 4. `c4 cp` — Affected (indirectly)

`c4 cp` is the universal copy verb. Flow links affect it in two ways:

**a. Flow links don't change `cp` behavior directly.** When you run
`c4 cp files/ project.c4m:footage/`, the copy happens regardless of
whether `footage/` has a flow declaration. The flow link is a
declaration for c4d to act on — it does not gate or redirect cp
operations.

**b. `cp` to a `location:path` triggers the push-intent mechanism.**
`c4 cp project.c4m: studio:` pushes the c4m (intent) to the local
c4d, which forwards it to the studio peer. This is already the
designed behavior per push-intent-pull-content architecture. Flow
links may make this implicit — instead of the user explicitly running
`c4 cp project.c4m: studio:`, the flow declaration `-> studio:` on
a path means c4d handles the propagation automatically.

**Key distinction:** `cp` is explicit, imperative action. Flow is
declarative, automatic. They don't conflict — `cp` remains the
manual override for one-shot transfers. Flow handles ongoing
relationships.

**c. Copying a c4m that contains flow declarations.** If you
`c4 cp project.c4m: output.c4m:`, the flow declarations copy with
the entries. This is correct — the c4m is a self-contained description
that includes its flow relationships. Whether the receiving
infrastructure honors those declarations is up to the receiving c4d.

### 5. `c4 mv` — Affected (minor)

`mv` renames entries within a c4m file. If a renamed directory has
flow declarations, the flow entries must move with it. The current
implementation adjusts child depths when moving directories — flow
declarations that are children of the moved directory get the same
treatment.

If the all-nil entry representation is used, flow declarations are
just entries with depths and names — they move naturally with their
parent directory. No special handling needed beyond what `mv` already
does.

### 6. `c4 ln` — Primary verb for flow links

`ln` is the natural verb for creating flow links. The progression:

- `c4 ln source dest` — hard link (two entries, same C4 ID)
- `c4 ln -s target dest` — symbolic link (one entry references a path)
- `c4 ln -> location: dest` — flow link (one entry declares propagation)

**Syntax:**

```bash
c4 ln -> nas: project.c4m:footage/         # outbound flow
c4 ln <- incoming: project.c4m:backups/    # inbound flow
c4 ln <> desktop: project.c4m:projects/    # bidirectional sync
c4 ln -> nas:footage/ :footage/            # flow on managed dir
```

The direction marker (`->`, `<-`, `<>`) replaces the `-s` flag as the
mode selector. It reads naturally: "link outbound to nas, at footage
inside project.c4m."

**Implementation:** `runLn` currently checks for `-s` to distinguish
hard vs symlink, then checks for managed directory tag creation. Flow
link creation would be a third mode, detected by the first argument
starting with `->`, `<-`, or `<>`.

The flow link is an entry in the c4m file, nested under the directory
it annotates. Creating a flow link on `:footage/` (managed directory)
adds the declaration to the managed state.

**Removing flow links:** `c4 rm project.c4m:footage/-> nas:` or
perhaps `c4 rm project.c4m:footage/->` to remove all outbound flow
from that path. The exact syntax needs design — it must be
unambiguous within the pathspec grammar.

Alternatively, since flow declarations are entries, they can be
addressed by their path in the c4m:

```bash
c4 rm project.c4m:footage/->nas:footage/
```

This is consistent with how `c4 rm project.c4m:renders/old/` removes
directory entries.

**Listing flow links:** `c4 ls project.c4m:` shows them inline.
No separate command needed.

### 7. `c4 mkdir` — No change

`mkdir` creates directory entries in c4m files. Directories can later
have flow links added via `ln`. No reason for `mkdir` to know about
flow.

### 8. `c4 mk` (Establish) — Affected

`c4 mk` establishes locations and managed directories. Flow links
affect it in two areas:

**a. Location auto-resolution.** The design discussion considers
eliminating `c4 mk location:` for locations that can be auto-resolved
(mDNS, c4d config peers). With auto-resolution, `c4 ln -> studio:
project.c4m:renders/` would work without a prior `c4 mk studio:` —
the CLI resolves `studio` through c4d's peer list or mDNS.

If `mk` is no longer required for locations, then `mk` narrows to:
- `c4 mk :` — establish managed directory for tracking
- `c4 mk project.c4m:` — establish c4m file for writing

Locations become implicit — resolved on first use by the CLI talking
to c4d. This is cleaner: `mk` means "start tracking/writing here",
not "configure a remote address."

**b. `c4 mk : --sync location:` already exists in the spec.** The
`--sync` flag on managed directory establishment is essentially a
bidirectional flow declaration. With flow links as a first-class
concept:

```bash
c4 mk :                               # establish for tracking
c4 ln <> nas: :                        # add bidirectional sync
```

These two commands replace `c4 mk : --sync nas:`. The advantage:
flow links are visible, removable, and compose with permissions.
The `--sync` flag becomes sugar (or is removed in favor of the
explicit `ln` step).

**c. Renaming `mk` to `init`.** If `mk` no longer handles location
registration (auto-resolve), it becomes purely about initializing
local tracking. `init` is the more natural verb for that:

```bash
c4 init :                              # establish managed directory
c4 init project.c4m:                   # establish c4m for writing
```

This aligns with `git init`, `npm init`, `cargo init`. The verb says
"prepare this for c4 operations." Flow links are orthogonal — they
are added after initialization via `ln`.

### 9. `c4 rm` — Affected (minor)

`rm` needs to handle flow link entries. If flow links are all-nil
entries in the c4m, `rm` already handles entry removal. The only
question is addressing — how to specify "remove the flow link on
footage/" vs "remove the footage/ directory itself."

The flow entry's "name" field contains the direction and target:
`-> nas:footage/`. So the address would be:

```bash
c4 rm project.c4m:footage/            # remove the directory
c4 rm :footage/-> nas:footage/        # remove specific flow link
```

This requires `rm` to recognize the flow-link path syntax. Minimal
new code — the `findEntry` function needs to match flow entries by
their composite name.

### 10. `c4 diff` — Affected

`c4 diff` compares two c4m states. Flow declarations are entries in
c4m state. When comparing two versions of a c4m:

```bash
c4 diff project-v1.c4m: project-v2.c4m:
```

If v2 added a flow link on `renders/`, the diff output includes that
addition. This is correct and requires no special handling — the diff
algorithm already detects new entries.

**Cross-location diff:** `c4 diff nas:project/ studio:project/`
compares c4m states across locations. This requires the CLI to fetch
c4m state from each location via c4d. Flow links in those states are
part of what gets diffed. The diff algorithm doesn't need to treat
flow entries specially — they are entries like any other.

**Flow-aware diff (future):** A richer question is whether `diff`
should understand flow semantics — "this directory has outbound flow
to nas, is the nas copy in sync?" That's a `c4 diff :footage/
nas:footage/` operation, which already works (compare two sources).
The flow link makes it *discoverable* (you can see the relationship
in `ls`), but `diff` doesn't need to auto-follow flow links.

### 11. `c4 patch` — Affected (minor)

`patch` applies c4m changes to a target. If the patch source contains
flow declarations, those declarations are applied to the target c4m.
This is the correct behavior — patching converges state, and flow
declarations are part of state.

When patching a managed directory (`c4 patch desired.c4m :`), if the
desired state includes flow declarations that weren't present before,
those declarations are added. c4d then sees the new flow declarations
and begins propagation.

No special handling needed if flow links are standard c4m entries.

### 12. `c4 undo` / `c4 redo` — No change

Undo/redo operate on snapshots of managed directory state. If a flow
link was added in the last operation, `undo` reverts it (removes the
flow declaration from the managed state). Redo re-adds it. Since flow
declarations are entries in the c4m state, undo/redo handles them
automatically.

One subtlety: undoing a flow link doesn't undo the propagation that
already happened. If content already reached `nas:` via the outbound
flow, removing the declaration stops future propagation but doesn't
recall already-propagated content. This is consistent with how undo
works for all operations — it reverts the local state, not remote
effects.

### 13. `c4 unrm` — No change

`unrm` recovers items from prior snapshots. If a flow declaration was
removed and the user wants it back, `unrm` can recover it like any
other entry. No special handling.

### 14. `c4 du` — No change

`du` shows storage usage. Flow links are metadata, not content. They
don't consume storage in the CAS.

---

## Commands That Don't Need Changes

These verbs are completely unaffected by flow links:

| Verb | Why unaffected |
|------|----------------|
| `c4` (identify) | Scans filesystem, produces c4m. Flow is c4m metadata, not filesystem. |
| `c4 cat` | Outputs file content bytes. Flow links have no content. |
| `c4 mkdir` | Creates directory entries. Flow is added separately via `ln`. |
| `c4 undo` | Operates on snapshots. Flow entries are just entries. |
| `c4 redo` | Same as undo. |
| `c4 unrm` | Recovery from snapshots. Flow entries are just entries. |
| `c4 du` | Storage accounting. Flow links are metadata, not blobs. |

These seven verbs need zero awareness of flow.

---

## Commands That Need Changes

| Verb | Change scope |
|------|-------------|
| `c4 ls` | Display flow entries in listings. Canonical and pretty formats. |
| `c4 ln` | Primary creation verb. New mode: `c4 ln -> location: target` |
| `c4 mk` | Evaluate removing `--sync` in favor of `ln <>`. Consider rename to `init`. Location auto-resolve changes `mk`'s scope. |
| `c4 rm` | Address flow entries for removal. Minor pathspec extension. |
| `c4 cp` | No behavioral change, but flow declarations copy with c4m content. |
| `c4 mv` | Flow entries move with their parent directory. Already works if entries are depth-based. |
| `c4 diff` | Flow entries diff like any other entry. No special code, but cross-location diff needs location read support. |
| `c4 patch` | Flow entries apply like any other entry. No special code needed. |

Of these, only `ls`, `ln`, and `mk` require meaningful new code. The
rest work automatically if flow links are represented as standard c4m
entries.

---

## `c4 ln` and Flow: Detailed Design

### Creation syntax

```bash
# Outbound flow: content here propagates there
c4 ln -> nas: :footage/
c4 ln -> nas:archive/footage/ project.c4m:footage/

# Inbound flow: content there propagates here
c4 ln <- incoming: :backups/
c4 ln <- studio:dailies/ :incoming/dailies/

# Bidirectional sync: content converges both ways
c4 ln <> desktop: :projects/
c4 ln <> nas: :
```

The pattern is: `c4 ln DIRECTION REMOTE_REF LOCAL_TARGET`

- DIRECTION: `->` (outbound), `<-` (inbound), `<>` (bidirectional)
- REMOTE_REF: `location:` or `location:subpath/`
- LOCAL_TARGET: where the flow is declared (c4m path or managed path)

### Shell escaping

`<>`, `<-`, and `->` are all safe in most shells without quoting.
`>` redirects stdout, but `->` is a single token that shells pass
through. `<` redirects stdin, but `<-` and `<>` are single tokens.

**Caution:** In some shells, `<>` opens a file for read-write. This
is only true when `<>` appears before a filename as a redirect
operator. As a command argument (not the first token after `|`, `&&`,
etc.), it passes through literally. However, to be safe:

```bash
c4 ln '<>' desktop: :projects/        # quoted, always safe
c4 ln -- '<>' desktop: :projects/     # with -- separator
```

Or use a flag instead: `c4 ln --sync desktop: :projects/`. But flags
fight the minimalist philosophy. The direction-as-first-argument
approach is cleaner if shell escaping is manageable.

**Alternative: dedicate `c4 flow` as a subcommand.** This avoids
overloading `ln` and sidesteps shell escaping:

```bash
c4 flow out nas: :footage/
c4 flow in incoming: :backups/
c4 flow sync desktop: :projects/
c4 flow rm :footage/->nas:
```

This adds a 15th verb. It must earn its place. The argument for it:
flow is conceptually different from linking — it's about ongoing
relationships, not entry creation. The argument against: flow links
ARE links, and adding a verb violates "nothing enters this CLI
without a fight."

**Recommendation:** Use `c4 ln` with direction markers. The `ln`
verb already handles three link types (hard, symlink, tag). Flow is
the fourth. The progression is natural: hard (same volume) -> sym
(same network) -> flow (any boundary). Keep the verb count at 14.

### Entry representation in c4m

Flow declarations as all-nil entries nested under their target:

```
drwxr-xr-x 2026-03-04T14:22:10Z - footage/ c4xyz...
  - - - -> nas:footage/ -
  -rw-r--r-- 2026-03-04T14:22:10Z 108 clip001.mxf c4abc...
drwxr-xr-x 2026-03-04T14:22:10Z - renders/ c4def...
  - - - <> studio:renders/ -
  drwxr-xr-x 2026-03-04T14:22:10Z - final/ c4ghi...
```

The flow entry is a sibling of the directory's children. Its depth is
one greater than the annotated directory, same as the directory's
regular children.

**Sort order:** Flow entries sort before regular entries within their
parent (they start with special characters that sort before
alphanumeric filenames). This puts them at the top of the listing for
each directory — visible and prominent.

**Identity impact:** Flow entries are all-nil (no C4 ID). They do NOT
affect the parent directory's C4 ID computation. The directory's C4 ID
is the hash of its children's canonical listing — flow entries, being
all-nil metadata, are excluded from this computation. This is critical:
the same directory with or without flow declarations has the same
identity if the content is the same. Flow is about *where* content
goes, not *what* the content is.

Wait — this creates a problem. If flow entries don't affect C4 ID,
then two c4m files with different flow declarations but the same
content would be identical by C4 ID. That may be correct (identity
is about content, not routing) or it may be a problem (you want to
detect when flow configuration changed).

**Resolution:** Flow entries SHOULD affect the c4m's C4 ID because
the c4m *is* the description, and flow declarations are part of the
description. But directory content IDs should NOT be affected because
the directory ID represents the content tree, not the routing.

This distinction already exists: a directory's C4 ID is the hash of
its children's listing, while the c4m file's C4 ID is the hash of
the entire c4m (including flow entries). So: the c4m file changes
identity when flow is added, but individual directory entries within
it keep their content-based identity.

The all-nil entry representation supports this naturally: all-nil
entries (like exclusion patterns) are structural metadata that
participates in the c4m file's identity but is excluded from
directory content identity (nil propagation makes the directory ID
nil if flow entries are included — unless flow entries are filtered
out of directory computation).

**Decision needed:** Are flow entries included in the canonical
listing that determines a directory's C4 ID? The answer should be
no — they are observer metadata (like ignore patterns), not content
metadata. The c4m encoder should skip all-nil entries when computing
directory IDs.

---

## `c4 ls` and Flow: Display

### Canonical format

```
drwxr-xr-x 2026-03-04T14:22:10Z - footage/ c4xyz...
  - - - -> nas:footage/ -
  -rw-r--r-- 2026-03-04T14:22:10Z 42000000 clip001.mxf c4abc...
```

This is valid c4m. The flow entry is an all-nil entry with a name
that starts with a direction marker. Tools that process c4m can
filter these with `awk '$1 != "-"'` to get only real entries.

### Pretty format

```
drwxr-xr-x  Mar 04 14:22:10 2026 CST  footage/           c4xyz...
  -> nas:footage/
  -rw-r--r--  Mar 04 14:22:10 2026 CST  42,000,000  clip001.mxf   c4abc...
```

Pretty format collapses the all-nil fields, showing only the
direction and target. Clean, readable.

### Filtering

```bash
c4 ls : | awk '$1 == "-" && $4 ~ /^->/'     # all outbound flows
c4 ls : | awk '$1 == "-" && $4 ~ /^<>/'     # all syncs
c4 ls : | awk '$1 != "-"'                    # entries only, no flow
```

---

## `c4 diff` Across Locations

`c4 diff nas:project/ studio:project/` compares c4m state between
two locations. This requires:

1. CLI asks local c4d for the c4m state at `nas:project/`
2. CLI asks local c4d for the c4m state at `studio:project/`
3. CLI diffs the two manifests locally

The CLI never talks to remote peers directly — it always goes through
the local c4d. The local c4d fetches the remote c4m state from the
named peer.

This is not a flow-link feature — it's basic location support. But
flow links make it discoverable: if `c4 ls :` shows
`<> studio:renders/`, the user knows to run
`c4 diff :renders/ studio:renders/` to check sync status.

**Implementation note:** The `getManifest` function in `main.go`
already has a `pathspec.Location` case. It currently falls through
to "not yet supported." Wiring it to c4d's namespace API completes
location support for all commands that use `getManifest` (diff, cp,
patch, etc.).

---

## `c4 init` vs `c4 mk`: Impact of Flow

With location auto-resolution and flow links as explicit `ln`
operations, `mk` narrows to:

**Before (current `mk`):**
- `c4 mk :` — establish managed directory
- `c4 mk project.c4m:` — establish c4m for writing
- `c4 mk studio: host:port` — register a location
- `c4 mk : --sync nas:` — establish with sync

**After (with flow + auto-resolve):**
- `c4 init :` — establish managed directory
- `c4 init project.c4m:` — establish c4m for writing
- (locations auto-resolved via c4d — no `init` needed)
- (sync declared via `c4 ln <> nas: :` — no `--sync` flag)

`init` is a cleaner verb for what remains. The rename also resolves
the naming overlap with `mkdir` — `mk` and `mkdir` are too similar.
`init` and `mkdir` are clearly distinct operations.

**Migration:** `c4 mk` can remain as an alias for `c4 init` during
transition. The CLI spec lists 14 verbs — replacing `mk` with `init`
keeps the count at 14.

Flow links don't force this rename. But they remove enough
responsibility from `mk` (no location registration, no `--sync`)
that the rename becomes natural.

---

## New User Stories

### DIT on set with flow channels (Kai)

Kai is a DIT on a TV series shooting across two stages. Each stage
has a c4d node on the DIT cart. Camera cards come in fast — sometimes
A-cam and B-cam simultaneously from both stages.

**Setup (once per show):**
```bash
# Initialize the daily ingest directory for tracking
c4 init :

# Camera cards drain to the shuttle drive — outbound flow
c4 ln -> shuttle: :A-cam/
c4 ln -> shuttle: :B-cam/

# Lab also gets a copy via the satellite uplink (slow, but starts
# immediately so partial content arrives before the shuttle)
c4 ln -> lab-relay: :A-cam/
c4 ln -> lab-relay: :B-cam/
```

**Daily workflow:**
```bash
# Card comes in. Kai ingests.
c4 cp /mnt/card-A/ :A-cam/day-12/

# The flow declarations do the rest:
# - Content drains to the shuttle drive automatically
# - c4m (description only) pushes to lab-relay immediately
# - Lab starts pulling content over satellite

# Kai checks flow status
c4 diff :A-cam/day-12/ shuttle:A-cam/day-12/
# Empty diff = shuttle has everything. Ready to hand off.

c4 diff :A-cam/day-12/ lab-relay:A-cam/day-12/
# Shows what's still in transit over satellite.
```

**Kai never runs `c4 cp` to the shuttle or the lab.** The flow
declarations handle propagation. `cp` is for ingest from the camera
card. After that, flow takes over.

### Studio admin deploying project templates (Reina)

Reina manages infrastructure for a mid-size VFX studio. When a new
project starts, artists need a workspace with the right structure
and flow channels already configured.

**Template creation (once):**
```bash
# Build the project template as a c4m file
c4 init template-vfx.c4m:
c4 mkdir -p template-vfx.c4m:footage/plates/
c4 mkdir -p template-vfx.c4m:footage/reference/
c4 mkdir -p template-vfx.c4m:renders/wip/
c4 mkdir -p template-vfx.c4m:renders/final/
c4 mkdir -p template-vfx.c4m:deliveries/

# Add flow channels to the template
# Plates arrive from the ingest station
c4 ln <- ingest: template-vfx.c4m:footage/plates/

# Final renders flow to the review server
c4 ln -> review: template-vfx.c4m:renders/final/

# Deliveries sync bidirectionally with the delivery staging area
c4 ln <> delivery-staging: template-vfx.c4m:deliveries/

# The template is now a c4m file: structure + flow channels.
c4 ls -p template-vfx.c4m:
# footage/
#   plates/
#     <- ingest:
#   reference/
# renders/
#   wip/
#   final/
#     -> review:
# deliveries/
#   <> delivery-staging:
```

**Deploying to a workstation (per project):**
```bash
# On the artist's workstation, create the project workspace
c4 init :
c4 patch template-vfx.c4m :

# Done. The workspace has the right directory structure and the
# flow channels are live. Plates will arrive from ingest. Final
# renders will flow to review. Deliveries sync with staging.
```

The artist doesn't configure anything. The template carries the
flow declarations. `c4 patch` applies them to the managed directory.
c4d sees the new flow declarations and begins honoring them.

### Artist with automatic backup (Tomoko)

Tomoko is a compositor. Her workstation has c4d running. IT set up
her home directory with flow channels when she started.

**What she sees:**
```bash
$ c4 ls :
drwxr-xr-x 2026-03-04T14:22:10Z - projects/ c4xyz...
  <> nas:artists/tomoko/projects/
drwxr-xr-x 2026-03-04T14:22:10Z - reference/ c4abc...
  <- library:reference/
-rw-r--r-- 2026-03-04T14:22:10Z 1200 .nuke/ c4def...
```

`projects/` syncs bidirectionally with her folder on the NAS. She
doesn't think about backup — changes propagate automatically. If
her workstation dies, IT gives her a new one, runs `c4 patch` from
the template, and her projects materialize from the NAS.

`reference/` has inbound flow from the studio reference library.
When someone adds new reference material, it appears in her
`reference/` directory automatically. She can browse it, but she
can't modify it (the directory has read-only permissions locally,
and the flow is inbound-only).

**What she does daily:**
```bash
# Work on shots. Save files normally. Nuke, After Effects, etc.
# c4d watches the managed directory and propagates changes.

# Check that her work is backed up
c4 diff :projects/shot-1420/ nas:artists/tomoko/projects/shot-1420/
# Empty diff = NAS has everything. Done.
```

Tomoko never runs `c4 cp` to the NAS. She never thinks about backup.
The flow declaration handles it. Her interaction with c4 is limited
to occasionally checking sync status and browsing reference material.

---

## Summary of Design Decisions Needed

1. **Flow entry representation in c4m:** All-nil entry (recommended)
   vs. dedicated flow line syntax. The all-nil approach preserves the
   positional grammar and works with existing c4m tooling.

2. **Directory C4 ID computation:** Flow entries should be excluded
   from directory identity (they are routing metadata, not content).
   The c4m file's identity includes them.

3. **`c4 ln` syntax for flow:** Direction marker as first argument
   (`c4 ln -> nas: :footage/`). Shell-safe, reads naturally, keeps
   verb count at 14.

4. **`c4 rm` for flow entries:** Needs addressable syntax for flow
   entries. Proposal: full path including direction and target as
   the entry name.

5. **`mk` rename to `init`:** Natural consequence of location
   auto-resolve removing `mk`'s location registration role. Not
   blocked by flow — can happen independently.

6. **`--sync` flag removal:** Flow links via `ln <>` subsume the
   `--sync` flag on `mk`/`init`. The flag can remain as sugar or
   be deprecated.

7. **Location auto-resolution:** Prerequisite for flow links to
   feel natural. Without auto-resolve, users must `c4 mk studio:`
   before `c4 ln -> studio: :renders/`. With auto-resolve, the
   `ln` command just works.
