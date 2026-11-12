package main

import (
	"bytes"
	"crypto/sha512"
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
		fmt.Fprintf(os.Stderr, "  -r, --recursive    Recursively expand directory entries (a listing the store\n")
		fmt.Fprintf(os.Stderr, "                     lacks is named on stderr as partial: ...; exit 2)\n")
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

	// Without a rendering flag, cat is the bytes (design/no-self-id.md on
	// master). Through 1.0.26 a file whose bytes parsed as c4m was written
	// re-rendered, so cat of a pretty (-e) file did not return the file.
	if !ergonomic && !recursive {
		os.Stdout.Write(data)
		return
	}
	m := tryParseC4m(data)
	if m == nil {
		// Not c4m — the formatting flags have nothing to act on.
		os.Stdout.Write(data)
		return
	}

	var missing []missingRecord
	if recursive {
		// A nil store expands nothing and names every collapsed directory
		// as missing: -r without a store is a partial answer, not a quiet one.
		m, missing = expandRecursive(m, openStoreOrNil())
	}

	outputManifest(m, ergonomic)
	if reportPartialTree(missing) {
		os.Exit(2)
	}
}

// catFromStore fetches content from the store, verifies it against the
// requested ID, and displays it. Nothing is written on a failed
// verification: exit 0 means the object is present and intact.
func catFromStore(s store.Store, id c4.ID, ergonomic, recursive bool) {
	// cat returns the stored bytes. Recognizing that an object parses as
	// c4m is what enables -e and -r; it never licenses rewriting what
	// was stored. Without a formatting flag the bytes go out verbatim,
	// so an object retrieved by ID is the object that was put.
	if !ergonomic && !recursive {
		streamVerified(s, id)
		return
	}

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
		fatalMismatch(id)
	}

	m := tryParseC4m(data)
	if m == nil {
		// Not c4m — the formatting flags have nothing to act on.
		os.Stdout.Write(data)
		return
	}

	var missing []missingRecord
	if recursive {
		m, missing = expandRecursive(m, s)
	}

	// The tree goes out as far as the store can supply it, then every
	// listing it could not supply is named on stderr and cat exits 2:
	// partial is a valid answer, silence about it is not.
	outputManifest(m, ergonomic)
	if reportPartialTree(missing) {
		os.Exit(2)
	}
}

// streamVerified hashes a stored object on a first pass and streams it
// to stdout on a second, so objects of any size are verified before a
// single byte is written, without holding them in memory.
func streamVerified(s store.Store, id c4.ID) {
	rc, err := s.Open(id)
	if err != nil {
		fatalf("Error: content not found for %s", id)
	}
	defer func() { rc.Close() }()

	h := sha512.New()
	if _, err := io.Copy(h, rc); err != nil {
		fatalf("Error reading content: %v", err)
	}
	var got c4.ID
	copy(got[:], h.Sum(nil))
	if got != id {
		fatalMismatch(id)
	}

	// Rewind the same handle when possible (a local file), so the bytes
	// streamed are the bytes hashed; otherwise open the object again.
	if sk, ok := rc.(io.Seeker); ok {
		_, err = sk.Seek(0, io.SeekStart)
	} else {
		rc.Close()
		rc, err = s.Open(id)
	}
	if err != nil {
		fatalf("Error reading content: %v", err)
	}
	if _, err := io.Copy(os.Stdout, rc); err != nil {
		fatalf("Error reading content: %v", err)
	}
}

func fatalMismatch(id c4.ID) {
	fatalf("Error: stored content for %s does not match its ID (object is damaged or altered)", id)
}

// missingRecord names a directory whose listing could not be inlined: the
// path as recorded, the listing's ID, and why the store could not supply
// it. A partial tree is a valid answer (partial knowledge is first-class),
// but it is never a silent one.
type missingRecord struct {
	path string
	id   c4.ID
	why  string
}

// emptyListingID is the ID of an empty directory's listing (zero bytes):
// such a directory has no children to fetch and is complete as a line.
var emptyListingID = c4.Identify(bytes.NewReader(nil))

