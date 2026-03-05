# C4M 1.0 Release Implementation Plan

## Current State

The c4m package has undergone significant hardening (session 3d91cf12). All readiness
checklist items in READINESS.md pass mechanically. Coverage is 95.9%. The parser was
rewritten with character-level scanning. Core correctness bugs (time.Now(), quoting
round-trip, symlink targets, null value handling) are fixed.

This plan addresses what remains for a confident 1.0 release.

## Phase 1: Specification Alignment

The spec coverage audit identified gaps between what the spec promises and what the
implementation delivers. These must be reconciled — either implement what the spec
says, or update the spec to reflect deliberate scope limits.

### 1.1 Decide on @expand

`@expand` is in the spec, parsed by the decoder, but returns `ErrNotSupported`. Options:
- **Remove from spec** (defer to post-1.0) — simplest
- **Implement** — significant work, no current consumer
Decision required.

### 1.2 Decide on symlink ranges

The spec describes `render.[0001-0100].exr -> /cache/source.[0001-0100].exr` for range
symlinks. Not implemented. Same decision: remove from spec or implement.

### 1.3 Sequence escape notation

Spec section "Escaping in Sequence Notation" describes `\[`, `\]`, `\,`, `\-` escaping.
Not implemented. If sequences stay in 1.0, this escaping must work or be spec'd out.

### 1.4 CRLF handling

Spec says "LF only, no CR." Decoder tolerates CRLF. Options:
- **Spec stays strict, decoder stays tolerant** (be liberal in what you accept)
- **Validator warns on CRLF** (reasonable middle ground)

### 1.5 Column alignment minimum

Spec says "at least 2 spaces between content and C4 ID." Implementation uses 10.
Update spec to match implementation, or reduce to spec value.

### 1.6 @by / @note quoting

Spec examples show quoted values (`@by "Joshua Kolden"`). Implementation writes bare
text. Decide whether to quote in encoder output.

### 1.7 Directory size semantics

Spec says "total size of all contents (recursive)." Builder defaults to 0, not null.
`propagateMetadata` only computes for entries with null values. Clarify in spec what
directory size means when not explicitly set, or auto-compute.

## Phase 2: API Review

The public API surface is what users will code against. It must be minimal, correct,
and unsurprising.

### 2.1 Audit exported symbols

Run `go doc` on every exported type, function, and method. Verify:
- Each is intentionally public
- Naming is consistent (no GetFoo / FetchBar inconsistency)
- No internal implementation details leak

### 2.2 Review error types

Verify sentinel errors cover all user-distinguishable failure modes. Ensure errors
wrap properly for `errors.Is()` / `errors.As()` usage.

### 2.3 Review CANONICAL_FORM_ENFORCEMENT.md

This design doc describes a `MetadataResolver` interface and `ComputeC4ID() (c4.ID,
error)` signature change. The current `Canonicalize()` was simplified (no resolver).
Decide if the resolver pattern is needed for 1.0 or if the current simpler approach
is sufficient. Update or archive the design doc accordingly.

### 2.4 Builder API ergonomics

Verify the builder produces valid manifests for all common use cases. Test that a
new user can construct a manifest, add entries, and encode it without reading source.

## Phase 3: Edge Case Hardening

### 3.1 Broken symlinks

The spec allows symlinks with nil C4 IDs (broken symlinks, symlink-to-symlink).
Add explicit test cases for these.

### 3.2 Directory sequences

Test directory sequence notation (`shot_[001-100]/`).

### 3.3 Empty directory handling

Verify behavior for directories with no children: encoding, decoding, canonical
form, C4 ID computation.

### 3.4 Maximum values

Test maximum file sizes (int64 max), extremely long filenames, deeply nested
directories (100+ levels), manifests with 100K+ entries.

## Phase 4: Documentation

### 4.1 Reconcile spec with implementation

After Phase 1 decisions, update SPECIFICATION.md to accurately reflect what 1.0
actually does. Remove anything deferred. Mark experimental features clearly.

### 4.2 README accuracy

Verify every code example in README.md compiles and produces the shown output.
Remove or update any stale examples.

### 4.3 Godoc review

Ensure every exported symbol has a clear, accurate doc comment. Run `go doc -all`
and review.

### 4.4 Archive superseded design docs

Move design docs that are fully implemented or intentionally not implemented to
`design/archived/`. Keep only active design discussions in `design/`.

## Phase 5: Final Verification

### 5.1 Full test suite

`go test ./c4m/... -count=1 -race` must pass cleanly.

### 5.2 Fuzz for 5 minutes

Run each fuzz target for at least 5 minutes: `FuzzDecoder`, `FuzzRoundTrip`,
`FuzzValidator`, `FuzzNaturalSort`.

### 5.3 go vet / staticcheck

Zero warnings from `go vet ./c4m/...` and `staticcheck ./c4m/...` (if available).

### 5.4 Coverage gate

Coverage must remain >= 95%.

### 5.5 Independent review

A fresh subagent reads the spec cold, then tests every claim against the
implementation, reporting any discrepancy. No prior session context.

### 5.6 Update READINESS.md

Add any new checklist items from this plan. All must pass mechanically.

## Out of Scope for 1.0

Items explicitly deferred to post-1.0:
- MetadataResolver interface / canonicalization from filesystem
- @expand implementation (if removed from spec)
- Symlink ranges (if removed from spec)
- Interactive / streaming parser mode
- transform/ sub-package (marked EXPERIMENTAL)
- Performance optimization (current perf is adequate)

## Sequencing

Phase 1 must come first — spec decisions drive everything else.
Phases 2-4 can partially overlap.
Phase 5 is strictly last.
