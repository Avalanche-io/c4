# c4 intersect — Find Common Entries Between C4M Files

## Summary

`c4 intersect` finds entries that appear in both of two c4m files, matching by either content identity or path. No default mode — the user must specify `id` or `path`.

## Usage

```
c4 intersect id <a.c4m> <b.c4m>      # match by C4 ID
c4 intersect path <a.c4m> <b.c4m>    # match by path
```

Both arguments can be c4m files or directories (scanned on the fly).

## Output

A valid c4m file containing the entries from the **second** argument that match entries in the first. Parent directories are included to preserve hierarchy.

## c4 intersect id

Find entries in `b.c4m` whose C4 IDs appear anywhere in `a.c4m`, regardless of path.

This is the operation only C4 can do — matching by content identity across different directory structures.

```
$ c4 intersect id found.c4m animatedFeature.c4m
drwxr-xr-x - - shots/
  drwxr-xr-x - - sh010/
    -rw-r--r-- 2026-03-24T10:15:00Z 842K monkey.blend  c46tRvWx...
    drwxr-xr-x - - renders/
      -rw-r--r-- 2026-03-24T11:30:00Z 2.1M render_001.png  c45KgBYE...

$ c4 intersect id found.c4m animatedFeature.c4m | c4 paths
shots/sh010/monkey.blend
shots/sh010/renders/render_001.png
```

Use case: "I found files on a server. Which project are they from, and where do they belong?"

## c4 intersect path

Find entries in `b.c4m` whose full paths also appear in `a.c4m`, regardless of content.

```
$ c4 intersect path v1.c4m v2.c4m
-rw-r--r-- 2026-03-20T10:00:00Z 842K scenes/monkey.blend  c46tRvWx...

$ c4 intersect path v2.c4m v1.c4m
-rw-r--r-- 2026-03-22T14:00:00Z 851K scenes/monkey.blend  c48nYqLm...
```

Use case: "These two versions have the same paths. Show me the entries from the second one." Diffing the two outputs shows what changed at each path.

## Composition

```
# Which of my mystery files are in any project?
for proj in ~/projects/*.c4m; do
  echo "--- $proj ---"
  c4 intersect id found.c4m "$proj" | c4 paths
done

# Files in both v1 and v2, with their v2 metadata
c4 intersect path v1.c4m v2.c4m

# Files in v1 but NOT in v2 (subtract = v1 minus intersection)
c4 diff <(c4 intersect path v1.c4m v2.c4m) v1.c4m
```

## Future extensions

```
c4 intersect name <a.c4m> <b.c4m>   # match by bare filename
c4 intersect size <a.c4m> <b.c4m>   # match by file size
```

These are natural extensions but not needed for the initial release.

## Notes

- No default mode. `c4 intersect` with no subcommand prints usage.
- Output is always a valid c4m from the second argument's perspective.
- Both inputs can be directories (scanned with `c4 id` internally).
- Output composes with `c4 paths`, `c4 diff`, `grep`, etc.
