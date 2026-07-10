$ c4 --help
c4 - content-addressed identification and preservation (SMPTE ST 2114)

Usage:
  c4 id [flags] <path>...          Describe files, directories, or c4m files (c4m text to stdout)
  c4 id -s <path>                  Snapshot into the store; print the snapshot ID
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
                Equal exactly when contents are byte-identical, on any
                machine, clock, or umask. This is c4 id's default level;
                print it alone with: c4 id -q
  snapshot ID   the ID of a recorded description: everything observed at
                a moment (modes, times, sizes, names, content). Printed
                by: c4 id -s. The store alone returns all of it.
  Any store-resolved ID may be followed by /path to descend into it by
  entry name:   c4 cat <ID>/src/main.go

Nothing is written anywhere without -s (or restore --force), and -s
writes only inside the store: no .c4m file, nothing in the scanned tree,
nothing in your working directory. -q changes output form (bare ID
instead of c4m text), never what is identified. A snapshot ID is printed
only after everything it stored is on stable media. The store records
every snapshot in its journal: run `c4 log` to list them.

Store: set C4_STORE=/path/to/store, or list stores in ~/.c4/config (the
first store is written to and holds the journal). Default: ~/.c4/store

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

    With -s, id also stores a complete snapshot in the configured store:
    every file's bytes, every directory's one-level listing (root
    included, at both full and content detail), and one entry in the
    store journal. -s writes only inside the store directory: it creates
    no .c4m file, nothing in the scanned tree, nothing in your working
    directory. stdout is then a single line — the snapshot ID — printed
    only after everything stored is on stable media. Narration and
    progress go to stderr: a live file/byte count on a terminal, one
    summary line otherwise.

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
    is the ID of the exact text of the object it names. A directory's
    size field is the total bytes of the files it contains. A single
    file's snapshot ID and content ID coincide: the ID of its bytes.
    ID equality is SHA-512 equality: distinct byte streams share an ID
    only by a SHA-512 collision, and every store read is rehashed
    against the requested ID rather than trusted.
    The ID of the empty description (and of zero bytes) is
    c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT

DESCRIPTIONS AS INPUT
    An argument ending in .c4m, and `-`, are parsed as descriptions;
    a .c4m argument that does not parse is an error. All other file
    arguments are bytes. Files inside a scanned tree are always treated
    as bytes, whatever their name — never parsed or rewritten. A patch
    chain given to id resolves to its final state first (use
    `c4 patch -n N` for earlier states).

    For a description, id computes at the description's own level of
    detail, so its printed ID is the description's own canonical ID
    (for a full description, its snapshot ID). -m c projects a fuller
    description down to content form; fields cannot be added, only
    nulled. Ingesting a description with -s stores its listings; it
    does not store content the description refers to.

FLAGS
    -q, --quiet          Print only the C4 ID, one line per path.
                         -q changes output form, never identity: the
                         level identified is chosen by -m (default:
                         content). `c4 id -q dir` is machine-independent
                         because content is the default, not because
                         of -q.
    -s, --store          Snapshot into the store and print the snapshot
                         ID (implies -q)
    -m, --mode MODE      Scan detail: s, c, m, f (see MODES)
    -e, --ergonomic      Column-aligned output
    -S, --sequence       Detect and fold file sequences
    --exclude GLOB       Exclude pattern (repeatable)
    --exclude-file FILE  File of exclude patterns, one per line
    -c, --continue C4M   Use an existing c4m as a scan guide

DURABILITY
    If the snapshot ID printed, the snapshot — content, listings, and
    its journal entry — survives power loss, even one microsecond after
    printing. If it did not print, nothing was claimed: the scanned tree
    was never written to, the store's prior contents and journal are
    intact, and re-running the same command continues where it left off
    (objects already present are skipped). One in-between case exists:
    if power failed after the journal entry but before the print, the
    snapshot is complete and durable and `c4 log` lists it — whatever
    the journal shows after a crash is real.

    Objects are written to a temporary name, synced, and atomically
    renamed; a device flush barrier precedes the journal entry and a
    second precedes the printed ID. Durability overhead — beyond
    reading and hashing the data, which dominate — is roughly 130
    microseconds per file plus two ~5 ms barriers, measured on an
    Apple-silicon APFS SSD. Worked example: 20,312 files, 4.8 GB is
    about 3.5 s to read and hash plus about 2.6 s of durability
    overhead, roughly 6 s total. On other filesystems the constants
    differ; the ordering contract (synced before renamed, barriered
    before journaled, journaled before printed) does not.

