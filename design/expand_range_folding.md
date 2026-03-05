# @expand and Range Folding Assessment

## Context

When `c4 scan` encounters numbered files (e.g., `comp.0001.exr` through `comp.2400.exr`), the default behavior is to fold them into a range expression: `comp.[0001-2400].exr`. This is valuable — 2,400 lines become 1.

But some use cases need the individual entries. A CLI flag (`--no-fold`) is too coarse: with thousands of ranges in a scan, only some may need different treatment. This document assesses the `@expand` annotation as the mechanism for per-range folding control within c4m files.

## 1. Direction: `@expand`, not `@nofold`

**Recommendation: `@expand` only. No new directive needed.**

The spec already defines `@expand` (SPECIFICATION.md lines 289-298). It provides inline expansion of a range:

```
-rw-r--r-- 2023-01-01T12:00:00Z 10000 frames.[01-03].exr c4XYZ...

@expand c4XYZ
-rw-r--r-- 2023-01-01T12:00:00Z 3000 frames.01.exr c4AAA...
-rw-r--r-- 2023-01-01T12:00:00Z 3500 frames.02.exr c4BBB...
-rw-r--r-- 2023-01-01T12:00:00Z 3500 frames.03.exr c4CCC...
```

`@expand` is the right mechanism for three reasons:

1. **It provides data, not just a flag.** An `@nofold` annotation says "don't fold" but provides nothing. `@expand` says "here are the individual entries" — actionable information.

2. **It works at the right point in the pipeline.** Folding is a scan-time decision. Once entries are in a c4m file, the question isn't "should I fold?" — it's "do I have the expansion?" `@expand` answers that question.

3. **It already exists in the spec.** Adding `@nofold` would create two overlapping mechanisms for the same concern. One is enough.

**The semantic**: The presence of `@expand` for a range means the individual entries are available. Tools SHOULD present expanded entries when `@expand` is present and compact ranges when it is not.

**Scan-time control** remains a CLI concern:
- `--no-fold` or `--expand-all` — don't fold any sequences during scan
- `--fold` (default) — fold sequences during scan
- Per-pattern control (e.g., `--no-fold "*.exr"`) could be added later as a CLI feature, independent of the c4m format

## 2. Scope: Per-Range via C4 ID Reference

`@expand` references a specific range by its C4 ID:

```
@expand c4XYZ
```

This is per-range granularity. Multiple `@expand` blocks can coexist in one c4m file for different ranges:

```
-rw-r--r-- ... comp.[0001-2400].exr c4ABC...
-rw-r--r-- ... plate.[0001-0500].exr c4DEF...

@expand c4ABC
... (2400 individual entries for comp)

# plate range stays folded — no @expand block
```

Per-range is the right scope because:
- **Per-directory is too coarse** — you might want some ranges folded and others expanded within the same directory
- **Per-pattern is redundant** — each range IS a pattern; referencing by C4 ID is more precise
- **Per-entry is per-range** — each range entry has exactly one C4 ID

## 3. Round-Trip Behavior

### Current State: NOT Stable

The scan → expand → re-scan cycle is currently lossy:

**Metadata loss during expansion** (`sequence.go:496-498`):
```go
expandedEntry := &Entry{
    Size:      -1,              // Individual size unknown from range
    Timestamp: entry.Timestamp, // Shared timestamp (not individual)
}
```

When expanding via `@data` (ID-list-only blocks), individual sizes become null (-1) and all entries share the range's aggregated timestamp. Re-scanning these expanded entries produces different aggregated metadata.

### With `@expand`: Lossless Round-Trip

The spec's `@expand` format uses **full entries** (not just C4 IDs):

```
@expand c4XYZ
-rw-r--r-- 2023-01-01T12:00:00Z 3000 frames.01.exr c4AAA...
-rw-r--r-- 2023-01-01T12:01:00Z 3500 frames.02.exr c4BBB...
-rw-r--r-- 2023-01-01T12:02:00Z 3500 frames.03.exr c4CCC...
```

Each expanded entry preserves its individual mode, timestamp, size, and C4 ID. This makes the round-trip lossless:

1. **Scan** → individual entries with full metadata
2. **Fold** → range entry (aggregated metadata) + `@expand` block (individual metadata preserved)
3. **Expand** → recover individual entries from `@expand` block with original metadata
4. **Re-fold** → same range entry (aggregation produces same result from same inputs)

**Idempotency**: fold(expand(fold(entries))) = fold(entries), because `@expand` preserves the exact entries that produced the aggregation.

### `@expand` vs `@data`: Complementary, Not Competing

The two mechanisms serve different purposes:

