package main

import (
	"bytes"
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
	fs.help(catHelp)
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

	// A store address — an argument whose first slash-separated
	// component is syntactically a C4 ID — is tested before the
	// filesystem (prefix ./ to force a filesystem path).
	if first := strings.SplitN(target, "/", 2)[0]; looksLikeC4ID(first) {
		id, err := c4.Parse(first)
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

		finalID, assertListing := storeDescend(s, id, target[len(first):])
		catFromStore(s, finalID, *ergonomic, *recursive, assertListing)
		return
	}

	// A file path (c4m file on disk).
	if _, err := os.Stat(target); err == nil {
		catFile(target, *ergonomic, *recursive)
		return
	}

	fatalf("Error: %q is not a file path or C4 ID", target)
}

// storeDescend resolves the /path part of a store address by recorded
// entry name — byte-exact, never following symlinks — and returns the
// final object's ID plus whether a trailing slash asserted a listing.
// Every level is fetched and rehash-verified.
func storeDescend(s store.Store, id c4.ID, pathPart string) (c4.ID, bool) {
	assertListing := strings.HasSuffix(pathPart, "/")
	pathPart = strings.Trim(pathPart, "/")
	if pathPart == "" {
		return id, assertListing
	}

	components := strings.Split(pathPart, "/")
	for i, comp := range components {
		if comp == "" || comp == "." || comp == ".." {
			fatalf("Error: invalid path component %q", comp)
		}
		final := i == len(components)-1

		listing := verifiedManifestFromStore(s, id)
		if listing == nil {
			fatalf("Error: %s does not resolve to a listing", id)
		}

		// Match by recorded name among the listing's one level. Both
		// x and x/ present makes bare x ambiguous.
		var dirMatch, fileMatch *c4m.Entry
		for _, e := range listing.Entries {
			if e.Depth != 0 {
				continue
			}
			if e.IsDir() {
				if strings.TrimSuffix(e.Name, "/") == comp {
					dirMatch = e
				}
			} else if e.Name == comp {
				fileMatch = e
			}
		}

		var entry *c4m.Entry
		switch {
		case dirMatch != nil && fileMatch != nil && !(final && assertListing):
			fatalf("Error: %q is ambiguous: both %s and %s/ are recorded (trailing slash asserts the listing)", comp, comp, comp)
		case dirMatch != nil && fileMatch != nil:
			entry = dirMatch
		case dirMatch != nil:
			entry = dirMatch
		case fileMatch != nil:
			entry = fileMatch
		default:
			fatalf("Error: no entry named %q", comp)
		}

		if entry.Target != "" || (!entry.IsDir() && entry.Mode&os.ModeSymlink != 0) {
			fatalf("Error: %q is a symlink (recorded target: %s); recorded paths never follow symlinks", comp, entry.Target)
		}
		if !final && !entry.IsDir() {
			fatalf("Error: %q is not a directory", comp)
		}
		if final && assertListing && !entry.IsDir() {
			fatalf("Error: %q is not a listing (trailing slash asserts one)", comp)
		}
		if entry.C4ID.IsNil() {
			fatalf("Error: %q has no recorded ID (unreadable at scan time)", comp)
		}
		id = entry.C4ID
	}
	return id, assertListing
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

// catFromStore fetches content from the store, verifies it against the
// requested ID, and displays it. Nothing is written on a failed
// verification: absent and damaged are one answer to a consumer.
func catFromStore(s store.Store, id c4.ID, ergonomic, recursive, assertListing bool) {
	data := readVerified(s, id)

	// Try to parse as c4m for formatting flags.
	m := tryParseC4m(data)
	if m == nil {
		if assertListing {
			fatalf("Error: %s does not parse as a listing", id)
		}
		// Not c4m — output raw bytes.
		os.Stdout.Write(data)
		return
	}

	if recursive {
		m = expandRecursive(m, s)
	}

	outputManifest(m, ergonomic)
}

// readVerified reads a store object and rehashes it: the bytes must
// hash to the requested ID or nothing is returned. This is what makes
// a cat probe a checked yes rather than an asserted one.
func readVerified(s store.Store, id c4.ID) []byte {
	rc, err := s.Open(id)
	if err != nil {
		fatalf("Error: content not found for %s", id)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		fatalf("Error reading content: %v", err)
	}
	if c4.Identify(bytes.NewReader(data)) != id {
		fatalf("Error: object failed verification: bytes at %s do not hash to it", id)
	}
	return data
}

// verifiedManifestFromStore fetches and rehash-verifies an object,
// then parses it as a listing. Returns nil when it does not parse.
func verifiedManifestFromStore(s store.Store, id c4.ID) *c4m.Manifest {
	return tryParseC4m(readVerified(s, id))
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

// expandIfRecord turns a one-level directory record (all entries at
// depth 0) into the full tree by inlining stored per-directory records.
// Full manifests — anything with nested entries — pass through
// unchanged.
func expandIfRecord(m *c4m.Manifest, s store.Store) *c4m.Manifest {
	hasDir := false
	for _, e := range m.Entries {
		if e.Depth > 0 {
			return m // nested entries: already a full manifest
		}
		if e.IsDir() && !e.C4ID.IsNil() {
			hasDir = true
		}
	}
	if !hasDir {
		return m
	}
	return expandRecursive(m, s)
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
