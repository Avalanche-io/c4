$ c4 --help
c4 - content-addressed identification and preservation (SMPTE ST 2114)

Usage:
  c4 id [flags] <path>...          Describe files, directories, or c4m files (c4m text to stdout)
  c4 id -s <path>...               Snapshot into the store; print one snapshot ID per path
  c4 restore [--force] <target> <dir>
                                   Make dir match a description (dry run by default;
                                   --force snapshots dir first, then applies)
  c4 cat [-e] [-r] <id[/path] | file.c4m>
                                   Retrieve a store object by ID or ID/path (verified),
                                   or display a c4m file
  c4 diff [-e] <old> <new>         Produce a c4m diff (patch); args may be dirs, c4m files, or IDs
  c4 patch [-n N] [-e] <chain.c4m>...  Resolve patch chains to c4m text (never touches directories)
  c4 log [<chain.c4m>...]          List a patch chain's sections (default: the store journal)
  c4 gc [--force] [-v] [<roots.c4m>...]  Collect unreachable store objects (dry run unless --force)
  c4 merge <path>...               Combine filesystem trees (c4m or directories)
  c4 paths [<file.c4m> | -]        Convert between c4m text and path lists
  c4 intersect <a> <b>             Find common entries between c4m files
  c4 split <file.c4m> <N> <before.c4m> <after.c4m>   Split a chain at section N
  c4 explain <command> [args]      Plain-language description of what a command would do
  c4 version                       Print version

Shortcuts:
  c4 <path>            same as: c4 id <path>        (read-only)
  c4 -s <path>         same as: c4 id -s <path>
  ... | c4             print the C4 ID of stdin bytes (nothing stored)
  ... | c4 -s          store stdin bytes; print their ID
  ... | c4 id -        parse stdin as a c4m description

Identity:
  content ID    the ID of bytes alone: for a file, its contents; for a
                directory, its child listing with mode and time null.
                Scanned alike (same -S choice), equal exactly when
                contents are byte-identical, on any machine, clock, or
                umask. This is c4 id's default level; print it alone
                with: c4 id -q
  snapshot ID   the ID of a recorded description: everything observed at
                a moment (modes, times, sizes, names, content). Printed
                by: c4 id -s. The store alone returns all of it.
  A store-resolved ID may be followed by /path to descend into it by
  entry name (paths never follow symlinks):
                c4 cat <ID>/src/main.go
  ID/path is accepted by c4 cat, restore targets, and diff sides.

Nothing is written anywhere without -s (or restore --force), and -s
writes only inside the store: no .c4m file, nothing in the scanned tree,
nothing in your working directory. -q changes output form (bare ID
instead of c4m text), never what is identified. A snapshot ID is printed
only after everything it stored is on stable media. The store records
every snapshot in its journal: run `c4 log` to list them.

Store: set C4_STORE=/path/to/store, or list stores in ~/.c4/config.
A store is a directory on a local filesystem. Reads consult stores in
order; the first store is written to and holds the journal.
Default: ~/.c4/store

════════════════════════════════════════════════════════════════════════

C4-ID(1)

NAME
    c4 id - describe files and directories as c4m text; snapshot with -s

SYNOPSIS
    c4 id [-q] [-s] [-m s|c|m|f] [-e] [-S] [--exclude GLOB]...
          [--exclude-file FILE] [-c GUIDE] <path>...
    c4 id [-q] [-m MODE] -          (parse stdin as a c4m description)

DESCRIPTION
    Writes a plain-text c4m description of each path to stdout. Without
    -s, id reads only; nothing is written anywhere.

    With -s, id stores a complete snapshot of each path in the
    configured store: every file's bytes, every directory's one-level
    listing (root included, at both full and content detail), the
    ID-list object of every folded sequence entry (see -S and
    IDENTITY), and one entry in the store journal per path. -s writes
    only inside the store directory: it creates no .c4m file, nothing
    in the scanned tree, nothing in your working directory. stdout is
    one line per path — the snapshot ID, in argument order — each
    printed only after everything that path stored is on stable
    media. Paths are snapshotted in argument order and claimed
    independently: an earlier snapshot stands even if a later path
    fails, and all paths are attempted. With a file argument, -s
    stores the file's bytes and journals them (a file's snapshot ID
    is the ID of its bytes; restore targets are directory
    descriptions).

    Narration and progress go to stderr and are not part of the
    contract: when stderr is a terminal, one live line is rewritten in
    place (files and bytes so far; no color, no escape codes) and
    replaced at completion by a single summary line; otherwise exactly
    one summary line prints at completion. Detection is
    isatty(stderr), nothing else. Parse nothing from stderr — every
    load-bearing datum is on stdout or in the journal.

MODES (-m)
    s  structure   names only (no IDs)
    c  content     names, sizes, symlink targets, IDs computed from
                   bytes alone; mode and timestamp null (-). Default.
    m  metadata    modes, times, sizes, names (no IDs)
    f  full        everything observed
    -q requires a mode that records IDs (c or f). -s always records
    full detail and conflicts with -m.