// expandRecursive walks a manifest and expands every collapsed directory
// entry (one that carries a listing ID and lists no children) by fetching
// the listing from the store, verifying it, and inlining the children at
// the right depth, recursively. It returns the expanded manifest and
// every directory it could not expand. Until 2026-11-08 a missing listing
// was skipped silently and the caller exited 0 on a partial tree; that
// hid a real bug in a c4d prototype whose check read only the exit status.
func expandRecursive(m *c4m.Manifest, s store.Store) (*c4m.Manifest, []missingRecord) {
	result := c4m.NewManifest()
	var missing []missingRecord
	var dirs []string // recorded directory names by depth, for paths
	for i, entry := range m.Entries {
		result.AddEntry(entry)
		if entry.Depth < len(dirs) {
			dirs = dirs[:entry.Depth]
		}
		if !entry.IsDir() {
			continue
		}
		path := strings.Join(dirs, "") + entry.Name
		dirs = append(dirs, entry.Name)
		collapsed := i+1 >= len(m.Entries) || m.Entries[i+1].Depth <= entry.Depth
		if !collapsed || entry.C4ID.IsNil() || entry.C4ID == emptyListingID {
			continue
		}
		if s == nil {
			missing = append(missing, missingRecord{path, entry.C4ID, "no store configured"})
			continue
		}
		sub, why := fetchListing(s, entry.C4ID)
		if sub == nil {
			missing = append(missing, missingRecord{path, entry.C4ID, why})
			continue
		}
		sub, more := expandRecursive(sub, s)
		for _, mr := range more {
			mr.path = path + mr.path
			missing = append(missing, mr)
		}
		// Inline children at depth = entry.Depth + 1.
		for _, child := range sub.Entries {
			childCopy := *child
			childCopy.Depth += entry.Depth + 1
			result.AddEntry(&childCopy)
		}
	}
	return result, missing
}

// expandIfRecord turns a one-level directory record (all entries at
// depth 0) into the full tree by inlining stored per-directory records,
// reporting every listing the store could not supply. Full manifests —
// anything with nested entries — pass through unchanged.
func expandIfRecord(m *c4m.Manifest, s store.Store) (*c4m.Manifest, []missingRecord) {
	hasDir := false
	for _, e := range m.Entries {
		if e.Depth > 0 {
			return m, nil // nested entries: already a full manifest
		}
		if e.IsDir() && !e.C4ID.IsNil() {
			hasDir = true
		}
	}
	if !hasDir {
		return m, nil
	}
	return expandRecursive(m, s)
}

// reportPartialTree narrates every missing listing on stderr, one line
// each, and returns true when there were any. Callers that have printed
// a partial tree exit 2, as a partial scan does (c4 id's rule since
// 1.0.23); callers that need the whole tree exit 1.
func reportPartialTree(missing []missingRecord) bool {
	for _, mr := range missing {
		fmt.Fprintf(os.Stderr, "partial: %s listing %s %s\n", mr.path, mr.id, mr.why)
	}
	return len(missing) > 0
}

// requireCompleteTree exits 1 when a tree the verb must have in full has
// missing listings, naming each of them.
func requireCompleteTree(what string, missing []missingRecord) {
	if !reportPartialTree(missing) {
		return
	}
	fatalf("Error: %s is incomplete in the store: %d listing(s) missing (see partial: lines above)", what, len(missing))
}

// fetchListing fetches a C4 ID from the store through the verifying
// reader and parses the bytes as a listing. A listing that is absent or
// does not parse returns nil and the reason, for the partial: narration;
// one whose bytes do not hash to its ID is a verification failure, and
// like every other failed verification in cat it is fatal (exit 1) before
// anything is written: a damaged record is never narrated as "partial".
func fetchListing(s store.Store, id c4.ID) (*c4m.Manifest, string) {
	data, err := readVerified(s, id)
	if _, ok := err.(storeMismatchError); ok {
		fatalf("Error: %v", err)
	}
	if err != nil {
		return nil, "not in store"
	}
	m := tryParseC4m(data)
	if m == nil {
		return nil, "does not parse as a listing"
	}
	return m, ""
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

