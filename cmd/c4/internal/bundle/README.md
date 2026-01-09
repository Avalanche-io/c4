# Bundle Package (Work In Progress)

## Status: Moved from c4m, needs integration work

These files were moved from `c4m/` on 2025-01-08. The bundle format is an implementation detail of the c4 command, not part of the c4m specification.

## TODO to complete migration

1. **Add c4m import** - All files need `"github.com/Avalanche-io/c4/c4m"` import

2. **Prefix c4m types** - Update references throughout:
   - `Entry` → `c4m.Entry`
   - `*Entry` → `*c4m.Entry`
   - `Manifest` → `c4m.Manifest`
   - `*Manifest` → `*c4m.Manifest`
   - `NewManifest()` → `c4m.NewManifest()`
   - `NewParser()` → `c4m.NewParser()`
   - `NaturalLess()` → `c4m.NaturalLess()`

3. **Import scan package** - Bundle code uses scanning types:
   - May need to import `internal/scan` or merge packages

4. **Update main.go** - Change imports to use this internal package

5. **Move tests** - Related test files need to move here too:
   - `bundle_test.go`
   - `bundle_cli_test.go`
   - `bundle_integration_test.go`
   - `base_chain_test.go`

## Files

- `bundle.go` - Core bundle structure (Bundle, BundleScan, BundleConfig)
- `bundle_scanner.go` - Scanning into bundles with chunking
- `bundle_extract.go` - Extract/materialize bundles to single manifest
- `bundle_cli_simple.go` - CLI helpers for bundle operations
- `base_chain.go` - @base chain resolution for composing manifests

## Design Notes

The bundle format will likely be refactored. Current structure:
```
name.c4m_bundle/
  header.c4       # C4 ID of header manifest
  c4/             # Content-addressed storage
    <c4id>        # Manifest chunks and metadata
  scans/
    1/
      path.txt
      progress/
        1.c4m, 2.c4m, ...
      snapshot.c4m
```

Consider whether this complexity is needed vs simpler @base chaining of plain .c4m files.