IDENTITY
    A file's content ID is the ID of its bytes. A directory's ID is the
    ID of the one-level canonical listing of its direct children; with
    stat fields null this is its content ID (identical on every
    machine); with observed modes and times it is a snapshot ID. In a
    content-form listing, directory entries carry child content IDs; in
    a full-form listing, child full-listing IDs — every ID in a listing
    is the ID of the exact text of the object it names. A single file's
    snapshot ID and content ID coincide: the ID of its bytes.

    Recorded sizes come from the scan itself: a file's size is the
    byte count that was read and hashed; a directory's size field is
    the total file bytes the scan read beneath it (recursive; an
    entry recorded with null ID contributes nothing); a symlink's
    size is the byte length of its target text. A symlink entry
    records the link's name and target text, with mode, time, and ID
    null in every listing form; a symlink names no store object. A
    hard-linked file is recorded as an ordinary file — its bytes,
    and in full form its observed mode and mtime; no listing form
    records link structure, so linkness never affects an ID at
    either level. A directory's mode and mtime never appear in its
    own listing — a listing records children; its parent's
    full-form listing records them like any entry's. The scan root
    has no parent entry, so the root's own mode and mtime appear
    nowhere: provenance, not identity. Nothing outside a scanned
    tree affects that tree's ID, and a directory's ID is the same
    whether it is scanned directly or as a subdirectory of a larger
    scan.

    Sequences fold only under -S: without it, every frame records as
    an ordinary file entry. A folded sequence entry records the
    folded name and an ID naming its ID list — the member files'
    IDs, one per line in member order, which is the folded range's
    order: line N names the range's Nth member (line 42 of
    frame.[0001-0100].exr is frame.0042.exr) — stored as an object
    like any listing and identical at both knowledge levels
    (members are files, whose two IDs coincide). Its other fields
    derive from its members: its size is the sum of the member file
    sizes the scan read and hashed (never the byte length of the
    ID-list text); in full form its mode is the members' shared
    mode and its mtime is the newest member mtime. A run folds only
    when every member is a plain file, every member's bytes were
    read and hashed — a folded entry never records a null-ID
    member — and every observed member mode is equal; runs that do
    not qualify, an unreadable member included, record as ordinary
    entries (the unreadable ones per PARTIAL SCANS). What
    constitutes a foldable run is fixed by the frozen c4m sequence
    form, identically in every implementation; -S never changes
    what verification demands (see c4-restore(1) VERIFICATION).

    ID equality is SHA-512 equality: distinct byte streams share an ID
    only by a SHA-512 collision, and every store read is rehashed
    against the requested ID rather than trusted.
    The ID of the empty description — equally, of an empty directory
    and of zero bytes — is
    c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT

DESCRIPTIONS AS INPUT
    An argument ending in .c4m, and `-`, are parsed as descriptions; a
    .c4m argument that does not parse is an error. All other file
    arguments are bytes. Files inside a scanned tree are always
    treated as bytes, whatever their name — never parsed or rewritten.
    A patch chain given to id resolves to its final state first (use
    `c4 patch -n N` for earlier states).

    In every verb, an argument whose first slash-separated component
    is syntactically a C4 ID — 90 characters, beginning c4, all base58
    — is a store address, never a local file; prefix ./ to force a
    filesystem path that merely looks like one. This test precedes the
    .c4m rule. Store addresses are accepted by c4 cat, restore
    targets, and diff sides only. id does not read the store; given a
    store address it exits 1 with:
        c4 id: c461...igYz is a store address; read the store with
        c4 cat:  c4 cat -r c461...igYz | c4 id -q -

    Filesystem path arguments resolve with ordinary operating-system
    path resolution, in every verb — every component, the final one
    included, so an argument that is a symlink names what it
    resolves to: `c4 id link` identifies the referent's bytes, and a
    directory reached through a link scans as that directory. An
    argument that resolves to nothing — a dangling link included —
    is an error. Symlink-ness is recorded for entries inside a
    scanned tree, never for the argument root: the root's kind
    joins its mode and mtime as provenance, not identity — to
    record a symlink as a symlink, scan the directory that contains
    it. Recorded paths are the opposite, by design: store-side
    /path components (c4-cat(1) PATHS) and restore's
    materialization never follow symlinks.

    For a description, id computes at the description's own level of
    detail, so its printed ID is the description's own canonical ID
    (for a full description, its snapshot ID). -m c projects a fuller
    description down to content form; fields cannot be added, only
    nulled. Ingesting a description with -s stores its listings; it
    does not store content the description refers to.

FLAGS
    -q, --quiet          Print only the C4 ID, one line per path, in
                         argument order. -q changes output form, never
                         identity: the level identified is chosen by -m
                         (default: content). `c4 id -q dir` is machine-
                         independent because content is the default,
                         not because of -q.
    -s, --store          Snapshot into the store and print the snapshot
                         ID (implies -q)
    -m, --mode MODE      Scan detail: s, c, m, f (see MODES)
    -e, --ergonomic      Column-aligned output
    -S, --sequence       Detect and fold file sequences into single
                         entries; folding never happens without this
                         flag. A folded entry is addressed by its
                         folded name — members are fetched by the IDs
                         in its ID list, never by member filename
                         (see c4-cat(1) PATHS) — and records one mode
                         and one mtime for all members (see IDENTITY).
    --exclude GLOB       Exclude pattern (repeatable)
    --exclude-file FILE  File of exclude patterns, one per line
    -c, --continue C4M   Use an existing c4m as a scan guide

