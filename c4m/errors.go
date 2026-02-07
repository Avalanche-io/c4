package c4m

import "errors"

// Sentinel errors for the c4m package. Use errors.Is() to check for these.
var (
	// ErrInvalidHeader indicates the manifest header is missing or malformed.
	ErrInvalidHeader = errors.New("c4m: invalid header")

	// ErrUnsupportedVersion indicates the manifest version is not supported.
	ErrUnsupportedVersion = errors.New("c4m: unsupported version")

	// ErrInvalidEntry indicates a manifest entry line is malformed.
	ErrInvalidEntry = errors.New("c4m: invalid entry")

	// ErrDuplicatePath indicates a duplicate path was found in the manifest.
	ErrDuplicatePath = errors.New("c4m: duplicate path")

	// ErrPathTraversal indicates a path traversal attempt (../ or ./).
	ErrPathTraversal = errors.New("c4m: path traversal")

	// ErrNotSupported indicates a feature that is not yet implemented.
	ErrNotSupported = errors.New("c4m: not supported")
)
