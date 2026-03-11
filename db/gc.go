package db

import (
	"io"
	"os"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/hashlib"
)

// defaultMaxFilterMemory is the maximum bytes a CuckooFilter may consume
// before GC falls back to a map. 64 MB supports ~32M entries at ~2 bytes
// each, which covers all but the most extreme namespaces.
const defaultMaxFilterMemory = 64 << 20 // 64 MB

// GCResult summarizes a garbage collection run.
type GCResult struct {
	Checked      int   // blobs examined
	Live         int   // blobs reachable from live roots
	Collected    int   // blobs deleted
	FreedBytes   int64 // bytes reclaimed
	Errors       int   // deletion errors (non-fatal)
	MarkRetries  int   // number of filter capacity doublings before success
	UsedFallback bool  // true if mark phase used map fallback
}

// GC performs mark-and-sweep garbage collection. It traces all blobs
// reachable from the current root (and any held snapshots), then deletes
// unreachable blobs. Safe to call concurrently with reads and writes.
//
// The mark phase uses a CuckooFilter for O(1) membership probing at ~2
// bytes per entry. If the filter overflows, it retries with doubled
// capacity up to a memory ceiling (64 MB by default). If the ceiling is
// reached, it falls back to a plain map — slower and more memory, but
// guaranteed to complete. There is no failure mode where GC cannot
// determine the live set.
//
// CuckooFilter false positives (~0.012%) mean a dead blob may survive
// one extra cycle. This is safe: collected on the next pass.
func (db *DB) GC() GCResult {
	live, retries, fallback := db.markLive()
	result := db.sweep(live)
	result.MarkRetries = retries
	result.UsedFallback = fallback
	return result
}

// liveSet is implemented by both CuckooFilter and mapLiveSet,
// allowing the mark phase to use either without changing the sweep.
type liveSet interface {
	Has(id c4.ID) bool
}

// mapLiveSet is the guaranteed-completion fallback. It cannot overflow.
type mapLiveSet struct {
	m map[c4.ID]struct{}
}

func newMapLiveSet() *mapLiveSet {
	return &mapLiveSet{m: make(map[c4.ID]struct{})}
}

func (s *mapLiveSet) Add(id c4.ID) { s.m[id] = struct{}{} }
func (s *mapLiveSet) Has(id c4.ID) bool {
	_, ok := s.m[id]
	return ok
}

// markLive traces all C4 IDs reachable from the current root tree.
// It tries a CuckooFilter first (memory-efficient), retries with doubled
// capacity on overflow (up to a memory ceiling), and falls back to a map
// if the ceiling is reached. The map fallback guarantees completion.
func (db *DB) markLive() (liveSet, int, bool) {
	root := db.root.Load()

	// Estimate live set size: leaves + ~10 content blobs each + directory nodes.
	est := root.leafCount()*12 + root.size() + 256

	maxFilter := db.maxFilterMemory
	if maxFilter <= 0 {
		maxFilter = defaultMaxFilterMemory
	}

	// Try CuckooFilter with exponential growth, capped by memory.
	for attempt := 0; ; attempt++ {
		capacity := est * 2 * (1 << attempt)

		// Each CuckooFilter slot is 2 bytes (uint16), with 4 slots per bucket.
		// Memory ~ (capacity / 4) * 8 bytes (rounded up to power of 2).
		filterMemory := nextPow2(capacity/4) * 8
		if filterMemory > maxFilter {
			break // ceiling reached, fall back to map
		}

		live := hashlib.NewCuckooFilter(capacity)
		if db.fillLiveFilter(root, live) {
			return live, attempt, false
		}
	}

	// Map fallback: guaranteed completion, no overflow possible.
	live := newMapLiveSet()
	db.fillMapLiveSet(root, live)
	return live, -1, true
}