PARTIAL SCANS
    An unreadable file is reported on stderr and recorded with a null
    ID; sockets, fifos, and devices are reported and omitted. The
    description (or snapshot) is still produced, and id exits 2.

FILES
    Store root: $C4_STORE, or the first store in ~/.c4/config, or
    ~/.c4/store. Journal: <store>/log.c4m — an ordinary c4m patch
    chain; one section per ingest. See c4-log(1).

EXIT STATUS
    0 success; 1 error; 2 partial scan (details on stderr)

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
             /path to descend into it by entry name (e.g. SNAP/src); a
             c4m file path (patch chains are resolved); or - (stdin).
             A directory is never a target.
    dir:     the directory to reconcile; created if absent.

DESCRIPTION
    Reconciles <dir> to match <target>: entries are created, moved,
    updated, and removed as needed. A snapshot-ID target restores
    recorded modes and times as well as content; a content-ID target
    restores byte-exact content with platform-default metadata (see
    VERIFICATION). Content is pulled by ID from the store and from
    bytes already present in <dir> (moves are detected).

    Without --force, nothing is written. stdout is the plan in c4m
    patch format: the C4 ID of <dir>'s current state, the entries that
    differ, and the target's ID. The two boundary IDs always appear;
    when <dir> already matches, they are equal and no entries appear.

    With --force:
      1. Every ID the target names must be resolvable; otherwise the
         missing IDs are listed on stderr and restore exits 1 without
         touching <dir>.
      2. If <dir> contains anything a snapshot cannot fully record
         (unreadable files, sockets, fifos, devices), restore refuses,
         lists the offenders on stderr, and exits 1 without touching
         <dir>.
      3. <dir>'s current state is snapshotted into the store — content,
         listings, and journal entry, durable — and that pre-image
         snapshot ID is printed as stdout line 1, always before the
         first modification.
      4. <dir> is reconciled, then verified (see VERIFICATION). Only
         then is the target ID printed as line 2.

VERIFICATION
    Verification is recomputation at the target's knowledge level.
    Snapshot-ID target: recorded modes and mtimes are applied
    explicitly (your umask is irrelevant), then the full-form listing
    is recomputed from the resulting tree and must reproduce the
    snapshot ID. Content-ID target: the content-form listing is
    recomputed and must reproduce the content ID — names, sizes,
    symlink targets, and bytes are verified; modes and times are
    platform-default metadata, meaning whatever file creation under
    your umask and clock produced — not recorded by the target, not
    claimed, and not verified. Line 2 prints only if recomputation
    reproduces the target ID exactly.

UNDO
    Line 1 reverses the operation:
        c4 restore --force <line-1> <dir>
    restores <dir> byte-exact, including recorded modes and times. The
    undo prints its own line 1, so it is itself undoable. There is no
    ref namespace to update or lose: the undo handle is the ID itself —
    printed on stdout, echoed in the stderr undo hint as a complete
    command, and recorded permanently in the store journal. `c4 log` is
    the reflog: a lost terminal does not lose the undo handle.

OUTPUT
    Dry run: the plan (c4m patch format) on stdout.
    --force: exactly two lines on stdout —
      line 1: pre-image snapshot ID of <dir> as it was
      line 2: the target's ID, verified from the resulting tree
    If <dir> already matches the target exactly, both lines are the
    same ID. If <dir> was absent or empty, line 1 is the empty
    description
    c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
    Restoring that ID empties the directory. Counts, warnings, and the
    undo command go to stderr.

DURABILITY
    Line 1 prints only after the pre-image is on stable media and
    always before the first modification. Line 2 prints only after the
    restored tree is verified and synced. If power fails between the
    lines, <dir> holds a mix of old and new whole files (no torn
    files); both end states are safe in the store — re-run the same
    command to finish, or restore line 1 to go back.

