# Store layout — lowercased shard directories

Status: **draft conclusions from a partial study, 2026-10-03. Not attacked,
not decided.** The study was stopped at the end of the working session with
two of three grounding agents complete (compatibility, portability); the
scale-measurement agent, the drafting agent and the three adversaries did
not run. Their completed output is in `store-layout-partial-results.json`
beside this file. Everything marked "run" was executed on this Mac
(case-insensitive APFS); case-sensitive behavior was emulated, not run on a
real Linux filesystem.

## The problem

The 1.0.x store finds an object by walking a trie of 2-character
directories named from successive pairs of ID characters
(`<store>/c4/4E/<ID>`), flat until 4,096 objects, each leaf splitting again
above 4,096. IDs are case-sensitive; default macOS volumes are not. On a Mac
`4e` and `4E` are one directory, named by whichever was created first (in
practice the uppercase form, because split creates directories in sorted
order). Copied to Linux, about 40% of objects were unreachable. v1.0.18
added a tolerant lookup as a workaround.

## The rule (Joshua and the lead reached it independently, 2026-10-03)

Joshua: "the rule is probably that a store is toLower(id[0-6]) or something
like that, for directory names in the store's shards for all filesystems
regardless." And on scale: "c4 should support any size filesystem that one
might ever create... without making it unruly in the much more common low
end."

**Conclusion from the evidence: keep the adaptive 2-character trie and
lowercase its directory names.** Read as pairs, `toLower(id[0:6])` is
exactly the first three levels of that trie:

```
c44EGr…  →  <store>/c4/4e/gr/c44EGr…      (levels appear only as a store grows)
```

- Directory names are the ASCII-lowercased ID characters, two per level.
- The object file keeps its full exact-case 90-character ID as its name.
- A store starts flat (`<store>/<ID>`, no directories) and deepens only when
  a directory passes the split threshold. The small store stays tidy: zero
  shard directories up to 4,096 objects, then 182.
- On an object store the key is the exact-case ID: the depth-zero case of
  the same rule. No change to the S3 store.

## Why this and not a fixed prefix

| Candidate | Existing readers | Low end | High end |
|---|---|---|---|
| Lowercased adaptive 2-character trie | 1.0.18 reads it on every filesystem (run on the Mac; emulated case-sensitive). master, c4py and imgbin read it on a Mac. | Flat, then 182 dirs | No ceiling from the layout |
| `toLower(id[0:4])`, one level | Not readable by any existing tool | 182 dirs | Fails on FAT32 near 1.5M objects, ext4 near 7–10M per directory |
| `toLower(id[0:5])`, one level | Not readable | 6,370 dirs | Same ceilings, 35x later |
| `toLower(id[0:6])`, one level | Not readable | 120,568 dirs for 194,127 objects, 71,131 of them holding one object; full walk 35x slower (measured) | Exceeds FAT32's entry cap and ext3's subdirectory cap |
| Two fixed 2-character levels | Not readable | About 120k leaf dirs at 194k objects | Still a ceiling |

With either fixed layout every existing writer silently writes a second
copy flat at the store root (run), and migration moves every object. The
lowercased trie migrates a Mac store with 120 case-only directory renames
and no object moves (0.05 s on a 4,201-object copy; 300 of 300 sampled
objects read back with 1.0.18 and with master).

## Constraints the rule must state

1. **Every level is exactly 2 characters.** No 2-character name can be a
   Windows reserved device name. A 3-character level could spell `con`,
   `prn`, `aux`, `nul`; a 4-character one `com1`–`lpt9`.
2. **Fold to lowercase, ASCII A–Z only.** Base58 has no `I`, so lowercasing
   is locale-proof; uppercasing `i` is not (Turkish and Azeri locales).
3. **Split threshold at or below about 7,800** if FAT32 is ever a target (a
   90-character name takes 8 directory entries). 4,096 has a 2x margin.
4. **Split moves only names that parse as C4 IDs**, and the store root
   reserves non-ID names (`journal`, `log.c4m`, temp files).
5. Case sensitivity cannot be inferred from the OS or filesystem type
   (ext4 casefold, ZFS, per-directory NTFS), so "for all filesystems
   regardless" is the right scope.

## Candidate invariant: an object is valid at any depth along its path