DURABILITY
    If the snapshot ID printed, the snapshot — content, listings,
    ID-list objects, and its journal entry — survives power loss,
    even one microsecond after printing. If it did not print,
    nothing was claimed: the scanned tree was never written to, the
    store's prior contents and journal are intact, and re-running
    the same command continues where it left off (objects already
    present are skipped). One in-between case exists: if power
    failed after the journal entry but before the print, the
    snapshot is complete and durable and `c4 log` lists it —
    whatever the journal shows after a crash is real, and
    checkable like any journal line: c4 cat -r <ID> | c4 id -q -
    recomputes the claim rather than trusting it. These guarantees follow from the write ordering alone, so
    "crash" means any interruption — a killed process, a kernel
    panic, or power loss alike. They also hold if `c4 gc --force`
    runs at the same time (see c4-gc(1)).

    Objects are written to a temporary name, synced, and atomically
    renamed; a device flush barrier precedes the journal entry and a
    second precedes the printed ID. That ordering — synced before
    renamed, barriered before journaled, journaled before printed —
    holds on every platform, enforced with the strongest primitives
    each offers (macOS: fsync then an F_FULLFSYNC barrier; Linux:
    fsync of files and containing directories plus a device flush;
    Windows: FlushFileBuffers and atomic replace). The promise is
    exactly as strong as those primitives. Durability overhead —
    beyond reading and hashing the data, which dominate — is roughly
    130 microseconds per file plus two ~5 ms barriers, measured on an
    Apple-silicon APFS SSD. Worked example: 20,312 files, 4.8 GB is
    about 3.5 s to read and hash plus about 2.6 s of durability
    overhead, roughly 6 s total. On other filesystems the constants
    differ; the ordering does not.

PARTIAL SCANS
    An unreadable entry is reported on stderr and recorded with
    exactly what the scan observed: its name and kind come from the
    parent directory's listing; fields the scan observed are recorded
    (full form keeps an observed mode and mtime; content form nulls
    them as always); everything unread is null — size and ID always,
    because recorded sizes and IDs derive from reading, and nothing
    was read. An unreadable directory is recorded as a directory
    entry with null size and null ID: it records no children and
    contributes no bytes to any ancestor's size. A symlink whose
    target text could not be read (the scan saw the link but reading
    it was refused) records its name and kind, keeps an observed
    mode and mtime in full form, and records null target text, null
    size, and null ID — a symlink's read is readlink, and nothing
    was read. A scan attempts every name its parent's listing
    enumerates, in canonical order; sockets, fifos, and devices are
    reported and omitted, and an entry whose kind could not be
    observed at all is reported and omitted with them — kinds a
    listing does not record. An unreadable member of an otherwise
    foldable run prevents the fold: a folded entry never records a
    null-ID member, so under -S the run's files record as ordinary
    entries instead (see IDENTITY). The same tree under the same
    access outcomes yields the same listing bytes from every
    conforming scanner. The description (or snapshot) is still
    produced, and id exits 2.

FILES
    Store root: $C4_STORE, or the first store in ~/.c4/config, or
    ~/.c4/store. A store is a directory on a local filesystem. Reads
    consult configured stores in order; writes and the journal go to
    the first. Journal: <store>/log.c4m — an ordinary c4m patch chain,
    one section per ingest. See c4-log(1).

EXIT STATUS
    0 success; 1 error; 2 partial scan (details on stderr). With
    several paths, all are attempted: the exit status is 1 if any path
    failed, else 2 if any scan was partial, else 0.

EXAMPLES
    c4 id src/ > src.c4m            # describe (content form); no store
    c4 id -m f src/                 # full description with modes/times
    c4 id -q src/                   # content ID: same on every machine
    c4 id -s src/                   # snapshot; prints snapshot ID
    c4 id -q saved.c4m              # a saved description's own ID
    c4 id src/ | c4 id -q -         # pipe law: prints src's content ID
    c4 patch chain.c4m | c4 id -s - # store a resolved description

SEE ALSO
    c4 restore (materialize), c4 cat (read), c4 log (journal),
    c4 diff (compare), c4 gc (collect), c4 explain id

════════════════════════════════════════════════════════════════════════

C4-RESTORE(1)

NAME
    c4 restore - make a directory match a description, undo-safely

SYNOPSIS
    c4 restore <target> <dir>            (dry run: print the plan)
    c4 restore --force <target> <dir>    (snapshot dir, then apply)

    target:  a C4 ID resolved from the store (a snapshot ID, a content
             ID, or any stored listing ID), optionally followed by
             /path to descend into it by entry name (e.g. SNAP/src;
             resolution rules in c4-cat(1) PATHS). The path must land
             on a directory entry: a final component naming a file or
             symlink is refused — extract files with c4 cat. <ID>/ and
             <ID> name the same target. Or: a c4m file path (patch
             chains are resolved); or - (stdin). The target must
             resolve to a listing; an ID naming bytes that do not
             parse as a listing is an error. A directory is never a
             target. .c4m file arguments and - are whole descriptions
             and never take /path.
    dir:     the directory to reconcile; created if absent, missing
             parent directories included (created directories are
             outside the recorded tree and get platform-default
             metadata). The path resolves with ordinary OS path
             resolution, final symlink followed: a symlink to a
             directory reconciles the directory it names, and the
             journal records the origin path as given. Anything
             else already at the name — a file, or a symlink that
             resolves to a non-directory or to nothing — is
             refused: exit 1, nothing modified. A name occupied by
             a dangling symlink is not absent.

