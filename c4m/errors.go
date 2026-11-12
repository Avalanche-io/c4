package c4m

import "errors"

// Sentinel errors for the c4m package. Use errors.Is() to check for these.
var (
	// ErrInvalidEntry indicates a manifest entry line is malformed.
	ErrInvalidEntry = errors.New("c4m: invalid entry")

	// ErrDuplicatePath indicates a duplicate path was found in the manifest.
	ErrDuplicatePath = errors.New("c4m: duplicate path")

	// ErrPathTraversal indicates a path traversal attempt (../ or ./).
	ErrPathTraversal = errors.New("c4m: path traversal")

	// ErrInvalidFlowTarget indicates a malformed flow link target.
	ErrInvalidFlowTarget = errors.New("c4m: invalid flow target")

	// ErrPatchIDMismatch indicates a bare C4 ID line does not match the
	// canonical C4 ID of the accumulated manifest content above it.
	ErrPatchIDMismatch = errors.New("c4m: patch ID does not match prior content")

	// ErrEmptyPatch indicates a patch section contains no entries.
	ErrEmptyPatch = errors.New("c4m: empty patch section")

	// ErrTrailingID indicates the stream ends with a bare C4 ID. A c4m
	// file never ends with its own ID: a bare ID is a boundary before a
	// patch, and a file that ends with one is a format error (ruled
	// 2026-11-08). Files written by c4 1.0.13 through 1.0.26 with
	// `c4 diff >>` have this shape; `c4 id FILE.c4m` rewrites them.
	ErrTrailingID = errors.New("c4m: a bare C4 ID ends the file; a c4m file never ends with its own ID")

	// ErrUnresolvedBase indicates a first-line external base reference
	// could not be fetched: no source was supplied, or the source does
	// not hold that ID. A description whose base is unreadable is an
	// error, never a manifest presented as complete.
	ErrUnresolvedBase = errors.New("c4m: unresolved external base")

	// ErrBaseCycle indicates a chain of external base references revisits
	// an ID it already resolved.
	ErrBaseCycle = errors.New("c4m: external base reference cycle")
)
