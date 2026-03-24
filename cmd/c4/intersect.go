package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
)

func runIntersect(args []string) {
	if len(args) == 0 {
		intersectUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "id":
		runIntersectID(args[1:])
	case "path":
		runIntersectPath(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "c4 intersect: unknown subcommand %q\n", args[0])
		intersectUsage()
		os.Exit(1)
	}
}

func intersectUsage() {
	fmt.Fprintf(os.Stderr, `c4 intersect — find common entries between c4m files

Usage:
  c4 intersect id <a> <b>      Match by C4 ID (content identity)
  c4 intersect path <a> <b>    Match by full path

Both arguments can be c4m files or directories.
Output is a valid c4m from the second argument's perspective.
`)
}

func runIntersectID(args []string) {
	fs := newFlags("intersect id")
	modeFlag := fs.stringFlag("mode", 'm', "f", "Scan mode for directories: s/m/f")
	fs.parse(args)

	if len(fs.args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 intersect id <a> <b>\n")
		os.Exit(1)
	}

	mode, err := scan.ParseScanMode(*modeFlag)
	if err != nil {
		fatalf("Error: %v", err)
	}

	aManifest := resolveManifestOrDir(fs.args[0], mode)
	bManifest := resolveManifestOrDir(fs.args[1], mode)

	// Build set of C4 IDs from manifest A.
	idSet := make(map[c4.ID]bool)
	for _, e := range aManifest.Entries {
		if e.C4ID.IsNil() || e.IsDir() {
			continue
		}
		idSet[e.C4ID] = true
	}

	// Walk manifest B and collect entries whose C4 ID is in the set.
	bPaths := c4m.EntryPaths(bManifest.Entries)
	matchedPaths := make(map[string]*c4m.Entry)
	for path, e := range bPaths {
		if e.IsDir() {
			continue
		}
		if idSet[e.C4ID] {
			matchedPaths[path] = e
		}
	}

	result := buildIntersectionManifest(matchedPaths)
	enc := c4m.NewEncoder(os.Stdout)
	enc.Encode(result)
}

func runIntersectPath(args []string) {
	fs := newFlags("intersect path")
	modeFlag := fs.stringFlag("mode", 'm', "f", "Scan mode for directories: s/m/f")
	fs.parse(args)

	if len(fs.args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 intersect path <a> <b>\n")
		os.Exit(1)
	}

	mode, err := scan.ParseScanMode(*modeFlag)
	if err != nil {
		fatalf("Error: %v", err)
	}

	aManifest := resolveManifestOrDir(fs.args[0], mode)
	bManifest := resolveManifestOrDir(fs.args[1], mode)

	// Build set of full paths from manifest A.
	aPaths := c4m.EntryPaths(aManifest.Entries)
	aPathSet := make(map[string]bool, len(aPaths))
	for p, e := range aPaths {
		if e.IsDir() {
			continue
		}
		aPathSet[p] = true
	}

	// Walk manifest B and collect entries whose path is in the set.
	bPaths := c4m.EntryPaths(bManifest.Entries)
	matchedPaths := make(map[string]*c4m.Entry)
	for path, e := range bPaths {
		if e.IsDir() {
			continue
		}
		if aPathSet[path] {
			matchedPaths[path] = e
		}
	}

	result := buildIntersectionManifest(matchedPaths)
	enc := c4m.NewEncoder(os.Stdout)
	enc.Encode(result)
}

// buildIntersectionManifest creates a valid c4m from a set of matched
// full-path entries, adding parent directories as needed.
func buildIntersectionManifest(matched map[string]*c4m.Entry) *c4m.Manifest {
	if len(matched) == 0 {
		return c4m.NewManifest()
	}

	// Ensure parent directories exist.
	all := make(map[string]*c4m.Entry, len(matched)*2)
	for p, e := range matched {
		clone := *e
		all[p] = &clone
	}
	ensureParentDirs(all)

	// Sort paths and rebuild manifest.
	paths := make([]string, 0, len(all))
	for p := range all {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	m := c4m.NewManifest()
	for _, p := range paths {
		e := all[p]
		e.Name = pathEntryName(p)
		e.Depth = pathToDepth(p)
		m.AddEntry(e)
	}
	m.SortEntries()
	return m
}

// ensureParentDirs adds directory entries for any implicit parent paths.
func ensureParentDirs(m map[string]*c4m.Entry) {
	var dirs []string
	for p := range m {
		parts := strings.Split(strings.TrimSuffix(p, "/"), "/")
		for i := 1; i < len(parts); i++ {
			dirPath := strings.Join(parts[:i], "/") + "/"
			if _, exists := m[dirPath]; !exists {
				dirs = append(dirs, dirPath)
			}
		}
	}
	for _, d := range dirs {
		if _, exists := m[d]; exists {
			continue
		}
		m[d] = &c4m.Entry{
			// Mode stays 0 (null) — directory-ness is indicated by trailing "/" in name.
			Size:      -1,
			Timestamp: c4m.NullTimestamp(),
			Name:      pathEntryName(d),
		}
	}
}