DESCRIPTION
    Reconciles <dir> to match <target>: entries are created, moved,
    updated, and removed as needed; an absent <dir> is created
    first, missing parents included. A snapshot-ID target restores
    recorded modes and times as well as content; a content-ID target
    restores byte-exact content with platform-default metadata (see
    VERIFICATION). Content is pulled by ID from the store and from
    bytes already present in <dir> (moves are detected).

    Without --force, nothing is written. stdout is the plan in c4m
    patch format, computed at the target's knowledge level and folded
    as the target folds (see VERIFICATION): the first line is <dir>'s
    current ID at that level, then one line per differing entry, then
    the target's ID. <dir> already matches exactly when —
    equivalently — the plan has no entry lines and its two boundary
    IDs are equal. For a snapshot-ID target that records no folded
    entries, the plan's first line is the same ID that --force prints
    as line 1; a folded entry makes them differ, because the plan
    folds as the target folds while the pre-image (line 1) never
    folds — two IDs for the same bytes at different foldings.

    With --force:
      1. Every ID the target names must be resolvable; otherwise the
         missing IDs are listed on stderr and restore exits 1 without
         touching <dir>.
      2. If <dir> contains anything a snapshot cannot fully record
         (unreadable files, directories, or symlink targets; sockets,
         fifos, devices), restore refuses, lists the offenders on
         stderr, and exits 1 without touching <dir>. A symlink whose
         target text is readable is fully recordable — broken or
         not; the target need not exist: a link is its name and
         target.
      3. <dir>'s current state is snapshotted into the store — a
         complete default scan (nothing excluded, nothing folded):
         content, listings, and journal entry, durable — and that
         pre-image snapshot ID is printed as stdout line 1, always
         before the first modification.
      4. <dir> is reconciled — within each directory, entries the
         target does not record are removed first, then recorded
         entries are materialized in listing order — then verified
         (see VERIFICATION), then line 2 prints.

VERIFICATION
    Verification is recomputation at the target's knowledge level,
    and every field of line 2 is observed from the filesystem in
    this run, each at the last moment it remains observable.
    Snapshot-ID target: every file restore writes is read back from
    disk and rehashed after it is synced and before it takes its
    recorded mode; a file restore kept was read and hashed by this
    run's own pre-image scan; recorded modes and mtimes are then
    applied explicitly (your umask is irrelevant), children before
    parents, each entry's recomputed line settling as its own
    metadata lands — beneath a directory whose recorded mode denies
    the restorer read or search, every line is settled before that
    mode is applied. Applying a recorded mode can therefore never
    blind verification: a snapshot whose modes deny their owner read
    (recordable through group or other permission bits, or by root)
    restores and verifies exit 0. Content-ID target: the
    content-form listing is recomputed the same way — names, sizes,
    symlink names and targets, and bytes are verified; modes and
    times are platform-default metadata, meaning whatever file
    creation under your umask and clock produced — not recorded by
    the target, not claimed, and not verified.

    Recomputation takes the target as its guide for sequences:
    folding is a scan choice recorded in the listing, never a
    verification choice. Wherever the target records a folded
    sequence entry, the recomputed listing folds exactly the member
    files that entry records — their recomputed IDs form the ID list,
    hashed as the entry's ID — and nothing else is folded, however
    foldable the tree looks. A byte-perfect restore of a folded
    target therefore recomputes to the target ID exactly; -S at
    snapshot time never changes what verification demands. The
    dry-run plan is computed the same way.

    Symlink entries recompute as name, target text, and
    target length with mode, time, and ID null, so verification
    depends only on <dir> and never reads outside it. The root
    directory's own mode and mtime are recorded in no listing; <dir>
    itself keeps platform-default metadata in both cases. Line 2 is
    the verified, recomputed ID of <dir> as restore left it; restore
    exits 0 exactly when line 2 equals the target ID.

UNDO
    Line 1 reverses the operation:
        c4 restore --force <line-1> <dir>
    restores <dir> byte-exact, including recorded modes and times. The
    undo prints its own line 1, so it is itself undoable. There is no
    ref namespace to update or lose: the undo handle is the ID itself —
    printed on stdout, echoed in the stderr undo hint as a complete
    command (a convenience for a human retyping; scripts take stdout
    line 1 — parse nothing from stderr), and recorded permanently in
    the store journal. `c4 log` is the reflog: a lost terminal does
    not lose the undo handle.

OUTPUT
    Dry run: the plan (c4m patch format) on stdout.
    --force: at most two lines on stdout, and nothing else —
      line 1: the pre-image snapshot ID of <dir> as it was (always a
              full-form snapshot ID), printed before the first
              modification
      line 2: the verified, recomputed ID of <dir> as restore left
              it, at the target's knowledge level; printed whenever
              recomputation of the resulting tree completed (always
              on exit 0 and exit 2)
    If <dir> was absent or empty, line 1 is the empty description
    c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
    Restoring that ID empties the directory. Counts, warnings, and the
    undo command go to stderr (narration; parse nothing from it).

DURABILITY
    Line 1 prints only after the pre-image is on stable media and
    always before the first modification. Line 2 prints only after the
    restored tree is verified and synced. If the run is interrupted
    between the lines — killed, crashed, or powered off — <dir> holds
    a mix of old and new whole files (no torn files); both end states
    are safe in the store — re-run the same command to finish, or
    restore line 1 to go back (if the interruption left a recorded
    mode that denies you read, grant read first — see NOTES).

