// Package scan provides directory scanning for c4m manifests.
//
// Basic usage:
//
//	m, err := scan.Dir("/path/to/dir")
//
// With options:
//
//	m, err := scan.Dir("/path/to/dir",
//	    scan.WithMode(scan.ModeFull),
//	    scan.WithSequenceDetection(true),
//	    scan.WithExclude([]string{"*.tmp", ".git"}),
//	    scan.WithGuide(existingManifest),
//	)
//
// The scanner supports three modes:
//   - ModeStructure: names and hierarchy only (fast)
//   - ModeMetadata: adds permissions, timestamps, sizes
//   - ModeFull: adds C4 IDs (hashes every file)
//
// The guided scan optimization (WithGuide) reuses C4 IDs from a reference
// manifest for files with matching size and timestamp, avoiding expensive
// rehashing of unchanged files.
package scan
