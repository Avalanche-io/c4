package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
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
	for path, entry := range scanPaths {
		if entry.IsDir() {
			continue
		}
		refEntry, ok := refPaths[path]
		if ok && !refEntry.C4ID.IsNil() &&
			entry.Size == refEntry.Size &&
			entry.Timestamp.Equal(refEntry.Timestamp) {
			entry.C4ID = refEntry.C4ID
		} else if mode == scan.ModeFull {
			fullPath := filepath.Join(dirPath, filepath.FromSlash(path))
			f, err := os.Open(fullPath)
			if err != nil {
				continue
			}
			entry.C4ID = c4.Identify(f)
			f.Close()
		}
	}

	return metaManifest
}

// fatalf prints to stderr and exits.
func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
