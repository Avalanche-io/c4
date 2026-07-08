package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// runGC implements mark-and-sweep garbage collection for the local store.
// The keep-set roots are the c4m files named on the command line. Dry run
// by default; --force deletes. See design/store-gc.md.
func runGC(args []string) {
	fs := newFlags("gc")
	force := fs.boolFlag("force", 0, false, "Delete unreferenced objects (default is dry run)")
	verbose := fs.boolFlag("verbose", 'v', false, "List each unreferenced object")
	fs.parse(args)

	if len(fs.args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: c4 gc [--force] [-v] <file.c4m>...\n")
		fmt.Fprintf(os.Stderr, "\nMark-and-sweep the local store: keep objects reachable from the\nnamed c4m files, collect the rest. Dry run unless --force.\n")
		os.Exit(1)
	}

	// Parse all roots before touching the store: any failure is fatal.
	marked := markRoots(fs.args)

	s, err := store.OpenConfigured()
	if err != nil {
		fatalf("Error opening store: %v", err)
	}
	if s == nil {
		fatalf("No local store configured. Set C4_STORE=/path/to/store (gc is local-only).")
	}

	markClosure(s, marked)

	// Sweep: everything in the store that is not marked is garbage.
	type object struct {
		id   c4.ID
		size int64
	}
	var total, reachable int
	var totalBytes, reachableBytes, garbageBytes int64
	var garbage []object
	err = s.Walk(func(id c4.ID, size int64) error {
		total++
		totalBytes += size
		if marked[id] {
			reachable++
			reachableBytes += size
			return nil
		}
		garbageBytes += size
		garbage = append(garbage, object{id, size})
		return nil
	})
	if err != nil {
		fatalf("Error walking store: %v", err)
	}

	if *verbose {
		for _, g := range garbage {
			fmt.Println(g.id)
		}
		if len(garbage) > 0 {
			fmt.Println()
		}
	}

	fmt.Printf("objects    %12s   (%s)\n", commaFormat(int64(total)), formatBytes(totalBytes))
	fmt.Printf("reachable  %12s   (%s)\n", commaFormat(int64(reachable)), formatBytes(reachableBytes))
	fmt.Printf("garbage    %12s   (%s)\n", commaFormat(int64(len(garbage))), formatBytes(garbageBytes))

	if !*force {
		fmt.Printf("\nDry run — nothing deleted. Re-run with --force to delete.\n")
		return
	}

	var deleted int
	var deletedBytes int64
	failed := 0
	for _, g := range garbage {
		if err := s.Remove(g.id); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", g.id, err)
			failed++
			continue
		}
		deleted++
		deletedBytes += g.size
	}
	fmt.Printf("\ndeleted %s objects (%s)\n", commaFormat(int64(deleted)), formatBytes(deletedBytes))
	if failed > 0 {
		os.Exit(1)
	}
}

// markRoots parses each named c4m file and returns the initial mark set:
// every C4 ID appearing anywhere in any root (entry IDs, bare chain/base
// IDs, sequence range data), plus the raw and canonical IDs of the roots
// themselves so stored copies of kept descriptions survive.
//
// Any parse failure is fatal. A root set that references no objects at all
// is refused — an empty keep-set is never a license to collect everything.
func markRoots(paths []string) map[c4.ID]bool {
	marked := make(map[c4.ID]bool)
	var selfIDs []c4.ID
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			fatalf("Error reading %s: %v", path, err)
		}
		canonical, m := canonicalizeC4mBytes(data)
		if m == nil {
			fatalf("Error parsing %s: not a valid c4m file", path)
		}
		for _, id := range scanIDs(data) {
			marked[id] = true
		}
		selfIDs = append(selfIDs,
			c4.Identify(bytes.NewReader(data)),
			c4.Identify(bytes.NewReader(canonical)))
	}
	if len(marked) == 0 {
		fatalf("Refusing to collect: the given c4m files reference no C4 IDs (empty keep-set).")
	}
	for _, id := range selfIDs {
		marked[id] = true
	}
	return marked
}

// markClosure expands the mark set transitively: any marked object whose
// stored content is itself a c4m description (directory records, external
// base manifests, chain blocks) is read and every ID it references is
// marked in turn, until no new IDs appear.
func markClosure(s *store.TreeStore, marked map[c4.ID]bool) {
	queue := make([]c4.ID, 0, len(marked))
	for id := range marked {
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		data := readIfDescription(s, id)
		if data == nil {
			continue
		}
		for _, ref := range scanIDs(data) {
			if marked[ref] {
				continue
			}
			marked[ref] = true
			queue = append(queue, ref)
		}
	}
}

// readIfDescription returns the stored content for id if it looks like a
// c4m description, nil otherwise (missing, unreadable, or plain content).
func readIfDescription(s *store.TreeStore, id c4.ID) []byte {
	rc, err := s.Open(id)
	if err != nil {
		return nil
	}
	defer rc.Close()

	prefix := make([]byte, 512)
	n, _ := io.ReadFull(rc, prefix)
	prefix = prefix[:n]
	if !looksLikeDescription(prefix) {
		return nil
	}
	rest, err := io.ReadAll(rc)
	if err != nil {
		return nil
	}
	return append(prefix, rest...)
}

// looksLikeDescription reports whether the leading bytes could open a c4m
// description: an entry line, or a bare C4 ID line (chain blocks and
// external base references). False positives only cost an extra read and
// over-marking — safe for GC, which must never under-mark.
func looksLikeDescription(prefix []byte) bool {
	if looksLikeC4m(prefix) {
		return true
	}
	fields := bytes.Fields(prefix)
	if len(fields) == 0 || len(fields[0]) != 90 || fields[0][0] != 'c' || fields[0][1] != '4' {
		return false
	}
	_, err := c4.Parse(string(fields[0]))
	return err == nil
}

// scanIDs returns every parseable C4 ID token in data. Marking is
// deliberately token-based: over-marking retains at worst a few extra
// objects, under-marking destroys data.
func scanIDs(data []byte) []c4.ID {
	var ids []c4.ID
	for _, f := range bytes.Fields(data) {
		if len(f) != 90 || f[0] != 'c' || f[1] != '4' {
			continue
		}
		id, err := c4.Parse(string(f))
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}