| | `@data` (ID list) | `@expand` (full entries) |
|---|---|---|
| **Content** | C4 IDs only | Full entries (mode, timestamp, size, name, C4 ID) |
| **Size** | Compact (~90 bytes/entry) | Verbose (~120+ bytes/entry) |
| **Use case** | Content verification, deduplication | Round-trip fidelity, per-file metadata |
| **Metadata preserved** | C4 IDs | Everything |
| **Round-trip stable** | No (sizes, timestamps lost) | Yes |

A range can have both:
```
-rw-r--r-- ... frames.[0001-1000].exr c4SEQ...

@data c4IDLIST
c4AAA...
c4BBB...
... (compact ID list for storage/transfer)

@expand c4SEQ
-rw-r--r-- ... 3000 frames.0001.exr c4AAA...
-rw-r--r-- ... 3500 frames.0002.exr c4BBB...
... (full entries for metadata fidelity)
```

In practice, if `@expand` is present, `@data` is redundant (the C4 IDs are extractable from the expanded entries). Implementations should prefer `@expand` when individual metadata matters and `@data` when only C4 IDs are needed.

## 4. Identity Impact — DECIDED: Option A

### Decision

**Option A is correct.** Folded and unfolded forms have different C4 IDs. This was decided by Joshua on 2025-03-05 with the following reasoning:

1. **Folding is lossy.** Individual per-frame metadata (timestamps, sizes) aggregates into a single line. The folded form is a different description than the unfolded form — different text, different identity. Claiming otherwise would be dishonest.
2. **Once folded, identity is idempotent.** Hydrate (via `@expand`) and re-scan always produces the same folded ID. The round-trip is stable after the first fold.
3. **If individual per-frame metadata matters, don't fold.** That's what `@expand` and the unfolded form are for. The user chooses the representation that matches their needs.
4. **Option B is rejected.** It would pretend metadata loss didn't happen, which violates C4's honesty-about-content philosophy.

### Key Finding: Folding Changes Parent Directory Identity

This is the most critical finding. A directory containing 3 individual files has a **different C4 ID** than the same directory with those files folded into a range.

**Individual entries → parent canonical form:**
```
-rw-r--r-- 2025-01-01T10:00:00Z 3000 frame.001.exr c4AAA...
-rw-r--r-- 2025-01-01T10:03:00Z 2800 frame.002.exr c4BBB...
-rw-r--r-- 2025-01-01T10:05:00Z 3200 frame.003.exr c4CCC...
```

**Range entry → parent canonical form:**
```
-rw-r----- 2025-01-01T10:05:00Z 9000 frame.[001-003].exr c4SEQ...
```

These are different texts → different C4 IDs for the parent directory. This means `c4 scan dir/` produces different directory IDs depending on whether sequence detection is enabled.

### Why This Is Correct

Ranges ARE containers. The spec says sequences are "virtual containers" (SPECIFICATION.md line 235: "This treats the sequence as a virtual container of its members"). A directory with a range is structurally different from a directory with individual files — because folding is lossy, aggregating per-frame metadata into a single line. Identity reflects structure, and different structures produce different identities.

The "same filesystem → same identity" intuition is misleading here. The c4m file describes the filesystem at a chosen level of detail. Folded and unfolded are two different (both valid) descriptions. Neither is wrong — they capture different amounts of metadata. C4 is honest about what each description contains.

### Implications

1. **`--fold` vs `--no-fold` produces different directory C4 IDs.** This is expected and correct. Document it clearly so users understand the trade-off.
2. **`@expand` blocks do NOT affect identity.** The `@expand` section is metadata — it enriches the c4m file but doesn't change C4 IDs. Only the range entry line (or individual entry lines) participate in identity computation.
3. **Idempotency after folding.** Once a description is folded, it stays stable: fold(expand(fold(entries))) = fold(entries), because `@expand` preserves the exact entries that produced the aggregation.

## 5. Concrete Test Cases

### Case 1: Partial Delivery with Different Frame Status

**Scenario**: VFX studio delivers `comp.[0001-1000].exr`. QC approves frames 1-500, rejects 501-600 (re-render needed), and 601-1000 are pending review.

**Problem with folding**: The single range `comp.[0001-1000].exr` can't express per-frame status. Layers could annotate subsets, but the folded form hides which frames are in which category.

**With `@expand`**: Individual entries in the `@expand` block can be combined with `@layer` annotations to mark status per-frame or per-sub-range.

### Case 2: Gap Preservation with Semantic Meaning

**Scenario**: A render job assigns frames 1-100. Frames 45-50 fail. The manifest shows `comp.[0001-0044,0051-0100].exr`.

