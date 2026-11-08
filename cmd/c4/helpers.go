package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
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

// loadManifestMigrating is loadManifest for the normaliser only: a file
// that ends with a bare C4 ID (the shape `c4 diff >>` wrote through
// 1.0.24) is accepted, its trailing ID verified as a checkpoint and
// dropped, and dropped reports that it happened so the caller can say
// so. Every other verb reads strictly and refuses that shape.
func loadManifestMigrating(path string) (m *c4m.Manifest, dropped bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	d := c4m.NewDecoder(f)
	d.AllowTrailingID()
	m, err = d.Decode()
	if err != nil {
		return nil, false, err
	}
	m, err = c4m.ResolveBase(m, nil)
	return m, d.DroppedTrailingID(), err
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

// scanForCompare scans a directory for diff, restore and explain. Every
// file is hashed, unless the caller names a guide with -c: then the scan
// applies exactly c4 id -c's re-scan trust (RE-SCAN TRUST in c4 id
// --help), stated out loud and racy-safe, with one implementation.
//
// Until 2026-11-03 these verbs trusted size and mtime silently, against
// the other side's description, with no racy rule: a file whose bytes
// changed but whose size and mtime did not went undetected by c4 diff,
// and c4 restore planned nothing for it, then failed its own
// verification and left the changed bytes in place. Whether to trust
// metadata is the client's choice (Joshua: trusted HPC versus exchanges
// between untrusted parties); it is never the silent default.
func scanForCompare(dirPath string, mode scan.ScanMode, guidePath string) *c4m.Manifest {
	var guide *c4m.Manifest
	var guideStart time.Time
	if guidePath != "" {
		var err error
		guide, err = loadManifest(guidePath)
		if err != nil {
			fatalf("Error loading guide %s: %v", guidePath, err)
		}
		guideStart = resolveGuideScanStart(guidePath, guide)
	}
	return scanDirectory(dirPath, mode, false, false, nil, "", guide, guideStart, false)
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
// bytes like any other: its ID is the ID that summons it. A file that
// cannot be opened or read has no ID: the error is returned and the ID
// is nil, never the ID of the bytes read before the failure.
func identifyFileBytes(path string) (c4.ID, error) {
	f, err := os.Open(path)
	if err != nil {
		return c4.ID{}, err
	}
	defer f.Close()
	id, err := c4.IdentifyReader(f)
	if err != nil {
		return c4.ID{}, fmt.Errorf("read %s: %w", path, err)
	}
	return id, nil
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
// becomes durable. No-op for stores without batch semantics. A barrier
// that fails is fatal: every caller journals and prints a claim right
// after it, and a claim names bytes that are on stable media, so nothing
// may be claimed when the barrier did not hold. Until 2026-11-09 this
// only warned and the claim went out anyway (found in the storage project's review).
func syncStore(s store.Store) {
	if err := storeBarrier(s); err != nil {
		fatalf("Error: store sync failed; nothing claimed: %v", err)
	}
}

// storeBarrier runs the store's batch barrier and returns its error; nil
// for a nil store or one without batch semantics.
func storeBarrier(s store.Store) error {
	if s == nil {
		return nil
	}
	sy, ok := s.(store.Syncer)
	if !ok {
		return nil
	}
	return sy.Sync()
}
