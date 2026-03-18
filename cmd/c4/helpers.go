package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
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

// fatalf prints to stderr and exits.
func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
