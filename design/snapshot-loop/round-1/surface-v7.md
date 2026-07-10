$ c4 --help
c4 - content-addressed identification and preservation (SMPTE ST 2114)

Usage:
  c4 id [flags] <path>...              Describe files/directories/c4m files (c4m text to stdout)
  c4 id -s <path>...                   Snapshot into the store; print one snapshot ID per path
  c4 restore [--force] <target> <dir>  Make dir match a description (dry run by default;
                                       --force snapshots dir first, then applies)
  c4 cat [-e] [-r] <id[/path]|f.c4m>   Retrieve a store object by ID or ID/path (verified)
  c4 diff [-e] <old> <new>             Produce a c4m diff; sides: dirs, c4m files, or IDs
  c4 patch [-n N] [-e] <chain.c4m>...  Resolve patch chains to c4m text (never touches directories)
  c4 log [<chain.c4m>...]              List a patch chain's sections (default: the store journal)
  c4 split <file.c4m> <N> <before.c4m> <after.c4m>   Split a chain at section N
  c4 merge | paths | intersect | explain | version   (see c4 explain <verb>)

Shortcuts:  c4 <path> = c4 id <path> (read-only);   c4 -s <path> = c4 id -s <path>
  pipes:  ...|c4  ID of stdin (nothing stored);  ...|c4 -s  store stdin, print ID;  ...|c4 id -  parse

Identity - two concepts:
  content ID    the ID of bytes alone: a file's contents; a directory's child listing with mode
                and timestamp nulled entirely (the exec bit included). Scanned alike (same -S and
                exclusion choices), equal exactly when contents are byte-identical - on any
                machine, clock, or umask. The default level for c4 id; bare ID with -q.
  snapshot ID   the ID of a full description: everything observed at a moment (modes, times,
                sizes, names, content). Printed by c4 id -s; the store alone returns all of it.
  An ID never says which kind it is: compare like with like. A store-resolved ID may take /path
  to descend by recorded entry name (never following symlinks); store addresses are accepted by
  c4 cat, restore targets, and diff sides.

Scripting - the machine contract. stdout is data, byte-pure; stderr is narration: parse nothing
from stderr, and never scrape an ID out of entry text or prose.
  SNAP=$(c4 id -s dir/)              THE snapshot ID: one line, nothing else
  CID=$(c4 id -q dir/)               THE content ID of a file or directory
  c4 id -q saved.c4m                 a description's own ID, at its own detail level
  c4 cat "$ID" >/dev/null            in the store? exit 0 = present AND byte-verified (reads the
                                     whole object - a checked yes; nonzero is one answer: no
                                     verified bytes, absent or damaged alike)
  out=$(c4 restore --force "$T" d/)  two lines, each a bare ID: the undo handle, then the
                                     as-built ID
  c4 restore --force "$(printf '%s\n' "$out" | sed -n 1p)" d/
                                     undo: line 1 is itself a target - and the undo prints
                                     its own undo handle
  c4 log | awk '{print $NF}'         every ID the installed journal lists, oldest first
  c4 cat -r "$ID" | c4 id -q -       recompute any claim instead of trusting it
Exit codes: 0 ok; 1 error - lines already printed remain true claims; 2 completed with declared
partiality on stderr; 3 (restore only) curable - cure, then re-run. In scripts, snapshot one
path per invocation - a multi-path call prints one line per path that SUCCEEDED, so pairing
lines to paths is guesswork unless exit is 0: one path, one line, one exit.

Nothing is written anywhere without -s (or restore --force), and -s writes only inside the store.
A snapshot ID prints only after everything it stored is on stable media - a killed process
(kill -9 included), a kernel panic, and power loss are one case, and a printed ID never dangles:
if it printed, you can get it back. No c4 verb deletes store objects; the journal (c4 log)
records every claim.

