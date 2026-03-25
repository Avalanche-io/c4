package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

func runCat(args []string) {
	fs := newFlags("cat")
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Pretty-print c4m content")
	recursive := fs.boolFlag("recursive", 'r', false, "Recursively expand directory entries in c4m")
	fs.parse(args)

	if len(fs.args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 cat [flags] <c4id|file.c4m>\n")
		fmt.Fprintf(os.Stderr, "\nRetrieve content by C4 ID from the configured store,\n")
		fmt.Fprintf(os.Stderr, "or display a c4m file from disk.\n")
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		fmt.Fprintf(os.Stderr, "  -e, --ergonomic    Pretty-print c4m content\n")
		fmt.Fprintf(os.Stderr, "  -r, --recursive    Recursively expand directory entries\n")
		os.Exit(1)
	}

	target := fs.args[0]

	// Check if target is a file path (c4m file on disk).
	if _, err := os.Stat(target); err == nil {
		catFile(target, *ergonomic, *recursive)
		return
	}

	// Otherwise, treat as a C4 ID to fetch from store.
	if !looksLikeC4ID(target) {
		fatalf("Error: %q is not a file path or C4 ID", target)
	}

	id, err := c4.Parse(target)
	if err != nil {
		fatalf("Error: invalid C4 ID: %v", err)
	}

	s, err := store.OpenStore()
	if err != nil {
		fatalf("Error opening store: %v", err)
	}
	if s == nil {
		fatalf("Error: no content store configured.\nSet C4_STORE=/path/to/store or s3://bucket/prefix")
	}

	catFromStore(s, id, *ergonomic, *recursive)
}

// catFile displays a c4m file from disk.
func catFile(path string, ergonomic, recursive bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("Error reading %s: %v", path, err)
	}

	m := tryParseC4m(data)
	if m == nil {
		// Not c4m — output raw bytes.
		os.Stdout.Write(data)
		return
	}

	if recursive {
		s := openStoreOrNil()
		if s != nil {
			m = expandRecursive(m, s)
		}
	}

	outputManifest(m, ergonomic)
}

// catFromStore fetches content from the store and displays it.
func catFromStore(s store.Store, id c4.ID, ergonomic, recursive bool) {
	rc, err := s.Open(id)
	if err != nil {
		fatalf("Error: content not found for %s", id)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		fatalf("Error reading content: %v", err)
	}

	// Try to parse as c4m for formatting flags.
	m := tryParseC4m(data)
	if m == nil {
		// Not c4m — output raw bytes.
		os.Stdout.Write(data)
		return
	}

	if recursive {
		m = expandRecursive(m, s)
	}

	outputManifest(m, ergonomic)
}

// expandRecursive walks a manifest and expands directory entries that have
// C4 IDs by fetching the directory's c4m from the store and inlining the
// children at the appropriate depth.
func expandRecursive(m *c4m.Manifest, s store.Store) *c4m.Manifest {
	result := c4m.NewManifest()
	for _, entry := range m.Entries {
		result.AddEntry(entry)
		if !entry.IsDir() || entry.C4ID.IsNil() {
			continue
		}
		// Try to fetch the directory's c4m from the store.
		sub := fetchManifestFromStore(s, entry.C4ID)
		if sub == nil {
			continue
		}
		// Recursively expand the sub-manifest.
		sub = expandRecursive(sub, s)
		// Inline children at depth = entry.Depth + 1.
		for _, child := range sub.Entries {
			childCopy := *child
			childCopy.Depth += entry.Depth + 1
			result.AddEntry(&childCopy)
		}
	}
	return result
}

// fetchManifestFromStore fetches a C4 ID from the store and tries to parse
// it as a c4m manifest. Returns nil if not found or not c4m.
func fetchManifestFromStore(s store.Store, id c4.ID) *c4m.Manifest {
	rc, err := s.Open(id)
	if err != nil {
		return nil
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil
	}
	return tryParseC4m(data)
}

// openStoreOrNil opens the configured store, returning nil on error or if
// no store is configured. Unlike getOrSetupStore, this never prompts.
func openStoreOrNil() store.Store {
	s, err := store.OpenStore()
	if err != nil || s == nil {
		return nil
	}
	return s
}

func looksLikeC4ID(s string) bool {
	if len(s) != 90 || !strings.HasPrefix(s, "c4") {
		return false
	}
	for _, ch := range s[2:] {
		if !isBase58(byte(ch)) {
			return false
		}
	}
	return true
}

func isBase58(b byte) bool {
	return (b >= '1' && b <= '9') ||
		(b >= 'A' && b <= 'H') ||
		(b >= 'J' && b <= 'N') ||
		(b >= 'P' && b <= 'Z') ||
		(b >= 'a' && b <= 'k') ||
		(b >= 'm' && b <= 'z')
}