NOTES
    A description records names, sizes, modes, mtimes, symlink names
    and targets, sequences, and content IDs. It does not record
    ownership, extended attributes, ACLs, special files, or hard-link
    structure; restore neither preserves nor restores those. A
    hard-linked file is recorded as an ordinary file — its bytes, and
    in full form its observed mode and mtime — so a hard-linked tree
    snapshots and verifies exactly, and no listing form records
    linkness. Every file restore materializes is an independent file:
    a tree that contained hard links restores with identical bytes
    and independent inodes, wherever the link partners lived. A
    folded sequence entry materializes its members as independent
    files, each member with the entry's one recorded mode and mtime —
    folding collapses per-member metadata to one value each (see
    c4-id(1) IDENTITY); snapshot without -S when per-member metadata
    must survive. Symlink entries carry no ID; one whose target text
    is recorded is complete and always materializes from its recorded
    name and target. The root directory's own mode and mtime are
    recorded nowhere: <dir> itself keeps platform-default metadata,
    and verification is unaffected.

    An incomplete target entry — a file or directory entry whose
    recorded ID is null, or a symlink entry whose recorded target
    text is null: the mark of something unreadable at scan time —
    cannot be materialized, and restore keeps what it finds: it never
    creates, modifies, or removes anything at that entry's name;
    whatever <dir> holds there is left exactly as found, and an
    absent name stays absent.

    Recorded names are materialized exactly as recorded, in listing
    order, and materialization never replaces an entry it did not
    itself reconcile: every name is claimed with create-exclusive
    operations. A destination filesystem that does not distinguish
    two recorded sibling names (case-insensitive or Unicode-
    normalizing naming) can represent only one of them: the first in
    listing order claims the name, and each later sibling the
    filesystem folds into that claim is unrepresentable there —
    restore materializes nothing for it and lists it on stderr.
    Store-side path matching never folds names (c4-cat(1) PATHS);
    destination filesystems may.

    A destination can also be unable to spell a recorded name at
    all: bytes invalid in its encoding, characters it forbids, or a
    name longer than it permits. A create the destination refuses
    for the name itself is the same declared class — restore
    materializes nothing for that entry and lists it on stderr.
    Only the name decides membership: failures of permission,
    space, or I/O are not name rejections and exit 3, where curing
    the reported failure and re-running finishes the job.

    The rest is restored, each declared omission is listed on stderr,
    and restore exits 2 with line 2 printed. Re-running cannot close
    a declared omission: an incomplete entry the target lacks the
    knowledge to fill; an unrepresentable name the destination lacks
    the alphabet to spell. restore never deletes <dir> itself:
    undoing a restore into a previously absent directory leaves an
    empty directory.

    A snapshot can record modes that deny its restorer later reads —
    a scanner may read a file through group or other permission
    bits, or as root, whose mode denies its owner. Restore's own
    verification reads every byte back before such a mode lands, so
    line 2 is still a recomputed truth; but a later re-scan — and
    the pre-image scan of any further restore or undo over the same
    directory — needs read granted first (the completeness gate
    refuses what it cannot capture). Granting it is a mode change
    the next pre-image records honestly.

    Concurrent restores of the same directory are not coordinated:
    the store stays safe; the tree's final state is undefined.
    Content read from stores other than the write store is outside
    the write store's locks; a read that fails mid-run exits 3 and
    line 1's undo stands.

EXIT STATUS
    0 plan emitted, or reconciled and verified equal to the target
    1 refused before any modification: missing content, uncapturable
      entries, unresolvable or unparseable target, or <dir> occupied
      by anything but a directory after path resolution (a dangling
      symlink included) — nothing modified
    2 reconciled with declared omissions: the resulting tree differs
      from the target only at entries restore declared on stderr —
      incomplete target entries (null recorded ID; null symlink
      target text), kept as found; recorded sibling names the
      destination filesystem cannot distinguish, first claimant in
      listing order kept; and recorded names the destination cannot
      represent at all (refused for the name itself: encoding,
      forbidden characters, or length), nothing materialized;
      line 2 is the verified as-built ID; re-running cannot close
      these
    3 modification began but the resulting tree differs from the
      target in any other way (an operation failed for a curable
      reason — permission, space, transient I/O — concurrent
      mutation, a store read failed): line 1 printed and its undo
      stands; line 2 prints if recomputation completed; cure the
      reported failure and re-run to finish, or restore line 1 to
      go back. No permanent condition routes here: whatever a
      re-run cannot close is declared and owned by exit 2

EXAMPLES
    c4 restore c461iQnu...igYz proj/          # preview
    c4 restore --force c461iQnu...igYz proj/  # apply, undoably
    c4 restore --force c461iQnu...igYz/src src/   # just one subtree
    c4 restore --force $(c4 id -s good/) bad/ # mirror good into bad
        # (mirroring requires id -s, so the content is in the store;
        #  piping `c4 id good/` alone fails the missing-content check)
    c4 restore --force c459dsjf...c1sFT tmp/  # empty tmp/, undoably

SEE ALSO
    c4 id -s (create snapshots), c4 log (find snapshot IDs),
    c4 diff (compare), c4 explain restore

════════════════════════════════════════════════════════════════════════

C4-CAT(1)

NAME
    c4 cat - retrieve store objects by ID or ID/path; display c4m files

SYNOPSIS
    c4 cat [-e] [-r] <c4id[/path] | file.c4m>