NOTES
    A description records names, sizes, modes, mtimes, symlink targets,
    hard links, sequences, and content IDs. It does not record
    ownership, extended attributes, ACLs, or special files; restore
    neither preserves nor restores those. Target entries with null IDs
    cannot be materialized: the rest is restored, the omissions are
    listed on stderr, and restore exits 2. restore never deletes <dir>
    itself: undoing a restore into a previously absent directory leaves
    an empty directory. Concurrent restores of the same directory are
    not coordinated: the store stays safe; the tree's final state is
    undefined.

EXIT STATUS
    0 plan emitted, or reconciled and verified
    1 refused: missing content, uncapturable entries, or unresolvable
      target — nothing modified
    2 reconciled with omissions (listed on stderr)

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
    With a C4 ID, fetches the object from the store and writes it to
    stdout. Every read is verified: the bytes must hash to the
    requested ID, or cat writes nothing and exits 1. A file ID yields
    the file's bytes. A listing ID (snapshot or content) yields the
    one-level child listing; with -r, directory entries are followed by
    their own listings, indented two spaces per level, fetched from the
    store.

    An ID may be followed by a slash-separated relative path:
        c4 cat <ID>/src/parser.go
    Each component descends one level by exact entry-name match in the
    stored listing, every level verified by rehash; the final component
    may name a file (its bytes) or a directory (its listing). This is
    sugar over the equivalent walk — cat the listing, read the entry's
    ID, cat that ID — and works at either knowledge level. An unknown
    name exits 1 with the missing component on stderr.

    A c4m file path displays the file; patch chains display resolved
    (use c4 log for history).

FLAGS
    -e, --ergonomic   Column-aligned c4m output
    -r, --recursive   Expand directory entries through the store

EXAMPLES
    c4 cat c461iQnu...igYz                 # one-level listing
    c4 cat c461iQnu...igYz/src/            # a subdirectory's listing
    c4 cat c461iQnu...igYz/src/parser.go > parser.go   # extract by name
    c4 cat -r c461iQnu...igYz              # whole tree, from the store
    c4 cat -r c461iQnu...igYz | c4 paths   # all paths
    c4 cat c45PDQdg...UNN > hello.txt      # extract by ID
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
    may be a directory, a c4m file, or a stored description ID
    (optionally ID/path). Directories are scanned at content level, or
    at the other side's level of detail when that side is a
    description.

════════════════════════════════════════════════════════════════════════

C4-LOG(1)

NAME
    c4 log - list a patch chain's sections; default: the store journal

SYNOPSIS
    c4 log [<chain.c4m>...]

DESCRIPTION
    Lists one line per section: index, entry fields, section ID. With
    no arguments, reads the store journal <store>/log.c4m, in which the
    store records every ingest: snapshots from `c4 id -s`, stdin blobs
    from `| c4 -s`, and every pre-image taken by `c4 restore --force`.
    Each journal section is one entry:

        - <time> <listing-size> <name>.c4m <- <host:abs-path> <snapshot-id>

    followed by the chain boundary line. The journal is ordinary c4m
    text: any text tool can read it, and nothing in it is ever
    overwritten — history only.

    Appends are serialized with an advisory lock. Before appending, a
    writer truncates any trailing partial section; readers parse
    complete sections and warn once on stderr if a torn tail exists. A
    torn tail can only be the residue of an ingest that never printed
    its ID (the entry is appended before the ID prints), so nothing
    claimed is ever missing from the journal — and the converse holds
    after a crash: every complete journal line names a fully durable
    snapshot, even if its command never lived to print. If no append
    ever follows, a torn tail persists harmlessly; every complete
    section stays readable.

