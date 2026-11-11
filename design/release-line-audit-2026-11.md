# Release line and master reconciled (2026-11-11)

On 2026-11-11 master was found to crash with a stack overflow on a symlink to
an ancestor directory. Release 1.0.x had fixed that on 2026-07-29 (9092fb1),
and master never took the fix. That prompted an audit of every commit on
`release/1.0.x` with no patch-identical commit on master.

**Method.** `git cherry origin/master origin/release/1.0.x` listed 59 such
commits. 38 change behaviour. Of those, 8 were backports of master changes
made that day, so master had their equivalents. That left 29 to check. A
commit with no patch-identical twin may still have been reimplemented in
different code, so each was checked by behaviour, never by reading the code:
- one repro per commit, run against a master binary (d4d10f2) and the v1.0.25
  binary;
- the commit's parent built and run too, to prove the repro catches the bug;
- every MISSING fix forward-ported with that repro as its test, failing on
  master.

| commit          | fixes                                                                | master before                                          | verdict                   | landed         |
| --------------- | -------------------------------------------------------------------- | ------------------------------------------------------ | ------------------------- | -------------- |
| 9092fb1         | a symlink cycle yields a nil ID, not a stack overflow                | `up -> ..` and `here -> .` crashed, exit 2             | MISSING                   | #109 (71a6b54) |
| 56c4416         | restore never writes through a symlink in the destination            | wrote `outside/payload.txt` outside the destination    | MISSING (security)        | #111 (dec8ad8) |
| 5b05207         | store content is checked against its ID before it is written         | a tampered object was written over the user's file     | MISSING                   | #113 (dded287) |
| 59ce7cc         | an entry whose depth skips its parent is an error                    | 8 verbs panicked, exit 2                               | MISSING                   | #114 (bc972ee) |
| 6cfba2c         | the library's depth-jump panic wraps ErrInvalidEntry; CLI exits 1    | bare `runtime.boundsError`                             | MISSING                   | #114 (bc972ee) |
| 7bac715         | (b) `id -s -S` stores a folded sequence's ID list                    | the list was never stored                              | MISSING                   | #115 (b130832) |
| 7afd4d8         | reconcile refuses folded sequence entries                            | members deleted, ID list written as a file             | MISSING                   | #115 (b130832) |
| 9a40c14         | TreeStore finds objects under case-collided shard dirs               | "content not found" on case-sensitive APFS             | MISSING                   | #116 (fb64f43) |
| 52fe445         | trust is the client's choice: `restore --force -c` honours the guide | `--force` rewrote a file the dry run trusted           | DIFFERENT (the `-c` half) | #117 (2d88fac) |
| 8950bdb         | racy-file rule                                                       | protected at trust time, not scan time                 | DIFFERENT, correct        | —              |
| 77a1de3         | 1.0.x `-q` prints nothing, by design, and says so                    | `-q` prints the ID (the 1.1 surface)                   | DIFFERENT by design       | —              |
| 0ae600b         | cancelling a scan panicked                                           | in `cmd/c4/internal/scan`, which the CLI does not link | N/A                       | —              |
| 4e0c2f5         | test only                                                            | not on master                                          | N/A                       | —              |
| fdffcab         | test only (null-ID directory shapes)                                 | behaviour fixed on master; test not carried            | N/A                       | —              |
| 6617b53         | directory mtime after a nested change                                | correct; the parent build was correct too (dfe0261)    | could not reproduce       | —              |
| 7bac715         | (a) store and return file bytes unaltered                            | correct                                                | PRESENT                   | —              |
| 15 more (below) | each its own fix                                                     | each measured correct on master                        | PRESENT                   | —              |

The 15 more measured PRESENT: 20c8066, a873508, 368c24b, dfe0261, e9f77e7,
1ef5fcf, af17535, ff2b1f3, c6a1651, bdefb7c, 106c03c, 1417e8d, 852b9da,
4d7ac48 and 85cea32.

**What else the audit found:**
- **`cmd/c4/internal/scan` is unlinked.** The CLI has scanned through the public
  `scan` package since 2026-03-19 (616e211). The cancel panic, the four data
  races and the test deflakes fixed there changed no shipped behaviour. Its
  removal is held for Joshua.
- **The live scanner has no such races.** `go test -race -count=20 ./scan/`
  found 0; its tests drive parallel scans, progress and cancel.
- **On 1.0.25, `c4 patch` of a one-level record emptied the subdirectories it
  lists as one line.** Fixed on release/1.0.x in #110 (7dbad34), for 1.0.26.
- **The CLI tests wrote into the runner's real store**, and on Windows into its
  real profile: `os.UserHomeDir` reads USERPROFILE there, not HOME. A
  `TestMain` guard with a regression test now isolates every `c4` the suite
  starts (#110 on 1.0.x, #112 on master).

**After 2026-11-11:** every behavioural fix on release/1.0.x is on master,
unless the table says DIFFERENT or N/A. Re-run the method before calling the
two lines reconciled again: list with `git cherry`, run one repro per commit
against both binaries, and check that the parent build fails.