Store: a plain directory of hash-named objects plus one journal file (default ~/.c4/store; set
C4_STORE, or list stores in ~/.c4/config - reads consult stores in order; the first is written
and holds the journal). Copy a store anywhere and every printed ID resolves there identically;
the history travels inside it (log.c4m). A git repository's .git is ordinary bytes here -
recorded and restored like any other data.
═══════════════════════════════════════════════════════════════════════════════════════════════
C4-ID(1) - describe files and directories as c4m text; snapshot with -s
SYNOPSIS
    c4 id [-q] [-s] [-m s|c|m|f] [-e] [-S] [--exclude GLOB]... [--exclude-file FILE] <path>...
    c4 id [-q] [-m MODE] -            (parse stdin as a c4m description)
DESCRIPTION
    Writes a plain-text c4m description of each path to stdout. Without -s, id reads only;
    nothing is written anywhere. The printed text is entries, not the identity: THE identity is
    c4 id -q <path> - the C4 ID alone, one line; an ID scraped out of entry text is some entry's.

    With -s, id stores a complete snapshot of each path: every file's bytes, every directory's
    one-level listing (root included, full and content form), the ID-list object of every folded
    entry (-S), and one journal entry per path; a FILE argument stores and journals its bytes.
    -s writes only inside the store - no .c4m file, nothing in the scanned tree or working
    directory. stdout is one line per path that succeeded - the snapshot ID, argument order -
    each printed only after everything that path stored is on stable media. Paths are claimed
    independently and all attempted: earlier snapshots stand if a later path fails; a failed
    path prints no stdout line, so match lines to paths only on exit 0 - in scripts, snapshot
    one path per invocation. A stdout write failure never rolls back store work: the snapshot
    stays claimed and journaled (exit 1; c4 log holds the ID; a re-run reprints it).

    Narration goes to stderr and is never contract: one live line on a terminal, replaced at
    completion by one summary line counting files, bytes, excluded entries, and - on a repeat
    snapshot - store writes skipped; otherwise the summary line alone (isatty(stderr) only).
IDENTITY
    A file's content ID is the ID of its bytes (its two IDs coincide). A directory's ID is the
    ID of the one-level canonical listing of its direct children: stat fields null = content ID
    (identical on every machine); observed modes and times = snapshot ID. Content-form listings
    carry child content IDs; full-form, child full-listing IDs - every ID in a listing is the ID
    of the exact text of the object it names. The content projection nulls mode and timestamp
    entirely, the exec bit included; an ID does not say which projection it is - compare like
    with like, scanned alike.

    Sizes come from the scan: a file's size is the byte count read and hashed; a directory's is
    the total file bytes read beneath it (null-ID entries contribute nothing); a symlink's is
    its target text's length. A symlink entry is its name and target text - mode, time, and ID
    null in every form; it names no store object; null target text when readlink was refused.
    Hard links are not recorded: a hard-linked file scans as an ordinary file. A directory's own
    mode and mtime appear only in its parent's full-form entry, never in its own listing; the
    scan root's appear nowhere. Nothing outside a scanned tree affects its ID.

    Sequences fold only under -S. A folded entry records the folded name and the ID of its ID
    list - member file IDs, one per line, in the folded range's order: line N names the range's
    Nth member. Its size is the sum of member sizes read and hashed; full-form mode = the
    members' shared mode; mtime = the newest member's. A run folds only when every member is a
    plain file, every member's bytes were read and hashed (a folded entry never records a
    null-ID member), and every observed mode is equal; any other run - an unreadable member
    included - records as ordinary entries, identically everywhere. Members are fetched by
    ID-list line, never by member filename (see c4-cat(1) PATHS).

    ID equality is SHA-512 equality; every store read is rehashed, never trusted. The ID of the
    empty description - an empty directory, zero bytes - is
    c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
