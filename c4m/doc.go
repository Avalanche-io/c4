// Package c4m implements the C4 Manifest Format (C4M) Specification v1.0
//
// C4M is a text-based (UTF-8) format for describing filesystem contents with
// content-addressed identification using C4 IDs (SMPTE ST 2114:2017).
// It preserves filesystem metadata while enabling content verification,
// deduplication, and distributed workflows.
//
// Key features:
//   - Text-based format that fits in an email
//   - Cryptographic content identification via C4 IDs
//   - Compact representation of media file sequences
//   - Natural sorting for numbered files
//   - Streaming parser support
//
// Format example:
//
//	drwxr-xr-x 2023-01-01T12:00:00Z 4096 project/
//	  -rw-r--r-- 2023-01-01T12:00:00Z 1024 README.md c4ABC123...
//	  -rw-r--r-- 2023-01-01T12:00:00Z 2048 config.json c4DEF456...
package c4m