# @by and @note: Identity Impact Analysis

## Question

Do `@by` and `@note` annotations affect a c4m file's C4 ID? Should they?

## Current Behavior

**No.** `@by` and `@note` do not affect the C4 ID of a manifest or any entry within it.

### Why

C4 IDs for directories (and manifests) are computed from the **canonical form**, which consists solely of entry metadata:

```
mode timestamp size name [-> target] [c4id]
```

The canonical form (`Manifest.Canonical()`) iterates top-level entries, outputs each in canonical format, and hashes the result. It does not include:

- `@by` (who made a change)
- `@note` (human-readable comment)
- `@time` (when a layer was created)
- `@data` (application metadata reference)
- `@base` (base manifest reference)
- `@layer` / `@remove` (layer structure)
- `@intent` (intent flag)

These are all **metadata about the manifest itself**, not about the filesystem it describes.

### The Key Distinction

A c4m file describes a filesystem. Its C4 ID answers: "what filesystem does this describe?" Two manifests describing the same files with the same content, timestamps, modes, and names have the same C4 ID — regardless of who created them, when, or what notes they attached.

This is correct. `@by` and `@note` are annotations on the *act of recording*, not on the *thing recorded*.

## Changing @note on a Directory

**Does changing `@note` on a directory change its C4 ID?** No.

`@note` is a layer-level annotation. It attaches to a `@layer` or `@remove` section, not to individual entries. There is no mechanism to attach a `@note` to a specific directory entry — it's always on the layer.

Even if per-entry notes were added in the future, they should NOT affect identity for the same reason: the note describes the act of observation, not the thing observed.

## Analogy

Consider two photographers who independently photograph the same scene at the same instant with identical cameras. The photographs are identical. One photographer writes "vacation photo" on the back; the other writes "assignment #47." The photographs — the content — are identical. The annotations differ, but the content identity does not.

Similarly, two people scanning the same directory tree independently will produce the same C4 ID, even if they attach different `@by` and `@note` annotations.

## Recommendation

**Keep the current behavior.** `@by` and `@note` should never affect C4 ID.

The separation is philosophically sound and practically important:

1. **Determinism**: The same filesystem always produces the same C4 ID, regardless of who scans it or what they write about it.

2. **Deduplication**: Two manifests from different sources describing the same content can be recognized as equivalent.

3. **Layer composability**: Adding/removing layers with different `@by`/`@note` doesn't change the fundamental identity of what's described.

4. **Trust model**: Identity is a statement about content. Attribution (`@by`) and commentary (`@note`) are statements about provenance. Mixing them would conflate "what is it?" with "who said so?"

## Edge Case: @data

The `@data` annotation at the manifest level (`Manifest.Data`) is also excluded from C4 ID computation. This is correct — `@data` references application-specific metadata that is orthogonal to filesystem content.

However, `@data` blocks embedded in the manifest (for sequence expansion) ARE indirectly reflected in the C4 ID through the sequence entries they support. This is because the sequence C4 ID is computed from the expanded entries, and the data block provides the per-file C4 IDs needed for that expansion.

## Summary

| Annotation | Affects C4 ID? | Should it? | Rationale |
|------------|---------------|------------|-----------|
| `@by`      | No            | No         | Attribution, not content |
| `@note`    | No            | No         | Commentary, not content |
| `@time`    | No            | No         | Layer creation time, not file time |
| `@data`    | No            | No         | Application metadata |
| `@intent`  | No            | No         | Manifest type flag |
| `@base`    | No            | No         | Layer reference |

Entry-level fields that DO affect C4 ID: mode, timestamp, size, name, target, c4id.
