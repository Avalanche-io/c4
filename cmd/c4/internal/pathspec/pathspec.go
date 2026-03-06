// Package pathspec implements the colon syntax path parser for the c4 CLI.
//
// The colon is the portal between local paths, capsule contents, and remote
// locations. Everything after the colon is just a path.
//
//	renders/                   → local directory
//	project.c4m:renders/       → described directory (capsule)
//	studio:project/renders/    → remote directory (location)
package pathspec

import (
	"fmt"
	"strings"
)

// Type distinguishes the three kinds of paths.
type Type int

const (
	Local     Type = iota // Regular filesystem path
	Capsule               // Path into a .c4m file
	Location              // Path into a named location
	Container             // Path into a tar/tgz archive
	Managed               // Managed directory (bare colon)
)

func (t Type) String() string {
	switch t {
	case Local:
		return "local"
	case Capsule:
		return "capsule"
	case Location:
		return "location"
	case Container:
		return "container"
	case Managed:
		return "managed"
	default:
		return "unknown"
	}
}

// PathSpec is a parsed colon-syntax path.
type PathSpec struct {
	Type    Type
	Source  string // capsule file path, location name, or local path
	SubPath string // path within capsule/location (empty = root)
}

// IsRoot returns true if this refers to the root of a capsule or location
// (trailing colon with no subpath).
func (p PathSpec) IsRoot() bool {
	return p.SubPath == ""
}

// String returns the original colon-syntax representation.
func (p PathSpec) String() string {
	switch p.Type {
	case Local:
		return p.Source
	case Capsule, Location:
		if p.SubPath == "" {
			return p.Source + ":"
		}
		return p.Source + ":" + p.SubPath
	case Managed:
		return ":" + p.SubPath
	default:
		return p.Source
	}
}

// Parse parses a colon-syntax path string into a PathSpec.
//
// Resolution rules (applied in order):
//  1. No colon → local path
//  2. Starts with ./ or / → local path (colon is inside a path component)
//  3. Left side contains / → local path (colon is inside a path component)
//  4. Left side ends with .c4m → capsule path
//  5. Left side matches a known location → location path
//  6. Otherwise → error
//
// The isLocation function is optional. If nil, only capsule paths are
// recognized through the colon syntax. Pass a lookup function to enable
// location resolution.
func Parse(s string, isLocation func(string) bool) (PathSpec, error) {
	// Rule 1: no colon → local
	colonIdx := strings.IndexByte(s, ':')
	if colonIdx < 0 {
		return PathSpec{Type: Local, Source: s}, nil
	}

	// Rule 2: starts with ./ or / → local (colon is in a path component)
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "/") {
		return PathSpec{Type: Local, Source: s}, nil
	}

	left := s[:colonIdx]
	right := s[colonIdx+1:]

	// Rule 2b: bare colon or :~ prefix → managed directory
	if left == "" {
		return PathSpec{Type: Managed, SubPath: right}, nil
	}

	// Rule 3: left side contains / → local
	if strings.Contains(left, "/") {
		return PathSpec{Type: Local, Source: s}, nil
	}

	// Strip leading "/" from subpath — everything after the colon
	// is relative to the root of the capsule/location.
	// test.c4m:/files/ → SubPath "files/", not "/files/"
	right = strings.TrimPrefix(right, "/")

	// Rule 4: left side ends with .c4m → capsule
	if strings.HasSuffix(left, ".c4m") {
		return PathSpec{Type: Capsule, Source: left, SubPath: right}, nil
	}

	// Rule 4b: left side ends with a container extension → container
	if isContainerExt(left) {
		return PathSpec{Type: Container, Source: left, SubPath: right}, nil
	}

	// Rule 5: check location registry
	if isLocation != nil && isLocation(left) {
		return PathSpec{Type: Location, Source: left, SubPath: right}, nil
	}

	// Rule 6: unrecognized
	return PathSpec{}, fmt.Errorf("%q is not a capsule (.c4m) or known location", left)
}

// containerExts lists recognized archive extensions.
var containerExts = []string{".tar.zst", ".tar.gz", ".tar.bz2", ".tar.xz", ".tgz", ".tar"}

// isContainerExt reports whether name ends with a recognized archive extension.
func isContainerExt(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range containerExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// ContainerFormat returns the compression format for a container source path.
// Returns "tar", "gzip", "bzip2", "xz", or "zstd".
func ContainerFormat(source string) string {
	lower := strings.ToLower(source)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "gzip"
	case strings.HasSuffix(lower, ".tar.bz2"):
		return "bzip2"
	case strings.HasSuffix(lower, ".tar.xz"):
		return "xz"
	case strings.HasSuffix(lower, ".tar.zst"):
		return "zstd"
	default:
		return "tar"
	}
}

// MustParse is like Parse but panics on error. For tests only.
func MustParse(s string, isLocation func(string) bool) PathSpec {
	p, err := Parse(s, isLocation)
	if err != nil {
		panic(err)
	}
	return p
}
