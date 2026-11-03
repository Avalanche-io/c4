package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
	"github.com/Avalanche-io/c4/store"
)

// loadManifest reads and decodes a c4m file. The returned manifest has
// any patch chain fully resolved. An external base reference is an error
// here, not a silent omission — see loadManifestFromStore for the verbs
// that can read one.
func loadManifest(path string) (*c4m.Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Deliberately no content source: id never reads the store, so a
	// description naming an external base fails here, naming the ID,
	// rather than resolving to a manifest silently missing its base.
	return c4m.DecodeResolved(f, nil)
}

// loadManifestFromStore loads a description and backfills an external base
// reference from the configured store — for the verbs that already read it.
// With no store configured it degrades to loadManifest's loud failure.
func loadManifestFromStore(path string) (*c4m.Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, _ := store.OpenStore()
	if s == nil {
		return c4m.DecodeResolved(f, nil)
	}
	return c4m.DecodeResolved(f, s)
}

// filterBySubpath returns a manifest containing only entries under subPath.
func filterBySubpath(manifest *c4m.Manifest, subPath string) *c4m.Manifest {
	if !strings.HasSuffix(subPath, "/") {
		subPath += "/"
	}
	filtered := c4m.NewManifest()
	depth := strings.Count(subPath, "/")
	inSubtree := false

	for _, entry := range manifest.Entries {
		var fullPath string
		if entry.IsDir() {
			fullPath = buildFullPath(manifest, entry)
		} else {
			fullPath = buildFullPath(manifest, entry)
		}

		if strings.HasPrefix(fullPath, subPath) || fullPath == strings.TrimSuffix(subPath, "/") {
			e := *entry
			e.Depth = entry.Depth - depth
			if e.Depth < 0 {
				e.Depth = 0
			}
			filtered.AddEntry(&e)
			inSubtree = true
		} else if inSubtree {
			break
		}
	}
	return filtered
}

// buildFullPath reconstructs the full path of an entry within a manifest.
func buildFullPath(manifest *c4m.Manifest, target *c4m.Entry) string {
	var dirStack []string
	for _, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
		if entry == target {
			if len(dirStack) > 0 {
				return strings.Join(dirStack[:entry.Depth], "") + entry.Name
			}
			return entry.Name
		}
	}
	return target.Name
}