EXAMPLES
    c4 log                            # every snapshot this store holds
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
    listings at both levels of detail, and everything those name — plus
    any c4m files given as arguments. Arguments add roots; they never
    replace the journal roots, so nothing journaled is ever collected.
    Objects staged by an interrupted ingest that never printed are
    unrooted and appear in the dry-run report. To let old snapshots go,
    first truncate the journal explicitly:
        c4 split <store>/log.c4m <N> archive.c4m keep.c4m
    install keep.c4m as the journal (with snapshot activity stopped),
    then run c4 gc --force.

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
    1  2026-07-08T09:14:02Z  366  proj.c4m  <- abyss:/Users/joshua/ws/proj  c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz

    # preview, then apply
    $ c4 restore c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz proj/
    c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
    -rwxr-xr-x 2026-07-08T09:14:02Z 3410 build.sh c459EKo6k9LDYghQsEe6UjE7NN6h3ND91cfj1qoaB4AD6CM4A1Yuvt2DxTRKk9XzotSNziDtwMqYRWDgieZ2fbzf2y
    -rw-r--r-- 2026-07-08T09:14:02Z 12 hello.txt c45PDQdgo41nG9RopnFaMCwBng3fnziEXcJEdPpFh8JeTeLXkHHAji8Zx8fwbwtBV5paRGPUZyrN5yhWLMrmfGyUNN
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
    $ c4 restore --force c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz ~/notes/
    c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn
    c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    restored /Users/joshua/notes: 20,312 created, 214 removed — undo: c4 restore --force c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn /Users/joshua/notes

    # scrollback lost? the journal has the pre-image
    $ c4 log
    1  2026-07-08T09:14:02Z  366    proj.c4m   <- abyss:/Users/joshua/ws/proj  c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    2  2026-07-08T09:22:41Z  0      proj.c4m   <- abyss:/Users/joshua/ws/proj  c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
    3  2026-07-08T09:31:12Z  18743  notes.c4m  <- abyss:/Users/joshua/notes    c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn

    # undo: restore the pre-image
    $ c4 restore --force c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn ~/notes/
    c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    c451RgrpofqN9qu4BdyADTXGYcNckJYHQ4rB3JW7VT3WwgSDk1k3iZGsEJ6XzofwkrJ994sJwtbuqpXe5pLkLyQqdn
    restored /Users/joshua/notes: 214 created, 20,312 removed — undo: c4 restore --force c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz /Users/joshua/notes

    # read the store, given one ID
    $ c4 cat c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz
    -rwxr-xr-x 2026-07-08T09:14:02Z 3410 build.sh c459EKo6k9LDYghQsEe6UjE7NN6h3ND91cfj1qoaB4AD6CM4A1Yuvt2DxTRKk9XzotSNziDtwMqYRWDgieZ2fbzf2y
    -rw-r--r-- 2026-07-08T09:14:02Z 12 hello.txt c45PDQdgo41nG9RopnFaMCwBng3fnziEXcJEdPpFh8JeTeLXkHHAji8Zx8fwbwtBV5paRGPUZyrN5yhWLMrmfGyUNN
    drwxr-xr-x 2026-07-08T10:02:44Z 2147201530 src/ c4292kxKWDo4UZZU7ijvijLJZzRB9aQ8tE8u1v6QpbnxxAKQtxTpGXDoyYDeBEmwkVSDqCYNV3yaRAenUSgDmtHDkE

    # extract one named file by path (each level verified)
    $ c4 cat c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz/src/parser.go > parser.go

    # or compose the same walk by hand: list, find the ID, cat it
    $ c4 cat -r c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz | grep ' parser.go '
      -rw-r--r-- 2026-07-08T09:14:02Z 2410 parser.go c49PVGPHf8wE5xQVVq1XdLnpK7S4u2dDxEBxhnbpstSBnqyLtTvdwPhuxHvLd1yn12DKsyPAjTgp47V4gASAdM7pRq
    $ c4 cat c49PVGPHf8wE5xQVVq1XdLnpK7S4u2dDxEBxhnbpstSBnqyLtTvdwPhuxHvLd1yn12DKsyPAjTgp47V4gASAdM7pRq > parser.go

    $ c4 cat c461iQnu6QW7Bsp866Gb8bcXCMhkYuSugp3B3bPDUK2kSYofnc4ZWvZZmKzFTm4t86mmSytBY8fqYN4SCR7x68igYz/hello.txt
    hello world

    # patch never touches directories
    $ c4 patch release.c4m tools/
    patch composes descriptions; to change a directory: c4 restore <target> <dir>
    $ echo $?
    1