**Problem with folding**: The discontinuous range notation preserves the *fact* of missing frames but not the *expectation*. There's no way to tell from the range alone that 100 frames were assigned and 6 are missing — versus a deliberate 94-frame sequence.

**With `@expand`**: The expansion lists all 94 frames individually. Combined with `@intent` or `@note`, the expected full range can be documented alongside the actual delivery.

### Case 3: Re-Render Provenance

**Scenario**: Initial render produces frames 1-1000. Supervisor requests re-render of frames 500-600 with different lighting. Same filenames, new content.

**Problem with folding**: `comp.[0001-1000].exr` is unchanged — same range, same count. But frames 500-600 now have different C4 IDs, different timestamps, possibly different sizes. The folded form aggregates timestamp to the latest (now from the re-render) and sums sizes, hiding which frames actually changed.

**With `@expand`**: Individual entries show exactly which frames have different C4 IDs and timestamps, making the re-render visible.

### Case 4: Selective Materialization

**Scenario**: An editor needs only frames 100-200 for a rough cut. The c4m file describes the full sequence `comp.[0001-2400].exr`.

**Problem with folding**: The range is atomic — to materialize, you need the ID list (`@data` block) and then can selectively fetch. But c4m tools that work on entries see one entry, not 2,400.

**With `@expand`**: The expanded entries can be filtered to frames 100-200 using standard c4m operations (layer, subset, etc.), enabling selective transfer requests.

### Case 5: Mixed-Codec or Mixed-Resolution Sequence

**Scenario**: A plate scan where frames 1-500 are 4K and 501-1000 are 2K (camera swap mid-shoot). Same naming pattern, wildly different file sizes.

**Problem with folding**: `plate.[0001-1000].exr` aggregates sizes to total, hiding the 2x size difference between halves. An anomaly detector looking at the range sees nothing unusual.

**With `@expand`**: Individual sizes reveal the resolution change point, enabling automated detection.

### Case 6: Archive Verification

**Scenario**: Long-term archive of 10,000-frame sequence. Years later, verifying integrity.

**Problem with folding**: The range entry has one sequence C4 ID. To verify individual frames, you need the `@data` ID list. If the ID list is missing or its referenced content is unavailable, verification is impossible at the per-frame level.

**With `@expand`**: Individual entries with C4 IDs enable per-frame verification directly from the c4m file, without needing to resolve external references.

## Implementation Path

### Phase 1: Implement `@expand` Parsing (Current Cycle)

Replace the `ErrNotSupported` return in `decoder.go:650` with actual parsing:

```go
case "@expand":
    if len(parts) < 2 {
        return fmt.Errorf("@expand requires C4 ID")
    }
    id, err := c4.Parse(parts[1])
    if err != nil {
        return fmt.Errorf("invalid @expand C4 ID: %w", err)
    }
    // Read expanded entries until next directive or EOF
    // Store as ExpandBlock on the manifest
```

The `@expand` block contains full c4m entries (not just C4 IDs). Parsing reuses the existing entry parser.

### Phase 2: Encoder Support

The encoder writes `@expand` blocks after all entries and layers, before `@data` blocks:

```go
// Write expand blocks
for _, expand := range m.ExpandBlocks {
    fmt.Fprintf(w, "@expand %s\n", expand.ID)
    for _, entry := range expand.Entries {
        fmt.Fprintf(w, "%s\n", entry.Format(indentWidth, false))
    }
}
```

### Phase 3: Scanner Integration

Add `--expand` flag to `c4 scan`:
- When `--fold` (default) is active, produce range entries
- When `--fold --expand` is active, produce range entries AND `@expand` blocks with full individual entries
- When `--no-fold` is active, produce individual entries (no ranges, no `@expand`)

### Phase 4: Identity Documentation

Document that folded and unfolded forms have different C4 IDs (Option A — decided). Update CLI help text and user-facing docs to explain the `--fold`/`--no-fold` trade-off.

## Open Questions

1. **Should `@expand` presence affect `c4 fmt` behavior?** If `c4 fmt --fold` encounters a file with `@expand` blocks, should it preserve them? (Recommended: yes.)

2. **Should `@expand` entries be validated against the range?** The expanded entries should produce the same sequence C4 ID as the range entry's C4 ID. Should the parser enforce this? (Recommended: validate on decode, warn on mismatch.)

3. **Can `@expand` exist without a corresponding range entry?** If so, it would mean "here are entries that COULD be folded but shouldn't be." This is closer to `@nofold` semantics. (Recommended: no — `@expand` always references an existing range.)

4. ~~**Identity resolution timeline.**~~ **DECIDED.** Option A — folded and unfolded forms have different C4 IDs. Folding is lossy and identity is honest about that.