ARGUMENTS
    Arguments ending .c4m, and -, parse as descriptions (unparseable .c4m errors; patch chains
    resolve to final state, -n N for earlier). Every other file argument is bytes, and files
    inside scanned trees are ALWAYS bytes, whatever their name. A description computes at its
    own knowledge level, so its -q ID is its own canonical ID (-m projects downward only;
    ingesting a description stores its listings, not the content they refer to). An argument
    whose first slash-separated component is syntactically a C4 ID - 90 chars, c4 prefix, all
    base58 - is a store address in every verb (tested before the .c4m rule; prefix ./ to force
    a filesystem path), accepted by c4 cat, restore targets, and diff sides; id never reads the
    store - it exits 1 pointing at:  c4 cat -r <ID> | c4 id -q -

    Filesystem paths resolve with ordinary OS path resolution in every verb, final component
    included: a symlink argument names what it resolves to; an argument resolving to nothing -
    dangling links included - is an error. Symlink-ness is recorded for entries inside scanned
    trees, never for the argument root: scan the parent to record a link as a link. Store-side
    /path and restore's materialization never follow symlinks.
EXCLUSIONS
    --exclude GLOB (repeatable) and --exclude-file FILE are the only exclusion mechanism.
    Nothing is excluded by default; no ignore file - .gitignore included - is ever read, and no
    environment variable adds patterns: c4 records everything it is not told to exclude (.git
    and node_modules included). Patterns are portable globs - *, ?, [...] - with no **; a
    pattern containing / matches the entry's slash-separated path relative to the scan root,
    otherwise its bare name; matching is byte-exact (no case folding, no normalization); a
    matched directory is pruned whole. An exclude file holds one pattern per line; blank lines
    are ignored; there is no comment syntax.

    An excluded entry is simply absent from the listing: exclusion is part of what an ID names,
    so reproducing an ID needs the same exclusions, supplied each run - the snapshot records
    what was scanned, never what was omitted. Where you see them: the stderr summary counts
    excluded entries and active patterns; c4 explain id narrates the effective set. Right after
    a snapshot, c4 diff <SNAP> <dir> lists exactly what was left out; once the tree drifts,
    exclusions and ordinary edits are indistinguishable there. WARNING: restoring a snapshot
    taken with exclusions removes what was excluded from the live tree (the pre-image holds it;
    see c4-restore(1) NOTES). Exclusions never change exit status. restore --force's pre-image
    is always a complete default scan: exclusions never narrow the safety net.
DURABILITY
    If the snapshot ID printed, the snapshot - content, listings, ID-list objects, journal
    entry - survives power loss, even one microsecond after printing. If it did not print,
    nothing was claimed: the tree was never written to, the store's prior contents and journal
    are intact, and re-running continues where it left off. One in-between case: power failing
    after the journal append but before the print leaves a complete, durable snapshot that
    c4 log lists - the journal after a crash is real, and checkable: c4 cat -r <ID> | c4 id -q -.
    All of it follows from write ordering alone: a killed process (kill -9 included), a kernel
    panic, and power loss are one case. A printed ID never dangles: everything it names is
    already on stable media - if it printed, you can get it back.

    The ordering is the contract on every platform: staged bytes reach stable media BEFORE any
    object appears at its hash name; every object a claim depends on is stable and in place
    before its journal entry; the entry is stable before the ID prints. Enforcement per platform:
      macOS/Linux  staged objects get cheap per-file fsync (~126 us measured); one device-cache
                   barrier (~5 ms) makes all staged bytes stable, then renames claim their hash
                   names (Linux: containing directories fsynced); the journal entry is appended
                   and fsynced; a second barrier precedes the print. Measured (APFS, M-series):
                   20,312 files / 105 MB ingests with store in ~4.3 s; materialize ~4.4 s;
                   no-op re-apply ~0.3 s.
      Windows      a directory handle cannot be flushed - no directory or device barrier
                   exists - so the per-file flush IS the durability: each object's bytes are
                   flushed through to media before its rename, each rename is issued with
                   write-through semantics, and the journal append is flushed before the print.
                   Same ordering, same sentences; cost scales per file (write-through costs
                   milliseconds per file, so a first 20k-file snapshot plausibly takes minutes -
                   derived; no Windows constant has been measured).
    On every platform the promise is exactly as strong as its flush primitives: a drive that
    acknowledges a flush it has not performed can lose what was printed.

    A repeat snapshot re-reads and re-hashes every byte - mtimes are never trusted - and skips
    only the store writes of objects already present, counted on the stderr summary line. The
    skip is safe two ways: an object can appear at its hash name only after its bytes reached
    stable media (the barrier precedes every rename), and every later read rehashes, so a wrong
    object at a right name cannot pass silently. Derived, not measured: a re-snapshot after
    touching 3 files pays full read+hash but almost none of the per-file durability work that
    dominates the first run - well under half its time, and on Windows the saving is larger
    still.
