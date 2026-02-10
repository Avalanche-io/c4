# C4M 1.0 Release Readiness Checklist

Mechanically verified by `TestReadiness` in `readiness_test.go`.

## Parser Correctness
- [x] Quoted filenames round-trip (backslashes, quotes, newlines)
- [x] Leading-space filenames round-trip
- [x] Symlink targets with spaces round-trip
- [x] Natural sort: text segments sort before numeric segments
- [x] Natural sort: ASCII digits only (no Arabic-Indic/Devanagari)

## Correctness Semantics
- [x] Canonicalize is deterministic (no time.Now())
- [x] Validate accepts null timestamps
- [x] Validate accepts null sizes
- [x] No stderr output from validator
- [x] @expand returns ErrNotSupported
- [x] NullTimestamp sentinel is exported as function

## API Surface
- [x] currentLayer is unexported
- [x] propagateMetadata is unexported
- [x] GenerateFromReader removed
- [x] Single public sort method: SortEntries()
- [x] Single public lookup method: GetEntry() (O(1) indexed)
- [x] Copy() is deep (Layers, DataBlocks, Entry slices)
- [x] Sentinel errors exported (ErrInvalidHeader, ErrInvalidEntry, etc.)

## Documentation
- [x] Every .go file listed in README exists
- [x] No stale API names in WORKFLOWS.md
- [x] No stale API names in README.md
- [x] IMPLEMENTATION_NOTES.md symlink section matches spec

## Hardening
- [x] Fuzz tests exist (FuzzDecoder, FuzzRoundTrip, FuzzValidator, FuzzNaturalSort)
- [x] Adversarial input tests exist
- [x] Test coverage >= 94.8%

## Transform
- [x] transform/doc.go has EXPERIMENTAL warning