DESCRIPTION
    With a C4 ID, cat fetches the object and writes its bytes to
    stdout, whatever they are — the store has no object types, so
    what an ID names is knowledge you bring from wherever you got
    the ID (a journal line, a listing entry's kind, a folded
    entry's ID list); cat never guesses. A listing is c4m text, so
    catting a listing shows the directory's one-level children. Forms that must interpret the object — /path
    descent, a trailing slash, -r — parse the fetched bytes as a
    listing and exit 1 if they do not parse; below the root, entry
    kinds come from the listing itself (a trailing-slash name is a
    directory entry). With -r, directory entries are followed by their
    own listings, indented two spaces per level, fetched from the
    store. Every read is verified: the bytes must hash to the
    requested ID, or cat writes nothing and exits 1 — "absent from
    store" and "failed verification" are distinguished on stderr.

    A c4m file path displays the file; patch chains display resolved
    (use c4 log for history). .c4m file arguments and - are whole
    descriptions and never take /path.

PATHS
    An ID may be followed by a slash-separated relative path:
        c4 cat <ID>/src/parser.go
    A path selects entries by name in the stored listings. The same
    rules apply wherever a store address is accepted (cat, restore
    targets, diff sides):

    Address     An argument whose first slash-separated component is
                syntactically a C4 ID — 90 characters, beginning c4,
                all base58 — is a store address; this test precedes
                the .c4m rule, so a store path may end in .c4m (its
                final component then yields bytes, like any file
                entry). Prefix ./ to force a filesystem path that
                merely looks like an ID.
    Splitting   Components split on "/". Empty components, ".", and
                ".." are errors. A single trailing "/" is not a
                component (see Trailing slash).
    Matching    A component matches the entry whose recorded name,
                decoded, is byte-identical to it; directory entries
                match with or without their trailing "/". No globbing,
                no case folding, no Unicode normalization. A folded
                sequence entry matches only its folded name
                (frame.[0001-0100].exr), never a member filename; a
                member filename is an error that names the folded
                entry. If a listing contains both a file x and a
                directory x/, the component x is refused as ambiguous:
                x/ names the directory, and the file is reached by its
                entry's ID.
    Descending  Every component before the last must match a directory
                entry; each level is fetched from the store and
                verified by rehash. A file mid-path is an error: not a
                directory. A symlink mid-path is an error naming its
                recorded target — paths never follow symlinks. Descent
                from the root ID requires the fetched object to parse
                as a listing.
    Final       The last component may name a directory (yields its
                listing), a file (yields its bytes), or a folded
                sequence (yields the object its recorded ID names —
                its ID list, one member ID per line in the folded
                range's order, so line N names the range's Nth
                member; members are fetched by the IDs in that
                list, never by member filename). A symlink as the
                final component is an error naming its recorded
                target: a symlink names no store object. An entry
                whose ID is null cannot be fetched.
    Trailing /  A trailing slash additionally requires the result to
                be a listing: <ID>/src/ yields src's listing and
                errors if src is a file or symlink; <ID>/ requires the
                root object to parse as a listing.

    ID/path is the same walk as: cat the listing, read the entry's ID,
    cat that ID — and works at either knowledge level.

ERRORS
    Exit 1; stderr names the failing component and the reason:
        c4 cat: src2: not found in c461...igYz
        c4 cat: frame.0042.exr: not found in c461...igYz
            (folded entry: frame.[0001-0100].exr)
        c4 cat: hello.txt: not a directory
        c4 cat: vendor: is a symlink -> ../shared/vendor; paths never
            follow symlinks
        c4 cat: c455...Qqdn: object is not a listing
        c4 cat: sparse.bin: entry has null ID
        c4 cat: x: ambiguous: both x and x/ exist; x/ names the
            directory

    Every resolution failure exits 1, and the messages are narration:
    a machine distinguishing these cases does not match stderr text —
    it walks the listings themselves (c4 cat <ID>/, then each level)
    and inspects what is recorded.

FLAGS
    -e, --ergonomic   Column-aligned c4m output
    -r, --recursive   Expand directory entries through the store

EXAMPLES
    c4 cat c461iQnu...igYz                 # object bytes: the listing
    c4 cat c461iQnu...igYz/src/            # a subdirectory's listing
    c4 cat c461iQnu...igYz/src/parser.go > parser.go   # extract by name
    c4 cat -r c461iQnu...igYz              # whole tree, from the store
    c4 cat -r c461iQnu...igYz | c4 paths   # all paths
    c4 cat c45PDQdg...UNN > hello.txt      # extract by ID
    c4 cat c461iQnu...igYz/frame.[0001-0100].exr | sed -n '42p' \
        | xargs c4 cat > frame0042.exr     # folded member 42: line 42
                                           # of the ID list names
                                           # frame.0042.exr
    c4 cat -r ID | c4 id -q -              # recompute: verifies ID

════════════════════════════════════════════════════════════════════════

C4-PATCH(1) / C4-DIFF(1)

SYNOPSIS
    c4 patch [-n N] [-e] <chain.c4m> [<dest.c4m>]
    c4 patch [-n N] [-e] <file.c4m>...
    c4 diff [-e] <old> <new>

DESCRIPTION
    patch resolves patch chains: c4m text in, resolved c4m text out
    (stdout, or <dest.c4m>). patch reads and writes c4m text only; a
    directory argument exits 1 with:
        patch composes descriptions; to change a directory:
        c4 restore <target> <dir>
    -n N resolves to section N (1-based; 0 = final state).

    diff compares two states and emits a patch on stdout. Either side
    may be a directory, a c4m file, or a stored description ID,
    optionally followed by /path (resolved by the rules in c4-cat(1)
    PATHS; the resolved object must be a listing). Directories are
    scanned at content level, or at the other side's level of detail
    when that side is a description, folding exactly the sequences
    that side's folded entries record.

════════════════════════════════════════════════════════════════════════

C4-LOG(1)

NAME
    c4 log - list a patch chain's sections; default: the store journal

SYNOPSIS
    c4 log [<chain.c4m>...]

DESCRIPTION
    With no arguments, log reads the store journal <store>/log.c4m, in
    which the store records every ingest: snapshots from `c4 id -s`,
    stdin blobs from `| c4 -s`, and every pre-image taken by
    `c4 restore --force`. Each journal section is one entry line
    followed by the chain boundary line. With chain file arguments,
    log lists those chains' sections the same way. A store that has
    never ingested has no journal file yet: log lists nothing and
    exits 0 — an absent journal is an empty history, not an error.

OUTPUT
    One line per entry: the 1-based index of the entry's section, a
    single space, then the entry line exactly as recorded (canonical
    c4m: single-space-separated fields; fields never contain unescaped
    spaces). A journal section holds one entry, so the journal prints
    one line per ingest, in append order, oldest first:
    `c4 log | tail -1` is the latest ingest. Output is byte-stable
    per journal file: over any given journal file, a line, once
    printed, is printed identically by every future run. The index is
    a coordinate in the file being listed, never a permanent name —
    splitting the journal installs a new, shorter file whose sections
    count from 1 again (see c4-gc(1)); the entry line itself —
    everything after the index — is immutable history in every file
    that carries it. The first field is the index — the N that
    c4 split accepts — and the last field is the ID. The journal file
    itself is the record; log output is a view of it.

JOURNAL ENTRY
    - <time> <size> <name>[ <- <host>:<path>] <c4id>

    mode    always null (-).
    time    UTC wall clock at append (RFC 3339, seconds).
    size    byte size of the object the entry's ID names: for a
            snapshot, its root listing text; for a blob, the blob;
            0 for the empty description.
    name    the final component of the argument's absolute path, with
            .c4m appended when the ingest produced a description and
            the name does not already end in .c4m. The filesystem
            root records root.c4m. Stdin records `stdin` (raw bytes,
            | c4 -s) or `stdin.c4m` (parsed description, c4 id -s -).
            Names use the grammar's name encoding (my\ dir.c4m).
            Names are history only — never used to look anything up —
            so equal names may repeat freely.
    origin  <host>:<absolute path> of the argument as given; symlinks
            are not resolved. Parsers split at the first colon (a
            Windows drive colon belongs to the path). host is the
            machine hostname's first dot-delimited label, characters
            outside [a-zA-Z0-9_-] replaced by '-', prefixed with 'h'
            when it would not begin with a letter, and 'localhost'
            when empty. Stdin ingests record no origin field.
    c4id    the recorded snapshot or blob ID — always the last field.

    The journal is ordinary c4m text: any text tool can read it, and
    nothing in it is ever overwritten — history only. It only grows;
    growth is one short line per ingest, not data. Appends inspect
    only the file tail and log streams, so cost does not grow with
    history. To shorten it, see c4-gc(1).

    Appends are serialized with an advisory append lock, honored by
    every c4 writer — and c4's own tools are the only sanctioned
    writers of files inside a store, so advisory costs nothing: a
    foreign process that edits store files is corrupting the store,
    and corruption cannot be silent, because every object read is
    verified by rehash and every journal line is a claim you can
    recompute in one pipe (c4 cat -r <ID> | c4 id -q -). Before
    appending, a writer truncates any trailing partial section;
    readers parse complete sections and warn once on stderr if a torn
    tail exists. These properties follow from write ordering alone,
    so any interruption — a killed process, a kernel panic, power
    loss — is one case: a torn tail can only be the residue of an
    ingest that never printed its ID (the entry is appended before
    the ID prints), so nothing claimed is ever missing from the
    journal — and the converse holds after any interruption: every
    complete journal line names a fully durable snapshot, even if its
    command never lived to print. If no append ever follows, a torn
    tail persists harmlessly; every complete section stays readable.

EXAMPLES
    c4 log                            # every ingest this store holds
    c4 log | tail -1                  # the latest ingest
    c4 log | awk '{print $NF}'        # just the IDs
    c4 log /backup/store/log.c4m      # another store's journal

════════════════════════════════════════════════════════════════════════

C4-GC(1)

NAME
    c4 gc - collect store objects unreachable from the roots

SYNOPSIS
    c4 gc [--force] [-v] [<roots.c4m>...]

DESCRIPTION
    Mark-and-sweep. Dry run unless --force. The roots are every ID
    recorded in any section of the store journal — full history, their
    listings at both levels of detail, and everything those name; the
    closure of a folded sequence includes its ID-list object and every
    member ID in it — plus any c4m files given as arguments. Arguments
    add roots; they never replace the journal roots, so nothing
    journaled is ever collected. Objects staged by an interrupted
    ingest that never printed are unrooted and appear in the dry-run
    report; re-running that ingest claims them.

    gc --force takes the store's sweep lock exclusively before it
    reads the journal, and holds it through the sweep. Snapshot
    ingests and restore --force hold the same sweep lock shared for
    their whole run: a sweep waits for work in flight and blocks new
    work until it finishes, so a snapshot that prints — or that shows
    in c4 log — never names missing objects, even if gc --force ran
    concurrently. (Journal appends are serialized separately by the
    append lock; see c4-log(1).) The dry run takes no lock and
    deletes nothing. Locks are per store; gc operates on the write
    store.

    To let old snapshots go, first truncate the journal explicitly:
        c4 split <store>/log.c4m <N> archive.c4m keep.c4m
    install keep.c4m as the journal with snapshot and restore activity
    stopped (the swap is a plain file rename and takes no lock; the
    new journal's sections count from 1 — log's index is a per-file
    coordinate), then run c4 gc --force.

FLAGS
    --force     Delete unreferenced objects (default: report only)
    -v          List each unreferenced object

════════════════════════════════════════════════════════════════════════

TRANSCRIPT

    $ export C4_STORE=~/c4store

    # snapshot a working tree (20,312 files, 4.8 GB)
    # writes only into ~/c4store; no .c4m file or other output is created
    $ time c4 id -s proj/
    snapshot proj/: 20,312 files, 4.8 GB (18,207 objects new) — logged
    c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    real    0m6.1s
    # the first line is stderr narration; stdout is the ID alone
    # ≈ 3.5 s reading + hashing 4.8 GB, ≈ 2.6 s durability overhead

    # same content? two machines, different mtimes and umask
    # (content level is the default; -q just prints the bare ID)
    machineA$ c4 id -q proj/
    c43EUxRwQiCMxU7mKaDhv6kwibMtJsqpxpLg6vLrPrur75jKbPRyysxJvVjhnjeUaBKC8K8ppdGARxS39s5HLx7bdt
    machineB$ c4 id -q /mnt/checkout/proj/
    c43EUxRwQiCMxU7mKaDhv6kwibMtJsqpxpLg6vLrPrur75jKbPRyysxJvVjhnjeUaBKC8K8ppdGARxS39s5HLx7bdt

    # destroy the tree; the store is all that remains
    $ rm -rf proj/
    $ c4 log
    1 - 2026-07-08T09:14:02Z 412 proj.c4m <- abyss:/Users/joshua/ws/proj c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz

    # preview, then apply
    $ c4 restore c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz proj/
    c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
    -rwxr-xr-x 2026-07-08T08:59:10Z 3410 build.sh c459EKo6k9LDYghQsEe6UjE7NN6h3ND91cfj1qoaB4AD6CM4A1Yuvt2DxTRKk9XzotSNziDtwMqYRWDgieZ2fbzf2y
    -rw-r--r-- 2026-07-08T08:59:10Z 12 hello.txt c45PDQdgo41nG9RopnFaMCwBng3fnziEXcJEdPpFh8JeTeLXkHHAji8Zx8fwbwtBV5paRGPUZyrN5yhWLMrmfGyUNN
    [... 20,310 more entry lines ...]
    c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    $ c4 restore --force c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz proj/
    c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
    c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    restored proj/: 20,312 created — undo: c4 restore --force c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT proj/

    # verify the snapshot from the store alone
    $ c4 cat -r c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz | c4 id -q -
    c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz

    # the accident: restored into the wrong directory
    $ c4 restore --force c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz /Users/joshua/notes/
    c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn
    c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    restored /Users/joshua/notes/: 20,312 created, 214 removed — undo: c4 restore --force c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn /Users/joshua/notes/

    # store a raw blob from stdin (journaled as 'stdin', no origin)
    $ tar cz proj/src | c4 -s
    c455XkWdnvqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn

    # scrollback lost? the journal has the pre-image
    $ c4 log
    1 - 2026-07-08T09:14:02Z 412 proj.c4m <- abyss:/Users/joshua/ws/proj c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    2 - 2026-07-08T09:22:41Z 0 proj.c4m <- abyss:/Users/joshua/ws/proj c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
    3 - 2026-07-08T09:31:12Z 18743 notes.c4m <- abyss:/Users/joshua/notes c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn
    4 - 2026-07-08T09:38:09Z 81234567 stdin c455XkWdnvqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn

    # undo: restore the pre-image
    $ c4 restore --force c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn /Users/joshua/notes/
    c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn
    restored /Users/joshua/notes/: 214 created, 20,312 removed — undo: c4 restore --force c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz /Users/joshua/notes/

    # read the store, given one ID (cat prints the object's bytes —
    # a listing is c4m text)
    $ c4 cat c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    -rwxr-xr-x 2026-07-08T08:59:10Z 3410 build.sh c459EKo6k9LDYghQsEe6UjE7NN6h3ND91cfj1qoaB4AD6CM4A1Yuvt2DxTRKk9XzotSNziDtwMqYRWDgieZ2fbzf2y
    -rw-r--r-- 2026-07-08T08:59:10Z 12 hello.txt c45PDQdgo41nG9RopnFaMCwBng3fnziEXcJEdPpFh8JeTeLXkHHAji8Zx8fwbwtBV5paRGPUZyrN5yhWLMrmfGyUNN
    drwxr-xr-x 2026-07-08T09:11:37Z 4799996578 src/ c4292kxKWDo4UZZU7ijvijLJZzRB9aQ8tE8u1v6QpbnxxAKQtxTpGXDoyYDeBEmwkVSDqCYNV3yaRAenUSgDmtHDkE

    # extract one named file by path (each level verified; symlinks
    # never followed)
    $ c4 cat c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz/src/parser.go > parser.go

    # a trailing slash asserts a directory
    $ c4 cat c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz/src/
    -rw-r--r-- 2026-07-08T08:41:27Z 2410 parser.go c49PVGPHf8wE5xQVVq1XdLnpK7S4u2dDxEBxhnbpstSBnqyLtTvdwPhuxHvLd1yn12DKsyPAjTgp47V4gASAdM7pRq
    [... more entry lines ...]

    # a file mid-path is a clean error, exactly like the filesystem
    $ c4 cat c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz/hello.txt/x
    c4 cat: hello.txt: not a directory
    $ echo $?
    1

    # or compose the same walk by hand: list, find the ID, cat it
    $ c4 cat -r c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz | grep ' parser.go '
      -rw-r--r-- 2026-07-08T08:41:27Z 2410 parser.go c49PVGPHf8wE5xQVVq1XdLnpK7S4u2dDxEBxhnbpstSBnqyLtTvdwPhuxHvLd1yn12DKsyPAjTgp47V4gASAdM7pRq
    $ c4 cat c49PVGPHf8wE5xQVVq1XdLnpK7S4u2dDxEBxhnbpstSBnqyLtTvdwPhuxHvLd1yn12DKsyPAjTgp47V4gASAdM7pRq > parser.go

    $ c4 cat c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz/hello.txt
    hello world

    # id never reads the store; cat does
    $ c4 id c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    c4 id: c461iQnu...igYz is a store address; read the store with c4 cat:
    c4 cat -r c461iQnu...igYz | c4 id -q -
    $ echo $?
    1

    # patch never touches directories
    $ c4 patch release.c4m tools/
    patch composes descriptions; to change a directory: c4 restore <target> <dir>
    $ echo $?
    1