// isDirectory returns true if the path refers to a directory, either by
// trailing slash convention or by stat on disk.
func isDirectory(path string) bool {
	if strings.HasSuffix(path, "/") {
		return true
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// guidedScan scans a directory using a reference manifest to avoid rehashing
// unchanged files. Files with matching size and timestamp get their C4 ID
// from the reference. Only files with changed metadata are hashed.
func guidedScan(dirPath string, ref *c4m.Manifest, mode scan.ScanMode) *c4m.Manifest {
	refPaths := c4m.EntryPaths(ref.Entries)

	// Scan in metadata mode (fast — no hashing).
	gen := scan.NewGeneratorWithOptions(scan.WithMode(scan.ModeMetadata))
	metaManifest, err := gen.GenerateFromPath(dirPath)
	if err != nil {
		fatalf("Error scanning %s: %v", dirPath, err)
	}

	scanPaths := c4m.EntryPaths(metaManifest.Entries)

	// For each entry, use the reference C4 ID if metadata matches,
	// otherwise hash the file.
	changed := make(map[string]bool) // track paths that changed
	for path, entry := range scanPaths {
		if entry.IsDir() {
			continue
		}
		refEntry, ok := refPaths[path]
		// Compare timestamps at second precision — c4m truncates to seconds.
		if ok && !refEntry.C4ID.IsNil() &&
			entry.Size == refEntry.Size &&
			entry.Timestamp.Truncate(time.Second).Equal(refEntry.Timestamp.Truncate(time.Second)) {
			entry.C4ID = refEntry.C4ID
		} else if mode == scan.ModeFull {
			fullPath := filepath.Join(dirPath, filepath.FromSlash(path))
			f, err := os.Open(fullPath)
			if err != nil {
				continue
			}
			entry.C4ID = c4.Identify(f)
			f.Close()
			changed[path] = true
		} else {
			changed[path] = true
		}
	}

	// If nothing changed and the entry sets are identical, the scanned
	// manifest is equivalent to the reference. Return the reference to
	// preserve directory C4 IDs and canonical structure.
	if len(changed) == 0 {
		sameEntries := true
		if len(scanPaths) != len(refPaths) {
			sameEntries = false
		} else {
			for p := range scanPaths {
				if _, ok := refPaths[p]; !ok {
					sameEntries = false
					break
				}
			}
		}
		if sameEntries {
			return ref
		}
	}

	return metaManifest
}

// tryParseC4m attempts to parse data as a c4m file.
// Returns the parsed manifest if successful, nil otherwise.
func tryParseC4m(data []byte) *c4m.Manifest {
	m, err := c4m.Unmarshal(data)
	if err != nil || len(m.Entries) == 0 {
		return nil
	}
	return m
}

// identifyFileBytes returns the C4 ID of a file's bytes. A .c4m file is
// bytes like any other: its ID is the ID that summons it.
func identifyFileBytes(path string) c4.ID {
	f, err := os.Open(path)
	if err != nil {
		fatalf("Error reading %s: %v", path, err)
	}
	defer f.Close()
	return c4.Identify(f)
}

// storeFileBytes stores a file's bytes and returns their C4 ID.
func storeFileBytes(s store.Store, path string) c4.ID {
	f, err := os.Open(path)
	if err != nil {
		fatalf("Error reading %s: %v", path, err)
	}
	defer f.Close()
	id, err := s.Put(f)
	if err != nil {
		fatalf("Error storing %s: %v", path, err)
	}
	return id
}

// storeContentBytes stores a reader's bytes and returns their C4 ID and
// size. Nothing is parsed: identify means the bytes.
func storeContentBytes(s store.Store, r io.Reader) (c4.ID, int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return c4.ID{}, 0, err
	}
	id, err := s.Put(bytes.NewReader(data))
	return id, int64(len(data)), err
}

// journalClaim appends a claim for a stored description to the store's
// journal and returns only when the append is durable. This is the
// print barrier's final leg: callers may report the ID only after this
// returns; if the journal append fails, the run dies WITHOUT reporting
// the ID, so anything printed is always journaled and recoverable.
// Stores without a local root (e.g. pure remote stores) are not
// journaled — the journal is a local-store contract.
func journalClaim(s store.Store, id c4.ID, scanStart time.Time) {
	if id.IsNil() {
		return
	}
	type rooted interface{ Root() string }
	r, ok := s.(rooted)
	if !ok {
		return
	}
	j := c4m.OpenJournal(r.Root())
	if err := j.Append(c4m.Claim{ScanStart: scanStart, ID: id}); err != nil {
		fatalf("Error: journal append failed; ID not reported: %v", err)
	}
}

// fatalf prints to stderr and exits.
func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// ingestSync is the write-durability mode for store ingest. The default
// is batch: objects land atomically (complete or absent) during the run
// and one device flush at the end of the command makes the whole batch
// durable. See design/store-ingest-performance.md.
var ingestSync = store.SyncBatch

// applyIngestSync configures a store for the ingest durability mode.
func applyIngestSync(s store.Store) store.Store {
	if sm, ok := s.(interface{ SetSyncMode(store.SyncMode) }); ok {
		sm.SetSyncMode(ingestSync)
	}
	return s
}

// syncStore issues the ingest batch barrier: everything stored so far
// becomes durable. No-op for stores without batch semantics.
func syncStore(s store.Store) {
	if s == nil {
		return
	}
	sy, ok := s.(store.Syncer)
	if !ok {
		return
	}
	if err := sy.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: store sync: %v\n", err)
	}
}
