# Cross-implementation vector: backslashes in names and link targets (2026-11-11)

The shared conformance fixture for reading a backslash back as itself. A
backslash is a literal character in a c4m name or link target: only `/`
separates paths (Joshua, 2026-11-10). SafeName writes it as `\\`. Every c4m
implementation MUST pass this vector.

## Files

| File            | Made by                                                                  |
| --------------- | ------------------------------------------------------------------------ |
| `canonical.c4m` | the Go encoder, canonical form (`TestBackslashNameVector`)               |
| `pretty.c4m`    | the Go encoder, pretty form, timestamps rendered in UTC                  |
| `patch.c4m`     | `EncodePatch` over the empty tree: the empty tree's ID, then every entry |
| `root-id.txt`   | `ComputeC4ID` of the manifest all three files describe                   |

No line was written by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestBackslashNameVector` rewrites the files from the library.

## What a conforming implementation MUST do

Decode `canonical.c4m` and `pretty.c4m` to one manifest whose root ID is
`root-id.txt`, and `patch.c4m` to one section over the empty tree's ID, with
exactly these names and targets:

| Name (as on disk) | Written as  | Note                                      |
| ----------------- | ----------- | ----------------------------------------- |
| `plain.txt`       | `plain.txt` | control                                   |
| `trail\`          | `trail\\`   | ends in a backslash                       |
| `end\\`           | `end\\\\`   | ends in two backslashes                   |
| `a\ b`            | `a\\\ b`    | a backslash before a real (escaped) space |
| `link`            | `link`      | a symlink whose target is `target\`       |
| `dir\x/`          | `dir\\x/`   | a directory, holding `in.txt`             |

## The rule under test

A backslash in a name or target always starts a two-byte escape. A c4m
field-boundary escape (`\ `, `\"`, and in names `\[` and `\]`) becomes its
literal character; any other escape, such as SafeName's `\\`, passes through
whole to UnsafeName. Through 1.0.25 the Go decoder consumed only the
backslash of a SafeName escape, so its second byte started a new escape with
the field separator after it: `trail\\` then a space read back as `trail\ `
(or, in the library, as the name with its C4 ID swallowed), a different name
with no error, and a link target ending in `\` gained the following field.