Proposed by the lead, evaluated by one agent, **not attacked**: an object
may sit at any level of its own lowercased prefix path (`<store>/<ID>`,
`<store>/c4/<ID>`, `<store>/c4/4e/<ID>`, …); readers check each level;
writers use the deepest directory that exists.

- True as a design, and a reader-only change. Existing writers already
  follow its write rule.
- **No existing reader honors it.** Run: an object left above the deepest
  existing directory returns "content not found" in 1.0.18, master and
  c4py, and storing it again makes a deeper duplicate. So today a crash in
  mid-split, or rsync of a small flat store into a split one, hides objects.
- Derived cost, not run: a hit is unchanged (4 stats at depth 2); a miss is
  about 6, against 12–13 for the 1.0.18 fallback.
- It requires `Remove` to delete at every level and `Walk` consumers to
  tolerate duplicates. It makes split optional outside Go (imgbin already
  never splits) and makes the flat `Folder` store simply depth zero.
- On FAT32 the split stays mandatory before about 7,800 files per
  directory.

## Scale: where the layout stops and what takes over

The trie has no ceiling of its own: about 3 levels below `c4/` at 10^12
objects, 12 extra path characters; Windows MAX_PATH is not binding. The
ceiling is the filesystem: ext4 and NTFS hold about 4.29 billion files per
volume, so 10^12 objects cannot live on one such volume under any layout.
Beyond that the organization is XFS or ZFS, several volumes or buckets each
owning a set of lowercased prefixes (the same rule one level up), or an
object store keyed by the ID. Prefix routing across stores would be new
code: `MultiStore` is ordered search today.

## Defects found on the way (independent of the decision)

- **master: the root split moves the journal.** When the store root passes
  4,096 objects, split moves `journal` to `<store>/jo/journal` and `c4 log`
  loses the earlier entries (reproduced). v1.1 blocker.
- **master lacks the 1.0.18 tolerant lookup**, so master on Linux still has
  the original 40% bug.
- **rsync with `--delete-after` or `--delete-delay` between two
  case-insensitive stores whose shard directory names differ only in case
  deletes the whole shard at the destination, exit 0** (run with rsync
  3.5.0 and Apple's openrsync). Plain `rsync -a` merges but keeps the old
  spelling. Stores must be normalized by c4 on each machine, not by rsync.
- **1.0.18 on a case-sensitive filesystem writing into a lowercased store**
  puts new uppercase-segment objects loose in `c4/` (still readable to it;
  a later split would recreate exact-case sibling directories). Linux
  stores should not be normalized until the rule ships in a release.
- **Three other layouts exist and should converge or go:** `c4ts` writes
  `c4/<88 characters without the c4 prefix>` and can read no Go store;
  `c4d` uses `<root>/<id[3:5]>/<ID>`; `store/sharded.go` (`ShardedFolder`)
  is a third exact-case layout in the Go package. c4py and imgbin are
  exact-case ports of the trie with no tolerant lookup.

## Cost to adopt (from code reading)

About two lines per trie implementation: Go `treestore.go` `path()` and
`split()`; c4py `store.py` lines 166 and 198; imgbin `Store.swift` line 73;
c4ts `tree-store.ts` needs a rewrite regardless; c4d adopts the shared
package or the rule when revived; delete `store/sharded.go`. One shared set
of ID-to-path conformance vectors. The any-depth reader adds a short loop
per reader.

## Not done

- Scale measurements on synthetic stores (the agent was stopped mid-run),
  including the measured stat cost of the any-depth reader.
- The precise drafted rule with conformance vectors.
- All three adversarial passes (portability, mixed old and new writers,
  simplicity). **Nothing above has been attacked.**
- Any run on a real case-sensitive filesystem, Windows, FAT/exFAT, SMB/NFS
  or a cloud sync client.

## Decisions for Joshua

1. Adopt "lowercased adaptive 2-character trie" as the rule (versus a fixed
   prefix, which is a clean break with every existing store and tool).
2. Adopt the any-depth invariant, after it has been attacked.
3. Which release carries it: v1.1 only, or also a 1.0.19 so released
   readers and writers agree before any Linux store is normalized.
4. Whether normalizing an existing store is automatic on first write by a
   new tool, or an explicit step.