// fillLiveFilter populates a CuckooFilter with all live IDs.
// Returns true on success, false if the filter overflowed.
func (db *DB) fillLiveFilter(root *node, live *hashlib.CuckooFilter) bool {
	// The root c4m blob.
	rootData, _ := db.readFile(db.rootPointerPath())
	if rootID, err := c4.Parse(rootData); err == nil {
		if !live.Add(rootID) {
			return false
		}
	}

	// All leaf IDs and their referenced content blobs.
	var overflow bool
	root.walkLeaves(func(_ string, id c4.ID) {
		if overflow {
			return
		}
		if !live.Add(id) {
			overflow = true
			return
		}
		if !db.traceC4mRefsFilter(id, live) {
			overflow = true
		}
	})
	if overflow {
		return false
	}

	// All Merkle tree directory blobIDs.
	if !root.collectDirBlobIDs(live) {
		return false
	}

	// Snapshot root IDs.
	db.snapMu.Lock()
	for _, id := range db.snaps {
		if !id.IsNil() {
			if !live.Add(id) {
				db.snapMu.Unlock()
				return false
			}
		}
	}
	db.snapMu.Unlock()

	return true
}

// fillMapLiveSet populates a map with all live IDs. Cannot fail.
func (db *DB) fillMapLiveSet(root *node, live *mapLiveSet) {
	// The root c4m blob.
	rootData, _ := db.readFile(db.rootPointerPath())
	if rootID, err := c4.Parse(rootData); err == nil {
		live.Add(rootID)
	}

	// All leaf IDs and their referenced content blobs.
	root.walkLeaves(func(_ string, id c4.ID) {
		live.Add(id)
		db.traceC4mRefsMap(id, live)
	})

	// All Merkle tree directory blobIDs.
	root.collectDirBlobIDsMap(live)

	// Snapshot root IDs.
	db.snapMu.Lock()
	for _, id := range db.snaps {
		if !id.IsNil() {
			live.Add(id)
		}
	}
	db.snapMu.Unlock()
}

// traceC4mRefsFilter parses a c4m blob and adds referenced IDs to a
// CuckooFilter. Returns false if the filter overflows.
func (db *DB) traceC4mRefsFilter(id c4.ID, live *hashlib.CuckooFilter) bool {
	m := db.loadC4mFromStore(id)
	if m == nil {
		return true
	}
	for _, e := range m.Entries {
		if !e.C4ID.IsNil() {
			if !live.Add(e.C4ID) {
				return false
			}
		}
	}
	return true
}

// traceC4mRefsMap parses a c4m blob and adds referenced IDs to a map.
func (db *DB) traceC4mRefsMap(id c4.ID, live *mapLiveSet) {
	m := db.loadC4mFromStore(id)
	if m == nil {
		return
	}
	for _, e := range m.Entries {
		if !e.C4ID.IsNil() {
			live.Add(e.C4ID)
		}
	}
}

// loadC4mFromStore reads and parses a c4m blob. Returns nil if the blob
// is not in the store, unreadable, or not valid c4m.
func (db *DB) loadC4mFromStore(id c4.ID) *c4m.Manifest {
	rc, err := db.store.Get(id)
	if err != nil {
		return nil
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil
	}
	m, err := c4m.Unmarshal(data)
	if err != nil {
		return nil
	}
	return m
}

// sweep walks the store and deletes blobs not in the live set.
// Respects holds: held blobs are never deleted.
func (db *DB) sweep(live liveSet) GCResult {
	db.holdMu.RLock()
	held := make(map[c4.ID]struct{}, len(db.holds))
	for id := range db.holds {
		held[id] = struct{}{}
	}
	db.holdMu.RUnlock()

	var result GCResult

	db.store.Walk(func(id c4.ID) error {
		result.Checked++
		if live.Has(id) {
			result.Live++
			return nil
		}
		if _, ok := held[id]; ok {
			result.Live++ // held counts as live
			return nil
		}
		if err := db.store.Delete(id); err != nil {
			result.Errors++
			return nil
		}
		result.Collected++
		return nil
	})

	return result
}

// nextPow2 rounds up to the nearest power of 2.
func nextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

func (db *DB) readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
