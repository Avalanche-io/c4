# Scan Package (Work In Progress)

## Status: Moved from c4m, needs integration work

These files were moved from `c4m/` on 2025-01-08 to separate filesystem scanning from the manifest format specification.

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

3. **Update main.go** - Change imports from `c4m.Generator` etc. to use this internal package

4. **Move tests** - Related test files need to move here too:
   - `scanner_interface_test.go`
   - `progressive_scanner_test.go`
   - `example_scanner_test.go`

## Files

- `generator.go` - Generate manifests from filesystem paths
- `progressive_scanner.go` - Multi-stage concurrent scanner
- `progressive_cli.go` - CLI wrapper for progressive scanning
- `scanner_interface.go` - Scanner abstractions
- `scanner_darwin.go` - macOS-specific fast directory scanning
- `scanner_linux.go` - Linux-specific fast directory scanning
- `scanner_generic.go` - Fallback for other platforms
- `signal_darwin.go` - SIGINFO handling (macOS)
- `signal_other.go` - Signal handling (Linux/Unix)
- `signal_windows.go` - Signal handling (Windows)
- `metadata.go` - FileMetadata interface bridging OS info to Entry
- `timing.go` - Elapsed time tracking utilities

## Design Notes

This scanning implementation may be significantly refactored. Consider:
- Simpler single-pass scanning vs progressive stages
- Whether sequence detection belongs here or is VFX-specific
- Integration with bundle system