PARTIAL SCANS
    An unreadable entry is reported on stderr and recorded with exactly what the scan observed:
    name and kind from its parent's listing, observed mode/mtime in full form, null for
    everything unread - size and ID always. An unreadable directory records no children; an
    unreadable symlink target records null target, size, and ID (a symlink's read is readlink).
    A scan attempts every name its parent enumerates, in canonical order; sockets, fifos,
    devices, and kind-unobservable entries are reported and omitted. Same tree, same access
    outcomes: same listing bytes from every conforming scanner. The description is still
    produced; id exits 2.
FLAGS
    -q  bare ID only, one line per path (output form, never identity; needs an ID-bearing
        level: the default c, or f)
    -s  snapshot into the store; print the snapshot ID (implies -q; always full detail -
        conflicts with -m)
    -m  scan detail: s structure (names) | c content (default) | m metadata (no IDs) | f full
    -e  column-aligned output    -S  fold sequences    --exclude, --exclude-file  see EXCLUSIONS
EXIT STATUS
    0 success; 1 error; 2 partial scan (declared on stderr). Several paths: all attempted - 1
    if any failed, else 2 if any partial, else 0.
EXAMPLES
    SNAP=$(c4 id -s src/)                                   # capture THE ID - no regex
    c4 id -s --exclude node_modules --exclude .venv proj/   # scan choice, reported on stderr
═══════════════════════════════════════════════════════════════════════════════════════════════
C4-RESTORE(1) - make a directory match a description, undo-safely
SYNOPSIS
    c4 restore <target> <dir>              (dry run: print the plan)
    c4 restore --force <target> <dir>      (snapshot dir, then apply)

    target  a store-resolved C4 ID (snapshot, content, or any stored listing ID), optionally
            /path descending by entry name (c4-cat(1) PATHS; must land on a directory entry -
            extract files with c4 cat); or a c4m file (chains resolve); or - (stdin). Must
            resolve to a listing; a directory is never a target (mirror: EXAMPLES).
    dir     the directory to reconcile; created if absent, missing parents included (created
            directories sit outside the recorded tree: platform-default metadata). OS path
            resolution, final symlink followed: a symlink to a directory reconciles the
            directory it names; anything else at the name after resolution - a file, a dangling
            link - refuses: exit 1, nothing modified.
DESCRIPTION
    Reconciles <dir> to <target>: entries created, moved, updated, removed; content is pulled by
    ID from the store and from bytes already in <dir> (moves detected). A snapshot-ID target
    restores recorded modes and times; a content-ID target restores byte-exact content with
    platform-default metadata - modes are not recorded there at all, so exec bits are not
    preserved: snapshot IDs are the recovery handle, content IDs the comparison handle.

    Without --force nothing is written; stdout is the plan in c4m patch form, computed at the
    target's knowledge level and folded as the target folds: line 1 is <dir>'s current ID at
    that level, one line per differing entry, then the target ID. Matching = no entry lines =
    equal boundary IDs. For a snapshot-ID target recording no folded entries, the plan's first
    line equals what --force prints as line 1.

    With --force: (1) every ID the target names must resolve, else listed on stderr - exit 1,
    <dir> untouched; (2) anything in <dir> a snapshot cannot fully record (unreadable files,
    dirs, or symlink targets; sockets, fifos, devices) refuses - exit 1, untouched (a symlink
    with readable target text, broken or not, is recordable); (3) <dir>'s pre-image is
    snapshotted - a complete default scan, nothing excluded, nothing folded - durable and
    journaled before the first destructive operation, printed as stdout line 1; (4) reconcile -
    per directory, entries the target does not record are removed first, then recorded entries
    materialize in listing order, every name claimed create-exclusively - verify by
    recomputation, print line 2.
VERIFICATION
    Every field of line 2 is observed from the filesystem in this run, each at the last moment
    it remains observable: files restore writes are read back and rehashed after sync, before
    their recorded mode lands; files restore kept were hashed by this run's pre-image scan;
    modes and mtimes apply children before parents - so a snapshot whose recorded modes deny
    the restorer read still restores and verifies exit 0. Recomputation folds exactly what the
    target's folded entries record, nothing else. Content-ID targets verify names, sizes,
    symlink targets, bytes; modes/times are unclaimed platform defaults. Exit 0 exactly when
    line 2 equals the target ID.
UNDO
    Line 1 reverses the operation:  c4 restore --force <line-1> <dir>  - byte-exact, recorded
    modes and times included. The undo prints its own line 1, so it is itself undoable, to any
    depth. Handles are IDs, never refs: stdout line 1 (scripts take stdout), echoed as a
    complete command on stderr for a human, and permanent in the journal - c4 log is the
    reflog; a lost terminal loses nothing. No c4 verb deletes store objects, so every undo
    handle keeps working for as long as the store directory exists (c4-log(1) RETENTION).
OUTPUT
    Dry run: the plan. --force: at most two lines on stdout, each a bare ID, nothing else -
      line 1  the pre-image snapshot ID of <dir> as it was (always full form), before the
              first modification; itself a valid restore target - the undo handle
      line 2  the verified, recomputed ID of <dir> as restore left it, at the target's level;
              printed whenever recomputation completed (always on exit 0 and 2)
    Absent or empty <dir>: line 1 is the empty-description constant (c4-id(1)); restoring that
    ID empties a directory. Counts and the undo hint are stderr narration. Capture stdout
    whole, then split (closing the pipe early can interrupt a verb mid-print); a lost line is
    never lost state - line 1 is journaled, and the as-built ID is recomputable:
    c4 id -q -m f <dir>. A failed stdout write never rolls back filesystem work: it exits 3.
DURABILITY
    Line 1 prints only after the pre-image is on stable media; restored files are written
    stage + flush + rename under the same per-platform ordering as c4-id(1) DURABILITY; line 2
    prints only after the result is verified and stable. Interrupted between the lines, <dir>
    holds a mix of old and new whole files - never torn files - with both end states safe in
    the store: re-run to finish, or restore line 1 to go back.
NOTES
    Recorded: names, sizes, modes, mtimes, symlink names/targets, sequences, content. Not
    recorded: ownership, xattrs, ACLs, special files, hard-link structure (a hard-linked tree
    restores as identical bytes, independent inodes). Folded members materialize with the
    entry's one recorded mode and mtime - snapshot without -S when per-member metadata must
    survive. .git is ordinary recorded bytes: destroying and restoring it is the normal case.
    A snapshot taken with exclusions records the excluded names nowhere, so restoring it
    REMOVES them from <dir> like any unrecorded entry - the pre-image holds them.

    An incomplete target entry - null recorded ID, or a symlink with null target text: the mark
    of something unreadable at scan time - is kept as found: nothing created, modified, or
    removed at that name. A destination that folds two recorded sibling names (case-insensitive
    or normalizing filesystems) keeps the first claimant in listing order; each later folded
    sibling - and any recorded name the destination cannot spell at all (invalid encoding,
    forbidden characters, reserved names, length) - is a declared unrepresentable omission:
    nothing materialized, listed on stderr, exit 2. Only the name decides that class;
    permission, space, and I/O failures are curable and exit 3. Re-running cannot close a
    declared omission. Store-side matching never folds names; destinations may.

    A snapshot can record modes that deny later reads; restore verifies before such modes land,
    but a later re-scan or restore there needs read granted first - that chmod is honest
    history in the next pre-image. Concurrent restores of one directory are uncoordinated: the
    store stays safe; the tree's outcome is undefined. restore never deletes <dir> itself:
    undoing a restore into a previously absent <dir> leaves it empty.
EXIT STATUS
    0 plan emitted, or reconciled and verified equal to the target
    1 refused before any modification - nothing modified
    2 reconciled with declared omissions, each listed on stderr (incomplete entries kept as
      found; name-folded or unspellable names, nothing materialized); line 2 printed and true;
      re-running cannot close these
    3 curable failure after line 1 (permission, space, transient I/O, concurrent mutation, a
      failed store read or stdout write): line 1's undo stands; line 2 prints if recomputation
      completed; cure, then re-run. No permanent condition routes here.
EXAMPLES
    out=$(c4 restore --force "$SNAP" proj/)
    undo=$(printf '%s\n' "$out" | sed -n 1p)       # line 1 = the undo handle
    c4 restore --force "$undo" proj/               # undo; prints its own undo
    c4 restore --force "$(c4 id -s good/)" bad/    # mirror good into bad
    c4 id -q -m f proj/                            # recompute line 2 independently
═══════════════════════════════════════════════════════════════════════════════════════════════
C4-CAT(1) - retrieve store objects by ID or ID/path; display c4m files
SYNOPSIS
    c4 cat [-e] [-r] <c4id[/path] | file.c4m>
    -e ergonomic (column-aligned)     -r recursive (expand via store)
DESCRIPTION
    With a C4 ID, cat writes the object's bytes to stdout, whatever they are: the store has no
    object types - what an ID names is knowledge you bring from wherever you got the ID (a
    journal line, a listing entry's kind, an ID list); cat never guesses. A listing is c4m
    text, so catting a directory's ID shows its one-level children; -r expands directory
    entries through the store; forms that must interpret the object (/path, trailing slash, -r)
    parse it as a listing, exit 1 if it does not parse. A .c4m file path displays the file,
    chains resolved. Every read is verified: the bytes must hash to the requested ID or cat
    writes nothing and exits 1. "Absent" and "failed verification" are distinguished on stderr
    only, for a human - to a script both are one answer (no verified bytes; do not trust the
    ID): the why is diagnosis, never contract.

    Membership test, verified:   c4 cat "$ID" >/dev/null
    Exit 0 means present AND intact - cat reads and rehashes the whole object (the cost is the
    object's size); that is what makes the yes checked rather than asserted. A nonzero probe
    does not say why: absent and damaged are one answer to a consumer; the difference is
    stderr narration.
PATHS
    <ID>/a/b selects entries by recorded name, byte-exact: no globbing, case folding, or
    normalization; empty components, ".", ".." are errors. Non-final components must match
    directory entries, each level fetched and rehash-verified; a file mid-path is an error; a
    symlink mid-path is an error naming its recorded target - recorded paths never follow
    symlinks. Final component: directory = its listing; file = its bytes; folded sequence = its
    ID list (line N is the range's Nth member; members fetch by those IDs, never by member
    filename); symlink = error naming its target; null-ID entry = error. Both x and x/ present:
    bare x is refused as ambiguous. A trailing slash asserts a listing. Every resolution
    failure exits 1 with human-level stderr; a machine distinguishing cases walks the listings.
EXAMPLES
    c4 cat "$SNAP"                             # the root listing, one level
    c4 cat "$SNAP"/src/parser.go > parser.go   # extract by name, verified
    c4 cat -r "$SNAP" | c4 paths               # every path in the snapshot
    c4 cat "$SNAP"/frame.[0001-0100].exr | sed -n 42p | xargs c4 cat \
        > frame0042.exr                        # folded member 42 = line 42
    c4 cat -r "$SNAP" | c4 id -q -             # recompute: verifies SNAP
═══════════════════════════════════════════════════════════════════════════════════════════════
C4-LOG(1) - list a patch chain's sections; default: the store journal
SYNOPSIS
    c4 log [<chain.c4m>...]
DESCRIPTION
    With no arguments, log reads the store journal <store>/log.c4m - an ordinary c4m patch
    chain in which the store records every claim it makes: each c4 id -s snapshot, each stdin
    blob, every pre-image taken by restore --force. An absent journal is an empty history: log
    lists nothing, exits 0. Chain arguments list those files the same way - a copied store's
    journal included.
OUTPUT
    One line per section, append order, oldest first: the 1-based section index (what c4 split
    takes), a single space, then the entry exactly as recorded - canonical c4m; fields never
    contain unescaped spaces, so field extraction is safe. First field = the index; last field
    = the ID. Byte-stable per journal file: a line, once printed from a given file, reprints
    identically forever (split renumbers indexes; the entry text is immutable history).
        c4 log | tail -1              # the latest claim
        c4 log | awk '{print $NF}'    # the IDs - fields, never regex
JOURNAL ENTRY
    - <UTC time> <size> <name>[ <- <host>:<abs-path>] <c4id>
    mode always null; time = RFC 3339 seconds at append (provenance, never identity); size =
    bytes of the object the ID names; name = final component of the argument's path (.c4m
    appended when the ingest produced a description; root.c4m at /; stdin or stdin.c4m for
    pipes, which record no origin); origin = the argument as given, symlinks unresolved
    (parsers split at the first colon - a Windows drive colon belongs to the path). Names are
    history, never resolved; equal names repeat freely.

    Appends are serialized by an advisory lock that dies with its holder - an OS lock on the
    journal file, never a lockfile whose existence blocks - so a killed writer never wedges the
    store. A writer truncates a torn tail before appending; readers parse complete sections and
    warn once. The entry is flushed before the ID prints, so a torn tail can only belong to an
    ingest that never printed, and after any interruption every complete line names a fully
    durable snapshot - recompute any line's claim: c4 cat -r <ID> | c4 id -q -. A line proves
    its claim, not its file's completeness: c4's tools are the only sanctioned writers inside a
    store; foreign edits are corruption, and object reads rehash, so object corruption is loud.
    Growth is one short line per claim, not data; appends inspect only the tail; log streams.
RETENTION
    Nothing expires. No c4 verb deletes store objects: every journaled ID and everything it
    names stays restorable for as long as the store directory exists, and nothing a journal
    line reaches will ever be collectable - a future collection verb (none ships) must take the
    installed journal as its complete root set. To shorten the listing:
        c4 split <store>/log.c4m <N> archive.c4m keep.c4m
    and install keep.c4m as the journal with ingest activity stopped (a plain whole-file swap -
    the one sanctioned by-hand operation inside a store; sections count from 1 again). Splitting
    changes what c4 log lists, never what the store retains; the archive remains part of the
    store's root record - keep it. Reclaiming space today means deleting a whole store yourself.
═══════════════════════════════════════════════════════════════════════════════════════════════
C4-DIFF(1) / C4-PATCH(1)
    diff compares two states - directories, c4m files, or store IDs (ID/path allowed; the
    resolved object must be a listing) - and emits a c4m patch on stdout; directories scan at
    content level, or at the other side's level when that side is a description, folding
    exactly what it folds. c4 diff <SNAP> <dir> right after a snapshot shows everything the
    snapshot left out. patch resolves chains - c4m text in, c4m text out (-n N picks a
    section) - and never touches directories; a directory argument exits 1 with:
        patch composes descriptions; to change a directory: c4 restore <target> <dir>