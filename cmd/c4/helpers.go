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
// any patch chain fully resolved.
func loadManifest(path string) (*c4m.Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return c4m.NewDecoder(f).Decode()
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

// looksLikeC4m returns true if the raw bytes look like they might be a c4m file.
// Detection heuristic: if the first non-blank line starts with a valid mode
// character (-, d, l, or a 10-char Unix permission string), or whitespace
// followed by such characters, it's worth trying to parse.
func looksLikeC4m(data []byte) bool {
	// Find first non-blank line.
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimLeft(line, " ")
		if len(trimmed) == 0 {
			continue
		}
		ch := trimmed[0]
		return ch == '-' || ch == 'd' || ch == 'l' || ch == 'p' || ch == 's' || ch == 'b' || ch == 'c'
	}
	return false
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

// canonicalizeC4mBytes takes raw bytes, tries to parse as c4m, and if
// successful returns the canonical form bytes and the manifest. If parsing
// fails, returns nil, nil.
func canonicalizeC4mBytes(data []byte) ([]byte, *c4m.Manifest) {
	m := tryParseC4m(data)
	if m == nil {
		return nil, nil
	}
	canonical, err := c4m.Marshal(m)
	if err != nil {
		return nil, nil
	}
	return canonical, m
}

// identifyC4mFile reads a file, detects if it's c4m, and returns the
// canonical C4 ID. For c4m files, the ID is computed from the canonical form.
// For non-c4m files, the ID is computed from raw bytes.
// Returns the C4 ID and canonical bytes (non-nil only for c4m files).
func identifyC4mFile(path string) (c4.ID, []byte) {
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("Error reading %s: %v", path, err)
	}

	// Try c4m detection: check extension first, then heuristic.
	if strings.HasSuffix(path, ".c4m") || looksLikeC4m(data) {
		canonical, _ := canonicalizeC4mBytes(data)
		if canonical != nil {
			id := c4.Identify(bytes.NewReader(canonical))
			return id, canonical
		}
	}

	// Not c4m — identify raw bytes.
	id := c4.Identify(bytes.NewReader(data))
	return id, nil
}

// storeC4mAware stores content in the store with c4m canonicalization.
// If the content is c4m, stores the canonical form. Returns the C4 ID.
func storeC4mAware(s store.Store, path string) c4.ID {
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("Error reading %s: %v", path, err)
	}

	// Try c4m detection.
	if strings.HasSuffix(path, ".c4m") || looksLikeC4m(data) {
		canonical, _ := canonicalizeC4mBytes(data)
		if canonical != nil {
			id, err := s.Put(bytes.NewReader(canonical))
			if err != nil {
				fatalf("Error storing %s: %v", path, err)
			}
			return id
		}
	}

	// Not c4m — store raw bytes.
	id, err := s.Put(bytes.NewReader(data))
	if err != nil {
		fatalf("Error storing %s: %v", path, err)
	}
	return id
}

// storeContentC4mAware stores content from a reader in the store with c4m
// canonicalization. Reads all content into memory first to detect c4m.
// Returns the C4 ID.
func storeContentC4mAware(s store.Store, r io.Reader) (c4.ID, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return c4.ID{}, err
	}

	if looksLikeC4m(data) {
		canonical, _ := canonicalizeC4mBytes(data)
		if canonical != nil {
			return s.Put(bytes.NewReader(canonical))
		}
	}

	return s.Put(bytes.NewReader(data))
}

// fatalf prints to stderr and exits.
func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
