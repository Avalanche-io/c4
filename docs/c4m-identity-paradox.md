# Two IDs for a c4m file: its bytes and its description

*(Rewritten 2026-11-11. Until then this page said C4 re-renders c4m text to
its canonical form before identifying or storing it, so that a c4m file's ID
was the ID of its description, not of its bytes, and it defended that as the
design. That was wrong, and the tools no longer do it. The rule since
2026-11-03 is the one below. The earlier text is in this file's history.)*

## The rule

**A file's C4 ID is the ID of its bytes, and it summons exactly that file.**
A file that holds c4m text is no exception. Identifying it, storing it and
fetching it back by its ID all mean the bytes: `c4 cat <ID>` returns the
file you stored, byte for byte, whatever it contains.

A c4m file also *describes* something: a filesystem. That description has
its own C4 ID. The two IDs answer two different questions, and C4 keeps them
apart.

| Question                                  | The ID                                                                                    | Changes when                       |
| ----------------------------------------- | ----------------------------------------------------------------------------------------- | ---------------------------------- |
| Which file is this?                       | The **byte ID**: SHA-512 of the file's bytes                                              | any byte changes, formatting too   |
| Which filesystem does this file describe? | The **description ID**: the byte ID of the root's canonical one-level listing (see below) | the described tree changes, only   |

## How to get each

The byte ID of any file, a c4m file included:

```
c4 id < tree.c4m          # the bytes read on stdin
c4 id dir/                # every file inside a scanned folder: its entry ID
```

The description ID, which comes out the same however the description was
written:

```
c4 id -q tree.c4m         # a .c4m argument is read as a description
c4 id -q dir/             # the directory the description came from
c4 id dir/ | c4 id -q -   # a description on stdin ("-" means: parse it)
```

All three print the same ID for the same tree. For a flat tree with no subdirectories, the canonical text *is*
the one-level listing, so its byte ID and its description ID are the same
number. For a nested or pretty file they differ, by design.

## What "canonical" and "one-level" mean

A canonical c4m listing has:
- entries sorted: files before directories, natural sort within each group;
- single spaces between fields, no column alignment or pretty-printing;
- timestamps in UTC with a `Z` suffix;
- null fields as a single `-`;
- one entry per line, each ending in a newline.

The description ID hashes the **root's one-level listing** in that form:
one line per direct child, each subdirectory a single line carrying its own
C4 ID (its own one-level listing is a separate object). So two c4m files
that describe the same tree, whatever their formatting, nesting or
indentation, resolve to the same description ID.

## What follows

- **A pretty file and a canonical file are two files.**
  `c4 id -e dir/ > pretty.c4m` and `c4 id dir/ > plain.c4m` have different
  byte IDs and the same description ID.
- **Storing keeps the bytes.** `c4 id -s < pretty.c4m` stores the file's
  bytes under its byte ID, and `c4 cat` of that ID returns the pretty file,
  not a re-rendered one. (`c4 id -s tree.c4m` and `c4 id -s -` read the
  file as a description and store the description.)
- **`c4 cat FILE` is the file.** Without `-e` or `-r`, `c4 cat tree.c4m`
  writes the file's bytes. `c4 cat -e` renders the description in the
  pretty form, and `c4 cat -r` expands collapsed directories from the store.
- **`c4 id FILE.c4m` is the normaliser.** Given a `.c4m` file as an argument,
  `c4 id` reads it as a description and prints it in canonical form; with
  `-q` it prints the description ID. To normalise a file in place:
  `c4 id tree.c4m > tree.tmp && mv tree.tmp tree.c4m`.
- **A c4m file inside a project is just a file.** A scan records its byte
  ID, and two differently formatted copies of the same description are two
  files with two IDs.

## Why

An ID that cannot fetch the thing it names is not an identity. If C4
hashed a re-rendered form, the ID of `pretty.c4m` would name bytes that
exist nowhere on disk, `c4 cat` of that ID would return a different file,
and every tool that checks a file against its ID (a store, a mirror, a
restore, `sha512sum`) would disagree with C4 about the same file. Keeping
the two questions apart costs nothing: the description ID is always one
command away, and it is the ID the diffing, merging and patching verbs
already use.

## Display depth: what `c4 cat` shows

```
$ c4 cat -e project.c4m          # the file as written: nesting included
$ c4 cat -e <root-dir-id>        # a stored listing: direct children only
$ c4 cat -er <root-dir-id>       # the whole tree, fetched level by level
```

A c4m file on disk may inline nested directories; a directory's stored
listing is one level, because that is what its ID hashes. Use `-r` to
expand from the store.

## Through 1.0.26

Through 1.0.26 several tools re-rendered c4m text before hashing or storing
it: `c4 id FILE` (any name whose bytes parsed as c4m), `c4 id` on stdin and
`c4 id -s`, while a file inside a scanned folder already got its byte ID.
The store then held the re-rendered text, so `c4 cat` of that ID did not
return the file, and `c4 cat FILE` printed the re-rendered form. The c4py
port did the same in its identify, scan and store paths, and follows the
same rule from 1.0.27. Canonical text,
such as plain `c4 id` output, was unaffected, since re-rendering it changes
nothing; the pretty form `-e` writes was. From 1.0.27, and in 1.1, `c4`
identifies, stores and cats by the bytes on every path.
